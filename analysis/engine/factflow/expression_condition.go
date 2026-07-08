package factflow

import "github.com/wippyai/go-lua/analysis/domain/path"

// ExpressionCondition describes path facts selected by the boolean value of an
// expression.
type ExpressionCondition struct {
	trueFacts  ExpressionConditionFacts
	falseFacts ExpressionConditionFacts
}

// ExpressionConditionFacts is the atomic fact bundle selected by one boolean
// result of an expression condition.
type ExpressionConditionFacts struct {
	refinements   []PostconditionRefinement
	pathRelations []PostconditionPathRelation
}

// NewExpressionCondition creates an expression condition fact.
func NewExpressionCondition(
	trueRefinements []PostconditionRefinement,
	falseRefinements []PostconditionRefinement,
	truePathRelations []PostconditionPathRelation,
	falsePathRelations []PostconditionPathRelation,
) ExpressionCondition {
	return ExpressionCondition{
		trueFacts:  NewExpressionConditionFacts(trueRefinements, truePathRelations),
		falseFacts: NewExpressionConditionFacts(falseRefinements, falsePathRelations),
	}
}

// NewExpressionConditionFacts creates the selected fact bundle for one boolean
// result of an expression condition.
func NewExpressionConditionFacts(
	refinements []PostconditionRefinement,
	pathRelations []PostconditionPathRelation,
) ExpressionConditionFacts {
	return ExpressionConditionFacts{
		refinements:   copyPostconditionRefinementSlice(refinements),
		pathRelations: copyPostconditionPathRelationSlice(pathRelations),
	}
}

// IsEmpty reports whether c carries no selectable facts.
func (c ExpressionCondition) IsEmpty() bool {
	return c.trueFacts.IsEmpty() && c.falseFacts.IsEmpty()
}

// FactsForValue returns the atomic facts selected by expression value.
func (c ExpressionCondition) FactsForValue(value bool) ExpressionConditionFacts {
	if value {
		return c.trueFacts
	}
	return c.falseFacts
}

// BranchRefinementsForValue converts expression-selected postconditions into
// branch-edge refinements when the branch true edge means the expression has
// the given boolean value.
func (c ExpressionCondition) BranchRefinementsForValue(trueEdgeValue bool) []BranchRefinement {
	trueFacts := c.FactsForValue(trueEdgeValue)
	falseFacts := c.FactsForValue(!trueEdgeValue)
	return branchRefinementsFromPostconditions(trueFacts.refinements, falseFacts.refinements)
}

// BranchPathRelationsForValue converts expression-selected path relations into
// branch-edge path relations when the branch true edge means the expression has
// the given boolean value.
func (c ExpressionCondition) BranchPathRelationsForValue(trueEdgeValue bool) []BranchPathRelation {
	trueFacts := c.FactsForValue(trueEdgeValue)
	falseFacts := c.FactsForValue(!trueEdgeValue)
	return branchPathRelationsFromPostconditions(trueFacts.pathRelations, falseFacts.pathRelations)
}

// BranchPathEvidenceForValue converts expression-selected path relations into
// branch-edge path evidence when the branch true edge means the expression has
// the given boolean value.
func (c ExpressionCondition) BranchPathEvidenceForValue(trueEdgeValue bool) []BranchPathEvidence {
	trueFacts := c.FactsForValue(trueEdgeValue)
	falseFacts := c.FactsForValue(!trueEdgeValue)
	return branchPathEvidenceFromPostconditions(trueFacts.pathRelations, falseFacts.pathRelations)
}

// IsEmpty reports whether f carries no selected facts.
func (f ExpressionConditionFacts) IsEmpty() bool {
	return len(f.refinements) == 0 && len(f.pathRelations) == 0
}

// Refinements returns value refinements selected with this fact bundle.
func (f ExpressionConditionFacts) Refinements() []PostconditionRefinement {
	return copyPostconditionRefinementSlice(f.refinements)
}

// PathRelations returns path relations selected with this fact bundle.
func (f ExpressionConditionFacts) PathRelations() []PostconditionPathRelation {
	return copyPostconditionPathRelationSlice(f.pathRelations)
}

func (c ExpressionCondition) copy() ExpressionCondition {
	return ExpressionCondition{
		trueFacts:  c.trueFacts.copy(),
		falseFacts: c.falseFacts.copy(),
	}
}

func (f ExpressionConditionFacts) copy() ExpressionConditionFacts {
	return NewExpressionConditionFacts(f.refinements, f.pathRelations)
}

func branchRefinementsFromPostconditions(
	trueRefinements []PostconditionRefinement,
	falseRefinements []PostconditionRefinement,
) []BranchRefinement {
	byPath := make(map[path.PathKey]*branchRefinementBuilder, len(trueRefinements)+len(falseRefinements))
	var order []path.PathKey
	add := func(ref PostconditionRefinement, edge bool) {
		key := ref.TargetPathRef().Key()
		builder := byPath[key]
		if builder == nil {
			builder = &branchRefinementBuilder{target: ref.TargetPathRef()}
			byPath[key] = builder
			order = append(order, key)
		}
		if edge {
			builder.trueValue = ref.Value()
			builder.hasTrue = true
		} else {
			builder.falseValue = ref.Value()
			builder.hasFalse = true
		}
	}
	for _, ref := range trueRefinements {
		add(ref, true)
	}
	for _, ref := range falseRefinements {
		add(ref, false)
	}
	out := make([]BranchRefinement, 0, len(order))
	for _, key := range order {
		builder := byPath[key]
		out = append(out, NewBranchRefinement(
			builder.target,
			builder.trueValue,
			builder.hasTrue,
			builder.falseValue,
			builder.hasFalse,
		))
	}
	return out
}

type branchRefinementBuilder struct {
	target     path.Path
	trueValue  ValueRefinement
	hasTrue    bool
	falseValue ValueRefinement
	hasFalse   bool
}

func branchPathRelationsFromPostconditions(
	trueRelations []PostconditionPathRelation,
	falseRelations []PostconditionPathRelation,
) []BranchPathRelation {
	out := make([]BranchPathRelation, 0, 2*(len(trueRelations)+len(falseRelations)))
	for _, relation := range trueRelations {
		out = append(out, branchPathRelationsForPostcondition(relation, true)...)
	}
	for _, relation := range falseRelations {
		out = append(out, branchPathRelationsForPostcondition(relation, false)...)
	}
	return out
}

func branchPathRelationsForPostcondition(relation PostconditionPathRelation, edge bool) []BranchPathRelation {
	switch relation.Kind() {
	case PostconditionPathRelationEqual:
		left := relation.LeftPath()
		right := relation.RightPath()
		return []BranchPathRelation{
			NewBranchPathEquality(left, right, edge, !edge),
			NewBranchPathInequality(left, right, !edge, edge),
		}
	default:
		return nil
	}
}

func branchPathEvidenceFromPostconditions(
	trueRelations []PostconditionPathRelation,
	falseRelations []PostconditionPathRelation,
) []BranchPathEvidence {
	out := make([]BranchPathEvidence, 0, 2*(len(trueRelations)+len(falseRelations)))
	for _, relation := range trueRelations {
		out = append(out, branchPathEvidenceForPostcondition(relation, true)...)
	}
	for _, relation := range falseRelations {
		out = append(out, branchPathEvidenceForPostcondition(relation, false)...)
	}
	return out
}

func branchPathEvidenceForPostcondition(relation PostconditionPathRelation, edge bool) []BranchPathEvidence {
	switch relation.Kind() {
	case PostconditionPathRelationEqual:
		left := relation.LeftPath()
		right := relation.RightPath()
		return []BranchPathEvidence{
			NewBranchPathEqualityEvidenceOnEdge(left, right, edge),
			NewBranchPathInequalityEvidenceOnEdge(left, right, !edge),
		}
	default:
		return nil
	}
}
