package constraint

import "github.com/wippyai/go-lua/types/narrow"

// PathPredicateKind is the normalized semantic kind of a single-path
// refinement. It lets consumers reason about predicate meaning without
// enumerating the concrete constraint structs that encode it.
type PathPredicateKind uint8

const (
	PathPredicateInvalid PathPredicateKind = iota
	PathPredicateTruthy
	PathPredicateFalsy
	PathPredicateIsNil
	PathPredicateNotNil
	PathPredicateHasType
	PathPredicateNotHasType
)

// PathPredicate is the canonical view for constraints that refine exactly one
// path by truthiness, nilness, or type membership.
type PathPredicate struct {
	Path Path
	Kind PathPredicateKind
	Type narrow.TypeKey
}

type pathPredicateResult struct {
	p  PathPredicate
	ok bool
}

// SinglePathPredicate returns the normalized single-path predicate represented
// by c. Constraints that relate multiple paths or literals return false.
func SinglePathPredicate(c Constraint) (PathPredicate, bool) {
	result := VisitConstraint(c, ConstraintVisitor[pathPredicateResult]{
		Truthy: func(v Truthy) pathPredicateResult {
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateTruthy}, true}
		},
		Falsy: func(v Falsy) pathPredicateResult {
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateFalsy}, true}
		},
		IsNil: func(v IsNil) pathPredicateResult {
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateIsNil}, true}
		},
		NotNil: func(v NotNil) pathPredicateResult {
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateNotNil}, true}
		},
		HasType: func(v HasType) pathPredicateResult {
			if v.Type.IsZero() {
				return pathPredicateResult{}
			}
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateHasType, Type: v.Type}, true}
		},
		NotHasType: func(v NotHasType) pathPredicateResult {
			if v.Type.IsZero() {
				return pathPredicateResult{}
			}
			return pathPredicateResult{PathPredicate{Path: v.Path, Kind: PathPredicateNotHasType, Type: v.Type}, true}
		},
		Default: func(Constraint) pathPredicateResult {
			return pathPredicateResult{}
		},
	})
	return result.p, result.ok
}

// NonNilBranch reports the nil/truthiness side represented by this predicate.
// true means a non-nil/truthy side; false means a nil/falsy side.
func (p PathPredicate) NonNilBranch() (bool, bool) {
	switch p.Kind {
	case PathPredicateTruthy, PathPredicateNotNil:
		return true, true
	case PathPredicateFalsy, PathPredicateIsNil:
		return false, true
	default:
		return false, false
	}
}
