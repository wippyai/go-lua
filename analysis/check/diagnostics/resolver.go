package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type resultResolver struct {
	result *check.Result

	aliases    map[bind.TypeDeclID]*ast.TypeDefStmt
	interfaces map[bind.TypeDeclID]*ast.InterfaceDefStmt
	aliasNames map[string]bind.TypeDecl
	ifaceNames map[string]bind.TypeDecl
	knownNames map[string]struct{}
	cache      map[bind.TypeDeclID]typ.Type
	params     map[bind.TypeDeclID]*typ.TypeParam
	active     map[typeDeclKey]bool

	current []ast.TypeExpr

	explicit typeannotation.Resolver
	parent   typeannotation.Resolver
}

type typeDeclKey struct {
	kind bind.TypeDeclKind
	id   bind.TypeDeclID
}

func newResultResolver(result *check.Result, explicit, parent typeannotation.Resolver) *resultResolver {
	r := &resultResolver{
		result:     result,
		aliases:    make(map[bind.TypeDeclID]*ast.TypeDefStmt),
		interfaces: make(map[bind.TypeDeclID]*ast.InterfaceDefStmt),
		aliasNames: make(map[string]bind.TypeDecl),
		ifaceNames: make(map[string]bind.TypeDecl),
		knownNames: make(map[string]struct{}),
		cache:      make(map[bind.TypeDeclID]typ.Type),
		params:     make(map[bind.TypeDeclID]*typ.TypeParam),
		active:     make(map[typeDeclKey]bool),
		explicit:   explicit,
		parent:     parent,
	}
	if result == nil || result.Graph() == nil {
		return r
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.TypeDefinition(point)
		if !ok {
			continue
		}
		switch fact.Kind {
		case cfgfacts.TypeDefinitionAlias:
			if fact.Type == nil || fact.Type.Name == "" || fact.Type.Type == nil {
				continue
			}
			decl, ok := result.TypeDef(fact.Type)
			if ok {
				r.aliases[decl.ID] = fact.Type
				r.aliasNames[decl.Name] = decl
				r.knownNames[decl.Name] = struct{}{}
			}
		case cfgfacts.TypeDefinitionInterface:
			if fact.Interface == nil || fact.Interface.Name == "" {
				continue
			}
			decl, ok := result.InterfaceDef(fact.Interface)
			if ok {
				r.interfaces[decl.ID] = fact.Interface
				r.ifaceNames[decl.Name] = decl
				r.knownNames[decl.Name] = struct{}{}
			}
		}
	}
	return r
}

func (r *resultResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) != 1 {
		return resolveFallback(path, r.explicit, r.parent)
	}
	if decl, ok := r.currentBinding(path[0]); ok {
		if t, ok := r.resolveDecl(decl); ok {
			return t, true
		}
	}
	if decl, ok := r.namedBinding(path[0]); ok {
		if t, ok := r.resolveDecl(decl); ok {
			return t, true
		}
	}
	return resolveFallback(path, r.explicit, r.parent)
}

func (r *resultResolver) Type(expr ast.TypeExpr) (typ.Type, bool) {
	if r == nil {
		return typeannotation.Type(expr, nil)
	}
	r.current = append(r.current, expr)
	t, ok := typeannotation.Type(expr, r)
	r.current = r.current[:len(r.current)-1]
	return t, ok
}

func (r *resultResolver) TypeRefResolved(ref *ast.TypeRefExpr) bool {
	if r == nil || ref == nil || len(ref.Path) == 0 {
		return false
	}
	if len(ref.Path) != 1 {
		return true
	}
	if r.result != nil {
		if decl, ok := r.result.TypeRef(ref); ok {
			return r.declVisible(decl)
		}
	}
	_, ok := resolveFallback(ref.Path, r.explicit, r.parent)
	return ok || !r.hasKnownTypeName(ref.Path[0])
}

func (r *resultResolver) PrimitiveTypeResolved(expr *ast.PrimitiveTypeExpr) bool {
	if r == nil || expr == nil {
		return false
	}
	if isBuiltinPrimitiveTypeName(expr.Name) {
		return true
	}
	if r.result != nil {
		if decl, ok := r.result.PrimitiveTypeRef(expr); ok {
			return r.declVisible(decl)
		}
	}
	_, ok := resolveFallback([]string{expr.Name}, r.explicit, r.parent)
	return ok || !r.hasKnownTypeName(expr.Name)
}

func (r *resultResolver) hasKnownTypeName(name string) bool {
	if r == nil || name == "" {
		return false
	}
	if _, ok := r.knownNames[name]; ok {
		return true
	}
	if parent, ok := r.parent.(*resultResolver); ok {
		return parent.hasKnownTypeName(name)
	}
	return false
}

func (r *resultResolver) declVisible(decl bind.TypeDecl) bool {
	if decl.ID == 0 {
		return false
	}
	switch decl.Kind {
	case bind.TypeDeclParam:
		return true
	case bind.TypeDeclAlias:
		if r.aliases[decl.ID] != nil {
			return true
		}
	case bind.TypeDeclInterface:
		if r.interfaces[decl.ID] != nil {
			return true
		}
	}
	if parent, ok := r.parent.(*resultResolver); ok {
		return parent.declVisible(decl)
	}
	return false
}

func (r *resultResolver) currentBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || r.result == nil || name == "" || len(r.current) == 0 {
		return bind.TypeDecl{}, false
	}
	return typeBindingInExpr(r.result, r.current[len(r.current)-1], name)
}

func (r *resultResolver) namedBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || name == "" {
		return bind.TypeDecl{}, false
	}
	if decl, ok := r.aliasNames[name]; ok {
		return decl, true
	}
	if decl, ok := r.ifaceNames[name]; ok {
		return decl, true
	}
	return bind.TypeDecl{}, false
}

func (r *resultResolver) resolveDecl(decl bind.TypeDecl) (typ.Type, bool) {
	if decl.ID == 0 {
		return nil, false
	}
	switch decl.Kind {
	case bind.TypeDeclParam:
		return r.resolveTypeParam(decl)
	case bind.TypeDeclAlias:
		if stmt := r.aliases[decl.ID]; stmt != nil {
			return r.resolveAlias(decl, stmt)
		}
	case bind.TypeDeclInterface:
		if r.interfaces[decl.ID] != nil {
			return typ.NewRef("", decl.Name), true
		}
	}
	if parent, ok := r.parent.(*resultResolver); ok {
		return parent.resolveDecl(decl)
	}
	return nil, false
}

func (r *resultResolver) resolveAlias(decl bind.TypeDecl, stmt *ast.TypeDefStmt) (typ.Type, bool) {
	if stmt == nil {
		return nil, false
	}
	if t, ok := r.cache[decl.ID]; ok {
		return t, true
	}
	key := typeDeclKey{kind: decl.Kind, id: decl.ID}
	if r.active[key] {
		return typ.NewRef("", decl.Name), true
	}
	r.active[key] = true
	var t typ.Type
	var ok bool
	if params := r.result.TypeDefParams(stmt); len(params) > 0 {
		typeParams := make([]*typ.TypeParam, 0, len(params))
		typeParamScope := make(map[string]*typ.TypeParam, len(params))
		for _, param := range params {
			tp, ok := r.resolveTypeParam(param)
			if !ok {
				delete(r.active, key)
				return nil, false
			}
			typeParams = append(typeParams, tp)
			typeParamScope[tp.Name] = tp
		}
		var body typ.Type
		body, ok = typeannotation.Type(stmt.Type, diagnosticTypeParamResolver{
			typeParams: typeParamScope,
			parent:     r,
		})
		if ok {
			t = typ.NewGeneric(decl.Name, typeParams, body)
		}
	} else {
		t, ok = r.Type(stmt.Type)
	}
	delete(r.active, key)
	if !ok {
		return resolveFallback([]string{decl.Name}, r.explicit, r.parent)
	}
	r.cache[decl.ID] = t
	return t, true
}

type diagnosticTypeParamResolver struct {
	typeParams map[string]*typ.TypeParam
	parent     typeannotation.Resolver
}

func (r diagnosticTypeParamResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
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

func (r *resultResolver) resolveTypeParam(decl bind.TypeDecl) (*typ.TypeParam, bool) {
	if decl.ID == 0 {
		return nil, false
	}
	if t, ok := r.params[decl.ID]; ok {
		return t, true
	}
	key := typeDeclKey{kind: decl.Kind, id: decl.ID}
	if r.active[key] {
		return typ.NewTypeParam(decl.Name, nil), true
	}
	r.active[key] = true
	var constraint typ.Type
	if decl.Constraint != nil {
		t, ok := r.Type(decl.Constraint)
		if !ok {
			delete(r.active, key)
			return nil, false
		}
		constraint = t
	}
	delete(r.active, key)
	t := typ.NewTypeParam(decl.Name, constraint)
	r.params[decl.ID] = t
	return t, true
}

func lowerType(expr ast.TypeExpr, resolver typeannotation.Resolver) (typ.Type, bool) {
	if r, ok := resolver.(*resultResolver); ok {
		return r.Type(expr)
	}
	return typeannotation.Type(expr, resolver)
}

func resolveFallback(path []string, resolvers ...typeannotation.Resolver) (typ.Type, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		if t, ok := resolver.ResolveTypeRef(path); ok {
			return t, true
		}
	}
	return nil, false
}

func typeBindingInExpr(result *check.Result, expr ast.TypeExpr, name string) (bind.TypeDecl, bool) {
	if result == nil || expr == nil || name == "" {
		return bind.TypeDecl{}, false
	}
	var found bind.TypeDecl
	var ok bool
	walkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || len(ref.Path) != 1 || ref.Path[0] != name {
			return true
		}
		decl, hasDecl := result.TypeRef(ref)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || prim.Name != name || isBuiltinPrimitiveTypeName(prim.Name) {
			return true
		}
		decl, hasDecl := result.PrimitiveTypeRef(prim)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	})
	return found, ok
}

func walkTypeExpr(expr ast.TypeExpr, visit func(*ast.TypeRefExpr) bool) bool {
	return walkTypeNameExpr(expr, visit, func(*ast.PrimitiveTypeExpr) bool { return true })
}

func walkTypeNameExpr(expr ast.TypeExpr, visitRef func(*ast.TypeRefExpr) bool, visitPrimitive func(*ast.PrimitiveTypeExpr) bool) bool {
	switch expr := expr.(type) {
	case nil:
	case *ast.PrimitiveTypeExpr:
		return visitPrimitive(expr)
	case *ast.SelfTypeExpr, *ast.LiteralTypeExpr, *ast.TypeOfExpr:
	case *ast.OptionalTypeExpr:
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.UnionTypeExpr:
		return walkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.IntersectionTypeExpr:
		return walkTypeNameExprs(expr.Types, visitRef, visitPrimitive)
	case *ast.ArrayTypeExpr:
		return walkTypeNameExpr(expr.Element, visitRef, visitPrimitive)
	case *ast.MapTypeExpr:
		return walkTypeNameExpr(expr.Key, visitRef, visitPrimitive) && walkTypeNameExpr(expr.Value, visitRef, visitPrimitive)
	case *ast.RecordTypeExpr:
		for _, field := range expr.Fields {
			if !walkTypeNameExpr(field.Type, visitRef, visitPrimitive) {
				return false
			}
		}
	case *ast.FunctionTypeExpr:
		for _, param := range expr.TypeParams {
			if !walkTypeNameExpr(param.Constraint, visitRef, visitPrimitive) {
				return false
			}
		}
		for _, param := range expr.Params {
			if !walkTypeNameExpr(param.Type, visitRef, visitPrimitive) {
				return false
			}
		}
		return walkTypeNameExpr(expr.Variadic, visitRef, visitPrimitive) && walkTypeNameExprs(expr.Returns, visitRef, visitPrimitive)
	case *ast.AssertsTypeExpr:
		return walkTypeNameExpr(expr.NarrowTo, visitRef, visitPrimitive)
	case *ast.TypeRefExpr:
		return visitRef(expr)
	case *ast.GenericTypeExpr:
		if !walkTypeNameExpr(expr.Base, visitRef, visitPrimitive) {
			return false
		}
		return walkTypeNameExprs(expr.Args, visitRef, visitPrimitive)
	case *ast.MetaTypeExpr:
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.TupleTypeExpr:
		return walkTypeNameExprs(expr.Elements, visitRef, visitPrimitive)
	case *ast.KeyOfExpr:
		return walkTypeNameExpr(expr.Inner, visitRef, visitPrimitive)
	case *ast.IndexAccessExpr:
		return walkTypeNameExpr(expr.Object, visitRef, visitPrimitive) && walkTypeNameExpr(expr.Index, visitRef, visitPrimitive)
	case *ast.ConditionalTypeExpr:
		return walkTypeNameExpr(expr.Check, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Extends, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Then, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Else, visitRef, visitPrimitive)
	}
	return true
}

func walkTypeExprs(exprs []ast.TypeExpr, visit func(*ast.TypeRefExpr) bool) bool {
	return walkTypeNameExprs(exprs, visit, func(*ast.PrimitiveTypeExpr) bool { return true })
}

func walkTypeNameExprs(exprs []ast.TypeExpr, visitRef func(*ast.TypeRefExpr) bool, visitPrimitive func(*ast.PrimitiveTypeExpr) bool) bool {
	for _, expr := range exprs {
		if !walkTypeNameExpr(expr, visitRef, visitPrimitive) {
			return false
		}
	}
	return true
}

func isBuiltinPrimitiveTypeName(name string) bool {
	switch name {
	case "nil", "boolean", "number", "integer", "string", "any", "unknown", "never", "self":
		return true
	default:
		return false
	}
}
