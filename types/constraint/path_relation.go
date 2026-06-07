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
