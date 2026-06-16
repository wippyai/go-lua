package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
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
		return fmt.Errorf("manifest: inactive effect label %s (%s)", desc.ID, desc.Status)
	default:
		return nil
	}
}

func encodeEffectLabel(label effect.Label) (effectLabelWire, error) {
	label = effect.NormalizeLabel(label)
	switch l := label.(type) {
	case control.Throw:
		return effectLabelWire{Kind: "control.throw"}, nil
	case control.IO:
		return effectLabelWire{Kind: "control.io"}, nil
	case dispatch.ModuleLoad:
		return effectLabelWire{Kind: "dispatch.moduleLoad"}, nil
	case dispatch.VariadicTransform:
		return effectLabelWire{Kind: "dispatch.variadicTransform"}, nil
	case dispatch.TypePredicate:
		return effectLabelWire{Kind: "dispatch.typePredicate"}, nil
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
			Delta:  l.Delta,
		}, nil
	case mutation.TableMutator:
		return effectLabelWire{
			Kind:   "mutation.tableMutator",
			Target: encodeParamRef(l.Target),
			Value:  encodeParamRef(l.Value),
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
		return effectLabelWire{Kind: "ownership.send", FromParam: l.FromParam}, nil
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
		return effectLabelWire{Kind: "returns.return", ReturnIndex: l.ReturnIndex, ReturnType: transform}, nil
	case returns.ErrorReturn:
		return effectLabelWire{Kind: "returns.errorReturn", ValueIndex: l.ValueIndex, ErrorIndex: l.ErrorIndex}, nil
	case returns.ReturnLength:
		length, err := encodeExpr(l.Length)
		if err != nil {
			return effectLabelWire{}, err
		}
		return effectLabelWire{Kind: "returns.returnLength", ReturnIndex: l.ReturnIndex, Length: length}, nil
	case returns.CorrelatedReturn:
		indices := append([]int(nil), l.Indices...)
		return effectLabelWire{Kind: "returns.correlatedReturn", Indices: indices}, nil
	default:
		return effectLabelWire{}, fmt.Errorf("manifest: unsupported effect label %T", label)
	}
}

func decodeEffectLabel(w effectLabelWire) (effect.Label, error) {
	switch w.Kind {
	case "control.throw":
		return control.Throw{}, nil
	case "control.io":
		return control.IO{}, nil
	case "dispatch.moduleLoad":
		return dispatch.ModuleLoad{}, nil
	case "dispatch.variadicTransform":
		return dispatch.VariadicTransform{}, nil
	case "dispatch.typePredicate":
		return dispatch.TypePredicate{}, nil
	case "iteration.iterator":
		kind, err := decodeIteratorKind(w.IteratorKind)
		if err != nil {
			return nil, err
		}
		return iteration.Iterator{Source: decodeParamRef(w.Source), Kind: kind}, nil
	case "mutation.mutate":
		transform, err := decodeEffectTransform(w.Transform)
		if err != nil {
			return nil, err
		}
		length, err := decodeExpr(w.Length)
		if err != nil {
			return nil, err
		}
		return mutation.Mutate{Target: decodeParamRef(w.Target), Transform: transform, LengthDelta: length}, nil
	case "mutation.lengthChange":
		return mutation.LengthChange{Target: decodeParamRef(w.Target), Delta: w.Delta}, nil
	case "mutation.tableMutator":
		return mutation.TableMutator{Target: decodeParamRef(w.Target), Value: decodeParamRef(w.Value)}, nil
	case "ownership.borrow":
		return ownership.Borrow{Param: decodeParamRef(w.Param)}, nil
	case "ownership.retain":
		return ownership.Retain{Param: decodeParamRef(w.Param)}, nil
	case "ownership.store":
		return ownership.Store{Param: decodeParamRef(w.Param), Into: decodeParamRef(w.Into)}, nil
	case "ownership.borrowAll":
		return ownership.BorrowAll{}, nil
	case "ownership.send":
		return ownership.Send{FromParam: w.FromParam}, nil
	case "ownership.sendParam":
		return ownership.SendParam{Param: decodeParamRef(w.Param)}, nil
	case "ownership.export":
		return ownership.Export{Param: decodeParamRef(w.Param)}, nil
	case "ownership.opaque":
		return ownership.Opaque{Param: decodeParamRef(w.Param)}, nil
	case "ownership.freeze":
		return ownership.Freeze{Param: decodeParamRef(w.Param)}, nil
	case postcondition.NormalReturnRefinementKind:
		refinement, err := decodeEffectRefinement(w.Refinement)
		if err != nil {
			return nil, err
		}
		return postcondition.NormalReturnRefinement{Target: decodeParamRef(w.Target), Refinement: refinement}, nil
	case "returns.return":
		transform, err := decodeEffectReturn(w.ReturnType)
		if err != nil {
			return nil, err
		}
		return returns.Return{ReturnIndex: w.ReturnIndex, Transform: transform}, nil
	case "returns.errorReturn":
		return returns.ErrorReturn{ValueIndex: w.ValueIndex, ErrorIndex: w.ErrorIndex}, nil
	case "returns.returnLength":
		length, err := decodeExpr(w.Length)
		if err != nil {
			return nil, err
		}
		return returns.ReturnLength{ReturnIndex: w.ReturnIndex, Length: length}, nil
	case "returns.correlatedReturn":
		indices := append([]int(nil), w.Indices...)
		return returns.CorrelatedReturn{Indices: indices}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect label kind %q", w.Kind)
	}
}
