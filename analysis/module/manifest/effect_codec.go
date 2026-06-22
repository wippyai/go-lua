package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func encodeEffectRow(row effect.Row) (*effectRowWire, error) {
	if row.Pure() {
		return nil, nil
	}

	out := &effectRowWire{Labels: make([]effectLabelWire, 0, len(row.Labels))}
	for _, label := range row.Labels {
		if err := validateManifestEffectLabel(label); err != nil {
			return nil, err
		}
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
		if err := validateManifestEffectLabel(decoded); err != nil {
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

func validateManifestEffectLabel(label effect.Label) error {
	desc, ok := caplabel.DescriptorFor(label)
	if !ok {
		return fmt.Errorf("manifest: unaudited effect label %T", label)
	}
	switch desc.Status {
	case capability.StatusReserved, capability.StatusReservedHighRisk:
		return inactiveManifestEffectLabelError(desc)
	default:
		return nil
	}
}

func inactiveManifestEffectLabel(label effect.Label) error {
	desc, ok := caplabel.DescriptorFor(label)
	if !ok {
		return fmt.Errorf("manifest: unaudited effect label %T", label)
	}
	return inactiveManifestEffectLabelError(desc)
}

func inactiveManifestEffectID(id string) error {
	desc, ok := capability.Lookup(id)
	if !ok {
		return fmt.Errorf("manifest: unaudited effect label %s", id)
	}
	return inactiveManifestEffectLabelError(desc)
}

func inactiveManifestEffectLabelError(desc capability.Descriptor) error {
	return fmt.Errorf("manifest: inactive effect label %s (%s)", desc.ID, desc.Status)
}

func encodeEffectLabel(label effect.Label) (effectLabelWire, error) {
	label = effect.NormalizeLabel(label)
	switch l := label.(type) {
	case dispatch.ModuleLoad:
		return effectLabelWire{Kind: "dispatch.moduleLoad"}, nil
	case iteration.Iterator:
		kind, err := encodeIteratorKind(l.Kind)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:         "iteration.iterator",
			Source:       encodeParamRef(l.Source),
			IteratorKind: kind,
		}, nil
	case mutation.Mutate:
		transform, err := encodeEffectTransform(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		length, err := encodeExpr(l.LengthDelta)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:      "mutation.mutate",
			Target:    encodeParamRef(l.Target),
			Transform: transform,
			Length:    length,
		}, nil
	case mutation.LengthChange:
		return effectLabelWire{
			Kind:   "mutation.lengthChange",
			Target: encodeParamRef(l.Target),
			Delta:  encodeInt(l.Delta),
		}, nil
	case mutation.TableMutator:
		return effectLabelWire{
			Kind:   "mutation.tableMutator",
			Target: encodeParamRef(l.Target),
			Value:  encodeParamRef(l.Value),
		}, nil
	case lifecycle.Acquire:
		protocol, err := encodeLifecycleProtocol(l.Protocol, "manifest: lifecycle acquire missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		to, err := encodeRequiredLifecycleState(l.State, "manifest: lifecycle acquire missing state")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:     "lifecycle.acquire",
			Target:   encodeParamRef(l.Target),
			Protocol: protocol,
			To:       to,
			Final:    encodeOptionalLifecycleState(l.Obligation.Final),
		}, nil
	case lifecycle.Transition:
		protocol, err := encodeLifecycleProtocol(l.Protocol, "manifest: lifecycle transition missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		to, err := encodeRequiredLifecycleState(l.To, "manifest: lifecycle transition missing target state")
		if err != nil {
			return effectLabelWire{}, err
		}
		from, err := encodeRequiredLifecycleState(l.From, "manifest: lifecycle transition missing source state")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:     "lifecycle.transition",
			Target:   encodeParamRef(l.Target),
			Protocol: protocol,
			From:     from,
			To:       to,
		}, nil
	case lifecycle.Escape:
		protocol, err := encodeLifecycleProtocol(l.Protocol, "manifest: lifecycle escape missing protocol")
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:     "lifecycle.escape",
			Target:   encodeParamRef(l.Target),
			Protocol: protocol,
		}, nil
	case ownership.Borrow:
		return effectLabelWire{Kind: "ownership.borrow", Param: encodeParamRef(l.Param)}, nil
	case ownership.Retain:
		return effectLabelWire{Kind: "ownership.retain", Param: encodeParamRef(l.Param)}, nil
	case ownership.Store:
		return effectLabelWire{Kind: "ownership.store", Param: encodeParamRef(l.Param), Into: encodeParamRef(l.Into)}, nil
	case ownership.BorrowAll:
		return effectLabelWire{Kind: "ownership.borrowAll"}, nil
	case ownership.Send:
		fromParam := l.FromParam
		return effectLabelWire{Kind: "ownership.send", FromParam: &fromParam}, nil
	case ownership.SendParam:
		return effectLabelWire{Kind: "ownership.sendParam", Param: encodeParamRef(l.Param)}, nil
	case ownership.Export:
		return effectLabelWire{Kind: "ownership.export", Param: encodeParamRef(l.Param)}, nil
	case ownership.Opaque:
		return effectLabelWire{Kind: "ownership.opaque", Param: encodeParamRef(l.Param)}, nil
	case ownership.Freeze:
		return effectLabelWire{Kind: "ownership.freeze", Param: encodeParamRef(l.Param)}, nil
	case postcondition.NormalReturnRefinement:
		refinement, err := encodeEffectRefinement(l.Refinement)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{
			Kind:       postcondition.NormalReturnRefinementKind,
			Target:     encodeParamRef(l.Target),
			Refinement: refinement,
		}, nil
	case returns.Return:
		transform, err := encodeEffectReturn(l.Transform)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Kind: "returns.return", ReturnIndex: encodeInt(l.ReturnIndex), ReturnType: transform}, nil
	case returns.ErrorReturn:
		return effectLabelWire{Kind: "returns.errorReturn", ValueIndex: encodeInt(l.ValueIndex), ErrorIndex: encodeInt(l.ErrorIndex)}, nil
	default:
		return effectLabelWire{}, fmt.Errorf("manifest: unsupported effect label %T", label)
	}
}

func decodeEffectLabel(w effectLabelWire) (effect.Label, error) {
	switch w.Kind {
	case "control.throw":
		return nil, inactiveManifestEffectID(capability.ControlThrow)
	case "control.io":
		return nil, inactiveManifestEffectID(capability.ControlIO)
	case "dispatch.moduleLoad":
		return dispatch.ModuleLoad{}, nil
	case "dispatch.variadicTransform":
		return nil, inactiveManifestEffectID(capability.DispatchVariadicTransform)
	case "dispatch.typePredicate":
		return nil, inactiveManifestEffectID(capability.DispatchTypePredicate)
	case "iteration.iterator":
		kind, err := decodeIteratorKind(w.IteratorKind)
		if err != nil {
			return nil, err
		}
		source, err := decodeRequiredParamRef(w.Source, "iterator source missing param ref")
		if err != nil {
			return nil, err
		}
		return iteration.Iterator{Source: source, Kind: kind}, nil
	case "mutation.mutate":
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
	case "mutation.lengthChange":
		target, err := decodeRequiredParamRef(w.Target, "length change target missing param ref")
		if err != nil {
			return nil, err
		}
		delta, err := decodeRequiredInt(w.Delta, "length change delta missing")
		if err != nil {
			return nil, err
		}
		return mutation.LengthChange{Target: target, Delta: delta}, nil
	case "mutation.tableMutator":
		target, err := decodeRequiredParamRef(w.Target, "table mutator target missing param ref")
		if err != nil {
			return nil, err
		}
		value, err := decodeRequiredParamRef(w.Value, "table mutator value missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.TableMutator{Target: target, Value: value}, nil
	case "lifecycle.acquire":
		protocol, err := decodeLifecycleProtocol(w.Protocol, "manifest: lifecycle acquire missing protocol")
		if err != nil {
			return nil, err
		}
		to, err := decodeRequiredLifecycleState(w.To, "manifest: lifecycle acquire missing state")
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "lifecycle acquire target missing param ref")
		if err != nil {
			return nil, err
		}
		return lifecycle.Acquire{
			Target:   target,
			Protocol: protocol,
			State:    to,
			Obligation: typestate.Obligation{
				Final: decodeOptionalLifecycleState(w.Final),
			},
		}, nil
	case "lifecycle.transition":
		protocol, err := decodeLifecycleProtocol(w.Protocol, "manifest: lifecycle transition missing protocol")
		if err != nil {
			return nil, err
		}
		to, err := decodeRequiredLifecycleState(w.To, "manifest: lifecycle transition missing target state")
		if err != nil {
			return nil, err
		}
		from, err := decodeRequiredLifecycleState(w.From, "manifest: lifecycle transition missing source state")
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
	case "lifecycle.escape":
		protocol, err := decodeLifecycleProtocol(w.Protocol, "manifest: lifecycle escape missing protocol")
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "lifecycle escape target missing param ref")
		if err != nil {
			return nil, err
		}
		return lifecycle.Escape{
			Target:   target,
			Protocol: protocol,
		}, nil
	case "ownership.borrow":
		param, err := decodeRequiredParamRef(w.Param, "borrow param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Borrow{Param: param}, nil
	case "ownership.retain":
		param, err := decodeRequiredParamRef(w.Param, "retain param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Retain{Param: param}, nil
	case "ownership.store":
		param, err := decodeRequiredParamRef(w.Param, "store param missing param ref")
		if err != nil {
			return nil, err
		}
		into, err := decodeRequiredParamRef(w.Into, "store target missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Store{Param: param, Into: into}, nil
	case "ownership.borrowAll":
		return ownership.BorrowAll{}, nil
	case "ownership.send":
		if w.FromParam == nil {
			return nil, fmt.Errorf("manifest: send fromParam missing")
		}
		return ownership.Send{FromParam: *w.FromParam}, nil
	case "ownership.sendParam":
		param, err := decodeRequiredParamRef(w.Param, "send param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.SendParam{Param: param}, nil
	case "ownership.export":
		param, err := decodeRequiredParamRef(w.Param, "export param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Export{Param: param}, nil
	case "ownership.opaque":
		param, err := decodeRequiredParamRef(w.Param, "opaque param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Opaque{Param: param}, nil
	case "ownership.freeze":
		param, err := decodeRequiredParamRef(w.Param, "freeze param missing param ref")
		if err != nil {
			return nil, err
		}
		return ownership.Freeze{Param: param}, nil
	case postcondition.NormalReturnRefinementKind:
		refinement, err := decodeEffectRefinement(w.Refinement)
		if err != nil {
			return nil, err
		}
		target, err := decodeRequiredParamRef(w.Target, "normal return refinement target missing param ref")
		if err != nil {
			return nil, err
		}
		return postcondition.NormalReturnRefinement{Target: target, Refinement: refinement}, nil
	case "returns.return":
		transform, err := decodeEffectReturn(w.ReturnType)
		if err != nil {
			return nil, err
		}
		returnIndex, err := decodeRequiredInt(w.ReturnIndex, "return index missing")
		if err != nil {
			return nil, err
		}
		return returns.Return{ReturnIndex: returnIndex, Transform: transform}, nil
	case "returns.errorReturn":
		valueIndex, err := decodeRequiredInt(w.ValueIndex, "error return value index missing")
		if err != nil {
			return nil, err
		}
		errorIndex, err := decodeRequiredInt(w.ErrorIndex, "error return error index missing")
		if err != nil {
			return nil, err
		}
		return returns.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex}, nil
	case "returns.returnLength":
		return nil, inactiveManifestEffectLabel(returns.ReturnLength{})
	case "returns.correlatedReturn":
		return nil, inactiveManifestEffectLabel(returns.CorrelatedReturn{})
	default:
		return nil, fmt.Errorf("manifest: unknown effect label kind %q", w.Kind)
	}
}
