package body

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// UnresolvedTypeReferenceOccurrence is an annotation type name that binding
// could not resolve in the current lexical/module type namespace.
type UnresolvedTypeReferenceOccurrence struct {
	Point cfg.Point
	Name  string
	Key   string
	Span  SourceSpan
}

// ForEachUnresolvedTypeReferenceOccurrence visits unresolved annotation type
// references. Parent results are included only to build the lexical type scope.
func (r *Result) ForEachUnresolvedTypeReferenceOccurrence(parents []*Result, visit func(UnresolvedTypeReferenceOccurrence) bool) bool {
	if r == nil || visit == nil {
		return false
	}
	scope := unresolvedTypeScopeFromResults(r, parents)
	visited := false
	seenRefs := make(map[*ast.TypeRefExpr]struct{})
	seenPrimitives := make(map[*ast.PrimitiveTypeExpr]struct{})
	emit := func(point cfg.Point, expr ast.TypeExpr) bool {
		return r.walkUnresolvedTypeExpr(point, expr, scope, seenRefs, seenPrimitives, func(ref UnresolvedTypeReferenceOccurrence) bool {
			visited = true
			return visit(ref)
		})
	}
	emitMany := func(point cfg.Point, exprs []ast.TypeExpr) bool {
		for _, expr := range exprs {
			if !emit(point, expr) {
				return false
			}
		}
		return true
	}
	if fn := r.Function(); fn != nil {
		for _, param := range fn.TypeParams {
			if !emit(0, param.Constraint) {
				return true
			}
		}
		if fn.ParList != nil {
			if !emitMany(0, fn.ParList.Types) {
				return true
			}
			if !emit(0, fn.ParList.VarargType) {
				return true
			}
		}
		if !emitMany(0, fn.ReturnTypes) {
			return true
		}
	}
	graph := r.Graph()
	if graph == nil {
		return visited
	}
	for _, point := range graph.RPO() {
		if fact, ok := r.LocalAssignment(point); ok {
			if !emit(point, fact.Type) {
				return true
			}
		}
		if fact, ok := r.TypeDefinition(point); ok {
			if !emitUnresolvedTypeDefinitionRefs(point, fact, emit) {
				return true
			}
		}
		if fact, ok := r.SourceCall(point); ok && fact.Call != nil {
			if !emitMany(point, fact.TypeArgs) {
				return true
			}
		}
	}
	return visited
}

type unresolvedTypeScope struct {
	known map[string]struct{}
}

func unresolvedTypeScopeFromResults(result *Result, parents []*Result) unresolvedTypeScope {
	known := make(map[string]struct{})
	collect := func(result *Result) {
		if result == nil || result.Graph() == nil {
			return
		}
		for _, point := range result.Graph().RPO() {
			fact, ok := result.TypeDefinition(point)
			if !ok {
				continue
			}
			switch fact.Kind {
			case cfgbuild.TypeDefinitionAlias:
				if fact.Type != nil && fact.Type.Name != "" {
					known[fact.Type.Name] = struct{}{}
				}
			case cfgbuild.TypeDefinitionInterface:
				if fact.Interface != nil && fact.Interface.Name != "" {
					known[fact.Interface.Name] = struct{}{}
				}
			}
		}
	}
	collect(result)
	for _, parent := range parents {
		collect(parent)
	}
	return unresolvedTypeScope{known: known}
}

func emitUnresolvedTypeDefinitionRefs(point cfg.Point, fact cfgbuild.TypeDefinition, emit func(cfg.Point, ast.TypeExpr) bool) bool {
	switch fact.Kind {
	case cfgbuild.TypeDefinitionAlias:
		if fact.Type == nil {
			return true
		}
		for _, param := range fact.Type.TypeParams {
			if !emit(point, param.Constraint) {
				return false
			}
		}
		return emit(point, fact.Type.Type)
	case cfgbuild.TypeDefinitionInterface:
		if fact.Interface == nil {
			return true
		}
		for _, ref := range fact.Interface.Extends {
			if !emit(point, ref) {
				return false
			}
		}
		for _, field := range fact.Interface.Fields {
			if !emit(point, field.Type) {
				return false
			}
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil && !emit(point, method.Type) {
				return false
			}
		}
	}
	return true
}

func (r *Result) walkUnresolvedTypeExpr(
	point cfg.Point,
	expr ast.TypeExpr,
	scope unresolvedTypeScope,
	seenRefs map[*ast.TypeRefExpr]struct{},
	seenPrimitives map[*ast.PrimitiveTypeExpr]struct{},
	visit func(UnresolvedTypeReferenceOccurrence) bool,
) bool {
	if expr == nil {
		return true
	}
	keepGoing := true
	typeresolve.WalkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || !keepGoing {
			return keepGoing
		}
		if _, ok := seenRefs[ref]; ok {
			return true
		}
		seenRefs[ref] = struct{}{}
		if r.typeRefResolved(ref, scope) {
			return true
		}
		keepGoing = visit(unresolvedTypeReference(point, ref, typeRefName(ref)))
		return keepGoing
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || !keepGoing || typ.BuiltinPrimitiveName(prim.Name) {
			return keepGoing
		}
		if _, ok := seenPrimitives[prim]; ok {
			return true
		}
		seenPrimitives[prim] = struct{}{}
		if r.primitiveTypeResolved(prim, scope) {
			return true
		}
		keepGoing = visit(unresolvedTypeReference(point, prim, prim.Name))
		return keepGoing
	})
	return keepGoing
}

func (r *Result) typeRefResolved(ref *ast.TypeRefExpr, scope unresolvedTypeScope) bool {
	if ref == nil || len(ref.Path) == 0 {
		return false
	}
	if len(ref.Path) != 1 {
		return r.qualifiedTypeRefResolved(ref.Path)
	}
	if _, ok := r.TypeRef(ref); ok {
		return true
	}
	if _, ok := scope.known[ref.Path[0]]; ok {
		return false
	}
	return true
}

func (r *Result) primitiveTypeResolved(expr *ast.PrimitiveTypeExpr, scope unresolvedTypeScope) bool {
	if expr == nil || typ.BuiltinPrimitiveName(expr.Name) {
		return true
	}
	if _, ok := r.PrimitiveTypeRef(expr); ok {
		return true
	}
	if _, ok := scope.known[expr.Name]; ok {
		return false
	}
	return true
}

func (r *Result) qualifiedTypeRefResolved(path []string) bool {
	if len(path) < 2 {
		return false
	}
	moduleRefs := r.ModuleTypes()
	if modulePath, ok := r.RequireAliasModulePath(path[0]); ok {
		if _, resolved := moduleRefs.ResolveTypeRefWithModulePrefix(modulePath, path[1:]); resolved {
			return true
		}
	}
	_, resolved := moduleRefs.ResolveTypeRef(path)
	return resolved
}

func unresolvedTypeReference(point cfg.Point, node ast.PositionHolder, name string) UnresolvedTypeReferenceOccurrence {
	if name == "" {
		name = "<missing>"
	}
	span := sourceSpanFromAST(ast.SpanOf(node))
	return UnresolvedTypeReferenceOccurrence{
		Point: point,
		Name:  name,
		Key:   "type:" + name + ":" + strconv.Itoa(span.StartLine) + ":" + strconv.Itoa(span.StartCol),
		Span:  span,
	}
}

func typeRefName(ref *ast.TypeRefExpr) string {
	if ref == nil || len(ref.Path) == 0 {
		return "<missing>"
	}
	return strings.Join(ref.Path, ".")
}
