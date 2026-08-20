package compiler_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/domain/composite"
)

// populationCompileFixtures names the fixtures BenchmarkPopulationCompile
// exercises: two structural-conformance fixtures (a wrong-typed field and a
// missing required field, so both the member and absent-member populations
// emit rows) and one branch-bearing fixture (so the branch-condition
// population emits a row too). coldCompileFixture in
// cold_compile_bench_test.go emits zero rows from either population under
// review in the hostile review (finding 10); this benchmark exists so a
// population change is visible in allocs/op and B/op.
var populationCompileFixtures = []string{
	"types/wrong-field-type",
	"types/missing-field",
	"flow/if-else",
}

func populationCompileFixtureSources(tb testing.TB) []lower.Source {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("cannot resolve population fixture location")
	}
	repository := populationRepositoryRoot(tb, filepath.Dir(thisFile))
	sources := make([]lower.Source, len(populationCompileFixtures))
	for index, name := range populationCompileFixtures {
		path := filepath.Join(repository, "testdata", "fixtures", filepath.FromSlash(name), "main.lua")
		text, err := os.ReadFile(path)
		if err != nil {
			tb.Fatalf("read fixture %s: %v", name, err)
		}
		sources[index] = lower.Source{Name: name + "/main.lua", Text: text}
	}
	return sources
}

func populationRepositoryRoot(tb testing.TB, start string) string {
	tb.Helper()
	for directory := filepath.Clean(start); ; {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			tb.Fatalf("cannot locate repository root from %q", start)
		}
		directory = parent
	}
}

// BenchmarkPopulationCompile compiles the fixtures above through the real
// artifact compiler with the real, registry-sealed IssuanceDirectory (the
// same directory analysis/compile.go builds via
// composite.ArtifactIssuanceDirectory, not the empty IssuanceDirectory{}
// cold_compile_bench_test.go uses). Gate on allocs/op and B/op; treat ns/op
// as advisory below a 30% delta at this benchtime, per finding 10's amendment.
func BenchmarkPopulationCompile(b *testing.B) {
	sources := populationCompileFixtureSources(b)
	grammar, grammarOK := programartifact.NewGrammarIdentity(identity.ContentID{1}, programartifact.GrammarABIVersion)
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory()
	if !grammarOK || !issuanceOK {
		b.Fatal("population benchmark inputs")
	}
	for _, source := range sources {
		b.Run(source.Name, func(b *testing.B) {
			publishedProgram, err := lower.Lower(source)
			if err != nil || publishedProgram == nil || !publishedProgram.Available() {
				b.Fatalf("lower %s: %v", source.Name, err)
			}
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				artifact, failure := artifactcompiler.CompileDetailed(publishedProgram, grammar, issuance)
				if failure.Available() || artifact == nil || !artifact.Available() {
					b.Fatalf("compile %s: %s", source.Name, failure.Error())
				}
			}
		})
	}
}
