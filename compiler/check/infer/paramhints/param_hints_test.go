package paramhints

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenParamHintType_Nil(t *testing.T) {
	result := WidenParamHintType(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestWidenParamHintType_BooleanLiteral(t *testing.T) {
	lit := typ.LiteralBool(true)
	result := WidenParamHintType(lit)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestWidenParamHintType_IntegerLiteral(t *testing.T) {
	lit := typ.LiteralInt(42)
	result := WidenParamHintType(lit)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestWidenParamHintType_NumberLiteral(t *testing.T) {
	lit := typ.LiteralNumber(3.14)
	result := WidenParamHintType(lit)
	if result != typ.Number {
		t.Errorf("expected Number, got %v", result)
	}
}

func TestWidenParamHintType_StringLiteral(t *testing.T) {
	lit := typ.LiteralString("hello")
	result := WidenParamHintType(lit)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestWidenParamHintType_NonLiteral(t *testing.T) {
	result := WidenParamHintType(typ.String)
	if result != typ.String {
		t.Errorf("expected String unchanged, got %v", result)
	}
}

func TestWidenParamHintType_Optional(t *testing.T) {
	lit := typ.LiteralString("hello")
	opt := typ.NewOptional(lit)
	result := WidenParamHintType(opt)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	optResult, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if optResult.Inner != typ.String {
		t.Errorf("expected inner to be String, got %v", optResult.Inner)
	}
}

func TestWidenParamHintType_Union(t *testing.T) {
	lit1 := typ.LiteralString("a")
	lit2 := typ.LiteralNumber(1.0)
	union := typ.NewUnion(lit1, lit2)
	result := WidenParamHintType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBuildParamHintSigView_NilInputs(t *testing.T) {
	result := BuildParamHintSigView(nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil inputs, got %v", result)
	}
}

func TestIsInformativeHintType(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: typ.Any, want: false},
		{name: "unknown", in: typ.Unknown, want: false},
		{name: "never", in: typ.Never, want: false},
		{name: "nil type", in: typ.Nil, want: false},
		{name: "empty record", in: typ.NewRecord().Build(), want: false},
		{name: "map with string key", in: typ.NewMap(typ.String, typ.NewArray(typ.Any)), want: true},
		{name: "record map component", in: typ.NewRecord().MapComponent(typ.String, typ.Any).Build(), want: true},
		{name: "string", in: typ.String, want: true},
		{name: "literal", in: typ.LiteralString("x"), want: true},
	}

	for _, tt := range tests {
		if got := IsInformativeHintType(tt.in); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
