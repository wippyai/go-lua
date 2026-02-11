// key.go provides path key construction and manipulation utilities.
//
// Path keys are the canonical string identifiers for versioned paths in the
// flow solver. This file provides functions for building, parsing, and
// comparing path keys.
package pathkey

import (
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
)

// SegmentsSuffix converts path segments to a suffix string for key construction.
//
// The suffix format matches Lua syntax:
//   - Field segments: ".field"
//   - Integer index: "[0]"
//   - String index: "[\"key\"]" (with escaping)
//
// This is a thin wrapper around constraint.FormatSegments for API consistency
// within the pathkey package.
func SegmentsSuffix(segs []constraint.Segment) string {
	return constraint.FormatSegments(segs)
}

// ParseSuffix converts a suffix string like ".field" or "[0]" to segments.
//
// This is the inverse of SegmentsSuffix. It parses path suffixes extracted
// from canonical keys back into structured segments.
//
// Supported syntax:
//   - ".field" -> SegmentField{Name: "field"}
//   - "[0]" -> SegmentIndexInt{Index: 0}
//   - "[\"key\"]" -> SegmentIndexString{Name: "key"}
//   - "[key]" -> SegmentIndexString{Name: "key"} (legacy form)
//
// The parser handles multiple chained segments: ".foo[0].bar" produces
// [SegmentField{foo}, SegmentIndexInt{0}, SegmentField{bar}].
func ParseSuffix(suffix string) []constraint.Segment {
	if suffix == "" {
		return nil
	}
	var segs []constraint.Segment
	i := 0
	for i < len(suffix) {
		switch suffix[i] {
		case '.':
			i++
			start := i
			for i < len(suffix) && suffix[i] != '.' && suffix[i] != '[' {
				i++
			}
			if start < i {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: suffix[start:i]})
			}
		case '[':
			i++
			if i >= len(suffix) {
				return nil
			}

			if suffix[i] == '"' {
				i++
				var out []byte
				for i < len(suffix) {
					ch := suffix[i]
					if ch == '\\' {
						if i+1 >= len(suffix) {
							return nil
						}
						escaped := suffix[i+1]
						if escaped != '\\' && escaped != '"' {
							return nil
						}
						out = append(out, escaped)
						i += 2
						continue
					}
					if ch == '"' {
						break
					}
					out = append(out, ch)
					i++
				}

				if i >= len(suffix) || suffix[i] != '"' {
					return nil
				}
				i++
				if i >= len(suffix) || suffix[i] != ']' {
					return nil
				}
				i++

				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: string(out)})
				continue
			}

			start := i
			for i < len(suffix) && suffix[i] != ']' {
				i++
			}
			if start >= i {
				return nil
			}

			name := suffix[start:i]
			if i >= len(suffix) || suffix[i] != ']' {
				return nil
			}
			i++

			if idx, ok := ParseIntLiteral(name); ok {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
			} else {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: name})
			}
		default:
			return nil
		}
	}
	return segs
}

// splitVersionKey splits a version key into root and suffix.
//
// Given "sym42@3.field[0]", returns ("sym42", ".field[0]", true).
// The root is everything before @, and the suffix starts at the first
// '.' or '[' after the version number.
//
// Returns ("", "", false) if the key has no @ separator.
func splitVersionKey(key string) (root string, suffix string, ok bool) {
	at := strings.IndexByte(key, '@')
	if at <= 0 {
		return "", "", false
	}
	root = key[:at]
	start := at + 1
	for start < len(key) {
		ch := key[start]
		if ch == '.' || ch == '[' {
			break
		}
		start++
	}
	if start >= len(key) {
		return root, "", true
	}
	return root, key[start:], true
}

// PathRelated returns true if target and other paths share identity and overlap.
//
// Two paths are related if:
//  1. They share the same symbol (SSA identity) or both are placeholders with
//     the same Root name
//  2. One is a prefix of the other (including equality)
//
// This relationship is used for constraint filtering: a constraint on x.foo
// is related to path x (constraint might narrow x) and to path x.foo.bar
// (constraint directly applies).
//
// Strictly requires symbol matching - no name-based fallback. If one path has
// a symbol and the other doesn't, they are not related. This prevents unsound
// constraint propagation across different scopes.
func PathRelated(target constraint.Path, other constraint.Path) bool {
	if target.IsEmpty() || other.IsEmpty() {
		return false
	}

	// Both must have symbols, and they must match
	if target.Symbol != 0 && other.Symbol != 0 {
		if target.Symbol != other.Symbol {
			return false
		}
		if target.Version != 0 && other.Version != 0 && target.Version != other.Version {
			return false
		}
	} else if target.Symbol == 0 && other.Symbol == 0 {
		// Both are placeholders - compare by Root
		if target.Root != other.Root {
			return false
		}
	} else {
		// One has symbol, one doesn't - NOT related (no name fallback)
		return false
	}

	return SegmentsPrefix(target.Segments, other.Segments) || SegmentsPrefix(other.Segments, target.Segments)
}

// SegmentsPrefix returns true if a is a prefix of b (or equal).
//
// Segment comparison is exact: both kind and value must match.
// An empty slice is a prefix of any slice.
func SegmentsPrefix(a, b []constraint.Segment) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// FilterConstraintsForPath returns constraints relevant to a target path.
//
// A constraint is kept if it references the target path or an equivalent path
// (via EqPath constraints). This filtering is used before applying constraints
// to narrow a specific path, avoiding application of unrelated constraints.
//
// The function handles several cases:
//   - Direct reference: Constraint path related to target
//   - Equivalence: Constraint path equivalent to target via EqPath
//   - Field/Index relationships: Value paths from FieldEqualsPath constraints
//
// Special handling for NotEquals constraints: They narrow the target, not the
// value side. If filtering for a value-only path, NotEquals are dropped.
func FilterConstraintsForPath(constraints []constraint.Constraint, target constraint.Path) []constraint.Constraint {
	if len(constraints) == 0 || target.IsEmpty() {
		return constraints
	}

	equivalentPaths := CollectEquivalentPaths(constraints, target)
	// If target is used in Field/Index EqualsPath constraints, keep constraints on the value path too.
	relatedValuePaths := make(map[constraint.PathKey]struct{})
	for _, c := range constraints {
		constraint.VisitConstraint(c, constraint.ConstraintVisitor[struct{}]{
			FieldEqualsPath: func(v constraint.FieldEqualsPath) struct{} {
				if PathRelated(target, v.Target) {
					relatedValuePaths[v.Value.Key()] = struct{}{}
				}
				return struct{}{}
			},
			IndexEqualsPath: func(v constraint.IndexEqualsPath) struct{} {
				if PathRelated(target, v.Target) {
					relatedValuePaths[v.Value.Key()] = struct{}{}
				}
				return struct{}{}
			},
		})
	}

	var filtered []constraint.Constraint
	for _, c := range constraints {
		keep := false
		// Field/Index NotEquals constraints are asymmetric: they narrow the target,
		// not the value. If we're filtering for the value side only, drop them.
		if drop := constraint.VisitConstraint(c, constraint.ConstraintVisitor[bool]{
			FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) bool {
				valueRelated := PathRelated(target, v.Value)
				targetRelated := PathRelated(target, v.Target)
				return valueRelated && !targetRelated
			},
			IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) bool {
				valueRelated := PathRelated(target, v.Value)
				targetRelated := PathRelated(target, v.Target)
				return valueRelated && !targetRelated
			},
			Default: func(constraint.Constraint) bool {
				return false
			},
		}); drop {
			continue
		}
		for _, p := range c.Paths() {
			if PathRelated(target, p) {
				keep = true
				break
			}
			if _, isEquiv := equivalentPaths[p.Key()]; isEquiv {
				keep = true
				break
			}
			if _, isRelated := relatedValuePaths[p.Key()]; isRelated {
				keep = true
				break
			}
		}
		if keep {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// CollectEquivalentPaths finds all paths equivalent to target via equality constraints.
//
// Equivalence is transitive: if x == y and y == z, then x, y, and z are all
// equivalent. The function uses a fixed-point algorithm to compute the full
// equivalence closure.
//
// Supported constraint types for equivalence:
//   - EqPath: x == y means x and y are equivalent
//   - FieldEqualsPath: x.field == y means x.field and y are equivalent
//   - IndexEqualsPath: x[i] == y means x[i] and y are equivalent
//
// Returns a set of PathKeys that are transitively equivalent to target.
func CollectEquivalentPaths(constraints []constraint.Constraint, target constraint.Path) map[constraint.PathKey]bool {
	result := make(map[constraint.PathKey]bool)
	result[target.Key()] = true

	type eqPair struct {
		left, right constraint.Path
	}
	var pairs []eqPair

	for _, c := range constraints {
		constraint.VisitConstraint(c, constraint.ConstraintVisitor[struct{}]{
			EqPath: func(v constraint.EqPath) struct{} {
				pairs = append(pairs, eqPair{v.Left, v.Right})
				return struct{}{}
			},
			FieldEqualsPath: func(v constraint.FieldEqualsPath) struct{} {
				fieldPath := v.Target.Append(constraint.Segment{Kind: constraint.SegmentField, Name: v.Field})
				if !fieldPath.IsEmpty() {
					pairs = append(pairs, eqPair{fieldPath, v.Value})
				}
				return struct{}{}
			},
			IndexEqualsPath: func(v constraint.IndexEqualsPath) struct{} {
				pairs = append(pairs, eqPair{v.Target, v.Value})
				return struct{}{}
			},
		})
	}

	if len(pairs) == 0 {
		return result
	}

	changed := true
	for changed {
		changed = false
		for _, pair := range pairs {
			leftKey := pair.left.Key()
			rightKey := pair.right.Key()
			leftIn := result[leftKey]
			rightIn := result[rightKey]
			if leftIn && !rightIn {
				result[rightKey] = true
				changed = true
			} else if rightIn && !leftIn {
				result[leftKey] = true
				changed = true
			}
		}
	}

	return result
}
