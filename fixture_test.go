package lua

import "testing"

func TestFixtures(t *testing.T) {
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
			t.Run("check", func(t *testing.T) {
				runCheckPhase(t, s)
			})
			t.Run("run", func(t *testing.T) {
				runExecPhase(t, s)
			})
		})
	}
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
