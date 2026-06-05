package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRefinementEffectDoesNotInvalidateAssignmentOwnedAxes(t *testing.T) {
	sym := cfg.SymbolID(901)
	keySym := cfg.SymbolID(902)
	path := constraint.NewPath(sym, "value")
	memberPath := path.Field("payload")
	memberKey := flow.SymbolPathKey(sym, memberPath.Segments)
	ref := flow.FunctionRef{GraphID: 903}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 904}, flow.CaptureCellsDomain.Bottom(), nil)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.NewOptional(typ.String)),
		},
		StaticMembers: flow.StaticMemberFacts{}.WithAddress(testStaticMemberAddressKey(t, memberKey), product.FromType(typ.Boolean)),
		KeyPresence:   testKeyPresenceWith(t, flow.KeyPresenceFacts{}, path, constraint.NewPath(keySym, "k")),
		FunctionRefs:  flow.WithFunctionRef(nil, memberPath.Key(), flow.FunctionRefSetOf(ref)),
		ClosureRefs:   flow.WithClosureRef(nil, memberPath.Key(), flow.ClosureRefSetOf(closure)),
		Rel:           flow.PointRelations{}.WithSiblingNil(keySym, []cfg.SymbolID{sym}),
	}

	tr.applyRefinementEffect(&out, RefinementEffect{
		Place: Place{Root: sym},
		Kind:  RefinementSetValue,
		Value: product.FromType(typ.String),
	})

	if got := out.Env[flow.SymbolValueKey(sym)].ProjectValue(); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("refined value = %v, want string", got)
	}
	if _, ok := out.StaticMembers.ValueAtAddress(testStaticMemberAddressKey(t, memberKey)); !ok {
		t.Fatalf("refinement killed static member fact: %s", out.StaticMembers.Format())
	}
	if !testKeyPresenceHas(t, out.KeyPresence, path, constraint.NewPath(keySym, "k")) {
		t.Fatalf("refinement killed key presence: %s", out.KeyPresence.Format())
	}
	if _, ok := flow.FunctionRefAt(out.FunctionRefs, memberPath.Key()); !ok {
		t.Fatalf("refinement killed function refs: %#v", out.FunctionRefs)
	}
	if _, ok := flow.ClosureRefAt(out.ClosureRefs, memberPath.Key()); !ok {
		t.Fatalf("refinement killed closure refs: %#v", out.ClosureRefs)
	}
	if _, ok := out.Rel.SiblingNil(keySym); !ok {
		t.Fatalf("refinement killed relation: %#v", out.Rel)
	}
}

func TestRefinementBottomCollapsesPointState(t *testing.T) {
	sym := cfg.SymbolID(905)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(sym): product.FromType(typ.Nil),
		},
		Cond:          constraint.TrueCondition(),
		Cells:         flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Nil)}}),
		CellEffects:   flow.CaptureEffectsIdentity(),
		StaticMembers: flow.StaticMemberFactsDomain.Top(),
		KeyPresence:   flow.KeyPresenceFactsDomain.Top(),
	}

	if !tr.applyRefinementEffect(&out, RefinementEffect{
		Place: Place{Root: sym},
		Kind:  RefinementSetValue,
		Value: product.Bottom(),
	}) {
		t.Fatal("bottom refinement did not report a state change")
	}

	if !flow.PointStateDomain.Equal(out, flow.PointStateDomain.Bottom()) {
		t.Fatalf("bottom refinement left state reachable: %#v", out)
	}
}

func TestRefinementEffectTypeCastsStaticPlace(t *testing.T) {
	sym := cfg.SymbolID(911)
	place := Place{
		Root: sym,
		Steps: []PlaceStep{{
			Kind:   PlaceStepStaticMember,
			Member: value.MemberField("payload"),
		}},
	}
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{
		flow.SymbolValueKey(sym): product.FromType(typ.NewRecord().
			Field("payload", typ.NewUnion(typ.String, typ.Number)).
			Field("stable", typ.Boolean).
			Build()),
	}}

	tr.applyRefinementEffect(&out, RefinementEffect{
		Place:     place,
		Kind:      RefinementTypeCast,
		Target:    typ.String,
		PreferEnv: true,
	})

	root := out.Env[flow.SymbolValueKey(sym)]
	payload, ok := product.MemberOf(root, value.MemberField("payload"))
	if !ok || !typ.TypeEquals(payload.ProjectValue(), typ.String) {
		t.Fatalf("payload = %v/%v, want string; root=%v", payload.ProjectValue(), ok, root.ProjectValue())
	}
	stable, ok := product.MemberOf(root, value.MemberField("stable"))
	if !ok || !typ.TypeEquals(stable.ProjectValue(), typ.Boolean) {
		t.Fatalf("stable = %v/%v, want boolean; root=%v", stable.ProjectValue(), ok, root.ProjectValue())
	}
}
