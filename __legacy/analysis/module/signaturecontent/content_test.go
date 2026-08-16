package signaturecontent

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestDeriveIsStableAcrossOwnedClone(t *testing.T) {
	sig := signature.Function{Type: typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()}
	left, err := Derive(context.Background(), sig)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Derive(context.Background(), sig.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !left.Available() || left != right {
		t.Fatalf("clone identities = %x and %x", left, right)
	}
}

func TestFramedDigestSeparatesDomainAndComponentBoundaries(t *testing.T) {
	ctx := context.Background()
	first, err := framedDigest(ctx, "ab", []byte("c"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := framedDigest(ctx, "a", []byte("bc"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := framedDigest(ctx, allocationDomain, []byte("c"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == third {
		t.Fatal("canonical framing failed to separate ambiguous components or domains")
	}
}

func TestDeriveHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if id, err := Derive(ctx, signature.Function{Type: typ.Func().Build()}); err == nil || id.Available() {
		t.Fatalf("canceled derive = %x, %v", id, err)
	}
}

func TestAllocationIdentityUsesCanonicalSemanticTypes(t *testing.T) {
	allocation := func(value typ.Type) signature.Function {
		return signature.Function{
			Type: typ.Func().Returns(typ.Any).Build(),
			OperationalEffects: &signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
				ReturnIndex: 0,
				Root:        "root",
				Objects:     []signature.AllocationObjectTemplate{{ID: "root", Type: value}},
			}}},
		}
	}
	identity := func(sig signature.Function) signature.ContentID {
		t.Helper()
		id, err := DeriveAllocationTemplates(context.Background(), sig)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	leftFunction := typ.Func().Param("left", typ.String).Build()
	rightFunction := typ.Func().Param("right", typ.String).Build()
	if left, right := identity(allocation(leftFunction)), identity(allocation(rightFunction)); left != right || !left.Available() {
		t.Fatalf("function-label allocation identities = %x and %x", left, right)
	}
	if alias, plain := identity(allocation(typ.NewAlias("NumberAlias", typ.Number))), identity(allocation(typ.Number)); alias != plain {
		t.Fatalf("alias allocation identities = %x and %x", alias, plain)
	}
}

func TestAllocationIdentityRejectsUnboundEmbeddedCycle(t *testing.T) {
	iface := &typ.Interface{Name: "cycle"}
	cycle := &typ.Function{Returns: []typ.Type{iface}}
	iface.Methods = []typ.Method{{Name: "next", Type: cycle}}
	sig := signature.Function{
		Type: typ.Func().Build(),
		OperationalEffects: &signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0, Root: "root", Objects: []signature.AllocationObjectTemplate{{ID: "root", Type: cycle}},
		}}},
	}
	if id, err := DeriveAllocationTemplates(context.Background(), sig); err == nil || id.Available() {
		t.Fatalf("cyclic allocation identity = %x, %v", id, err)
	}
}
