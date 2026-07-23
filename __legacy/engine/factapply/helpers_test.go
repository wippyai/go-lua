package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

func assertValue(t *testing.T, reg *axis.Registry, gotState state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := gotState.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("slot %v = %s, want %s", slot, formatValue(reg, got), formatValue(reg, want))
	}
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

func assertPathValue(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, gotState state.State, pathKey path.PathKey, want product.Value) {
	t.Helper()
	if got := gotState.ReadPathKey(reg, ks, pathKey); !product.Equal(reg, got, want) {
		t.Fatalf("path %s = %s, want %s", pathKey, formatValue(reg, got), formatValue(reg, want))
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

func fieldSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}

func fieldStaticKey(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromRootlessSuffix(fieldSuffix(name).Segments)
	if !ok {
		t.Fatalf("FromRootlessSuffix(%q) failed", name)
	}
	return k
}

func mustStateKey(t *testing.T, ks *keyspace.KeySpace, key path.PathKey) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(key)
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", key)
	}
	return k
}

func testStateKey(t *testing.T, key path.PathKey) pathaddr.StateKey {
	t.Helper()
	got, ok := pathaddr.StateKeyFromPathKey(key)
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", key)
	}
	return got
}
