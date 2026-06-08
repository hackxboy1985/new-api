package cacheabuse

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var (
	// dedup: per-token last-alert timestamp to avoid spam.
	dedupMu    sync.Mutex
	dedupMap   = make(map[int]int64) // token_id -> last alert unix timestamp
	dedupWindow = int64(300)         // 5 minutes
)

func emit(mg *matchedGroup) {
	now := time.Now().Unix()

	// Dedup: skip if this token was alerted within the last dedupWindow.
	dedupMu.Lock()
	lastAlert, exists := dedupMap[mg.TokenId]
	if exists && now-lastAlert < dedupWindow {
		dedupMu.Unlock()
		return
	}
	dedupMap[mg.TokenId] = now
	// Clean old entries.
	for k, v := range dedupMap {
		if now-v > int64(600) {
			delete(dedupMap, k)
		}
	}
	dedupMu.Unlock()

	// Log to system log.
	msg := fmt.Sprintf(
		"[CACHE_ABUSE] token_id=%d token_name=%s user_id=%d model=%s "+
			"count=%d cache_read_diffs=%s cache_write_diffs=%s "+
			"first_at=%d last_at=%d",
		mg.TokenId, mg.TokenName, mg.UserId, mg.ModelName,
		len(mg.Records),
		summarizeDiffs(mg.ReadDiffs),
		summarizeDiffs(mg.WriteDiffs),
		mg.Records[0].CreatedAt,
		mg.Records[len(mg.Records)-1].CreatedAt,
	)
	common.SysLog(msg)

	// Write to Redis for alerting.
	alertKey := fmt.Sprintf("cache_abuse:alert:%d:%d", mg.TokenId, now)
	alertValue := fmt.Sprintf(
		`{"token_id":%d,"token_name":"%s","user_id":%d,"model":"%s",`+
			`"count":%d,"read_diffs":"%s","write_diffs":"%s",`+
			`"first_at":%d,"last_at":%d}`,
		mg.TokenId, mg.TokenName, mg.UserId, mg.ModelName,
		len(mg.Records),
		summarizeDiffs(mg.ReadDiffs),
		summarizeDiffs(mg.WriteDiffs),
		mg.Records[0].CreatedAt,
		mg.Records[len(mg.Records)-1].CreatedAt,
	)
	ttl := 30 * 24 * time.Hour // 30 days
	if err := common.RedisSet(alertKey, alertValue, ttl); err != nil {
		common.SysLog(fmt.Sprintf("[CACHE_ABUSE] failed to write Redis alert: %v", err))
	}
}
