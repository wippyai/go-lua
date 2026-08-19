package containment

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// emitStaticContainment emits the Static-owned rows which close the one
// containment relation.  Static's local proof is a typed relation, not a
// generic graph: this pass merely projects each already-proved row into the
// coordinator's private kernel input.  Scope and occurrence forwarding are
// emitted separately by emitStaticFallbacks; no expression relation is
// reconstructed here.
func emitStaticContainment(
	preimage source.Preimage,
	staticView staticquery.View,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	result *emission,
) (*staticScopeResolver, error) {
	if result == nil {
		return nil, errors.New("program/flow/containment: nil Static emission")
	}
	if !staticView.Available() {
		return nil, errors.New("program/flow/containment: Static view expired")
	}
	// LocalContainment is a construction capability owned by this exact Static
	// view. Deriving it here prevents a caller from splicing a proof from a
	// different draft/lifecycle into the canonical relation. A terminal view
	// fails closed through Available above and cannot yield local rows.
	local := staticView.LocalContainment()
	if err := emitStaticLocalParents(result, local, counts); err != nil {
		return nil, err
	}
	resolver := newStaticScopeResolver(staticView, view, counts)
	if err := emitStaticCrossParents(result, staticView, view, local, resolver, counts); err != nil {
		return nil, err
	}

	fallback, roots, err := emitStaticFallbacks(preimage, staticView, view, resolver, counts)
	if err != nil {
		return nil, err
	}
	result.fallback = fallback
	result.static, err = emitStaticMarks(preimage, staticView, view, roots, counts)
	if err != nil {
		return nil, err
	}
	return resolver, nil
}

func emitStaticLocalParents(result *emission, local staticquery.LocalContainment, counts [keyspace.FamilyCount]uint32) error {
	if result == nil {
		return errors.New("program/flow/containment: nil Static emission")
	}
	for index := 0; index < local.Count(); index++ {
		child, ok := local.At(index)
		if !ok || !validTerm(child, counts) {
			return errors.New("program/flow/containment: invalid Static local child")
		}
		parent, hasParent := local.Parent(child)
		if !hasParent {
			if parent != 0 {
				return errors.New("program/flow/containment: Static local root has a parent handle")
			}
			continue
		}
		if !validTerm(parent, counts) || parent == child {
			return errors.New("program/flow/containment: invalid Static local parent")
		}
		result.edges = append(result.edges, kernelEdge{child: child, parent: parent})
	}
	return nil
}

// emitStaticCrossParents closes relations whose endpoint is owned by Static
// but whose parent is Source/Flow geometry.  It intentionally emits no Alias
// or Interface source edge: Source owns those direct lexical occurrences.
func emitStaticCrossParents(
	result *emission,
	staticView staticquery.View,
	view authored.View,
	local staticquery.LocalContainment,
	resolver *staticScopeResolver,
	counts [keyspace.FamilyCount]uint32,
) error {
	if result == nil || resolver == nil {
		return errors.New("program/flow/containment: invalid Static cross-owner emitter")
	}
	// Scope roots are ranked within their lexical Body and relation family;
	// the rank is a semantic sibling slot, never a published Term ordinal.
	scopeRanks := make(map[keyspace.Term]uint32)
	nextScopeRank := make(map[keyspace.Term]uint32)
	scopeEdge := func(child, parent keyspace.Term) kernelEdge {
		rank, ok := scopeRanks[child]
		if !ok {
			nextScopeRank[parent]++
			rank = nextScopeRank[parent]
			scopeRanks[child] = rank
		}
		return kernelEdge{child: child, parent: parent, role: structuralRoleStaticScope, rank: rank}
	}

	// Type parameters have an explicit declared owner. Static's local proof
	// does not own this declaration edge, so it is installed here unless a
	// future Static version has already materialized the same exact row.
	params := staticView.Declarations().TypeParams()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeParam]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeParam, ordinal)
		owner, _, _, ok := params.Get(term)
		if !ok || !staticrole.TypeParameterOwner(counts, owner) {
			return errors.New("program/flow/containment: invalid TypeParam owner")
		}
		parent, hasParent := local.Parent(term)
		if hasParent {
			if parent != owner {
				return errors.New("program/flow/containment: TypeParam local owner disagrees with declaration")
			}
			continue
		}
		result.edges = append(result.edges, kernelEdge{child: term, parent: owner})
	}

	// Fields have two independent typed relations. FieldOwner is the owner of
	// the field itself; its value type is already represented by Local.Parent.
	fields := staticView.Types().Fields()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTypeField, ordinal)
		owner, ok := local.FieldOwner(field)
		if !ok || !validTerm(owner, counts) ||
			(keyspace.TermFamily(owner) != keyspace.FamilyTypeRecord && keyspace.TermFamily(owner) != keyspace.FamilyTypeInterface) {
			return errors.New("program/flow/containment: invalid Field owner")
		}
		result.edges = append(result.edges, kernelEdge{child: field, parent: owner})
		if _, _, _, ok := fields.Get(field); !ok {
			return errors.New("program/flow/containment: invalid Field row")
		}
	}

	declared := staticView.Declarations().DeclaredTypes()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyDeclaredType]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyDeclaredType, ordinal)
		cell, target, ok := declared.Get(term)
		if !ok || !termInFamily(cell, keyspace.FamilyCell, counts) ||
			!staticrole.AnnotationTarget(counts, target) || keyspace.TermFamily(target) == keyspace.FamilyTypeField {
			return errors.New("program/flow/containment: invalid DeclaredType relation")
		}
		result.edges = append(result.edges, kernelEdge{child: term, parent: cell})
	}

	publications := staticView.Publications()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypePublication]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypePublication, ordinal)
		assign, _, target, ok := publications.Get(term)
		if !ok || !termInFamily(assign, keyspace.FamilyAssign, counts) || !validTerm(target, counts) ||
			keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return errors.New("program/flow/containment: invalid TypePublication relation")
		}
		result.edges = append(result.edges, kernelEdge{child: term, parent: assign})
	}

	// TypeFunction and TypeOf are static nodes with two possible structural
	// owners. A local parent is authoritative when present (for example an
	// interface method or KeyOf); only a local root forwards through Scope to
	// its lexical Body.
	typeFunctions := staticView.Signatures().TypeFunctions()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeFunction]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeFunction, ordinal)
		scope, _, _, _, ok := typeFunctions.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, scope) {
			return errors.New("program/flow/containment: invalid TypeFunction row")
		}
		parent, hasParent := local.Parent(term)
		if hasParent {
			if !validTerm(parent, counts) {
				return errors.New("program/flow/containment: invalid TypeFunction local parent")
			}
			continue
		}
		body, _, ok := resolver.resolveObservation(scope)
		if !ok {
			return errors.New("program/flow/containment: TypeFunction scope has no lexical Body")
		}
		result.edges = append(result.edges, scopeEdge(term, body))
	}

	typeOfs := staticView.Operators().TypeOfs()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyTypeOf]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeOf, ordinal)
		scope, operand, ok := typeOfs.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, scope) || !flowrole.ValueOccurrence(counts, operand) {
			return errors.New("program/flow/containment: invalid TypeOf row")
		}
		parent, hasParent := local.Parent(term)
		if hasParent {
			if !validTerm(parent, counts) {
				return errors.New("program/flow/containment: invalid TypeOf local parent")
			}
			continue
		}
		body, _, ok := resolver.resolveObservation(scope)
		if !ok {
			return errors.New("program/flow/containment: TypeOf scope has no lexical Body")
		}
		result.edges = append(result.edges, scopeEdge(term, body))
	}

	annotations := staticView.Operands().Annotations()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyAnnotation]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyAnnotation, ordinal)
		row, ok := annotations.Get(term)
		if !ok || !staticrole.ScopeHandle(counts, row.Scope) ||
			!staticrole.AnnotationTarget(counts, row.Target) {
			return errors.New("program/flow/containment: invalid Annotation row")
		}
		// Annotation is a child of its annotated static occurrence. Scope and
		// Values are cross-owner references and are handled by fallback/marks.
		result.edges = append(result.edges, kernelEdge{child: term, parent: row.Target})
	}
	return nil
}
