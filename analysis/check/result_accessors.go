package check

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) Registry() *axis.Registry {
	if r == nil {
		return nil
	}
	return r.registry
}

func (r *Result) Graph() cfg.Graph {
	if r == nil || r.cfg == nil {
		return nil
	}
	return r.cfg.Graph
}

func (r *Result) StateAt(point cfg.Point) (state.State, bool) {
	if r == nil || r.flow == nil {
		return state.State{}, false
	}
	st, ok := r.flow[point]
	if !ok {
		return state.State{}, false
	}
	return st.Clone(), true
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

func (r *Result) ReturnPoints() []cfg.Point {
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	points := graph.RPO()
	out := make([]cfg.Point, 0, len(points))
	for _, point := range points {
		if _, ok := r.ReturnFact(point); ok {
			out = append(out, point)
		}
	}
	return out
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

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil || r.semantics == nil {
		return nil
	}
	return r.semantics.Function()
}

func (r *Result) FunctionResults() []*Result {
	if r == nil || len(r.functions) == 0 {
		return nil
	}
	return append([]*Result(nil), r.functions...)
}

func (r *Result) SymbolName(id symbol.ID) string {
	if r == nil || r.bindings == nil {
		return ""
	}
	return r.bindings.Name(id)
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

func (r *Result) CallSignature(site factflow.CallSite) (signature.Function, bool) {
	if r == nil {
		return signature.Function{}, false
	}
	name, ok := r.stableCalleeName(site.CalleeSymbol(), site.CalleePath())
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

func (r *Result) ReturnArity(point cfg.Point) (int, bool) {
	if r == nil {
		return 0, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return 0, false
	}
	return len(fact.Sources()), true
}

func (r *Result) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	if r == nil {
		return nil, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return nil, false
	}
	return fact.Sources(), true
}

func (r *Result) ReturnPresenceRelations(point cfg.Point) []factflow.ReturnPresenceRelation {
	if r == nil {
		return nil
	}
	relations := r.facts.ReturnPresenceRelations(point)
	if delegated := r.openTailReturnPresenceRelations(point); len(delegated) != 0 {
		relations = append(relations, delegated...)
	}
	return relations
}

func (r *Result) openTailReturnPresenceRelations(point cfg.Point) []factflow.ReturnPresenceRelation {
	if r == nil || r.callOutcome == nil {
		return nil
	}
	ret, ok := r.facts.Return(point)
	if !ok {
		return nil
	}
	sources := ret.Sources()
	if len(sources) != 1 {
		return nil
	}
	source := sources[0]
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || !source.OpenTail || !source.Expanded {
		return nil
	}
	site, ok := r.facts.CallSite(source.CallPoint)
	if !ok {
		return nil
	}
	in, ok := r.StateAt(source.CallPoint)
	if !ok {
		return nil
	}
	graph := r.Graph()
	ctx := transfer.NodeContext{
		Graph:    graph,
		Point:    source.CallPoint,
		Registry: r.registry,
		Read:     r.boundaryRead,
	}
	if graph != nil {
		ctx.Node = graph.Node(source.CallPoint)
	}
	outcome := r.callOutcome(ctx, site, in, r.boundaryRead)
	if len(outcome.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]factflow.ReturnPresenceRelation, 0, len(outcome.ReturnPresenceRelations))
	for _, relation := range outcome.ReturnPresenceRelations {
		out = append(out, factflow.NewReturnPresenceRelation(
			relation.TriggerIndex,
			relation.TriggerPresence,
			relation.TargetIndex,
			relation.TargetPresence,
		))
	}
	return out
}

func (r *Result) ExpressionCondition(ref factflow.ExprRef) (factflow.ExpressionCondition, bool) {
	if r == nil {
		return factflow.ExpressionCondition{}, false
	}
	return r.facts.ExpressionCondition(ref)
}

func (r *Result) ParameterValueSlots() []statekey.Value {
	if r == nil || r.bindings == nil {
		return nil
	}
	slots := r.bindings.ParamSlots(r.Function())
	out := make([]statekey.Value, 0, len(slots))
	for _, slot := range slots {
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			continue
		}
		out = append(out, valueSlot)
	}
	return out
}

func (r *Result) ReassignedParameterValueSlots() map[statekey.Value]struct{} {
	if r == nil || r.bindings == nil {
		return nil
	}
	params := make(map[statekey.Value]struct{})
	for _, slot := range r.ParameterValueSlots() {
		params[slot] = struct{}{}
	}
	if len(params) == 0 {
		return nil
	}
	out := make(map[statekey.Value]struct{})
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	for _, point := range graph.RPO() {
		assignment, ok := r.facts.RootAssignment(point)
		if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
			continue
		}
		slot := statekey.SymbolValue(assignment.TargetSymbol())
		if _, ok := params[slot]; ok {
			out[slot] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Result) LocalSymbols(stmt *ast.LocalAssignStmt) []symbol.ID {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.LocalSymbols(stmt)
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
