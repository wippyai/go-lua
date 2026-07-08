package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) Registry() *axis.Registry {
	if r == nil {
		return nil
	}
	return r.registry
}

func (r *Result) TypeValues() *typevalue.Cache {
	if r == nil {
		return nil
	}
	return r.typeValues
}

// ModuleTypes returns the module type-definition read model used while
// checking this body.
func (r *Result) ModuleTypes() typelookup.Source {
	if r == nil {
		return typelookup.Source{}
	}
	return r.moduleTypes
}

// TypeResolver returns the lexical/module-aware type resolver prepared for this
// body. Boundary code that replays declared annotations should use this instead
// of constructing a local-only resolver.
func (r *Result) TypeResolver() *typeresolve.Resolver {
	if r == nil {
		return nil
	}
	return r.typeNS
}

// SignatureManifests returns the imported manifests consulted for module
// function signatures while checking this body.
func (r *Result) SignatureManifests() []*manifest.Manifest {
	if r == nil || len(r.signatures.Manifests) == 0 {
		return nil
	}
	return append([]*manifest.Manifest(nil), r.signatures.Manifests...)
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

// PointReachable reports whether point has a solved non-bottom input state in
// this body's active state domain.
func (r *Result) PointReachable(point cfg.Point) bool {
	if r == nil || r.registry == nil {
		return false
	}
	st, ok := r.solvedStateAt(point)
	if !ok {
		return false
	}
	domain, err := state.TryDomainWithOptionalLanes(r.registry, r.stateLanes)
	if err != nil {
		domain = state.Domain(r.registry)
	}
	return !domain.Equal(state.NormalizeForDomain(domain, st), domain.Bottom())
}

// PointNormallyReachable reports whether normal control flow can reach point
// after branch-edge facts are replayed. It is stricter than PointReachable:
// state lanes can be populated by node-local facts even on an impossible
// branch, so post-solve consumers that enumerate user-facing obligations should
// use this query to avoid reading resurrected unreachable nodes.
func (r *Result) PointNormallyReachable(point cfg.Point) bool {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return false
	}
	if !r.queries.normalReachableSet {
		r.queries.normalReachable = r.computeNormalReachability()
		r.queries.normalReachableSet = true
	}
	return r.queries.normalReachable[point]
}

func (r *Result) computeNormalReachability() map[cfg.Point]bool {
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	reachable := make(map[cfg.Point]bool, graph.Size())
	reachable[graph.Entry()] = true
	for _, point := range graph.RPO() {
		if !reachable[point] {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if graph.IsBranch(point) && !r.EdgeCanCompleteNormally(point, succ) {
				continue
			}
			reachable[succ] = true
		}
	}
	return reachable
}

// EdgeCanCompleteNormally reports whether the selected outgoing edge can carry
// a non-bottom state in this solved body. It reuses the same node and edge
// transfer functions as the body solve so summary projection can distinguish
// syntactic bypasses from paths proven unreachable by the analyzed state.
func (r *Result) EdgeCanCompleteNormally(from, to cfg.Point) bool {
	key := edgeNormalCacheKey{from: from, to: to}
	if r != nil {
		if cached, ok := r.queries.edgeCanCompleteNormally(key); ok {
			return cached
		}
	}
	normal := r.computeEdgeCanCompleteNormally(from, to)
	if r != nil {
		r.queries.rememberEdgeCanCompleteNormally(key, normal)
	}
	return normal
}

func (r *Result) computeEdgeCanCompleteNormally(from, to cfg.Point) bool {
	if r == nil || r.cfg == nil || r.registry == nil || r.boundaryXfer == nil || r.edgeXfer == nil {
		return true
	}
	graph := r.cfg.Graph
	if graph == nil {
		return true
	}
	in, ok := r.solvedStateAt(from)
	if !ok {
		return false
	}
	domain, err := state.TryDomainWithOptionalLanes(r.registry, r.stateLanes)
	if err != nil {
		domain = state.Domain(r.registry)
	}
	if domain.Equal(state.NormalizeForDomain(domain, in), domain.Bottom()) {
		return false
	}
	out := r.boundaryXfer(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    from,
		Node:     graph.Node(from),
		Read: func(point cfg.Point) state.State {
			if st, ok := r.solvedStateAt(point); ok {
				return st
			}
			return domain.Bottom()
		},
	}, in)
	cond, hasCond := graph.EdgeCond(from, to)
	hasCond = hasCond && graph.IsBranch(from)
	out = r.edgeXfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: r.registry,
		Edge:     cfg.Edge{From: from, To: to, Cond: cond},
		HasCond:  hasCond,
	}, out)
	return !domain.Equal(state.NormalizeForDomain(domain, out), domain.Bottom())
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

func (r *Result) ReturnFact(point cfg.Point) (ReturnFact, bool) {
	if r == nil || r.cfg == nil {
		return ReturnFact{}, false
	}
	return r.cfg.Returns.Get(point)
}

func (r *Result) LocalAssignment(point cfg.Point) (LocalAssignmentFact, bool) {
	if r == nil || r.cfg == nil {
		return LocalAssignmentFact{}, false
	}
	return r.cfg.Assignments.Local(point)
}

func (r *Result) LoweredLocalAssignment(point cfg.Point) (factflow.RootAssignment, bool) {
	if r == nil {
		return factflow.RootAssignment{}, false
	}
	return r.facts.LocalAssignment(point)
}

func (r *Result) RootAssignment(point cfg.Point) (factflow.RootAssignment, bool) {
	if r == nil {
		return factflow.RootAssignment{}, false
	}
	return r.facts.RootAssignment(point)
}

func (r *Result) PathAssignment(point cfg.Point) (factflow.PathAssignment, bool) {
	if r == nil {
		return factflow.PathAssignment{}, false
	}
	return r.facts.PathAssignment(point)
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

func (r *Result) ObjectLiteralViewForSource(source factflow.ValueSource) (factflow.ObjectLiteralView, bool) {
	if r == nil || !source.HasExpr || source.ExprRef == 0 {
		return factflow.ObjectLiteralView{}, false
	}
	return r.facts.ObjectLiteralView(source.ExprRef)
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (OrdinaryAssignmentFact, bool) {
	if r == nil || r.cfg == nil {
		return OrdinaryAssignmentFact{}, false
	}
	return r.cfg.Assignments.Ordinary(point)
}

func (r *Result) Call(point cfg.Point) (CallFact, bool) {
	if r == nil || r.cfg == nil {
		return CallFact{}, false
	}
	return r.cfg.Calls.Get(point)
}

func (r *Result) CallView(point cfg.Point) (CallFactView, bool) {
	if r == nil || r.cfg == nil {
		return CallFactView{}, false
	}
	return r.cfg.Calls.View(point)
}

func (r *Result) CallSite(point cfg.Point) (factflow.CallSite, bool) {
	if r == nil {
		return factflow.CallSite{}, false
	}
	return r.facts.CallSite(point)
}

func (r *Result) CallSiteView(point cfg.Point) (factflow.CallSiteView, bool) {
	if r == nil {
		return factflow.CallSiteView{}, false
	}
	return r.facts.CallSiteView(point)
}

func (r *Result) ChannelSelects(point cfg.Point) []factflow.ChannelSelect {
	if r == nil {
		return nil
	}
	return r.facts.ChannelSelects(point)
}

func (r *Result) DynamicIndexWrite(point cfg.Point) (factflow.DynamicIndexWrite, bool) {
	if r == nil {
		return factflow.DynamicIndexWrite{}, false
	}
	return r.facts.DynamicIndexWrite(point)
}

func (r *Result) PathDescendantInvalidation(point cfg.Point) (factflow.PathDescendantInvalidation, bool) {
	if r == nil {
		return factflow.PathDescendantInvalidation{}, false
	}
	return r.facts.PathDescendantInvalidation(point)
}

func (r *Result) ObjectLiteralExpr(expr factflow.ExprRef) (factflow.ObjectLiteral, bool) {
	if r == nil {
		return factflow.ObjectLiteral{}, false
	}
	return r.facts.ObjectLiteral(expr)
}

func (r *Result) ExpressionValueRef(expr factflow.ExprRef) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	return r.facts.ExpressionValue(expr)
}

func (r *Result) ExpressionPathRef(expr factflow.ExprRef) (path.Path, bool) {
	if r == nil {
		return path.Path{}, false
	}
	return r.facts.ExpressionPathRef(expr)
}

func (r *Result) ExpressionOperationRef(expr factflow.ExprRef) (factflow.ExpressionOperation, bool) {
	if r == nil {
		return factflow.ExpressionOperation{}, false
	}
	return r.facts.ExpressionOperation(expr)
}

func (r *Result) DominatingPathRootDeclarationSource(point cfg.Point, target path.Path) (factquery.RootDeclarationSource, bool) {
	if r == nil || point == 0 || target.IsEmpty() || target.Symbol == 0 {
		return factquery.RootDeclarationSource{}, false
	}
	graph := r.Graph()
	if graph == nil {
		return factquery.RootDeclarationSource{}, false
	}
	return factquery.NewRootDeclarationQueryWithDominators(
		r.facts,
		r.queries.immediateDominatorMap(graph),
		graph.Size(),
	).DominatingPathRootDeclarationSource(point, target)
}

func (r *Result) DynamicIndexExpressionRef(expr factflow.ExprRef) (factflow.DynamicIndexExpression, bool) {
	if r == nil {
		return factflow.DynamicIndexExpression{}, false
	}
	return r.facts.DynamicIndexExpression(expr)
}

func (r *Result) CovariantExposures(point cfg.Point) []factflow.CovariantExposure {
	if r == nil {
		return nil
	}
	return r.facts.CovariantExposures(point)
}

func (r *Result) BranchPathEvidence(point cfg.Point) []factflow.BranchPathEvidence {
	if r == nil {
		return nil
	}
	return r.facts.BranchPathEvidence(point)
}

func (r *Result) BranchPathRelations(point cfg.Point) []factflow.BranchPathRelation {
	if r == nil {
		return nil
	}
	return r.facts.BranchPathRelations(point)
}

func (r *Result) BranchSufficientLiteralCases(point cfg.Point) []factflow.BranchSufficientLiteralCase {
	if r == nil {
		return nil
	}
	return r.facts.BranchSufficientLiteralCases(point)
}

func (r *Result) NoNormalReturn(point cfg.Point) bool {
	if r == nil {
		return false
	}
	return r.facts.NoNormalReturn(point)
}

func (r *Result) BranchCondition(point cfg.Point) (BranchConditionFact, bool) {
	if r == nil {
		return BranchConditionFact{}, false
	}
	site, ok := r.branchSite(point)
	if !ok {
		return BranchConditionFact{}, false
	}
	check, ok := r.BranchConditionCheck(point)
	if !ok {
		return BranchConditionFact{}, false
	}
	return branchConditionFactFromSite(site, check), true
}

// BranchConditionCheck returns the canonical direct branch check for point.
// WIR owns normalized branch predicates; callers that need expression-level
// implication checks should query expression facts instead of semantic sidecars.
func (r *Result) BranchConditionCheck(point cfg.Point) (branchcond.Check, bool) {
	if r == nil {
		return branchcond.Check{}, false
	}
	return r.branchConditionCheckFromWIR(point)
}

func (r *Result) branchConditionCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	if r == nil || r.wir == nil {
		return branchcond.Check{}, false
	}
	var out branchcond.Check
	var found bool
	r.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchConditionCheckFromWIR(check)
		if candidate.Kind == branchcond.CheckNone {
			return true
		}
		out = candidate
		found = true
		return false
	})
	return out, found
}

func branchConditionCheckFromWIR(check wir.Check) branchcond.Check {
	return branchcond.Check{
		Kind:          branchcond.CheckKind(check.Kind),
		Path:          check.Path,
		OtherPath:     check.OtherPath,
		TypeName:      check.TypeName,
		Literal:       check.Literal,
		LiteralString: check.LiteralString,
		LenFloor:      check.LenFloor,
		NumFloor:      check.NumFloor,
		Negated:       check.Negated,
	}
}

func (r *Result) ExpressionImpliedChecksOnEdge(expr ast.Expr, cond bool) []branchcond.ImpliedCheck {
	if r == nil || r.bindings == nil || expr == nil {
		return nil
	}
	return branchcond.ImpliedChecksOnEdge(expr, r.bindings, cond)
}

func (r *Result) TypeDefinition(point cfg.Point) (cfgbuild.TypeDefinition, bool) {
	if r == nil || r.cfg == nil {
		return cfgbuild.TypeDefinition{}, false
	}
	return r.cfg.Declarations.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgbuild.FunctionDefinition, bool) {
	if r == nil || r.cfg == nil {
		return cfgbuild.FunctionDefinition{}, false
	}
	return r.cfg.Declarations.FunctionDefinition(point)
}

func (r *Result) NumericFor(point cfg.Point) (cfgbuild.NumericFor, bool) {
	if r == nil || r.cfg == nil {
		return cfgbuild.NumericFor{}, false
	}
	return r.cfg.NumericFors.Get(point)
}

func (r *Result) GenericFor(point cfg.Point) (cfgbuild.GenericFor, bool) {
	if r == nil || r.cfg == nil {
		return cfgbuild.GenericFor{}, false
	}
	return r.cfg.GenericFors.Get(point)
}

func (r *Result) ExpressionEvaluation(point cfg.Point) (cfgbuild.ExpressionEvaluation, bool) {
	if r == nil || r.cfg == nil {
		return cfgbuild.ExpressionEvaluation{}, false
	}
	return r.cfg.ShortCircuits.Evaluation(point)
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return r.function
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

func (r *Result) FunctionParamTypeSpan(fn *ast.FunctionExpr, index int) (factflow.SourceSpan, bool) {
	if r == nil || r.bindings == nil || fn == nil || index < 0 {
		return factflow.SourceSpan{}, false
	}
	slots := r.bindings.ParamSlots(fn)
	if index >= len(slots) || slots[index].Type == nil {
		return factflow.SourceSpan{}, false
	}
	span := ast.SpanOf(slots[index].Type)
	if span.StartLine == 0 || span.StartCol == 0 {
		return factflow.SourceSpan{}, false
	}
	return factflow.SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}, true
}

func (r *Result) FunctionParamTypeSpansForTargetPath(target path.Path) []factflow.SourceSpan {
	if r == nil || r.bindings == nil || target.IsEmpty() || r.Graph() == nil {
		return nil
	}
	for _, point := range r.Graph().RPO() {
		def, ok := r.FunctionDefinition(point)
		if !ok || !def.HasTargetPath || !def.TargetPath.Equal(target) || def.Func == nil {
			continue
		}
		slots := r.bindings.ParamSlots(def.Func)
		if len(slots) == 0 {
			return nil
		}
		out := make([]factflow.SourceSpan, len(slots))
		for i := range slots {
			if span, ok := r.FunctionParamTypeSpan(def.Func, i); ok {
				out[i] = span
			}
		}
		return out
	}
	return nil
}

func (r *Result) FunctionReturnTypeSpansForTargetPath(target path.Path) []factflow.SourceSpan {
	if r == nil || target.IsEmpty() || r.Graph() == nil {
		return nil
	}
	for _, point := range r.Graph().RPO() {
		def, ok := r.FunctionDefinition(point)
		if !ok || !def.HasTargetPath || !def.TargetPath.Equal(target) || def.Func == nil {
			continue
		}
		if len(def.Func.ReturnTypes) == 0 {
			return nil
		}
		out := make([]factflow.SourceSpan, len(def.Func.ReturnTypes))
		for i, ret := range def.Func.ReturnTypes {
			span := ast.SpanOf(ret)
			if span.StartLine == 0 || span.StartCol == 0 {
				continue
			}
			out[i] = factflow.SourceSpan{
				StartLine: span.StartLine,
				StartCol:  span.StartCol,
				EndLine:   span.EndLine,
				EndCol:    span.EndCol,
			}
		}
		return out
	}
	return nil
}

func (r *Result) FunctionTypeParams(fn *ast.FunctionExpr) []bind.TypeDecl {
	if r == nil || r.bindings == nil || fn == nil {
		return nil
	}
	return r.bindings.FunctionTypeParams(fn)
}

func (r *Result) MethodReceiverTypeDecl(fn *ast.FunctionExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil || fn == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.MethodReceiverType(fn)
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

// IsCallContextResult reports whether this result was solved for one concrete
// call context rather than the function's context-independent summary body.
func (r *Result) IsCallContextResult() bool {
	return r != nil && r.callContext
}

// HasBodyOwnedParamObligations reports whether this function body's solved
// summary contains parameter preconditions inferred from how the body uses its
// own parameters. Such preconditions are enforced at call boundaries, so
// top/unknown values derived solely from them should not also become return
// contract diagnostics inside the body.
func (r *Result) HasBodyOwnedParamObligations() bool {
	return r != nil && r.bodyParamObligations
}

// WithBodyOwnedParamObligations records whether result's summary contains
// body-owned parameter preconditions. Program-level summary materialization
// owns this metadata; diagnostics only read it through the read model.
func WithBodyOwnedParamObligations(result *Result, has bool) *Result {
	if result == nil {
		return nil
	}
	result.bodyParamObligations = has
	return result
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

// WithCallContextResult marks result as the materialization for one caller's
// argument/effect context. Diagnostics use this to keep cross-boundary reports
// owned by the call site instead of also reporting inside the specialized body.
func WithCallContextResult(result *Result) *Result {
	if result == nil {
		return nil
	}
	result.callContext = true
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

func (r *Result) IdentResolvesToGlobal(ident *ast.IdentExpr, name string) bool {
	if r == nil || r.bindings == nil {
		return false
	}
	return r.bindings.ResolvesToGlobal(ident, name)
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

// ExpressionRefPath resolves a factflow expression reference to the static path
// recorded during lowering.
func (r *Result) ExpressionRefPath(expr factflow.ExprRef) (path.Path, bool) {
	if r == nil {
		return path.Path{}, false
	}
	return r.facts.ExpressionPath(expr)
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

// ExpressionSignatureNameAt resolves an expression to the stable signature name
// used for imported or explicit global function lookups at point.
func (r *Result) ExpressionSignatureNameAt(point cfg.Point, expr ast.Expr) (string, bool) {
	if r == nil || expr == nil {
		return "", false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok {
		return "", false
	}
	return r.PathSignatureNameAt(point, p)
}

// PathSignatureNameAt resolves a path to the stable imported or explicit global
// function signature name at point.
func (r *Result) PathSignatureNameAt(point cfg.Point, p path.Path) (string, bool) {
	if r == nil {
		return "", false
	}
	if r.signatureID != nil {
		if name, ok := r.signatureID.stableCalleeName(p.Symbol, p); ok {
			return name, true
		}
	}
	return r.modules.SignatureName(point, p)
}

// PathSignatureAt resolves a path to a known imported or explicit global
// function signature at point.
func (r *Result) PathSignatureAt(point cfg.Point, p path.Path) (signature.Function, bool) {
	name, ok := r.PathSignatureNameAt(point, p)
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

// PathSignatureTypeAt resolves a path to the read-only function type of a known
// imported or explicit global function signature at point.
func (r *Result) PathSignatureTypeAt(point cfg.Point, p path.Path) (*typ.Function, bool) {
	name, ok := r.PathSignatureNameAt(point, p)
	if !ok {
		return nil, false
	}
	return r.SignatureType(name)
}

func (r *Result) CallSignature(site factflow.CallSite) (signature.Function, bool) {
	name, ok := r.CallSignatureName(site)
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

// CallSignatureType resolves a call site to the read-only function type of its
// known signature. It avoids cloning signature carriers for callers that only
// need the type; CallSignature retains the defensive-copy API for effect-aware
// consumers.
func (r *Result) CallSignatureType(site factflow.CallSite) (*typ.Function, bool) {
	name, ok := r.CallSignatureName(site)
	if !ok {
		return nil, false
	}
	return r.SignatureType(name)
}

// CallSignatureTypeAt resolves the known function type for a call site at its
// CFG point. This is the canonical solver/readmodel query when point-sensitive
// facts, such as ambient global replacement, can affect signature identity.
func (r *Result) CallSignatureTypeAt(point cfg.Point, site factflow.CallSite) (*typ.Function, bool) {
	name, ok := r.CallSignatureNameAt(point, site)
	if !ok {
		return nil, false
	}
	return r.SignatureType(name)
}

// CallSiteViewSignatureType resolves a read-only call-site view to the read-only
// function type of its known signature.
func (r *Result) CallSiteViewSignatureType(site factflow.CallSiteView) (*typ.Function, bool) {
	if r == nil || r.signatureID == nil {
		return nil, false
	}
	name, ok := r.signatureID.nameForIndexedCallSiteView(site)
	if !ok {
		return nil, false
	}
	return r.SignatureType(name)
}

// SignatureType returns the read-only function type for signature name.
func (r *Result) SignatureType(name string) (*typ.Function, bool) {
	if r == nil || name == "" {
		return nil, false
	}
	if cached, ok := r.queries.signatureType(name); ok {
		return cached.value, cached.ok
	}
	sig, ok := r.signatures.Lookup(name)
	cached := cachedSignatureType{ok: ok && sig.Type != nil}
	if cached.ok {
		cached.value = sig.Type
	}
	r.queries.rememberSignatureType(name, cached)
	return cached.value, cached.ok
}

func (r *Result) CallSignatureName(site factflow.CallSite) (string, bool) {
	if r == nil || r.signatureID == nil {
		return "", false
	}
	return r.signatureID.nameForSite(site)
}

// CallSignatureNameAt resolves the signature name for a call at its CFG point.
func (r *Result) CallSignatureNameAt(point cfg.Point, site factflow.CallSite) (string, bool) {
	if r == nil || r.signatureID == nil {
		return "", false
	}
	graph := r.Graph()
	ctx := transfer.NodeContext{
		Graph: graph,
		Point: point,
	}
	if graph != nil {
		ctx.Node = graph.Node(point)
	}
	return r.signatureID.nameForCallSiteView(ctx, site.View())
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

func (r *Result) LocalOrigin(id symbol.ID) (bind.LocalOrigin, bool) {
	if r == nil || r.bindings == nil {
		return bind.LocalOrigin{}, false
	}
	return r.bindings.LocalOrigin(id)
}

func (r *Result) IsImplicitGlobalUse(ident *ast.IdentExpr) bool {
	if r == nil || r.bindings == nil {
		return false
	}
	return r.bindings.IsImplicitGlobalUse(ident)
}

func (r *Result) IsFunctionDefinitionTarget(id symbol.ID) bool {
	if r == nil || r.bindings == nil || id == 0 {
		return false
	}
	for _, origin := range r.bindings.FunctionOrigins() {
		if origin.HasTargetSymbol && origin.TargetSymbol == id {
			return true
		}
	}
	return false
}

func (r *Result) TypeRef(ref *ast.TypeRefExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeRef(ref)
}

func (r *Result) TypeValueRef(ident *ast.IdentExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeValueRef(ident)
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
