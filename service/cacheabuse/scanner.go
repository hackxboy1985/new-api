package cacheabuse

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

var (
	startOnce      sync.Once
	scannerStarted atomic.Bool
	scannerRunning atomic.Bool
)

// init auto-starts the scanner via package import.
// It polls until model.LOG_DB is available, then begins scanning.
func init() {
	gopool.Go(func() {
		// Wait for LOG_DB to be initialized (happens in main goroutine).
		for i := 0; i < 300; i++ {
			if model.LOG_DB != nil && common.IsMasterNode {
				break
			}
			time.Sleep(time.Second)
		}
		if model.LOG_DB == nil {
			common.SysLog("[CACHE_ABUSE] LOG_DB not available, scanner disabled")
			return
		}
		start()
	})
}

// start begins the background scanner. Safe to call multiple times.
func start() {
	startOnce.Do(func() {
		c := getConfig()
		if !c.Enabled {
			return
		}
		if !scannerStarted.CompareAndSwap(false, true) {
			return
		}
		logger.LogInfo(context.Background(), fmt.Sprintf(
			"cache abuse scanner started: interval=%ds lookback=%ds",
			c.ScanInterval, c.Lookback,
		))
		runScanOnce()
		ticker := time.NewTicker(time.Duration(c.ScanInterval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runScanOnce()
		}
	})
}

func runScanOnce() {
	if !scannerRunning.CompareAndSwap(false, true) {
		return
	}
	defer scannerRunning.Store(false)

	c := getConfig()
	cutoff := time.Now().Unix() - int64(c.Lookback)
	records, err := fetchRecentLogs(cutoff)
	if err != nil {
		common.SysLog(fmt.Sprintf("[CACHE_ABUSE] failed to fetch logs: %v", err))
		return
	}
	if len(records) == 0 {
		return
	}

	detectAbuse(records)
}
