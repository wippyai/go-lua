package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
)

func BenchmarkCompileDetailed(b *testing.B) {
	published, err := lower.Lower(lower.Source{Name: "artifact-compile-bench.lua", Text: []byte("local n = 1\nlocal function add(x) return x + n end\nreturn add(2)")})
	if err != nil {
		b.Fatal(err)
	}
	grammar, ok := programartifact.NewGrammarIdentity(identity.ContentID{1}, programartifact.GrammarABIVersion)
	if !ok {
		b.Fatal("valid grammar identity was rejected")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		artifact, failure := artifactcompiler.CompileDetailed(published, grammar, artifactcompiler.IssuanceDirectory{})
		if failure.Available() || artifact == nil || !artifact.Available() {
			b.Fatalf("artifact compile failed: %s", failure.Error())
		}
	}
}
