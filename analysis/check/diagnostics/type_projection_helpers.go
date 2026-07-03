package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expressionSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
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

func projectionHasNil(t typ.Type) bool {
	return typevalue.ProjectionHasNil(t)
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}

func transparentExpectedType(t typ.Type) typ.Type {
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
