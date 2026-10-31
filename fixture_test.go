package lua

import (
	"fmt"
	"os"
	"runtime"
	"testing"
)

func TestFixtures(t *testing.T) {
	if os.Getenv("WIPPY_FIXTURE_MEM") == "1" {
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		defer func() {
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			fmt.Printf("FIXTURE_TOTAL_ALLOC_BYTES=%d\n", after.TotalAlloc-before.TotalAlloc)
		}()
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("no fixture suites found")
	}
	for _, s := range suites {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			if s.Suite.Skip != "" {
				t.Skip(s.Suite.Skip)
			}
			for _, mode := range checkModes(s) {
				name := "check"
				if mode != modeGradual {
					name = "check-" + mode
				}
				t.Run(name, func(t *testing.T) {
					runCheckPhase(t, s, mode)
				})
			}
			t.Run("run", func(t *testing.T) {
				runExecPhase(t, s)
			})
		})
	}
}

// TestFixturesFixpointReplay compares final facts and diagnostics with the
// original full schedule for every fixture and every supported checking mode.
// It doubles the fixture suite, so it runs locally on request:
// WIPPY_FIXPOINT_REPLAY=1 go test -run '^TestFixturesFixpointReplay$' .
func TestFixturesFixpointReplay(t *testing.T) {
	if os.Getenv("WIPPY_FIXPOINT_REPLAY") != "1" {
		t.Skip("set WIPPY_FIXPOINT_REPLAY=1 to replay every fixture against the original fixpoint schedule")
	}
	t.Setenv("WIPPY_FIXPOINT_ASSERT", "1")
	TestFixtures(t)
}

func BenchmarkFixtures(b *testing.B) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		b.Fatalf("discovering fixtures: %v", err)
	}
	for _, s := range suites {
		if s.Suite.Bench == nil {
			continue
		}
		s := s
		b.Run(s.Name, func(b *testing.B) {
			runBenchPhase(b, s)
		})
	}
}

func TestFixtureOrder_GenericRegistryThenMultiReturn(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}

	var generic namedSuite
	var multi namedSuite
	for _, s := range suites {
		switch s.Name {
		case "realworld/generic-registry":
			generic = s
		case "realworld/multi-return-error-chain":
			multi = s
		}
	}

	if generic.Name == "" || multi.Name == "" {
		t.Fatalf("missing target suites: generic=%q multi=%q", generic.Name, multi.Name)
	}

	runCheckPhase(t, generic, modeGradual)
	runCheckPhase(t, multi, modeGradual)
}
