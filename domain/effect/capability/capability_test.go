package capability_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/domain/effect/control"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/iteration"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/effect/returns"
)

func TestDescriptorsClassifyAuditedVocabularyExactlyOnce(t *testing.T) {
	expected := map[string]capability.Status{
		capability.ReturnsReturnSameAs:                capability.StatusOperational,
		capability.ReturnsReturnElementOf:             capability.StatusOperational,
		capability.ReturnsReturnOptionalElementOf:     capability.StatusOperational,
		capability.ReturnsReturnCallbackReturn:        capability.StatusOperational,
		capability.ReturnsReturnArrayOfCallbackReturn: capability.StatusOperational,
		capability.ReturnsReturnTypeProjection:        capability.StatusOperational,
		capability.ReturnsReturnConditionalType:       capability.StatusOperational,
		capability.ReturnsErrorReturn:                 capability.StatusOperational,
		capability.ReturnsReturnLength:                capability.StatusReserved,
		capability.ReturnsCorrelatedReturn:            capability.StatusReservedHighRisk,

		capability.PostconditionNormalReturnRefinement: capability.StatusOperational,

		capability.OwnershipBorrow:    capability.StatusOperational,
		capability.OwnershipRetain:    capability.StatusOperational,
		capability.OwnershipStore:     capability.StatusOperational,
		capability.OwnershipSend:      capability.StatusImportOrStdlib,
		capability.OwnershipSendParam: capability.StatusOperational,
		capability.OwnershipExport:    capability.StatusOperational,
		capability.OwnershipOpaque:    capability.StatusOperational,
		capability.OwnershipFreeze:    capability.StatusOperational,
		capability.OwnershipBorrowAll: capability.StatusImportOrStdlib,

		capability.IterationIterator: capability.StatusImportOrStdlib,

		capability.DispatchModuleLoad: capability.StatusImportOrStdlib,

		capability.MutationMutate:       capability.StatusPartial,
		capability.MutationLengthChange: capability.StatusPartial,
		capability.MutationTableMutator: capability.StatusOperational,

		capability.LifecycleAcquire:    capability.StatusManifestValidated,
		capability.LifecycleTransition: capability.StatusManifestValidated,
		capability.LifecycleEscape:     capability.StatusManifestValidated,

		capability.ControlThrow: capability.StatusReservedHighRisk,
		capability.ControlIO:    capability.StatusReservedHighRisk,
	}

	all := capability.All()
	if len(all) != len(expected) {
		t.Fatalf("descriptor count = %d, want %d", len(all), len(expected))
	}
	for id, want := range expected {
		got, ok := capability.Lookup(id)
		if !ok {
			t.Fatalf("missing descriptor for %s", id)
		}
		if got.ID != id {
			t.Fatalf("%s descriptor ID = %q", id, got.ID)
		}
		if got.Status != want {
			t.Fatalf("%s status = %q, want %q", id, got.Status, want)
		}
	}
}

func TestFormalOwnershipLabelsAreManifestOperational(t *testing.T) {
	for _, id := range []string{
		capability.OwnershipExport,
		capability.OwnershipOpaque,
		capability.OwnershipFreeze,
	} {
		t.Run(id, func(t *testing.T) {
			desc, ok := capability.Lookup(id)
			if !ok {
				t.Fatalf("missing descriptor %s", id)
			}
			if desc.Status != capability.StatusOperational {
				t.Fatalf("%s status = %q, want %q", id, desc.Status, capability.StatusOperational)
			}
			for _, want := range []string{"Formal ownership declaration", "carried by manifests", "Target/Placement"} {
				if !strings.Contains(desc.Rationale, want) {
					t.Fatalf("%s rationale = %q, want to contain %q", id, desc.Rationale, want)
				}
			}
		})
	}
}

func TestDescriptorFieldsAreStableAndDocumented(t *testing.T) {
	for _, desc := range capability.All() {
		if desc.ID == "" {
			t.Fatal("descriptor has empty ID")
		}
		if desc.Family == "" {
			t.Fatalf("%s has empty family", desc.ID)
		}
		if desc.Symbol == "" {
			t.Fatalf("%s has empty symbol", desc.ID)
		}
		if !strings.HasPrefix(desc.ID, desc.Family+".") {
			t.Fatalf("%s does not include family prefix %q", desc.ID, desc.Family)
		}
		if strings.TrimSpace(desc.Rationale) == "" {
			t.Fatalf("%s has empty rationale", desc.ID)
		}
	}
}

func TestAllReturnsDeterministicCopy(t *testing.T) {
	first := capability.All()
	second := capability.All()
	if len(first) == 0 {
		t.Fatal("All returned no descriptors")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("All is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].ID >= first[i].ID {
			t.Fatalf("All is not sorted by ID at %d: %q >= %q", i, first[i-1].ID, first[i].ID)
		}
	}

	first[0].ID = "mutated"
	again := capability.All()
	if !reflect.DeepEqual(second, again) {
		t.Fatal("mutating All result changed canonical descriptors")
	}
}

// auditedLabels is the concrete effect vocabulary, one term per audited
// capability. The seven return transforms are distinguished by the Return label
// that carries them, which is how the vocabulary distinguishes them.
func auditedLabels() []effect.Label {
	return []effect.Label{
		returns.Return{Transform: returns.SameAs{}},
		returns.Return{Transform: returns.ElementOf{}},
		returns.Return{Transform: returns.OptionalElementOf{}},
		returns.Return{Transform: returns.CallbackReturn{}},
		returns.Return{Transform: returns.ArrayOfCallbackReturn{}},
		returns.Return{Transform: returns.TypeProjection{}},
		returns.Return{Transform: returns.ConditionalType{}},
		returns.ErrorReturn{},
		returns.ReturnLength{},
		returns.CorrelatedReturn{},

		postcondition.NormalReturnRefinement{},

		ownership.Borrow{},
		ownership.Retain{},
		ownership.Store{},
		ownership.Send{},
		ownership.SendParam{},
		ownership.Export{},
		ownership.Opaque{},
		ownership.Freeze{},
		ownership.BorrowAll{},

		iteration.Iterator{},

		dispatch.ModuleLoad{},

		mutation.Mutate{},
		mutation.LengthChange{},
		mutation.TableMutator{},

		lifecycle.Acquire{},
		lifecycle.Transition{},
		lifecycle.Escape{},

		control.Throw{},
		control.IO{},
	}
}

// TestAuditedLabelsAndDescriptorsAreABijection states the pairing between the
// Go vocabulary and the catalog: every label term classifies itself as exactly
// one audited capability, no two terms claim the same one, and no descriptor is
// left unclaimed. That a label term states an ID at all is the compiler's job,
// since effect.Label requires the method; what this law adds is that the stated
// IDs and the catalog are the same set.
func TestAuditedLabelsAndDescriptorsAreABijection(t *testing.T) {
	claimedBy := map[string]reflect.Type{}
	for _, label := range auditedLabels() {
		typ := reflect.TypeOf(label)
		id := label.CapabilityID()
		if id == "" {
			t.Fatalf("%s states no capability", typ)
		}
		if _, known := capability.Lookup(id); !known {
			t.Fatalf("%s states unaudited capability %s", typ, id)
		}
		if previous, ok := claimedBy[id]; ok {
			t.Fatalf("%s is claimed by both %s and %s", id, previous, typ)
		}
		claimedBy[id] = typ
	}

	all := capability.All()
	for _, desc := range all {
		if _, claimed := claimedBy[desc.ID]; !claimed {
			t.Errorf("descriptor %s is reachable from no label term", desc.ID)
		}
	}
	if len(claimedBy) != len(all) {
		t.Fatalf("audited label terms = %d, descriptors = %d", len(claimedBy), len(all))
	}
}

// TestPointerLabelsStateTheSameCapability states that the value and pointer
// spellings of a label, and of the transform it carries, are the same audited
// term once normalized.
func TestPointerLabelsStateTheSameCapability(t *testing.T) {
	for _, label := range auditedLabels() {
		pointer := reflect.New(reflect.TypeOf(label))
		pointer.Elem().Set(reflect.ValueOf(label))
		normalized := effect.NormalizeLabel(pointer.Interface().(effect.Label))
		if got, want := normalized.CapabilityID(), label.CapabilityID(); got != want {
			t.Errorf("pointer %T states %q, value states %q", label, got, want)
		}
	}

	pointerTransform := effect.NormalizeLabel(&returns.Return{Transform: &returns.ElementOf{}})
	if got := pointerTransform.CapabilityID(); got != capability.ReturnsReturnElementOf {
		t.Errorf("pointer return transform states %q, want %q", got, capability.ReturnsReturnElementOf)
	}
}

// TestReturnWithoutTransformStatesNoCapability states the one label term that
// carries no classifiable payload: a return whose transform is absent names no
// capability, so a consumer refuses it instead of defaulting it to one.
func TestReturnWithoutTransformStatesNoCapability(t *testing.T) {
	if got := (returns.Return{}).CapabilityID(); got != "" {
		t.Fatalf("return without transform states %q, want no capability", got)
	}
}

func TestCorrelatedReturnIsPinnedReservedHighRisk(t *testing.T) {
	desc, ok := capability.Lookup(capability.ReturnsCorrelatedReturn)
	if !ok {
		t.Fatal("missing CorrelatedReturn descriptor")
	}
	if desc.Status == capability.StatusOperational {
		t.Fatal("CorrelatedReturn must not be classified as operational")
	}
	if desc.Status != capability.StatusReservedHighRisk {
		t.Fatalf("CorrelatedReturn status = %q, want %q", desc.Status, capability.StatusReservedHighRisk)
	}
	if !strings.Contains(desc.Rationale, "Reserved metadata") {
		t.Fatalf("CorrelatedReturn rationale = %q, want reserved metadata rationale", desc.Rationale)
	}
	if !strings.Contains(desc.Rationale, "stdlib must not declare it while inactive") {
		t.Fatalf("CorrelatedReturn rationale = %q, want stdlib quarantine rationale", desc.Rationale)
	}
}

func TestInactiveControlLabelsArePinnedReservedHighRisk(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		rationale string
	}{
		{
			name:      "throw",
			id:        capability.ControlThrow,
			rationale: "behavior is represented by Never/postconditions/module-load",
		},
		{
			name:      "io",
			id:        capability.ControlIO,
			rationale: "IO policy/enforcement is inactive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, ok := capability.Lookup(tt.id)
			if !ok {
				t.Fatalf("missing descriptor %s", tt.id)
			}
			if desc.Status != capability.StatusReservedHighRisk {
				t.Fatalf("%s status = %q, want %q", tt.id, desc.Status, capability.StatusReservedHighRisk)
			}
			if !strings.Contains(desc.Rationale, "Reserved metadata") {
				t.Fatalf("%s rationale = %q, want reserved metadata rationale", tt.id, desc.Rationale)
			}
			if !strings.Contains(desc.Rationale, tt.rationale) {
				t.Fatalf("%s rationale = %q, want %q", tt.id, desc.Rationale, tt.rationale)
			}
			if !strings.Contains(desc.Rationale, "stdlib must not declare") {
				t.Fatalf("%s rationale = %q, want stdlib quarantine rationale", tt.id, desc.Rationale)
			}
		})
	}
}

func TestDispatchModuleLoadDocumentsCapabilityBinding(t *testing.T) {
	desc, ok := capability.Lookup(capability.DispatchModuleLoad)
	if !ok {
		t.Fatal("missing ModuleLoad descriptor")
	}
	if desc.Status != capability.StatusImportOrStdlib {
		t.Fatalf("ModuleLoad status = %q, want %q", desc.Status, capability.StatusImportOrStdlib)
	}
	for _, want := range []string{
		"operational capability",
		"bind through this label",
	} {
		if !strings.Contains(desc.Rationale, want) {
			t.Fatalf("ModuleLoad rationale = %q, want to contain %q", desc.Rationale, want)
		}
	}
}

// TestLifecycleLabelsArePinnedManifestValidated states the operational truth of
// the lifecycle vocabulary: manifests carry it and the typestate conformance
// relation validates it, and nothing lowers it into analysis facts. The reserved
// tiers are excluded because they bar a label from manifests entirely, which is
// the opposite of how this vocabulary is used.
func TestLifecycleLabelsArePinnedManifestValidated(t *testing.T) {
	for _, id := range []string{
		capability.LifecycleAcquire,
		capability.LifecycleTransition,
		capability.LifecycleEscape,
	} {
		t.Run(id, func(t *testing.T) {
			desc, ok := capability.Lookup(id)
			if !ok {
				t.Fatalf("missing descriptor %s", id)
			}
			switch desc.Status {
			case capability.StatusOperational:
				t.Fatalf("%s claims operational lowering that no consumer performs", id)
			case capability.StatusReserved, capability.StatusReservedHighRisk:
				t.Fatalf("%s is reserved out of manifests it is declared in", id)
			}
			if desc.Status != capability.StatusManifestValidated {
				t.Fatalf("%s status = %q, want %q", id, desc.Status, capability.StatusManifestValidated)
			}
			for _, want := range []string{
				"carried in signature manifests",
				"validated against the declared typestate FSM",
				"no lowering consumes it",
			} {
				if !strings.Contains(desc.Rationale, want) {
					t.Fatalf("%s rationale = %q, want to contain %q", id, desc.Rationale, want)
				}
			}
		})
	}
}

func TestPartialMutationDescriptorsDocumentTargetOnlyLowering(t *testing.T) {
	tests := []struct {
		id   string
		want []string
	}{
		{
			id: capability.MutationMutate,
			want: []string{
				"consumes only Target",
				"path-invalidation authority",
				"Transform and LengthDelta are metadata",
			},
		},
		{
			id: capability.MutationLengthChange,
			want: []string{
				"consumes Target",
				"path-invalidation authority",
				"positive Delta as a length-floor proof",
				"negative Delta remains metadata",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			desc, ok := capability.Lookup(tt.id)
			if !ok {
				t.Fatalf("missing descriptor %s", tt.id)
			}
			if desc.Status != capability.StatusPartial {
				t.Fatalf("%s status = %q, want %q", tt.id, desc.Status, capability.StatusPartial)
			}
			for _, want := range tt.want {
				if !strings.Contains(desc.Rationale, want) {
					t.Fatalf("%s rationale = %q, want to contain %q", tt.id, desc.Rationale, want)
				}
			}
		})
	}
}

func TestTableMutatorDescriptorDocumentsElementEvidenceLowering(t *testing.T) {
	desc, ok := capability.Lookup(capability.MutationTableMutator)
	if !ok {
		t.Fatal("missing TableMutator descriptor")
	}
	if desc.Status != capability.StatusOperational {
		t.Fatalf("TableMutator status = %q, want %q", desc.Status, capability.StatusOperational)
	}
	for _, want := range []string{"invalidates the target", "indexed element evidence", "Value end-to-end"} {
		if !strings.Contains(desc.Rationale, want) {
			t.Fatalf("TableMutator rationale %q missing %q", desc.Rationale, want)
		}
	}
}
