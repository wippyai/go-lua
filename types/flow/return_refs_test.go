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
	fnA := WithFunctionRef(nil, constraint.NewPlaceholder(0).Field("a").Key(), FunctionRefSetOf(FunctionRef{GraphID: 1}))
	fnB := WithFunctionRef(nil, constraint.NewPlaceholder(1).Field("b").Key(), FunctionRefSetOf(FunctionRef{GraphID: 2}))
	closureA := WithClosureRef(nil, constraint.NewPlaceholder(0).Field("c").Key(), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 3}, CaptureCellsDomain.Bottom(), nil)))

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
			WithFunctionRef(nil, fnPath.Key(), FunctionRefSetOf(fn)),
			WithClosureRef(nil, clPath.Key(), ClosureRefSetOf(cl)),
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
	if set, ok := FunctionRefAt(ctx.FunctionRefs(), fnPath.Key()); !ok {
		t.Fatalf("function refs missing: %#v", ctx.FunctionRefs())
	} else if gotRef, singleton := set.Singleton(); !singleton || gotRef != fn {
		t.Fatalf("function ref = %s, want %v", set.Format(), fn)
	}
	if set, ok := ClosureRefAt(ctx.ClosureRefs(), clPath.Key()); !ok {
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
		WithFunctionRef(nil, fnPath.Key(), FunctionRefSetOf(FunctionRef{GraphID: 12})),
		ClosureRefsDomain.Bottom(),
	))

	refs := slot.ReferenceContext()
	if entries := refs.CaptureCells().Entries(); len(entries) != 0 {
		t.Fatalf("return slot retained capture cells: %#v", entries)
	}
	if set, ok := FunctionRefAt(refs.FunctionRefs(), fnPath.Key()); !ok || set.IsBottom() {
		t.Fatalf("return slot lost callable identity: %#v", refs.FunctionRefs())
	}
}
