# Seedance 上游配置速查表

## 配置位置

渠道管理 → 编辑渠道 → 其他配置（JSON 格式）

---

## Gateway 上游（咪咕）

### 最简配置 ⭐ 推荐

```json
{
  "asset_upstream_version": "gateway",
  "seedance_asset_base_url": "https://sd.dawnloadai.com:9444"
}
```

**说明**：
- ✅ 只需 2 个字段
- ✅ 视频接口路径自动推断为 `/api/v3/contents/generations/tasks`
- ✅ 适用于大部分场景

### 完整配置

```json
{
  "asset_upstream_version": "gateway",
  "seedance_asset_base_url": "https://sd.dawnloadai.com:9444",
  "seedance_relay_mode": false,
  "doubao_video_generate_path": "/api/v3/contents/generations/tasks",
  "doubao_video_fetch_path": "/api/v3/contents/generations/tasks",
  "doubao_video_always_fetch_upstream": false
}
```

**说明**：
- 适用于需要自定义路径的场景
- `seedance_relay_mode`: 是否使用中继模式
- `doubao_video_always_fetch_upstream`: 是否强制实时查询上游（ARK 格式）
  - `false`（默认）: 仅当 task.Data 不完整时才查询上游
  - `true`: 每次查询都实时调用上游获取最新状态

---

## KWJM 上游

### 最简配置 ⭐ 推荐

```json
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2"
}
```

**说明**：
- ✅ 只需 3 个字段
- ✅ 视频接口路径自动推断为 `/v1/videos/generations`
- ✅ 适用于大部分场景

### 完整配置

```json
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2",
  "doubao_video_generate_path": "/v1/videos/generations",
  "doubao_video_fetch_path": "/v1/videos/generations",
  "doubao_video_always_fetch_upstream": false
}
```

**说明**：
- 适用于需要自定义路径的场景
- `kwjm_asset_model`: 默认模型，可选 `sd-video-v2`、`kw-video-v2-fast` 等
- `doubao_video_always_fetch_upstream`: 是否强制实时查询上游（ARK 格式）
  - `false`（默认）: 仅当 task.Data 不完整时才查询上游
  - `true`: 每次查询都实时调用上游获取最新状态

---

## 字段说明

### 必填字段

| 字段 | Gateway | KWJM | 说明 |
|------|---------|------|------|
| `asset_upstream_version` | ✅ | ✅ | 上游类型标识 |
| `seedance_asset_base_url` | ✅ | ❌ | Gateway 资产库地址 |
| `kwjm_asset_base_url` | ❌ | ✅ | KWJM 资产库地址 |
| `kwjm_asset_model` | ❌ | ✅ | KWJM 默认模型 |

### 自动推断字段（可选）

| 字段 | Gateway 默认值 | KWJM 默认值 |
|------|--------------|------------|
| `doubao_video_generate_path` | `/api/v3/contents/generations/tasks` | `/v1/videos/generations` |
| `doubao_video_fetch_path` | `/api/v3/contents/generations/tasks` | `/v1/videos/generations` |

**注意**：手动配置会覆盖自动推断值。

### 可选字段

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `doubao_video_always_fetch_upstream` | bool | `false` | ARK 格式查询是否强制实时调用上游 |
| `openai_video_always_fetch_upstream` | bool | `false` | OpenAI Video API 格式查询是否强制实时调用上游 |
| `seedance_relay_mode` | bool | `false` | 是否使用中继模式 |

#### `doubao_video_always_fetch_upstream` 详细说明

此配置仅影响 **ARK 格式**（`GET /api/v3/contents/generations/tasks/{id}`）的查询行为：

**`false`（默认）**：
- 仅当 `task.Data` 不完整时才调用上游
- 正常情况下只读数据库，速度快
- 适合大部分场景，减少上游压力

**`true`（强制实时查询）**：
- 每次查询都调用上游获取最新状态
- 适合需要实时状态的场景
- 会增加上游负载和查询延迟

#### `openai_video_always_fetch_upstream` 详细说明

此配置仅影响 **OpenAI Video API 格式**（`GET /v1/videos/{id}`）的查询行为：

**`false`（默认）**：
- 永不调用上游，总是读取数据库
- 速度最快，延迟最低
- 适合大部分场景

**`true`（强制实时查询）**：
- 每次查询都调用上游获取最新状态
- 适合需要实时状态的场景
- 会增加上游负载和查询延迟

#### 两种格式对比

| 配置 | ARK 格式 (默认) | ARK 格式 (强制) | OpenAI 格式 (默认) | OpenAI 格式 (强制) |
|------|---------------|----------------|------------------|------------------|
| 是否调用上游 | 仅 Data 不完整时 | 总是调用 | 永不调用 | 总是调用 |
| 查询速度 | 快 | 慢 | 最快 | 慢 |
| 上游压力 | 低 | 高 | 无 | 高 |
| 状态实时性 | 依赖后台轮询 | 实时 | 依赖后台轮询 | 实时 |

**注意**：
- 状态更新由后台轮询服务自动处理（每分钟一次）
- 默认值适合 99% 的场景
- 仅在需要强制实时状态时才设为 `true`

**使用场景**：
- 默认配置：适合绝大多数场景
- 强制实时查询：监控面板、实时状态展示等特殊需求

---

## 接口路径对照表

### 资产库接口

| 操作 | Gateway | KWJM |
|------|---------|------|
| **创建素材组** | `POST /api/seedance/proxy/assets/groups` | `POST /v3/open/CreateAssetGroup` |
| **查询素材组** | `GET /api/seedance/proxy/assets/groups/{id}` | `POST /v3/open/GetAssetGroup` |
| **创建素材** | `POST /api/seedance/proxy/assets` | `POST /v3/open/CreateAsset` |
| **查询素材** | `GET /api/seedance/proxy/assets/{id}` | `POST /v3/open/GetAsset` |

**说明**：
- Gateway 使用 RESTful 风格（GET/POST）
- KWJM 使用 Action 风格（全部 POST）
- 通过适配器自动处理，无需手动配置

### 视频生成接口

| 操作 | Gateway | KWJM |
|------|---------|------|
| **提交任务** | `POST /api/v3/contents/generations/tasks` | `POST /v1/videos/generations` |
| **查询任务** | `GET /api/v3/contents/generations/tasks/{id}` | `GET /v1/videos/generations/{id}` |

**说明**：
- 通过 `asset_upstream_version` 自动推断
- 也可以手动配置覆盖

---

## 常见配置场景

### 场景 1：单一 Gateway 上游

```json
{
  "asset_upstream_version": "gateway",
  "seedance_asset_base_url": "https://sd.dawnloadai.com:9444"
}
```

### 场景 2：单一 KWJM 上游

```json
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2"
}
```

### 场景 3：多渠道混合（推荐）

创建两个渠道，分别配置：

**渠道 A（Gateway）**：
```json
{
  "asset_upstream_version": "gateway",
  "seedance_asset_base_url": "https://sd.dawnloadai.com:9444"
}
```

**渠道 B（KWJM）**：
```json
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2"
}
```

通过用户组或权重控制使用哪个渠道。

---

## 验证配置

### 1. 查看日志

```bash
tail -f logs/new-api.log | grep AssetAdapter
```

**预期输出**：

```
[AssetAdapter] selected Gateway adapter for group 'default': channel=5, url=https://sd.dawnloadai.com:9444
```

或

```
[AssetAdapter] selected KWJM adapter for group 'default': channel=6, url=https://kwjm.com, model=sd-video-v2
```

### 2. 运行测试

```bash
./test-seedance-simple.sh --execute
```

---

## 配置优先级

1. **手动配置** > 自动推断
2. 如果同时配置了 `doubao_video_generate_path` 和 `asset_upstream_version`，使用手动配置
3. 如果只配置了 `asset_upstream_version`，自动推断路径

---

## 故障排查

### 问题 1：找不到适配器

**日志**：
```
no available asset adapter for group default
```

**原因**：
- 渠道未启用
- 用户组不匹配
- 配置缺失

**解决**：
1. 检查渠道状态是否启用
2. 检查用户组配置
3. 确认必填字段已配置

### 问题 2：路径错误

**日志**：
```
404 page not found
```

**原因**：
- 视频接口路径配置错误
- 上游 URL 错误

**解决**：
1. 检查 `doubao_video_generate_path` 配置
2. 或删除该字段，使用自动推断
3. 检查上游 URL 是否正确

### 问题 3：适配器选择错误

**日志**：
```
gateway adapter: POST https://kwjm.com/api/seedance/proxy/assets
```

**原因**：
- `asset_upstream_version` 配置错误
- 系统选择了错误的适配器

**解决**：
1. 检查 `asset_upstream_version` 值
2. 确保值为 `"gateway"` 或 `"kwjm"`（小写）
3. 重启服务使配置生效

---

## 配置模板下载

### Gateway 模板

```bash
cat > gateway-config.json <<EOF
{
  "asset_upstream_version": "gateway",
  "seedance_asset_base_url": "https://sd.dawnloadai.com:9444"
}
EOF
```

### KWJM 模板

```bash
cat > kwjm-config.json <<EOF
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2"
}
EOF
```

---

## 更新日志

### 2026-09-05
- ✅ 增加自动推断视频接口路径功能
- ✅ 简化配置，Gateway 只需 2 个字段，KWJM 只需 3 个字段
- ✅ 更新配置说明和示例

### 2026-01-28
- 初始版本发布
