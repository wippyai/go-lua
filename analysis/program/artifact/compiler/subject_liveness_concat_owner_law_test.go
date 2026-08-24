package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/composite"
)

// subjectLivenessConcatSource is one authored program whose `..` result is
// also a Flow suspension subject: the concatenated left operand is the first
// result of a call, so the concat term carries a subject-liveness row.
const subjectLivenessConcatSource = `
local function f()
    return "a", 1
end
local s = f() .. "b"
`

// TestArtifactSubjectLivenessResolvesConcatResultOwner states that the
// artifact compiler can resolve the Program owner of an executable concat
// whose result carries a Flow subject-liveness row.
//
// Concat carries no Flow operator primitive, so its mounted identity is the
// artifact occurrence issued at the concat's evaluation span. That occurrence
// is the authority the liveness join authenticates against, which makes the
// occurrence catalog a precondition of the liveness projection rather than a
// later independent plane. A program whose concat result is a suspension
// subject must therefore compile, and its published liveness plane must name
// the concat's own span identity.
func TestArtifactSubjectLivenessResolvesConcatResultOwner(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "subject-liveness-concat.lua", Text: []byte(subjectLivenessConcatSource)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, lowered, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("concat subject-liveness compilation failed: %s", failure.Error())
	}
	concatSpans := make(map[identity.ContentID]keyspace.Term)
	candidates := lowered.Flow().Candidates()
	if candidates == nil {
		t.Fatal("flow candidate buckets are unavailable")
	}
	concat := candidates.Concat()
	for index := 0; index < concat.Count(); index++ {
		term, termOK := concat.At(index)
		span, spanOK := lowered.Span(term)
		if !termOK || !spanOK || !lowered.OwnsSpan(span) {
			t.Fatalf("concat candidate %d carries no owned evaluation span", index)
		}
		concatSpans[span.ContextID()] = term
	}
	if len(concatSpans) == 0 {
		t.Fatal("authored concat plane is empty")
	}
	state, stateOK := artifact.Program().ColdState()
	if !stateOK {
		t.Fatal("program cold state is not published")
	}
	view, viewOK := lifecycle.NewView(state)
	if !viewOK {
		t.Fatal("lifecycle plane is not published")
	}
	count, published := view.SubjectLivenessSpanCount()
	if !published {
		t.Fatal("subject liveness span family is not published")
	}
	named := false
	for index := 0; index < count; index++ {
		row, rowOK := view.SubjectLivenessSpanAt(index)
		if !rowOK {
			t.Fatalf("subject liveness span %d is unavailable", index)
		}
		if _, isConcat := concatSpans[row.SubjectID()]; !isConcat {
			continue
		}
		if row.SubjectKind() != lifecycle.SubjectLivenessValue {
			t.Fatalf("concat subject %v is published as kind %v", row.SubjectID(), row.SubjectKind())
		}
		named = true
	}
	if !named {
		t.Fatal("no published liveness span names the concat result's span identity")
	}
}
