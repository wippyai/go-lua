package visibility

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFreezeDynamicReadAddressKeepsCandidateSetAndExactVisibleDistinct(t *testing.T) {
	point := cfg.Point(7)
	sym := symbol.ID(41)
	builder := NewBuilder()
	builder.Define(point, sym, "value")
	resolver := NewResolver(builder.Build())
	p := pathdom.NewPath(sym, "value").Field("member")

	got, ok := FreezeDynamicReadAddress(resolver.KeySpace(), resolver, point, p)
	if !ok {
		t.Fatal("FreezeDynamicReadAddress returned false")
	}
	wantKeys := AddressAt(resolver, point, p).StateKeys(StateKeyVisible, StateKeyRootOrVisible)
	if !reflect.DeepEqual(got.StateKeys, wantKeys) {
		t.Fatalf("candidate keys = %v, want %v", got.StateKeys, wantKeys)
	}
	wantVisible, visible := AddressAt(resolver, point, p).VisibleStateKey()
	if !visible || !got.HasVisible || got.Visible != wantVisible {
		t.Fatalf("exact visible = %q/%v, want %q/true", got.Visible, got.HasVisible, wantVisible)
	}
}

func TestFreezeDynamicReadAddressDoesNotInventBoundaryVisibleKey(t *testing.T) {
	keys := keyspace.New()
	p := pathdom.NewPath(symbol.ID(42), "boundary")
	got, ok := FreezeDynamicReadAddress(keys, nil, 0, p)
	if !ok {
		t.Fatal("FreezeDynamicReadAddress returned false")
	}
	if got.HasVisible || got.Visible != "" {
		t.Fatalf("boundary visible = %q/%v, want absent", got.Visible, got.HasVisible)
	}
	want := pathaddr.StateKey(keys.FormatReadOnly(keys.FromPath(p)))
	if !reflect.DeepEqual(got.StateKeys, []pathaddr.StateKey{want}) {
		t.Fatalf("boundary keys = %v, want [%s]", got.StateKeys, want)
	}
}
