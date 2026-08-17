package wire

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/domain/effect/capability/label"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/iteration"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/typestate"
)

// The effect vocabulary is the capability catalog's, and which entries a
// manifest may carry is that catalog's status. What this package owns is the
// boundary spelling of those entries: the wire kind written for each audited
// capability and the wire fields that kind carries. That spelling lives in the
// table below and nowhere else, so the write side and the read side consult one
// statement instead of two hand-kept switches.
//
// The kind is stated per row rather than derived from the capability ID. Two
// rows make a derivation impossible: control.IO is spelled "control.io" rather
// than a mechanical lowering of "IO", and the seven returns.Return.* transform
// capabilities all share the single kind "returns.return", which resolves to
// one of them only after the transform payload is read.

// effectLabelPayload is the wire fields a kind carries, in both directions. It
// is the field applicability rule of the wire struct, stated once beside the
// vocabulary: the write side fills exactly these fields and the read side reads
// exactly them, so neither can quietly carry a field the other ignores. The
// kind itself is never written here; the row stamps it.
type effectLabelPayload struct {
	write func(effect.Label) (effectLabelWire, error)
	read  func(effectLabelWire) (effect.Label, error)
}

// effectLabelWireRow is one audited capability's boundary spelling. A row with
// no payload states a kind the boundary recognizes but carries no fields for,
// which is only reachable for a capability the catalog bars from manifests.
type effectLabelWireRow struct {
	kind    string
	payload *effectLabelPayload
}

// effectLabelWireRows is the boundary vocabulary, one row per audited
// capability ID.
var effectLabelWireRows = map[string]effectLabelWireRow{
	capability.ControlThrow: {kind: "control.throw"},
	capability.ControlIO:    {kind: "control.io"},

	capability.DispatchModuleLoad: {kind: "dispatch.moduleLoad", payload: dispatchModuleLoadPayload},

	capability.IterationIterator: {kind: "iteration.iterator", payload: iterationIteratorPayload},

	capability.LifecycleAcquire:    {kind: "lifecycle.acquire", payload: lifecycleAcquirePayload},
	capability.LifecycleTransition: {kind: "lifecycle.transition", payload: lifecycleTransitionPayload},
	capability.LifecycleEscape:     {kind: "lifecycle.escape", payload: lifecycleEscapePayload},

	capability.MutationMutate:       {kind: "mutation.mutate", payload: mutationMutatePayload},
	capability.MutationLengthChange: {kind: "mutation.lengthChange", payload: mutationLengthChangePayload},
	capability.MutationTableMutator: {kind: "mutation.tableMutator", payload: mutationTableMutatorPayload},

	capability.OwnershipBorrow:    {kind: "ownership.borrow", payload: ownershipBorrowPayload},
	capability.OwnershipRetain:    {kind: "ownership.retain", payload: ownershipRetainPayload},
	capability.OwnershipStore:     {kind: "ownership.store", payload: ownershipStorePayload},
	capability.OwnershipBorrowAll: {kind: "ownership.borrowAll", payload: ownershipBorrowAllPayload},
	capability.OwnershipSend:      {kind: "ownership.send", payload: ownershipSendPayload},
	capability.OwnershipSendParam: {kind: "ownership.sendParam", payload: ownershipSendParamPayload},
	capability.OwnershipExport:    {kind: "ownership.export", payload: ownershipExportPayload},
	capability.OwnershipOpaque:    {kind: "ownership.opaque", payload: ownershipOpaquePayload},
	capability.OwnershipFreeze:    {kind: "ownership.freeze", payload: ownershipFreezePayload},

	capability.PostconditionNormalReturnRefinement: {
		kind:    postcondition.NormalReturnRefinementKind,
		payload: postconditionNormalReturnRefinementPayload,
	},

	capability.ReturnsErrorReturn: {kind: "returns.errorReturn", payload: returnsErrorReturnPayload},

	capability.ReturnsReturnSameAs:                {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnElementOf:             {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnOptionalElementOf:     {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnCallbackReturn:        {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnArrayOfCallbackReturn: {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnTypeProjection:        {kind: "returns.return", payload: returnsReturnPayload},
	capability.ReturnsReturnConditionalType:       {kind: "returns.return", payload: returnsReturnPayload},

	capability.ReturnsReturnLength:     {kind: "returns.returnLength"},
	capability.ReturnsCorrelatedReturn: {kind: "returns.correlatedReturn"},
}

// effectLabelWireRowsByKind is the read side's index into the same rows. A kind
// the vocabulary does not spell is unknown to the boundary by construction, and
// a kind several capabilities share lists them in a stable order so the verdict
// a shared kind produces does not depend on map iteration.
var effectLabelWireRowsByKind = func() map[string][]string {
	byKind := make(map[string][]string, len(effectLabelWireRows))
	for id, row := range effectLabelWireRows {
		byKind[row.kind] = append(byKind[row.kind], id)
	}
	for _, ids := range byKind {
		sort.Strings(ids)
	}
	return byKind
}()

// manifestCarriesEffectCapability is the catalog's carriage verdict, read off
// descriptor status. Both directions of the codec ask this one question.
func manifestCarriesEffectCapability(desc capability.Descriptor) bool {
	switch desc.Status {
	case capability.StatusReserved, capability.StatusReservedHighRisk:
		return false
	default:
		return true
	}
}

// effectLabelWireKindFor returns the kind an audited capability is spelled as.
func effectLabelWireKindFor(id string) (string, bool) {
	row, ok := effectLabelWireRows[id]
	if !ok {
		return "", false
	}
	return row.kind, true
}

// effectLabelCapabilityIDsForWireKind returns the audited capabilities a wire
// kind can carry.
func effectLabelCapabilityIDsForWireKind(kind string) ([]string, bool) {
	ids, ok := effectLabelWireRowsByKind[kind]
	return ids, ok
}

func encodeEffectRow(row effect.Row) (*effectRowWire, error) {
	if row.Pure() {
		return nil, nil
	}

	out := &effectRowWire{Labels: make([]effectLabelWire, 0, len(row.Labels))}
	for _, label := range row.Labels {
		encoded, err := encodeEffectLabel(label)
		if err != nil {
			return nil, err
		}
		out.Labels = append(out.Labels, encoded)
	}
	sort.Slice(out.Labels, func(i, j int) bool {
		return effectLabelWireKey(out.Labels[i]) < effectLabelWireKey(out.Labels[j])
	})
	if row.Tail != nil {
		tail := row.Tail.Name
		out.Tail = &tail
	}
	return out, nil
}

func decodeEffectRow(w *effectRowWire) (effect.Row, error) {
	if w == nil {
		return effect.Empty, nil
	}

	labels := make([]effect.Label, 0, len(w.Labels))
	for _, label := range w.Labels {
		decoded, err := decodeEffectLabel(label)
		if err != nil {
			return effect.Row{}, err
		}
		labels = append(labels, decoded)
	}
	var tail *effect.Var
	if w.Tail != nil {
		tail = &effect.Var{Name: *w.Tail}
	}
	return effect.Row{Labels: labels, Tail: tail}, nil
}

func inactiveManifestEffectLabelError(desc capability.Descriptor) error {
	return fmt.Errorf("signature/wire: inactive effect label %s (%s)", desc.ID, desc.Status)
}

func encodeEffectLabel(label effect.Label) (effectLabelWire, error) {
	label = effect.NormalizeLabel(label)
	_, row, err := effectLabelWireRowFor(label)
	if err != nil {
		return effectLabelWire{}, err
	}
	wire, err := row.payload.write(label)
	if err != nil {
		return effectLabelWire{}, err
	}
	wire.Kind = row.kind
	return wire, nil
}

func decodeEffectLabel(w effectLabelWire) (effect.Label, error) {
	payload, err := effectLabelWirePayloadFor(w.Kind)
	if err != nil {
		return nil, err
	}
	label, err := payload.read(w)
	if err != nil {
		return nil, err
	}
	// The rebuilt label's own audited identity decides the carriage verdict, so a
	// kind several capabilities share lands on the one the payload actually read.
	id, row, err := effectLabelWireRowFor(label)
	if err != nil {
		return nil, err
	}
	if row.kind != w.Kind {
		return nil, fmt.Errorf("signature/wire: effect label kind %q read back as %s", w.Kind, id)
	}
	return label, nil
}

// effectLabelWireRowFor answers a label with the row that spells it, refusing a
// label the catalog does not audit, states no spelling for, or bars from
// manifests. It is the one carriage verdict both directions consult.
func effectLabelWireRowFor(label effect.Label) (string, effectLabelWireRow, error) {
	id, audited := caplabel.IDFor(label)
	if !audited {
		return "", effectLabelWireRow{}, fmt.Errorf("signature/wire: unaudited effect label %T", label)
	}
	desc, known := capability.Lookup(id)
	if !known {
		return id, effectLabelWireRow{}, fmt.Errorf("signature/wire: unaudited effect label %s", id)
	}
	row, spelled := effectLabelWireRows[id]
	if !spelled {
		return id, effectLabelWireRow{}, fmt.Errorf("signature/wire: effect label %s has no wire spelling", id)
	}
	if !manifestCarriesEffectCapability(desc) {
		return id, effectLabelWireRow{}, inactiveManifestEffectLabelError(desc)
	}
	if row.payload == nil {
		return id, effectLabelWireRow{}, fmt.Errorf("signature/wire: effect label %s has no wire payload", id)
	}
	return id, row, nil
}

// effectLabelWirePayloadFor answers a wire kind with the payload that reads it,
// refusing a kind whose every capability the catalog bars from manifests before
// any field of it is interpreted.
func effectLabelWirePayloadFor(kind string) (*effectLabelPayload, error) {
	ids, known := effectLabelWireRowsByKind[kind]
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown effect label kind %q", kind)
	}
	var barred *capability.Descriptor
	for _, id := range ids {
		desc, audited := capability.Lookup(id)
		if !audited {
			return nil, fmt.Errorf("signature/wire: unaudited effect label %s", id)
		}
		if !manifestCarriesEffectCapability(desc) {
			if barred == nil {
				refused := desc
				barred = &refused
			}
			continue
		}
		row := effectLabelWireRows[id]
		if row.payload == nil {
			return nil, fmt.Errorf("signature/wire: effect label %s has no wire payload", id)
		}
		return row.payload, nil
	}
	return nil, inactiveManifestEffectLabelError(*barred)
}

// effectLabelPayloadFor binds a payload to the label type it carries, so a row
// states its Go term once and the boundary never re-derives it from a type
// switch.
func effectLabelPayloadFor[T effect.Label](
	write func(T) (effectLabelWire, error),
	read func(effectLabelWire) (effect.Label, error),
) *effectLabelPayload {
	return &effectLabelPayload{
		write: func(label effect.Label) (effectLabelWire, error) {
			typed, ok := label.(T)
			if !ok {
				return effectLabelWire{}, fmt.Errorf("signature/wire: effect label %T does not carry its stated wire payload", label)
			}
			return write(typed)
		},
		read: read,
	}
}

// singleParamEffectLabelPayload is the shape shared by the ownership labels that
// carry exactly one param reference.
func singleParamEffectLabelPayload[T effect.Label](
	param func(T) effect.ParamRef,
	build func(effect.ParamRef) effect.Label,
	missing string,
) *effectLabelPayload {
	return effectLabelPayloadFor(
		func(label T) (effectLabelWire, error) {
			return effectLabelWire{Param: encodeParamRef(param(label))}, nil
		},
		func(w effectLabelWire) (effect.Label, error) {
			ref, err := decodeRequiredParamRef(w.Param, missing)
			if err != nil {
				return nil, err
			}
			return build(ref), nil
		},
	)
}

var dispatchModuleLoadPayload = effectLabelPayloadFor(
	func(dispatch.ModuleLoad) (effectLabelWire, error) { return effectLabelWire{}, nil },
	func(effectLabelWire) (effect.Label, error) { return dispatch.ModuleLoad{}, nil },
)

var iterationIteratorPayload = effectLabelPayloadFor(
	func(l iteration.Iterator) (effectLabelWire, error) {
		kind, err := encodeIteratorKind(l.Kind)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Source: encodeParamRef(l.Source), IteratorKind: kind}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		kind, err := decodeIteratorKind(w.IteratorKind)
		if err != nil {
			return nil, err
		}
		source, err := decodeRequiredParamRef(w.Source, "iterator source missing param ref")
		if err != nil {
			return nil, err
		}
		return iteration.Iterator{Source: source, Kind: kind}, nil
	},
)

var lifecycleAcquirePayload = effectLabelPayloadFor(
	func(l lifecycle.Acquire) (effectLabelWire, error) {
		protocol, err := encodeLifecycleProtocol(l.Protocol, "signature/wire: lifecycle acquire missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		to, err := encodeRequiredLifecycleState(l.State, "signature/wire: lifecycle acquire missing state")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Target:   encodeParamRef(l.Target),
			Protocol: protocol,
			To:       to,
			Final:    encodeOptionalLifecycleState(l.Obligation.Final),
			Finals:   encodeOptionalLifecycleFinalStates(l.Obligation.Finals),
		}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		protocol, err := decodeLifecycleProtocol(w.Protocol, "signature/wire: lifecycle acquire missing protocol")
		if err != nil {
			return nil, err
		}
		to, err := decodeRequiredLifecycleState(w.To, "signature/wire: lifecycle acquire missing state")
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "lifecycle acquire target missing param ref")
		if err != nil {
			return nil, err
		}
		finals, err := decodeOptionalLifecycleFinalStates(w.Finals)
		if err != nil {
			return nil, err
		}
		return lifecycle.Acquire{
			Target:   target,
			Protocol: protocol,
			State:    to,
			Obligation: typestate.Obligation{
				Final:  decodeOptionalLifecycleState(w.Final),
				Finals: finals,
			},
		}, nil
	},
)

var lifecycleTransitionPayload = effectLabelPayloadFor(
	func(l lifecycle.Transition) (effectLabelWire, error) {
		protocol, err := encodeLifecycleProtocol(l.Protocol, "signature/wire: lifecycle transition missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		to, err := encodeRequiredLifecycleState(l.To, "signature/wire: lifecycle transition missing target state")
		if err != nil {
			return effectLabelWire{}, err
		}
		from, err := encodeRequiredLifecycleState(l.From, "signature/wire: lifecycle transition missing source state")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Target:   encodeParamRef(l.Target),
			Protocol: protocol,
			From:     from,
			To:       to,
		}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		protocol, err := decodeLifecycleProtocol(w.Protocol, "signature/wire: lifecycle transition missing protocol")
		if err != nil {
			return nil, err
		}
		to, err := decodeRequiredLifecycleState(w.To, "signature/wire: lifecycle transition missing target state")
		if err != nil {
			return nil, err
		}
		from, err := decodeRequiredLifecycleState(w.From, "signature/wire: lifecycle transition missing source state")
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "lifecycle transition target missing param ref")
		if err != nil {
			return nil, err
		}
		return lifecycle.Transition{
			Target:   target,
			Protocol: protocol,
			From:     from,
			To:       to,
		}, nil
	},
)

var lifecycleEscapePayload = effectLabelPayloadFor(
	func(l lifecycle.Escape) (effectLabelWire, error) {
		protocol, err := encodeLifecycleProtocol(l.Protocol, "signature/wire: lifecycle escape missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Target: encodeParamRef(l.Target), Protocol: protocol}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		protocol, err := decodeLifecycleProtocol(w.Protocol, "signature/wire: lifecycle escape missing protocol")
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "lifecycle escape target missing param ref")
		if err != nil {
			return nil, err
		}
		return lifecycle.Escape{Target: target, Protocol: protocol}, nil
	},
)

var mutationMutatePayload = effectLabelPayloadFor(
	func(l mutation.Mutate) (effectLabelWire, error) {
		transform, err := encodeEffectTransform(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		length, err := encodeExpr(l.LengthDelta)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Target:    encodeParamRef(l.Target),
			Transform: transform,
			Length:    length,
		}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		transform, err := decodeEffectTransform(w.Transform)
		if err != nil {
			return nil, err
		}
		length, err := decodeExpr(w.Length)
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "mutation target missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.Mutate{Target: target, Transform: transform, LengthDelta: length}, nil
	},
)

var mutationLengthChangePayload = effectLabelPayloadFor(
	func(l mutation.LengthChange) (effectLabelWire, error) {
		return effectLabelWire{Target: encodeParamRef(l.Target), Delta: encodeInt(l.Delta)}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		target, err := decodeRequiredParamRef(w.Target, "length change target missing param ref")
		if err != nil {
			return nil, err
		}
		delta, err := decodeRequiredInt(w.Delta, "length change delta missing")
		if err != nil {
			return nil, err
		}
		return mutation.LengthChange{Target: target, Delta: delta}, nil
	},
)

var mutationTableMutatorPayload = effectLabelPayloadFor(
	func(l mutation.TableMutator) (effectLabelWire, error) {
		return effectLabelWire{Target: encodeParamRef(l.Target), Value: encodeParamRef(l.Value)}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		target, err := decodeRequiredParamRef(w.Target, "table mutator target missing param ref")
		if err != nil {
			return nil, err
		}
		value, err := decodeRequiredParamRef(w.Value, "table mutator value missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.TableMutator{Target: target, Value: value}, nil
	},
)

var ownershipBorrowPayload = singleParamEffectLabelPayload(
	func(l ownership.Borrow) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.Borrow{Param: ref} },
	"borrow param missing param ref",
)

var ownershipRetainPayload = singleParamEffectLabelPayload(
	func(l ownership.Retain) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.Retain{Param: ref} },
	"retain param missing param ref",
)

var ownershipSendParamPayload = singleParamEffectLabelPayload(
	func(l ownership.SendParam) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.SendParam{Param: ref} },
	"send param missing param ref",
)

var ownershipExportPayload = singleParamEffectLabelPayload(
	func(l ownership.Export) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.Export{Param: ref} },
	"export param missing param ref",
)

var ownershipOpaquePayload = singleParamEffectLabelPayload(
	func(l ownership.Opaque) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.Opaque{Param: ref} },
	"opaque param missing param ref",
)

var ownershipFreezePayload = singleParamEffectLabelPayload(
	func(l ownership.Freeze) effect.ParamRef { return l.Param },
	func(ref effect.ParamRef) effect.Label { return ownership.Freeze{Param: ref} },
	"freeze param missing param ref",
)

var ownershipStorePayload = effectLabelPayloadFor(
	func(l ownership.Store) (effectLabelWire, error) {
		return effectLabelWire{Param: encodeParamRef(l.Param), Into: encodeParamRef(l.Into)}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		param, err := decodeRequiredParamRef(w.Param, "store param missing param ref")
		if err != nil {
			return nil, err
		}
		into, err := decodeRequiredParamRef(w.Into, "store target missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Store{Param: param, Into: into}, nil
	},
)

var ownershipBorrowAllPayload = effectLabelPayloadFor(
	func(ownership.BorrowAll) (effectLabelWire, error) { return effectLabelWire{}, nil },
	func(effectLabelWire) (effect.Label, error) { return ownership.BorrowAll{}, nil },
)

var ownershipSendPayload = effectLabelPayloadFor(
	func(l ownership.Send) (effectLabelWire, error) {
		fromParam := l.FromParam
		return effectLabelWire{FromParam: &fromParam}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		if w.FromParam == nil {
			return nil, fmt.Errorf("signature/wire: send fromParam missing")
		}
		return ownership.Send{FromParam: *w.FromParam}, nil
	},
)

var postconditionNormalReturnRefinementPayload = effectLabelPayloadFor(
	func(l postcondition.NormalReturnRefinement) (effectLabelWire, error) {
		refinement, err := encodeEffectRefinement(l.Refinement)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Target: encodeParamRef(l.Target), Refinement: refinement}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		refinement, err := decodeEffectRefinement(w.Refinement)
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "normal return refinement target missing param ref")
		if err != nil {
			return nil, err
		}
		return postcondition.NormalReturnRefinement{Target: target, Refinement: refinement}, nil
	},
)

var returnsReturnPayload = effectLabelPayloadFor(
	func(l returns.Return) (effectLabelWire, error) {
		transform, err := encodeEffectReturn(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{ReturnIndex: encodeInt(l.ReturnIndex), ReturnType: transform}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		transform, err := decodeEffectReturn(w.ReturnType)
		if err != nil {
			return nil, err
		}
		returnIndex, err := decodeRequiredInt(w.ReturnIndex, "return index missing")
		if err != nil {
			return nil, err
		}
		return returns.Return{ReturnIndex: returnIndex, Transform: transform}, nil
	},
)

var returnsErrorReturnPayload = effectLabelPayloadFor(
	func(l returns.ErrorReturn) (effectLabelWire, error) {
		return effectLabelWire{ValueIndex: encodeInt(l.ValueIndex), ErrorIndex: encodeInt(l.ErrorIndex)}, nil
	},
	func(w effectLabelWire) (effect.Label, error) {
		valueIndex, err := decodeRequiredInt(w.ValueIndex, "error return value index missing")
		if err != nil {
			return nil, err
		}
		errorIndex, err := decodeRequiredInt(w.ErrorIndex, "error return error index missing")
		if err != nil {
			return nil, err
		}
		return returns.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex}, nil
	},
)
