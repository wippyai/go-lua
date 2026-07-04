package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func TypeAtSegment(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if field, ok := access.Field(t, seg.Name); ok {
			return field, true
		}
		if access.MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		key, ok := luatypeprojection.SegmentKeyType(seg)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(t, key)
	default:
		return nil, false
	}
}

func ExpectedTypeAtSegments(t typ.Type, segments []segment.Segment) (typ.Type, bool) {
	return luatypeprojection.ExpectedTypeAtSegments(t, segments)
}

func ExpectedConstructorEntryType(t typ.Type, segments []segment.Segment) (typ.Type, bool) {
	return luatypeprojection.ExpectedConstructorEntryType(t, segments)
}

func MissingRequiredRecordField(t typ.Type, hasField func(string) bool) (string, bool) {
	return luatypeprojection.MissingRequiredRecordField(t, hasField)
}

func ConstructorPathFromSegments(segments []segment.Segment) ([]typetable.ConstructorKey, bool) {
	return luatypeprojection.ConstructorPathFromSegments(segments)
}

func ObjectLiteralShapeType(literal ObjectLiteralFact, entryType func(ObjectEntryFact) (typ.Type, bool)) (typ.Type, bool) {
	if entryType == nil {
		return nil, false
	}
	builder := typetable.NewConstructorBuilder()
	seen := false
	for _, entry := range literal.Entries {
		path, ok := ConstructorPathFromSegments(entry.Suffix.Segments)
		if !ok {
			continue
		}
		t, ok := entryType(entry)
		if !ok || t == nil {
			continue
		}
		if !builder.Add(path, t) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func TypeHasField(t typ.Type, name string) bool {
	_, ok := access.Field(t, name)
	return ok
}

func TypeField(t typ.Type, name string) (typ.Type, bool) {
	return access.Field(t, name)
}

func TypeMissingFieldReadsNil(t typ.Type) bool {
	return access.MissingFieldReadsNil(t)
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
	return closedRecordLacksField(t, name, 0)
}

func closedRecordLacksField(t typ.Type, name string, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return closedRecordWithoutField(v, name)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !closedRecordLacksField(member, name, depth+1) {
				return false
			}
		}
		return true
	case *typ.Alias:
		return closedRecordLacksField(v.UnaliasedTarget(), name, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return closedRecordLacksField(v.Body, name, depth+1)
	default:
		return false
	}
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
	return intersectionFamilyBase(t, 0)
}

func intersectionFamilyBase(t typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		next := v.UnaliasedTarget()
		if next == nil || next == t {
			return nil, false
		}
		return intersectionFamilyBase(next, depth+1)
	case *typ.Optional:
		return intersectionFamilyBase(v.Inner, depth+1)
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
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return true
	case *typ.Alias:
		return TypeIsUnionReceiver(v.UnaliasedTarget())
	default:
		return false
	}
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
	return staticTypeChildren(t, 0)
}

func staticTypeChildren(t typ.Type, depth int) []StaticTypeChild {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return staticTypeChildren(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return staticTypeChildren(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil
		}
		return staticTypeChildren(v.Body, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil
		}
		return staticTypeChildren(expanded, depth+1)
	case *typ.Record:
		out := make([]StaticTypeChild, 0, len(v.Fields)+len(v.StaticMembers))
		for _, field := range v.Fields {
			out = append(out, StaticTypeChild{
				Segment: segment.Segment{Kind: segment.SegmentField, Name: field.Name},
				Type:    field.Type,
			})
		}
		for _, member := range v.StaticMembers {
			if member.Kind != typ.StaticMemberStringIndex {
				continue
			}
			out = append(out, StaticTypeChild{
				Segment: segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name},
				Type:    member.Type,
			})
		}
		return out
	default:
		return nil
	}
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
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
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
