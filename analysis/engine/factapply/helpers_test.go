package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type sourceValueCall struct {
	point  cfg.Point
	source factflow.ValueSource
}

type recordingSourceValues struct {
	values map[factflow.ValueSource]product.Value
	calls  []sourceValueCall
}

func (r *recordingSourceValues) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if read == nil {
		panic("nil read function")
	}
	_ = read(point)
	r.calls = append(r.calls, sourceValueCall{point: point, source: source})
	value, ok := r.values[source]
	return value, ok
}

type panicSourceValues struct{}

func (panicSourceValues) ValueOfSource(
	cfg.Point,
	factflow.ValueSource,
	state.State,
	func(cfg.Point) state.State,
) (product.Value, bool) {
	panic("ValueOfSource should not be called")
}

func assertResolverCall(t *testing.T, resolver *recordingSourceValues, point cfg.Point, source factflow.ValueSource) {
	t.Helper()
	if len(resolver.calls) != 1 {
		t.Fatalf("resolver calls = %d, want 1", len(resolver.calls))
	}
	if got := resolver.calls[0]; got.point != point || got.source != source {
		t.Fatalf("resolver call = %#v, want point %d source %#v", got, point, source)
	}
}

func assertValue(t *testing.T, reg *axis.Registry, gotState state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := gotState.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("slot %s = %s, want %s", slot, formatValue(reg, got), formatValue(reg, want))
	}
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

func assertPathValue(t *testing.T, reg *axis.Registry, gotState state.State, pathKey path.PathKey, want product.Value) {
	t.Helper()
	if got := gotState.ReadPathKey(reg, pathKey); !product.Equal(reg, got, want) {
		t.Fatalf("path %s = %s, want %s", pathKey, formatValue(reg, got), formatValue(reg, want))
	}
}

func assertPathPresence(t *testing.T, reg *axis.Registry, gotState state.State, pathKey path.PathKey, want presence.Value) {
	t.Helper()
	got := gotState.ReadPathKey(reg, pathKey)
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("path %s = bottom, want presence %s", pathKey, want)
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("path %s presence = %s in %s, want %s", pathKey, gotPresence, formatValue(reg, got), want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtime kind = %s in %s, want %s", kind, formatValue(reg, got), want)
	}
}

func assertVariantOriginType(t *testing.T, reg *axis.Registry, gotState state.State, slot symbol.ID, base typ.Type, want typ.Type) {
	t.Helper()
	value := gotState.ReadValue(reg, key.SymbolValue(slot))
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("variant origin = %v, want concrete in %s", origin, formatValue(reg, value))
	}
	got, ok := variant.NarrowByOrigin(base, origin.Family(), origin.Cases())
	if !ok {
		t.Fatalf("origin family %d cases %v did not narrow %s", origin.Family(), origin.Cases(), base)
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("origin narrowed type = %s, want %s", got, want)
	}
}

func testLuaPathTypeProjector(root typ.Type, p path.Path) (typ.Type, bool) {
	return typeprojection.ApplySegments(root, p.Segments)
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func runtimeKindConstraint(value runtimekind.Value) product.Value {
	return product.Set(standard.Registry(), product.Top(), runtimekind.Key, value)
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

type widening uint8

const (
	wideningBottom   widening = 0
	wideningOne      widening = 1
	wideningExactMax widening = 4
	wideningTop      widening = 100
)

var wideningKey = axis.NewKey[widening]("factflow.test.widening")

func wideningRegistry() *axis.Registry {
	reg := axis.NewRegistry()
	axis.Register(reg, axis.Spec[widening]{
		Key:    wideningKey,
		Bottom: func() widening { return wideningBottom },
		Top:    func() widening { return wideningTop },
		Equal:  func(a, b widening) bool { return a == b },
		LessOrEq: func(a, b widening) bool {
			return a == b || a != wideningTop && b == wideningTop || a < b && b != wideningTop
		},
		Join: func(a, b widening) widening {
			if a == wideningTop || b == wideningTop {
				return wideningTop
			}
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b widening) widening {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next widening) widening {
			if prev == next {
				return prev
			}
			return wideningTop
		},
		Hash: func(v widening) uint64 {
			return uint64(v) + 1
		},
	})
	return reg.Freeze()
}

func wideningValue(reg *axis.Registry, value widening) product.Value {
	return product.Set(reg, product.Top(), wideningKey, value)
}

type testSparseAxis uint8

const (
	testSparseAxisBottom testSparseAxis = iota
	testSparseAxisLow
	testSparseAxisHigh
	testSparseAxisTop
)

var testSparseAxisKey = axis.NewKey[testSparseAxis]("factflow.test.sparse")

func testSparseAxisSpec() axis.Spec[testSparseAxis] {
	return axis.Spec[testSparseAxis]{
		Key:    testSparseAxisKey,
		Bottom: func() testSparseAxis { return testSparseAxisBottom },
		Top:    func() testSparseAxis { return testSparseAxisTop },
		Equal:  func(a, b testSparseAxis) bool { return a == b },
		LessOrEq: func(a, b testSparseAxis) bool {
			return a <= b
		},
		Join: func(a, b testSparseAxis) testSparseAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b testSparseAxis) testSparseAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next testSparseAxis) testSparseAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v testSparseAxis) uint64 {
			return uint64(v) + 1
		},
	}
}

type testReducerAxis uint8

const (
	testReducerAxisBottom testReducerAxis = iota
	testReducerAxisRaw
	testReducerAxisNormalized
	testReducerAxisTop
)

var testReducerAxisKey = axis.NewKey[testReducerAxis]("factflow.test.reducer.source")
var testReducerMirrorAxisKey = axis.NewKey[testReducerAxis]("factflow.test.reducer.mirror")

func testReducerAxisSpec() axis.Spec[testReducerAxis] {
	spec := axis.Spec[testReducerAxis]{
		Key:    testReducerAxisKey,
		Bottom: func() testReducerAxis { return testReducerAxisBottom },
		Top:    func() testReducerAxis { return testReducerAxisTop },
		Equal:  func(a, b testReducerAxis) bool { return a == b },
		LessOrEq: func(a, b testReducerAxis) bool {
			return a <= b
		},
		Join: func(a, b testReducerAxis) testReducerAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b testReducerAxis) testReducerAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next testReducerAxis) testReducerAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v testReducerAxis) uint64 {
			return uint64(v) + 1
		},
	}
	spec.Reducer = func(w axis.Writer) bool {
		source, ok := axis.Get(w, testReducerAxisKey)
		if !ok || source != testReducerAxisRaw {
			return false
		}
		if mirror, ok := axis.Get(w, testReducerMirrorAxisKey); ok && mirror == testReducerAxisNormalized {
			return false
		}
		axis.Set(w, testReducerMirrorAxisKey, testReducerAxisNormalized)
		return true
	}
	return spec
}

func testReducerMirrorAxisSpec() axis.Spec[testReducerAxis] {
	spec := testReducerAxisSpec()
	spec.Key = testReducerMirrorAxisKey
	spec.Reducer = nil
	return spec
}

func assertSparseAxisValue[T comparable](t *testing.T, reg *axis.Registry, got product.Value, axisKey axis.Key[T], want T) {
	t.Helper()
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("value is bottom, want axis %s = %v", axisKey.ID(), want)
	}
	if gotAxis := product.Get(reg, got, axisKey); gotAxis != want {
		t.Fatalf("axis %s = %v, want %v", axisKey.ID(), gotAxis, want)
	}
}

func branchWithPresence(
	targetPath path.Path,
	truePresence presence.Value,
	hasTrue bool,
	falsePresence presence.Value,
	hasFalse bool,
) factflow.BranchRefinement {
	var trueValue factflow.ValueRefinement
	if hasTrue {
		trueValue = factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, truePresence))
	}
	var falseValue factflow.ValueRefinement
	if hasFalse {
		falseValue = factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, falsePresence))
	}
	return factflow.NewBranchRefinement(targetPath, trueValue, hasTrue, falseValue, hasFalse)
}

func branchWithRuntimeKind(
	targetPath path.Path,
	trueRuntimeKind runtimekind.Value,
	hasTrue bool,
	falseRuntimeKind runtimekind.Value,
	hasFalse bool,
) factflow.BranchRefinement {
	var trueValue factflow.ValueRefinement
	if hasTrue {
		trueValue = factflow.NewValueConstraint(product.Set(standard.Registry(), product.Top(), runtimekind.Key, trueRuntimeKind))
	}
	var falseValue factflow.ValueRefinement
	if hasFalse {
		falseValue = factflow.NewValueConstraint(product.Set(standard.Registry(), product.Top(), runtimekind.Key, falseRuntimeKind))
	}
	return factflow.NewBranchRefinement(targetPath, trueValue, hasTrue, falseValue, hasFalse)
}

func fieldSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}
