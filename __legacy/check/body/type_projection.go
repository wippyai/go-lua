package body

import (
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/inspect"
	"github.com/wippyai/go-lua/analysis/domain/type/literal"
	"github.com/wippyai/go-lua/analysis/domain/type/subst"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/transform"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func TypeHasField(t typ.Type, name string) bool {
	_, ok := checkprojection.Field(t, name)
	return ok
}

func TypeField(t typ.Type, name string) (typ.Type, bool) {
	return checkprojection.Field(t, name)
}

func TypeMissingFieldReadsNil(t typ.Type) bool {
	return checkprojection.MissingFieldReadsNil(t)
}

func TypeFieldProvablyAbsent(t typ.Type, name string) bool {
	if t == nil || TypeIsGradual(t) || typ.IsNever(t) || typevalue.TypeIncludesNil(t) {
		return false
	}
	if TypeHasField(t, name) {
		return false
	}
	return ClosedRecordLacksField(t, name)
}

func ClosedRecordLacksField(t typ.Type, name string) bool {
	if !inspect.LeastBoolFixedPoint(t, typeProjectionProductivityEquation) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch v := current.(type) {
		case *typ.Annotated:
			return typeBoolAll(v.Inner)
		case *typ.Record:
			return typeBoolConstant(closedRecordWithoutField(v, name))
		case *typ.Union:
			if len(v.Members) == 0 {
				return typeBoolConstant(false)
			}
			return typeBoolAll(v.Members...)
		case *typ.Alias:
			return typeBoolAll(v.UnaliasedTarget())
		case *typ.Recursive:
			return typeBoolAll(v.Body)
		default:
			return typeBoolConstant(false)
		}
	})
}

func closedRecordWithoutField(r *typ.Record, name string) bool {
	if r == nil || r.Open || r.HasMapComponent() || r.Metatable != nil {
		return false
	}
	if r.GetField(name) != nil {
		return false
	}
	if r.GetStaticStringIndex(name) != nil {
		return false
	}
	return true
}

func TypeIsGradual(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t)
}

func TypeFamilyBase(t typ.Type) (typ.Type, bool) {
	if base, ok := literal.FamilyBase(t); ok {
		return base, true
	}
	return intersectionFamilyBase(t)
}

func intersectionFamilyBase(t typ.Type) (typ.Type, bool) {
	seen := inspect.NewIdentitySeen(nil)
	for t != nil && !seen.Contains(t) {
		seen.Remember(t)
		switch v := t.(type) {
		case *typ.Annotated:
			t = v.Inner
		case *typ.Alias:
			t = v.UnaliasedTarget()
		case *typ.Optional:
			t = v.Inner
		case *typ.Intersection:
			for _, candidate := range v.Members {
				if candidate == nil {
					continue
				}
				coversAll := true
				for _, member := range v.Members {
					if member == nil || typ.TypeEquals(member, candidate) {
						continue
					}
					if !subtype.IsSubtype(member, candidate) {
						coversAll = false
						break
					}
				}
				if coversAll {
					return candidate, true
				}
			}
			return nil, false
		default:
			return nil, false
		}
	}
	return nil, false
}

func TypeIsMultiArmUnion(t typ.Type) bool {
	return inspect.IsMultiArmUnion(t)
}

func TypeWithoutOptionalNil(t typ.Type) typ.Type {
	return unwrap.Optional(t)
}

func TypeIsUnionReceiver(t typ.Type) bool {
	seen := inspect.NewIdentitySeen(nil)
	for t != nil && !seen.Contains(t) {
		seen.Remember(t)
		switch v := t.(type) {
		case *typ.Annotated:
			t = v.Inner
		case *typ.Alias:
			t = v.UnaliasedTarget()
		case *typ.Union:
			return true
		default:
			return false
		}
	}
	return false
}

func TypeIsClosedConcreteRecord(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil || rec.Open || rec.HasMapComponent() {
		return false
	}
	return len(rec.Fields) != 0 || len(rec.StaticMembers) != 0
}

func UnionArmRejectsFieldRead(t typ.Type, name string) bool {
	if t == nil || TypeIsGradual(t) || typ.IsNever(t) || typevalue.TypeIncludesNil(t) {
		return false
	}
	union, ok := unwrap.Annotated(t).(*typ.Union)
	if !ok || len(union.Members) < 2 {
		return false
	}
	carriesField := false
	rejectingArm := false
	for _, member := range union.Members {
		if _, ok := TypeField(member, name); ok {
			carriesField = true
			continue
		}
		if TypeMissingFieldReadsNil(member) {
			continue
		}
		rejectingArm = true
	}
	return carriesField && rejectingArm
}

type StaticTypeChild struct {
	Segment segment.Segment
	Type    typ.Type
}

func StaticTypeChildren(t typ.Type) []StaticTypeChild {
	seen := inspect.NewIdentitySeen(nil)
	for t != nil && !seen.Contains(t) {
		seen.Remember(t)
		switch v := t.(type) {
		case *typ.Annotated:
			t = v.Inner
		case *typ.Alias:
			t = v.UnaliasedTarget()
		case *typ.Optional:
			t = v.Inner
		case *typ.Recursive:
			t = v.Body
		case *typ.Instantiated:
			expanded, ok := subst.ExpandInstantiatedChanged(v)
			if !ok {
				return nil
			}
			t = expanded
		case *typ.Record:
			out := make([]StaticTypeChild, 0, len(v.Fields)+len(v.StaticMembers))
			for _, field := range v.Fields {
				out = append(out, StaticTypeChild{Segment: segment.Segment{Kind: segment.SegmentField, Name: field.Name}, Type: field.Type})
			}
			for _, member := range v.StaticMembers {
				if member.Kind == typ.StaticMemberStringIndex {
					out = append(out, StaticTypeChild{Segment: segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name}, Type: member.Type})
				}
			}
			return out
		default:
			return nil
		}
	}
	return nil
}

func (r *Result) TransparentComparableType(t typ.Type) typ.Type {
	t = TransparentExpectedType(t)
	if r == nil {
		return t
	}
	moduleTypes := r.ModuleTypes()
	if len(moduleTypes.Manifests) == 0 {
		return t
	}
	resolved := transform.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		ref, ok := node.(*typ.Ref)
		if !ok || ref.Module == "" {
			return nil, false
		}
		if resolved, ok := moduleTypes.Lookup(ref.Module, ref.Name); ok {
			return resolved, true
		}
		if modulePath, ok := r.RequireAliasModulePath(ref.Module); ok {
			if resolved, ok := moduleTypes.Lookup(modulePath, ref.Name); ok {
				return resolved, true
			}
		}
		return nil, false
	})
	return TransparentExpectedType(resolved)
}

func TransparentExpectedType(t typ.Type) typ.Type {
	seen := inspect.NewIdentitySeen(nil)
	for t != nil && !seen.Contains(t) {
		seen.Remember(t)
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return typ.Unknown
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return t
			}
			t = tt.Body
		case *typ.Instantiated:
			next := shallowExpandExpectedInstantiated(tt)
			if next == nil || next == t {
				return t
			}
			t = next
		default:
			return t
		}
	}
	return t
}

func typeProjectionProductivityEquation(current typ.Type) inspect.BoolEquation {
	switch v := current.(type) {
	case *typ.Annotated:
		return typeBoolAny(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return typeBoolConstant(false)
		}
		return typeBoolAny(v.Members...)
	case *typ.Alias:
		return typeBoolAny(v.UnaliasedTarget())
	case *typ.Recursive:
		return typeBoolAny(v.Body)
	default:
		return typeBoolConstant(current != nil)
	}
}

func typeBoolConstant(value bool) inspect.BoolEquation {
	return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: value}
}

func typeBoolAll(inputs ...typ.Type) inspect.BoolEquation {
	return inspect.BoolEquation{Join: inspect.BoolAll, Inputs: inputs}
}

func typeBoolAny(inputs ...typ.Type) inspect.BoolEquation {
	return inspect.BoolEquation{Join: inspect.BoolAny, Inputs: inputs}
}

func shallowExpandExpectedInstantiated(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil || len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
		return inst
	}
	body := subst.Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
	if body == nil {
		return inst
	}
	return subst.Self(body, inst)
}
