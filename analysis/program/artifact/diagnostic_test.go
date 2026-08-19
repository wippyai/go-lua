package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestCompileFailureIsClosedAndFailClosed(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "diagnostic-failure.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	_, failure := programartifact.CompileDetailed(published, programartifact.GrammarIdentity{}, programartifact.IssuanceDirectory{})
	if !failure.Available() || failure.Stage() != programartifact.CompileStageAuthority || failure.RowKind() != programartifact.CompileRowAuthority || failure.Reason() != programartifact.CompileReasonGrammarUnavailable {
		t.Fatalf("invalid grammar did not produce a closed authority failure: %s", failure.Error())
	}
	if failure.Error() == (programartifact.CompileFailure{}).Error() {
		t.Fatal("a real compile failure was indistinguishable from success")
	}
	if success := (programartifact.CompileFailure{}); success.Available() || success.Error() != "program artifact compile succeeded" {
		t.Fatal("zero compile failure was not the closed success state")
	}
}
