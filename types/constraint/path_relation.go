package constraint

// PathRelationKind is the normalized semantic kind of a direct relation between
// two paths.
type PathRelationKind uint8

const (
	PathRelationInvalid PathRelationKind = iota
	PathRelationEqual
	PathRelationNotEqual
)

// PathRelation is the canonical view for constraints that relate two paths
// directly. The concrete constraint constructors still own canonical operand
// ordering; this view only exposes the relation meaning to consumers.
type PathRelation struct {
	Left  Path
	Right Path
	Kind  PathRelationKind
}

type pathRelationResult struct {
	relation PathRelation
	ok       bool
}

// DirectPathRelation returns the normalized path relation represented by c.
// Constraints involving literals, fields, indexes, variants, or key-presence
// return false.
func DirectPathRelation(c Constraint) (PathRelation, bool) {
	result := VisitConstraint(c, ConstraintVisitor[pathRelationResult]{
		EqPath: func(v EqPath) pathRelationResult {
			return pathRelationResult{
				relation: PathRelation{Left: v.Left, Right: v.Right, Kind: PathRelationEqual},
				ok:       true,
			}
		},
		NotEqPath: func(v NotEqPath) pathRelationResult {
			return pathRelationResult{
				relation: PathRelation{Left: v.Left, Right: v.Right, Kind: PathRelationNotEqual},
				ok:       true,
			}
		},
		Default: func(Constraint) pathRelationResult {
			return pathRelationResult{}
		},
	})
	return result.relation, result.ok
}

// IsEquality reports whether this relation is equality or inequality.
func (r PathRelation) IsEquality() (bool, bool) {
	switch r.Kind {
	case PathRelationEqual:
		return true, true
	case PathRelationNotEqual:
		return false, true
	default:
		return false, false
	}
}

// SpecializePathRelationConstraint rewrites a direct path relation into the
// most specific relation carrier when either side is a static field or index
// member path. Non-direct relations and already-specialized constraints are
// returned unchanged.
func SpecializePathRelationConstraint(c Constraint) Constraint {
	relation, ok := DirectPathRelation(c)
	if !ok {
		return c
	}
	equality, ok := relation.IsEquality()
	if !ok {
		return c
	}
	if equality {
		if target, field, ok := SplitFieldPath(relation.Left); ok {
			return FieldEqualsPath{Target: target, Field: field, Value: relation.Right}
		}
		if target, field, ok := SplitFieldPath(relation.Right); ok {
			return FieldEqualsPath{Target: target, Field: field, Value: relation.Left}
		}
		if target, key, ok := SplitIndexPath(relation.Left); ok {
			return IndexEqualsPath{Target: target, Key: key, Value: relation.Right}
		}
		if target, key, ok := SplitIndexPath(relation.Right); ok {
			return IndexEqualsPath{Target: target, Key: key, Value: relation.Left}
		}
		return c
	}
	if target, field, ok := SplitFieldPath(relation.Left); ok {
		return FieldNotEqualsPath{Target: target, Field: field, Value: relation.Right}
	}
	if target, field, ok := SplitFieldPath(relation.Right); ok {
		return FieldNotEqualsPath{Target: target, Field: field, Value: relation.Left}
	}
	if target, key, ok := SplitIndexPath(relation.Left); ok {
		return IndexNotEqualsPath{Target: target, Key: key, Value: relation.Right}
	}
	if target, key, ok := SplitIndexPath(relation.Right); ok {
		return IndexNotEqualsPath{Target: target, Key: key, Value: relation.Left}
	}
	return c
}

// SpecializePathRelationConstraints applies SpecializePathRelationConstraint to
// each constraint in a conjunction.
func SpecializePathRelationConstraints(conj []Constraint) []Constraint {
	if len(conj) == 0 {
		return conj
	}
	out := make([]Constraint, 0, len(conj))
	for _, c := range conj {
		out = append(out, SpecializePathRelationConstraint(c))
	}
	return out
}
