package lua_test

import (
	lua "github.com/wippyai/go-lua"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestReviewCoverageFiltersCanReadCoverage(t *testing.T) {
	if !lua.CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	source := "@" + filepath.ToSlash(filepath.Join(t.TempDir(), "filter.lua"))
	L := lua.NewState()
	defer L.Close()
	proto, err := lua.CompileString("return 42", source)
	if err != nil {
		t.Fatal(err)
	}
	L.Push(L.LoadProto(proto))
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"summary", "LCOV"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "coverage.info")
			done := make(chan error, 1)
			go func() {
				nested := false
				filter := func(s string) bool {
					if !nested {
						nested = true
						lua.CoverageSummary(nil)
					}
					return s == source
				}
				if operation == "summary" {
					lua.CoverageSummary(filter)
					done <- nil
				} else {
					done <- lua.WriteCoverageLCOV(path, filter)
				}
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("coverage callback deadlocked while reading the collector")
			}
		})
	}
}

func TestReviewCoverageConcurrentExecutionAndReports(t *testing.T) {
	if !lua.CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	source := "@" + filepath.ToSlash(filepath.Join(t.TempDir(), "concurrent.lua"))
	proto, err := lua.CompileString("return 42", source)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			L := lua.NewState()
			defer L.Close()
			for j := 0; j < 30; j++ {
				L.Push(L.LoadProto(proto))
				if err := L.PCall(0, 0, nil); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	filter := func(s string) bool { return s == source }
	path := filepath.Join(t.TempDir(), "concurrent.info")
	for i := 0; i < 5; i++ {
		if err := lua.WriteCoverageLCOV(path, filter); err != nil {
			t.Error(err)
		}
		found, hit := lua.CoverageSummary(filter)
		if found < 1 || hit > found {
			t.Errorf("inconsistent snapshot %d/%d", hit, found)
		}
	}
	workers.Wait()
	if _, hit := lua.CoverageSummary(filter); hit == 0 {
		t.Fatal("concurrent execution supplied no line hits")
	}
}
