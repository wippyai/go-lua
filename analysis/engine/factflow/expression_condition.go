package factflow

// ExpressionCondition describes path facts selected by the boolean value of an
// expression.
type ExpressionCondition struct {
	trueRefinements  []PostconditionRefinement
	falseRefinements []PostconditionRefinement

	truePathRelations  []PostconditionPathRelation
	falsePathRelations []PostconditionPathRelation
}

// NewExpressionCondition creates an expression condition fact.
func NewExpressionCondition(
	trueRefinements []PostconditionRefinement,
	falseRefinements []PostconditionRefinement,
	truePathRelations []PostconditionPathRelation,
	falsePathRelations []PostconditionPathRelation,
) ExpressionCondition {
	return ExpressionCondition{
		trueRefinements:    copyPostconditionRefinementSlice(trueRefinements),
		falseRefinements:   copyPostconditionRefinementSlice(falseRefinements),
		truePathRelations:  copyPostconditionPathRelationSlice(truePathRelations),
		falsePathRelations: copyPostconditionPathRelationSlice(falsePathRelations),
	}
}

// IsEmpty reports whether c carries no selectable facts.
func (c ExpressionCondition) IsEmpty() bool {
	return len(c.trueRefinements) == 0 &&
		len(c.falseRefinements) == 0 &&
		len(c.truePathRelations) == 0 &&
		len(c.falsePathRelations) == 0
}

// RefinementsForValue returns value refinements selected by expression value.
func (c ExpressionCondition) RefinementsForValue(value bool) []PostconditionRefinement {
	if value {
		return copyPostconditionRefinementSlice(c.trueRefinements)
	}
	return copyPostconditionRefinementSlice(c.falseRefinements)
}

// PathRelationsForValue returns path relations selected by expression value.
func (c ExpressionCondition) PathRelationsForValue(value bool) []PostconditionPathRelation {
	if value {
		return copyPostconditionPathRelationSlice(c.truePathRelations)
	}
	return copyPostconditionPathRelationSlice(c.falsePathRelations)
}

func (c ExpressionCondition) copy() ExpressionCondition {
	return NewExpressionCondition(
		c.trueRefinements,
		c.falseRefinements,
		c.truePathRelations,
		c.falsePathRelations,
	)
}

func copyExpressionConditionMap(in map[ExprRef]ExpressionCondition) map[ExprRef]ExpressionCondition {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ExpressionCondition, len(in))
	for ref, condition := range in {
		out[ref] = condition.copy()
	}
	return out
}
