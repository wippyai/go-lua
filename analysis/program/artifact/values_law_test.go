package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func valuesLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func TestProgramArtifactValuesRowsPreserveOrderAndTail(t *testing.T) {
	row := ValuesRow{
		id:      valuesLawID(1),
		body:    valuesLawID(2),
		span:    valuesLawID(6),
		members: []ValuesMember{{id: valuesLawID(3)}, {id: valuesLawID(4)}},
		tail:    ValuesTail{id: valuesLawID(5), span: valuesLawID(7), kind: ValuesTailCall, present: true},
		sealed:  true,
	}
	if !row.Available() {
		t.Fatal("valid Values row unavailable")
	}
	for index, want := range []identity.ContentID{valuesLawID(3), valuesLawID(4)} {
		member, ok := row.MemberAt(index)
		if !ok || member.ID() != want {
			t.Fatalf("MemberAt(%d) = %v,%v; want %v,true", index, member.ID(), ok, want)
		}
	}
	tail, ok := row.Tail()
	rootSpan, rootSpanOK := row.RootSpanID()
	if !rootSpanOK || rootSpan != valuesLawID(6) {
		t.Fatal("Values row did not preserve its canonical root span")
	}
	if !ok || !tail.Present() || tail.Kind() != ValuesTailCall || tail.ID() != valuesLawID(5) || tail.SpanID() != valuesLawID(7) {
		t.Fatal("open Values tail did not preserve its canonical scalar row")
	}
}

func TestProgramArtifactValuesTailShapeFailsClosed(t *testing.T) {
	invalid := []ValuesTail{
		{id: valuesLawID(1), span: valuesLawID(2), kind: ValuesTailInvalid, present: true},
		{id: identity.ContentID{}, span: valuesLawID(2), kind: ValuesTailCall, present: true},
		{id: valuesLawID(1), kind: ValuesTailCall, present: true},
		{id: valuesLawID(1), span: valuesLawID(2), kind: ValuesTailCall, present: false},
	}
	for index, tail := range invalid {
		if tail.Available() {
			t.Fatalf("invalid tail %d was available", index)
		}
	}
	if !(ValuesTail{}).Available() {
		t.Fatal("closed Values tail was unavailable")
	}
}
