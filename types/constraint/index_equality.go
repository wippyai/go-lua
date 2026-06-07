package constraint

import "github.com/wippyai/go-lua/types/typ"

// IndexLiteralEqualityConstraints lowers a dynamic-index equality against a
// literal value into canonical constraint atoms. Literal union keys are expanded
// so downstream domains see precise per-key facts instead of re-decoding key
// unions at each consumer.
func IndexLiteralEqualityConstraints(target Path, keyType typ.Type, value *typ.Literal) []Constraint {
	if lit, ok := keyType.(*typ.Literal); ok {
		return []Constraint{IndexEquals{Target: target, Key: lit, Value: value}}
	}

	if union, ok := keyType.(*typ.Union); ok {
		var constraints []Constraint
		for _, member := range union.Members {
			if lit, ok := member.(*typ.Literal); ok {
				constraints = append(constraints, IndexEquals{Target: target, Key: lit, Value: value})
			}
		}
		if len(constraints) > 0 {
			return constraints
		}
	}

	return []Constraint{IndexEquals{Target: target, Key: keyType, Value: value}}
}

// IndexPathEqualityConstraints lowers a dynamic-index equality or inequality
// against another path into canonical constraint atoms.
func IndexPathEqualityConstraints(target Path, keyType typ.Type, valuePath Path, equals bool) []Constraint {
	if lit, ok := keyType.(*typ.Literal); ok {
		if equals {
			return []Constraint{IndexEqualsPath{Target: target, Key: lit, Value: valuePath}}
		}
		return []Constraint{IndexNotEqualsPath{Target: target, Key: lit, Value: valuePath}}
	}

	if union, ok := keyType.(*typ.Union); ok {
		var constraints []Constraint
		for _, member := range union.Members {
			if lit, ok := member.(*typ.Literal); ok {
				if equals {
					constraints = append(constraints, IndexEqualsPath{Target: target, Key: lit, Value: valuePath})
				} else {
					constraints = append(constraints, IndexNotEqualsPath{Target: target, Key: lit, Value: valuePath})
				}
			}
		}
		if len(constraints) > 0 {
			return constraints
		}
	}

	if equals {
		return []Constraint{IndexEqualsPath{Target: target, Key: keyType, Value: valuePath}}
	}
	return []Constraint{IndexNotEqualsPath{Target: target, Key: keyType, Value: valuePath}}
}
