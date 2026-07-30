package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestApplySegmentsUsesProductiveRecursiveProjection(t *testing.T) {
	path := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	node := typ.NewRecursivePlaceholder("Node")
	node.SetBody(&typ.Union{Members: []typ.Type{
		node,
		typetable.NewRecord().Field("value", typ.String).Build(),
	}})
	got, ok := ApplySegments(node, path)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("recursive projection = %v/%v, want string", got, ok)
	}

	bad := typ.NewRecursivePlaceholder("Bad")
	bad.SetBody(&typ.Union{Members: []typ.Type{bad, typ.Boolean}})
	if got, ok := ApplySegments(bad, path); ok || got != nil {
		t.Fatalf("productive projection mismatch = %v/%v, want failure", got, ok)
	}
}

func TestConstructorProjectionTraversesDeepAcyclicPath(t *testing.T) {
	var source typ.Type = typ.String
	path := make([]segment.Segment, 257)
	for i := len(path) - 1; i >= 0; i-- {
		path[i] = segment.Segment{Kind: segment.SegmentField, Name: "next"}
		source = typetable.NewRecord().Field("next", source).Build()
	}
	got, ok := ExpectedConstructorEntryType(source, path)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("deep constructor projection = %v/%v, want string", got, ok)
	}
}

func TestDynamicWriteUsesProductiveRecursiveContract(t *testing.T) {
	container := typ.NewRecursivePlaceholder("Container")
	container.SetBody(&typ.Union{Members: []typ.Type{
		container,
		typ.NewMap(typ.String, typ.Number),
	}})
	got, ok := DynamicWriteValueType(container, typ.String)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("recursive dynamic write = %v/%v, want number", got, ok)
	}
	if !DynamicWriteNilDeletionAllowed(container, typ.String) {
		t.Fatal("recursive map contract lost deletion semantics")
	}

	loop := typ.NewRecursive("Loop", func(self typ.Type) typ.Type { return self })
	if got, ok := DynamicWriteValueType(loop, typ.String); ok || got != nil {
		t.Fatalf("cycle-only write contract = %v/%v, want failure", got, ok)
	}
}

func TestExpectedObjectLiteralRecordSelectsThroughRecursiveUnion(t *testing.T) {
	start := typetable.NewRecord().Field("kind", typ.LiteralString("start")).Build()
	stop := typetable.NewRecord().Field("kind", typ.LiteralString("stop")).Build()
	expected := typ.NewRecursivePlaceholder("Command")
	expected.SetBody(&typ.Union{Members: []typ.Type{expected, start, stop}})
	got, ok := ExpectedObjectLiteralRecord(expected, func(name string) (typ.Type, bool) {
		if name == "kind" {
			return typ.LiteralString("start"), true
		}
		return nil, false
	})
	if !ok || got != start {
		t.Fatalf("recursive object-literal selection = %v/%v, want start arm", got, ok)
	}
}
