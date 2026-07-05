package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestWIRAddressResolverSeparatesVisibleAndRootOrVisibleLocalRoots(t *testing.T) {
	point := cfg.Point(7)
	sym := symbol.ID(42)
	body := wir.NewBody("resolver-test")
	op := pathOperand(body, pathdom.Path{Root: "value", Symbol: sym})

	builder := visibility.NewBuilder()
	builder.Define(point, sym, "value")
	resolver := visibility.NewResolver(builder.Build())
	addresses := NewWIRAddressResolver(body, resolver)

	visible, ok := addresses.Resolve(point, op, wir.AccessReadBefore)
	if !ok {
		t.Fatal("Resolve(read-before) returned !ok")
	}
	root, ok := addresses.Resolve(point, op, wir.AccessRootOrVisible)
	if !ok {
		t.Fatal("Resolve(root-or-visible) returned !ok")
	}
	if got := resolver.KeySpace().Format(visible); got != "sym42@1" {
		t.Fatalf("read-before key = %q, want visible local key", got)
	}
	if got := resolver.KeySpace().Format(root); got != "sym42" {
		t.Fatalf("root-or-visible key = %q, want structural root key", got)
	}
	if visible == root {
		t.Fatal("visible and root-or-visible keys collapsed; mutable locals need point-visible identity")
	}
}

func TestWIRAddressResolverUsesVisibleKeysForMemberRootOrVisibleAndEvidence(t *testing.T) {
	point := cfg.Point(3)
	sym := symbol.ID(9)
	member := pathdom.Path{Root: "box", Symbol: sym}.Field("value")
	body := wir.NewBody("resolver-test")
	op := pathOperand(body, member)

	builder := visibility.NewBuilder()
	builder.Define(point, sym, "box")
	resolver := visibility.NewResolver(builder.Build())
	addresses := NewWIRAddressResolver(body, resolver)

	rootOrVisible, ok := addresses.Resolve(point, op, wir.AccessRootOrVisible)
	if !ok {
		t.Fatal("Resolve(root-or-visible member) returned !ok")
	}
	evidence, ok := addresses.Resolve(point, op, wir.AccessEvidence)
	if !ok {
		t.Fatal("Resolve(evidence member) returned !ok")
	}
	want := `sym9@1.value`
	if got := string(resolver.KeySpace().Format(rootOrVisible)); got != want {
		t.Fatalf("root-or-visible member key = %q, want %q", got, want)
	}
	if got := string(resolver.KeySpace().Format(evidence)); got != want {
		t.Fatalf("evidence member key = %q, want %q", got, want)
	}
}

func TestWIRAddressResolverRejectsNonPathOperands(t *testing.T) {
	resolver := NewWIRAddressResolver(wir.NewBody("resolver-test"), visibility.NewResolver(visibility.NewTable(nil)))
	if key, ok := resolver.Resolve(cfg.Point(1), wir.Operand{Kind: wir.OperandConst, Ref: 1}, wir.AccessReadBefore); ok || key.Kind != 0 {
		t.Fatalf("Resolve(non-path) = %#v/%v, want invalid false", key, ok)
	}
}

func pathOperand(body *wir.Body, p pathdom.Path) wir.Operand {
	return wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(p))}
}

func TestWIRAddressResolverWriteLocalUsesVisibleLocalKey(t *testing.T) {
	point := cfg.Point(11)
	sym := symbol.ID(17)
	body := wir.NewBody("resolver-test")
	op := pathOperand(body, pathdom.Path{Root: "target", Symbol: sym, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}})

	builder := visibility.NewBuilder()
	builder.Define(point, sym, "target")
	resolver := visibility.NewResolver(builder.Build())
	addresses := NewWIRAddressResolver(body, resolver)

	got, ok := addresses.Resolve(point, op, wir.AccessWriteLocal)
	if !ok {
		t.Fatal("Resolve(write-local) returned !ok")
	}
	if formatted := resolver.KeySpace().Format(got); formatted != "sym17@1.field" {
		t.Fatalf("write-local key = %q, want visible local member", formatted)
	}
}
