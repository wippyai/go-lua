package wire

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/projection"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// The effect label codec writes a format other builds already read, so the
// bytes it produces for a label are a commitment. These laws hold it to that
// commitment from both ends and, at the same time, hold the boundary to one
// statement of the vocabulary: the audited capability catalog names what may
// cross, and the wire kind, payload and status verdict for each entry are read
// off a single table rather than restated per direction.

// effectLabelWireCase is one audited capability, the label that carries it, the
// wire kind it is spelled as, and the exact bytes of that spelling. For an
// entry the catalog bars from manifests the bytes are the probe a foreign
// writer might send, which the boundary must refuse by status.
type effectLabelWireCase struct {
	name  string
	id    string
	kind  string
	label effect.Label
	wire  string
}

func effectLabelWireCorpus() []effectLabelWireCase {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}

	return []effectLabelWireCase{
		{
			name:  "control.Throw",
			id:    capability.ControlThrow,
			kind:  "control.throw",
			label: control.Throw{},
			wire:  `{"kind":"control.throw"}`,
		},
		{
			name:  "control.IO",
			id:    capability.ControlIO,
			kind:  "control.io",
			label: control.IO{},
			wire:  `{"kind":"control.io"}`,
		},
		{
			name:  "dispatch.ModuleLoad",
			id:    capability.DispatchModuleLoad,
			kind:  "dispatch.moduleLoad",
			label: dispatch.ModuleLoad{},
			wire:  `{"kind":"dispatch.moduleLoad"}`,
		},
		{
			name:  "iteration.Iterator/indexed",
			id:    capability.IterationIterator,
			kind:  "iteration.iterator",
			label: iteration.Iterator{Source: p0, Kind: iteration.IterateIndexed},
			wire:  `{"kind":"iteration.iterator","source":{"index":0},"iteratorKind":"indexed"}`,
		},
		{
			name:  "iteration.Iterator/keyed",
			id:    capability.IterationIterator,
			kind:  "iteration.iterator",
			label: iteration.Iterator{Source: p1, Kind: iteration.IterateKeyed},
			wire:  `{"kind":"iteration.iterator","source":{"index":1},"iteratorKind":"keyed"}`,
		},
		{
			name: "lifecycle.Acquire/final",
			id:   capability.LifecycleAcquire,
			kind: "lifecycle.acquire",
			label: lifecycle.Acquire{
				Target:     p0,
				Protocol:   typestate.Protocol("transaction"),
				State:      typestate.State("active"),
				Obligation: typestate.Obligation{Final: typestate.State("finished")},
			},
			wire: `{"kind":"lifecycle.acquire","target":{"index":0},"protocol":"transaction","to":"active","final":"finished"}`,
		},
		{
			name: "lifecycle.Acquire/finals",
			id:   capability.LifecycleAcquire,
			kind: "lifecycle.acquire",
			label: lifecycle.Acquire{
				Target:   p1,
				Protocol: typestate.Protocol("transaction"),
				State:    typestate.State("active"),
				Obligation: typestate.Obligation{
					Finals: typestate.NewFinalStates(typestate.State("committed"), typestate.State("rolledback")),
				},
			},
			wire: `{"kind":"lifecycle.acquire","target":{"index":1},"protocol":"transaction","to":"active","finals":["committed","rolledback"]}`,
		},
		{
			name: "lifecycle.Transition",
			id:   capability.LifecycleTransition,
			kind: "lifecycle.transition",
			label: lifecycle.Transition{
				Target:   p0,
				Protocol: typestate.Protocol("transaction"),
				From:     typestate.State("active"),
				To:       typestate.State("finished"),
			},
			wire: `{"kind":"lifecycle.transition","target":{"index":0},"protocol":"transaction","from":"active","to":"finished"}`,
		},
		{
			name:  "lifecycle.Escape",
			id:    capability.LifecycleEscape,
			kind:  "lifecycle.escape",
			label: lifecycle.Escape{Target: p2, Protocol: typestate.Protocol("transaction")},
			wire:  `{"kind":"lifecycle.escape","target":{"index":2},"protocol":"transaction"}`,
		},
		{
			name:  "mutation.Mutate/unchanged",
			id:    capability.MutationMutate,
			kind:  "mutation.mutate",
			label: mutation.Mutate{Target: p0, Transform: mutation.Unchanged{}},
			wire:  `{"kind":"mutation.mutate","target":{"index":0},"transform":{"kind":"mutation.unchanged"}}`,
		},
		{
			name: "mutation.Mutate/element union with length",
			id:   capability.MutationMutate,
			kind: "mutation.mutate",
			label: mutation.Mutate{
				Target:      p1,
				Transform:   mutation.ElementUnion{Source: p2},
				LengthDelta: expr.C(1),
			},
			wire: `{"kind":"mutation.mutate","target":{"index":1},"transform":{"kind":"mutation.elementUnion","source":{"index":2}},"length":{"kind":"const","value":1}}`,
		},
		{
			name:  "mutation.LengthChange",
			id:    capability.MutationLengthChange,
			kind:  "mutation.lengthChange",
			label: mutation.LengthChange{Target: p0, Delta: 2},
			wire:  `{"kind":"mutation.lengthChange","delta":2,"target":{"index":0}}`,
		},
		{
			name:  "mutation.TableMutator",
			id:    capability.MutationTableMutator,
			kind:  "mutation.tableMutator",
			label: mutation.TableMutator{Target: p0, Value: p1},
			wire:  `{"kind":"mutation.tableMutator","target":{"index":0},"value":{"index":1}}`,
		},
		{
			name:  "ownership.Borrow",
			id:    capability.OwnershipBorrow,
			kind:  "ownership.borrow",
			label: ownership.Borrow{Param: p0},
			wire:  `{"kind":"ownership.borrow","param":{"index":0}}`,
		},
		{
			name:  "ownership.Retain",
			id:    capability.OwnershipRetain,
			kind:  "ownership.retain",
			label: ownership.Retain{Param: p1},
			wire:  `{"kind":"ownership.retain","param":{"index":1}}`,
		},
		{
			name:  "ownership.Store",
			id:    capability.OwnershipStore,
			kind:  "ownership.store",
			label: ownership.Store{Param: p0, Into: p1},
			wire:  `{"kind":"ownership.store","param":{"index":0},"into":{"index":1}}`,
		},
		{
			name:  "ownership.BorrowAll",
			id:    capability.OwnershipBorrowAll,
			kind:  "ownership.borrowAll",
			label: ownership.BorrowAll{},
			wire:  `{"kind":"ownership.borrowAll"}`,
		},
		{
			name:  "ownership.Send",
			id:    capability.OwnershipSend,
			kind:  "ownership.send",
			label: ownership.Send{FromParam: 1},
			wire:  `{"kind":"ownership.send","fromParam":1}`,
		},
		{
			name:  "ownership.SendParam",
			id:    capability.OwnershipSendParam,
			kind:  "ownership.sendParam",
			label: ownership.SendParam{Param: p2},
			wire:  `{"kind":"ownership.sendParam","param":{"index":2}}`,
		},
		{
			name:  "ownership.Export",
			id:    capability.OwnershipExport,
			kind:  "ownership.export",
			label: ownership.Export{Param: p0},
			wire:  `{"kind":"ownership.export","param":{"index":0}}`,
		},
		{
			name:  "ownership.Opaque",
			id:    capability.OwnershipOpaque,
			kind:  "ownership.opaque",
			label: ownership.Opaque{Param: p1},
			wire:  `{"kind":"ownership.opaque","param":{"index":1}}`,
		},
		{
			name:  "ownership.Freeze",
			id:    capability.OwnershipFreeze,
			kind:  "ownership.freeze",
			label: ownership.Freeze{Param: p2},
			wire:  `{"kind":"ownership.freeze","param":{"index":2}}`,
		},
		{
			name:  "postcondition.NormalReturnRefinement/present",
			id:    capability.PostconditionNormalReturnRefinement,
			kind:  postcondition.NormalReturnRefinementKind,
			label: postcondition.NormalReturnRefinement{Target: p0, Refinement: postcondition.Present{}},
			wire:  `{"kind":"postcondition.normalReturnRefinement","target":{"index":0},"refinement":{"kind":"present"}}`,
		},
		{
			name:  "postcondition.NormalReturnRefinement/absent",
			id:    capability.PostconditionNormalReturnRefinement,
			kind:  postcondition.NormalReturnRefinementKind,
			label: postcondition.NormalReturnRefinement{Target: p1, Refinement: postcondition.Absent{}},
			wire:  `{"kind":"postcondition.normalReturnRefinement","target":{"index":1},"refinement":{"kind":"absent"}}`,
		},
		{
			name:  "returns.ErrorReturn",
			id:    capability.ReturnsErrorReturn,
			kind:  "returns.errorReturn",
			label: returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
			wire:  `{"kind":"returns.errorReturn","valueIndex":0,"errorIndex":1}`,
		},
		{
			name:  "returns.Return.SameAs",
			id:    capability.ReturnsReturnSameAs,
			kind:  "returns.return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p1}},
			wire:  `{"kind":"returns.return","returnIndex":0,"returnType":{"kind":"returns.sameAs","source":{"index":1}}}`,
		},
		{
			name:  "returns.Return.ElementOf",
			id:    capability.ReturnsReturnElementOf,
			kind:  "returns.return",
			label: returns.Return{ReturnIndex: 1, Transform: returns.ElementOf{Source: p0}},
			wire:  `{"kind":"returns.return","returnIndex":1,"returnType":{"kind":"returns.elementOf","source":{"index":0}}}`,
		},
		{
			name:  "returns.Return.OptionalElementOf",
			id:    capability.ReturnsReturnOptionalElementOf,
			kind:  "returns.return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p2}},
			wire:  `{"kind":"returns.return","returnIndex":0,"returnType":{"kind":"returns.optionalElementOf","source":{"index":2}}}`,
		},
		{
			name:  "returns.Return.CallbackReturn",
			id:    capability.ReturnsReturnCallbackReturn,
			kind:  "returns.return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p1}},
			wire:  `{"kind":"returns.return","returnIndex":0,"returnType":{"kind":"returns.callbackReturn","callbackParam":{"index":1}}}`,
		},
		{
			name:  "returns.Return.ArrayOfCallbackReturn",
			id:    capability.ReturnsReturnArrayOfCallbackReturn,
			kind:  "returns.return",
			label: returns.Return{ReturnIndex: 2, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p0}},
			wire:  `{"kind":"returns.return","returnIndex":2,"returnType":{"kind":"returns.arrayOfCallbackReturn","callbackParam":{"index":0}}}`,
		},
		{
			name: "returns.Return.TypeProjection",
			id:   capability.ReturnsReturnTypeProjection,
			kind: "returns.return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
				Source: p0,
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("payload"),
					projection.CallableReturn(),
					projection.GenericArg(0),
					projection.InstantiateGeneric(typ.String),
				}},
			}},
			wire: `{"kind":"returns.return","returnIndex":0,"returnType":{"kind":"returns.typeProjection","source":{"index":0},"projection":[{"kind":"field","field":"payload"},{"kind":"callableReturn"},{"kind":"genericArg","index":0},{"kind":"instantiateGeneric","type":{"kind":"string"}}]}}`,
		},
		{
			name: "returns.Return.ConditionalType",
			id:   capability.ReturnsReturnConditionalType,
			kind: "returns.return",
			label: returns.Return{ReturnIndex: 1, Transform: returns.ConditionalType{
				Source: p1,
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("message"),
				}},
				When: typ.LiteralBool(true),
				Then: typ.String,
			}},
			wire: `{"kind":"returns.return","returnIndex":1,"returnType":{"kind":"returns.conditionalType","source":{"index":1},"projection":[{"kind":"field","field":"message"}],"when":{"kind":"literal","base":"boolean","bool":true},"then":{"kind":"string"}}}`,
		},
		{
			name:  "returns.ReturnLength",
			id:    capability.ReturnsReturnLength,
			kind:  "returns.returnLength",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.C(1)},
			wire:  `{"kind":"returns.returnLength","returnIndex":0,"length":{"kind":"const","value":1}}`,
		},
		{
			name:  "returns.CorrelatedReturn",
			id:    capability.ReturnsCorrelatedReturn,
			kind:  "returns.correlatedReturn",
			label: returns.CorrelatedReturn{Indices: []int{0, 2}},
			wire:  `{"kind":"returns.correlatedReturn","indices":[0,2]}`,
		},
	}
}

// effectLabelCaseIsCarriable reads the corpus entry's verdict off the audited
// catalog, so the laws below never restate which entries a manifest may carry.
func effectLabelCaseIsCarriable(t *testing.T, tc effectLabelWireCase) bool {
	t.Helper()
	desc, ok := capability.Lookup(tc.id)
	if !ok {
		t.Fatalf("corpus entry %s names capability %q that the catalog does not audit", tc.name, tc.id)
	}
	switch desc.Status {
	case capability.StatusReserved, capability.StatusReservedHighRisk:
		return false
	default:
		return true
	}
}

// TestEffectLabelStatusCarriageIsStated pins which audited statuses a manifest
// may carry, so a status added to the catalog is a verdict here rather than a
// silent admission at the boundary.
func TestEffectLabelStatusCarriageIsStated(t *testing.T) {
	carriage := map[capability.Status]bool{
		capability.StatusOperational:       true,
		capability.StatusImportOrStdlib:    true,
		capability.StatusPartial:           true,
		capability.StatusManifestValidated: true,
		capability.StatusReserved:          false,
		capability.StatusReservedHighRisk:  false,
	}
	seen := make(map[capability.Status]bool, len(carriage))
	for _, desc := range capability.All() {
		want, stated := carriage[desc.Status]
		if !stated {
			t.Fatalf("capability %s carries status %q that this law does not state", desc.ID, desc.Status)
		}
		seen[desc.Status] = true
		got := manifestCarriesEffectCapability(desc)
		if got != want {
			t.Fatalf("manifest carriage of status %q = %v, want %v", desc.Status, got, want)
		}
	}
	if !seen[capability.StatusManifestValidated] {
		t.Fatal("no audited capability carries the manifest-validated status")
	}
}

// TestEffectLabelWireTableCoversAuditedCatalog derives the table's coverage
// from the catalog in both directions: every audited capability has a row, a
// carriable one has the payload that carries it, and every row names a
// capability the catalog audits.
func TestEffectLabelWireTableCoversAuditedCatalog(t *testing.T) {
	for _, desc := range capability.All() {
		row, spelled := effectLabelWireRows[desc.ID]
		if !spelled {
			t.Fatalf("audited capability %s has no manifest wire row", desc.ID)
		}
		if row.kind == "" {
			t.Fatalf("manifest wire row for %s states no kind", desc.ID)
		}
		if manifestCarriesEffectCapability(desc) && row.payload == nil {
			t.Fatalf("manifest carries capability %s but its wire row states no payload", desc.ID)
		}
	}
	for id, row := range effectLabelWireRows {
		if _, audited := capability.Lookup(id); !audited {
			t.Fatalf("manifest wire row %q names a capability the catalog does not audit", id)
		}
		ids, known := effectLabelCapabilityIDsForWireKind(row.kind)
		if !known {
			t.Fatalf("wire kind %q written for %s is not readable", row.kind, id)
		}
		payloads := make(map[*effectLabelPayload]bool)
		for _, kindID := range ids {
			payloads[effectLabelWireRows[kindID].payload] = true
		}
		delete(payloads, nil)
		if len(payloads) > 1 {
			t.Fatalf("wire kind %q is shared by capabilities %v that read different payloads", row.kind, ids)
		}
	}
}

// TestEffectLabelCorpusCoversAuditedCatalog derives coverage from the audited
// catalog: an audited capability with no corpus entry is unproven at the
// boundary and says so here.
func TestEffectLabelCorpusCoversAuditedCatalog(t *testing.T) {
	covered := make(map[string]bool)
	for _, tc := range effectLabelWireCorpus() {
		if _, ok := capability.Lookup(tc.id); !ok {
			t.Fatalf("corpus entry %s names capability %q that the catalog does not audit", tc.name, tc.id)
		}
		covered[tc.id] = true
	}
	for _, desc := range capability.All() {
		if !covered[desc.ID] {
			t.Fatalf("audited capability %s has no effect label corpus entry", desc.ID)
		}
	}
}

// TestEffectLabelBoundaryStatesEveryAuditedCapability is the totality law: for
// every audited capability the boundary has a stated verdict. A carriable one
// serializes to its recorded bytes under its recorded kind; a barred one is
// refused by the catalog's status. Nothing audited is "unsupported" or
// "unaudited" on the write side.
func TestEffectLabelBoundaryStatesEveryAuditedCapability(t *testing.T) {
	for _, tc := range effectLabelWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := encodeEffectLabel(tc.label)
			if effectLabelCaseIsCarriable(t, tc) {
				if err != nil {
					t.Fatalf("encodeEffectLabel(%v): %v", tc.label, err)
				}
				if wire.Kind != tc.kind {
					t.Fatalf("wire kind = %q, want %q", wire.Kind, tc.kind)
				}
				data, err := json.Marshal(wire)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				if string(data) != tc.wire {
					t.Fatalf("wire bytes = %s, want %s", data, tc.wire)
				}
				return
			}
			if err == nil {
				t.Fatalf("encodeEffectLabel(%v) wrote %+v, want refusal by capability status", tc.label, wire)
			}
			wantInactive := "inactive effect label " + tc.id
			if !strings.Contains(err.Error(), wantInactive) {
				t.Fatalf("encodeEffectLabel error = %v, want refusal naming %q", err, wantInactive)
			}
		})
	}
}

// TestEffectLabelRoundTripsThroughItsOwnBytes states the read commitment: the
// recorded bytes of a carriable label parse back into that label and rewrite to
// the same bytes.
func TestEffectLabelRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range effectLabelWireCorpus() {
		if !effectLabelCaseIsCarriable(t, tc) {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			var read effectLabelWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			row, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{read}})
			if err != nil {
				t.Fatalf("decodeEffectRow: %v", err)
			}
			if len(row.Labels) != 1 {
				t.Fatalf("decoded %d labels, want 1", len(row.Labels))
			}
			decoded := row.Labels[0]
			if !decoded.Equals(effect.NormalizeLabel(tc.label)) {
				t.Fatalf("decoded label = %v, want %v", decoded, tc.label)
			}
			rewritten, err := encodeEffectLabel(decoded)
			if err != nil {
				t.Fatalf("encodeEffectLabel(decoded): %v", err)
			}
			data, err := json.Marshal(rewritten)
			if err != nil {
				t.Fatalf("marshal rewritten: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("rewritten bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestEffectLabelReadSideRefusesByCapabilityStatus is the read-side half of the
// same statement: a payload naming a barred capability's kind is refused by the
// catalog's status, naming that capability, and a carriable one is admitted.
func TestEffectLabelReadSideRefusesByCapabilityStatus(t *testing.T) {
	for _, tc := range effectLabelWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read effectLabelWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{read}})
			if effectLabelCaseIsCarriable(t, tc) {
				if err != nil {
					t.Fatalf("decodeEffectRow: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("decodeEffectRow admitted a barred capability")
			}
			wantInactive := "inactive effect label " + tc.id
			if !strings.Contains(err.Error(), wantInactive) {
				t.Fatalf("decodeEffectRow error = %v, want refusal naming %q", err, wantInactive)
			}
		})
	}
}

// TestEffectLabelWireKindsAreOneVocabulary states that the kind a label is
// written under is the kind the read side resolves it by, for every audited
// capability, so neither direction can hold a spelling the other does not.
func TestEffectLabelWireKindsAreOneVocabulary(t *testing.T) {
	for _, tc := range effectLabelWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			kind, ok := effectLabelWireKindFor(tc.id)
			if !ok {
				t.Fatalf("audited capability %s has no manifest wire kind", tc.id)
			}
			if kind != tc.kind {
				t.Fatalf("wire kind for %s = %q, want %q", tc.id, kind, tc.kind)
			}
			ids, ok := effectLabelCapabilityIDsForWireKind(kind)
			if !ok {
				t.Fatalf("wire kind %q resolves to no audited capability", kind)
			}
			found := false
			for _, id := range ids {
				if id == tc.id {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("wire kind %q resolves to %v, which excludes %s", kind, ids, tc.id)
			}
		})
	}
}

// TestEffectLabelUnknownWireKindIsRefused states that a kind outside the
// vocabulary is unknown to the boundary by construction.
func TestEffectLabelUnknownWireKindIsRefused(t *testing.T) {
	_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{{Kind: "ownership.teleport"}}})
	if err == nil {
		t.Fatal("decodeEffectRow admitted a kind outside the vocabulary")
	}
	if !strings.Contains(err.Error(), "unknown effect label kind") {
		t.Fatalf("decodeEffectRow error = %v, want unknown-kind refusal", err)
	}
}
