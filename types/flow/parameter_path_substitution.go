package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// parameterPathSubstituter converts function-local parameter paths into
// call-site placeholders for interprocedural refinement effects.
type parameterPathSubstituter struct {
	placeholders       map[cfg.SymbolID]string
	placeholdersByName map[string]string
}

func newParameterPathSubstituter(
	placeholders map[cfg.SymbolID]string,
	placeholdersByName map[string]string,
) parameterPathSubstituter {
	return parameterPathSubstituter{
		placeholders:       placeholders,
		placeholdersByName: placeholdersByName,
	}
}

func (s parameterPathSubstituter) constraint(c constraint.Constraint) constraint.Constraint {
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[constraint.Constraint]{
		Truthy:             s.truthy,
		Falsy:              s.falsy,
		IsNil:              s.isNil,
		NotNil:             s.notNil,
		HasType:            s.hasType,
		NotHasType:         s.notHasType,
		HasField:           s.hasField,
		FieldEquals:        s.fieldEquals,
		FieldNotEquals:     s.fieldNotEquals,
		IndexEquals:        s.indexEquals,
		IndexNotEquals:     s.indexNotEquals,
		EqPath:             s.eqPath,
		NotEqPath:          s.notEqPath,
		FieldEqualsPath:    s.fieldEqualsPath,
		FieldNotEqualsPath: s.fieldNotEqualsPath,
		IndexEqualsPath:    s.indexEqualsPath,
		IndexNotEqualsPath: s.indexNotEqualsPath,
		Default: func(constraint.Constraint) constraint.Constraint {
			return nil
		},
	})
}

func (s parameterPathSubstituter) truthy(v constraint.Truthy) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.Truthy{Path: path}
}

func (s parameterPathSubstituter) falsy(v constraint.Falsy) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.Falsy{Path: path}
}

func (s parameterPathSubstituter) isNil(v constraint.IsNil) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.IsNil{Path: path}
}

func (s parameterPathSubstituter) notNil(v constraint.NotNil) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.NotNil{Path: path}
}

func (s parameterPathSubstituter) hasType(v constraint.HasType) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.HasType{Path: path, Type: v.Type}
}

func (s parameterPathSubstituter) notHasType(v constraint.NotHasType) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.NotHasType{Path: path, Type: v.Type}
}

func (s parameterPathSubstituter) hasField(v constraint.HasField) constraint.Constraint {
	path, ok := s.path(v.Path)
	if !ok {
		return nil
	}
	return constraint.HasField{Path: path, Field: v.Field}
}

func (s parameterPathSubstituter) fieldEquals(v constraint.FieldEquals) constraint.Constraint {
	target, ok := s.path(v.Target)
	if !ok {
		return nil
	}
	return constraint.FieldEquals{Target: target, Field: v.Field, Value: v.Value}
}

func (s parameterPathSubstituter) fieldNotEquals(v constraint.FieldNotEquals) constraint.Constraint {
	target, ok := s.path(v.Target)
	if !ok {
		return nil
	}
	return constraint.FieldNotEquals{Target: target, Field: v.Field, Value: v.Value}
}

func (s parameterPathSubstituter) indexEquals(v constraint.IndexEquals) constraint.Constraint {
	target, ok := s.path(v.Target)
	if !ok {
		return nil
	}
	return constraint.IndexEquals{Target: target, Key: v.Key, Value: v.Value}
}

func (s parameterPathSubstituter) indexNotEquals(v constraint.IndexNotEquals) constraint.Constraint {
	target, ok := s.path(v.Target)
	if !ok {
		return nil
	}
	return constraint.IndexNotEquals{Target: target, Key: v.Key, Value: v.Value}
}

func (s parameterPathSubstituter) eqPath(v constraint.EqPath) constraint.Constraint {
	left, right, ok := s.pathPair(v.Left, v.Right)
	if !ok {
		return nil
	}
	return constraint.NewEqPath(left, right)
}

func (s parameterPathSubstituter) notEqPath(v constraint.NotEqPath) constraint.Constraint {
	left, right, ok := s.pathPair(v.Left, v.Right)
	if !ok {
		return nil
	}
	return constraint.NewNotEqPath(left, right)
}

func (s parameterPathSubstituter) fieldEqualsPath(v constraint.FieldEqualsPath) constraint.Constraint {
	target, value, ok := s.pathPair(v.Target, v.Value)
	if !ok {
		return nil
	}
	return constraint.FieldEqualsPath{Target: target, Field: v.Field, Value: value}
}

func (s parameterPathSubstituter) fieldNotEqualsPath(v constraint.FieldNotEqualsPath) constraint.Constraint {
	target, value, ok := s.pathPair(v.Target, v.Value)
	if !ok {
		return nil
	}
	return constraint.FieldNotEqualsPath{Target: target, Field: v.Field, Value: value}
}

func (s parameterPathSubstituter) indexEqualsPath(v constraint.IndexEqualsPath) constraint.Constraint {
	target, value, ok := s.pathPair(v.Target, v.Value)
	if !ok {
		return nil
	}
	return constraint.IndexEqualsPath{Target: target, Key: v.Key, Value: value}
}

func (s parameterPathSubstituter) indexNotEqualsPath(v constraint.IndexNotEqualsPath) constraint.Constraint {
	target, value, ok := s.pathPair(v.Target, v.Value)
	if !ok {
		return nil
	}
	return constraint.IndexNotEqualsPath{Target: target, Key: v.Key, Value: value}
}

func (s parameterPathSubstituter) path(path constraint.Path) (constraint.Path, bool) {
	newRoot, ok := lookupPlaceholder(path, s.placeholders, s.placeholdersByName)
	if !ok {
		return constraint.Path{}, false
	}
	return pathWithNewRoot(path, newRoot), true
}

func (s parameterPathSubstituter) pathPair(left, right constraint.Path) (constraint.Path, constraint.Path, bool) {
	newLeft, leftOk := s.path(left)
	newRight, rightOk := s.path(right)
	switch {
	case leftOk && rightOk:
		return newLeft, newRight, true
	case leftOk:
		return newLeft, right, true
	case rightOk:
		return left, newRight, true
	default:
		return constraint.Path{}, constraint.Path{}, false
	}
}
