package typeannotation

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/type/ambient"
	luatable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Resolver resolves named type references from the current annotation scope.
type Resolver interface {
	ResolveTypeRef(path []string) (typ.Type, bool)
}

// Type lowers an AST type expression into the active analysis type model.
func Type(expr ast.TypeExpr, resolver Resolver) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		return primitive(e, resolver)
	case *ast.SelfTypeExpr:
		return typ.Self, true
	case *ast.OptionalTypeExpr:
		inner, ok := Type(e.Inner, resolver)
		if !ok {
			return nil, false
		}
		return typeexpr.Optional(inner), true
	case *ast.UnionTypeExpr:
		members, ok := typeList(e.Types, resolver)
		if !ok {
			return nil, false
		}
		return typeexpr.Union(members...), true
	case *ast.IntersectionTypeExpr:
		members, ok := typeList(e.Types, resolver)
		if !ok {
			return nil, false
		}
		return typeexpr.Intersection(members...), true
	case *ast.ArrayTypeExpr:
		return array(e, resolver)
	case *ast.MapTypeExpr:
		return mapType(e, resolver)
	case *ast.RecordTypeExpr:
		return record(e, resolver)
	case *ast.FunctionTypeExpr:
		return function(e, resolver)
	case *ast.TypeRefExpr:
		return ref(e.Path, resolver), true
	case *ast.GenericTypeExpr:
		return generic(e, resolver)
	case *ast.LiteralTypeExpr:
		return literal(e.Value)
	case *ast.MetaTypeExpr:
		inner, ok := Type(e.Inner, resolver)
		if !ok {
			return nil, false
		}
		return typ.NewMeta(inner), true
	case *ast.TupleTypeExpr:
		elems, ok := typeList(e.Elements, resolver)
		if !ok {
			return nil, false
		}
		return typ.NewTuple(elems...), true
	default:
		return nil, false
	}
}

func primitive(expr *ast.PrimitiveTypeExpr, resolver Resolver) (typ.Type, bool) {
	var t typ.Type
	switch expr.Name {
	case "nil":
		t = typ.Nil
	case "boolean":
		t = typ.Boolean
	case "number":
		t = typ.Number
	case "integer":
		t = typ.Integer
	case "string":
		t = typ.String
	case "any":
		t = typ.Any
	case "unknown":
		t = typ.Unknown
	case "never":
		t = typ.Never
	case "self":
		t = typ.Self
	default:
		t = ref([]string{expr.Name}, resolver)
	}
	anns, ok := Annotations(expr.Annotations)
	if !ok {
		return nil, false
	}
	return typ.NewAnnotated(t, anns), true
}

func array(expr *ast.ArrayTypeExpr, resolver Resolver) (typ.Type, bool) {
	if expr.Readonly {
		return nil, false
	}
	elem, ok := Type(expr.Element, resolver)
	if !ok {
		return nil, false
	}
	elemAnns, ok := Annotations(expr.ElementAnnotations)
	if !ok {
		return nil, false
	}
	arrAnns, ok := Annotations(expr.ArrayAnnotations)
	if !ok {
		return nil, false
	}
	elem = typ.NewAnnotated(elem, elemAnns)
	return typ.NewAnnotated(typ.NewArray(elem), arrAnns), true
}

func mapType(expr *ast.MapTypeExpr, resolver Resolver) (typ.Type, bool) {
	key, ok := Type(expr.Key, resolver)
	if !ok {
		return nil, false
	}
	value, ok := Type(expr.Value, resolver)
	if !ok {
		return nil, false
	}
	if expr.Readonly {
		return luatable.NewReadonlyMap(key, value), true
	}
	return luatable.NewMap(key, value), true
}

func record(expr *ast.RecordTypeExpr, resolver Resolver) (typ.Type, bool) {
	fields := make([]typ.Field, 0, len(expr.Fields))
	for _, field := range expr.Fields {
		t, ok := Type(field.Type, resolver)
		if !ok {
			return nil, false
		}
		anns, ok := Annotations(field.Annotations)
		if !ok {
			return nil, false
		}
		fields = append(fields, typ.Field{
			Name:     field.Name,
			Type:     typ.NewAnnotated(t, anns),
			Optional: field.Optional,
			Readonly: expr.Readonly,
		})
	}
	return luatable.RebuildRecord(typ.RecordParts{Fields: fields}), true
}

func function(expr *ast.FunctionTypeExpr, resolver Resolver) (typ.Type, bool) {
	builder := typ.Func()
	typeParams := make(map[string]*typ.TypeParam, len(expr.TypeParams))
	for _, param := range expr.TypeParams {
		var constraint typ.Type
		if param.Constraint != nil {
			t, ok := Type(param.Constraint, resolver)
			if !ok {
				return nil, false
			}
			constraint = t
		}
		tp := typ.NewTypeParam(param.Name, constraint)
		typeParams[param.Name] = tp
		builder.TypeParamRef(tp)
	}
	localResolver := resolver
	if len(typeParams) > 0 {
		localResolver = scopedResolver{typeParams: typeParams, parent: resolver}
	}
	for _, param := range expr.Params {
		t, ok := Type(param.Type, localResolver)
		if !ok {
			return nil, false
		}
		builder.Param(param.Name, t)
	}
	if expr.Variadic != nil {
		t, ok := Type(expr.Variadic, localResolver)
		if !ok {
			return nil, false
		}
		builder.Variadic(t)
	}
	returns, ok := typeList(expr.Returns, localResolver)
	if !ok {
		return nil, false
	}
	builder.Returns(returns...)
	return builder.Build(), true
}

type scopedResolver struct {
	typeParams map[string]*typ.TypeParam
	parent     Resolver
}

func (r scopedResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) == 1 {
		if t, ok := r.typeParams[path[0]]; ok {
			return t, true
		}
	}
	if r.parent == nil {
		return nil, false
	}
	return r.parent.ResolveTypeRef(path)
}

func generic(expr *ast.GenericTypeExpr, resolver Resolver) (typ.Type, bool) {
	if expr.Base == nil {
		return nil, false
	}
	args, ok := typeList(expr.Args, resolver)
	if !ok {
		return nil, false
	}
	base := ref(expr.Base.Path, resolver)
	if g, ok := base.(*typ.Generic); ok {
		return typ.Instantiate(g, args...), true
	}
	return nil, false
}

func typeList(exprs []ast.TypeExpr, resolver Resolver) ([]typ.Type, bool) {
	types := make([]typ.Type, 0, len(exprs))
	for _, expr := range exprs {
		t, ok := Type(expr, resolver)
		if !ok {
			return nil, false
		}
		types = append(types, t)
	}
	return types, true
}

func ref(path []string, resolver Resolver) typ.Type {
	if resolver != nil {
		if t, ok := resolver.ResolveTypeRef(path); ok {
			return t
		}
	}
	if len(path) == 1 {
		if t, ok := ambient.Lookup(path[0]); ok {
			return t
		}
	}
	if len(path) == 0 {
		return typ.NewRef("", "")
	}
	if len(path) == 1 {
		return typ.NewRef("", path[0])
	}
	return typ.NewRef(strings.Join(path[:len(path)-1], "."), path[len(path)-1])
}

func literal(value any) (typ.Type, bool) {
	switch v := value.(type) {
	case string:
		return typ.LiteralString(v), true
	case bool:
		return typ.LiteralBool(v), true
	case int:
		return typ.LiteralInt(int64(v)), true
	case int64:
		return typ.LiteralInt(v), true
	case float64:
		return typ.LiteralNumber(v), true
	default:
		return nil, false
	}
}
