// Package typeresolve resolves Lua lexical type references into typ.Type values.
package typeresolve

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Bindings is the lexical type-binding surface required by Resolver.
type Bindings interface {
	TypeRef(*ast.TypeRefExpr) (bind.TypeDecl, bool)
	PrimitiveTypeRef(*ast.PrimitiveTypeExpr) (bind.TypeDecl, bool)
	TypeDefParams(*ast.TypeDefStmt) []bind.TypeDecl
}

// Resolver resolves AST type expressions through lexical type bindings.
type Resolver struct {
	bindings  Bindings
	external  typeannotation.Resolver
	cache     map[bind.TypeDeclID]typ.Type
	params    map[bind.TypeDeclID]*typ.TypeParam
	active    map[bind.TypeDeclID]bool
	activeRec map[bind.TypeDeclID]*typ.Recursive
	current   []ast.TypeExpr
}

// New creates a lexical type resolver over bindings.
func New(bindings Bindings) *Resolver {
	return NewWithExternal(bindings, nil)
}

// NewWithExternal creates a lexical resolver with a secondary qualified-name
// resolver for module-boundary type definitions.
func NewWithExternal(bindings Bindings, external typeannotation.Resolver) *Resolver {
	return &Resolver{
		bindings:  bindings,
		external:  external,
		cache:     make(map[bind.TypeDeclID]typ.Type),
		params:    make(map[bind.TypeDeclID]*typ.TypeParam),
		active:    make(map[bind.TypeDeclID]bool),
		activeRec: make(map[bind.TypeDeclID]*typ.Recursive),
	}
}

// Type lowers an AST type expression to a typ.Type.
func (r *Resolver) Type(expr ast.TypeExpr) (typ.Type, bool) {
	if r == nil {
		return typeannotation.Type(expr, nil)
	}
	r.current = append(r.current, expr)
	t, ok := typeannotation.Type(expr, r)
	r.current = r.current[:len(r.current)-1]
	return t, ok
}

// Decl resolves a bound type declaration.
func (r *Resolver) Decl(decl bind.TypeDecl) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.resolveDecl(decl)
}

// ResolveTypeRef resolves a typeannotation reference through the current
// expression's lexical bindings, falling back to the external module-boundary
// resolver for qualified or unbound names.
func (r *Resolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) == 1 {
		if decl, ok := r.currentBinding(path[0]); ok {
			return r.resolveDecl(decl)
		}
	}
	if r.external != nil {
		return r.external.ResolveTypeRef(path)
	}
	return nil, false
}

func (r *Resolver) currentBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil || name == "" || len(r.current) == 0 {
		return bind.TypeDecl{}, false
	}
	return BindingInExpr(r.bindings, r.current[len(r.current)-1], name)
}

func (r *Resolver) resolveDecl(decl bind.TypeDecl) (typ.Type, bool) {
	if decl.ID == 0 {
		return nil, false
	}
	if r.active[decl.ID] && decl.Kind == bind.TypeDeclAlias {
		return r.activeAliasRef(decl)
	}
	switch decl.Kind {
	case bind.TypeDeclParam:
		return r.resolveTypeParam(decl)
	case bind.TypeDeclAlias:
		if decl.Type != nil {
			return r.resolveAlias(decl, decl.Type)
		}
	case bind.TypeDeclInterface:
		if decl.Interface != nil {
			return typ.NewRef("", decl.Name), true
		}
	}
	return nil, false
}

func (r *Resolver) activeAliasRef(decl bind.TypeDecl) (typ.Type, bool) {
	if decl.Type == nil || r.bindings == nil || len(r.bindings.TypeDefParams(decl.Type)) != 0 {
		return typ.NewRef("", decl.Name), true
	}
	if rec := r.activeRec[decl.ID]; rec != nil {
		return rec, true
	}
	rec := typ.NewRecursivePlaceholder(decl.Name)
	r.activeRec[decl.ID] = rec
	return rec, true
}

func (r *Resolver) resolveAlias(decl bind.TypeDecl, stmt *ast.TypeDefStmt) (typ.Type, bool) {
	if stmt == nil {
		return nil, false
	}
	if t, ok := r.cache[decl.ID]; ok {
		return t, true
	}
	if r.active[decl.ID] {
		return typ.NewRef("", decl.Name), true
	}
	r.active[decl.ID] = true
	var t typ.Type
	var ok bool
	var params []bind.TypeDecl
	if r.bindings != nil {
		params = r.bindings.TypeDefParams(stmt)
	}
	if len(params) > 0 {
		typeParams := make([]*typ.TypeParam, 0, len(params))
		for _, param := range params {
			tp, paramOK := r.resolveTypeParam(param)
			if !paramOK {
				delete(r.active, decl.ID)
				return nil, false
			}
			typeParams = append(typeParams, tp)
		}
		var body typ.Type
		body, ok = r.Type(stmt.Type)
		if ok {
			t = typ.NewGeneric(decl.Name, typeParams, body)
		}
	} else {
		t, ok = r.Type(stmt.Type)
	}
	rec := r.activeRec[decl.ID]
	delete(r.active, decl.ID)
	delete(r.activeRec, decl.ID)
	if !ok {
		return nil, false
	}
	if rec != nil {
		rec.SetBody(t)
		t = rec
	}
	r.cache[decl.ID] = t
	return t, true
}

func (r *Resolver) resolveTypeParam(decl bind.TypeDecl) (*typ.TypeParam, bool) {
	if decl.ID == 0 {
		return nil, false
	}
	if t, ok := r.params[decl.ID]; ok {
		return t, true
	}
	if r.active[decl.ID] {
		return typ.NewTypeParam(decl.Name, nil), true
	}
	r.active[decl.ID] = true
	var constraint typ.Type
	if decl.Constraint != nil {
		t, ok := r.Type(decl.Constraint)
		if !ok {
			delete(r.active, decl.ID)
			return nil, false
		}
		constraint = t
	}
	delete(r.active, decl.ID)
	t := typ.NewTypeParam(decl.Name, constraint)
	r.params[decl.ID] = t
	return t, true
}
