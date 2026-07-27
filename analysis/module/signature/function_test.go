package signature

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFunctionEqualsComparesTypeAndEffectSeparately(t *testing.T) {
	fn := typ.Func().
		Param("value", typ.String).
		Returns(typ.Boolean).
		Build()
	nonPure := Function{Type: fn, Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})}
	same := Function{Type: fn, Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})}
	pure := Function{Type: fn, Effect: effect.Empty}
	differentType := Function{
		Type: typ.Func().
			Param("value", typ.Number).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	if !nonPure.Equals(same) {
		t.Fatalf("equal signatures were not equal")
	}
	if nonPure.Equals(pure) {
		t.Fatalf("effect rows should be part of signature equality")
	}
	if nonPure.Equals(differentType) {
		t.Fatalf("function type should be part of signature equality")
	}
}

func TestFunctionCloneCopiesEffectRowAndFunctionSlices(t *testing.T) {
	original := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Open("rho", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	clone := original.Clone()
	if !clone.Equals(original) {
		t.Fatalf("clone = %v, want %v", clone, original)
	}
	if clone.Type == original.Type {
		t.Fatalf("clone should rebuild the function carrier type")
	}

	clone.Type.Params[0].Type = typ.Number
	clone.Effect.Tail.Name = "sigma"

	if original.Type.Params[0].Type != typ.String {
		t.Fatalf("clone mutation changed original function params")
	}
	if original.Effect.Tail.Name != "rho" {
		t.Fatalf("clone mutation changed original effect tail")
	}
}

func TestFunctionCloneCopiesOperationalEffects(t *testing.T) {
	original := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Build(),
		OperationalEffects: &OperationalEffects{
			EscapeEvents: []EscapeEvent{{
				Target:    pathdom.NewPlaceholder(0).Field("child"),
				Kind:      placement.Send,
				Recursive: true,
			}},
			PathStaticMembers: []PathStaticMemberFact{{
				Path: pathdom.NewPlaceholder(0).Field("member"),
				Type: typ.String,
			}},
			BranchProofs: []BranchProof{{
				Kind:  BranchProofPathNotEqual,
				Path:  pathdom.NewPlaceholder(0).Field("channel"),
				Other: pathdom.NewPlaceholder(1),
			}},
			KeyMemberships: []KeyMembership{{
				Key:   pathdom.NewPlaceholder(0).Field("key"),
				Table: pathdom.NewPlaceholder(0).Field("table"),
			}},
			DynamicValueKeys: []DynamicValueKeyMembership{{
				Container: pathdom.Path{Root: "ret[0]"},
				Site:      "return.keys",
				Table:     pathdom.NewPlaceholder(0).Field("table"),
			}},
			LifecycleEffects: []LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0).Field("tx"),
				Kind:     LifecycleTransition,
				Protocol: typestate.Protocol("transaction"),
				From:     typestate.State("active"),
				To:       typestate.State("committed"),
			}},
			ReturnAllocationTemplates: []ReturnAllocationTemplate{{
				ReturnIndex: 0,
				Root:        "ret0",
				Objects: []AllocationObjectTemplate{{
					ID:   "ret0",
					Type: typetable.NewRecord().Build(),
					StaticMembers: []AllocationStaticMemberTemplate{{
						Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "child"}},
						Value:  "ret0.child",
					}},
					DynamicEntries: []AllocationDynamicEntryTemplate{{
						KeyType: typ.String,
						Value:   "ret0.entry",
					}},
				}},
			}},
		},
	}

	clone := original.Clone()
	if !clone.Equals(original) {
		t.Fatalf("clone = %#v, want %#v", clone, original)
	}
	if clone.OperationalEffects == original.OperationalEffects {
		t.Fatalf("clone should not share operational effects pointer")
	}

	clone.OperationalEffects.EscapeEvents[0].Target.Segments[0].Name = "other"
	clone.OperationalEffects.EscapeEvents[0].Kind = placement.Opaque
	clone.OperationalEffects.PathStaticMembers[0].Path.Segments[0].Name = "changed"
	clone.OperationalEffects.BranchProofs[0].Path.Segments[0].Name = "changed"
	clone.OperationalEffects.KeyMemberships[0].Key.Segments[0].Name = "changed"
	clone.OperationalEffects.DynamicValueKeys[0].Table.Segments[0].Name = "changed"
	clone.OperationalEffects.LifecycleEffects[0].Target.Segments[0].Name = "changed"
	clone.OperationalEffects.LifecycleEffects[0].To = typestate.State("rolled_back")
	clone.OperationalEffects.ReturnAllocationTemplates[0].Objects[0].StaticMembers[0].Suffix[0].Name = "mutated"
	clone.OperationalEffects.ReturnAllocationTemplates[0].Objects[0].DynamicEntries[0].KeyType = typ.Number

	if got := original.OperationalEffects.EscapeEvents[0].Target.String(); got != "$0.child" {
		t.Fatalf("clone mutation changed original target: %s", got)
	}
	if got := original.OperationalEffects.EscapeEvents[0].Kind; got != placement.Send {
		t.Fatalf("clone mutation changed original kind: %v", got)
	}
	if got := original.OperationalEffects.PathStaticMembers[0].Path.String(); got != "$0.member" {
		t.Fatalf("clone mutation changed original static member path: %s", got)
	}
	if got := original.OperationalEffects.BranchProofs[0].Path.String(); got != "$0.channel" {
		t.Fatalf("clone mutation changed original branch proof path: %s", got)
	}
	if got := original.OperationalEffects.KeyMemberships[0].Key.String(); got != "$0.key" {
		t.Fatalf("clone mutation changed original key membership path: %s", got)
	}
	if got := original.OperationalEffects.DynamicValueKeys[0].Table.String(); got != "$0.table" {
		t.Fatalf("clone mutation changed original dynamic value key path: %s", got)
	}
	if got := original.OperationalEffects.LifecycleEffects[0].Target.String(); got != "$0.tx" {
		t.Fatalf("clone mutation changed original lifecycle target: %s", got)
	}
	if got := original.OperationalEffects.LifecycleEffects[0].To; got != typestate.State("committed") {
		t.Fatalf("clone mutation changed original lifecycle state: %s", got)
	}
	if got := segment.FormatSegments(original.OperationalEffects.ReturnAllocationTemplates[0].Objects[0].StaticMembers[0].Suffix); got != ".child" {
		t.Fatalf("clone mutation changed original allocation suffix: %s", got)
	}
	if got := original.OperationalEffects.ReturnAllocationTemplates[0].Objects[0].DynamicEntries[0].KeyType; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("clone mutation changed original allocation key type: %v", got)
	}
}

func TestSubstituteOperationalTypesRewritesEmbeddedTypeFields(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	effects := &OperationalEffects{
		NormalReturnTypeRefinements: []PathTypeRefinement{{
			Path: pathdom.NewPlaceholder(0).Field("value"),
			Type: param,
		}},
		PathStaticMembers: []PathStaticMemberFact{{
			Path: pathdom.NewPlaceholder(0).Field("member"),
			Type: typ.NewArray(param),
		}},
		DynamicIndexFacts: []DynamicIndexFact{{
			Table: pathdom.NewPlaceholder(0),
			Key:   DynamicIndexOperand{Path: pathdom.NewPlaceholder(1), Type: param},
			Value: DynamicIndexOperand{Path: pathdom.NewPlaceholder(2), Type: typ.NewArray(param)},
		}},
		ReturnAllocationTemplates: []ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        "ret0",
			Objects: []AllocationObjectTemplate{{
				ID:   "ret0",
				Type: typetable.NewRecord().Field("value", param).Build(),
				DynamicEntries: []AllocationDynamicEntryTemplate{{
					KeyType: param,
					Value:   "ret0.entry",
				}},
			}},
		}},
	}

	got := SubstituteOperationalTypes(effects, []*typ.TypeParam{param}, []typ.Type{typ.String})
	if got == nil {
		t.Fatal("SubstituteOperationalTypes returned nil")
	}
	if got == effects {
		t.Fatal("SubstituteOperationalTypes should return a copy")
	}
	if !typ.TypeEquals(got.NormalReturnTypeRefinements[0].Type, typ.String) {
		t.Fatalf("type refinement = %v, want string", got.NormalReturnTypeRefinements[0].Type)
	}
	if !typ.TypeEquals(got.PathStaticMembers[0].Type, typ.NewArray(typ.String)) {
		t.Fatalf("static member type = %v, want {string}", got.PathStaticMembers[0].Type)
	}
	if !typ.TypeEquals(got.DynamicIndexFacts[0].Key.Type, typ.String) {
		t.Fatalf("dynamic key type = %v, want string", got.DynamicIndexFacts[0].Key.Type)
	}
	if !typ.TypeEquals(got.DynamicIndexFacts[0].Value.Type, typ.NewArray(typ.String)) {
		t.Fatalf("dynamic value type = %v, want {string}", got.DynamicIndexFacts[0].Value.Type)
	}
	wantObject := typetable.NewRecord().Field("value", typ.String).Build()
	if !typ.TypeEquals(got.ReturnAllocationTemplates[0].Objects[0].Type, wantObject) {
		t.Fatalf("allocation object type = %v, want %v", got.ReturnAllocationTemplates[0].Objects[0].Type, wantObject)
	}
	if !typ.TypeEquals(got.ReturnAllocationTemplates[0].Objects[0].DynamicEntries[0].KeyType, typ.String) {
		t.Fatalf("allocation dynamic key type = %v, want string", got.ReturnAllocationTemplates[0].Objects[0].DynamicEntries[0].KeyType)
	}
	if typ.TypeEquals(effects.NormalReturnTypeRefinements[0].Type, typ.String) {
		t.Fatal("substitution mutated original effects")
	}
}

func TestFunctionEqualsDistinguishesOperationalEffectsAuthority(t *testing.T) {
	fn := typ.Func().Param("value", typ.String).Build()
	nilOperational := Function{Type: fn}
	emptyOperational := Function{Type: fn, OperationalEffects: &OperationalEffects{}}
	withOperational := Function{Type: fn, OperationalEffects: &OperationalEffects{
		FrozenTables: []FrozenTable{{Target: pathdom.NewPlaceholder(0)}},
	}}
	withStaticMembers := Function{Type: fn, OperationalEffects: &OperationalEffects{
		PathStaticMembers: []PathStaticMemberFact{{Path: pathdom.NewPlaceholder(0), Type: typ.String}},
	}}
	withBranchProof := Function{Type: fn, OperationalEffects: &OperationalEffects{
		BranchProofs: []BranchProof{{
			Kind:  BranchProofPathEqual,
			Path:  pathdom.NewPlaceholder(0),
			Other: pathdom.NewPlaceholder(1),
		}},
	}}
	withKeyMemberships := Function{Type: fn, OperationalEffects: &OperationalEffects{
		KeyMemberships: []KeyMembership{{Key: pathdom.NewPlaceholder(0), Table: pathdom.NewPlaceholder(0).Field("table")}},
	}}
	withAllocationTemplate := Function{Type: fn, OperationalEffects: &OperationalEffects{
		ReturnAllocationTemplates: []ReturnAllocationTemplate{{ReturnIndex: 0, Root: "ret0", Objects: []AllocationObjectTemplate{{
			ID:   "ret0",
			Type: typetable.NewRecord().Build(),
		}}}},
	}}
	withLifecycle := Function{Type: fn, OperationalEffects: &OperationalEffects{
		LifecycleEffects: []LifecycleEffect{{
			Target:   pathdom.NewPlaceholder(0),
			Kind:     LifecycleEscape,
			Protocol: typestate.Protocol("resource"),
		}},
	}}

	if nilOperational.Equals(emptyOperational) {
		t.Fatalf("nil operational effects should differ from authoritative empty effects")
	}
	if emptyOperational.Equals(withOperational) {
		t.Fatalf("different operational effects should not compare equal")
	}
	if withOperational.Equals(withStaticMembers) {
		t.Fatalf("different operational effects lanes should not compare equal")
	}
	if withOperational.Equals(withBranchProof) {
		t.Fatalf("branch proofs should be part of operational effect equality")
	}
	if withOperational.Equals(withKeyMemberships) {
		t.Fatalf("key membership facts should be part of operational effect equality")
	}
	if withOperational.Equals(withAllocationTemplate) {
		t.Fatalf("allocation templates should be part of operational effect equality")
	}
	if withOperational.Equals(withLifecycle) {
		t.Fatalf("lifecycle facts should be part of operational effect equality")
	}
}

func TestFunctionStringIncludesNonPureEffect(t *testing.T) {
	sig := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	if got, want := sig.String(), "fun(value: string) -> boolean ! {errret(val[0], err[1])}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
