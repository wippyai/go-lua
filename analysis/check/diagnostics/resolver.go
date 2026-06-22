package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type resultResolver struct {
	result *body.Result

	aliases    map[bind.TypeDeclID]*ast.TypeDefStmt
	interfaces map[bind.TypeDeclID]*ast.InterfaceDefStmt
	aliasNames map[string]bind.TypeDecl
	ifaceNames map[string]bind.TypeDecl
	knownNames map[string]struct{}
	cache      map[bind.TypeDeclID]typ.Type
	params     map[bind.TypeDeclID]*typ.TypeParam
	active     map[typeDeclKey]bool
	activeRec  map[bind.TypeDeclID]*typ.Recursive
	generic    map[bind.TypeDeclID]bool

	current []ast.TypeExpr

	parent     typeannotation.Resolver
	moduleRefs typeannotation.Resolver
}

type typeDeclKey struct {
	kind bind.TypeDeclKind
	id   bind.TypeDeclID
}

func newResultResolver(result *body.Result, parent typeannotation.Resolver) *resultResolver {
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
		activeRec:  make(map[bind.TypeDeclID]*typ.Recursive),
		generic:    make(map[bind.TypeDeclID]bool),
		parent:     parent,
	}
	if result != nil {
		r.moduleRefs = result.ModuleTypes()
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
		if t, ok := resolveInParentScope(path, r.parent); ok {
			return t, true
		}
		if r.moduleRefs != nil {
			if r.result != nil && len(path) > 1 {
				if modulePath, ok := r.result.RequireAliasModulePath(path[0]); ok {
					rewritten := append(strings.Split(modulePath, "."), path[1:]...)
					if t, ok := r.moduleRefs.ResolveTypeRef(rewritten); ok {
						return t, true
					}
				}
			}
			return r.moduleRefs.ResolveTypeRef(path)
		}
		return nil, false
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
	return resolveInParentScope(path, r.parent)
}

func (r *resultResolver) Type(expr ast.TypeExpr) (typ.Type, bool) {
	if r == nil {
		return typeannotation.Type(expr, nil)
	}
	return typeannotation.TypeWithGuard(expr, r, &r.current)
}

func (r *resultResolver) TypeRefResolved(ref *ast.TypeRefExpr) bool {
	if r == nil || ref == nil || len(ref.Path) == 0 {
		return false
	}
	if _, ok := r.ResolveTypeRef(ref.Path); ok {
		return true
	}
	if len(ref.Path) != 1 {
		return false
	}
	if r.result != nil {
		if _, ok := r.result.TypeRef(ref); ok {
			return true
		}
	}
	_, ok := resolveInParentScope(ref.Path, r.parent)
	return ok || !r.hasKnownTypeName(ref.Path[0])
}

func (r *resultResolver) PrimitiveTypeResolved(expr *ast.PrimitiveTypeExpr) bool {
	if r == nil || expr == nil {
		return false
	}
	if typ.BuiltinPrimitiveName(expr.Name) {
		return true
	}
	if r.result != nil {
		if _, ok := r.result.PrimitiveTypeRef(expr); ok {
			return true
		}
	}
	_, ok := resolveInParentScope([]string{expr.Name}, r.parent)
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

func (r *resultResolver) currentBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || r.result == nil || name == "" || len(r.current) == 0 {
		return bind.TypeDecl{}, false
	}
	return typeresolve.BindingInExpr(r.result, r.current[len(r.current)-1], name)
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
	key := typeDeclKey{kind: decl.Kind, id: decl.ID}
	if r.active[key] {
		switch decl.Kind {
		case bind.TypeDeclAlias:
			if !r.generic[decl.ID] {
				rec, ok := r.activeRec[decl.ID]
				if !ok {
					rec = typ.NewRecursivePlaceholder(decl.Name)
					r.activeRec[decl.ID] = rec
				}
				return rec, true
			}
			return typ.NewRef("", decl.Name), true
		case bind.TypeDeclInterface:
			return r.activeInterfaceRef(decl)
		}
	}
	switch decl.Kind {
	case bind.TypeDeclParam:
		return r.resolveTypeParam(decl)
	case bind.TypeDeclAlias:
		if stmt := r.aliases[decl.ID]; stmt != nil {
			return r.resolveAlias(decl, stmt)
		}
	case bind.TypeDeclInterface:
		if stmt := r.interfaces[decl.ID]; stmt != nil {
			return r.resolveInterface(decl, stmt)
		}
	}
	if parent, ok := r.parent.(*resultResolver); ok {
		return parent.resolveDecl(decl)
	}
	return nil, false
}

func (r *resultResolver) activeInterfaceRef(decl bind.TypeDecl) (typ.Type, bool) {
	if r.interfaces[decl.ID] == nil {
		return nil, false
	}
	if rec := r.activeRec[decl.ID]; rec != nil {
		return rec, true
	}
	rec := typ.NewRecursivePlaceholder(decl.Name)
	r.activeRec[decl.ID] = rec
	return rec, true
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
		// A direct self-reference reached during this alias's own body
		// resolution binds to a shared recursive placeholder so the resulting
		// type is a closed mu-type whose self-references downstream projection
		// and the subtype checker can unfold, rather than a Ref resolved only
		// by name. Generic decls recurse through their Generic body and keep a
		// plain Ref placeholder.
		if !r.generic[decl.ID] {
			rec, ok := r.activeRec[decl.ID]
			if !ok {
				rec = typ.NewRecursivePlaceholder(decl.Name)
				r.activeRec[decl.ID] = rec
			}
			return rec, true
		}
		return typ.NewRef("", decl.Name), true
	}
	r.active[key] = true
	var t typ.Type
	var ok bool
	if params := r.result.TypeDefParams(stmt); len(params) > 0 {
		r.generic[decl.ID] = true
		typeParams := make([]*typ.TypeParam, 0, len(params))
		typeParamScope := make(map[string]*typ.TypeParam, len(params))
		for _, param := range params {
			tp, ok := r.resolveTypeParam(param)
			if !ok {
				delete(r.active, key)
				delete(r.generic, decl.ID)
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
	rec := r.activeRec[decl.ID]
	delete(r.active, key)
	delete(r.activeRec, decl.ID)
	delete(r.generic, decl.ID)
	if !ok {
		return resolveInParentScope([]string{decl.Name}, r.parent)
	}
	if rec != nil {
		rec.SetBody(t)
		t = rec
	}
	r.cache[decl.ID] = t
	return t, true
}

func (r *resultResolver) resolveInterface(decl bind.TypeDecl, stmt *ast.InterfaceDefStmt) (typ.Type, bool) {
	if stmt == nil {
		return nil, false
	}
	if t, ok := r.cache[decl.ID]; ok {
		return t, true
	}
	key := typeDeclKey{kind: decl.Kind, id: decl.ID}
	if r.active[key] {
		return r.activeInterfaceRef(decl)
	}
	if len(stmt.Fields) != 0 {
		return nil, false
	}
	r.active[key] = true

	methods := make([]typ.Method, 0, len(stmt.Methods))
	seen := make(map[string]*typ.Function, len(stmt.Methods))
	merge := func(method typ.Method) bool {
		if method.Name == "" || method.Type == nil {
			return false
		}
		if existing, ok := seen[method.Name]; ok {
			return typ.TypeEquals(existing, method.Type)
		}
		seen[method.Name] = method.Type
		methods = append(methods, method)
		return true
	}

	ok := true
	for _, ref := range stmt.Extends {
		parentType, parentOK := r.Type(ref)
		parent, ifaceOK := diagnosticInterfaceBody(parentType)
		if !parentOK || !ifaceOK {
			ok = false
			break
		}
		for _, method := range parent.Methods {
			if !merge(method) {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
	}
	if ok {
		for _, method := range stmt.Methods {
			if method.Type == nil {
				ok = false
				break
			}
			t, methodOK := r.Type(method.Type)
			fn, fnOK := t.(*typ.Function)
			if !methodOK || !fnOK || !merge(typ.Method{Name: method.Name, Type: fn}) {
				ok = false
				break
			}
		}
	}

	rec := r.activeRec[decl.ID]
	delete(r.active, key)
	delete(r.activeRec, decl.ID)
	if !ok {
		return nil, false
	}
	var t typ.Type = typ.NewInterface(stmt.Name, methods)
	if rec != nil {
		rec.SetBody(t)
		t = rec
	}
	r.cache[decl.ID] = t
	return t, true
}

func diagnosticInterfaceBody(t typ.Type) (*typ.Interface, bool) {
	switch v := t.(type) {
	case *typ.Interface:
		return v, true
	case *typ.Recursive:
		if body, ok := v.Body.(*typ.Interface); ok {
			return body, true
		}
	}
	return nil, false
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

func resolveInParentScope(path []string, parent typeannotation.Resolver) (typ.Type, bool) {
	if parent == nil {
		return nil, false
	}
	if t, ok := parent.ResolveTypeRef(path); ok {
		return t, true
	}
	return nil, false
}
