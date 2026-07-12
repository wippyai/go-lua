package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
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
			fragment, valid := resolve(resolved)
			if !valid || !r.authority.allowsEffectFragment(resolved, fragment) {
				return summary.Summary{}, false
			}
			candidate = summary.Join(reg, candidate, fragment)
		}
		if !have {
			accumulated = candidate
			have = true
		} else {
			accumulated = summary.Join(reg, accumulated, candidate)
		}
	}
	if !have {
		return summary.Summary{}, true
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
