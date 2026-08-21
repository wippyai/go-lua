package subjectflow_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
)

// A wide return emits many alias candidates in one body. The body route
// denominator is nevertheless one row, referenced by every candidate.
func TestWideReturnSharesOneAliasRouteScope(t *testing.T) {
	const width = 64
	var source strings.Builder
	source.WriteString("return ")
	for index := 0; index < width; index++ {
		if index != 0 {
			source.WriteString(", ")
		}
		source.WriteString(strconv.Itoa(index + 1))
	}
	program, err := lower.Lower(lower.Source{Name: "alias_route_scope_wide_return.lua", Text: []byte(source.String())})
	if err != nil {
		t.Fatal(err)
	}
	projection := program.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("subject-flow projection")
	}
	if got := projection.AliasRouteScopeCount(); got != 1 {
		t.Fatalf("route scopes = %d, want one body denominator", got)
	}
	scope, scopeOK := projection.AliasRouteScopeAt(0)
	if !scopeOK || !scope.Available() {
		t.Fatal("body route scope")
	}
	if got := projection.AliasCandidateCount(); got < width {
		t.Fatalf("alias candidates = %d, want at least %d return members", got, width)
	}
	for index := 0; index < projection.AliasCandidateCount(); index++ {
		candidate, candidateOK := projection.AliasCandidateAt(index)
		if !candidateOK || !candidate.Available() || candidate.ScopeID() != scope.ID {
			t.Fatalf("candidate %d did not reference the sole body scope", index)
		}
	}
}
