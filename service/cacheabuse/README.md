# Cache Abuse 检测器

## 背景问题

在 `feature/merge-upstream` 分支的日志中发现了一种 API 滥用模式，出现在 `/v1/messages`（Claude API）请求中。

### 问题特征

分析实际日志数据发现以下规律：

| 请求 | 缓存读取 | 缓存写入 | 非缓存输入 | 非缓存输出 |
|------|---------|---------|-----------|-----------|
| 请求1 | 39,294 | 2,586 | 极小 | 极小 |
| 请求2 | 30,997 | 23,573 | 极小 | 极小 |
| 请求3 | 30,997 | 24,473 | 极小 | 极小 |
| 请求4 | 30,997 | 26,152 | 极小 | 极小 |
| 请求5 | 40,008 | 18,903 | 极小 | 极小 |

**关键特征：**

1. **缓存读取 tokens 相近** — 相邻请求的 `cache_read` 差值 ≤ 100
2. **缓存写入 tokens 超 2W 且非常接近** — `cache_write` > 20,000，且相邻差值 ≤ 400
3. **真正的非缓存输入、输出 tokens 极少** — ≤ 100
4. **同一个 API Key + 连续出现** — 同一 `token_id` 在一分钟内多次请求
5. **时间窗口：1 分钟内出现** — 快速连续请求

### 为什么写的缓存不被命中？

Anthropic 的 prompt caching 采用 **(内容 + 位置) 双重匹配**，不是单纯的内容哈希匹配：

```
请求1: [System Prompt][Tools][Msg1]             → 缓存 System+Tools (39k), 写 Msg1 (2.5k)
请求2: [System Prompt][Tools][Msg1][Msg2]       → 缓存 System+Tools (30k), 写 Msg1+Msg2 (23k)
请求3: [System Prompt][Tools][Msg1][Msg2][Msg3] → 缓存 System+Tools (30k), 写更多 (24k)
```

- **~30k 固定命中**：是 system prompt + tool definitions 等静态前缀，位置始终不变
- **23k~26k 写了但不被读**：是多轮对话中动态增长的内容，每轮新增消息导致位置的偏移，之前写的缓存 key 对不上新请求的位置

**这意味着：**
- System prompt 部分始终能被缓存（省钱，$0.3/1M tokens）
- 动态对话部分每次都写新缓存但下次用不上（浪费钱，$3.75/1M tokens）
- 如果请求中 23k tokens 都花在缓存写入上，而真正的"新内容"只有不到 100 tokens，**这就是极不正常的滥用行为**

---

## 检测方案

### 设计原则

- **完全自包含** — 所有代码封装在 `service/cacheabuse/` 包内
- **零业务侵入** — 仅 `main.go` 加一行空白导入触发 `init()` 自动启动
- **独立配置** — 直接读 `os.Getenv`，不依赖 `constant/env.go` 或 `common/init.go`
- **定时后台扫描** — `init()` 启动 goroutine，轮询等待 `LOG_DB` 就绪后开始扫描
- **跨数据库兼容** — JSON 解析在 Go 里做，SQL 只做基础查询
- **输出到系统日志 + Redis 告警** — 方便监控和报警集成

### 架构

```
main.go 空白导入 → cacheabuse.init() 启动 goroutine
  └── 等待 model.LOG_DB 就绪
        └── 每 60s 执行 runScanOnce()
              ├── fetchRecentLogs()  → 查询 logs 表（最近 120s）
              ├── detectAbuse()      → 按 token_id 分组，检测匹配模式
              └── emit()             → common.SysLog + Redis 告警
```

### 检测算法

```
输入: 从 logs 表查询的所有近期消费记录
输出: 匹配的滥用请求组

1. 按 token_id 分组（SQL 已排序）
2. 每组内解析 other JSON → 提取 cache_tokens, cache_creation_tokens
3. 连续遍历每对相邻请求 (prev, curr):
   a. 时间间隔 ≤ 60s?
   b. |cache_read_prev - cache_read_curr| ≤ 100?
   c. cache_write_prev > 20000 && cache_write_curr > 20000?
   d. |cache_write_prev - cache_write_curr| ≤ 400?
   e. 非缓存输入 ≤ 100 && 输出 ≤ 100?
   → 全部满足: 加入匹配序列
   → 不满足: 结束当前序列，记录为一次匹配
4. 取最长的连续匹配序列
5. 去重检查 → 写入日志 + Redis
```

### 文件结构

```
service/cacheabuse/
  ├── config.go       — 内部配置，从 os.Getenv 读取（不依赖 constant/init）
  ├── scanner.go      — init() 自动启动 + 定时扫描循环
  ├── detector.go     — 数据库查询、模式匹配算法
  └── redis.go        — 告警输出（系统日志 + Redis）
```

### 修改的文件

| 文件 | 改动 | 说明 |
|------|------|------|
| `service/cacheabuse/config.go` | **新建** | 内部配置，直接读环境变量 |
| `service/cacheabuse/scanner.go` | **新建** | 扫描器，`init()` 自动启动 |
| `service/cacheabuse/detector.go` | **新建** | 核心检测逻辑 |
| `service/cacheabuse/redis.go` | **新建** | 告警输出 |
| `main.go` | **加 1 行** | 空白导入 `_ "...cacheabuse"` 触发 `init()` |

**所有业务逻辑代码封装在 `service/cacheabuse/` 包内，仅 `main.go` 加一行空白导入。**

---

## 配置项

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `CACHE_ABUSE_SCAN_ENABLED` | `true` | 总开关，`false` 关闭扫描 |
| `CACHE_ABUSE_SCAN_INTERVAL` | `60` | 扫描间隔（秒） |
| `CACHE_ABUSE_LOOKBACK_SECONDS` | `120` | 查询回溯窗口（秒），应为间隔的 2 倍以上防止遗漏 |
| `CACHE_ABUSE_READ_DIFF` | `100` | 缓存读取 tokens 最大差值 |
| `CACHE_ABUSE_WRITE_MIN` | `20000` | 缓存写入 tokens 最低阈值 |
| `CACHE_ABUSE_WRITE_DIFF` | `400` | 缓存写入 tokens 最大差值 |
| `CACHE_ABUSE_NONCACHED_INPUT_MAX` | `5` | 非缓存输入 tokens 上限 |
| `CACHE_ABUSE_NONCACHED_OUTPUT_MAX` | `1000` | 非缓存输出 tokens 上限 |
| `CACHE_ABUSE_TIME_WINDOW_SECONDS` | `60` | 相邻请求最大时间间隔（秒） |

禁用扫描:
```bash
export CACHE_ABUSE_SCAN_ENABLED=false
```

调低阈值测试:
```bash
export CACHE_ABUSE_WRITE_MIN=100
export CACHE_ABUSE_NONCACHED_INPUT_MAX=10000
export CACHE_ABUSE_NONCACHED_OUTPUT_MAX=10000
```

---

## 告警输出

### 系统日志

匹配成功后在系统日志输出（`[SYS]` 前缀）:
```
[SYS] 2026/06/07 - 15:30:00 | [CACHE_ABUSE] token_id=42 token_name=sk-xxx user_id=5 model=claude-sonnet-4-7 count=5 cache_read_diffs=min=0 max=45 cache_write_diffs=min=12 max=380 first_at=1686000000 last_at=1686000056
```

### Redis 告警

Key 格式: `cache_abuse:alert:{token_id}:{unix_timestamp}`
TTL: 300 秒（5 分钟）

Value (JSON):
```json
{
  "token_id": 42,
  "token_name": "sk-xxx",
  "user_id": 5,
  "model": "claude-sonnet-4-7",
  "count": 5,
  "read_diffs": "min=0 max=45",
  "write_diffs": "min=12 max=380",
  "first_at": 1686000000,
  "last_at": 1686000056
}
```

### 去重机制

- 同一个 `token_id` 在 5 分钟内只告警一次
- Redis key 自动 TTL 过期，不占用存储

---

## 数据库查询

跨数据库兼容的查询（解析 JSON 在 Go 中完成）:

```sql
SELECT id, user_id, token_id, token_name, model_name, created_at,
       prompt_tokens, completion_tokens, other
FROM logs
WHERE type = 2 AND created_at >= ?
ORDER BY token_id ASC, created_at ASC
```

其中 `other` 字段在 Go 中解析提取:
- `cache_tokens` → 缓存读取 tokens
- `cache_creation_tokens` / `cache_creation_tokens_5m` → 缓存写入 tokens
- `claude` → 是否为 Claude 语义（影响非缓存输入的计算方式）

---

## 可执行 SQL

### MySQL / SQLite — 查询最近 2 分钟缓存写入超 1W 的请求

```sql
SELECT id, user_id, token_id, token_name, model_name, created_at,
       prompt_tokens, completion_tokens,
       JSON_EXTRACT(other, '$.cache_tokens') AS cache_read,
       JSON_EXTRACT(other, '$.cache_creation_tokens_5m') AS cache_write_5m,
       JSON_EXTRACT(other, '$.cache_creation_tokens') AS cache_write,
       other
FROM logs
WHERE type = 2
  AND created_at >= UNIX_TIMESTAMP() - 120
  AND JSON_EXTRACT(other, '$.cache_creation_tokens_5m') > 10000
ORDER BY token_id ASC, created_at ASC;
```

SQLite 把 `UNIX_TIMESTAMP()` 换成 `strftime('%s', 'now')`。

### PostgreSQL — 同上

```sql
SELECT id, user_id, token_id, token_name, model_name, created_at,
       prompt_tokens, completion_tokens,
       other::json->>'cache_tokens' AS cache_read,
       other::json->>'cache_creation_tokens_5m' AS cache_write_5m,
       other::json->>'cache_creation_tokens' AS cache_write,
       other
FROM logs
WHERE type = 2
  AND created_at >= EXTRACT(EPOCH FROM NOW()) - 120
  AND (other::json->>'cache_creation_tokens_5m')::int > 10000
ORDER BY token_id ASC, created_at ASC;
```

### 只看某个 token_id 的连续请求（替换 9999）

```sql
SELECT id, token_name, model_name, created_at,
       prompt_tokens, completion_tokens,
       JSON_EXTRACT(other, '$.cache_tokens') AS cache_read,
       JSON_EXTRACT(other, '$.cache_creation_tokens_5m') AS cache_write
FROM logs
WHERE type = 2
  AND token_id = 9999
  AND created_at >= UNIX_TIMESTAMP() - 3600
ORDER BY created_at ASC;
```
