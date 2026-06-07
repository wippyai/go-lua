package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnRefsLatticeLaws(t *testing.T) {
	fnA := WithFunctionRefPath(nil, constraint.NewPlaceholder(0).Field("a"), FunctionRefSetOf(FunctionRef{GraphID: 1}))
	fnB := WithFunctionRefPath(nil, constraint.NewPlaceholder(1).Field("b"), FunctionRefSetOf(FunctionRef{GraphID: 2}))
	closureA := WithClosureRefPath(nil, constraint.NewPlaceholder(0).Field("c"), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 3}, CaptureCellsDomain.Bottom(), nil)))

	lattice.LawSuite[ReturnRefs]{
		Name:   "ReturnRefs",
		Domain: ReturnRefsDomain,
		Sample: []ReturnRefs{
			ReturnRefsDomain.Bottom(),
			ReturnRefsDomain.Top(),
			ReturnRefsOfSlots([]ReturnRefSlot{ReturnRefSlotOf(fnA, ClosureRefsDomain.Bottom())}),
			ReturnRefsOfSlots([]ReturnRefSlot{ReturnRefSlotOf(FunctionRefsDomain.Bottom(), closureA)}),
			ReturnRefsOfSlots([]ReturnRefSlot{
				ReturnRefSlotOf(fnA, closureA),
				ReturnRefSlotOf(fnB, ClosureRefsDomain.Bottom()),
			}),
		},
	}.Run(t)
}

func TestReturnRefsOfSlotsCanonicalizesSlots(t *testing.T) {
	fnPath := constraint.NewPlaceholder(0).Field("factory")
	clPath := constraint.NewPlaceholder(0).Field("closure")
	fn := FunctionRef{GraphID: 10}
	cl := ClosureRefOf(FunctionRef{GraphID: 11}, CaptureCellsDomain.Bottom(), nil)

	got := ReturnRefsOfSlots([]ReturnRefSlot{
		ReturnRefSlotOf(
			WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(fn)),
			WithClosureRefPath(nil, clPath, ClosureRefSetOf(cl)),
		),
		ReturnRefSlotOf(FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom()),
	})
	if got.Len() != 1 {
		t.Fatalf("ReturnRefsOfSlots len = %d, want 1 after bottom trim", got.Len())
	}
	ctx, ok := got.SlotReferenceContext(0)
	if !ok {
		t.Fatal("SlotReferenceContext(0) missing")
	}
	if set, ok := FunctionRefAtPath(ctx.FunctionRefs(), fnPath); !ok {
		t.Fatalf("function refs missing: %#v", ctx.FunctionRefs())
	} else if gotRef, singleton := set.Singleton(); !singleton || gotRef != fn {
		t.Fatalf("function ref = %s, want %v", set.Format(), fn)
	}
	if set, ok := ClosureRefAtPath(ctx.ClosureRefs(), clPath); !ok {
		t.Fatalf("closure refs missing: %#v", ctx.ClosureRefs())
	} else if gotRef, singleton := set.Singleton(); !singleton || gotRef.Ref != cl.Ref {
		t.Fatalf("closure ref = %s, want %v", set.Format(), cl.Ref)
	}
	if _, ok := got.SlotReferenceContext(1); ok {
		t.Fatal("trimmed bottom slot should not report reference evidence")
	}
}

func TestReturnRefSlotDropsCaptureCells(t *testing.T) {
	fnPath := constraint.NewPlaceholder(0).Field("factory")
	slot := ReturnRefSlotOfReferenceContext(ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(12), Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(FunctionRef{GraphID: 12})),
		ClosureRefsDomain.Bottom(),
	))

	refs := slot.ReferenceContext()
	if entries := refs.CaptureCells().Entries(); len(entries) != 0 {
		t.Fatalf("return slot retained capture cells: %#v", entries)
	}
	if set, ok := FunctionRefAtPath(refs.FunctionRefs(), fnPath); !ok || set.IsBottom() {
		t.Fatalf("return slot lost callable identity: %#v", refs.FunctionRefs())
	}
}
