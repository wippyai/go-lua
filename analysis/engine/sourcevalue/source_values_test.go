package sourcevalue

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	. "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestSourceValuesPanicsWithoutRegistry(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "SourceValuesConfig.Registry is required") {
			t.Fatal("NewSourceValues did not panic")
		}
	}()

	_ = NewSourceValues(SourceValuesConfig{})
}

func TestSourceValuesExpressionMapResolvesAndDoesNotMutateState(t *testing.T) {
	reg := product.DefaultRegistry()
	expr := ExprRef(10)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	value := presentValue(reg)
	values := map[ExprRef]product.Value{expr: value}
	resolver := NewSourceValues(SourceValuesConfig{
		Registry:         reg,
		ExpressionValues: values,
	})
	values[expr] = absentValue(reg)

	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(1)), absentValue(reg))
	wantState := in.Clone()

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, in, nil)
	if !ok {
		t.Fatal("expression source did not resolve")
	}
	if !product.Equal(reg, got, value) {
		t.Fatalf("expression value = %s, want %s", formatValue(reg, got), formatValue(reg, value))
	}
	assertStateEqual(t, reg, in, wantState)
}

func TestSourceValuesNilReturnsAbsentPresence(t *testing.T) {
	reg := product.DefaultRegistry()
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})

	got, ok := resolver.ValueOfSource(cfg.Point(2), ValueSource{Kind: ValueSourceNil}, state.State{}, nil)
	if !ok {
		t.Fatal("nil source did not resolve")
	}
	if !presence.Equal(product.PresenceOf(got), presence.Absent()) {
		t.Fatalf("nil source presence = %s, want absent", product.PresenceOf(got))
	}
}

func TestSourceValuesCallReadsReturnSlot(t *testing.T) {
	reg := product.DefaultRegistry()
	callPoint := cfg.Point(33)
	source := ValueSource{
		Kind:         ValueSourceCall,
		CallPoint:    callPoint,
		HasCallPoint: true,
		ResultIndex:  1,
	}
	value := presentValue(reg)
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	var readPoint cfg.Point

	got, ok := resolver.ValueOfSource(cfg.Point(3), source, state.State{}, func(point cfg.Point) state.State {
		readPoint = point
		return state.State{}.WriteReturnSlot(reg, 1, value)
	})
	if !ok {
		t.Fatal("call source did not resolve")
	}
	if readPoint != callPoint {
		t.Fatalf("read point = %d, want %d", readPoint, callPoint)
	}
	if !product.Equal(reg, got, value) {
		t.Fatalf("call source value = %s, want %s", formatValue(reg, got), formatValue(reg, value))
	}
}

func TestSourceValuesMissingMetadataAndUnknownSourcesReturnFalse(t *testing.T) {
	reg := product.DefaultRegistry()
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	read := func(cfg.Point) state.State { return state.State{}.WriteReturnSlot(reg, 0, presentValue(reg)) }

	cases := []struct {
		name   string
		source ValueSource
		read   func(cfg.Point) state.State
	}{
		{
			name:   "expression without expr ref",
			source: ValueSource{Kind: ValueSourceExpression},
		},
		{
			name:   "expression missing from table",
			source: ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(100), HasExpr: true},
		},
		{
			name:   "call without call point",
			source: ValueSource{Kind: ValueSourceCall, ResultIndex: 0},
			read:   read,
		},
		{
			name:   "call negative result index",
			source: ValueSource{Kind: ValueSourceCall, HasCallPoint: true, CallPoint: cfg.Point(4), ResultIndex: -1},
			read:   read,
		},
		{
			name:   "call nil read",
			source: ValueSource{Kind: ValueSourceCall, HasCallPoint: true, CallPoint: cfg.Point(4), ResultIndex: 0},
		},
		{
			name:   "unknown",
			source: ValueSource{Kind: ValueSourceUnknown},
		},
		{
			name:   "vararg without provider",
			source: ValueSource{Kind: ValueSourceVararg},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := resolver.ValueOfSource(cfg.Point(4), tc.source, state.State{}, tc.read); ok {
				t.Fatalf("source resolved to %s, want false", formatValue(reg, got))
			}
		})
	}
}

func TestSourceValuesExpressionAndVarargProvidersAreGenericHooks(t *testing.T) {
	reg := product.DefaultRegistry()
	exprValue := presentValue(reg)
	varargValue := absentValue(reg)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValue: func(point cfg.Point, expr ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5) || expr != ExprRef(55) || source.Kind != ValueSourceExpression {
				return product.Value{}, false
			}
			return exprValue, true
		},
		VarargValue: func(point cfg.Point, source ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
			if point != cfg.Point(5) || source.Kind != ValueSourceVararg {
				return product.Value{}, false
			}
			return varargValue, true
		},
	})

	gotExpr, ok := resolver.ValueOfSource(cfg.Point(5), ValueSource{
		Kind:    ValueSourceExpression,
		ExprRef: ExprRef(55),
		HasExpr: true,
	}, state.State{}, nil)
	if !ok || !product.Equal(reg, gotExpr, exprValue) {
		t.Fatalf("expression provider = %s/%v, want %s/true", formatValue(reg, gotExpr), ok, formatValue(reg, exprValue))
	}

	gotVararg, ok := resolver.ValueOfSource(cfg.Point(5), ValueSource{Kind: ValueSourceVararg}, state.State{}, nil)
	if !ok || !product.Equal(reg, gotVararg, varargValue) {
		t.Fatalf("vararg provider = %s/%v, want %s/true", formatValue(reg, gotVararg), ok, formatValue(reg, varargValue))
	}
}

func TestValueOverlaySourceValuesMeetsOverlayAndDoesNotMutateBase(t *testing.T) {
	reg := product.DefaultRegistry()
	inner := ExprRef(10)
	outer := ExprRef(11)
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true}
	outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	overlay := runtimeKindOverlay(reg, runtimekind.Singleton(runtimekind.Table))
	overlays := map[ExprRef]ValueOverlay{
		outer: NewValueOverlay(innerSource, overlay),
	}
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: base,
		},
	})
	resolver := WithValueOverlays(reg, baseResolver, overlays)

	got, ok := resolver.ValueOfSource(cfg.Point(1), outerSource, state.State{}, nil)
	if !ok {
		t.Fatal("value overlay source did not resolve through inner expression")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want original present", gotPresence)
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtimekind = %s, want table", gotKind)
	}
	if baseKind := product.Get(reg, base, runtimekind.Key); !runtimekind.Equal(baseKind, runtimekind.Top()) {
		t.Fatalf("base expression value mutated with runtime kind = %s", baseKind)
	}
}

func TestValueOverlaySourceValuesAppliesOverlayToCallSource(t *testing.T) {
	reg := product.DefaultRegistry()
	inner := ExprRef(12)
	outer := ExprRef(13)
	callPoint := cfg.Point(44)
	callValue := presentValue(reg)
	outerSource := ValueSource{
		Kind:         ValueSourceCall,
		ExprRef:      outer,
		HasExpr:      true,
		CallPoint:    callPoint,
		HasCallPoint: true,
		ResultIndex:  0,
	}
	innerSource := outerSource
	innerSource.ExprRef = inner
	baseResolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	resolver := WithValueOverlays(reg, baseResolver, map[ExprRef]ValueOverlay{
		outer: NewValueOverlay(innerSource, runtimeKindOverlay(reg, runtimekind.Singleton(runtimekind.Function))),
	})

	var readPoint cfg.Point
	got, ok := resolver.ValueOfSource(cfg.Point(1), outerSource, state.State{}, func(point cfg.Point) state.State {
		readPoint = point
		return state.State{}.WriteReturnSlot(reg, 0, callValue)
	})
	if !ok {
		t.Fatal("asserted call source did not resolve")
	}
	if readPoint != callPoint {
		t.Fatalf("read point = %d, want call point %d", readPoint, callPoint)
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("overlaid call presence = %s, want original present", gotPresence)
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Function)) {
		t.Fatalf("overlaid call runtime kind = %s, want function", gotKind)
	}
	if baseKind := product.Get(reg, callValue, runtimekind.Key); !runtimekind.Equal(baseKind, runtimekind.Top()) {
		t.Fatalf("call return value mutated with runtime kind = %s", baseKind)
	}
}

func TestValueOverlaySourceValuesPanicsWithoutRegistry(t *testing.T) {
	reg := product.DefaultRegistry()
	base := NewSourceValues(SourceValuesConfig{Registry: reg})

	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "value overlay source values require a registry") {
			t.Fatal("WithValueOverlays did not panic")
		}
	}()

	_ = WithValueOverlays(nil, base, map[ExprRef]ValueOverlay{
		ExprRef(1): NewValueOverlay(
			ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(2), HasExpr: true},
			runtimeKindOverlay(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})
}

func TestValueOverlaySourceValuesCanMeetCorePresenceOverlay(t *testing.T) {
	reg := product.DefaultRegistry()
	inner := ExprRef(14)
	outer := ExprRef(15)
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: absentValue(reg),
		},
	})
	resolver := WithValueOverlays(reg, baseResolver, map[ExprRef]ValueOverlay{
		outer: NewValueOverlay(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("presence overlay source did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Absent()) {
		t.Fatalf("presence overlay = %s, want absent", gotPresence)
	}
}

func TestValueOverlaySourceValuesNestedOverlaysMeet(t *testing.T) {
	reg := product.DefaultRegistry()
	inner := ExprRef(20)
	middle := ExprRef(21)
	outer := ExprRef(22)
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: presentValue(reg),
		},
	})
	resolver := WithValueOverlays(reg, baseResolver, map[ExprRef]ValueOverlay{
		middle: NewValueOverlay(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			runtimeKindOverlay(reg, runtimekind.Top().Without(runtimekind.Nil)),
		),
		outer: NewValueOverlay(
			ValueSource{Kind: ValueSourceExpression, ExprRef: middle, HasExpr: true},
			runtimeKindOverlay(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(2), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("nested overlay source did not resolve")
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("nested runtime kind = %s, want table", gotKind)
	}
}

func TestValueOverlaySourceValuesMissingInnerSourceReturnsFalse(t *testing.T) {
	reg := product.DefaultRegistry()
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
	})
	resolver := WithValueOverlays(reg, baseResolver, map[ExprRef]ValueOverlay{
		ExprRef(30): NewValueOverlay(
			ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(31), HasExpr: true},
			runtimeKindOverlay(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})

	if got, ok := resolver.ValueOfSource(cfg.Point(3), ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(30), HasExpr: true}, state.State{}, nil); ok {
		t.Fatalf("missing overlay inner source resolved to %s, want false", formatValue(reg, got))
	}
}

func runtimeKindOverlay(reg *axis.Registry, value runtimekind.Value) product.Value {
	return product.Set(reg, product.Top(), runtimekind.Key, value)
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	default:
		return product.PresenceOf(v).String()
	}
}
