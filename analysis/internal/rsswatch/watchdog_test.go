package rsswatch

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWatchdogExitsSubprocessWithRSSAndAllStacks(t *testing.T) {
	if os.Getenv("GO_LUA_RSSWATCH_HELPER") == "1" {
		poll := make(chan time.Time, 1)
		poll <- time.Now()
		monitor(watchConfig{
			threshold: 1,
			poll:      poll,
			query:     func() (uint64, error) { return 2, nil },
			stderr:    os.Stderr,
			exit:      os.Exit,
		})
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestWatchdogExitsSubprocessWithRSSAndAllStacks$")
	command.Env = append(os.Environ(), "GO_LUA_RSSWATCH_HELPER=1")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != exitCode {
		t.Fatalf("watchdog subprocess = %v, output=%s", err, output)
	}
	if !bytes.Contains(output, []byte("pid=")) || !bytes.Contains(output, []byte("rss=2 bytes threshold=1 bytes")) ||
		!bytes.Contains(output, []byte("goroutine ")) || !bytes.Contains(output, []byte("[running]:")) || !bytes.Contains(output, []byte("goroutine 1 [")) {
		t.Fatalf("watchdog output lacks RSS/stacks:\n%s", output)
	}
}

func TestMonitorIgnoresQueryFailureAndBelowThreshold(t *testing.T) {
	poll := make(chan time.Time, 2)
	poll <- time.Now()
	poll <- time.Now()
	close(poll)
	queries := 0
	exits := 0
	var stderr strings.Builder
	monitor(watchConfig{
		threshold: 8,
		poll:      poll,
		query: func() (uint64, error) {
			queries++
			if queries == 1 {
				return 0, errors.New("unavailable")
			}
			return 8, nil
		},
		stderr: &stderr,
		exit:   func(int) { exits++ },
	})
	if queries != 2 || exits != 0 || stderr.Len() != 0 {
		t.Fatalf("monitor query/exits/stderr = %d/%d/%q", queries, exits, stderr.String())
	}
}

func TestStartOnceLaunchesOneProcessMonitor(t *testing.T) {
	var once sync.Once
	launches := 0
	for range 8 {
		startOnce(&once, func() { launches++ })
	}
	if launches != 1 {
		t.Fatalf("watchdog launches = %d, want 1", launches)
	}
}

func TestThresholdFromEnvDefaultsTo4GiB(t *testing.T) {
	t.Setenv("GOLUA_RSS_LIMIT_MB", "")
	if got, want := thresholdFromEnv(), defaultThreshold; got != want {
		t.Fatalf("thresholdFromEnv() = %d, want %d", got, want)
	}
}
