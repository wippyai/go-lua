package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// TestRecursiveWidenPathEqualIsKernelOfHash exercises the convergence widen path
// (value.WidenForConvergence), which the inter-procedural fixpoint applies to a
// function summary every iteration. The widened representative must be product-
// Equal to its source family and intern to the same node: if widening produces a
// hash-equal but Equal-distinct representative, the fixpoint never detects a fixed
// point and hangs.
func TestRecursiveWidenPathEqualIsKernelOfHash(t *testing.T) {
	families := []struct {
		name string
		mk   func() typ.Type
	}{
		{"self_param_method", func() typ.Type { return muSelfParamMethod("Node") }},
		{"metatable_self", func() typ.Type { return muMetatableSelf("Bus") }},
		{"method_chain", func() typ.Type { return muMethodChain("Chain") }},
	}
	for _, f := range families {
		t.Run(f.name, func(t *testing.T) {
			base := f.mk()
			widened := value.WidenForConvergence(base)

			a := FromType(base)
			b := FromType(widened)
			c := FromType(value.WidenForConvergence(widened))

			if a.Hash() == b.Hash() && !Equal(a, b) {
				t.Fatalf("widen produced a hash-equal but Equal-distinct family (fixpoint cannot converge)\n  base   =%s\n  widened=%s", a.ProjectValue().String(), b.ProjectValue().String())
			}
			if b.Hash() == c.Hash() && !Equal(b, c) {
				t.Fatalf("widen is not idempotent up to Equal: widen(widen(x)) hash-equal but not Equal to widen(x)")
			}
			if Equal(b, c) && b.n != c.n {
				t.Fatalf("idempotent widen must intern to one canonical node")
			}
		})
	}
}

// muSelfParamMethod builds mu X.{pending: number, run: (X) -> ()}: a multi-field
// record whose method takes the recursive self as a PARAMETER (the Bus:run shape),
// unlike muMethodChain which only returns self.
func muSelfParamMethod(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		run := typ.Func().Param("self", self).Build()
		return typ.NewRecord().
			Field("pending", typ.Number).
			Field("run", run).
			Build()
	})
}

// muMetatableSelf builds a record made recursive through its METATABLE
// (Bus.__index = Bus): {pending: number} with metatable {run: (self) -> ()}.
func muMetatableSelf(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		meta := typ.NewRecord().
			Field("run", typ.Func().Param("self", self).Build()).
			Build()
		return typ.NewRecord().
			Field("pending", typ.Number).
			Metatable(meta).
			Build()
	})
}

// candidateFamilies exercises recursive constructions that resemble the Bus
// self-record that the inter-procedural fixpoint failed to converge on. For each,
// two independent observations of the same family must be product-Equal, hash
// identically, and intern to one canonical node. Equal must be the kernel of Hash:
// equal hashes with unequal values is the change-detection defect that hangs the
// fixpoint.
func TestRecursiveFamilyEqualIsKernelOfHash(t *testing.T) {
	// unfoldOnce returns (μX.F(X), F(μX.F(X))): the recursive node and its
	// one-level unfolding. Both denote the same infinite type and the same
	// product family, but the top-level node kinds differ (Recursive vs Record),
	// which is how the inter-procedural fixpoint re-derives the same family with a
	// structurally distinct representative on a later iteration.
	unfoldOnce := func(mk func() typ.Type) (typ.Type, typ.Type) {
		rec := mk()
		r, ok := rec.(*typ.Recursive)
		if !ok || r.Body == nil {
			t.Fatalf("precondition: %T is not a sealed recursive node", rec)
		}
		return rec, r.Body
	}

	cases := []struct {
		name string
		pair func() (typ.Type, typ.Type)
	}{
		{"self_param_method_twice", func() (typ.Type, typ.Type) { return muSelfParamMethod("Node"), muSelfParamMethod("Node") }},
		{"metatable_self_twice", func() (typ.Type, typ.Type) { return muMetatableSelf("Bus"), muMetatableSelf("Bus") }},
		{"self_param_method_unfolded", func() (typ.Type, typ.Type) { return unfoldOnce(func() typ.Type { return muSelfParamMethod("Node") }) }},
		{"metatable_self_unfolded", func() (typ.Type, typ.Type) { return unfoldOnce(func() typ.Type { return muMetatableSelf("Bus") }) }},
		{"chain_unfolded", func() (typ.Type, typ.Type) { return unfoldOnce(func() typ.Type { return muMethodChain("Chain") }) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, bt := c.pair()
			a := FromType(at)
			b := FromType(bt)
			eq := Equal(a, b)
			sameHash := a.Hash() == b.Hash()
			if sameHash && !eq {
				t.Fatalf("Equal is not the kernel of Hash: equal hashes but Equal=false (sameNode=%v)\n  a=%s\n  b=%s",
					a.n == b.n, a.ProjectValue().String(), b.ProjectValue().String())
			}
			if eq && !sameHash {
				t.Fatalf("Equal values must hash identically")
			}
			if eq && a.n != b.n {
				t.Fatalf("Equal recursive values must intern to one canonical node")
			}
		})
	}
}
