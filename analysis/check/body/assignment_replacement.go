package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// OrdinaryWritableTargetType returns the target contract an ordinary assignment
// should be checked against. It owns diagnostic-time widening from a narrowed
// target value back to its full variant family and inferred write family.
func (r *Result) OrdinaryWritableTargetType(current typ.Type, targetValue product.Value, hasValue bool, declared bool) typ.Type {
	if current == nil {
		return nil
	}
	if hasValue {
		if family, familyOK := r.FullVariantOriginType(targetValue); familyOK && family != nil && r.IsSubtype(current, family) {
			return family
		}
	}
	if !declared {
		if base, ok := TypeFamilyBase(current); ok && base != nil {
			return base
		}
	}
	return current
}

// InferredReplacementAccepted reports whether a write to an inferred target is
// an accepted shape replacement rather than a type error. Declared targets never
// use this relaxation.
func (r *Result) InferredReplacementAccepted(point cfg.Point, target OrdinaryAssignmentTargetType, expected, actual typ.Type) bool {
	if expected == nil || actual == nil {
		return false
	}
	if target.Declared {
		return false
	}
	if typ.TypeEquals(actual, typ.Nil) {
		return true
	}
	if target.RetypeAllowed && scalarRetypeReplacementAccepted(expected, actual) {
		return true
	}
	if inferredNumericReplacementAccepted(expected, actual) {
		return true
	}
	if _, ok := unwrap.Annotated(expected).(*typ.Function); ok {
		_, actualOK := unwrap.Annotated(actual).(*typ.Function)
		return actualOK
	}
	if _, ok := unwrap.Annotated(expected).(*typ.Record); ok {
		localExclusive := target.HasValue && r != nil && r.ValueHasLocalExclusiveExactIdentity(point, target.TargetValue)
		return inferredRecordReplacementAccepted(actual, localExclusive)
	}
	return false
}

func scalarRetypeReplacementAccepted(expected, actual typ.Type) bool {
	return scalarRetypeType(expected) && scalarRetypeType(actual)
}

func scalarRetypeType(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch tt := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return scalarRetypeType(tt.Inner)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !scalarRetypeType(member) {
				return false
			}
		}
		return true
	default:
		switch tt.Kind() {
		case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
			return true
		default:
			return false
		}
	}
}

func inferredNumericReplacementAccepted(expected, actual typ.Type) bool {
	return typ.TypeEquals(unwrap.Annotated(expected), typ.Integer) && typ.TypeEquals(unwrap.Annotated(actual), typ.Number)
}

func inferredRecordReplacementAccepted(actual typ.Type, localExclusive bool) bool {
	ok, hasBroadTable, hasConcreteRecord := inferredRecordReplacementSurface(actual)
	return ok && (hasBroadTable || (localExclusive && hasConcreteRecord))
}

func inferredRecordReplacementSurface(actual typ.Type) (ok bool, hasBroadTable bool, hasConcreteRecord bool) {
	switch t := unwrap.Annotated(actual).(type) {
	case *typ.Record:
		return true, false, true
	case *typ.Optional:
		return false, false, false
	case *typ.Union:
		if len(t.Members) == 0 {
			return false, false, false
		}
		hasBroadTable := false
		hasConcreteRecord := false
		for _, member := range t.Members {
			ok, broad, record := inferredRecordReplacementSurface(member)
			if !ok {
				return false, false, false
			}
			hasBroadTable = hasBroadTable || broad
			hasConcreteRecord = hasConcreteRecord || record
		}
		return true, hasBroadTable, hasConcreteRecord
	default:
		if typetable.IsBuiltinTopMarker(t) {
			return true, true, false
		}
		return false, false, false
	}
}
