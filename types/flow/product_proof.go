package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
	"github.com/wippyai/go-lua/types/flow/domain"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// productSatisfiabilityProof is the read-only proof sibling of ProductDomain
// transfer. It may build local proof state, but it cannot call
// ApplyCondition/ApplyConjunction or mutate the source product.
type productSatisfiabilityProof struct {
	source        *ProductDomain
	env           constraint.Env
	types         *domain.TypeDomain
	numeric       *numeric.Domain
	shapes        map[constraint.PathKey]typ.Type
	equalities    *theory.EGraph
	structuralEnv constraint.Env
}

func newProductSatisfiabilityProof(source *ProductDomain) *productSatisfiabilityProof {
	p := &productSatisfiabilityProof{
		source:     source,
		env:        source.env,
		shapes:     make(map[constraint.PathKey]typ.Type),
		equalities: theory.NewEGraph(),
	}
	typeEnv := source.env.WithPathTypeOverlay(p.typeBaseForTypeDomain)
	p.types = domain.NewTypeDomain(typeEnv)
	if source.Numeric != nil {
		p.numeric = source.Numeric.Clone().(*numeric.Domain)
	} else {
		p.numeric = numeric.NewDomain(source.env)
	}
	p.structuralEnv = source.env.WithPathTypeOverlay(p.typeAt)
	return p
}

func (p *productSatisfiabilityProof) CanSatisfyConjunction(constraints []constraint.Constraint) bool {
	if p == nil || p.source == nil || p.source.IsUnsat() {
		return false
	}
	result := constraint.ToAtomsWithEnv(constraints, &p.env)
	p.buildEqualities(result.Atoms, constraints)
	if atomsContainContradiction(result.Atoms, p.equalities) {
		return false
	}
	for _, atom := range result.Atoms {
		if !p.applyAtom(atom) {
			return false
		}
	}
	if !p.propagateEqualityTypes() {
		return false
	}
	if !p.applyRelationalTypeConstraints(result.Leftover) {
		return false
	}
	if !p.applyStructuralConstraints(result.Leftover) {
		return false
	}
	return p.propagateEqualityTypes()
}

func (p *productSatisfiabilityProof) buildEqualities(atoms []constraint.Atom, constraints []constraint.Constraint) {
	if p.equalities == nil {
		p.equalities = theory.NewEGraph()
	}
	newConditionPathEvidence(atoms, constraints, p.resolvePath).RegisterInto(p.equalities)
}

func (p *productSatisfiabilityProof) applyAtom(atom constraint.Atom) bool {
	switch domain.ClassifyAtom(atom) {
	case domain.AtomClassType:
		return p.applyTypeAtom(atom)
	case domain.AtomClassNumeric:
		return p.applyNumericAtom(atom)
	case domain.AtomClassBoth:
		return p.applyTypeAtom(atom) && p.applyNumericAtom(atom)
	case domain.AtomClassNone:
		return true
	default:
		return true
	}
}

func (p *productSatisfiabilityProof) applyTypeAtom(atom constraint.Atom) bool {
	return p.types == nil || p.types.ApplyAtom(atom)
}

func (p *productSatisfiabilityProof) applyNumericAtom(atom constraint.Atom) bool {
	return p.numeric == nil || p.numeric.ApplyAtom(atom)
}

func (p *productSatisfiabilityProof) applyRelationalTypeConstraints(constraints []constraint.Constraint) bool {
	for _, c := range constraints {
		keyOf, ok := c.(constraint.KeyOf)
		if !ok {
			continue
		}
		if !p.applyKeyOfTypeConstraint(keyOf) {
			return false
		}
	}
	return true
}

func (p *productSatisfiabilityProof) applyKeyOfTypeConstraint(keyOf constraint.KeyOf) bool {
	tableKey := p.resolvePath(keyOf.Table)
	keyKey := p.resolvePath(keyOf.Key)
	if tableKey == "" || keyKey == "" {
		return true
	}
	tableType := p.typeAt(tableKey)
	if tableType == nil {
		return true
	}

	keyBase := p.typeAt(keyKey)
	tableNarrowed := refineTableByPresentEntryKey(tableType, keyBase)
	if tableNarrowed == nil || tableNarrowed.Kind().IsNever() {
		return false
	}
	if p.types != nil && !typ.TypeEquals(tableType, tableNarrowed) {
		p.types.Narrowed[tableKey] = tableNarrowed
		tableType = tableNarrowed
	}

	keyDomain := core.EntryKeyType(tableType)
	if keyDomain == nil {
		return false
	}
	if keyDomain.Kind().IsPlaceholder() {
		return true
	}
	narrowed := keyDomain
	if keyBase != nil {
		if !keyTypesOverlap(keyBase, keyDomain) {
			return false
		}
		narrowed = narrow.Intersect(keyBase, keyDomain)
	}
	if narrowed == nil || narrowed.Kind().IsNever() {
		return false
	}
	if p.types != nil {
		p.types.Narrowed[keyKey] = narrowed
	}
	return true
}

func (p *productSatisfiabilityProof) applyStructuralConstraints(constraints []constraint.Constraint) bool {
	for _, c := range constraints {
		if _, ok := c.(constraint.KeyOf); ok {
			continue
		}
		if !p.applyStructuralConstraint(c) {
			return false
		}
	}
	return true
}

func (p *productSatisfiabilityProof) applyStructuralConstraint(c constraint.Constraint) bool {
	targets := p.constraintTargetKeys(c)
	if len(targets) == 0 {
		return true
	}
	solver := constraint.Solver{Env: p.structuralEnv}
	for _, target := range targets {
		base := p.typeAt(target)
		if base == nil {
			continue
		}
		narrowed := solver.ApplyToSingleWithEnv([]constraint.Constraint{c}, target, base)
		if narrowed == nil || narrowed.Kind().IsNever() {
			return false
		}
		if !typ.TypeEquals(narrowed, base) {
			p.shapes[target] = narrowed
		}
	}
	return true
}

func (p *productSatisfiabilityProof) propagateEqualityTypes() bool {
	if p.equalities == nil || p.equalities.IsEmpty() {
		return true
	}
	classTypes := make(map[constraint.PathKey]typ.Type)
	for _, key := range p.equalities.AllPaths() {
		root := p.equalities.Find(key)
		t := p.typeAt(key)
		if t == nil {
			continue
		}
		if existing, ok := classTypes[root]; ok {
			if !narrow.TypesOverlap(existing, t) {
				return false
			}
			intersection := narrow.Intersect(existing, t)
			if intersection == nil || intersection.Kind().IsNever() {
				return false
			}
			classTypes[root] = intersection
			continue
		}
		classTypes[root] = t
	}
	for _, key := range p.equalities.AllPaths() {
		root := p.equalities.Find(key)
		if classType, ok := classTypes[root]; ok && p.types != nil {
			p.types.Narrowed[key] = classType
		}
	}
	return true
}

func (p *productSatisfiabilityProof) constraintTargetKeys(c constraint.Constraint) []constraint.PathKey {
	return newConstraintPathEvidence(c, p.resolvePath).Keys()
}

func (p *productSatisfiabilityProof) typeAt(key constraint.PathKey) typ.Type {
	if p == nil || p.source == nil || key == "" {
		return nil
	}
	base := p.typeBaseForTypeDomain(key)
	if p.types != nil {
		if narrowed := p.types.NarrowedTypeAt(key); narrowed != nil {
			return combineProofTypes(narrowed, p.shapes[key])
		}
	}
	return base
}

func (p *productSatisfiabilityProof) typeBaseForTypeDomain(key constraint.PathKey) typ.Type {
	if p == nil || p.source == nil || key == "" {
		return nil
	}
	var base typ.Type
	if p.source != nil {
		base = p.source.TypeAt(key)
	}
	return combineProofTypes(base, p.shapes[key])
}

func (p *productSatisfiabilityProof) resolvePath(path constraint.Path) constraint.PathKey {
	if path.IsEmpty() {
		return ""
	}
	if p != nil && p.env.HasPathResolver() {
		return p.env.ResolvePathKey(path)
	}
	return path.Key()
}

func combineProofTypes(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return narrow.Intersect(a, b)
}
