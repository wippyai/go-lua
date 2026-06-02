package assign

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

type preflowStaticReadOps struct {
	fieldName string
	indexKey  typ.Type
}

func (o *preflowStaticReadOps) Field(_ *db.QueryContext, _ typ.Type, name string) (typ.Type, bool) {
	o.fieldName = name
	return typ.String, true
}

func (o *preflowStaticReadOps) Index(_ *db.QueryContext, _ typ.Type, key typ.Type) (typ.Type, bool) {
	o.indexKey = key
	return typ.Number, true
}

func (*preflowStaticReadOps) Method(*db.QueryContext, typ.Type, string) (typ.Type, bool) {
	return nil, false
}

func (*preflowStaticReadOps) BinaryOp(*db.QueryContext, typ.Type, string, typ.Type) typ.Type {
	return nil
}

func (*preflowStaticReadOps) UnaryOp(*db.QueryContext, string, typ.Type) typ.Type {
	return nil
}

func (*preflowStaticReadOps) IsSubtype(*db.QueryContext, typ.Type, typ.Type) bool {
	return false
}

func (*preflowStaticReadOps) ExpandInstantiated(*db.QueryContext, typ.Type) typ.Type {
	return nil
}

func (*preflowStaticReadOps) Widen(*db.QueryContext, typ.Type) typ.Type {
	return nil
}

func (*preflowStaticReadOps) WidenForInference(*db.QueryContext, typ.Type) typ.Type {
	return nil
}

func TestStaticAttrReadTypeDispatchesBySegmentKind(t *testing.T) {
	base := typ.NewRecord().Build()

	fieldOps := &preflowStaticReadOps{}
	got, ok := staticAttrReadType(nil, fieldOps, base, constraint.Segment{Kind: constraint.SegmentField, Name: "name"})
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("field static read = %v/%v, want string/true", got, ok)
	}
	if fieldOps.fieldName != "name" || fieldOps.indexKey != nil {
		t.Fatalf("field static read called field=%q index=%v", fieldOps.fieldName, fieldOps.indexKey)
	}

	stringIndexOps := &preflowStaticReadOps{}
	got, ok = staticAttrReadType(nil, stringIndexOps, base, constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"})
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("string-index static read = %v/%v, want number/true", got, ok)
	}
	if stringIndexOps.fieldName != "" || !typ.TypeEquals(stringIndexOps.indexKey, typ.LiteralString("x-y")) {
		t.Fatalf("string-index static read called field=%q index=%v", stringIndexOps.fieldName, stringIndexOps.indexKey)
	}

	intIndexOps := &preflowStaticReadOps{}
	got, ok = staticAttrReadType(nil, intIndexOps, base, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 2})
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("int-index static read = %v/%v, want number/true", got, ok)
	}
	if intIndexOps.fieldName != "" || !typ.TypeEquals(intIndexOps.indexKey, typ.LiteralInt(2)) {
		t.Fatalf("int-index static read called field=%q index=%v", intIndexOps.fieldName, intIndexOps.indexKey)
	}
}
