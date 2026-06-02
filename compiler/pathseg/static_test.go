package pathseg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestStaticTableFieldKeySegment_Identifier(t *testing.T) {
	seg, ok := StaticTableFieldKeySegment(&ast.IdentExpr{Value: "name"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentField, Name: "name"}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticTableFieldSegmentWithSyntaxDistinguishesNameAndBracketString(t *testing.T) {
	dot, ok := StaticTableFieldSegment(&ast.Field{
		Key:       &ast.StringExpr{Value: "name"},
		KeySyntax: ast.AttrKeyDot,
	})
	if !ok {
		t.Fatal("expected dot field segment")
	}
	if dot != (constraint.Segment{Kind: constraint.SegmentField, Name: "name"}) {
		t.Fatalf("dot table field segment = %+v", dot)
	}

	index, ok := StaticTableFieldSegment(&ast.Field{
		Key:       &ast.StringExpr{Value: "name"},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok {
		t.Fatal("expected bracket string-index segment")
	}
	if index != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"}) {
		t.Fatalf("bracket table field segment = %+v", index)
	}

	legacy, ok := StaticTableFieldSegment(&ast.Field{Key: &ast.StringExpr{Value: "name"}})
	if !ok {
		t.Fatal("expected legacy static segment")
	}
	if legacy != dot {
		t.Fatalf("legacy table field segment = %+v, want dot-compatible %+v", legacy, dot)
	}
}

func TestStaticTableFieldSegmentWithConstResolvesBracketIdentAsIndex(t *testing.T) {
	resolver := func(name string) *flow.ConstValue {
		if name == "k" {
			return &flow.ConstValue{Kind: flow.ConstString, Str: "name"}
		}
		return nil
	}

	seg, ok := StaticTableFieldSegmentWithConst(&ast.Field{
		Key:       &ast.IdentExpr{Value: "k"},
		KeySyntax: ast.AttrKeyIndex,
	}, resolver)
	if !ok {
		t.Fatal("expected const bracket key segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"}) {
		t.Fatalf("const bracket key segment = %+v, want string index name", seg)
	}

	legacy, ok := StaticTableFieldSegmentWithConst(&ast.Field{
		Key: &ast.IdentExpr{Value: "k"},
	}, resolver)
	if !ok {
		t.Fatal("expected legacy identifier field segment")
	}
	if legacy != (constraint.Segment{Kind: constraint.SegmentField, Name: "k"}) {
		t.Fatalf("legacy identifier segment = %+v, want field k", legacy)
	}
}

func TestTableFieldMatchesSegmentUsesSyntaxCarrier(t *testing.T) {
	field := &ast.Field{
		Key:       &ast.StringExpr{Value: "name"},
		KeySyntax: ast.AttrKeyIndex,
	}
	if !TableFieldMatchesSegment(field, constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"}) {
		t.Fatal("expected bracket string field to match string-index segment")
	}
	if TableFieldMatchesSegment(field, constraint.Segment{Kind: constraint.SegmentField, Name: "name"}) {
		t.Fatal("expected bracket string field not to match dot-field segment")
	}
}

func TestStaticAttrKeySegment_StringNonIdentifier(t *testing.T) {
	seg, ok := StaticAttrKeySegment(&ast.StringExpr{Value: "x-y"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegmentWithSyntaxDistinguishesDotAndBracket(t *testing.T) {
	dot, ok := StaticAttrKeySegmentWithSyntax(&ast.StringExpr{Value: "field"}, ast.AttrKeyDot)
	if !ok {
		t.Fatal("expected dot field segment")
	}
	if dot != (constraint.Segment{Kind: constraint.SegmentField, Name: "field"}) {
		t.Fatalf("dot segment = %+v", dot)
	}

	index, ok := StaticAttrKeySegmentWithSyntax(&ast.StringExpr{Value: "field"}, ast.AttrKeyIndex)
	if !ok {
		t.Fatal("expected bracket string-index segment")
	}
	if index != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "field"}) {
		t.Fatalf("index segment = %+v", index)
	}

	legacy, ok := StaticAttrKeySegmentWithSyntax(&ast.StringExpr{Value: "field"}, ast.AttrKeyUnknown)
	if !ok {
		t.Fatal("expected legacy segment")
	}
	if legacy != dot {
		t.Fatalf("legacy segment = %+v, want dot-compatible %+v", legacy, dot)
	}
}

func TestStaticAttrKeySegment_EmptyString(t *testing.T) {
	seg, ok := StaticAttrKeySegment(&ast.StringExpr{Value: ""})
	if !ok {
		t.Fatal("expected empty string to be a static index segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: ""}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticTableFieldKeySegment_EmptyString(t *testing.T) {
	seg, ok := StaticTableFieldKeySegment(&ast.StringExpr{Value: ""})
	if !ok {
		t.Fatal("expected empty string table key to be a static index segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: ""}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegment_Number(t *testing.T) {
	seg, ok := StaticAttrKeySegment(&ast.NumberExpr{Value: "7"})
	if !ok {
		t.Fatal("expected static segment")
	}
	if seg != (constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 7}) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
}

func TestStaticAttrKeySegment_RejectsDynamicIdent(t *testing.T) {
	if _, ok := StaticAttrKeySegment(&ast.IdentExpr{Value: "k"}); ok {
		t.Fatal("expected dynamic identifier key to be rejected for attr access")
	}
}

func TestStaticAttrKeySegment_InvalidNumber(t *testing.T) {
	if _, ok := StaticAttrKeySegment(&ast.NumberExpr{Value: "1.5"}); ok {
		t.Fatal("expected invalid numeric index to be rejected")
	}
}
