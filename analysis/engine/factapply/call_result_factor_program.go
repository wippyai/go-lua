package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CallResultValueBinding is one frozen N0 result-index to Values-root binding.
// K is address syntax only: concrete State and the formal tuple carrier execute
// the same product-value law over different collision-free root vocabularies.
type CallResultValueBinding[K comparable] struct {
	Index int
	Slot  K
	Value product.Value
}

// CallResultMaterializeFactorProgram is the complete factor-native N0
// transaction. It contains no State, resolver, inventory or callback. The
// transaction is an endomorphism of exactly one Values factor; every residual
// product lane is therefore proved independent by construction.
type CallResultMaterializeFactorProgram[K comparable] struct {
	reg      *axis.Registry
	bindings []CallResultValueBinding[K]
}

// PrepareCallResultMaterializeFactorProgram binds the immutable call-result
// syntax to a carrier root vocabulary once. bind must be injective for the
// transaction's distinct result indexes; duplicate roots are rejected instead
// of silently introducing an order-dependent overwrite.
func PrepareCallResultMaterializeFactorProgram[K comparable](
	reg *axis.Registry,
	transaction CallResultTransaction,
	bind func(point uint32, result uint32) (K, bool),
) (CallResultMaterializeFactorProgram[K], error) {
	if reg == nil || bind == nil || !transaction.Valid(reg) {
		return CallResultMaterializeFactorProgram[K]{}, fmt.Errorf("factapply: invalid call-result materialize factor program")
	}
	out := CallResultMaterializeFactorProgram[K]{reg: reg}
	seen := make(map[K]int)
	for _, step := range transaction.steps {
		if step.kind != CallResultStepValue {
			continue
		}
		index := step.value.Index()
		if index < 0 {
			return CallResultMaterializeFactorProgram[K]{}, fmt.Errorf("factapply: invalid call-result value index")
		}
		slot, ok := bind(uint32(transaction.point), uint32(index))
		if !ok {
			return CallResultMaterializeFactorProgram[K]{}, fmt.Errorf("factapply: unresolved call-result value root %d", index)
		}
		if prior, duplicate := seen[slot]; duplicate && prior != index {
			return CallResultMaterializeFactorProgram[K]{}, fmt.Errorf("factapply: call-result indexes %d and %d share one Values root", prior, index)
		}
		seen[slot] = index
		out.bindings = append(out.bindings, CallResultValueBinding[K]{Index: index, Slot: slot, Value: step.value.Value()})
	}
	return out, nil
}

func (p CallResultMaterializeFactorProgram[K]) Len() int { return len(p.bindings) }

func (p CallResultMaterializeFactorProgram[K]) Bindings() []CallResultValueBinding[K] {
	return append([]CallResultValueBinding[K](nil), p.bindings...)
}

// Apply executes N0 atomically. Cancellation returns the exact input factor;
// Top is a fixed point, and finite maps retain the canonical omission of
// product Bottom. No work or depth budget changes the semantic result.
func (p CallResultMaterializeFactorProgram[K]) Apply(ctx context.Context, token *cancellation.Token, input state.ValueFactor[K]) (state.ValueFactor[K], error) {
	if ctx == nil || p.reg == nil {
		return input, fmt.Errorf("factapply: invalid call-result materialize factor execution")
	}
	if err := ctx.Err(); err != nil {
		return input, err
	}
	if token != nil && token.Canceled() {
		return input, context.Canceled
	}
	if input.Top || len(p.bindings) == 0 {
		return input, nil
	}
	for _, binding := range p.bindings {
		if binding.Index < 0 || !product.BelongsToRegistry(p.reg, binding.Value) {
			return input, fmt.Errorf("factapply: foreign call-result materialize binding")
		}
	}
	values := input.Values
	changed := false
	for index, binding := range p.bindings {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return input, err
			}
			if token != nil && token.Canceled() {
				return input, context.Canceled
			}
		}
		current := product.Bottom(p.reg)
		if value, present := values[binding.Slot]; present {
			current = value
		}
		next := constrainCallResultValue(p.reg, current, binding.Value)
		if product.Equal(p.reg, current, next) {
			continue
		}
		if !changed {
			values = make(map[K]product.Value, len(input.Values)+len(p.bindings))
			for slot, value := range input.Values {
				values[slot] = value
			}
			changed = true
		}
		if product.Equal(p.reg, next, product.Bottom(p.reg)) {
			delete(values, binding.Slot)
		} else {
			values[binding.Slot] = next
		}
	}
	// The polling stride is an amortization detail, not a publication window:
	// observe cancellation once more at the transaction boundary so a short
	// N0 batch cannot publish work completed after its owner was canceled.
	if err := ctx.Err(); err != nil {
		return input, err
	}
	if token != nil && token.Canceled() {
		return input, context.Canceled
	}
	if !changed {
		return input, nil
	}
	return state.ValueFactor[K]{Values: values}, nil
}

func constrainCallResultValue(reg *axis.Registry, current, value product.Value) product.Value {
	if product.Equal(reg, current, product.Bottom(reg)) {
		return value
	}
	if callResultValueLacksReadableType(reg, current) && callResultValueHasReadableType(reg, value) {
		return value
	}
	if callResultValueHasTrustedEvidence(reg, current) && callResultValueHasUntrustedTopEvidence(reg, value) {
		return current
	}
	return product.Meet(reg, current, value)
}

func callResultValueHasReadableType(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func callResultValueLacksReadableType(reg *axis.Registry, value product.Value) bool {
	return !callResultValueHasReadableType(reg, value)
}

func callResultValueHasTrustedEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func callResultValueHasUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func prepareConcreteCallResultMaterializeFactorProgram(reg *axis.Registry, transaction CallResultTransaction) (CallResultMaterializeFactorProgram[statekey.Value], error) {
	return PrepareCallResultMaterializeFactorProgram(reg, transaction, func(point, result uint32) (statekey.Value, bool) {
		return statekey.CallResult(point, result), true
	})
}
