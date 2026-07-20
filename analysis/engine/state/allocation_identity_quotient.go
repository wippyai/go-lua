package state

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// ApplyAllocationIdentityQuotientTuple is the factor-native form of
// ApplyAllocationIdentityQuotient. Values and factors must describe one
// complete ProductDomain tuple. The complete identity support is inspected
// before any factor is transformed because must lanes use the universal
// inverse fiber of every active preimage in that same correlated tuple.
//
// A tuple with no allocation template is returned unchanged, including when
// authority is nil or empty. This is the exact no-template identity contract
// of the whole-State entry point, without composing or decomposing State.
func ApplyAllocationIdentityQuotientTuple[K comparable](
	ctx context.Context,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	authority *BoundaryAllocationAuthority,
	values ValueFactor[K],
	factors []LaneFactor,
) (ValueFactor[K], []LaneFactor, error) {
	if ctx == nil || !domain.Valid() || keys == nil || !keys.Valid() {
		return ValueFactor[K]{}, nil, fmt.Errorf("state: allocation identity quotient tuple is unowned")
	}
	if err := ctx.Err(); err != nil {
		return ValueFactor[K]{}, nil, err
	}
	support, err := PrepareIdentitySubstitutionSupport(ctx, domain, values, factors)
	if err != nil {
		return ValueFactor[K]{}, nil, err
	}
	hasTemplate := false
	for _, term := range support.terms {
		if _, allocation := term.Allocation(); allocation {
			hasTemplate = true
			break
		}
	}
	if !hasTemplate {
		return values, factors, nil
	}
	if authority == nil || authority.Empty() {
		return ValueFactor[K]{}, nil, fmt.Errorf("state: allocation identity quotient has templates without route authority")
	}
	plan, unreachable, err := SealIdentitySubstitutionPlanWithSupport(
		ctx, domain, keys, NewIdentitySubstitutionAuthority(identity.Substitution{}, authority), support,
	)
	if err != nil {
		return ValueFactor[K]{}, nil, err
	}
	if unreachable {
		return ValueFactor[K]{}, nil, fmt.Errorf("state: allocation identity quotient unexpectedly eliminated its relation")
	}
	nextValues, unreachable, err := ApplyValueFactorIdentitySubstitution(ctx, domain, plan, values)
	if err != nil {
		return ValueFactor[K]{}, nil, err
	}
	if unreachable {
		return ValueFactor[K]{}, nil, fmt.Errorf("state: allocation identity quotient unexpectedly eliminated its Values relation")
	}
	nextFactors := make([]LaneFactor, len(factors))
	for index := range factors {
		nextFactors[index], unreachable, err = domain.ApplyIdentitySubstitutionFactor(ctx, plan, factors[index])
		if err != nil {
			return ValueFactor[K]{}, nil, err
		}
		if unreachable {
			return ValueFactor[K]{}, nil, fmt.Errorf("state: allocation identity quotient unexpectedly eliminated lane %q", factors[index].Lane().ID())
		}
	}
	return nextValues, nextFactors, nil
}

// ApplyAllocationIdentityQuotient alpha-renames every exact allocation
// identity in the complete enabled State lane inventory. Structural paths,
// slots, and state keys remain in their current owner keyspace; no boundary
// projection or root-reachability filter participates.
//
// Active inverse fibers are derived from the State itself. Thus a freshly
// introduced template maps to its route identity, a prior route identity is
// stable on the next application, and when both coexist every must lane uses
// universal quotient semantics while may lanes join their contributions. A
// missing or empty route authority is identity only when this same complete
// inventory proves that the input contains no lexical allocation template.
func ApplyAllocationIdentityQuotient(
	ctx context.Context,
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	authority *BoundaryAllocationAuthority,
	input State,
) (State, error) {
	if ctx == nil || reg == nil || keys == nil || !keys.Valid() {
		return State{}, fmt.Errorf("state: allocation identity quotient is unowned")
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	hasTemplate, err := stateContainsAllocationTemplate(ctx, reg, input)
	if err != nil {
		return State{}, err
	}
	if !hasTemplate {
		// The complete registered identity inventory proves that quotienting is
		// the mathematical identity. No authority is needed to preserve a State
		// which contains no lexical allocation identity.
		return input, nil
	}
	if authority == nil || authority.Empty() {
		return State{}, fmt.Errorf("state: allocation identity quotient has templates without route authority")
	}
	lanes := make([]LaneID, 0, len(defaultLaneCatalog.specs))
	for _, spec := range defaultLaneCatalog.specs {
		if input.laneMask.allows(spec.bit) {
			lanes = append(lanes, spec.id)
		}
	}
	domain, err := TryRegisteredProductDomainWithLanes(reg, lanes)
	if err != nil {
		return State{}, err
	}
	input = domain.Normalize(input)
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		return State{}, err
	}
	values, factors, err = ApplyAllocationIdentityQuotientTuple(ctx, domain, keys, authority, values, factors)
	if err != nil {
		return State{}, err
	}
	return domain.ComposeFactorTuple(values, factors)
}
