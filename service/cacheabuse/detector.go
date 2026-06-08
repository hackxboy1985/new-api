package cacheabuse

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/model"
)

// logRecord is a lightweight row from the logs table.
type logRecord struct {
	Id               int
	UserId           int
	TokenId          int
	TokenName        string
	ModelName        string
	CreatedAt        int64
	PromptTokens     int
	CompletionTokens int
	Other            string // JSON blob
}

// cacheMeta holds parsed cache fields from the other JSON column.
type cacheMeta struct {
	Claude                bool `json:"claude"`
	CacheTokens           int  `json:"cache_tokens"`
	CacheCreationTokens   int  `json:"cache_creation_tokens"`
	CacheCreationTokens5m int  `json:"cache_creation_tokens_5m"`
}

func (m cacheMeta) totalCacheWrite() int {
	if m.CacheCreationTokens5m > 0 {
		return m.CacheCreationTokens5m
	}
	return m.CacheCreationTokens
}

// nonCachedInput computes true non-cache input tokens.
// Claude: PromptTokens already excludes cache. OpenAI: PromptTokens includes cache.
func (m cacheMeta) nonCachedInput(promptTokens int) int {
	if m.Claude {
		return promptTokens
	}
	v := promptTokens - m.CacheTokens - m.CacheCreationTokens
	if v < 0 {
		return 0
	}
	return v
}

func fetchRecentLogs(cutoff int64) ([]logRecord, error) {
	var rows []logRecord
	err := model.LOG_DB.
		Table("logs").
		Select("id, user_id, token_id, token_name, model_name, created_at, prompt_tokens, completion_tokens, other").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, cutoff).
		Order("token_id ASC, created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// matchedGroup holds consecutive matching requests for one token.
type matchedGroup struct {
	TokenId    int
	TokenName  string
	UserId     int
	ModelName  string
	Records    []logRecord
	ReadDiffs  []int
	WriteDiffs []int
}

func detectAbuse(records []logRecord) {
	if len(records) < 2 {
		return
	}

	// Group by token_id (records are already ordered by token_id, created_at).
	var groups [][]logRecord
	var current []logRecord
	var currentTokenId int

	for _, r := range records {
		if r.TokenId != currentTokenId {
			if len(current) >= 2 {
				groups = append(groups, current)
			}
			current = nil
			currentTokenId = r.TokenId
		}
		current = append(current, r)
	}
	if len(current) >= 2 {
		groups = append(groups, current)
	}

	for _, group := range groups {
		matched := scanGroup(group)
		if matched != nil {
			emit(matched)
		}
	}
}

func scanGroup(group []logRecord) *matchedGroup {
	// Parse all cache metadata once.
	type enriched struct {
		rec  logRecord
		meta cacheMeta
	}
	parsed := make([]enriched, len(group))
	for i, r := range group {
		var m cacheMeta
		if r.Other != "" {
			_ = json.Unmarshal([]byte(r.Other), &m)
		}
		parsed[i] = enriched{rec: r, meta: m}
	}

	c := getConfig()
	windowSec := int64(c.TimeWindow)

	// Find the longest consecutive matching sequence.
	var bestMatch *matchedGroup
	var matchStart int
	inMatch := false

	for i := 1; i < len(parsed); i++ {
		prev := parsed[i-1]
		curr := parsed[i]

		ok := isPairMatch(prev, curr, windowSec, c)
		if ok {
			if !inMatch {
				matchStart = i - 1
				inMatch = true
			}
			if i == len(parsed)-1 {
				mg := buildMatch(parsed[matchStart : i+1])
				if bestMatch == nil || len(mg.Records) > len(bestMatch.Records) {
					bestMatch = mg
				}
			}
		} else if inMatch {
			mg := buildMatch(parsed[matchStart:i])
			if bestMatch == nil || len(mg.Records) > len(bestMatch.Records) {
				bestMatch = mg
			}
			inMatch = false
		}
	}
	return bestMatch
}

func isPairMatch(prev, curr enriched, windowSec int64, c config) bool {
	if curr.rec.CreatedAt-prev.rec.CreatedAt > windowSec {
		return false
	}

	cr1 := prev.meta.CacheTokens
	cr2 := curr.meta.CacheTokens
	cw1 := prev.meta.totalCacheWrite()
	cw2 := curr.meta.totalCacheWrite()

	if abs(cr1-cr2) > c.ReadDiff {
		return false
	}
	if cw1 <= c.WriteMin || cw2 <= c.WriteMin {
		return false
	}
	if abs(cw1-cw2) > c.WriteDiff {
		return false
	}

	nc1 := prev.meta.nonCachedInput(prev.rec.PromptTokens)
	nc2 := curr.meta.nonCachedInput(curr.rec.PromptTokens)
	if nc1 > c.NonCachedInputMax || nc2 > c.NonCachedInputMax {
		return false
	}
	if prev.rec.CompletionTokens > c.NonCachedOutputMax || curr.rec.CompletionTokens > c.NonCachedOutputMax {
		return false
	}

	return true
}

func buildMatch(parsed []enriched) *matchedGroup {
	if len(parsed) < 2 {
		return nil
	}
	mg := &matchedGroup{
		TokenId:   parsed[0].rec.TokenId,
		TokenName: parsed[0].rec.TokenName,
		UserId:    parsed[0].rec.UserId,
		ModelName: parsed[0].rec.ModelName,
	}
	for i := range parsed {
		mg.Records = append(mg.Records, parsed[i].rec)
	}
	for i := 1; i < len(parsed); i++ {
		mg.ReadDiffs = append(mg.ReadDiffs,
			abs(parsed[i].meta.CacheTokens-parsed[i-1].meta.CacheTokens))
		mg.WriteDiffs = append(mg.WriteDiffs,
			abs(parsed[i].meta.totalCacheWrite()-parsed[i-1].meta.totalCacheWrite()))
	}
	return mg
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func summarizeDiffs(diffs []int) string {
	if len(diffs) == 0 {
		return "[]"
	}
	sorted := make([]int, len(diffs))
	copy(sorted, diffs)
	sort.Ints(sorted)
	return fmt.Sprintf("min=%d max=%d", sorted[0], sorted[len(sorted)-1])
}
