package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// DescriptorHandler lowers one executable operation into the existing Summary
// payload. Handlers mutate only a transaction-local summary.
type DescriptorHandler interface {
	Kind() DescriptorKind
	ConditionalAllowed() bool
	Apply(*axis.Registry, *summary.Summary, uint32, product.Value) error
}

// DescriptorRegistry is the only transformer-to-Summary specialization seam.
type DescriptorRegistry struct {
	handlers map[DescriptorKind]DescriptorHandler
}

func NewDescriptorRegistry(handlers ...DescriptorHandler) (*DescriptorRegistry, error) {
	r := &DescriptorRegistry{handlers: make(map[DescriptorKind]DescriptorHandler, len(handlers))}
	for _, handler := range handlers {
		if handler == nil {
			return nil, fmt.Errorf("transformer: nil descriptor handler")
		}
		kind := handler.Kind()
		if kind == "" {
			return nil, fmt.Errorf("transformer: empty descriptor kind")
		}
		if _, exists := r.handlers[kind]; exists {
			return nil, fmt.Errorf("transformer: duplicate descriptor handler %q", kind)
		}
		r.handlers[kind] = handler
	}
	return r, nil
}

func newDefaultDescriptorRegistry() *DescriptorRegistry {
	r, err := NewDescriptorRegistry(returnHandler{}, obligationHandler{})
	if err != nil {
		panic(err)
	}
	return r
}

var defaultDescriptorRegistry = newDefaultDescriptorRegistry()

func DefaultDescriptorRegistry() *DescriptorRegistry { return defaultDescriptorRegistry }

type returnHandler struct {
	declared []product.Value
}

func (returnHandler) Kind() DescriptorKind     { return DescriptorReturn }
func (returnHandler) ConditionalAllowed() bool { return true }
func (h returnHandler) Apply(reg *axis.Registry, out *summary.Summary, slot uint32, value product.Value) error {
	if int(slot) < len(h.declared) {
		value = checkprojection.WithDeclaredContractPreservingPresence(reg, value, h.declared[slot])
	}
	priorLen := len(out.Returns)
	for len(out.Returns) <= int(slot) {
		out.Returns = append(out.Returns, product.Bottom(reg))
	}
	if int(slot) >= priorLen {
		out.Returns[slot] = value
		return nil
	}
	out.Returns[slot] = summary.JoinReturnValue(reg, out.Returns[slot], value)
	return nil
}

type obligationHandler struct{}

func (obligationHandler) Kind() DescriptorKind     { return DescriptorObligation }
func (obligationHandler) ConditionalAllowed() bool { return false }
func (obligationHandler) Apply(reg *axis.Registry, out *summary.Summary, slot uint32, value product.Value) error {
	priorLen := len(out.ParamObligations)
	for len(out.ParamObligations) <= int(slot) {
		out.ParamObligations = append(out.ParamObligations, product.Top())
	}
	if int(slot) >= priorLen {
		out.ParamObligations[slot] = value
	} else {
		out.ParamObligations[slot] = product.Meet(reg, out.ParamObligations[slot], value)
	}
	return nil
}

// Specialize transactionally evaluates every feasible correlated row and emits
// the existing Summary representation. False means the caller must run the
// contextual solver; out is guaranteed zero on failure.
func (r Relation) Specialize(cursor BindingCursor, descriptors *DescriptorRegistry, resolve CellResultResolver) (out summary.Summary, ok bool) {
	return r.SpecializeWithContext(cursor, descriptors, SpecializationContext{CellResult: resolve})
}

// SpecializeWithContext is the inactive full specialization seam for value
// terms that require caller-owned concrete read semantics.
func (r Relation) SpecializeWithContext(cursor BindingCursor, descriptors *DescriptorRegistry, context SpecializationContext) (out summary.Summary, ok bool) {
	return r.specializeWithEffects(cursor, descriptors, context, nil)
}

// SpecializeWithEffects is the inactive effect-aware specialization seam.
// Effects can only become fragments of the existing Summary; caller State
// application remains outside Relation and is owned by the call adapter.
func (r Relation) SpecializeWithEffects(cursor BindingCursor, descriptors *DescriptorRegistry, context SpecializationContext, resolve EffectSummaryResolver) (out summary.Summary, ok bool) {
	return r.specializeWithEffects(cursor, descriptors, context, resolve)
}

// SpecializationResult separates the canonical Summary payload from symbolic
// must-preservation metadata. PreservedParams contains sorted boundary
// parameter indexes preserved by every feasible row. The metadata is not an
// ordinary generic Summary fact: only a concrete certified entry may project
// it as a normal-return path refinement.
type SpecializationResult struct {
	Summary         summary.Summary
	PreservedParams []uint32
}

// SpecializeDetailed evaluates the same transaction as Specialize while
// retaining row-local parameter preservation. It deliberately walks guards a
// second time and is intended for freeze-time certified context publication,
// not the hot per-call path. Effectful relations fail closed because this
// migration API supplies no effect resolver. The returned slice is owned by
// the result. False guarantees a zero result.
func (r Relation) SpecializeDetailed(cursor BindingCursor, descriptors *DescriptorRegistry, context SpecializationContext) (SpecializationResult, bool) {
	sum, ok := r.specializeWithEffects(cursor, descriptors, context, nil)
	if !ok {
		return SpecializationResult{}, false
	}
	preserved, ok := r.specializedPreservedParams(cursor, context)
	if !ok {
		return SpecializationResult{}, false
	}
	return SpecializationResult{Summary: sum, PreservedParams: preserved}, true
}

// specializedPreservedParams intersects the must-preservation sets of every
// feasible correlated row. One non-preserving alternative removes the root;
// an infeasible alternative has no effect.
func (r Relation) specializedPreservedParams(cursor BindingCursor, context SpecializationContext) ([]uint32, bool) {
	if r.arena == nil || r.contextual != "" || cursor.shape != r.shape {
		return nil, false
	}
	var preserved []bool
	have := false
	for _, row := range r.rows {
		feasible, valid := r.arena.evalGuard(row.Guard, cursor, context)
		if !valid {
			return nil, false
		}
		if !feasible {
			continue
		}
		rowPreserved := make([]bool, r.shape.Params)
		for _, refinement := range row.PathRefinements {
			index, valid := refinement.preservedParam(r.arena)
			if !valid || index >= r.shape.Params {
				return nil, false
			}
			rowPreserved[index] = true
		}
		if !have {
			preserved = rowPreserved
			have = true
			continue
		}
		for index := range preserved {
			preserved[index] = preserved[index] && rowPreserved[index]
		}
	}
	if !have {
		return nil, true
	}
	out := make([]uint32, 0, len(preserved))
	for index, present := range preserved {
		if present {
			out = append(out, uint32(index))
		}
	}
	return out, true
}

func (r Relation) specializeWithEffects(cursor BindingCursor, descriptors *DescriptorRegistry, context SpecializationContext, resolve EffectSummaryResolver) (out summary.Summary, ok bool) {
	if r.arena == nil || r.contextual != "" || cursor.shape != r.shape {
		return summary.Summary{}, false
	}
	if descriptors == nil {
		descriptors = r.descriptors
		if descriptors == nil {
			descriptors = DefaultDescriptorRegistry()
		}
	}
	reg := r.arena.reg
	var accumulated summary.Summary
	var candidates []summary.Summary
	var rawCandidateReturns [][]product.Value
	have := false
	for _, row := range r.rows {
		feasible, valid := r.arena.evalGuard(row.Guard, cursor, context)
		if !valid {
			return summary.Summary{}, false
		}
		if !feasible {
			continue
		}
		candidate := row.Output.Clone()
		var rawReturns []product.Value
		if r.inferReturnCorrelations {
			rawReturns = append(rawReturns, row.Output.Returns...)
		}
		for _, operation := range row.Ops {
			handler := descriptors.handlers[operation.Descriptor]
			if handler == nil {
				return summary.Summary{}, false
			}
			if row.Guard != r.arena.True() && !handler.ConditionalAllowed() {
				return summary.Summary{}, false
			}
			value, valid := r.arena.evalValue(operation.Value, cursor, context)
			if !valid {
				return summary.Summary{}, false
			}
			if err := handler.Apply(reg, &candidate, operation.Slot, value); err != nil {
				return summary.Summary{}, false
			}
			if operation.Descriptor == DescriptorReturn {
				if r.inferReturnCorrelations {
					priorLen := len(rawReturns)
					for len(rawReturns) <= int(operation.Slot) {
						rawReturns = append(rawReturns, product.Bottom(reg))
					}
					if int(operation.Slot) >= priorLen {
						rawReturns[operation.Slot] = value
					} else {
						rawReturns[operation.Slot] = summary.JoinReturnValue(reg, rawReturns[operation.Slot], value)
					}
				}
				if param, exact := r.arena.directParamRoot(operation.Value); exact {
					if r.authority.allowsSummary("ReturnFlows") && r.authority.allowsSummary("ReturnParamPathAliases") {
						placeholder, placeholderOK := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(param))
						if !placeholderOK {
							return summary.Summary{}, false
						}
						candidate.ReturnFlows = append(candidate.ReturnFlows, summary.ReturnFlow{
							ReturnIndex: int(operation.Slot), Kind: summary.ReturnFlowParam, Param: param,
						})
						candidate.ReturnParamPathAliases = append(candidate.ReturnParamPathAliases, summary.ReturnParamPathAlias{
							ReturnIndex: int(operation.Slot), Source: placeholder,
						})
					}
				}
			}
		}
		if (len(row.Proofs) != 0 || len(row.PathRefinements) != 0) && !r.authority.allowsSummary("NormalReturnFacts") {
			return summary.Summary{}, false
		}
		for _, proof := range row.Proofs {
			tablePath, valid := proof.placeholderPath(r.arena)
			if !valid {
				return summary.Summary{}, false
			}
			proofPath := tablePath.Clone()
			if proof.Key != 0 {
				key, valid := r.arena.evalValue(proof.Key, cursor, context)
				if !valid {
					return summary.Summary{}, false
				}
				keySegment, valid := typevalue.ExactScalarKeySegment(reg, nil, key)
				if !valid {
					return summary.Summary{}, false
				}
				proofPath.Segments = append(proofPath.Segments, keySegment)
			}
			candidate.NormalReturnFacts.BranchProofs = append(candidate.NormalReturnFacts.BranchProofs, callboundary.BranchProof{
				Kind: proof.Kind, Path: proofPath, Presence: proof.Presence,
			})
		}
		if len(row.Effects) != 0 {
			if resolve == nil || r.effects == nil || r.authority == nil {
				return summary.Summary{}, false
			}
			resolved := make([]ResolvedEffect, len(row.Effects))
			for i, effect := range row.Effects {
				var valid bool
				resolved[i], valid = r.effects.resolve(effect, cursor, context)
				if !valid || resolved[i].Kind != r.effects.Kind(effect) {
					return summary.Summary{}, false
				}
			}
			resolution, valid := resolve(resolved)
			if !valid || !r.authority.allowsEffectResolution(resolved, resolution) {
				return summary.Summary{}, false
			}
			candidate = summary.Join(reg, candidate, resolution.Summary)
		}
		if !have {
			accumulated = candidate
			have = true
		} else {
			accumulated = summary.Join(reg, accumulated, candidate)
		}
		if r.inferReturnCorrelations {
			candidates = append(candidates, candidate)
			rawCandidateReturns = append(rawCandidateReturns, rawReturns)
		}
	}
	if !have {
		return summary.Summary{}, true
	}
	if r.inferReturnCorrelations {
		var declared []product.Value
		if handler, ok := descriptors.handlers[DescriptorReturn].(returnHandler); ok {
			declared = handler.declared
		}
		conditions, relations, exact := inferReturnRowCorrelations(reg, candidates, rawCandidateReturns, declared)
		if !exact {
			return summary.Summary{}, false
		}
		if len(conditions) != 0 && !r.authority.allowsSummary("ReturnConditionSlotRefinements") {
			return summary.Summary{}, false
		}
		if len(relations) != 0 && !r.authority.allowsSummary("ReturnPresenceRelations") {
			return summary.Summary{}, false
		}
		accumulated.ReturnConditionSlotRefinements = append(accumulated.ReturnConditionSlotRefinements, conditions...)
		accumulated.ReturnPresenceRelations = append(accumulated.ReturnPresenceRelations, relations...)
	}
	return summary.NormalizeOwned(reg, accumulated), true
}

func (a *Arena) evalGuard(guard Guard, cursor BindingCursor, context SpecializationContext) (bool, bool) {
	if guard == 0 || int(guard) >= len(a.guards) || a.reg == nil {
		return false, false
	}
	n := a.guards[guard]
	switch n.op {
	case guardTrue:
		return true, true
	case guardFalse:
		return false, true
	case guardTruthy, guardFalsy:
		value, ok := a.evalValue(n.value, cursor, context)
		if !ok {
			return false, false
		}
		if n.op == guardTruthy {
			return valueref.CanBeTruthy(a.reg, value), true
		}
		return valueref.CanBeFalsy(a.reg, value), true
	case guardAnd:
		for _, arg := range n.args {
			holds, ok := a.evalGuard(arg, cursor, context)
			if !ok {
				return false, false
			}
			if !holds {
				return false, true
			}
		}
		return true, true
	case guardOr:
		for _, arg := range n.args {
			holds, ok := a.evalGuard(arg, cursor, context)
			if !ok {
				return false, false
			}
			if holds {
				return true, true
			}
		}
		return false, true
	default:
		return false, false
	}
}
