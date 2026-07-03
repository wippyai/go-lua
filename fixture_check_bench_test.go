package lua

import "testing"

var fixtureCheckBenchmarkSuites = []string{
	"semantic/nested-channel-select-union-stress",
	"realworld/transactional-saga-orchestrator-soundness",
	"regression/deadlock-dataflow-node",
	"realworld/advanced-type-system-stress",
	"realworld/plugin-runtime-pipeline-soundness",
	"realworld/wippy-scheduler-create-integration",
}

func BenchmarkFixtureChecks(b *testing.B) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		b.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}

	for _, name := range fixtureCheckBenchmarkSuites {
		s, ok := byName[name]
		if !ok {
			b.Fatalf("missing check benchmark fixture %q", name)
		}
		b.Run(name, func(b *testing.B) {
			diags, entryFile := fixtureDiagnostics(s)
			if verdict := judgeAgainstCuratedExpectations(s, diags, entryFile); !verdict.passed {
				b.Fatalf("fixture %s no longer satisfies curated expectations: missing=%v unexpected=%v", name, verdict.missing, verdict.unexpected)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fixtureDiagnostics(s)
			}
		})
	}
}
