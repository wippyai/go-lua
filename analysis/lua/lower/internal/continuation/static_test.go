package continuation

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"testing"
)

func TestTypeSpanOwnsStaticQueueCoordinateValidation(t *testing.T) {
	span := programsource.Span{File: "type.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	typ := &ast.PrimitiveTypeExpr{Name: "number"}
	typ.SetLine(1)
	typ.SetColumn(1)
	typ.SetLastLine(1)
	typ.SetLastColumn(2)
	got, ok := TypeSpan(typ, span.File)
	if !ok || got != span {
		t.Fatalf("TypeSpan = %#v/%v, want %#v/true", got, ok, span)
	}
	queue := NewStatics(&Stack{})
	if err := queue.PushType(typ, keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 1), span); err != nil {
		t.Fatal(err)
	}
	if request, err := queue.PopType(); err != nil || request.Host == 0 || queue.Clean() != true {
		t.Fatalf("PopType/Clean = %#v/%v/%v", request, err, queue.Clean())
	}
}
