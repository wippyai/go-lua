package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestValueSourceShapeValidation(t *testing.T) {
	tests := []struct {
		name     string
		final    bool
		expanded bool
		adjusted bool
		openTail bool
		want     bool
	}{
		{name: "plain", want: true},
		{name: "adjusted", adjusted: true, want: true},
		{name: "expanded final", final: true, expanded: true, want: true},
		{name: "open tail", final: true, expanded: true, openTail: true, want: true},
		{name: "expanded adjusted", final: true, expanded: true, adjusted: true},
		{name: "expanded non-final", expanded: true},
		{name: "open tail without expanded", final: true, openTail: true},
		{name: "open tail non-final", expanded: true, openTail: true},
	}

	for _, test := range tests {
		shape, ok := NewValueSourceShape(test.final, test.expanded, test.adjusted, test.openTail)
		if ok != test.want {
			t.Fatalf("%s shape valid = %v, want %v", test.name, ok, test.want)
		}
		if shape.Valid() != test.want {
			t.Fatalf("%s shape.Valid() = %v, want %v", test.name, shape.Valid(), test.want)
		}
	}
}

func TestValueSourceConstructors(t *testing.T) {
	adjusted, ok := NewValueSourceShape(true, false, true, false)
	if !ok {
		t.Fatalf("adjusted shape rejected")
	}
	expr, ok := NewExpressionValueSource(ExprRef(1), 0, NoValueSourceIndex, 0, adjusted)
	if !ok || !expr.Valid() {
		t.Fatalf("expression source = %#v/%v, want valid", expr, ok)
	}
	if expr.Kind != ValueSourceExpression || expr.ExprRef != ExprRef(1) || !expr.HasExpr || expr.TargetIndex != NoValueSourceIndex || !expr.Final || !expr.Adjusted {
		t.Fatalf("expression source fields = %#v", expr)
	}

	expanded, ok := NewValueSourceShape(true, true, false, true)
	if !ok {
		t.Fatalf("expanded shape rejected")
	}
	call, ok := NewCallValueSource(ExprRef(2), 1, 2, 3, cfg.Point(10), expanded)
	if !ok || !call.Valid() {
		t.Fatalf("call source = %#v/%v, want valid", call, ok)
	}
	if call.Kind != ValueSourceCall || call.ExprRef != ExprRef(2) || !call.HasExpr || call.CallPoint != cfg.Point(10) || !call.HasCallPoint || call.ResultIndex != 3 || !call.OpenTail {
		t.Fatalf("call source fields = %#v", call)
	}

	callWithoutExpr, ok := NewCallValueSource(0, NoValueSourceIndex, NoValueSourceIndex, 0, cfg.Point(11), ValueSourceShape{})
	if !ok || !callWithoutExpr.Valid() {
		t.Fatalf("call source without expr = %#v/%v, want valid", callWithoutExpr, ok)
	}
	if callWithoutExpr.HasExpr || callWithoutExpr.ExprRef != 0 || callWithoutExpr.TargetIndex != NoValueSourceIndex {
		t.Fatalf("call source without expr fields = %#v", callWithoutExpr)
	}

	vararg, ok := NewVarargValueSource(ExprRef(3), 0, NoValueSourceIndex, 0, expanded)
	if !ok || !vararg.Valid() {
		t.Fatalf("vararg source = %#v/%v, want valid", vararg, ok)
	}
	if vararg.Kind != ValueSourceVararg || vararg.ExprRef != ExprRef(3) || !vararg.HasExpr || !vararg.OpenTail {
		t.Fatalf("vararg source fields = %#v", vararg)
	}

	varargWithoutExpr, ok := NewVarargValueSource(0, NoValueSourceIndex, NoValueSourceIndex, 0, ValueSourceShape{})
	if !ok || !varargWithoutExpr.Valid() {
		t.Fatalf("vararg source without expr = %#v/%v, want valid", varargWithoutExpr, ok)
	}
	if varargWithoutExpr.HasExpr || varargWithoutExpr.ExprRef != 0 || varargWithoutExpr.TargetIndex != NoValueSourceIndex {
		t.Fatalf("vararg source without expr fields = %#v", varargWithoutExpr)
	}

	nilSource := NewNilValueSource(4)
	if !nilSource.Valid() {
		t.Fatalf("nil source = %#v, want valid", nilSource)
	}
	if nilSource.Kind != ValueSourceNil || nilSource.ExprIndex != NoValueSourceIndex || nilSource.TargetIndex != 4 || nilSource.ResultIndex != NoValueSourceIndex || nilSource.HasExpr || nilSource.HasCallPoint {
		t.Fatalf("nil source fields = %#v", nilSource)
	}

	unknown := NewUnknownValueSource(NoValueSourceIndex)
	if !unknown.Valid() {
		t.Fatalf("unknown source = %#v, want valid", unknown)
	}
	if unknown.Kind != ValueSourceUnknown || unknown.ExprIndex != NoValueSourceIndex || unknown.TargetIndex != NoValueSourceIndex || unknown.ResultIndex != NoValueSourceIndex {
		t.Fatalf("unknown source fields = %#v", unknown)
	}
}

func TestValueSourceConstructorsRejectInvalidSources(t *testing.T) {
	validShape, ok := NewValueSourceShape(false, false, false, false)
	if !ok {
		t.Fatalf("plain shape rejected")
	}
	if _, ok := NewExpressionValueSource(0, 0, 0, 0, validShape); ok {
		t.Fatalf("expression source without expr ref accepted")
	}
	if _, ok := NewCallValueSource(0, 0, 0, 0, 0, validShape); ok {
		t.Fatalf("call source without call point accepted")
	}
	if _, ok := NewCallValueSource(0, 0, 0, NoValueSourceIndex, cfg.Point(1), validShape); ok {
		t.Fatalf("call source without result index accepted")
	}
	if _, ok := NewExpressionValueSource(ExprRef(1), 0, 0, 0, ValueSourceShape{Final: true, Expanded: true, Adjusted: true}); ok {
		t.Fatalf("source with expanded and adjusted shape accepted")
	}
}

func TestValueSourceValidRejectsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name   string
		source ValueSource
	}{
		{
			name: "expression without expr ref",
			source: ValueSource{
				Kind:        ValueSourceExpression,
				ExprIndex:   0,
				TargetIndex: 0,
				ResultIndex: 0,
			},
		},
		{
			name: "expression with call point",
			source: ValueSource{
				Kind:         ValueSourceExpression,
				ExprRef:      ExprRef(1),
				HasExpr:      true,
				ExprIndex:    0,
				TargetIndex:  0,
				ResultIndex:  0,
				CallPoint:    cfg.Point(1),
				HasCallPoint: true,
			},
		},
		{
			name: "call without call point",
			source: ValueSource{
				Kind:        ValueSourceCall,
				ResultIndex: 0,
			},
		},
		{
			name: "call without result index",
			source: ValueSource{
				Kind:         ValueSourceCall,
				ResultIndex:  NoValueSourceIndex,
				CallPoint:    cfg.Point(1),
				HasCallPoint: true,
			},
		},
		{
			name: "call with mismatched expr flags",
			source: ValueSource{
				Kind:         ValueSourceCall,
				ExprRef:      ExprRef(1),
				ResultIndex:  0,
				CallPoint:    cfg.Point(1),
				HasCallPoint: true,
			},
		},
		{
			name: "vararg with call point",
			source: ValueSource{
				Kind:         ValueSourceVararg,
				CallPoint:    cfg.Point(1),
				HasCallPoint: true,
			},
		},
		{
			name: "expanded adjusted",
			source: ValueSource{
				Kind:        ValueSourceExpression,
				ExprRef:     ExprRef(1),
				HasExpr:     true,
				ExprIndex:   0,
				TargetIndex: 0,
				ResultIndex: 0,
				Final:       true,
				Expanded:    true,
				Adjusted:    true,
			},
		},
		{
			name: "expanded non-final",
			source: ValueSource{
				Kind:        ValueSourceExpression,
				ExprRef:     ExprRef(1),
				HasExpr:     true,
				ExprIndex:   0,
				TargetIndex: 0,
				ResultIndex: 0,
				Expanded:    true,
			},
		},
		{
			name: "open tail without expanded",
			source: ValueSource{
				Kind:        ValueSourceExpression,
				ExprRef:     ExprRef(1),
				HasExpr:     true,
				ExprIndex:   0,
				TargetIndex: 0,
				ResultIndex: 0,
				Final:       true,
				OpenTail:    true,
			},
		},
		{
			name: "nil with result index",
			source: ValueSource{
				Kind:        ValueSourceNil,
				ExprIndex:   NoValueSourceIndex,
				ResultIndex: 0,
			},
		},
		{
			name: "nil with expr ref",
			source: ValueSource{
				Kind:        ValueSourceNil,
				ExprRef:     ExprRef(1),
				HasExpr:     true,
				ExprIndex:   NoValueSourceIndex,
				ResultIndex: NoValueSourceIndex,
			},
		},
	}

	for _, test := range tests {
		if test.source.Valid() {
			t.Fatalf("%s source valid: %#v", test.name, test.source)
		}
	}
}
