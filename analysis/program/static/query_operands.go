package static

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (view View) Operands() Operands { return Operands{component: view.component, state: view.state} }
func (view Operands) Claims() ClaimTargets {
	return ClaimTargets{component: view.component, state: view.state}
}
func (view Operands) TypeValues() TypeValueTargets {
	return TypeValueTargets{component: view.component, state: view.state}
}
func (view Operands) Annotations() Annotations {
	return Annotations{component: view.component, state: view.state}
}

// Count is the sparse semantic ClaimTarget denominator: only claims with an
// authored static target participate.
func (view ClaimTargets) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operands.claims)
}
func (view ClaimTargets) At(index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || index < 0 || index >= len(component.operands.claims) {
		return 0, false
	}
	return component.operands.claims[index].claim, true
}

// Target reports one member of the sparse semantic ClaimTarget relation. A
// false result means this Claim has no authored static target. Callers that
// need to distinguish a known Flow claim from an unknown identity query Flow;
// this relation must not overload its presence bit with Flow membership.
func (view ClaimTargets) Target(claim keyspace.Term) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(claim, keyspace.FamilyValueClaim, len(component.operands.claimTargets)) {
		return 0, false
	}
	target := component.operands.claimTargets[keyspace.TermOrdinal(claim)-1]
	return target, target != 0
}

func (view TypeValueTargets) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operands.typeValues)
}
func (view TypeValueTargets) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeValue, index, view.Count())
}
func (view TypeValueTargets) Target(term keyspace.Term) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeValue, len(component.operands.typeValues)) {
		return 0, false
	}
	return component.operands.typeValues[keyspace.TermOrdinal(term)-1], true
}

func (view Annotations) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operands.annotations)
}
func (view Annotations) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyAnnotation, index, view.Count())
}
func (view Annotations) Get(term keyspace.Term) (Annotation, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyAnnotation, len(component.operands.annotations)) {
		return Annotation{}, false
	}
	return component.operands.annotations[keyspace.TermOrdinal(term)-1], true
}

// ForCount distinguishes a valid target with no annotations (0, true) from
// an invalid target (0, false). The CSR is only an allocation-free query
// index; Annotation rows remain the sole semantic relation.
func (view Annotations) ForCount(target keyspace.Term) (int, bool) {
	component := view.componentOf()
	if component == nil || !annotationTargetPresent(component, target) {
		return 0, false
	}
	index := annotationTargetIndex(component.operands.annotationTargets, target)
	if index < 0 {
		return 0, true
	}
	range_ := component.operands.annotationRanges[index]
	return int(range_.End - range_.Start), true
}
func (view Annotations) ForAt(target keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || index < 0 || !annotationTargetPresent(component, target) {
		return 0, false
	}
	position := annotationTargetIndex(component.operands.annotationTargets, target)
	if position < 0 {
		return 0, false
	}
	range_ := component.operands.annotationRanges[position]
	if uint32(index) >= range_.End-range_.Start {
		return 0, false
	}
	return component.operands.annotationTerms[range_.Start+uint32(index)], true
}

func annotationTargetIndex(targets []keyspace.Term, target keyspace.Term) int {
	index := sort.Search(len(targets), func(index int) bool { return targets[index] >= target })
	if index == len(targets) || targets[index] != target {
		return -1
	}
	return index
}
