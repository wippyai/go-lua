package factflow

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
