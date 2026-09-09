#!/bin/bash
# ============================================================
# Seedance 2.0 简化测试脚本（仅测试本地 new-api）
# 对应文档: docs/咪咕/ARK-VIDEO-API.zh-CN.md
# 接口路径:
#   创建: POST /v1/video/generations
#   查询: GET  /v1/videos/{task_id}
# ============================================================

# ---------- 帮助信息 ----------
show_help() {
  cat <<'EOF'

Seedance 2.0 测试脚本
============================================================

用法:
  ./test-seedance-simple.sh --execute [选项]
  ./test-seedance-simple.sh --fetch-ark <task_id> [选项]
  ./test-seedance-simple.sh --test-asset [选项]

命令:
  --execute          执行完整测试（创建素材 + 提交视频任务 + 轮询状态）
  --fetch-ark        使用 ARK 格式查询任务（会实时调用上游）
  --test-asset       仅测试素材上传（上传 + 轮询状态，不生成视频）

必需参数:
  需要指定 --execute 或 --fetch-ark 或 --test-asset 其中之一

资产接口格式选项:
  ASSET_API_FORMAT=restful (默认)
      使用 RESTful 风格的资产接口
      创建素材: POST /api/seedance/assets
      查询素材: GET  /api/seedance/assets/{id}
      适用于: Gateway 上游 (咪咕)、KWJM 上游的 RESTful 模式

  ASSET_API_FORMAT=action
      使用 Action 风格的资产接口
      创建素材: POST /api/seedance/assets/v2/?Action=CreateAsset&Version=2024-01-01
      查询素材: POST /api/seedance/assets/v2/?Action=GetAsset&Version=2024-01-01
      适用于: 火山官方 API、KWJM 上游的 Action 模式

环境变量配置:
  NEWAPI_BASE_URL    本地 new-api 地址 (默认: http://book2:3002)
  NEWAPI_API_KEY     API 密钥 (默认: sk-zoPYrUW81cYIFdmcD8JHvhLtGWdTvKh41vBBJW8KSBvzWxVu)
  MODEL              视频模型 (默认: doubao-seedance-2-0-sd)
  PROMPT             提示词 (默认: 女子人物面对镜头自然说话...)
  DURATION           视频时长秒数 (默认: 4)
  RESOLUTION         分辨率 (默认: 480p)
  RATIO              宽高比 (默认: 16:9)
  ROLE_IMAGE_URL     角色图片 URL (需要上传到资产库)
  IMAGE_URL          场景图片 URL
  AUDIO_URL          参考音频 URL
  VIDEO_URL          参考视频 URL

使用示例:

  # 示例 1: 测试 RESTful 格式（默认）
  ./test-seedance-simple.sh --execute

  # 示例 2: 测试 RESTful 格式（显式指定）
  ASSET_API_FORMAT=restful ./test-seedance-simple.sh --execute

  # 示例 3: 测试 Action 格式
  ASSET_API_FORMAT=action ./test-seedance-simple.sh --execute

  # 示例 4: 自定义角色图和场景图
  ROLE_IMAGE_URL="https://example.com/role.png" \
  IMAGE_URL="https://example.com/scene.png" \
  ./test-seedance-simple.sh --execute

  # 示例 5: 测试不同模型
  MODEL="doubao-seedance-2-0-hd" ./test-seedance-simple.sh --execute

  # 示例 6: 使用 ARK 格式查询任务（实时调用上游）
  ./test-seedance-simple.sh --fetch-ark task_5uIdqhOnT05FvKvRzG0ogqPvjp6TWnG6

注意事项:
  - 首次运行会自动上传角色图到资产库
  - 资产激活可能需要等待 10-30 秒
  - 视频生成根据时长和分辨率，通常需要 1-3 分钟
  - 如遇到素材未激活超时，可稍后重试

============================================================

EOF
  exit 0
}

# ---------- 颜色 ----------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ---------- 检查参数 ----------
if [[ "$1" == "--fetch-ark" ]]; then
  # ARK 格式查询任务模式
  TASK_ID="$2"
  if [[ -z "$TASK_ID" ]]; then
    echo -e "${RED}错误: 缺少任务 ID${NC}"
    echo "用法: $0 --fetch-ark <task_id>"
    echo "示例: $0 --fetch-ark task_5uIdqhOnT05FvKvRzG0ogqPvjp6TWnG6"
    exit 1
  fi
  MODE="fetch-ark"
elif [[ "$1" == "--execute" ]]; then
  MODE="execute"
elif [[ "$1" == "--test-asset" ]]; then
  MODE="test-asset"
else
  show_help
fi

# ---------- 配置区（按需修改）----------
NEWAPI_BASE_URL="${NEWAPI_BASE_URL:-http://book2:3000}"
NEWAPI_API_KEY="${NEWAPI_API_KEY:-sk-2V6P5nj3JLnJrSprBxHe4pdwkttZEFJxYPeYcVjCK7g7QHXO}"

# 资产接口格式：
#   "restful" - RESTful 格式（默认）: POST /api/seedance/assets
#   "action"  - Action 格式: POST /api/seedance/assets/v2/?Action=CreateAsset&Version=2024-01-01
ASSET_API_FORMAT="${ASSET_API_FORMAT:-restful}"

# 视频模型 260128
MODEL="${MODEL:-doubao-seedance-2-0-sd}"

# 请求参数（Ark 原生格式，字段在根级）
PROMPT="${PROMPT:-女子人物面对镜头自然说话（使用音频1声音）："你好，我是归一体验官"}"
DURATION="${DURATION:-4}"           # integer，不是字符串
RESOLUTION="${RESOLUTION:-480p}"
RATIO="${RATIO:-16:9}"
GENERATE_AUDIO="${GENERATE_AUDIO:-true}"
WATERMARK="${WATERMARK:-false}"

# 参考图 URL（场景图，直接传 URL，留空则不传）
IMAGE_URL="${IMAGE_URL:-https://static.horse-world.mints-id.com/rh/20260604204600/1780577160529_9921.png}"
# 参考视频 URL（留空则不传）
VIDEO_URL="${VIDEO_URL:-}"
# 参考音频 URL（留空则不传）
AUDIO_URL="${AUDIO_URL:-https://static.horse-world.mints-id.com/audio/trim/f0eab3ec-2bbc-49cb-8e26-59ca050ceaf2.wav}"
# 角色图片 URL（会先上传到资产库，激活后用 asset:// 引用；留空则跳过资产库上传）
ROLE_IMAGE_URL="${ROLE_IMAGE_URL:-https://static.horse-world.mints-id.com/rh/20260602024700/1780339620793_5209.png}"
# 直接使用已有的资产 ID（格式: asset-xxxxxxxx，设置后跳过上传，优先级高于 ROLE_IMAGE_URL）
ASSET_ID="${ASSET_ID:-}"

# ---------- 颜色 ----------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

log_info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_sep()   { echo -e "${CYAN}============================================================${NC}"; }
log_title() { echo -e "\n${BOLD}${CYAN}$*${NC}"; log_sep; }

check_deps() {
  for cmd in curl jq bc; do
    command -v "$cmd" &>/dev/null || { log_error "$cmd 未安装"; exit 1; }
  done
}

# ---------- 积分转元（500000 积分 = 1 元）----------
quota_to_yuan() {
  local q="$1"
  if [ "$q" = "null" ] || [ "$q" = "-1" ] || [ -z "$q" ]; then echo "N/A"; return; fi
  echo "scale=6; $q / 500000" | bc 2>/dev/null || echo "N/A"
}

# ---------- 构建 content 数组 ----------
build_content_array() {
  # 始终包含 text
  local content
  content=$(jq -n --arg text "$PROMPT" '[{"type":"text","text":$text}]')

  # 追加角色图资产引用（asset://asset-xxx）
  if [ -n "$ASSET_REF" ]; then
    content=$(echo "$content" | jq \
      --arg url "$ASSET_REF" \
      '. + [{"type":"image_url","image_url":{"url":$url}}]')
  fi

  # 追加场景图 image_url（如果配置了）
  if [ -n "$IMAGE_URL" ]; then
    content=$(echo "$content" | jq \
      --arg url "$IMAGE_URL" \
      '. + [{"type":"image_url","image_url":{"url":$url}}]')
  fi

  # 追加 video_url（如果配置了）
  if [ -n "$VIDEO_URL" ]; then
    content=$(echo "$content" | jq \
      --arg url "$VIDEO_URL" \
      '. + [{"type":"video_url","video_url":{"url":$url}}]')
  fi

  # 追加 audio_url（如果配置了）
  if [ -n "$AUDIO_URL" ]; then
    content=$(echo "$content" | jq \
      --arg url "$AUDIO_URL" \
      '. + [{"type":"audio_url","audio_url":{"url":$url}}]')
  fi

  echo "$content"
}

# ---------- 提交视频任务（Ark 原生格式）----------
submit_task() {
  local content_array
  content_array=$(build_content_array)

  local body
  body=$(jq -n \
    --arg model "$MODEL" \
    --arg resolution "$RESOLUTION" \
    --arg ratio "$RATIO" \
    --argjson duration "$DURATION" \
    --argjson generate_audio "$GENERATE_AUDIO" \
    --argjson watermark "$WATERMARK" \
    --argjson content "$content_array" \
    '{
      model: $model,
      content: $content,
      resolution: $resolution,
      ratio: $ratio,
      duration: $duration,
      generate_audio: $generate_audio,
      watermark: $watermark
    }')

  log_info "请求 URL: POST ${NEWAPI_BASE_URL}/v1/video/generations"
  log_info "提交请求体（Ark 原生格式）:"
  echo "$body" | jq '.'
  echo ""

  local tmpfile; tmpfile=$(mktemp)
  HTTP_CODE=$(curl -s -o "$tmpfile" -w "%{http_code}" \
    -X POST "${NEWAPI_BASE_URL}/v1/video/generations" \
    -H "Authorization: Bearer ${NEWAPI_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$body")
  SUBMIT_BODY=$(cat "$tmpfile"); rm -f "$tmpfile"
}

# ---------- 轮询任务状态（Ark 原生查询路径）----------
poll_task() {
  local task_id="$1"
  local max_retries="${2:-40}"
  local interval="${3:-10}"
  local retry=0

  log_info "开始轮询 task_id=${task_id}，间隔 ${interval}s，最多 ${max_retries} 次"
  log_info "查询 URL: GET ${NEWAPI_BASE_URL}/v1/videos/${task_id}"

  while [ $retry -lt $max_retries ]; do
    retry=$((retry + 1))
    sleep "$interval"

    local tmpfile; tmpfile=$(mktemp)
    curl -s -o "$tmpfile" \
      -X GET "${NEWAPI_BASE_URL}/v1/videos/${task_id}" \
      -H "Authorization: Bearer ${NEWAPI_API_KEY}"
    POLL_BODY=$(cat "$tmpfile"); rm -f "$tmpfile"

    local status progress
    status=$(echo "$POLL_BODY" | jq -r '.status // "unknown"' 2>/dev/null)
    progress=$(echo "$POLL_BODY" | jq -r '.progress // ""' 2>/dev/null)

    log_info "#${retry} 状态: ${status} ${progress}"

    case "$status" in
      "succeeded"|"completed"|"success")
        FINAL_TASK_BODY="$POLL_BODY"
        return 0
        ;;
      "failed"|"error")
        log_error "任务失败"
        echo "$POLL_BODY" | jq '.error // .'
        FINAL_TASK_BODY="$POLL_BODY"
        return 1
        ;;
    esac
  done

  log_warn "轮询超时，任务可能仍在处理"
  FINAL_TASK_BODY="$POLL_BODY"
  return 2
}

# ---------- 查询日志 ----------
query_log() {
  local base_url="$1"
  local token="$2"
  local start_ts="$3"
  local model_name="$4"

  if [ -z "$token" ] || [ -z "$base_url" ]; then
    echo ""
    return
  fi

  local end_ts
  end_ts=$(date +%s)

  curl -s \
    "${base_url}/api/log/self?type=1&model_name=${model_name}&start_timestamp=${start_ts}&end_timestamp=${end_ts}&p=1&page_size=5" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" 2>/dev/null
}

# ============================================================
# 主流程
# ============================================================

check_deps

# 如果是 ARK 查询模式，跳过主流程
if [[ "$MODE" == "fetch-ark" ]]; then
  # ARK 查询逻辑在后面
  :
else
  # 执行完整测试流程
  echo ""
  log_sep
  echo -e "${BOLD}  Seedance 2.0 简化测试（仅本地 new-api）${NC}"
  log_sep
  echo -e "  本端 new-api:  ${NEWAPI_BASE_URL}"
  echo -e "  模型:          ${MODEL}"
  echo -e "  资产接口格式:  ${ASSET_API_FORMAT}"
  echo -e "  Prompt:        ${PROMPT}"
  echo -e "  时长/分辨率:   ${DURATION}s / ${RESOLUTION} / ${RATIO}"
  echo -e "  音频生成:      ${GENERATE_AUDIO}  水印: ${WATERMARK}"
  [ -n "$IMAGE_URL" ] && echo -e "  场景图:        ${IMAGE_URL}"
  [ -n "$ROLE_IMAGE_URL" ] && [ -z "$ASSET_ID" ] && echo -e "  角色图(资产库):${ROLE_IMAGE_URL}"
  [ -n "$ASSET_ID" ] && echo -e "  角色图(AssetID):${ASSET_ID}"
  [ -n "$VIDEO_URL" ] && echo -e "  参考视频:      ${VIDEO_URL}"
  [ -n "$AUDIO_URL" ] && echo -e "  参考音频:      ${AUDIO_URL}"
  log_sep

# ---------- Step 0: 上传角色图片到资产库 ----------
ASSET_REF=""
if [ -n "$ASSET_ID" ]; then
  # 如果指定了 ASSET_ID，直接使用，跳过上传
  log_title "Step 0 · 使用指定的资产 ID"
  ASSET_REF="asset://${ASSET_ID}"
  log_ok "使用资产引用: ${ASSET_REF}"
elif [ -n "$ROLE_IMAGE_URL" ]; then
  log_title "Step 0 · 上传角色图片到资产库"

  log_info "上传角色图: ${ROLE_IMAGE_URL}"

  # 根据接口格式选择不同的路径和请求体
  if [ "$ASSET_API_FORMAT" = "action" ]; then
    # Action 格式
    ASSET_URL="${NEWAPI_BASE_URL}/api/seedance/assets/v2/?Action=CreateAsset&Version=2024-01-01"
    log_info "请求 URL: POST ${ASSET_URL}"

    ASSET_REQ_BODY=$(jq -n --arg url "$ROLE_IMAGE_URL" --arg name "role-image" \
      '{URL:$url,AssetType:"Image",Name:$name}')
  else
    # RESTful 格式（默认）
    ASSET_URL="${NEWAPI_BASE_URL}/api/seedance/assets"
    log_info "请求 URL: POST ${ASSET_URL}"

    ASSET_REQ_BODY=$(jq -n --arg url "$ROLE_IMAGE_URL" --arg name "role-image" \
      '{URL:$url,AssetType:"Image",Name:$name}')
  fi

  log_info "请求体:"
  echo "$ASSET_REQ_BODY" | jq '.'

  ASSET_RESP=$(curl -s -X POST "${ASSET_URL}" \
    -H "Authorization: Bearer ${NEWAPI_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$ASSET_REQ_BODY")

  log_info "响应体:"
  echo "$ASSET_RESP" | jq '.' 2>/dev/null || echo "$ASSET_RESP"

  ASSET_LOCAL_ID=$(echo "$ASSET_RESP" | jq -r '.Result.LocalId // empty' 2>/dev/null)
  ASSET_REF=$(echo "$ASSET_RESP" | jq -r '.Result.AssetRef // empty' 2>/dev/null)
  ASSET_UPSTREAM_ID=$(echo "$ASSET_RESP" | jq -r '.Result.Id // empty' 2>/dev/null)
  ASSET_STATUS=$(echo "$ASSET_RESP" | jq -r '.Result.Status // "unknown"' 2>/dev/null)

  if [ -z "$ASSET_UPSTREAM_ID" ]; then
    log_error "上传素材失败，未获取到 asset_id"
    exit 1
  fi
  log_ok "素材已提交: local_id=${ASSET_LOCAL_ID}, asset_id=${ASSET_UPSTREAM_ID}, status=${ASSET_STATUS}"

  # 如果上传响应中状态已经是 Active，跳过轮询
  if [ "$ASSET_STATUS" = "Active" ]; then
    log_ok "素材已激活，引用: ${ASSET_REF}"
  else
    # 轮询等待资产激活
    log_info "等待素材激活（当前状态: ${ASSET_STATUS}）..."
    ASSET_MAX=30
    ASSET_RETRY=0
    while [ $ASSET_RETRY -lt $ASSET_MAX ]; do
      ASSET_RETRY=$((ASSET_RETRY + 1))
      sleep 3

      # 根据接口格式选择不同的查询路径
      if [ "$ASSET_API_FORMAT" = "action" ]; then
        ASSET_QUERY_URL="${NEWAPI_BASE_URL}/api/seedance/assets/v2/?Action=GetAsset&Version=2024-01-01"
        log_info "查询 URL: POST ${ASSET_QUERY_URL}"

        # Action 格式需要 POST + Body
        ASSET_QUERY_BODY=$(jq -n --arg id "$ASSET_UPSTREAM_ID" '{Id:$id}')
        ASSET_STATUS_RESP=$(curl -s -X POST "${ASSET_QUERY_URL}" \
          -H "Authorization: Bearer ${NEWAPI_API_KEY}" \
          -H "Content-Type: application/json" \
          -d "$ASSET_QUERY_BODY")
      else
        # RESTful 格式使用 GET
        ASSET_QUERY_URL="${NEWAPI_BASE_URL}/api/seedance/assets/${ASSET_UPSTREAM_ID}"
        log_info "查询 URL: GET ${ASSET_QUERY_URL}"

        ASSET_STATUS_RESP=$(curl -s "${ASSET_QUERY_URL}" \
          -H "Authorization: Bearer ${NEWAPI_API_KEY}")
      fi

      log_info "#${ASSET_RETRY} 响应体:"
      echo "$ASSET_STATUS_RESP" | jq '.' 2>/dev/null || echo "$ASSET_STATUS_RESP"

      ASSET_STATUS=$(echo "$ASSET_STATUS_RESP" | jq -r '.Result.Status // "unknown"')
      log_info "#${ASSET_RETRY} 素材状态: ${ASSET_STATUS}"

      case "$ASSET_STATUS" in
        "Active")
          log_ok "素材已激活，引用: ${ASSET_REF}"
          break
          ;;
        "Failed")
          log_error "素材处理失败"
          echo "$ASSET_STATUS_RESP" | jq '.'
          exit 1
          ;;
      esac
    done

    if [ "$ASSET_STATUS" != "Active" ]; then
      log_error "素材激活超时，退出"
      exit 1
    fi
  fi
fi

# ---------- Step 1: 提交任务 ----------
log_title "Step 1 · 提交 Seedance 任务（POST /v1/video/generations）"

SUBMIT_TS=$(date +%s)
submit_task

echo ""
log_info "HTTP ${HTTP_CODE}"
echo "$SUBMIT_BODY" | jq '.'

TASK_ID=$(echo "$SUBMIT_BODY" | jq -r '.id // .task_id // empty' 2>/dev/null)
SUBMIT_STATUS=$(echo "$SUBMIT_BODY" | jq -r '.status // "unknown"' 2>/dev/null)

if [ -z "$TASK_ID" ]; then
  log_error "未获取到 task_id，提交失败"
  exit 1
fi

log_ok "task_id = ${TASK_ID}"
log_info "提交响应状态 = ${SUBMIT_STATUS}"
log_info "提交时间戳 = ${SUBMIT_TS}"

# ---------- Step 2: 轮询等待完成 ----------
log_title "Step 2 · 轮询任务状态"

FINAL_TASK_BODY=""
poll_task "$TASK_ID" 40 10
POLL_RESULT=$?

FINISH_TS=$(date +%s)

if [ $POLL_RESULT -eq 1 ]; then
  log_error "任务失败，退出"
  exit 1
fi

echo ""
log_info "最终任务详情:"
echo "$FINAL_TASK_BODY" | jq '.'

# 视频地址在 content.video_url（Ark 原生响应格式）
VIDEO_URL_RESULT=$(echo "$FINAL_TASK_BODY" | jq -r '.content.video_url // .metadata.url // "N/A"' 2>/dev/null)

# ---------- Step 3: 查询本端日志 ----------
log_title "Step 3 · 查询本端扣费日志"

sleep 3

NEWAPI_LOG_RESP=$(query_log "$NEWAPI_BASE_URL" "Bearer $NEWAPI_API_KEY" "$SUBMIT_TS" "$MODEL")

if [ -z "$NEWAPI_LOG_RESP" ]; then
  log_warn "未配置 NEWAPI_API_KEY，跳过本端日志查询"
  NEWAPI_QUOTA="N/A"
else
  log_info "本端最近日志条目:"
  echo "$NEWAPI_LOG_RESP" | jq '.data.items[0] // .data[0] // "无日志"'
  NEWAPI_QUOTA=$(echo "$NEWAPI_LOG_RESP" | jq -r '(.data.items[0].quota // .data[0].quota // "N/A") | tostring' 2>/dev/null)
  NEWAPI_YUAN=$(quota_to_yuan "$NEWAPI_QUOTA")
  log_ok "本端实际扣费: ${NEWAPI_QUOTA} 积分 ≈ ${NEWAPI_YUAN} 元"
fi

# ---------- Step 4: 汇总报告 ----------
log_title "Step 4 · 测试报告"

echo ""
echo -e "  task_id:         ${TASK_ID}"
echo -e "  提交响应状态:   ${SUBMIT_STATUS}"
echo -e "  耗时:            $((FINISH_TS - SUBMIT_TS))s"
echo ""

[ "$NEWAPI_QUOTA" != "N/A" ] && \
  echo -e "  ${BOLD}本端扣费:${NC}  ${NEWAPI_QUOTA} 积分 ≈ ${NEWAPI_YUAN} 元"

if [ "$VIDEO_URL_RESULT" != "N/A" ] && [ -n "$VIDEO_URL_RESULT" ]; then
  echo ""
  log_ok "视频地址: ${VIDEO_URL_RESULT}"
fi

echo ""
log_sep
log_ok "测试完成"
log_sep
fi  # 结束 execute 模式

# ============================================================
# ARK 格式查询任务模式
# ============================================================

if [[ "$MODE" == "fetch-ark" ]]; then
  echo ""
  log_sep
  echo -e "${BOLD}  ARK 格式任务查询（实时调用上游）${NC}"
  log_sep
  echo -e "  本端 new-api:  ${NEWAPI_BASE_URL}"
  echo -e "  任务 ID:       ${TASK_ID}"
  log_sep

  # 根据配置确定上游类型和路径
  log_info "检测上游配置..."

  # 默认使用 Gateway 格式路径
  ARK_PATH="/api/v3/contents/generations/tasks"

  # 如果配置了 KWJM，使用 KWJM 路径
  # 注意：这里需要用户手动指定或自动检测
  # 为了简化，我们提供两种查询方式

  echo ""
  log_title "方式 1: Doubao 官方格式查询（ARK 格式）"

  ARK_URL="${NEWAPI_BASE_URL}/api/v3/contents/generations/tasks/${TASK_ID}"
  log_info "查询 URL: GET ${ARK_URL}"
  log_info "说明: 此格式仅在 task.Data 不完整时才会调用上游"

  ARK_RESP=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${ARK_URL}" \
    -H "Authorization: Bearer ${NEWAPI_API_KEY}")

  ARK_HTTP_CODE=$(echo "$ARK_RESP" | grep "HTTP_CODE:" | cut -d: -f2)
  ARK_BODY=$(echo "$ARK_RESP" | sed '/HTTP_CODE:/d')

  echo ""
  log_info "HTTP ${ARK_HTTP_CODE}"
  log_info "响应体:"
  echo "$ARK_BODY" | jq '.' 2>/dev/null || echo "$ARK_BODY"

  if [[ "$ARK_HTTP_CODE" == "200" ]]; then
    ARK_STATUS=$(echo "$ARK_BODY" | jq -r '.status // "unknown"')
    log_ok "任务状态: ${ARK_STATUS}"
  else
    log_warn "ARK 格式查询失败"
  fi

  echo ""
  log_title "方式 2: OpenAI Video API 格式查询（对比）"

  OPENAI_URL="${NEWAPI_BASE_URL}/v1/videos/${TASK_ID}"
  log_info "查询 URL: GET ${OPENAI_URL}"
  log_info "说明: 此格式不会调用上游，仅读取数据库"

  OPENAI_RESP=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${OPENAI_URL}" \
    -H "Authorization: Bearer ${NEWAPI_API_KEY}")

  OPENAI_HTTP_CODE=$(echo "$OPENAI_RESP" | grep "HTTP_CODE:" | cut -d: -f2)
  OPENAI_BODY=$(echo "$OPENAI_RESP" | sed '/HTTP_CODE:/d')

  echo ""
  log_info "HTTP ${OPENAI_HTTP_CODE}"
  log_info "响应体:"
  echo "$OPENAI_BODY" | jq '.' 2>/dev/null || echo "$OPENAI_BODY"

  if [[ "$OPENAI_HTTP_CODE" == "200" ]]; then
    OPENAI_STATUS=$(echo "$OPENAI_BODY" | jq -r '.status // "unknown"')
    log_ok "任务状态（本地数据库）: ${OPENAI_STATUS}"
  fi

  echo ""
  log_sep
  echo -e "${BOLD}查询结果对比:${NC}"
  log_sep
  echo -e "  Doubao 官方格式 (ARK):  HTTP ${ARK_HTTP_CODE}  状态: ${ARK_STATUS:-N/A}"
  echo -e "  OpenAI Video API 格式:  HTTP ${OPENAI_HTTP_CODE}  状态: ${OPENAI_STATUS:-N/A}"
  log_sep

  echo ""
  log_info "说明:"
  echo "  - Doubao 官方格式 (ARK) 仅在 task.Data 不完整时才会调用上游"
  echo "  - 正常情况下，task.Data 是完整的，所以两种格式都只读数据库"
  echo "  - OpenAI Video API 格式永远不会调用上游，总是读取数据库"
  echo "  - 两种格式返回的数据格式略有不同（字段命名等）"
  echo ""
  log_info "task.Data 何时不完整?"
  echo "  - 提交任务时上游响应保存失败"
  echo "  - 数据库损坏或字段丢失"
  echo "  - 手动测试场景（人为清空数据）"
  echo ""
  log_info "如何验证是否调用了上游:"
  echo "  tail -f logs/new-api.log | grep 'Doubao实时查询'"
  echo "  如果看到此日志，说明调用了上游；否则只是读了数据库"

  exit 0
fi

# ============================================================
# 素材上传测试模式
# ============================================================
if [[ "$MODE" == "test-asset" ]]; then
  log_title "素材上传测试模式"

  # 确保有角色图 URL
  if [[ -z "$ROLE_IMAGE_URL" ]]; then
    log_error "缺少角色图 URL，请设置 ROLE_IMAGE_URL 环境变量"
    echo "示例: ROLE_IMAGE_URL=https://example.com/image.jpg ./test-seedance-simple.sh --test-asset"
    exit 1
  fi

  log_info "API Base URL: ${NEWAPI_BASE_URL}"
  log_info "角色图 URL: ${ROLE_IMAGE_URL}"
  log_info "素材接口格式: ${ASSET_API_FORMAT}"
  if [[ -n "$ASSET_MODEL" ]]; then
    log_info "指定模型: ${ASSET_MODEL}"
  else
    log_warn "未指定模型（将使用默认渠道选择逻辑）"
  fi
  echo ""

  # ---------- 步骤 1: 上传素材 ----------
  log_title "步骤 1: 上传角色图素材"

  if [[ "$ASSET_API_FORMAT" == "action" ]]; then
    # Action 格式
    ASSET_CREATE_URL="${NEWAPI_BASE_URL}/api/seedance/assets/v2/?Action=CreateAsset&Version=2024-01-01"
    log_info "使用 Action 格式"
  else
    # RESTful 格式
    ASSET_CREATE_URL="${NEWAPI_BASE_URL}/api/seedance/assets"
    log_info "使用 RESTful 格式"
  fi

  log_info "POST ${ASSET_CREATE_URL}"

  # 构造请求体
  ASSET_REQ_BODY=$(jq -n \
    --arg url "$ROLE_IMAGE_URL" \
    --arg model "$ASSET_MODEL" \
    '{
      URL: $url,
      AssetType: "Image",
      Name: "test-role-image"
    } + (if $model != "" then {Model: $model} else {} end)')

  log_info "请求体:"
  echo "$ASSET_REQ_BODY" | jq '.'
  echo ""

  ASSET_RESP=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${ASSET_CREATE_URL}" \
    -H "Authorization: Bearer ${NEWAPI_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$ASSET_REQ_BODY")

  ASSET_HTTP_CODE=$(echo "$ASSET_RESP" | grep "HTTP_CODE:" | cut -d: -f2)
  ASSET_BODY=$(echo "$ASSET_RESP" | sed '/HTTP_CODE:/d')

  log_info "HTTP ${ASSET_HTTP_CODE}"
  log_info "响应体:"
  echo "$ASSET_BODY" | jq '.' 2>/dev/null || echo "$ASSET_BODY"
  echo ""

  if [[ "$ASSET_HTTP_CODE" != "200" ]]; then
    log_error "素材上传失败"
    exit 1
  fi

  # 提取素材信息
  ASSET_ID=$(echo "$ASSET_BODY" | jq -r '.Result.Id // empty')
  LOCAL_ASSET_ID=$(echo "$ASSET_BODY" | jq -r '.Result.LocalId // empty')
  ASSET_STATUS=$(echo "$ASSET_BODY" | jq -r '.Result.Status // "unknown"')

  if [[ -z "$ASSET_ID" ]]; then
    log_error "响应中没有素材 ID"
    exit 1
  fi

  log_ok "素材创建成功"
  log_info "上游素材 ID: ${ASSET_ID}"
  log_info "本地素材 ID: ${LOCAL_ASSET_ID}"
  log_info "初始状态: ${ASSET_STATUS}"
  echo ""

  # ---------- 步骤 2: 轮询素材状态 ----------
  log_title "步骤 2: 轮询素材状态"

  MAX_POLL_COUNT=20
  POLL_INTERVAL=3
  POLL_COUNT=0

  ASSET_QUERY_URL="${NEWAPI_BASE_URL}/api/seedance/assets/${LOCAL_ASSET_ID}"
  log_info "查询 URL: GET ${ASSET_QUERY_URL}"
  log_info "轮询间隔: ${POLL_INTERVAL} 秒"
  log_info "最大轮询次数: ${MAX_POLL_COUNT}"
  echo ""

  while [[ $POLL_COUNT -lt $MAX_POLL_COUNT ]]; do
    POLL_COUNT=$((POLL_COUNT + 1))
    log_info "第 ${POLL_COUNT}/${MAX_POLL_COUNT} 次查询..."

    QUERY_RESP=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${ASSET_QUERY_URL}" \
      -H "Authorization: Bearer ${NEWAPI_API_KEY}")

    QUERY_HTTP_CODE=$(echo "$QUERY_RESP" | grep "HTTP_CODE:" | cut -d: -f2)
    QUERY_BODY=$(echo "$QUERY_RESP" | sed '/HTTP_CODE:/d')

    if [[ "$QUERY_HTTP_CODE" != "200" ]]; then
      log_error "查询失败 HTTP ${QUERY_HTTP_CODE}"
      echo "$QUERY_BODY" | jq '.' 2>/dev/null || echo "$QUERY_BODY"
      exit 1
    fi

    CURRENT_STATUS=$(echo "$QUERY_BODY" | jq -r '.Result.Status // "unknown"')
    log_info "当前状态: ${CURRENT_STATUS}"

    if [[ "$CURRENT_STATUS" == "Active" ]]; then
      log_ok "素材处理完成！状态: Active"
      echo ""
      log_info "完整响应:"
      echo "$QUERY_BODY" | jq '.'
      echo ""
      log_sep
      log_ok "✅ 素材上传测试成功！"
      log_sep
      echo ""
      log_info "素材 ID: ${ASSET_ID}"
      log_info "本地 ID: ${LOCAL_ASSET_ID}"
      log_info "引用格式: asset://${ASSET_ID}"
      echo ""
      log_info "可以使用此素材 ID 进行视频生成测试:"
      echo "  ASSET_ID=${ASSET_ID} ./test-seedance-simple.sh --execute"
      exit 0
    elif [[ "$CURRENT_STATUS" == "Failed" ]]; then
      log_error "素材处理失败！"
      echo ""
      log_info "完整响应:"
      echo "$QUERY_BODY" | jq '.'
      exit 1
    else
      log_warn "状态: ${CURRENT_STATUS}，等待 ${POLL_INTERVAL} 秒后重试..."
      sleep $POLL_INTERVAL
    fi
  done

  log_error "轮询超时（${MAX_POLL_COUNT} 次），素材仍未完成"
  log_info "最后状态: ${CURRENT_STATUS}"
  exit 1
fi

echo ""
