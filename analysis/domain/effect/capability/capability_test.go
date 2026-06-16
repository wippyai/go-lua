package capability_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestDescriptorsClassifyAuditedVocabularyExactlyOnce(t *testing.T) {
	expected := map[string]capability.Status{
		capability.ReturnsReturnSameAs:                capability.StatusOperational,
		capability.ReturnsReturnElementOf:             capability.StatusOperational,
		capability.ReturnsReturnOptionalElementOf:     capability.StatusOperational,
		capability.ReturnsReturnCallbackReturn:        capability.StatusOperational,
		capability.ReturnsReturnArrayOfCallbackReturn: capability.StatusOperational,
		capability.ReturnsReturnTypeProjection:        capability.StatusOperational,
		capability.ReturnsErrorReturn:                 capability.StatusOperational,
		capability.ReturnsReturnLength:                capability.StatusReserved,
		capability.ReturnsReturnDeepElementOf:         capability.StatusReserved,
		capability.ReturnsReturnStringUnpackValue:     capability.StatusReservedHighRisk,
		capability.ReturnsReturnSelectCaseOfParam:     capability.StatusReserved,
		capability.ReturnsReturnSelectResultOfCases:   capability.StatusReserved,
		capability.ReturnsCorrelatedReturn:            capability.StatusReservedHighRisk,

		capability.PostconditionNormalReturnRefinement: capability.StatusOperational,

		capability.OwnershipBorrow:    capability.StatusOperational,
		capability.OwnershipRetain:    capability.StatusOperational,
		capability.OwnershipStore:     capability.StatusOperational,
		capability.OwnershipSendParam: capability.StatusOperational,
		capability.OwnershipExport:    capability.StatusOperational,
		capability.OwnershipOpaque:    capability.StatusOperational,
		capability.OwnershipFreeze:    capability.StatusOperational,
		capability.OwnershipBorrowAll: capability.StatusImportOnly,

		capability.IterationIterator: capability.StatusImportOnly,

		capability.DispatchModuleLoad:        capability.StatusPartial,
		capability.DispatchTypePredicate:     capability.StatusReservedHighRisk,
		capability.DispatchVariadicTransform: capability.StatusReservedHighRisk,

		capability.MutationMutate:       capability.StatusPartial,
		capability.MutationLengthChange: capability.StatusPartial,
		capability.MutationTableMutator: capability.StatusPartial,

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
		{returns.ErrorReturn{}, capability.ReturnsErrorReturn},
		{returns.ReturnLength{}, capability.ReturnsReturnLength},
		{returns.DeepElementOf{}, capability.ReturnsReturnDeepElementOf},
		{returns.StringUnpackValue{}, capability.ReturnsReturnStringUnpackValue},
		{returns.SelectCaseOfParam{}, capability.ReturnsReturnSelectCaseOfParam},
		{returns.SelectResultOfCases{}, capability.ReturnsReturnSelectResultOfCases},
		{returns.CorrelatedReturn{}, capability.ReturnsCorrelatedReturn},

		{postcondition.NormalReturnRefinement{}, capability.PostconditionNormalReturnRefinement},

		{ownership.Borrow{}, capability.OwnershipBorrow},
		{ownership.Retain{}, capability.OwnershipRetain},
		{ownership.Store{}, capability.OwnershipStore},
		{ownership.SendParam{}, capability.OwnershipSendParam},
		{ownership.Export{}, capability.OwnershipExport},
		{ownership.Opaque{}, capability.OwnershipOpaque},
		{ownership.Freeze{}, capability.OwnershipFreeze},
		{ownership.BorrowAll{}, capability.OwnershipBorrowAll},

		{iteration.Iterator{}, capability.IterationIterator},

		{dispatch.ModuleLoad{}, capability.DispatchModuleLoad},
		{dispatch.TypePredicate{}, capability.DispatchTypePredicate},
		{dispatch.VariadicTransform{}, capability.DispatchVariadicTransform},

		{mutation.Mutate{}, capability.MutationMutate},
		{mutation.LengthChange{}, capability.MutationLengthChange},
		{mutation.TableMutator{}, capability.MutationTableMutator},

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

func TestStringUnpackValueIsPinnedReservedHighRisk(t *testing.T) {
	desc, ok := capability.Lookup(capability.ReturnsReturnStringUnpackValue)
	if !ok {
		t.Fatal("missing StringUnpackValue descriptor")
	}
	if desc.Status == capability.StatusOperational {
		t.Fatal("StringUnpackValue must not be classified as operational")
	}
	if desc.Status != capability.StatusReservedHighRisk {
		t.Fatalf("StringUnpackValue status = %q, want %q", desc.Status, capability.StatusReservedHighRisk)
	}
	if !strings.Contains(desc.Rationale, "Reserved metadata") {
		t.Fatalf("StringUnpackValue rationale = %q, want reserved metadata rationale", desc.Rationale)
	}
	if !strings.Contains(desc.Rationale, "stdlib must not declare it while inactive") {
		t.Fatalf("StringUnpackValue rationale = %q, want stdlib quarantine rationale", desc.Rationale)
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

func TestInactiveDispatchLabelsArePinnedReservedHighRisk(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		rationale string
	}{
		{
			name:      "type predicate",
			id:        capability.DispatchTypePredicate,
			rationale: "type() narrowing is syntax/factflow based",
		},
		{
			name:      "variadic transform",
			id:        capability.DispatchVariadicTransform,
			rationale: "select() lowering ignores this",
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

func TestPartialDispatchModuleLoadDocumentsRequireNameBinding(t *testing.T) {
	desc, ok := capability.Lookup(capability.DispatchModuleLoad)
	if !ok {
		t.Fatal("missing ModuleLoad descriptor")
	}
	if desc.Status != capability.StatusPartial {
		t.Fatalf("ModuleLoad status = %q, want %q", desc.Status, capability.StatusPartial)
	}
	for _, want := range []string{
		"Metadata marker",
		"name-bound to require",
		"does not inspect this label",
	} {
		if !strings.Contains(desc.Rationale, want) {
			t.Fatalf("ModuleLoad rationale = %q, want to contain %q", desc.Rationale, want)
		}
	}
}

func TestPartialMutationDescriptorsDocumentTargetOnlyLowering(t *testing.T) {
	tests := []struct {
		id      string
		payload string
	}{
		{id: capability.MutationMutate, payload: "Transform and LengthDelta are metadata"},
		{id: capability.MutationLengthChange, payload: "Delta is metadata"},
		{id: capability.MutationTableMutator, payload: "Value is metadata"},
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
			for _, want := range []string{
				"consumes only Target",
				"path-invalidation authority",
				tt.payload,
			} {
				if !strings.Contains(desc.Rationale, want) {
					t.Fatalf("%s rationale = %q, want to contain %q", tt.id, desc.Rationale, want)
				}
			}
		})
	}
}
