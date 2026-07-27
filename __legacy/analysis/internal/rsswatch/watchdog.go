// Package rsswatch provides a process-wide machine-safety fuse for analysis.
// It is operational containment only: it never publishes a partial result or
// participates in solver convergence.
package rsswatch

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultThreshold = uint64(4 << 30)
	megabyte         = uint64(1 << 20)
	exitCode         = 86
)

var processWatchdog sync.Once
var observedPeak atomic.Uint64

// Start installs the process-wide RSS watchdog exactly once. Repeated analysis
// runs share the same goroutine and therefore cannot accumulate monitors.
func Start() {
	if !rssSupported() {
		return
	}
	startOnce(&processWatchdog, func() {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			monitor(watchConfig{
				threshold: thresholdFromEnv(),
				poll:      ticker.C,
				query:     currentRSS,
				stderr:    os.Stderr,
				exit:      os.Exit,
			})
		}()
	})
}

func thresholdFromEnv() uint64 {
	limitMB, err := strconv.ParseUint(os.Getenv("GOLUA_RSS_LIMIT_MB"), 10, 64)
	if err != nil || limitMB == 0 || limitMB > ^uint64(0)/megabyte {
		return defaultThreshold
	}
	return limitMB * megabyte
}

// Current returns this process's resident set size in bytes when the host
// exposes it. It is observational only: callers must never use it to change
// semantic convergence or publication.
func Current() (uint64, error) {
	rss, err := currentRSS()
	if err == nil {
		observe(rss)
	}
	return rss, err
}

// Peak returns the greatest resident set size observed by Current or by the
// process watchdog. It performs no host query and is safe on a hot path.
func Peak() uint64 { return observedPeak.Load() }

func observe(rss uint64) {
	for prior := observedPeak.Load(); rss > prior; prior = observedPeak.Load() {
		if observedPeak.CompareAndSwap(prior, rss) {
			return
		}
	}
}

func startOnce(once *sync.Once, launch func()) { once.Do(launch) }

type watchConfig struct {
	threshold uint64
	poll      <-chan time.Time
	query     func() (uint64, error)
	stderr    io.Writer
	exit      func(int)
}

func monitor(config watchConfig) {
	if config.threshold == 0 || config.poll == nil || config.query == nil || config.stderr == nil || config.exit == nil {
		return
	}
	for range config.poll {
		rss, err := config.query()
		if err == nil {
			observe(rss)
		}
		if err != nil || rss <= config.threshold {
			continue
		}
		fmt.Fprintf(config.stderr, "go-lua: RSS safety fuse exceeded: pid=%d rss=%d bytes threshold=%d bytes\n", os.Getpid(), rss, config.threshold)
		writeAllGoroutineStacks(config.stderr)
		config.exit(exitCode)
		return
	}
}

func writeAllGoroutineStacks(dst io.Writer) {
	buffer := make([]byte, 1<<20)
	for {
		written := runtime.Stack(buffer, true)
		if written < len(buffer) {
			_, _ = dst.Write(buffer[:written])
			return
		}
		buffer = make([]byte, len(buffer)*2)
	}
}
