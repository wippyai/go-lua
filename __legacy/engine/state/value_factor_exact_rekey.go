package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ExactValueSlotBinding is one injective structural scalar identity edge.
// Source and Target are vocabulary only; value semantics remain owned by the
// registered Values factor lattice.
type ExactValueSlotBinding[S, T comparable] struct {
	Source S
	Target T
}

// ExactValueFactorRekey is a ProductDomain-sealed total inverse on every
// non-Bottom source coordinate that appears at application. Unbound live
// coordinates fail; they are never dropped or existentially merged.
type ExactValueFactorRekey[S, T comparable] struct {
	seal  *productDomainSeal
	reg   *axis.Registry
	slots map[S]T
}

// SealExactValueFactorRekey registers an injective slot relation with domain.
func SealExactValueFactorRekey[S, T comparable](domain ProductDomain, bindings []ExactValueSlotBinding[S, T]) (ExactValueFactorRekey[S, T], error) {
	if !domain.Valid() {
		return ExactValueFactorRekey[S, T]{}, fmt.Errorf("%w: exact Values rekey domain", ErrInvalidLaneFactor)
	}
	slots := make(map[S]T, len(bindings))
	targets := make(map[T]S, len(bindings))
	for index, binding := range bindings {
		if _, duplicate := slots[binding.Source]; duplicate {
			return ExactValueFactorRekey[S, T]{}, fmt.Errorf("%w: duplicate exact Values source %d", ErrInvalidLaneFactor, index)
		}
		if prior, collision := targets[binding.Target]; collision && prior != binding.Source {
			return ExactValueFactorRekey[S, T]{}, fmt.Errorf("%w: non-injective exact Values target %d", ErrInvalidLaneFactor, index)
		}
		slots[binding.Source] = binding.Target
		targets[binding.Target] = binding.Source
	}
	return ExactValueFactorRekey[S, T]{seal: domain.seal, reg: domain.reg, slots: slots}, nil
}

func (p ExactValueFactorRekey[S, T]) valid() bool {
	return p.seal != nil && p.reg != nil && p.slots != nil
}

// Apply transports one complete Values factor without interpreting either
// slot vocabulary. Top is invariant; finite live coordinates require an exact
// binding and retain their product value unchanged.
func (p ExactValueFactorRekey[S, T]) Apply(source ValueFactor[S]) (ValueFactor[T], error) {
	if !p.valid() || source.Top && len(source.Values) != 0 {
		return ValueFactor[T]{}, fmt.Errorf("%w: exact Values rekey is malformed", ErrInvalidLaneFactor)
	}
	if source.Top {
		return ValueFactor[T]{Top: true}, nil
	}
	if len(source.Values) == 0 {
		return ValueFactor[T]{}, nil
	}
	out := make(map[T]product.Value, len(source.Values))
	for slot, value := range source.Values {
		if !product.BelongsToRegistry(p.reg, value) {
			return ValueFactor[T]{}, fmt.Errorf("%w: exact Values source contains a foreign product", ErrInvalidLaneFactor)
		}
		target, present := p.slots[slot]
		if !present {
			return ValueFactor[T]{}, fmt.Errorf("%w: exact Values source has no target", ErrInvalidLaneFactor)
		}
		if _, duplicate := out[target]; duplicate {
			return ValueFactor[T]{}, fmt.Errorf("%w: exact Values target is non-injective", ErrInvalidLaneFactor)
		}
		out[target] = value
	}
	return ValueFactor[T]{Values: out}, nil
}
