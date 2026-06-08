package cacheabuse

import (
	"os"
	"sync"
)

type config struct {
	Enabled           bool
	ScanInterval      int // seconds between scans
	Lookback          int // seconds to look back in logs
	ReadDiff          int // max cache read diff
	WriteMin          int // min cache write threshold
	WriteDiff         int // max cache write diff
	NonCachedInputMax  int // max non-cache input tokens
	NonCachedOutputMax int // max non-cache output tokens
	TimeWindow        int // max seconds between consecutive requests
}

var (
	cfg    config
	cfgOnce sync.Once
)

// getConfig lazy-loads config on first call (after .env is loaded by main).
func getConfig() config {
	cfgOnce.Do(func() {
		cfg = config{
			Enabled:           getEnvBool("CACHE_ABUSE_SCAN_ENABLED", true),
			ScanInterval:      getEnvInt("CACHE_ABUSE_SCAN_INTERVAL", 60),
			Lookback:          getEnvInt("CACHE_ABUSE_LOOKBACK_SECONDS", 120),
			ReadDiff:          getEnvInt("CACHE_ABUSE_READ_DIFF", 100),
			WriteMin:          getEnvInt("CACHE_ABUSE_WRITE_MIN", 10000),
			WriteDiff:         getEnvInt("CACHE_ABUSE_WRITE_DIFF", 2000),
			NonCachedInputMax:  getEnvInt("CACHE_ABUSE_NONCACHED_INPUT_MAX", 5),
			NonCachedOutputMax: getEnvInt("CACHE_ABUSE_NONCACHED_OUTPUT_MAX", 1000),
			TimeWindow:        getEnvInt("CACHE_ABUSE_TIME_WINDOW_SECONDS", 60),
		}
	})
	return cfg
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
