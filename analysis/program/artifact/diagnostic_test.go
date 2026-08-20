package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
)

func TestCompileFailureIsClosedAndFailClosed(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "diagnostic-failure.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	_, failure := artifactcompiler.CompileDetailed(published, programartifact.GrammarIdentity{}, artifactcompiler.IssuanceDirectory{})
	if !failure.Available() || failure.Stage() != artifactcompiler.CompileStageAuthority || failure.RowKind() != artifactcompiler.CompileRowAuthority || failure.Reason() != artifactcompiler.CompileReasonGrammarUnavailable {
		t.Fatalf("invalid grammar did not produce a closed authority failure: %s", failure.Error())
	}
	if failure.Error() == (artifactcompiler.CompileFailure{}).Error() {
		t.Fatal("a real compile failure was indistinguishable from success")
	}
	if success := (artifactcompiler.CompileFailure{}); success.Available() || success.Error() != "program artifact compile succeeded" {
		t.Fatal("zero compile failure was not the closed success state")
	}
}
