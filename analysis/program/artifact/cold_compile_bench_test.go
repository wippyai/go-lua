package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
)

const coldCompileFixture = "local n = 1\nlocal function add(x) return x + n end\nreturn add(2)"

func coldCompileFixtureProgram(tb testing.TB) (*program.Program, GrammarIdentity) {
	tb.Helper()
	published, err := lower.Lower(lower.Source{Name: "artifact-compile-bench.lua", Text: []byte(coldCompileFixture)})
	if err != nil {
		tb.Fatal(err)
	}
	grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion)
	if !ok {
		tb.Fatal("valid grammar identity was rejected")
	}
	return published, grammar
}

func BenchmarkColdCompilePhases(b *testing.B) {
	source := lower.Source{Name: "artifact-compile-bench.lua", Text: []byte(coldCompileFixture)}
	published, grammar := coldCompileFixtureProgram(b)
	sealed, failure := CompileDetailed(published, grammar, nil)
	if failure.Available() || sealed == nil || !sealed.Available() {
		b.Fatalf("artifact compile failed: %s", failure.Error())
	}
	b.Run("lower", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			got, err := lower.Lower(source)
			if err != nil || got == nil || !got.Available() {
				b.Fatal(err)
			}
		}
	})
	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			artifact, compileFailure := CompileDetailed(published, grammar, nil)
			if compileFailure.Available() || artifact == nil || !artifact.Available() {
				b.Fatalf("artifact compile failed: %s", compileFailure.Error())
			}
		}
	})
	b.Run("identity", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if id := artifactID(sealed); id != sealed.id {
				b.Fatal("seal identity changed under replay")
			}
		}
	})
}
