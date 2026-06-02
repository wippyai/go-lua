package fieldkey

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestSortedUsesStructuralOrder(t *testing.T) {
	fields := map[Key]int{
		{Kind: constraint.SegmentIndexString, Name: "a"}: 1,
		{Kind: constraint.SegmentField, Name: "b"}:       2,
		{Kind: constraint.SegmentField, Name: "a"}:       3,
		{Kind: constraint.SegmentIndexInt, Index: 1}:     4,
	}

	keys := Sorted(fields)
	want := []Key{
		{Kind: constraint.SegmentField, Name: "a"},
		{Kind: constraint.SegmentField, Name: "b"},
		{Kind: constraint.SegmentIndexString, Name: "a"},
		{Kind: constraint.SegmentIndexInt, Index: 1},
	}
	if len(keys) != len(want) {
		t.Fatalf("Sorted len = %d, want %d", len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("Sorted[%d] = %#v, want %#v", i, keys[i], want[i])
		}
	}
}

func TestFromSegmentKeepsStaticIndexesStructural(t *testing.T) {
	key, ok := FromSegment(constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"})
	if !ok {
		t.Fatal("FromSegment rejected static string index")
	}
	if key.Kind != constraint.SegmentIndexString || key.Name != "x-y" {
		t.Fatalf("FromSegment = %#v, want string-index x-y", key)
	}
	key, ok = FromSegment(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1})
	if !ok {
		t.Fatal("FromSegment rejected static int index")
	}
	if key.Kind != constraint.SegmentIndexInt || key.Index != 1 {
		t.Fatalf("FromSegment = %#v, want int-index 1", key)
	}
	key, ok = FromSegment(constraint.Segment{Kind: constraint.SegmentIndexString, Name: ""})
	if !ok {
		t.Fatal("FromSegment rejected empty static string index")
	}
	if key.Kind != constraint.SegmentIndexString || key.Name != "" {
		t.Fatalf("FromSegment = %#v, want empty string-index", key)
	}
}

func TestStringKeyFromTableFieldUsesRuntimeLuaKeySemantics(t *testing.T) {
	name, ok := StringKeyFromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "handler"},
		KeySyntax: ast.AttrKeyDot,
	})
	if !ok || name != "handler" {
		t.Fatalf("StringKeyFromTableField(dot) = %q,%v, want handler,true", name, ok)
	}

	name, ok = StringKeyFromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "handler"},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok || name != "handler" {
		t.Fatalf("StringKeyFromTableField(bracket string) = %q,%v, want handler,true", name, ok)
	}

	name, ok = StringKeyFromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: ""},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok || name != "" {
		t.Fatalf("StringKeyFromTableField(empty bracket string) = %q,%v, want empty,true", name, ok)
	}

	if name, ok := StringKeyFromTableField(&ast.Field{
		Key:       &ast.NumberExpr{Value: "1"},
		KeySyntax: ast.AttrKeyIndex,
	}); ok || name != "" {
		t.Fatalf("StringKeyFromTableField(int index) = %q,%v, want empty,false", name, ok)
	}
}

func TestFromTableFieldUsesSyntaxCarrier(t *testing.T) {
	field, ok := FromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "name"},
		KeySyntax: ast.AttrKeyDot,
	})
	if !ok {
		t.Fatal("FromTableField rejected dot field")
	}
	if field != (Key{Kind: constraint.SegmentField, Name: "name"}) {
		t.Fatalf("dot field key = %#v, want field name", field)
	}

	index, ok := FromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "name"},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok {
		t.Fatal("FromTableField rejected bracket string field")
	}
	if index != (Key{Kind: constraint.SegmentIndexString, Name: "name"}) {
		t.Fatalf("bracket field key = %#v, want string-index name", index)
	}

	intIndex, ok := FromTableField(&ast.Field{
		Key:       &ast.NumberExpr{Value: "1"},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok {
		t.Fatal("FromTableField rejected bracket int field")
	}
	if intIndex != (Key{Kind: constraint.SegmentIndexInt, Index: 1}) {
		t.Fatalf("bracket int key = %#v, want int-index 1", intIndex)
	}

	if _, ok := FromTableField(&ast.Field{
		Key:       &ast.IdentExpr{Value: "dynamic"},
		KeySyntax: ast.AttrKeyIndex,
	}); ok {
		t.Fatal("FromTableField accepted unresolved bracket identifier")
	}
}

func TestFromTableFieldWithConstResolvesBracketIdentifier(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "k" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "name"}
		}
		return nil
	}

	key, ok := FromTableFieldWithConst(&ast.Field{
		Key:       &ast.IdentExpr{Value: "k"},
		KeySyntax: ast.AttrKeyIndex,
	}, resolver)
	if !ok {
		t.Fatal("FromTableFieldWithConst rejected const bracket identifier")
	}
	if key != (Key{Kind: constraint.SegmentIndexString, Name: "name"}) {
		t.Fatalf("const bracket key = %#v, want string-index name", key)
	}
}

func TestRecordFieldNameFromTableFieldKeepsRecordSyntaxStrict(t *testing.T) {
	name, ok := RecordFieldNameFromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "handler"},
		KeySyntax: ast.AttrKeyDot,
	})
	if !ok || name != "handler" {
		t.Fatalf("RecordFieldNameFromTableField(dot) = %q,%v, want handler,true", name, ok)
	}

	if name, ok := RecordFieldNameFromTableField(&ast.Field{
		Key:       &ast.StringExpr{Value: "handler"},
		KeySyntax: ast.AttrKeyIndex,
	}); ok || name != "" {
		t.Fatalf("RecordFieldNameFromTableField(bracket string) = %q,%v, want empty,false", name, ok)
	}

	if name, ok := RecordFieldNameFromTableField(&ast.Field{
		Key:       &ast.IdentExpr{Value: "k"},
		KeySyntax: ast.AttrKeyIndex,
	}); ok || name != "" {
		t.Fatalf("RecordFieldNameFromTableField(dynamic bracket) = %q,%v, want empty,false", name, ok)
	}
}
