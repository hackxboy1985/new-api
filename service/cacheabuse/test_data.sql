-- Cache Abuse 检测测试数据
-- 5个连续请求，同一 token_id=9999，时间间隔在60s内
-- 特征: cache_read相近、cache_write>20K且相近、非缓存输入≤5、输出≤1000

INSERT INTO logs (user_id, token_id, token_name, model_name, created_at, type, prompt_tokens, completion_tokens, quota, is_stream, channel_id, `group`, other, content, request_id)
VALUES
-- 请求1: 2026-06-04 12:22:33
(9999, 9999, 'sk-test-cache-abuse', 'claude-sonnet-4-7', 1780489353, 2, 1, 130, 0, 1, 1, 'default',
 '{"claude":true,"cache_tokens":39294,"cache_creation_tokens":2586,"cache_creation_tokens_5m":2586,"model_ratio":1,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_creation_ratio_5m":1.25,"model_price":0,"user_group_ratio":1,"frt":1200,"request_path":"/v1/messages"}',
 '', '202606041222333625735528268d9d6nHlT2ius'),

-- 请求2: 2026-06-04 12:22:44 (间隔11s)
(9999, 9999, 'sk-test-cache-abuse', 'claude-sonnet-4-7', 1780489364, 2, 1, 380, 0, 1, 1, 'default',
 '{"claude":true,"cache_tokens":30997,"cache_creation_tokens":23573,"cache_creation_tokens_5m":23573,"model_ratio":1,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_creation_ratio_5m":1.25,"model_price":0,"user_group_ratio":1,"frt":1100,"request_path":"/v1/messages"}',
 '', '202606041222442784590518268d9d6zlK9JcIv'),

-- 请求3: 2026-06-04 12:22:54 (间隔10s)
(9999, 9999, 'sk-test-cache-abuse', 'claude-sonnet-4-7', 1780489374, 2, 1, 109, 0, 1, 1, 'default',
 '{"claude":true,"cache_tokens":30997,"cache_creation_tokens":24473,"cache_creation_tokens_5m":24473,"model_ratio":1,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_creation_ratio_5m":1.25,"model_price":0,"user_group_ratio":1,"frt":1050,"request_path":"/v1/messages"}',
 '', '202606041222546178040388268d9d6CX9Hyc2G'),

-- 请求4: 2026-06-04 12:23:01 (间隔7s)
(9999, 9999, 'sk-test-cache-abuse', 'claude-sonnet-4-7', 1780489381, 2, 1, 139, 0, 1, 1, 'default',
 '{"claude":true,"cache_tokens":30997,"cache_creation_tokens":26152,"cache_creation_tokens_5m":26152,"model_ratio":1,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_creation_ratio_5m":1.25,"model_price":0,"user_group_ratio":1,"frt":1300,"request_path":"/v1/messages"}',
 '', '202606041223017509146608268d9d6WUw27K1q'),

-- 请求5: 2026-06-04 12:23:10 (间隔9s)
(9999, 9999, 'sk-test-cache-abuse', 'claude-sonnet-4-7', 1780489390, 2, 1, 110, 0, 1, 1, 'default',
 '{"claude":true,"cache_tokens":40008,"cache_creation_tokens":18903,"cache_creation_tokens_5m":18903,"model_ratio":1,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_creation_ratio_5m":1.25,"model_price":0,"user_group_ratio":1,"frt":1150,"request_path":"/v1/messages"}',
 '', '202606041223107315386638268d9d61SLw0nHr');
