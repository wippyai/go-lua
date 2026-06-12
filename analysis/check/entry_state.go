package check

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type paramEntrySeed struct {
	slot  key.Value
	value product.Value
}

func parameterEntryState(
	reg *axis.Registry,
	graph cfg.Graph,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	entry state.State,
	initial transfer.InitialState,
) (state.State, transfer.InitialState) {
	seeds := functionParamEntrySeeds(reg, bindings, fn)
	if len(seeds) == 0 {
		return entry, initial
	}
	entry = seedEntryStateValues(reg, entry, seeds)
	if graph == nil || initial == nil {
		return entry, initial
	}
	entryPoint := graph.Entry()
	return entry, func(point cfg.Point) (state.State, bool) {
		st, ok := initial(point)
		if !ok {
			return state.State{}, false
		}
		if point == entryPoint {
			st = seedEntryStateValues(reg, st, seeds)
		}
		return st, true
	}
}

func functionParamEntrySeeds(reg *axis.Registry, bindings *bind.Result, fn *ast.FunctionExpr) []paramEntrySeed {
	if reg == nil || bindings == nil || fn == nil {
		return nil
	}
	resolver := newEntryTypeResolver(bindings)
	slots := bindings.ParamSlots(fn)
	seeds := make([]paramEntrySeed, 0, len(slots))
	for _, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		valueSlot := key.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			continue
		}
		if slot.Type == nil {
			seeds = append(seeds, paramEntrySeed{
				slot:  valueSlot,
				value: product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop()),
			})
			continue
		}
		t, ok := resolver.Type(slot.Type)
		if !ok {
			continue
		}
		seeds = append(seeds, paramEntrySeed{
			slot:  valueSlot,
			value: typevalue.FromType(reg, t),
		})
	}
	return seeds
}

func seedEntryStateValues(reg *axis.Registry, entry state.State, seeds []paramEntrySeed) state.State {
	if reg == nil || len(seeds) == 0 {
		return entry
	}
	bottom := product.Bottom(reg)
	out := entry
	for _, seed := range seeds {
		if seed.slot == "" {
			continue
		}
		if !product.Equal(reg, out.ReadValue(reg, seed.slot), bottom) {
			continue
		}
		out = out.WriteValue(reg, seed.slot, seed.value)
	}
	return out
}

type entryTypeResolver struct {
	bindings *bind.Result
	cache    map[bind.TypeDeclID]typ.Type
	params   map[bind.TypeDeclID]*typ.TypeParam
	active   map[bind.TypeDeclID]bool
	current  []ast.TypeExpr
}

func newEntryTypeResolver(bindings *bind.Result) *entryTypeResolver {
	return &entryTypeResolver{
		bindings: bindings,
		cache:    make(map[bind.TypeDeclID]typ.Type),
		params:   make(map[bind.TypeDeclID]*typ.TypeParam),
		active:   make(map[bind.TypeDeclID]bool),
	}
}

func (r *entryTypeResolver) Type(expr ast.TypeExpr) (typ.Type, bool) {
	if r == nil {
		return typeannotation.Type(expr, nil)
	}
	r.current = append(r.current, expr)
	t, ok := typeannotation.Type(expr, r)
	r.current = r.current[:len(r.current)-1]
	return t, ok
}

func (r *entryTypeResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) != 1 {
		return nil, false
	}
	decl, ok := r.currentBinding(path[0])
	if !ok {
		return nil, false
	}
	return r.resolveDecl(decl)
}

func (r *entryTypeResolver) currentBinding(name string) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil || name == "" || len(r.current) == 0 {
		return bind.TypeDecl{}, false
	}
	return typeBindingInExpr(r.bindings, r.current[len(r.current)-1], name)
}

func (r *entryTypeResolver) resolveDecl(decl bind.TypeDecl) (typ.Type, bool) {
	if decl.ID == 0 {
		return nil, false
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

func (r *entryTypeResolver) resolveAlias(decl bind.TypeDecl, stmt *ast.TypeDefStmt) (typ.Type, bool) {
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
	if params := r.bindings.TypeDefParams(stmt); len(params) > 0 {
		typeParams := make([]*typ.TypeParam, 0, len(params))
		for _, param := range params {
			tp, ok := r.resolveTypeParam(param)
			if !ok {
				delete(r.active, decl.ID)
				return nil, false
			}
			typeParams = append(typeParams, tp)
		}
		body, ok := r.Type(stmt.Type)
		if ok {
			t = typ.NewGeneric(decl.Name, typeParams, body)
		}
	} else {
		t, ok = r.Type(stmt.Type)
	}
	delete(r.active, decl.ID)
	if !ok {
		return nil, false
	}
	r.cache[decl.ID] = t
	return t, true
}

func (r *entryTypeResolver) resolveTypeParam(decl bind.TypeDecl) (*typ.TypeParam, bool) {
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

func typeBindingInExpr(bindings *bind.Result, expr ast.TypeExpr, name string) (bind.TypeDecl, bool) {
	if bindings == nil || expr == nil || name == "" {
		return bind.TypeDecl{}, false
	}
	var found bind.TypeDecl
	var ok bool
	walkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || len(ref.Path) != 1 || ref.Path[0] != name {
			return true
		}
		decl, hasDecl := bindings.TypeRef(ref)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || prim.Name != name || isBuiltinPrimitiveTypeName(prim.Name) {
			return true
		}
		decl, hasDecl := bindings.PrimitiveTypeRef(prim)
		if !hasDecl {
			return true
		}
		found, ok = decl, true
		return false
	})
	return found, ok
}

func walkTypeNameExpr(
	expr ast.TypeExpr,
	visitRef func(*ast.TypeRefExpr) bool,
	visitPrimitive func(*ast.PrimitiveTypeExpr) bool,
) bool {
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
		return walkTypeNameExpr(expr.Key, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Value, visitRef, visitPrimitive)
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
		return walkTypeNameExpr(expr.Variadic, visitRef, visitPrimitive) &&
			walkTypeNameExprs(expr.Returns, visitRef, visitPrimitive)
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
		return walkTypeNameExpr(expr.Object, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Index, visitRef, visitPrimitive)
	case *ast.ConditionalTypeExpr:
		return walkTypeNameExpr(expr.Check, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Extends, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Then, visitRef, visitPrimitive) &&
			walkTypeNameExpr(expr.Else, visitRef, visitPrimitive)
	}
	return true
}

func walkTypeNameExprs(
	exprs []ast.TypeExpr,
	visitRef func(*ast.TypeRefExpr) bool,
	visitPrimitive func(*ast.PrimitiveTypeExpr) bool,
) bool {
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
