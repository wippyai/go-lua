package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func clearMismatch(result *body.Result, got, want typ.Type) bool {
	if got == nil || want == nil || typ.IsAny(got) || typ.IsUnknown(got) || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if typePairClearlyCompatible(got, want) {
		return false
	}
	if sameNominalGenericInstantiation(got, want) {
		return true
	}
	got = transparentComparableType(result, got)
	want = transparentComparableType(result, want)
	if typePairClearlyCompatible(got, want) {
		return false
	}
	if explicitNilFieldFreshAssignable(got, want) {
		return false
	}
	if projectionHasNil(want) {
		nonNilWant := projectionWithoutNil(want)
		if nonNilWant != nil && !typ.IsNever(nonNilWant) &&
			subtype.IsSubtype(got, transparentComparableType(result, nonNilWant)) {
			return false
		}
	}
	return true
}

func typePairClearlyCompatible(got, want typ.Type) bool {
	return typ.TypeEquals(got, want) ||
		recursiveUnfoldingEquivalent(got, want) ||
		subtype.IsSubtype(got, want)
}

func recursiveUnfoldingEquivalent(left, right typ.Type) bool {
	return recursiveUnfoldingEquivalentDepth(left, right, 0)
}

func recursiveUnfoldingEquivalentDepth(left, right typ.Type, depth int) bool {
	if left == nil || right == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	if typ.TypeEquals(left, right) {
		return true
	}
	left = transparentNonRecursiveComparableType(left)
	right = transparentNonRecursiveComparableType(right)
	if typ.TypeEquals(left, right) {
		return true
	}
	if rec, ok := left.(*typ.Recursive); ok && rec.Body != nil && rec.Body != rec {
		if recursiveUnfoldingEquivalentDepth(rec.Body, right, depth+1) {
			return true
		}
	}
	if rec, ok := right.(*typ.Recursive); ok && rec.Body != nil && rec.Body != rec {
		if recursiveUnfoldingEquivalentDepth(left, rec.Body, depth+1) {
			return true
		}
	}
	switch l := left.(type) {
	case *typ.Optional:
		r, ok := right.(*typ.Optional)
		return ok && recursiveUnfoldingEquivalentDepth(l.Inner, r.Inner, depth+1)
	case *typ.Record:
		r, ok := right.(*typ.Record)
		return ok && recursiveRecordsEquivalent(l, r, depth+1)
	default:
		return false
	}
}

func transparentNonRecursiveComparableType(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return t
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
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

func recursiveRecordsEquivalent(left, right *typ.Record, depth int) bool {
	if left == nil || right == nil ||
		left.Open != right.Open ||
		len(left.Fields) != len(right.Fields) ||
		len(left.StaticMembers) != len(right.StaticMembers) {
		return false
	}
	for _, field := range left.Fields {
		other := right.GetField(field.Name)
		if other == nil ||
			field.Optional != other.Optional ||
			field.Readonly != other.Readonly ||
			!recursiveUnfoldingEquivalentDepth(field.Type, other.Type, depth+1) {
			return false
		}
	}
	for _, member := range left.StaticMembers {
		other := right.GetStaticMember(member.Kind, member.Name, member.Index)
		if other == nil ||
			member.Optional != other.Optional ||
			member.Readonly != other.Readonly ||
			!recursiveUnfoldingEquivalentDepth(member.Type, other.Type, depth+1) {
			return false
		}
	}
	if (left.Metatable == nil) != (right.Metatable == nil) ||
		(left.MapKey == nil) != (right.MapKey == nil) ||
		(left.MapValue == nil) != (right.MapValue == nil) {
		return false
	}
	if left.Metatable != nil && !recursiveUnfoldingEquivalentDepth(left.Metatable, right.Metatable, depth+1) {
		return false
	}
	if left.MapKey != nil && !recursiveUnfoldingEquivalentDepth(left.MapKey, right.MapKey, depth+1) {
		return false
	}
	if left.MapValue != nil && !recursiveUnfoldingEquivalentDepth(left.MapValue, right.MapValue, depth+1) {
		return false
	}
	return true
}

func explicitNilFieldFreshAssignable(got, want typ.Type) bool {
	return subtype.IsFreshAssignable(got, want) && hasExplicitNilToNilableMember(got, want, 0)
}

func hasExplicitNilToNilableMember(got, want typ.Type, depth int) bool {
	if got == nil || want == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	got = transparentExpectedType(got)
	want = transparentExpectedType(want)
	switch wt := want.(type) {
	case *typ.Optional:
		return hasExplicitNilToNilableMember(got, wt.Inner, depth+1)
	case *typ.Union:
		for _, member := range wt.Members {
			if subtype.IsFreshAssignable(got, member) && hasExplicitNilToNilableMember(got, member, depth+1) {
				return true
			}
		}
		return false
	}
	gotRecord, ok := got.(*typ.Record)
	if !ok || gotRecord == nil {
		return false
	}
	wantRecord, ok := want.(*typ.Record)
	if !ok || wantRecord == nil {
		return false
	}
	for _, field := range gotRecord.Fields {
		if field.Type == nil || field.Type.Kind() != kind.Nil {
			continue
		}
		wantField := wantRecord.GetField(field.Name)
		if wantField != nil && (wantField.Optional || projectionHasNil(wantField.Type)) {
			return true
		}
	}
	for _, member := range gotRecord.StaticMembers {
		if member.Type == nil || member.Type.Kind() != kind.Nil {
			continue
		}
		wantMember := wantRecord.GetStaticMember(member.Kind, member.Name, member.Index)
		if wantMember != nil && (wantMember.Optional || projectionHasNil(wantMember.Type)) {
			return true
		}
	}
	return false
}

func sameNominalGenericInstantiation(left, right typ.Type) bool {
	leftInst, leftOK := transparentInstantiation(left)
	rightInst, rightOK := transparentInstantiation(right)
	return leftOK && rightOK &&
		leftInst.Generic != nil &&
		rightInst.Generic != nil &&
		typ.TypeEquals(leftInst.Generic, rightInst.Generic) &&
		len(leftInst.TypeArgs) == len(rightInst.TypeArgs)
}

func transparentInstantiation(t typ.Type) (*typ.Instantiated, bool) {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return nil, false
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil, false
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return nil, false
			}
			t = tt.Body
		case *typ.Instantiated:
			return tt, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func transparentComparableType(result *body.Result, t typ.Type) typ.Type {
	t = transparentExpectedType(t)
	if result == nil {
		return t
	}
	moduleTypes := result.ModuleTypes()
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
		if modulePath, ok := result.RequireAliasModulePath(ref.Module); ok {
			if resolved, ok := moduleTypes.Lookup(modulePath, ref.Name); ok {
				return resolved, true
			}
		}
		return nil, false
	})
	return transparentExpectedType(resolved)
}
