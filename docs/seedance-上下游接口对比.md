# Seedance 上下游接口对比文档

## 概述

本文档说明 new-api 作为中间层，如何处理下游用户请求和上游接口的对接。

---

## 📊 下游用户请求（4 种组合）

### 提交任务

下游用户可以使用以下任意组合提交视频生成任务：

#### 组合 1: `/v1/video/generations` + OpenAI 格式

```bash
POST /v1/video/generations
Content-Type: application/json

{
  "model": "doubao-seedance-2-0-sd",
  "content": [
    {"type": "text", "text": "一个女孩在跳舞"},
    {"type": "image_url", "image_url": {"url": "..."}}
  ],
  "resolution": "480p",
  "duration": 5
}
```

**响应：OpenAI Video 格式**
```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",
  "status": "queued",
  "created_at": 1788780640
}
```

---

#### 组合 2: `/v1/video/generations` + ARK 格式

```bash
POST /v1/video/generations
Content-Type: application/json

{
  "model": "doubao-seedance-2.0-D",
  "content": [
    {"type": "text", "text": "一个女孩在跳舞"},
    {"type": "image_url", "image_url": {"url": "..."}}
  ],
  "resolution": "480p",
  "duration": 5
}
```

**响应：ARK 格式**
```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",
  "status": "queued",
  "created_at": 1788780640
}
```

---

#### 组合 3: `/api/v3/contents/generations/tasks` + OpenAI 格式

```bash
POST /api/v3/contents/generations/tasks
Content-Type: application/json

{
  "model": "doubao-seedance-2-0-sd",
  "content": [
    {"type": "text", "text": "一个女孩在跳舞"}
  ],
  "resolution": "480p",
  "duration": 5
}
```

**响应：可以是 OpenAI 或 ARK 格式（取决于 Accept header）**

---

#### 组合 4: `/api/v3/contents/generations/tasks` + ARK 格式

```bash
POST /api/v3/contents/generations/tasks
Content-Type: application/json

{
  "model": "doubao-seedance-2.0-D",
  "content": [
    {"type": "text", "text": "一个女孩在跳舞"}
  ],
  "resolution": "480p",
  "duration": 5
}
```

**响应：ARK 格式**

---

### 查询任务

下游用户可以使用以下路径查询任务状态：

#### OpenAI Video 格式查询

```bash
# 路径 1
GET /v1/video/generations/{task_id}

# 路径 2
GET /v1/videos/{task_id}
```

**响应：OpenAI Video 格式**
```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",
  "status": "completed",
  "progress": 100,
  "created_at": 1788780640,
  "completed_at": 1788836380,
  "metadata": {
    "url": "https://...",
    "duration": 4,
    "resolution": "480p",
    "ratio": "16:9"
  },
  "usage": {
    "completion_tokens": 40594,
    "total_tokens": 40594
  }
}
```

---

#### ARK 格式查询

```bash
GET /api/v3/contents/generations/tasks/{task_id}
```

**响应：Doubao 官方格式**
```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",
  "status": "succeeded",
  "content": {
    "video_url": "https://..."
  },
  "created_at": 1788780640,
  "updated_at": 1788780796,
  "duration": 4,
  "resolution": "480p",
  "ratio": "16:9",
  "seed": 29533,
  "usage": {
    "completion_tokens": 40594,
    "total_tokens": 40594
  }
}
```

---

## 🔄 上游接口（统一配置）

### 上游路径控制

**无论下游用户使用哪种路径或格式，向上游发送请求的路径由配置统一控制：**

```json
{
  "asset_upstream_version": "kwjm"
}
```

### 自动推断规则

| asset_upstream_version | 生成任务路径 | 查询任务路径 |
|----------------------|------------|------------|
| `"kwjm"` | `/v1/videos/generations` | `/v1/videos/generations/{id}` |
| `"gateway"` 或不配置 | `/api/v3/contents/generations/tasks` | `/api/v3/contents/generations/tasks/{id}` |

---

## 🎯 完整流程示例

### 提交任务流程

```
下游用户请求（任意组合）
POST /v1/video/generations
或
POST /api/v3/contents/generations/tasks
{
  "model": "doubao-seedance-2-0-sd",
  "content": [...]
}
  ↓
路由匹配 → controller.RelayTask
  ↓
RelayTaskSubmit()
  ↓
adaptor.Init(info)  ← 加载配置
  读取 asset_upstream_version = "kwjm"
  推断 videoGeneratePath = "/v1/videos/generations"
  ↓
adaptor.BuildRequestURL()
  构造 URL: https://kwjm.com/v1/videos/generations
  ↓
发送到上游
POST https://kwjm.com/v1/videos/generations
{
  "model": "sd-video-v2",  ← 可能经过模型映射
  "content": [...]
}
  ↓
上游返回
{
  "id": "kwf_xxx",
  "model": "doubao-seedance-2.0-D"
}
  ↓
保存到数据库
task.TaskID = "task_xxx"  ← 公开任务ID
task.UpstreamTaskID = "kwf_xxx"  ← 上游任务ID
task.Data = {...}  ← 上游原始响应
task.Properties.OriginModelName = "doubao-seedance-2-0-sd"  ← 用户请求的模型
  ↓
返回给用户（根据请求格式返回对应格式）
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",  ← 返回用户请求的模型名
  "status": "queued"
}
```

---

### 查询任务流程 - OpenAI 格式

```
下游用户请求
GET /v1/videos/task_xxx
  ↓
videoFetchByIDRespBodyBuilder()
  ↓
从数据库读取 task
  ↓
tryOpenAIVideoRealtimeFetch() 判断是否查询上游
  检查 openai_video_always_fetch_upstream 配置
  ↓
如果需要查询上游:
  adaptor.Init(info)  ← 加载配置
    读取 asset_upstream_version = "kwjm"
    推断 videoFetchPath = "/v1/videos/generations"
  ↓
  adaptor.FetchTask()
    构造 URL: https://kwjm.com/v1/videos/generations/kwf_xxx
  ↓
  查询上游
  GET https://kwjm.com/v1/videos/generations/kwf_xxx
  ↓
  上游返回最新状态
  {
    "id": "kwf_xxx",
    "model": "doubao-seedance-2.0-D",
    "status": "succeeded",
    ...
  }
  ↓
  更新数据库 task.Data
  ↓
adaptor.ConvertToOpenAIVideo(task)
  从 task.Data 读取数据
  使用 task.Properties.OriginModelName 作为模型名
  转换为 OpenAI Video 格式
  ↓
返回给用户
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",  ← 用户请求的模型名
  "status": "completed",
  "metadata": {
    "url": "https://...",
    ...
  }
}
```

---

### 查询任务流程 - ARK 格式

```
下游用户请求
GET /api/v3/contents/generations/tasks/task_xxx
  ↓
videoFetchByIDRespBodyBuilder()
  ↓
从数据库读取 task
  ↓
tryDoubaoRealtimeFetch() 判断是否查询上游
  检查 doubao_video_always_fetch_upstream 配置
  检查 task.Data 是否完整
  ↓
如果需要查询上游:
  adaptor.Init(info)  ← 加载配置
    读取 asset_upstream_version = "kwjm"
    推断 videoFetchPath = "/v1/videos/generations"
  ↓
  adaptor.FetchTask()
    构造 URL: https://kwjm.com/v1/videos/generations/kwf_xxx
  ↓
  查询上游
  GET https://kwjm.com/v1/videos/generations/kwf_xxx
  ↓
  上游返回最新状态
  更新数据库 task.Data
  ↓
从 task.Data 读取
替换 id = task.TaskID (task_xxx)
替换 model = task.Properties.OriginModelName (用户请求的模型)
  ↓
返回给用户
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-sd",  ← 用户请求的模型名
  "status": "succeeded",
  "content": {
    "video_url": "https://..."
  },
  ...
}
```

---

## ⚙️ 配置参数说明

### 基础配置

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `asset_upstream_version` | string | 上游类型，决定接口路径 | `"kwjm"` 或 `"gateway"` |
| `kwjm_asset_base_url` | string | KWJM 上游基础 URL | `"https://kwjm.com"` |
| `kwjm_asset_model` | string | KWJM 默认模型 | `"sd-video-v2"` |

### 路径配置（可选）

| 参数 | 类型 | 说明 | 默认值（KWJM） |
|------|------|------|--------------|
| `doubao_video_generate_path` | string | 提交任务的路径 | `/v1/videos/generations` |
| `doubao_video_fetch_path` | string | 查询任务的路径 | `/v1/videos/generations` |

**优先级：** 显式配置 > 自动推断

### 查询行为配置

| 参数 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `doubao_video_always_fetch_upstream` | boolean | ARK 格式查询时是否总是请求上游 | `false` |
| `openai_video_always_fetch_upstream` | boolean | OpenAI 格式查询时是否总是请求上游 | `false` |

**说明：**
- `false`：仅当 `task.Data` 不完整时才查询上游
- `true`：每次查询都实时从上游获取最新状态

---

## 📝 配置示例

### KWJM 上游配置（推荐）

```json
{
  "asset_upstream_version": "kwjm",
  "kwjm_asset_base_url": "https://kwjm.com",
  "kwjm_asset_model": "sd-video-v2",
  "doubao_video_always_fetch_upstream": true,
  "openai_video_always_fetch_upstream": true
}
```

**效果：**
- ✅ 提交路径：`POST https://kwjm.com/v1/videos/generations`
- ✅ 查询路径：`GET https://kwjm.com/v1/videos/generations/{id}`
- ✅ ARK 格式每次都查上游
- ✅ OpenAI 格式每次都查上游
- ✅ 两种格式都返回用户请求的模型名

---

### Gateway 上游配置

```json
{
  "asset_upstream_version": "gateway",
  "doubao_video_always_fetch_upstream": false,
  "openai_video_always_fetch_upstream": false
}
```

**效果：**
- ✅ 提交路径：`POST https://gateway.com/api/v3/contents/generations/tasks`
- ✅ 查询路径：`GET https://gateway.com/api/v3/contents/generations/tasks/{id}`
- ✅ 仅当 Data 不完整时才查询上游

---

## 🎯 总结

### 下游（用户侧）

**灵活性：支持多种路径和格式**

- **提交路径（2 个）**：
  - `/v1/video/generations`
  - `/api/v3/contents/generations/tasks`

- **请求格式（2 种）**：
  - OpenAI Video 格式
  - Doubao ARK 格式

- **查询路径（3 个）**：
  - `/v1/video/generations/{id}`
  - `/v1/videos/{id}`
  - `/api/v3/contents/generations/tasks/{id}`

- **返回格式（2 种）**：
  - OpenAI Video 格式
  - Doubao ARK 格式

**总共支持：2 × 2 = 4 种提交组合 + 2 种查询格式**

---

### 上游（KWJM/Gateway 侧）

**统一性：配置控制**

- **路径由 `asset_upstream_version` 统一控制**
- **KWJM**：`/v1/videos/generations`
- **Gateway**：`/api/v3/contents/generations/tasks`

---

### 关键设计原则

1. **下游灵活**：兼容多种客户端和使用习惯
2. **上游统一**：配置集中管理，便于切换上游
3. **格式转换**：自动处理格式转换，对用户透明
4. **模型名一致**：始终返回用户请求的模型名，模型映射对用户不可见
5. **实时查询可选**：根据业务需求配置是否实时查询上游

---

## 🔍 常见问题

### Q1: 为什么返回的模型名和上游不一样？

**A:** 这是设计行为。用户请求 `doubao-seedance-2-0-sd`，系统就应该返回这个名称。后端可能通过模型映射将其转换为上游的 `doubao-seedance-2.0-D`，但这个转换对用户是透明的。

---

### Q2: 什么时候会查询上游？

**A:** 取决于配置：

- **ARK 格式**：`doubao_video_always_fetch_upstream = true` 时总是查询
- **OpenAI 格式**：`openai_video_always_fetch_upstream = true` 时总是查询
- **否则**：仅当 `task.Data` 不完整时才查询

---

### Q3: 如何切换上游？

**A:** 修改渠道配置中的 `asset_upstream_version`：

```json
{
  "asset_upstream_version": "kwjm"  // 或 "gateway"
}
```

修改后：
- **提交新任务**：立即生效
- **查询旧任务**：重启服务后生效（因为 adaptor 实例会重新初始化）

---

### Q4: 两种查询格式有什么区别？

| 特性 | OpenAI Video 格式 | ARK 格式 |
|------|-----------------|---------|
| **路径** | `/v1/videos/{id}` | `/api/v3/contents/generations/tasks/{id}` |
| **返回结构** | 标准 OpenAI Video | Doubao 官方格式 |
| **metadata** | 嵌套在 `metadata` 字段 | 平铺在顶层 |
| **兼容性** | 兼容 OpenAI 生态 | 兼容 Doubao/火山生态 |
| **适用场景** | 统一接口，多模型切换 | 直接对接 Doubao |

---

## 📅 更新历史

- **2026-09-08**: 初始版本，整理上下游接口对比
