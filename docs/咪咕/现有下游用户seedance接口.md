# Seedance2.0接口文档

# Seedance 接口文档
- 包含素材创建及视频生成文档，可直接发起测试

## 概述

上传素材后，可在视频生成接口中通过 `asset://<asset_id>` 引用。

**无需手动创建素材组**：首次上传时系统自动为当前用户创建默认素材组。

## 鉴权

```http
Authorization: Bearer sk-<API_TOKEN>
Content-Type: application/json
```

---

## 1、创建素材

```http
POST /api/seedance/assets
```

**请求体：**

```json
{
  "URL": "https://example.com/image.jpg",
  "AssetType": "Image",
  "Name": "my-image",
  "Model": "doubao-seedance-2-0-260128"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `URL` | string | 是 | 国内公网可访问的 HTTPS 文件地址 |
| `AssetType` | string | 是 | `Image`、`Video` 或 `Audio` |
| `Name` | string | 否 | 素材名称 |
| `GroupId` | string | 否 | 指定素材组，不填则自动使用默认组 |
| `Model` | string | 否 | 指定模型名（如 `doubao-seedance-2-0-260128`），用于精确选择渠道。建议与后续视频生成使用相同模型名 |
| `Force` | bool | 否 | `true` 时强制重新上传，忽略本地缓存；默认 `false` |

> **幂等性**：相同 URL 已有 `Active` 状态的素材时，直接返回已有素材，不重复上传。设置 `Force: true` 可强制重新上传。
>
> **渠道选择**：
> - 如果指定 `Model` 字段：系统会选择支持该模型的渠道进行上传
> - 如果不指定 `Model` 字段：系统使用默认渠道（按分组选择第一个可用渠道）
> - **建议**：如果有多个渠道，建议指定 `Model` 字段，确保素材和视频在同一渠道

**成功响应：**

```json
{
  "Result": {
    "Id": "asset-xxxxxxxx",
    "LocalId": 12,
    "AssetRef": "asset://asset-xxxxxxxx",
    "Status": "Processing"
  }
}
```

| 字段 | 说明 |
|------|------|
| `Result.Id` | 上游 asset_id |
| `Result.LocalId` | 本地记录 ID，用于查询接口的 `:id` 参数 |
| `Result.AssetRef` | 视频生成时直接填入 `images[]` 的值 |

---

## 2、查询素材状态

```http
GET /api/seedance/assets/:id
```

`:id` 支持两种格式：
- **本地 ID**（数字）：创建时响应里的 `Result.LocalId`
- **上游 asset_id**：创建时响应里的 `Result.Id`，例如 `asset-xxxxxxxx`

**响应示例：**

```json
{
  "Result": {
    "Id": "asset-xxxxxxxx",
    "Status": "Active"
  }
}
```

| 状态 | 说明 |
|------|------|
| `Processing` | 正在预处理，稍后重试 |
| `Active` | 可用，可用于视频生成 |
| `Failed` | 处理失败 |

轮询直到状态变为 `Active`，建议间隔 3 秒。


---


## 3、生成视频及在视频中引用素材

素材状态为 `Active` 后，将 `Result.AssetRef`（即 `asset://asset-xxxxxxxx`）用于视频生成。

### 格式一：标准格式

```http
POST /v1/video/generations
```

**请求参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | 是 | 模型名称，见下方模型列表 |
| `prompt` | string | 是 | 文本描述 |
| `seconds` | string | 否 | 视频时长（秒），字符串形式，如 `"5"` |
| `duration` | int | 否 | 视频时长（秒），整数形式，与 `seconds` 二选一，`seconds` 优先 |
| `images` | string[] | 否 | 参考图/参考视频 URL 列表，支持 `asset://asset-xxxxxxxx` 格式 |
| `metadata` | object | 否 | 扩展参数，见下方 metadata 说明 |

**metadata 可选参数：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `resolution` | string | 分辨率，如 `"720p"`、`"1080p"` |
| `ratio` | string | 宽高比，如 `"16:9"`、`"9:16"`、`"1:1"` |
| `generate_audio` | bool | 是否生成音频，默认 `false` |
| `return_last_frame` | bool | 是否返回最后一帧 |
| `seed` | int | 随机种子 |
| `camera_fixed` | bool | 是否固定镜头 |
| `watermark` | bool | 是否添加水印 |
| `content` | array | Ark 原生 content 数组（音频/视频引用），与 `images` 配合使用 |

**支持的模型：**

| 模型名 | 说明 |
|--------|------|
| `doubao-seedance-2-0-260128` | Seedance 2.0 |
| `doubao-seedance-2-0-fast-260128` | Seedance 2.0 Fast |
| `doubao-seedance-2-0-mini-260615` | Seedance 2.0 Mini |


```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "人物自然介绍产品",
  "seconds": "5",
  "images": ["asset://asset-xxxxxxxx"],
  "metadata": {
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": false
  }
}
```

**提交成功响应：**

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "queued",
  "progress": 0,
  "created_at": 1700000000
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 任务 ID，查询时使用 |
| `task_id` | string | 同 `id`，兼容旧接口 |
| `object` | string | 固定为 `"video"` |
| `model` | string | 使用的模型名称 |
| `status` | string | 任务状态，见状态说明 |
| `progress` | int | 进度百分比（0-100） |
| `created_at` | int64 | 创建时间（Unix 时间戳，秒） |

---

### 格式二：Ark 原生格式

```http
POST /v1/video/generations
```

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {"type": "text", "text": "人物自然介绍产品（使用音频中的声音）"},
    {"type": "image_url", "image_url": {"url": "asset://asset-xxxxxxxx"}},
    {"type": "audio_url", "audio_url": {"url": "https://example.com/voice.wav"}},
    {"type":"video_url","video_url":{"url":"https://example.com/reference.mp4"}}
  ],
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 5,
  "generate_audio": true
}
```

两种格式均支持，`content[]` 中媒体类型会自动补充 `role` 字段（`image_url` → `reference_image`，`audio_url` → `reference_audio`，`video_url` → `reference_video`）。

## 4、查询视频任务

```http
GET /v1/videos/:task_id
```

```bash
curl http://open.mints-id.com/v1/videos/task_xxxxxxxx \
  -H "Authorization: Bearer sk-xxx"
```

**响应示例（处理中）：**

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "in_progress",
  "progress": 50,
  "created_at": 1700000000
}
```

**响应示例（完成）：**

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "completed",
  "progress": 100,
  "created_at": 1700000000,
  "completed_at": 1700000120,
  "metadata": {
    "url": "https://cdn.example.com/output.mp4"
  }
}
```

**响应示例（失败）：**

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "failed",
  "progress": 100,
  "created_at": 1700000000,
  "error": {
    "code": "content_policy_violation",
    "message": "内容违反使用政策"
  }
}
```

**响应字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 任务 ID |
| `task_id` | string | 同 `id`，兼容旧接口 |
| `object` | string | 固定为 `"video"` |
| `model` | string | 使用的模型名称 |
| `status` | string | 任务状态，见下方状态说明 |
| `progress` | int | 进度百分比（0-100） |
| `created_at` | int64 | 创建时间（Unix 时间戳，秒） |
| `completed_at` | int64 | 完成时间，仅完成后返回 |
| `expires_at` | int64 | 过期时间，按需返回 |
| `metadata.url` | string | 视频下载地址，`status=completed` 后可用 |
| `error.code` | string | 错误码，`status=failed` 时返回 |
| `error.message` | string | 错误信息，`status=failed` 时返回 |

**任务状态说明：**

| status | 说明 | 进度 |
|--------|------|------|
| `queued` | 排队中 | 0% |
| `in_progress` | 生成中 | 50% |
| `completed` | 已完成，`metadata.url` 可用 | 100% |
| `failed` | 失败，`error` 字段有详情 | 100% |

轮询直到 `status` 变为 `completed` 或 `failed`，建议间隔 5 秒。

---

## 完整示例

```bash
# 1. 上传素材（推荐指定 Model）
ASSET=$(curl -s -X POST http://open.mints-id.com/api/seedance/assets \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"URL":"https://example.com/photo.jpg","AssetType":"Image","Name":"photo","Model":"doubao-seedance-2-0-260128"}')

LOCAL_ID=$(echo $ASSET | jq -r '.Result.LocalId')
ASSET_REF=$(echo $ASSET | jq -r '.Result.AssetRef')
echo "local_id=$LOCAL_ID, asset_ref=$ASSET_REF"

# 2. 轮询等待 Active
while true; do
  STATUS=$(curl -s http://open.mints-id.com/api/seedance/assets/$LOCAL_ID \
    -H "Authorization: Bearer sk-xxx" | jq -r '.Result.Status')
  echo "Status: $STATUS"
  [ "$STATUS" = "Active" ] && break
  [ "$STATUS" = "Failed" ] && { echo "Failed"; exit 1; }
  sleep 3
done

# 3. 视频生成（使用相同模型名）
curl -X POST http://open.mints-id.com/v1/video/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"doubao-seedance-2-0-260128\",
    \"prompt\": \"人物自然介绍产品\",
    \"seconds\": \"5\",
    \"images\": [\"$ASSET_REF\"],
    \"metadata\": {\"resolution\": \"720p\", \"ratio\": \"16:9\"}
  }"
```

