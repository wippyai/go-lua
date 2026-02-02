package constprop_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	"github.com/wippyai/go-lua/types/flow"
)

func TestConstValueFromExpr_String(t *testing.T) {
	expr := &ast.StringExpr{Value: "hello"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(string) returned nil")
	}
	if val.Kind != flow.ConstString {
		t.Errorf("Kind = %v, want ConstString", val.Kind)
	}
	if val.Str != "hello" {
		t.Errorf("Str = %q, want %q", val.Str, "hello")
	}
}

func TestConstValueFromExpr_Number(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(number) returned nil")
	}
	if val.Kind != flow.ConstInt {
		t.Errorf("Kind = %v, want ConstInt", val.Kind)
	}
	if val.Int != 42 {
		t.Errorf("Int = %d, want %d", val.Int, 42)
	}
}

func TestConstValueFromExpr_Float(t *testing.T) {
	expr := &ast.NumberExpr{Value: "3.14"}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(float) returned nil")
	}
	if val.Kind != flow.ConstFloat {
		t.Errorf("Kind = %v, want ConstFloat", val.Kind)
	}
	if val.Float != 3.14 {
		t.Errorf("Float = %f, want %f", val.Float, 3.14)
	}
}

func TestConstValueFromExpr_Nil(t *testing.T) {
	val := constprop.ConstValueFromExpr(nil)
	if val != nil {
		t.Errorf("constprop.ConstValueFromExpr(nil) = %v, want nil", val)
	}
}

func TestConstValueFromExpr_True(t *testing.T) {
	expr := &ast.TrueExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(true) returned nil")
	}
	if val.Kind != flow.ConstBool {
		t.Errorf("Kind = %v, want ConstBool", val.Kind)
	}
	if !val.Bool {
		t.Errorf("Bool = %v, want true", val.Bool)
	}
}

func TestConstValueFromExpr_False(t *testing.T) {
	expr := &ast.FalseExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(false) returned nil")
	}
	if val.Kind != flow.ConstBool {
		t.Errorf("Kind = %v, want ConstBool", val.Kind)
	}
	if val.Bool {
		t.Errorf("Bool = %v, want false", val.Bool)
	}
}

func TestConstValueFromExpr_NilExpr(t *testing.T) {
	expr := &ast.NilExpr{}
	val := constprop.ConstValueFromExpr(expr)
	if val == nil {
		t.Fatal("constprop.ConstValueFromExpr(nil expr) returned nil")
	}
	if val.Kind != flow.ConstNil {
		t.Errorf("Kind = %v, want ConstNil", val.Kind)
	}
}

func TestConstValueFromExpr_Other(t *testing.T) {
	expr := &ast.IdentExpr{Value: "foo"}
	val := constprop.ConstValueFromExpr(expr)
	if val != nil {
		t.Errorf("constprop.ConstValueFromExpr(ident) = %v, want nil", val)
	}
}

func TestParseIntLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"42", 42, true},
		{"0", 0, true},
		{"-1", -1, true},
		{"123456", 123456, true},
		{"9223372036854775807", 9223372036854775807, true},
		{"3.14", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, ok := constprop.ParseIntLiteral(tt.input)
		if ok != tt.ok {
			t.Errorf("constprop.ParseIntLiteral(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("constprop.ParseIntLiteral(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseFloatLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"3.14", 3.14, true},
		{"0.0", 0.0, true},
		{"-1.5", -1.5, true},
		{"1e10", 1e10, true},
		{"42", 42.0, true},
		{"abc", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, ok := constprop.ParseFloatLiteral(tt.input)
		if ok != tt.ok {
			t.Errorf("constprop.ParseFloatLiteral(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("constprop.ParseFloatLiteral(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestConstValueEqual_Float(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstFloat, Float: 3.14}
	b := &flow.ConstValue{Kind: flow.ConstFloat, Float: 3.14}
	c := &flow.ConstValue{Kind: flow.ConstFloat, Float: 2.71}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(3.14, 3.14) = false, want true")
	}
	if constprop.ConstValueEqual(a, c) {
		t.Error("constprop.ConstValueEqual(3.14, 2.71) = true, want false")
	}
}

func TestConstValueEqual_Bool(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
	b := &flow.ConstValue{Kind: flow.ConstBool, Bool: true}
	c := &flow.ConstValue{Kind: flow.ConstBool, Bool: false}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(true, true) = false, want true")
	}
	if constprop.ConstValueEqual(a, c) {
		t.Error("constprop.ConstValueEqual(true, false) = true, want false")
	}
}

func TestConstValueEqual_Nil(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstNil}
	b := &flow.ConstValue{Kind: flow.ConstNil}

	if !constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(nil, nil) = false, want true")
	}
}

func TestConstValueEqual_DifferentKinds(t *testing.T) {
	a := &flow.ConstValue{Kind: flow.ConstInt, Int: 42}
	b := &flow.ConstValue{Kind: flow.ConstFloat, Float: 42.0}

	if constprop.ConstValueEqual(a, b) {
		t.Error("constprop.ConstValueEqual(int 42, float 42.0) = true, want false")
	}
}
