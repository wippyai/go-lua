package engine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/compiler/parse"
)

const corpusFileTimeout = 5 * time.Second

// TestCorpusSmoke is deliberately an availability probe, not an oracle.
// Fixture source is the only input: manifests, expected diagnostics, and every
// other oracle assertion remain unread.
func TestCorpusSmoke(t *testing.T) {
	if os.Getenv("CORPUS_SMOKE") != "1" {
		t.Skip("set CORPUS_SMOKE=1 to sweep the legacy fixture source corpus")
	}
	root := corpusRepositoryRoot(t)
	fixtures := filepath.Join(root, "__legacy", "harness_data", "fixtures")
	files, err := corpusLuaFiles(fixtures)
	if err != nil {
		t.Fatalf("discover corpus: %v", err)
	}

	started := time.Now()
	runner := newCorpusRunner(corpusFileTimeout)
	defer runner.close()
	timings := make([]time.Duration, 0, len(files))
	maxFile := ""
	maxDuration := time.Duration(0)
	completed := 0
	failures := make([]corpusFailure, 0)
	for _, file := range files {
		source, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("read corpus input %s: %v", file, readErr)
		}
		duration, err, panicked, timedOut := runner.check(string(source))
		timings = append(timings, duration)
		rel, relErr := filepath.Rel(fixtures, file)
		if relErr != nil {
			t.Fatalf("relativize corpus input %s: %v", file, relErr)
		}
		if duration > maxDuration {
			maxDuration = duration
			maxFile = filepath.ToSlash(rel)
		}
		if !timedOut {
			completed++
		}
		switch {
		case timedOut:
			failures = append(failures, corpusFailure{file: filepath.ToSlash(rel), class: "timeout"})
		case panicked != nil:
			failures = append(failures, corpusFailure{file: filepath.ToSlash(rel), class: "panic/" + fmt.Sprintf("%T", panicked)})
		case err != nil:
			failures = append(failures, corpusFailure{file: filepath.ToSlash(rel), class: corpusErrorClass(err), err: err})
		}
	}

	sort.Slice(failures, func(i, j int) bool { return failures[i].file < failures[j].file })
	for _, failure := range failures {
		t.Logf("CORPUS_SMOKE failure %s -> %s", failure.file, failure.class)
		if os.Getenv("CORPUS_SMOKE_FULL") == "1" && failure.err != nil {
			t.Logf("CORPUS_SMOKE detail %s -> %s", failure.file, failure.err)
		}
	}
	p50, p95, max := corpusPercentiles(timings)
	t.Logf("CORPUS_SMOKE total=%d completed=%d named_failures=%d", len(files), completed, len(failures))
	t.Logf("CORPUS_SMOKE WALL TIME total=%s p50=%s p95=%s max=%s", time.Since(started), p50, p95, max)
	if maxDuration > 2*time.Second {
		t.Logf("CORPUS_SMOKE PERF file=%s duration=%s exceeds=2s", maxFile, maxDuration)
	}
}

type corpusFailure struct {
	file  string
	class string
	err   error
}

type corpusCheckResult struct {
	err      error
	panicked any
}

// corpusRunner owns one timer for the sequential corpus sweep.  Reusing it
// avoids an otherwise short-lived timer allocation for every source file.
type corpusRunner struct {
	timeout time.Duration
	timer   *time.Timer
}

func newCorpusRunner(timeout time.Duration) *corpusRunner {
	timer := time.NewTimer(timeout)
	if !timer.Stop() {
		<-timer.C
	}
	return &corpusRunner{timeout: timeout, timer: timer}
}

func (r *corpusRunner) close() {
	if r != nil && r.timer != nil {
		r.timer.Stop()
	}
}

func (r *corpusRunner) check(source string) (time.Duration, error, any, bool) {
	started := time.Now()
	result := make(chan corpusCheckResult, 1)
	go func() {
		outcome := corpusCheckResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				outcome.panicked = recovered
			}
			result <- outcome
		}()
		_, outcome.err = engine.Check(source)
	}()
	r.timer.Reset(r.timeout)
	select {
	case outcome := <-result:
		if !r.timer.Stop() {
			select {
			case <-r.timer.C:
			default:
			}
		}
		return time.Since(started), outcome.err, outcome.panicked, false
	case <-r.timer.C:
		return time.Since(started), nil, nil, true
	}
}

func checkCorpusFile(source string, timeout time.Duration) (time.Duration, error, any, bool) {
	runner := newCorpusRunner(timeout)
	defer runner.close()
	return runner.check(source)
}

func corpusLuaFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".lua" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func corpusRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository go.mod")
		}
		dir = parent
	}
}

func corpusErrorClass(err error) string {
	var parseErr *parse.Error
	switch {
	case errors.Is(err, engine.ErrInternalPanic):
		return "engine.internal-panic"
	case errors.Is(err, front.ErrUnsupportedInstruction):
		return "front.unsupported-instruction"
	case errors.As(err, &parseErr):
		return "front.parse"
	}
	message := err.Error()
	for _, prefix := range []string{"engine: compile whole file: front: ", "engine: evaluate: ", "engine: "} {
		if strings.HasPrefix(message, prefix) {
			message = strings.TrimPrefix(message, prefix)
			break
		}
	}
	if boundary := strings.IndexByte(message, ':'); boundary > 0 {
		message = message[:boundary]
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "error"
	}
	return strings.ReplaceAll(message, " ", "-")
}

func corpusPercentiles(values []time.Duration) (time.Duration, time.Duration, time.Duration) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	nearestRank := func(percent int) time.Duration {
		index := (len(sorted)*percent + 99) / 100
		return sorted[index-1]
	}
	return nearestRank(50), nearestRank(95), sorted[len(sorted)-1]
}
