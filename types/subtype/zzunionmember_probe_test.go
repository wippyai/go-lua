package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZUnionMemberAssign pins the soundness boundary behind the
// plugin-supervisor-runtime "cannot assign Output to RenderOutput" diagnostic:
// a union (Output = RenderOutput | IndexOutput | AuditOutput) is NOT a subtype
// of one of its members (RenderOutput). Assigning the whole union to a single
// variant must stay rejected -- IndexOutput/AuditOutput values would flow into a
// RenderOutput slot otherwise. The correct fix for the fixture is discriminant
// narrowing of the source path BEFORE the assign-check (canonical observation),
// not a relaxation here.
func TestZZUnionMemberAssign(t *testing.T) {
	render := discriminatedRecord("rendered", "body", typ.String)
	index := discriminatedRecord("indexed", "count", typ.Integer)
	audit := discriminatedRecord("audited", "note", typ.String)

	output := typ.NewUnion(render, index, audit)

	// Soundness: the union is NOT a subtype of one member.
	if IsSubtype(output, render) {
		t.Errorf("IsSubtype(Output union, RenderOutput) = true, want false (union to one member is unsound)")
	}

	// The narrowed source (the single member) IS a subtype of itself: once
	// discriminant narrowing reduces Output to RenderOutput the assignment is
	// accepted.
	if !IsSubtype(render, render) {
		t.Errorf("IsSubtype(RenderOutput, RenderOutput) = false, want true")
	}

	// A single member IS a subtype of the union (covariant inject).
	if !IsSubtype(render, output) {
		t.Errorf("IsSubtype(RenderOutput, Output union) = false, want true")
	}

	// A distinct member must NOT be a subtype of RenderOutput.
	if IsSubtype(index, render) {
		t.Errorf("IsSubtype(IndexOutput, RenderOutput) = true, want false (distinct discriminated variant)")
	}
}

// discriminatedRecord builds a record with a literal-string discriminant `kind`
// field plus one payload field.
func discriminatedRecord(kindLit, payload string, payloadType typ.Type) *typ.Record {
	return typ.NewRecord().
		Field("kind", typ.LiteralString(kindLit)).
		Field(payload, payloadType).
		Build()
}
