package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// IdentitySubstitutionAuthority is the single exact image authority shared by
// every registered identity-bearing lane and coordinate. Formal variables and
// allocation templates remain disjoint: existential/formal substitution can
// never instantiate an allocation, and allocation alpha-renaming can never
// bind a formal variable.
type IdentitySubstitutionAuthority struct {
	formal      identity.Substitution
	allocations *BoundaryAllocationAuthority
}

// identitySubstitutionPlanSeal prevents callers from fabricating a plan that
// did not observe the complete product tuple. Completeness matters for must
// facts: their inverse fibers are defined by every active preimage across all
// factors, not by the factor currently being transformed.
type identitySubstitutionPlanSeal struct{ owned byte }

// IdentitySubstitutionPlan is an opaque, complete-tuple substitution proof.
// It is prepared once from Values plus the exact registry-ordered inventory of
// every enabled residual factor, then shared by all factor-native image laws.
type IdentitySubstitutionPlan struct {
	seal      *identitySubstitutionPlanSeal
	domain    *productDomainSeal
	reg       *axis.Registry
	keys      *keyspace.KeySpace
	authority *IdentitySubstitutionAuthority
	quotient  boundaryInverseQuotient
	unchanged bool
}

type identitySubstitutionSupportSeal struct{ owned byte }

// IdentitySubstitutionSupport is the sorted complete identity vocabulary of
// one factored product spelling. It is prepared when that factor vector is
// published and reused by every caller-derived substitution; Apply therefore
// never rescans Values or registered lanes to discover support.
type IdentitySubstitutionSupport struct {
	seal   *identitySubstitutionSupportSeal
	domain *productDomainSeal
	reg    *axis.Registry
	terms  []identity.Term
}

func identityImage(ctx *boundaryRebaseContext, term identity.Term) (identity.Value, bool) {
	if ctx == nil {
		return identity.Bottom(), false
	}
	authority := ctx.identities
	if authority == nil {
		authority = NewIdentitySubstitutionAuthority(identity.Substitution{}, ctx.allocations)
	}
	image, err := authority.Image(term)
	return image, err == nil
}

func NewIdentitySubstitutionAuthority(formal identity.Substitution, allocations *BoundaryAllocationAuthority) *IdentitySubstitutionAuthority {
	return &IdentitySubstitutionAuthority{formal: formal, allocations: allocations}
}

// Image returns the exact flat-lattice image of term. Bottom is a relation
// contradiction, Singleton is an exact rename, and Top invokes the registered
// unknown-image law of the containing factor.
func (a *IdentitySubstitutionAuthority) Image(term identity.Term) (identity.Value, error) {
	if a == nil || !term.Valid() {
		return identity.Bottom(), fmt.Errorf("state: identity substitution is unowned")
	}
	if concrete, ok := term.Concrete(); ok {
		return identity.Singleton(concrete), nil
	}
	if formal, ok := term.Formal(); ok {
		image, bound := a.formal.Image(formal)
		if !bound {
			return identity.Bottom(), fmt.Errorf("state: formal identity has no substitution")
		}
		// Substitution is simultaneous: Formal->Formal is one exact rename,
		// never recursively chased. Allocation is a disjoint authority; preserve
		// it during symbolic composition and instantiate it exactly only when a
		// frame authority is present.
		imageTerm, singleton := image.Term()
		if !singleton {
			return image, nil
		}
		if template, allocation := imageTerm.Allocation(); allocation && a.allocations != nil {
			concrete, ok := a.allocations.RebaseAllocation(template)
			if !ok {
				return identity.Bottom(), fmt.Errorf("state: substituted allocation is outside frame authority")
			}
			return identity.Singleton(concrete), nil
		}
		return image, nil
	}
	template, ok := term.Allocation()
	if !ok {
		return identity.Bottom(), fmt.Errorf("state: invalid identity term alternative")
	}
	if a.allocations == nil {
		return identity.SingletonTerm(term), nil
	}
	concrete, bound := a.allocations.RebaseAllocation(template)
	if !bound {
		return identity.Bottom(), fmt.Errorf("state: allocation identity is outside frame authority")
	}
	return identity.Singleton(concrete), nil
}

// MaterializeSingleton is the root/publication fence. Top and Bottom remain
// valid abstract identity values elsewhere; only a singleton must be concrete.
func (a *IdentitySubstitutionAuthority) MaterializeSingleton(term identity.Term) (identity.ID, bool) {
	image, err := a.Image(term)
	if err != nil {
		return identity.ID{}, false
	}
	return image.ID()
}

// PrepareIdentitySubstitutionSupport freezes the complete sorted support of a
// factor vector before it can become an Apply operand.
func PrepareIdentitySubstitutionSupport[K comparable](
	ctx context.Context,
	domain ProductDomain,
	values ValueFactor[K],
	factors []LaneFactor,
) (IdentitySubstitutionSupport, error) {
	if ctx == nil || !domain.Valid() {
		return IdentitySubstitutionSupport{}, fmt.Errorf("state: identity substitution support is unowned")
	}
	if err := ctx.Err(); err != nil {
		return IdentitySubstitutionSupport{}, err
	}
	width := domain.NonValuesLaneCount()
	if len(factors) != width {
		return IdentitySubstitutionSupport{}, fmt.Errorf("%w: identity tuple has %d residual factors, want %d", ErrIncompleteLaneFactors, len(factors), width)
	}
	for index := 0; index < width; index++ {
		lane, ok := domain.NonValuesLaneAt(index)
		runtime, err := domain.validateFactorFor(&domain.factorLanes[int(lane.ordinal)], factors[index])
		if !ok || err != nil || runtime.lane != lane {
			return IdentitySubstitutionSupport{}, fmt.Errorf("%w: identity tuple position %d: %v", ErrIncompleteLaneFactors, index, err)
		}
	}

	support := make(map[identity.Term]struct{})
	invalidValue := false
	if !values.Top {
		for _, value := range values.Values {
			if !product.BelongsToRegistry(domain.reg, value) {
				invalidValue = true
				break
			}
			visitProductIdentity(domain.reg, value, func(term identity.Term) bool {
				if !term.Valid() {
					invalidValue = true
					return false
				}
				support[term] = struct{}{}
				return true
			})
		}
	}
	if invalidValue {
		return IdentitySubstitutionSupport{}, fmt.Errorf("state: identity tuple contains a foreign or invalid product value")
	}
	for index := range factors {
		invalidTerm := false
		if err := domain.visitLaneIdentities(factors[index], func(term identity.Term) bool {
			if !term.Valid() {
				invalidTerm = true
				return false
			}
			support[term] = struct{}{}
			return true
		}); err != nil {
			return IdentitySubstitutionSupport{}, err
		}
		if invalidTerm {
			return IdentitySubstitutionSupport{}, fmt.Errorf("state: identity tuple contains an invalid identity term")
		}
		if err := ctx.Err(); err != nil {
			return IdentitySubstitutionSupport{}, err
		}
	}
	terms := make([]identity.Term, 0, len(support))
	for term := range support {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool { return identityTermLess(terms[i], terms[j]) })
	return IdentitySubstitutionSupport{
		seal: &identitySubstitutionSupportSeal{}, domain: domain.seal, reg: domain.reg, terms: terms,
	}, nil
}

// SealIdentitySubstitutionPlanWithSupport evaluates one caller-derived image
// over prefrozen support. Source iteration is already deterministic and no
// product factor is inspected here.
func SealIdentitySubstitutionPlanWithSupport(
	ctx context.Context,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	authority *IdentitySubstitutionAuthority,
	support IdentitySubstitutionSupport,
) (IdentitySubstitutionPlan, bool, error) {
	if ctx == nil || !domain.Valid() || keys == nil || !keys.Valid() || authority == nil ||
		support.seal == nil || support.domain != domain.seal || support.reg != domain.reg {
		return IdentitySubstitutionPlan{}, false, fmt.Errorf("state: identity substitution transaction is unowned")
	}
	if err := ctx.Err(); err != nil {
		return IdentitySubstitutionPlan{}, false, err
	}

	quotient := boundaryInverseQuotient{
		identities: make(map[identity.Term][]identity.Term, len(support.terms)), structuralIdentity: true,
	}
	unchanged := true
	for _, source := range support.terms {
		image, err := authority.Image(source)
		if err != nil {
			return IdentitySubstitutionPlan{}, false, err
		}
		if image.IsBottom() {
			return IdentitySubstitutionPlan{}, true, nil
		}
		if image.IsTop() {
			quotient.allIdentities = true
			unchanged = false
			continue
		}
		target, exact := image.Term()
		if !exact {
			return IdentitySubstitutionPlan{}, false, fmt.Errorf("state: identity substitution produced invalid singleton")
		}
		quotient.identities[target] = append(quotient.identities[target], source)
		unchanged = unchanged && target == source
	}
	for target, sources := range quotient.identities {
		sort.Slice(sources, func(i, j int) bool { return identityTermLess(sources[i], sources[j]) })
		quotient.identities[target] = compactComparable(sources)
	}
	return IdentitySubstitutionPlan{
		seal: &identitySubstitutionPlanSeal{}, domain: domain.seal, reg: domain.reg,
		keys: keys, authority: authority, quotient: quotient, unchanged: unchanged,
	}, false, nil
}

// SealIdentitySubstitutionPlan is the whole-product adapter retained for
// callers that do not own a factor-vector terminal. Formal Apply uses the two
// staged API above and never invokes this support scan in leaf execution.
func SealIdentitySubstitutionPlan[K comparable](
	ctx context.Context,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	authority *IdentitySubstitutionAuthority,
	values ValueFactor[K],
	factors []LaneFactor,
) (IdentitySubstitutionPlan, bool, error) {
	support, err := PrepareIdentitySubstitutionSupport(ctx, domain, values, factors)
	if err != nil {
		return IdentitySubstitutionPlan{}, false, err
	}
	return SealIdentitySubstitutionPlanWithSupport(ctx, domain, keys, authority, support)
}

func (p IdentitySubstitutionPlan) validFor(domain ProductDomain) bool {
	return p.seal != nil && domain.Valid() && p.domain == domain.seal && p.reg == domain.reg &&
		p.keys != nil && p.keys.Valid() && p.authority != nil
}

func (p IdentitySubstitutionPlan) rebaseContext() boundaryRebaseContext {
	return boundaryRebaseContext{
		reg: p.reg, fromKeys: p.keys, toKeys: p.keys, identities: p.authority,
		allocations: p.authority.allocations, quotient: p.quotient, structuralIdentity: true,
	}
}

// ApplyBoundaryFactorSelectionIdentitySubstitution maps the already-closed
// identity fiber through the same complete-tuple image plan used by Values and
// every residual factor. Structural slots/paths are unchanged; no product
// carrier is rescanned and inverse-fiber must semantics stay aligned.
func ApplyBoundaryFactorSelectionIdentitySubstitution(
	domain ProductDomain,
	plan IdentitySubstitutionPlan,
	selection BoundaryFactorSelection,
) (BoundaryFactorSelection, bool, error) {
	if !plan.validFor(domain) || !selection.valid() || selection.keys != plan.keys {
		return BoundaryFactorSelection{}, false, fmt.Errorf("state: identity selection substitution is unowned")
	}
	closure := cloneBoundaryFactorClosure(selection.closure)
	closure.identities = make(map[identity.Term]struct{}, len(selection.closure.identities))
	for source := range selection.closure.identities {
		image, err := plan.authority.Image(source)
		if err != nil {
			return BoundaryFactorSelection{}, false, err
		}
		if image.IsBottom() {
			return BoundaryFactorSelection{}, true, nil
		}
		if image.IsTop() {
			closure.allIdentities = true
			continue
		}
		target, exact := image.Term()
		if !exact {
			return BoundaryFactorSelection{}, false, fmt.Errorf("state: identity selection image is malformed")
		}
		closure.identities[target] = struct{}{}
	}
	return BoundaryFactorSelection{
		seal: &boundaryFactorSelectionSeal{}, keys: selection.keys, closure: closure,
		roots: append([]BoundaryFactorRoot(nil), selection.roots...),
	}, false, nil
}

// ApplyIdentitySubstitutionFactor applies a sealed complete-tuple plan to one
// opaque residual factor through its registered executable image law. It has
// no LaneID dispatch, payload inspection, or State adaptation.
func (d ProductDomain) ApplyIdentitySubstitutionFactor(
	ctx context.Context,
	plan IdentitySubstitutionPlan,
	input LaneFactor,
) (LaneFactor, bool, error) {
	if ctx == nil || !plan.validFor(d) {
		return LaneFactor{}, false, fmt.Errorf("state: identity substitution plan is unowned")
	}
	if err := ctx.Err(); err != nil {
		return LaneFactor{}, false, err
	}
	runtime, err := d.validateFactor(input)
	if err != nil || runtime.lane.slotFactored {
		return LaneFactor{}, false, fmt.Errorf("%w: identity substitution requires a residual factor", ErrInvalidLaneFactor)
	}
	if plan.unchanged {
		return input, false, nil
	}
	rebase := plan.rebaseContext()
	payload, ok := runtime.ops.boundaryRebase(&rebase, input.payload)
	if !ok {
		return LaneFactor{}, false, fmt.Errorf("state: identity substitution failed in lane %q", runtime.lane.id)
	}
	if rebase.relationBottom {
		return LaneFactor{}, true, nil
	}
	if runtime.ops.same(input.payload, payload) {
		return input, false, nil
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, false, nil
}

// ApplyValueFactorIdentitySubstitution applies the same sealed plan to the
// generic Values carrier. The slot vocabulary K remains pure address syntax.
func ApplyValueFactorIdentitySubstitution[K comparable](
	ctx context.Context,
	domain ProductDomain,
	plan IdentitySubstitutionPlan,
	input ValueFactor[K],
) (ValueFactor[K], bool, error) {
	if ctx == nil || !plan.validFor(domain) {
		return ValueFactor[K]{}, false, fmt.Errorf("state: identity substitution plan is unowned")
	}
	if err := ctx.Err(); err != nil {
		return ValueFactor[K]{}, false, err
	}
	if input.Top || plan.unchanged {
		return input, false, nil
	}
	var out map[K]product.Value
	for slot, value := range input.Values {
		if !product.BelongsToRegistry(domain.reg, value) {
			return ValueFactor[K]{}, false, fmt.Errorf("state: identity substitution contains a foreign product value")
		}
		rebase := plan.rebaseContext()
		next, ok := rebaseBoundaryProduct(&rebase, value)
		if !ok {
			return ValueFactor[K]{}, false, fmt.Errorf("state: Values identity substitution failed")
		}
		if rebase.relationBottom {
			return ValueFactor[K]{}, true, nil
		}
		if product.Equal(domain.reg, next, value) {
			continue
		}
		if out == nil {
			out = make(map[K]product.Value, len(input.Values))
			for existingSlot, existing := range input.Values {
				out[existingSlot] = existing
			}
		}
		out[slot] = next
	}
	if out == nil {
		return input, false, nil
	}
	return ValueFactor[K]{Values: out}, false, nil
}

// ApplyIdentitySubstitutionValue images one boundary-root scalar through the
// same sealed complete-tuple plan. It exists for value-only roots (notably
// return expressions) which are not stored in the source Values factor.
func ApplyIdentitySubstitutionValue(
	domain ProductDomain,
	plan IdentitySubstitutionPlan,
	value product.Value,
) (product.Value, bool, error) {
	if !plan.validFor(domain) || !product.BelongsToRegistry(domain.reg, value) {
		return product.Value{}, false, fmt.Errorf("state: identity scalar substitution is unowned")
	}
	if plan.unchanged {
		return value, false, nil
	}
	rebase := plan.rebaseContext()
	next, ok := rebaseBoundaryProduct(&rebase, value)
	if !ok {
		return product.Value{}, false, fmt.Errorf("state: scalar identity substitution failed")
	}
	return next, rebase.relationBottom, nil
}

// ApplyIdentitySubstitutionTuple is the atomic factor-native transaction. No
// transformed output is returned unless the complete tuple validates and
// every registered image law succeeds.
func ApplyIdentitySubstitutionTuple[K comparable](
	ctx context.Context,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	authority *IdentitySubstitutionAuthority,
	values ValueFactor[K],
	factors []LaneFactor,
) (ValueFactor[K], []LaneFactor, bool, error) {
	plan, unreachable, err := SealIdentitySubstitutionPlan(ctx, domain, keys, authority, values, factors)
	if err != nil || unreachable {
		return ValueFactor[K]{}, nil, unreachable, err
	}
	nextValues, unreachable, err := ApplyValueFactorIdentitySubstitution(ctx, domain, plan, values)
	if err != nil || unreachable {
		return ValueFactor[K]{}, nil, unreachable, err
	}
	nextFactors := make([]LaneFactor, len(factors))
	for index := range factors {
		nextFactors[index], unreachable, err = domain.ApplyIdentitySubstitutionFactor(ctx, plan, factors[index])
		if err != nil || unreachable {
			return ValueFactor[K]{}, nil, unreachable, err
		}
	}
	return nextValues, nextFactors, false, nil
}

// ApplyIdentitySubstitution is the whole-State entry point. It
// decomposes exactly once, delegates all semantics to the factor-native tuple
// transaction above, and recomposes exactly once; it owns no image law.
func ApplyIdentitySubstitution(
	ctx context.Context,
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	authority *IdentitySubstitutionAuthority,
	input State,
) (State, bool, error) {
	if reg == nil {
		return State{}, false, fmt.Errorf("state: identity substitution transaction is unowned")
	}
	lanes := make([]LaneID, 0, len(defaultLaneCatalog.specs))
	for _, spec := range defaultLaneCatalog.specs {
		if input.laneMask.allows(spec.bit) {
			lanes = append(lanes, spec.id)
		}
	}
	domain, err := TryRegisteredProductDomainWithLanes(reg, lanes)
	if err != nil {
		return State{}, false, err
	}
	input = domain.Normalize(input)
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		return State{}, false, err
	}
	nextValues, nextFactors, unreachable, err := ApplyIdentitySubstitutionTuple(ctx, domain, keys, authority, values, factors)
	if err != nil || unreachable {
		return State{}, unreachable, err
	}
	nextResidual, err := domain.ComposeSparse(nextFactors)
	if err != nil {
		return State{}, false, err
	}
	return RecomposeValueLane(reg, domain.Lattice(), nextResidual, nextValues), false, nil
}
