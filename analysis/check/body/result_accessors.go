package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) Registry() *axis.Registry {
	if r == nil {
		return nil
	}
	return r.registry
}

// ModuleTypes returns the module type-definition read model used while
// checking this body.
func (r *Result) ModuleTypes() typelookup.Source {
	if r == nil {
		return typelookup.Source{}
	}
	return r.moduleTypes
}

func (r *Result) Graph() cfg.Graph {
	if r == nil || r.cfg == nil {
		return nil
	}
	return r.cfg.Graph
}

// KeySpace returns the per-analysis structural key interner used by the
// path-evidence value lane. Snapshot and value-lane accessors thread it.
func (r *Result) KeySpace() *keyspace.KeySpace {
	if r == nil {
		return nil
	}
	return r.visibility.KeySpace()
}

func (r *Result) StateAt(point cfg.Point) (state.State, bool) {
	st, ok := r.solvedStateAt(point)
	if !ok {
		return state.State{}, false
	}
	return st.Snapshot(), true
}

func (r *Result) solvedStateAt(point cfg.Point) (state.State, bool) {
	if r == nil || r.flow == nil {
		return state.State{}, false
	}
	st, ok := r.flow[point]
	if !ok {
		return state.State{}, false
	}
	return st, true
}

func (r *Result) ExitState() (state.State, bool) {
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	return r.StateAt(graph.Exit())
}

func (r *Result) EntryState() (state.State, bool) {
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	return r.StateAt(graph.Entry())
}

func (r *Result) ReturnFact(point cfg.Point) (semantics.ReturnFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.ReturnFact{}, false
	}
	return r.semantics.Return(point)
}

func (r *Result) LocalAssignment(point cfg.Point) (semantics.LocalAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.LocalAssignmentFact{}, false
	}
	return r.semantics.LocalAssignment(point)
}

func (r *Result) LoweredLocalAssignment(point cfg.Point) (factflow.RootAssignment, bool) {
	if r == nil {
		return factflow.RootAssignment{}, false
	}
	return r.facts.LocalAssignment(point)
}

// RequireAliasModulePath resolves a local require-binding alias to the module
// path it imports. A statement such as local store_mod = require("store") binds
// the alias store_mod to module path store, so a qualified type reference
// store_mod.Store can be resolved against the importing module's manifest.
func (r *Result) RequireAliasModulePath(name string) (string, bool) {
	if r == nil || name == "" {
		return "", false
	}
	return r.modules.ModulePathForAlias(name)
}

func (r *Result) ObjectLiteral(expr ast.Expr) (semantics.ObjectLiteralFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.ObjectLiteralFact{}, false
	}
	return r.semantics.ObjectLiteral(expr)
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (semantics.OrdinaryAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.OrdinaryAssignmentFact{}, false
	}
	return r.semantics.OrdinaryAssignment(point)
}

func (r *Result) Call(point cfg.Point) (semantics.CallFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.CallFact{}, false
	}
	return r.semantics.Call(point)
}

func (r *Result) CallSite(point cfg.Point) (factflow.CallSite, bool) {
	if r == nil {
		return factflow.CallSite{}, false
	}
	return r.facts.CallSite(point)
}

func (r *Result) DynamicIndexWrite(point cfg.Point) (factflow.DynamicIndexWrite, bool) {
	if r == nil {
		return factflow.DynamicIndexWrite{}, false
	}
	return r.facts.DynamicIndexWrite(point)
}

func (r *Result) ObjectLiteralExpr(expr factflow.ExprRef) (factflow.ObjectLiteral, bool) {
	if r == nil {
		return factflow.ObjectLiteral{}, false
	}
	return r.facts.ObjectLiteral(expr)
}

func (r *Result) ExpressionPathRef(expr factflow.ExprRef) (path.Path, bool) {
	if r == nil {
		return path.Path{}, false
	}
	return r.facts.ExpressionPath(expr)
}

func (r *Result) CovariantExposures(point cfg.Point) []factflow.CovariantExposure {
	if r == nil {
		return nil
	}
	return r.facts.CovariantExposures(point)
}

func (r *Result) NoNormalReturn(point cfg.Point) bool {
	if r == nil {
		return false
	}
	return r.facts.NoNormalReturn(point)
}

func (r *Result) BranchCondition(point cfg.Point) (semantics.BranchConditionFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.BranchConditionFact{}, false
	}
	return r.semantics.BranchCondition(point)
}

func (r *Result) TypeDefinition(point cfg.Point) (cfgfacts.TypeDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.TypeDefinitionFact{}, false
	}
	return r.semantics.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.FunctionDefinitionFact{}, false
	}
	return r.semantics.FunctionDefinition(point)
}

func (r *Result) NumericFor(point cfg.Point) (cfgfacts.NumericForFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.NumericForFact{}, false
	}
	return r.semantics.NumericFor(point)
}

func (r *Result) GenericFor(point cfg.Point) (cfgfacts.GenericForFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.GenericForFact{}, false
	}
	return r.semantics.GenericFor(point)
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil || r.semantics == nil {
		return nil
	}
	return r.semantics.Function()
}

func (r *Result) FunctionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if r == nil || r.bindings == nil || fn == nil {
		return 0, false
	}
	return r.bindings.FunctionSymbol(fn)
}

func (r *Result) FunctionBySymbol(id symbol.ID) (*ast.FunctionExpr, bool) {
	if r == nil || r.bindings == nil || id == 0 {
		return nil, false
	}
	return r.bindings.FunctionBySymbol(id)
}

func (r *Result) FunctionOrigin(fn *ast.FunctionExpr) (bind.FunctionOrigin, bool) {
	if r == nil || r.bindings == nil || fn == nil {
		return bind.FunctionOrigin{}, false
	}
	return r.bindings.FunctionOrigin(fn)
}

func (r *Result) FunctionParamSlots(fn *ast.FunctionExpr) []bind.ParamSlot {
	if r == nil || r.bindings == nil || fn == nil {
		return nil
	}
	return r.bindings.ParamSlots(fn)
}

func (r *Result) ExpressionFunction(expr factflow.ExprRef) (symbol.ID, bool) {
	if r == nil {
		return 0, false
	}
	return r.facts.ExpressionFunction(expr)
}

func (r *Result) FunctionResults() []*Result {
	if r == nil || len(r.functions) == 0 {
		return nil
	}
	return append([]*Result(nil), r.functions...)
}

func (r *Result) DirectCaptures(fn *ast.FunctionExpr) []bind.Capture {
	if r == nil || r.bindings == nil || fn == nil {
		return nil
	}
	return r.bindings.DirectCaptures(fn)
}

// WithFunctionResults returns result after replacing its materialized nested
// function results. Program-level fixed-point materialization owns population;
// body analysis itself runs exactly one body.
func WithFunctionResults(result *Result, functions []*Result) *Result {
	if result == nil {
		return nil
	}
	result.functions = append([]*Result(nil), functions...)
	return result
}

func (r *Result) SymbolName(id symbol.ID) string {
	if r == nil || r.bindings == nil {
		return ""
	}
	return r.bindings.Name(id)
}

func (r *Result) SymbolKind(id symbol.ID) (symbol.Kind, bool) {
	if r == nil || r.bindings == nil {
		return symbol.Unknown, false
	}
	return r.bindings.Kind(id)
}

func (r *Result) SymbolOfIdent(ident *ast.IdentExpr) (symbol.ID, bool) {
	if r == nil || r.bindings == nil || ident == nil {
		return 0, false
	}
	return r.bindings.SymbolOf(ident)
}

func (r *Result) SymbolTypeAnnotation(id symbol.ID) (ast.TypeExpr, bool) {
	if r == nil || r.bindings == nil || id == 0 {
		return nil, false
	}
	return r.bindings.SymbolTypeAnnotation(id)
}

func (r *Result) ExpressionPath(expr ast.Expr) (path.Path, bool) {
	if r == nil || r.bindings == nil {
		return path.Path{}, false
	}
	return pathexpr.Resolve(expr, r.bindings)
}

// ExpressionSignatureAt resolves an expression to a known imported or explicit
// global function signature at point.
func (r *Result) ExpressionSignatureAt(point cfg.Point, expr ast.Expr) (signature.Function, bool) {
	if r == nil || expr == nil {
		return signature.Function{}, false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok {
		return signature.Function{}, false
	}
	return r.PathSignatureAt(point, p)
}

// PathSignatureAt resolves a path to a known imported or explicit global
// function signature at point.
func (r *Result) PathSignatureAt(point cfg.Point, p path.Path) (signature.Function, bool) {
	if r == nil {
		return signature.Function{}, false
	}
	if r.signatureID != nil {
		if name, ok := r.signatureID.stableCalleeName(p.Symbol, p); ok {
			return r.signatures.Lookup(name)
		}
	}
	name, ok := r.modules.SignatureName(point, p)
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

func (r *Result) CallSignature(site factflow.CallSite) (signature.Function, bool) {
	name, ok := r.CallSignatureName(site)
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

func (r *Result) CallSignatureName(site factflow.CallSite) (string, bool) {
	if r == nil || r.signatureID == nil {
		return "", false
	}
	return r.signatureID.nameForSite(site)
}

func (r *Result) ExpressionCondition(ref factflow.ExprRef) (factflow.ExpressionCondition, bool) {
	if r == nil {
		return factflow.ExpressionCondition{}, false
	}
	return r.facts.ExpressionCondition(ref)
}

func (r *Result) LocalSymbols(stmt *ast.LocalAssignStmt) []symbol.ID {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.LocalSymbols(stmt)
}

func (r *Result) SymbolHasRead(id symbol.ID) bool {
	if r == nil || r.bindings == nil {
		return false
	}
	return r.bindings.HasRead(id)
}

func (r *Result) SymbolReadIdents(id symbol.ID) []*ast.IdentExpr {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.ReadIdents(id)
}

func (r *Result) IsImplicitGlobalUse(ident *ast.IdentExpr) bool {
	if r == nil || r.bindings == nil {
		return false
	}
	return r.bindings.IsImplicitGlobalUse(ident)
}

func (r *Result) TypeRef(ref *ast.TypeRefExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeRef(ref)
}

func (r *Result) PrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.PrimitiveTypeRef(expr)
}

func (r *Result) TypeDef(stmt *ast.TypeDefStmt) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeDef(stmt)
}

func (r *Result) InterfaceDef(stmt *ast.InterfaceDefStmt) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.InterfaceDef(stmt)
}

func (r *Result) TypeDefParams(stmt *ast.TypeDefStmt) []bind.TypeDecl {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.TypeDefParams(stmt)
}

func (r *Result) SymbolValueAt(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if r == nil || id == 0 {
		return product.Value{}, false
	}
	st, ok := r.StateAt(point)
	if !ok {
		return product.Value{}, false
	}
	value := st.ReadValue(r.registry, statekey.SymbolValue(id))
	if product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}
