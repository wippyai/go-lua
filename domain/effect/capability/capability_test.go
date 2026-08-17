package capability_test

import (
	"reflect"
	"strings"
	"testing"

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
		capability.OwnershipExport:    capability.StatusReserved,
		capability.OwnershipOpaque:    capability.StatusReservedHighRisk,
		capability.OwnershipFreeze:    capability.StatusReserved,
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

func TestAuditedConcreteSymbolsHaveOneDescriptor(t *testing.T) {
	samples := []struct {
		symbol any
		id     string
	}{
		{returns.SameAs{}, capability.ReturnsReturnSameAs},
		{returns.ElementOf{}, capability.ReturnsReturnElementOf},
		{returns.OptionalElementOf{}, capability.ReturnsReturnOptionalElementOf},
		{returns.CallbackReturn{}, capability.ReturnsReturnCallbackReturn},
		{returns.ArrayOfCallbackReturn{}, capability.ReturnsReturnArrayOfCallbackReturn},
		{returns.TypeProjection{}, capability.ReturnsReturnTypeProjection},
		{returns.ConditionalType{}, capability.ReturnsReturnConditionalType},
		{returns.ErrorReturn{}, capability.ReturnsErrorReturn},
		{returns.ReturnLength{}, capability.ReturnsReturnLength},
		{returns.CorrelatedReturn{}, capability.ReturnsCorrelatedReturn},

		{postcondition.NormalReturnRefinement{}, capability.PostconditionNormalReturnRefinement},

		{ownership.Borrow{}, capability.OwnershipBorrow},
		{ownership.Retain{}, capability.OwnershipRetain},
		{ownership.Store{}, capability.OwnershipStore},
		{ownership.Send{}, capability.OwnershipSend},
		{ownership.SendParam{}, capability.OwnershipSendParam},
		{ownership.Export{}, capability.OwnershipExport},
		{ownership.Opaque{}, capability.OwnershipOpaque},
		{ownership.Freeze{}, capability.OwnershipFreeze},
		{ownership.BorrowAll{}, capability.OwnershipBorrowAll},

		{iteration.Iterator{}, capability.IterationIterator},

		{dispatch.ModuleLoad{}, capability.DispatchModuleLoad},

		{mutation.Mutate{}, capability.MutationMutate},
		{mutation.LengthChange{}, capability.MutationLengthChange},
		{mutation.TableMutator{}, capability.MutationTableMutator},

		{lifecycle.Acquire{}, capability.LifecycleAcquire},
		{lifecycle.Transition{}, capability.LifecycleTransition},
		{lifecycle.Escape{}, capability.LifecycleEscape},

		{control.Throw{}, capability.ControlThrow},
		{control.IO{}, capability.ControlIO},
	}

	seenTypes := map[reflect.Type]string{}
	seenIDs := map[string]reflect.Type{}
	for _, sample := range samples {
		typ := reflect.TypeOf(sample.symbol)
		if previous, ok := seenTypes[typ]; ok {
			t.Fatalf("%s and %s both classify %s", previous, sample.id, typ)
		}
		if previous, ok := seenIDs[sample.id]; ok {
			t.Fatalf("%s is assigned to both %s and %s", sample.id, previous, typ)
		}
		if _, ok := capability.Lookup(sample.id); !ok {
			t.Fatalf("%s has no descriptor for %s", typ, sample.id)
		}
		seenTypes[typ] = sample.id
		seenIDs[sample.id] = typ
	}
	all := capability.All()
	if len(seenIDs) != len(all) {
		t.Fatalf("concrete audited symbols = %d, descriptors = %d", len(seenIDs), len(all))
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
