package registrycache

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func TestGetForPassesRegistryAndCaches(t *testing.T) {
	var cache Cache[int]
	reg := axis.NewRegistry()
	builds := 0
	build := func(got *axis.Registry) int {
		if got != reg {
			t.Fatalf("build got registry %p, want %p", got, reg)
		}
		builds++
		return 42
	}

	if got := cache.GetFor(reg, build); got != 42 {
		t.Fatalf("first GetFor = %d, want 42", got)
	}
	if got := cache.GetFor(reg, build); got != 42 {
		t.Fatalf("second GetFor = %d, want 42", got)
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1", builds)
	}
}

func TestGetForCacheHitDoesNotAllocateBuilderClosure(t *testing.T) {
	var cache Cache[int]
	reg := axis.NewRegistry()
	_ = cache.GetFor(reg, buildRegistryCacheTestInt)

	allocs := testing.AllocsPerRun(1000, func() {
		_ = cache.GetFor(reg, buildRegistryCacheTestInt)
	})
	if allocs != 0 {
		t.Fatalf("GetFor cache-hit allocations/run = %.1f, want zero", allocs)
	}
}

func buildRegistryCacheTestInt(*axis.Registry) int {
	return 1
}

func TestGetForConcurrentMissSharesSingleBuild(t *testing.T) {
	var cache Cache[int]
	reg := axis.NewRegistry()
	start := make(chan struct{})
	entered := make(chan struct{})
	release := make(chan struct{})
	var builds atomic.Int32

	build := func(got *axis.Registry) int {
		if got != reg {
			t.Fatalf("build got registry %p, want %p", got, reg)
		}
		if builds.Add(1) == 1 {
			close(entered)
		}
		<-release
		return 99
	}

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if got := cache.GetFor(reg, build); got != 99 {
				t.Errorf("GetFor = %d, want 99", got)
			}
		}()
	}

	close(start)
	<-entered
	close(release)
	wg.Wait()

	if got := builds.Load(); got != 1 {
		t.Fatalf("builds = %d, want exactly one in-flight build", got)
	}
}

func TestGetForPanickingBuildDoesNotPoisonCache(t *testing.T) {
	var cache Cache[int]
	reg := axis.NewRegistry()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("first build did not panic")
			}
		}()
		_ = cache.GetFor(reg, func(*axis.Registry) int {
			panic("boom")
		})
	}()

	builds := 0
	got := cache.GetFor(reg, func(*axis.Registry) int {
		builds++
		return 7
	})
	if got != 7 {
		t.Fatalf("second GetFor = %d, want 7", got)
	}
	if builds != 1 {
		t.Fatalf("second build count = %d, want 1", builds)
	}
}
