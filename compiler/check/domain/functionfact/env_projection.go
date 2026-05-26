package functionfact

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// EnvReturnEvaluator evaluates one environment-return spec in the caller's
// abstract state and returns the callee result vector.
type EnvReturnEvaluator func(contract.EnvReturnSpec) []typ.Type

// ProjectEnvironmentReturns refines call returns using branch-guarded
// environment-return specs carried by the exported function contract.
func ProjectEnvironmentReturns(callee typ.Type, current []typ.Type, callArgs []typ.Type, eval EnvReturnEvaluator) []typ.Type {
	if eval == nil || len(current) == 0 {
		return current
	}
	spec := contract.ExtractSpec(callee)
	if spec == nil {
		return current
	}
	envReturns := spec.GetEnvReturns()
	if len(envReturns) == 0 {
		return current
	}
	out := CopyReturnVector(current)
	changed := false
	for _, envReturn := range envReturns {
		if !envReturnGuardMayMatch(envReturn.When, callArgs) {
			continue
		}
		results := eval(envReturn)
		if envReturn.ResultIndex < 0 || envReturn.ResultIndex >= len(results) {
			continue
		}
		candidate := results[envReturn.ResultIndex]
		if typ.IsAbsentOrUnknown(candidate) || typ.IsAny(candidate) {
			continue
		}
		if envReturn.ReturnIndex < 0 {
			continue
		}
		for len(out) <= envReturn.ReturnIndex {
			out = append(out, typ.Nil)
		}
		merged := projectEnvironmentReturnSlot(out[envReturn.ReturnIndex], candidate)
		if typ.TypeEquals(merged, out[envReturn.ReturnIndex]) {
			continue
		}
		out[envReturn.ReturnIndex] = merged
		changed = true
	}
	if !changed {
		return current
	}
	return out
}

type guardTruth uint8

const (
	guardFalse guardTruth = iota
	guardUnknown
	guardTrue
)

func envReturnGuardMayMatch(cond constraint.Condition, args []typ.Type) bool {
	return proveEnvReturnCondition(cond, args) != guardFalse
}

func proveEnvReturnCondition(cond constraint.Condition, args []typ.Type) guardTruth {
	if len(cond.Disjuncts) == 0 {
		return guardTrue
	}
	if cond.IsTrue() {
		return guardTrue
	}
	unknown := false
	for _, disjunct := range cond.Disjuncts {
		switch proveEnvReturnConjunction(disjunct, args) {
		case guardTrue:
			return guardTrue
		case guardUnknown:
			unknown = true
		}
	}
	if unknown {
		return guardUnknown
	}
	return guardFalse
}

func proveEnvReturnConjunction(conj []constraint.Constraint, args []typ.Type) guardTruth {
	unknown := false
	for _, c := range conj {
		switch proveEnvReturnConstraint(c, args) {
		case guardFalse:
			return guardFalse
		case guardUnknown:
			unknown = true
		}
	}
	if unknown {
		return guardUnknown
	}
	return guardTrue
}

func proveEnvReturnConstraint(c constraint.Constraint, args []typ.Type) guardTruth {
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[guardTruth]{
		Truthy: func(v constraint.Truthy) guardTruth {
			return proveTruthy(envGuardPathType(v.Path, args))
		},
		Falsy: func(v constraint.Falsy) guardTruth {
			return proveFalsy(envGuardPathType(v.Path, args))
		},
		IsNil: func(v constraint.IsNil) guardTruth {
			return proveNil(envGuardPathType(v.Path, args))
		},
		NotNil: func(v constraint.NotNil) guardTruth {
			return proveNotNil(envGuardPathType(v.Path, args))
		},
		FieldEquals: func(v constraint.FieldEquals) guardTruth {
			t := envGuardPathType(v.Target.Field(v.Field), args)
			return proveLiteralEqual(t, v.Value)
		},
		FieldNotEquals: func(v constraint.FieldNotEquals) guardTruth {
			t := envGuardPathType(v.Target.Field(v.Field), args)
			return invertKnown(proveLiteralEqual(t, v.Value))
		},
		HasField: func(v constraint.HasField) guardTruth {
			t := envGuardPathType(v.Path.Field(v.Field), args)
			if t == nil {
				return guardUnknown
			}
			if unwrap.IsNilType(t) {
				return guardFalse
			}
			return guardTrue
		},
		IndexEquals: func(v constraint.IndexEquals) guardTruth {
			t := envGuardPathType(v.Target.IndexStr(indexConstraintString(v.Key)), args)
			return proveLiteralEqual(t, v.Value)
		},
		IndexNotEquals: func(v constraint.IndexNotEquals) guardTruth {
			t := envGuardPathType(v.Target.IndexStr(indexConstraintString(v.Key)), args)
			return invertKnown(proveLiteralEqual(t, v.Value))
		},
		Default: func(constraint.Constraint) guardTruth {
			return guardUnknown
		},
	})
}

func envGuardPathType(path constraint.Path, args []typ.Type) typ.Type {
	idx, ok := constraint.PlaceholderArgIndex(path, len(args))
	if !ok || idx < 0 || idx >= len(args) {
		return nil
	}
	t := args[idx]
	for _, seg := range path.Segments {
		if t == nil {
			return nil
		}
		switch seg.Kind {
		case constraint.SegmentField:
			next, ok := core.Field(t, seg.Name)
			if !ok {
				if core.MissingFieldReadsNil(t) {
					next = typ.Nil
					ok = true
				}
			}
			if !ok {
				return nil
			}
			t = next
		case constraint.SegmentIndexString:
			next, ok := core.Index(t, typ.LiteralString(seg.Name))
			if !ok {
				if core.MissingFieldReadsNil(t) {
					next = typ.Nil
					ok = true
				}
			}
			if !ok {
				return nil
			}
			t = next
		case constraint.SegmentIndexInt:
			next, ok := core.Index(t, typ.LiteralInt(int64(seg.Index)))
			if !ok {
				if core.MissingFieldReadsNil(t) {
					next = typ.Nil
					ok = true
				}
			}
			if !ok {
				return nil
			}
			t = next
		}
	}
	return t
}

func proveTruthy(t typ.Type) guardTruth {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return guardUnknown
	}
	if narrow.ToTruthy(t).Kind().IsNever() {
		return guardFalse
	}
	if narrow.ToFalsy(t).Kind().IsNever() {
		return guardTrue
	}
	return guardUnknown
}

func proveFalsy(t typ.Type) guardTruth {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return guardUnknown
	}
	if narrow.ToFalsy(t).Kind().IsNever() {
		return guardFalse
	}
	if narrow.ToTruthy(t).Kind().IsNever() {
		return guardTrue
	}
	return guardUnknown
}

func proveNil(t typ.Type) guardTruth {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return guardUnknown
	}
	if unwrap.IsNilType(t) {
		return guardTrue
	}
	if narrow.RemoveNil(t).Kind().IsNever() {
		return guardTrue
	}
	if typ.TypeEquals(narrow.RemoveNil(t), t) {
		return guardFalse
	}
	return guardUnknown
}

func proveNotNil(t typ.Type) guardTruth {
	return invertKnown(proveNil(t))
}

func proveLiteralEqual(t typ.Type, lit *typ.Literal) guardTruth {
	if t == nil || lit == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return guardUnknown
	}
	if typ.TypeEquals(t, lit) {
		return guardTrue
	}
	if subtype.IsSubtype(t, lit) {
		return guardTrue
	}
	if subtype.IsSubtype(t, literalBaseType(lit)) && !subtype.IsSubtype(lit, t) {
		return guardUnknown
	}
	if !subtype.IsSubtype(lit, t) {
		return guardFalse
	}
	return guardUnknown
}

func invertKnown(v guardTruth) guardTruth {
	switch v {
	case guardTrue:
		return guardFalse
	case guardFalse:
		return guardTrue
	default:
		return guardUnknown
	}
}

func literalBaseType(lit *typ.Literal) typ.Type {
	if lit == nil {
		return typ.Unknown
	}
	switch lit.Base {
	case kind.Boolean:
		return typ.Boolean
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	case kind.String:
		return typ.String
	default:
		return typ.Unknown
	}
}

func indexConstraintString(t typ.Type) string {
	if lit, ok := unwrap.Alias(t).(*typ.Literal); ok {
		if s, ok := lit.Value.(string); ok {
			return s
		}
	}
	return ""
}

func projectEnvironmentReturnSlot(current, candidate typ.Type) typ.Type {
	if candidate == nil || typ.IsAbsentOrUnknown(candidate) || typ.IsAny(candidate) {
		return current
	}
	if current == nil || typ.IsAbsentOrUnknown(current) || typ.IsAny(current) {
		return candidate
	}
	if union := unwrap.Union(current); union != nil {
		members := make([]typ.Type, len(union.Members))
		copy(members, union.Members)
		changed := false
		for i, member := range members {
			if environmentCandidateRefinesMember(candidate, member) {
				members[i] = candidate
				changed = true
			}
		}
		if changed {
			return typ.NewUnion(members...)
		}
		return typ.JoinReturnSlot(current, candidate)
	}
	if environmentCandidateRefinesMember(candidate, current) {
		return candidate
	}
	return typ.JoinReturnSlot(current, candidate)
}

func environmentCandidateRefinesMember(candidate, member typ.Type) bool {
	if candidate == nil || member == nil || typ.TypeEquals(candidate, member) {
		return false
	}
	if subtype.IsSubtype(candidate, member) && !subtype.IsSubtype(member, candidate) {
		return true
	}
	if refines, changed := value.RefinesSoftContainer(candidate, member); refines && changed {
		return true
	}
	return false
}

// CopyReturnVector returns a copy of a return vector.
func CopyReturnVector(types []typ.Type) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	out := make([]typ.Type, len(types))
	copy(out, types)
	return out
}
