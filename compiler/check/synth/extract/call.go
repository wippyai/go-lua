package extract

import (
	"strconv"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/callreturn"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/callarg"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/callboundary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallQuery adapts the Synthesizer's type operations into the core.TypeOps interface.
//
// This allows the ops package (which handles generic call synthesis) to query
// methods, fields, and perform subtype checks without directly depending on
// the synthesizer. Instantiated type parameters are automatically substituted
// when looking up methods/fields.
type CallQuery struct {
	s *Synthesizer
}

func (q CallQuery) Method(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	mt, ok := q.s.deps.Types.Method(ctx, t, name)
	if !ok || mt == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		mt = subst.Params(mt, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return mt, true
}

func (q CallQuery) Field(ctx *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	return q.s.deps.Types.Field(ctx, t, name)
}

func (q CallQuery) Index(ctx *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool) {
	if q.s == nil || q.s.deps.Types == nil {
		return nil, false
	}
	return q.s.deps.Types.Index(ctx, t, key)
}

func (q CallQuery) BinaryOp(ctx *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return typ.Unknown
	}
	return q.s.deps.Types.BinaryOp(ctx, left, op, right)
}

func (q CallQuery) UnaryOp(ctx *db.QueryContext, op string, operand typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return typ.Unknown
	}
	return q.s.deps.Types.UnaryOp(ctx, op, operand)
}

func (q CallQuery) IsSubtype(ctx *db.QueryContext, sub, super typ.Type) bool {
	if q.s == nil || q.s.deps.Types == nil {
		return false
	}
	return q.s.deps.Types.IsSubtype(ctx, sub, super)
}

func (q CallQuery) ExpandInstantiated(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return t
	}
	return q.s.deps.Types.ExpandInstantiated(ctx, t)
}

func (q CallQuery) Widen(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return t
	}
	return q.s.deps.Types.Widen(ctx, t)
}

func (q CallQuery) WidenForInference(ctx *db.QueryContext, t typ.Type) typ.Type {
	if q.s == nil || q.s.deps.Types == nil {
		return t
	}
	return q.s.deps.Types.WidenForInference(ctx, t)
}

// GetCallQuery returns a core.TypeOps implementation.
func (s *Synthesizer) GetCallQuery() core.TypeOps {
	return CallQuery{s: s}
}

// SynthCallCore synthesizes return types from a function call expression.
//
// Call synthesis proceeds through several stages:
// 1. Intercept check: Special calls (require, select, type casts) may be handled
// 2. Argument synthesis: Compute types for all argument expressions
// 3. Call pipeline: Infer type args, re-synthesize callbacks, finish call
// 4. Post-call transforms: Apply spec-based return type overrides and effects
//
// For method calls (obj:method()), dispatches to synthMethodCallCoreWithExpected.
func (s *Synthesizer) SynthCallCore(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) []typ.Type {
	return s.synthCallCoreWithCaptureTypes(ex, p, sc, narrower, recurse, nil, nil)
}

// synthCallCoreWithNarrower synthesizes call with narrower context preserved.
func (s *Synthesizer) synthCallCoreWithNarrower(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth, expected typ.Type) []typ.Type {
	return s.synthCallCoreWithCaptureTypes(ex, p, sc, narrower, recurse, expected, nil)
}

func (s *Synthesizer) synthCallCoreWithCaptureTypes(
	ex *ast.FuncCallExpr,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	recurse ExprSynth,
	expected typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) []typ.Type {
	if callsite.IsMethodLikeExpr(ex) {
		return s.synthMethodCallCoreWithExpected(ex, p, sc, recurse, expected)
	}

	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredProjectionLookup(sc),
		StableType: func(expr ast.Expr, current typ.Type) typ.Type {
			return s.stablePrototypeType(expr, p, sc, current, recurse)
		},
		CanonicalMetatable: s.canonicalMetatableType,
		Bindings:           s.activeBindings(),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptCall(ex, env); result.Skip {
		return result.Types
	}

	ownerExpr := environmentOwnerExpr(ex)
	var ownerType typ.Type
	if ownerExpr != nil {
		ownerType = recurse(ownerExpr)
	}
	calleeType := recurse(ex.Func)
	if specialized := s.specializedLocalFunctionCalleeType(ex, p, sc, calleeType, captureTypes); specialized != nil {
		calleeType = specialized
	}
	typeArgs := s.resolveTypeArgs(ex.TypeArgs, sc)
	allowExtraArgs := s.localFunctionAllowsDiscardedExtraArgs(ex, p)
	probeDef := ops.CallDef{
		Callee:         calleeType,
		TypeArgs:       typeArgs,
		Query:          s.GetCallQuery(),
		ExpectedReturn: expected,
		AllowExtraArgs: allowExtraArgs,
	}
	args := s.synthArgsWithCallContext(ex.Args, p, sc, recurse, probeDef)

	def := ops.CallDef{
		Callee:         calleeType,
		Args:           args,
		TypeArgs:       typeArgs,
		Query:          s.GetCallQuery(),
		ExpectedReturn: expected,
		AllowExtraArgs: allowExtraArgs,
	}

	pipeline := ops.NewCallPipeline(s.deps.Ctx, def, len(ex.Args)).
		WithReSynth(s.contextualArgReSynth(calleeType, ex.Args, sc, p))

	if expected != nil {
		pipeline = pipeline.WithExpected(expected)
	}

	result := pipeline.Run()

	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns, nil, false, false)
	returns = s.applyEnvironmentReturnProjection(calleeType, ex, ownerExpr, ownerType, args, p, sc, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

func (s *Synthesizer) specializedLocalFunctionCalleeType(
	ex *ast.FuncCallExpr,
	p cfg.Point,
	sc *scope.State,
	current typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if s == nil || ex == nil || s.deps.CheckCtx == nil {
		return nil
	}
	if specialized := s.stableLocalFunctionValueType(ex.Func, p, sc, current, captureTypes); specialized != nil {
		return specialized
	}
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return nil
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	if bindings == nil {
		return nil
	}
	evidence := s.graphEvidence(graph)
	info := graph.CallSiteAt(p, ex)
	candidates := callsite.CallableCalleeSymbolCandidates(info, graph, bindings, nil)
	if len(candidates) == 0 {
		if sym := callsite.SymbolFromExpr(ex.Func, bindings); sym != 0 {
			candidates = append(candidates, sym)
		}
		if s.deps.ModuleBindings != nil && s.deps.ModuleBindings != bindings {
			if sym := callsite.SymbolFromExpr(ex.Func, s.deps.ModuleBindings); sym != 0 {
				candidates = append(candidates, sym)
			}
		}
	}
	for _, sym := range candidates {
		fn := callsite.FunctionLiteralForGraphSymbol(evidence, sym)
		if fn != nil && !s.hasDominatingDirectFunctionRebind(sym, fn, p) {
			factType := s.functionFactValueType(sym)
			hasCallPointCaptureMutation := hasNonGlobalFunctionCaptures(bindings, fn) && s.hasDominatingCapturedMutation(fn, p)
			if factType != nil && !hasCallPointCaptureMutation {
				return factType
			}
			expectedFn, _ := unwrap.Optional(unwrap.Alias(current)).(*typ.Function)
			if expectedFn == nil {
				expectedFn, _ = unwrap.Optional(unwrap.Alias(factType)).(*typ.Function)
			}
			if fnType := s.synthFunctionTypeWithCapturePoint(fn, sc, expectedFn, p, captureTypes); fnType != nil {
				return fnType
			}
			if factType != nil {
				return factType
			}
		}
		if typ.IsUnknownOrNil(current) {
			if t := s.functionFactValueType(sym); t != nil {
				return t
			}
		}
	}
	return nil
}

func (s *Synthesizer) localFunctionAllowsDiscardedExtraArgs(ex *ast.FuncCallExpr, p cfg.Point) bool {
	if s == nil || ex == nil || s.deps.CheckCtx == nil {
		return false
	}
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	evidence := s.graphEvidence(graph)
	info := graph.CallSiteAt(p, ex)
	for _, sym := range callsite.CallableCalleeSymbolCandidates(info, graph, bindings, nil) {
		if fn := callsite.FunctionLiteralForGraphSymbol(evidence, sym); callsite.AllowsDiscardedExtraArgs(fn) {
			return true
		}
	}
	if bindings != nil {
		if sym := callsite.SymbolFromExpr(ex.Func, bindings); sym != 0 {
			return callsite.AllowsDiscardedExtraArgs(callsite.FunctionLiteralForGraphSymbol(evidence, sym))
		}
	}
	return false
}

// SynthCallCoreWithExpected synthesizes call with optional expected return type for generic inference.
func (s *Synthesizer) SynthCallCoreWithExpected(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type) []typ.Type {
	return s.synthCallCoreWithNarrower(ex, p, sc, nil, recurse, expected)
}

// synthMethodCallCoreWithExpected synthesizes method call with optional expected return type.
func (s *Synthesizer) synthMethodCallCoreWithExpected(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recurse ExprSynth, expected typ.Type) []typ.Type {
	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredProjectionLookup(sc),
		StableType: func(expr ast.Expr, current typ.Type) typ.Type {
			return s.stablePrototypeType(expr, p, sc, current, recurse)
		},
		Bindings: s.activeBindings(),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptMethodCall(ex, env); result.Skip {
		return result.Types
	}

	recvType := recurse(ex.Receiver)
	calleeType := s.resolveMethodCallee(recvType, ex.Method)
	forceReceiver := s.forceMethodReceiverAtPoint(p, ex)
	probeDef := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Query:               s.GetCallQuery(),
		ExpectedReturn:      expected,
		ForceMethodReceiver: forceReceiver,
	}
	args := s.synthArgsWithCallContext(ex.Args, p, sc, recurse, probeDef)

	def := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Args:                args,
		Query:               s.GetCallQuery(),
		ExpectedReturn:      expected,
		ForceMethodReceiver: forceReceiver,
	}

	pipeline := ops.NewCallPipeline(s.deps.Ctx, def, len(ex.Args)).
		WithReSynth(s.contextualArgReSynth(calleeType, ex.Args, sc, p))

	if expected != nil {
		pipeline = pipeline.WithExpected(expected)
	}

	result := pipeline.Run()
	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns, recvType, true, forceReceiver)
	returns = s.applyEnvironmentReturnProjection(calleeType, ex, ex.Receiver, recvType, args, p, sc, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

// SynthCallWithReceiverType synthesizes method call with an explicit receiver type.
func (s *Synthesizer) SynthCallWithReceiverType(ex *ast.FuncCallExpr, p cfg.Point, sc *scope.State, recvType typ.Type, recurse ExprSynth) []typ.Type {
	env := intercept.CallEnv{
		Scope:      sc,
		Recurse:    intercept.ExprSynth(recurse),
		TypeLookup: s.declaredProjectionLookup(sc),
		StableType: func(expr ast.Expr, current typ.Type) typ.Type {
			return s.stablePrototypeType(expr, p, sc, current, recurse)
		},
		Bindings: s.activeBindings(),
	}

	chain := s.buildInterceptChain(sc)
	if result := chain.InterceptMethodCall(ex, env); result.Skip {
		return result.Types
	}

	calleeType := s.resolveMethodCallee(recvType, ex.Method)
	forceReceiver := s.forceMethodReceiverAtPoint(p, ex)
	probeDef := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Query:               s.GetCallQuery(),
		ForceMethodReceiver: forceReceiver,
	}
	args := s.synthArgsWithCallContext(ex.Args, p, sc, recurse, probeDef)

	def := ops.CallDef{
		IsMethod:            true,
		Receiver:            recvType,
		MethodName:          ex.Method,
		Args:                args,
		Query:               s.GetCallQuery(),
		ForceMethodReceiver: forceReceiver,
	}

	pipeline := ops.NewCallPipeline(s.deps.Ctx, def, len(ex.Args)).
		WithReSynth(s.contextualArgReSynth(calleeType, ex.Args, sc, p))

	result := pipeline.Run()
	returns := unwrapCallResult(result)
	returns = s.applyPostCallTransforms(calleeType, args, returns, recvType, true, forceReceiver)
	returns = s.applyEnvironmentReturnProjection(calleeType, ex, ex.Receiver, recvType, args, p, sc, returns)

	specOverride := s.specReturnOverride(calleeType, ex.Args, args)
	return intercept.ApplyOverride(returns, specOverride)
}

// declaredTypeLookup returns a function that resolves identifier names to their
// declared function types. Used by intercepts for effect-based dispatch.
func (s *Synthesizer) declaredProjectionLookup(sc *scope.State) func(string) typ.Type {
	return func(name string) typ.Type {
		// Check global types first (require, select, type, etc.)
		if s.deps.CheckCtx != nil {
			if t, ok := s.deps.CheckCtx.GlobalTypeOverlay().Type(name); ok {
				return t
			}
		}
		// Check scope metadata for type names (Number, Point, etc.)
		if sc != nil {
			if meta := sc.MetaForName(name); meta != nil {
				return typ.Func().
					Param("value", typ.Any).
					Returns(meta.Of).
					Effects(effect.WithCallableType()).
					Build()
			}
		}
		return nil
	}
}

func (s *Synthesizer) stablePrototypeType(expr ast.Expr, p cfg.Point, sc *scope.State, current typ.Type, recurse ExprSynth) typ.Type {
	return s.stablePrototypeTypeSeen(expr, p, sc, current, recurse, nil)
}

// canonicalMetatableType maps a setmetatable metatable argument to the shared
// interned class family keyed by the metatable binding's origin symbol. When expr
// is a direct class identifier and current is a class allocation, the result is
// the one interned *typ.Recursive family whose body widens in place across the
// inter-procedural fixpoint, so a constructor's stored metatable edge always sees
// the converged class. Otherwise current is returned unchanged: a non-identifier
// metatable, an unresolved symbol, or a plain (non-class) record keeps the
// existing structural-snapshot behavior.
func (s *Synthesizer) canonicalMetatableType(expr ast.Expr, current typ.Type) typ.Type {
	if s == nil || expr == nil || current == nil || s.deps == nil || s.deps.RecursiveFamilies == nil {
		return current
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident.Value == "" {
		return current
	}
	if !metatable.IsClassShaped(current) {
		return current
	}
	sym := s.metatableOriginSymbol(ident)
	if sym == 0 {
		return current
	}
	key := typ.FamilyKey{Namespace: "class", Owner: strconv.FormatUint(uint64(sym), 10)}
	return metatable.SealClassFamilyInterned(current, key, s.deps.RecursiveFamilies, functionfact.ClassFamilyJoin)
}

// metatableOriginSymbol resolves a class identifier to its binding symbol,
// preferring the active graph's bindings and falling back to module bindings.
func (s *Synthesizer) metatableOriginSymbol(ident *ast.IdentExpr) compcfg.SymbolID {
	if s == nil || ident == nil || s.deps == nil {
		return 0
	}
	var bindings *bind.BindingTable
	if s.deps.CheckCtx != nil {
		if graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok && graph != nil {
			bindings = graph.Bindings()
		}
	}
	if bindings != nil {
		if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
			return sym
		}
	}
	if s.deps.ModuleBindings != nil && s.deps.ModuleBindings != bindings {
		if sym, ok := s.deps.ModuleBindings.SymbolOf(ident); ok && sym != 0 {
			return sym
		}
	}
	return 0
}

func (s *Synthesizer) stablePrototypeTypeSeen(expr ast.Expr, p cfg.Point, sc *scope.State, current typ.Type, recurse ExprSynth, seen map[compcfg.SymbolID]bool) typ.Type {
	if s == nil || expr == nil || s.deps.CheckCtx == nil {
		return current
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident.Value == "" {
		return current
	}
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return current
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	if bindings == nil {
		return current
	}
	sym, ok := bindings.SymbolOf(ident)
	if (!ok || sym == 0) && s.deps.ModuleBindings != nil && s.deps.ModuleBindings != bindings {
		sym, ok = s.deps.ModuleBindings.SymbolOf(ident)
	}
	if !ok || sym == 0 {
		return current
	}
	if seen == nil {
		seen = make(map[compcfg.SymbolID]bool)
	}
	if seen[sym] {
		return current
	}
	seen[sym] = true
	defer delete(seen, sym)

	fields := s.stablePrototypeFields(s.stablePrototypeGraphs(graph), sym, sc, recurse, seen)
	if len(fields) == 0 {
		return current
	}

	var base *typ.Record
	if rec := unwrap.Record(current); rec != nil && !typ.IsUnknown(rec.Metatable) {
		base = rec
	}
	builder := typ.NewRecord()
	if base != nil {
		for _, field := range base.Fields {
			key, ok := fieldkey.FromName(field.Name)
			if !ok {
				continue
			}
			fields[key] = value.MergeForConvergence(fields[key], field.Type)
		}
		for _, member := range base.StaticMembers {
			key, ok := stablePrototypeStaticMemberKey(member)
			if !ok {
				continue
			}
			fields[key] = value.MergeForConvergence(fields[key], member.Type)
		}
		if base.Metatable != nil {
			builder.Metatable(base.Metatable)
		}
		if base.HasMapComponent() {
			builder.MapComponent(base.MapKey, base.MapValue)
		}
		builder.SetOpen(base.Open)
	}

	for _, key := range fieldkey.Sorted(fields) {
		t := fields[key]
		if t == nil {
			t = typ.Unknown
		}
		addStablePrototypeField(builder, key, t)
	}
	return builder.Build()
}

func (s *Synthesizer) stablePrototypeGraphs(current *compcfg.Graph) []*compcfg.Graph {
	if current == nil {
		return nil
	}
	graphs := []*compcfg.Graph{current}
	if s == nil || s.deps == nil || s.deps.Graphs == nil {
		return graphs
	}
	type rootGraphProvider interface {
		RootGraph() *compcfg.Graph
	}
	provider, ok := s.deps.Graphs.(rootGraphProvider)
	if !ok {
		return graphs
	}
	root := provider.RootGraph()
	if root != nil && root != current {
		graphs = append(graphs, root)
	}
	return graphs
}

type stablePrototypeFieldTypes map[fieldkey.Key]typ.Type

func (s *Synthesizer) stablePrototypeFields(graphs []*compcfg.Graph, sym compcfg.SymbolID, sc *scope.State, recurse ExprSynth, seen map[compcfg.SymbolID]bool) stablePrototypeFieldTypes {
	if len(graphs) == 0 || sym == 0 {
		return nil
	}
	var fields stablePrototypeFieldTypes
	addField := func(key fieldkey.Key, t typ.Type) {
		if !stablePrototypeFieldKeyValid(key) {
			return
		}
		if t == nil {
			t = typ.Unknown
		}
		if fields == nil {
			fields = make(stablePrototypeFieldTypes)
		}
		if existing := fields[key]; existing != nil {
			fields[key] = value.MergeForConvergence(existing, t)
		} else {
			fields[key] = t
		}
	}
	for _, graph := range graphs {
		if graph == nil {
			continue
		}
		bindings := graph.Bindings()
		evidence := s.graphEvidence(graph)
		for _, assign := range evidence.Assignments {
			p := assign.Point
			info := assign.Info
			if info == nil {
				continue
			}
			sources := info.Sources
			for i, target := range info.Targets {
				if target.Kind == compcfg.TargetIdent && target.Symbol == sym {
					if source := assignmentSourceAt(sources, i); source != nil {
						s.collectStablePrototypeLiteralFields(source, p, sc, bindings, recurse, seen, addField)
					}
				}
				fieldKey, ok := stablePrototypeFieldKey(target, sym)
				if !ok {
					continue
				}
				addField(fieldKey, s.stablePrototypeFieldType(assignmentSourceAt(sources, i), p, sc, bindings, recurse, seen))
			}
		}
		for _, def := range evidence.FunctionDefinitions {
			p := def.Nested.Point
			info := def.FuncDef
			fieldKey, ok := stablePrototypeFuncDefFieldKey(info, sym)
			if !ok {
				continue
			}
			addField(fieldKey, s.stablePrototypeFuncDefType(info, p, sc, bindings, recurse))
		}
	}
	return fields
}

func assignmentSourceAt(sources []ast.Expr, i int) ast.Expr {
	if i < 0 || i >= len(sources) {
		return nil
	}
	return sources[i]
}

func (s *Synthesizer) collectStablePrototypeLiteralFields(
	source ast.Expr,
	p compcfg.Point,
	sc *scope.State,
	bindings *bind.BindingTable,
	recurse ExprSynth,
	seen map[compcfg.SymbolID]bool,
	addField func(fieldkey.Key, typ.Type),
) {
	table, ok := source.(*ast.TableExpr)
	if !ok || table == nil || addField == nil {
		return
	}
	for _, field := range table.Fields {
		key, ok := fieldkey.FromTableField(field)
		if !ok {
			continue
		}
		addField(key, s.stablePrototypeFieldType(field.Value, p, sc, bindings, recurse, seen))
	}
}

func stablePrototypeFieldKey(target compcfg.AssignTarget, sym compcfg.SymbolID) (fieldkey.Key, bool) {
	if target.BaseSymbol != sym {
		return fieldkey.Key{}, false
	}
	switch target.Kind {
	case compcfg.TargetField:
		if len(target.FieldPath) == 1 {
			return fieldkey.FromName(target.FieldPath[0])
		}
	case compcfg.TargetIndex:
		return stablePrototypeIndexKey(target.Key)
	}
	return fieldkey.Key{}, false
}

func stablePrototypeIndexKey(key ast.Expr) (fieldkey.Key, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		return fieldkey.FromSegment(constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value})
	case *ast.NumberExpr:
		if idx, ok := pathkey.ParseIntLiteral(k.Value); ok {
			return fieldkey.FromSegment(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
		}
	}
	return fieldkey.Key{}, false
}

func stablePrototypeStaticMemberKey(member typ.StaticMember) (fieldkey.Key, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return fieldkey.FromSegment(constraint.Segment{Kind: constraint.SegmentIndexString, Name: member.Name})
	case typ.StaticMemberIntIndex:
		return fieldkey.FromSegment(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(member.Index)})
	default:
		return fieldkey.Key{}, false
	}
}

func stablePrototypeFieldKeyValid(key fieldkey.Key) bool {
	switch key.Kind {
	case constraint.SegmentField:
		return key.Name != ""
	case constraint.SegmentIndexString, constraint.SegmentIndexInt:
		return true
	default:
		return false
	}
}

func addStablePrototypeField(builder *typ.RecordBuilder, key fieldkey.Key, t typ.Type) {
	switch key.Kind {
	case constraint.SegmentField:
		if key.Name != "" {
			builder.Field(key.Name, t)
		}
	case constraint.SegmentIndexString:
		builder.StaticStringIndex(key.Name, t)
	case constraint.SegmentIndexInt:
		builder.StaticIntIndex(int64(key.Index), t)
	}
}

func stablePrototypeFuncDefFieldKey(info *compcfg.FuncDefInfo, sym compcfg.SymbolID) (fieldkey.Key, bool) {
	if info == nil || info.ReceiverSymbol != sym || info.Name == "" {
		return fieldkey.Key{}, false
	}
	switch info.TargetKind {
	case compcfg.FuncDefField, compcfg.FuncDefMethod:
		return fieldkey.FromName(info.Name)
	default:
		return fieldkey.Key{}, false
	}
}

func (s *Synthesizer) stablePrototypeFuncDefType(info *compcfg.FuncDefInfo, p compcfg.Point, sc *scope.State, bindings *bind.BindingTable, recurse ExprSynth) typ.Type {
	if info == nil {
		return nil
	}
	var factType typ.Type
	if info.Symbol != 0 {
		factType = s.functionFactType(info.Symbol)
	}
	if info.TargetKind == compcfg.FuncDefMethod && info.FuncExpr != nil && s != nil {
		expected := typ.Func().Param("self", typ.Self).Build()
		if projected := s.activeRecursiveFunctionType(info.FuncExpr, sc, expected); projected != nil {
			return projected
		}
		if sourceType := s.SynthFunctionTypeWithExpected(info.FuncExpr, sc, expected); sourceType != nil {
			if sourceFn := unwrap.Function(sourceType); sourceFn != nil {
				if factFn := unwrap.Function(factType); factFn != nil && len(factFn.Returns) > 0 {
					if aligned := typjoin.WithReturns(sourceFn, factFn.Returns); aligned != nil {
						return aligned
					}
				}
			}
			return sourceType
		}
	}
	if factType != nil {
		return factType
	}
	return s.stablePrototypeFieldType(info.FuncExpr, p, sc, bindings, recurse, nil)
}

func (s *Synthesizer) stablePrototypeFieldType(source ast.Expr, p compcfg.Point, sc *scope.State, bindings *bind.BindingTable, recurse ExprSynth, seen map[compcfg.SymbolID]bool) typ.Type {
	if source == nil {
		return nil
	}
	if fn, ok := source.(*ast.FunctionExpr); ok && bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
			if t := s.functionFactType(sym); t != nil {
				return t
			}
		}
		if s != nil {
			expected := typ.Func().Param("self", typ.Self).Build()
			if projected := s.activeRecursiveFunctionType(fn, sc, expected); projected != nil {
				return projected
			}
			if t := s.SynthFunctionTypeWithExpected(fn, sc, expected); t != nil {
				return t
			}
		}
	}
	if recurse != nil {
		current := recurse(source)
		if _, ok := source.(*ast.IdentExpr); ok {
			return s.stablePrototypeTypeSeen(source, p, sc, current, recurse, seen)
		}
		return current
	}
	return nil
}

// activeBindings returns the binding table for the function under synthesis,
// preferring the graph's own table and falling back to module bindings. It lets
// intercepts confirm a builtin-shaped callee is the genuine unshadowed global.
func (s *Synthesizer) activeBindings() *bind.BindingTable {
	if s == nil || s.deps.CheckCtx == nil {
		return s.moduleBindings()
	}
	if graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok && graph != nil {
		if b := graph.Bindings(); b != nil {
			return b
		}
	}
	return s.moduleBindings()
}

func (s *Synthesizer) moduleBindings() *bind.BindingTable {
	if s == nil {
		return nil
	}
	return s.deps.ModuleBindings
}

// buildInterceptChain creates the intercept chain for call synthesis.
func (s *Synthesizer) buildInterceptChain(sc *scope.State) *intercept.Chain {
	builder := intercept.NewChainBuilder()

	if s.deps.Manifests != nil {
		builder.WithManifests(s.deps.Manifests)
	}

	if sc != nil {
		builder.WithVariadicResolver(sc)
	}

	return builder.Build()
}

// synthArgs synthesizes types for argument expressions.
func synthArgs(exprs []ast.Expr, recurse ExprSynth) []typ.Type {
	args := make([]typ.Type, len(exprs))
	for i, arg := range exprs {
		args[i] = recurse(arg)
	}
	return args
}

func (s *Synthesizer) synthArgsWithCallContext(
	exprs []ast.Expr,
	p cfg.Point,
	sc *scope.State,
	recurse ExprSynth,
	def ops.CallDef,
) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	callbackArg := func(arg ast.Expr) *ast.FunctionExpr {
		return functionExprForCallbackArg(s, arg)
	}
	initial, hasCallbackArg := callarg.InitialInferenceTypes(
		exprs,
		func(arg ast.Expr) typ.Type { return recurse(arg) },
		callbackArg,
	)
	if !hasCallbackArg {
		return initial
	}
	def.Args = initial
	inferred := ops.InferCall(s.deps.Ctx, def)

	args := make([]typ.Type, len(exprs))
	for i, arg := range exprs {
		if fn := callarg.FunctionLiteralArg(arg, callbackArg); fn != nil {
			if expectedFn := phasecore.ExpectedFunctionLiteralSignature(fn, inferred.ExpectedArgType(i)); expectedFn != nil {
				if t := s.SynthFunctionTypeWithExpected(fn, sc, expectedFn); t != nil {
					args[i] = callboundary.ProjectContextualFunctionArg(expectedFn, t)
					continue
				}
			}
			args[i] = initial[i]
			continue
		}
		args[i] = initial[i]
	}
	return args
}

// resolveTypeArgs resolves explicit type arguments.
func (s *Synthesizer) resolveTypeArgs(typeExprs []ast.TypeExpr, sc *scope.State) []typ.Type {
	if len(typeExprs) == 0 || sc == nil {
		return nil
	}
	typeArgs := make([]typ.Type, len(typeExprs))
	for i, ta := range typeExprs {
		typeArgs[i] = s.ResolveType(ta, sc)
	}
	return typeArgs
}

// resolveMethodCallee resolves the callee type for a method call.
func (s *Synthesizer) resolveMethodCallee(recvType typ.Type, method string) typ.Type {
	if recvType == nil {
		return nil
	}
	if mt, ok := s.Method(recvType, method); ok {
		return mt
	}
	return nil
}

func (s *Synthesizer) forceMethodReceiverAtPoint(p cfg.Point, ex *ast.FuncCallExpr) bool {
	bindings := s.deps.ModuleBindings
	if bindings == nil && s.deps.CheckCtx != nil {
		bindings = s.deps.CheckCtx.Bindings()
	}
	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		graph, _ = s.deps.CheckCtx.Graph().(*compcfg.Graph)
	}
	return callsite.ForceMethodReceiverAtPoint(bindings, graph, s.graphEvidence(graph), p, ex)
}

// Method looks up a method type on a receiver type.
func (s *Synthesizer) Method(t typ.Type, name string) (typ.Type, bool) {
	if s.deps.Types == nil {
		return nil, false
	}
	mt, ok := s.deps.Types.Method(s.deps.Ctx, t, name)
	if !ok || mt == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		mt = subst.Params(mt, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return mt, true
}

// Field looks up a field type on a receiver type.
func (s *Synthesizer) Field(t typ.Type, name string) (typ.Type, bool) {
	if s.deps.Types == nil {
		return nil, false
	}
	ft, ok := s.deps.Types.Field(s.deps.Ctx, t, name)
	if !ok || ft == nil {
		return nil, false
	}
	recv := unwrap.Optional(unwrap.Alias(t))
	if inst, ok := recv.(*typ.Instantiated); ok && inst.Generic != nil {
		ft = subst.Params(ft, inst.Generic.TypeParams, inst.TypeArgs)
	}
	return ft, true
}

// specReturnOverride computes spec-based return type override.
func (s *Synthesizer) specReturnOverride(fnType typ.Type, astArgs []ast.Expr, argTypes []typ.Type) typ.Type {
	mode := api.SynthModeDeclared
	if s.IsNarrowing() {
		mode = api.SynthModeFlow
	}

	override := &intercept.SpecReturnOverride{
		SynthMode: mode,
	}
	if result := override.Override(fnType, astArgs); result != nil {
		return result
	}

	fn := intercept.ResolveSpecFunction(fnType)
	if fn == nil {
		return nil
	}
	return transform.ApplySpecReturnCases(fn, argTypes)
}

// unwrapCallResult converts CallResult to a slice of types.
func unwrapCallResult(result ops.CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		return CopyTypes(result.Returns)
	}
	if tuple, ok := result.Type.(*typ.Tuple); ok {
		return CopyTypes(tuple.Elements)
	}
	return []typ.Type{result.Type}
}

// CopyTypes returns a copy of a type slice.
func CopyTypes(types []typ.Type) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	result := make([]typ.Type, len(types))
	copy(result, types)
	return result
}

// applyPostCallTransforms applies effect-based return type transforms.
func (s *Synthesizer) applyPostCallTransforms(calleeType typ.Type, args []typ.Type, returns []typ.Type, receiver typ.Type, isMethod bool, forceMethodReceiver bool) []typ.Type {
	return callreturn.ApplyEffectTransforms(callreturn.EffectTransformInput{
		Ctx:                 s.deps.Ctx,
		Query:               s.GetCallQuery(),
		Callee:              calleeType,
		Args:                args,
		Returns:             returns,
		Receiver:            receiver,
		IsMethod:            isMethod,
		ForceMethodReceiver: forceMethodReceiver,
	})
}

func (s *Synthesizer) applyEnvironmentReturnProjection(
	calleeType typ.Type,
	ex *ast.FuncCallExpr,
	ownerExpr ast.Expr,
	ownerType typ.Type,
	args []typ.Type,
	p cfg.Point,
	sc *scope.State,
	returns []typ.Type,
) []typ.Type {
	if s == nil || ex == nil || ownerExpr == nil || len(returns) == 0 {
		return returns
	}
	ownerPath := s.environmentOwnerPath(p, ownerExpr, sc)
	return functionfact.ProjectEnvironmentReturns(calleeType, returns, args, func(spec contract.EnvReturnSpec) []typ.Type {
		return s.evaluateEnvironmentReturn(spec, ownerType, ownerPath, p)
	})
}

func environmentOwnerExpr(ex *ast.FuncCallExpr) ast.Expr {
	if ex == nil {
		return nil
	}
	if ex.Receiver != nil {
		return ex.Receiver
	}
	if attr, ok := ex.Func.(*ast.AttrGetExpr); ok {
		return attr.Object
	}
	return nil
}

func (s *Synthesizer) environmentOwnerPath(p cfg.Point, owner ast.Expr, sc *scope.State) constraint.Path {
	if s == nil || owner == nil {
		return constraint.Path{}
	}
	if s.deps.Paths != nil {
		return s.deps.Paths(p, owner, sc)
	}
	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		graph, _ = s.deps.CheckCtx.Graph().(*compcfg.Graph)
	}
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	return flowpath.FromExprWithBindingsAt(owner, nil, bindings, graph, p)
}

func (s *Synthesizer) evaluateEnvironmentReturn(
	spec contract.EnvReturnSpec,
	ownerType typ.Type,
	ownerPath constraint.Path,
	p cfg.Point,
) []typ.Type {
	if returns := s.evaluateEnvironmentReturnSource(spec, ownerPath, p); len(returns) > 0 {
		return returns
	}
	targetType := s.environmentPathType(ownerType, ownerPath, spec.Path, p)
	if targetType == nil {
		return nil
	}

	def := ops.CallDef{
		Args:  functionfact.CopyReturnVector(spec.Args),
		Query: s.GetCallQuery(),
	}
	callType := targetType
	if spec.Method != "" {
		def.IsMethod = true
		def.Receiver = targetType
		def.MethodName = spec.Method
		callType = s.resolveMethodCallee(targetType, spec.Method)
	} else {
		def.Callee = targetType
	}
	result := ops.NewCallPipeline(s.deps.Ctx, def, len(def.Args)).Run()
	returns := unwrapCallResult(result)
	return s.applyPostCallTransforms(callType, def.Args, returns, targetType, spec.Method != "", false)
}

func (s *Synthesizer) evaluateEnvironmentReturnSource(spec contract.EnvReturnSpec, ownerPath constraint.Path, p cfg.Point) []typ.Type {
	if s == nil || ownerPath.Symbol == 0 || spec.Method != "" || len(spec.Path) == 0 {
		return nil
	}
	source, point := s.environmentPathSource(ownerPath.Symbol, spec.Path, p)
	fn, ok := source.(*ast.FunctionExpr)
	if !ok || fn == nil {
		return nil
	}
	expected := expectedEnvironmentFunction(spec.Args)
	fnType := s.synthFunctionTypeWithCapturePoint(fn, s.deps.ScopeAt(point), expected, p, nil)
	if fnType == nil {
		return nil
	}
	def := ops.CallDef{
		Callee: fnType,
		Args:   functionfact.CopyReturnVector(spec.Args),
		Query:  s.GetCallQuery(),
	}
	result := ops.NewCallPipeline(s.deps.Ctx, def, len(def.Args)).Run()
	returns := unwrapCallResult(result)
	return s.applyPostCallTransforms(fnType, def.Args, returns, nil, false, false)
}

func expectedEnvironmentFunction(args []typ.Type) *typ.Function {
	builder := typ.Func().ReserveParams(len(args))
	for _, arg := range args {
		if arg == nil {
			arg = typ.Unknown
		}
		builder = builder.Param("", arg)
	}
	return builder.Build()
}

func (s *Synthesizer) environmentPathSource(ownerSym cfg.SymbolID, envPath []constraint.Segment, at cfg.Point) (ast.Expr, cfg.Point) {
	graph := s.currentGraph()
	if graph == nil || ownerSym == 0 || len(envPath) == 0 || len(s.deps.Evidence.Assignments) == 0 {
		return nil, 0
	}
	dom := cfganalysis.ImmediateDominatorsFor(s.deps.Ctx, graph.CFG())
	var best ast.Expr
	var bestPoint cfg.Point
	var bestLen int
	for _, evidence := range s.deps.Evidence.Assignments {
		if evidence.Info == nil || evidence.Point == at || !dom.StrictlyDominates(evidence.Point, at) {
			continue
		}
		for i, target := range evidence.Info.Targets {
			targetSegments, ok := assignmentTargetSegments(target, ownerSym)
			if !ok || !segmentsPrefix(targetSegments, envPath) {
				continue
			}
			source := nestedEnvironmentSource(evidence.Info.SourceAt(i), envPath[len(targetSegments):])
			if source == nil {
				continue
			}
			if best == nil || len(targetSegments) > bestLen || dom.StrictlyDominates(bestPoint, evidence.Point) {
				best = source
				bestPoint = evidence.Point
				bestLen = len(targetSegments)
			}
		}
	}
	return best, bestPoint
}

func (s *Synthesizer) currentGraph() *compcfg.Graph {
	if s == nil || s.deps.CheckCtx == nil {
		return nil
	}
	graph, _ := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	return graph
}

func assignmentTargetSegments(target compcfg.AssignTarget, ownerSym cfg.SymbolID) ([]constraint.Segment, bool) {
	if target.BaseSymbol != ownerSym {
		return nil, false
	}
	switch target.Kind {
	case compcfg.TargetField:
		if len(target.FieldPath) == 0 {
			return nil, false
		}
		segments := make([]constraint.Segment, len(target.FieldPath))
		for i, field := range target.FieldPath {
			if field == "" {
				return nil, false
			}
			segments[i] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
		}
		return segments, true
	case compcfg.TargetIndex:
		switch key := target.Key.(type) {
		case *ast.StringExpr:
			return []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: key.Value}}, true
		case *ast.NumberExpr:
			return []constraint.Segment{{Kind: constraint.SegmentIndexInt}}, true
		}
	}
	return nil, false
}

func segmentsPrefix(prefix, full []constraint.Segment) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if prefix[i] != full[i] {
			return false
		}
	}
	return true
}

func nestedEnvironmentSource(source ast.Expr, segments []constraint.Segment) ast.Expr {
	if len(segments) == 0 || source == nil {
		return source
	}
	table, ok := source.(*ast.TableExpr)
	if !ok {
		return nil
	}
	field := tableFieldValue(table, segments[0])
	return nestedEnvironmentSource(field, segments[1:])
}

func tableFieldValue(table *ast.TableExpr, segment constraint.Segment) ast.Expr {
	if table == nil {
		return nil
	}
	for _, field := range table.Fields {
		if field == nil || !flowpath.TableFieldMatchesSegment(field, segment) {
			continue
		}
		return field.Value
	}
	return nil
}

func (s *Synthesizer) environmentPathType(ownerType typ.Type, ownerPath constraint.Path, segments []constraint.Segment, p cfg.Point) typ.Type {
	if s == nil {
		return nil
	}
	path := appendEnvironmentSegments(ownerPath, segments)
	if s.deps.Flow != nil && !path.IsEmpty() {
		if narrowed := s.deps.Flow.NarrowedTypeAt(p, path); narrowed != nil {
			return narrowed
		}
	}
	t := ownerType
	for _, segment := range segments {
		t = s.environmentSegmentType(t, segment)
		if t == nil {
			return nil
		}
	}
	return t
}

func appendEnvironmentSegments(path constraint.Path, segments []constraint.Segment) constraint.Path {
	if path.IsEmpty() || len(segments) == 0 {
		return path
	}
	out := path
	for _, segment := range segments {
		out = out.Append(segment)
	}
	return out
}

func (s *Synthesizer) environmentSegmentType(base typ.Type, segment constraint.Segment) typ.Type {
	if base == nil {
		return nil
	}
	switch segment.Kind {
	case constraint.SegmentField:
		if t, ok := s.Field(base, segment.Name); ok {
			return t
		}
		return s.indexType(base, typ.LiteralString(segment.Name))
	case constraint.SegmentIndexString:
		if t := s.indexType(base, typ.LiteralString(segment.Name)); t != nil {
			return t
		}
		if t, ok := s.Field(base, segment.Name); ok {
			return t
		}
	case constraint.SegmentIndexInt:
		return s.indexType(base, typ.LiteralInt(int64(segment.Index)))
	}
	return nil
}

func (s *Synthesizer) indexType(base typ.Type, key typ.Type) typ.Type {
	if s == nil || s.deps.Types == nil || key == nil {
		return nil
	}
	t, ok := s.deps.Types.Index(s.deps.Ctx, base, key)
	if !ok {
		return nil
	}
	return t
}

// contextualArgReSynth creates the canonical call-site re-synthesizer.
//
// Calls are checked in two phases: first infer the callee and expected
// parameter types, then re-synthesize arguments that are sensitive to expected
// type context. Callback function literals additionally need spec-provided
// environment overlays, so they are handled before the general argument path.
func (s *Synthesizer) contextualArgReSynth(calleeType typ.Type, args []ast.Expr, sc *scope.State, p cfg.Point) ops.ArgReSynth {
	callbacks := s.callbackAwareReSynth(calleeType, sc)
	values := callarg.Full(
		func(arg ast.Expr, pt cfg.Point, expected typ.Type) typ.Type {
			return s.TypeOfWithExpected(arg, pt, expected)
		},
		nil,
		p,
	)
	return callarg.ForArgs(args, func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		if callbacks != nil {
			if t := callbacks(idx, arg, expected); t != nil {
				return t
			}
		}
		return values(idx, arg, expected)
	})
}

// callbackAwareReSynth creates an ArgReSynth that applies EnvOverlay from callback specs.
// For callback parameters with an EnvOverlay, the overlay globals are merged into the
// synthesizer's context so they are visible inside the callback body only.
func (s *Synthesizer) callbackAwareReSynth(calleeType typ.Type, sc *scope.State) callarg.ReSynth {
	return func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		fnExpr := functionExprForCallbackArg(s, arg)
		if fnExpr == nil {
			return nil
		}
		expectedFn := phasecore.ExpectedFunctionLiteralSignature(fnExpr, expected)
		if expectedFn == nil {
			return nil
		}

		synthFn := s.SynthFunctionTypeWithExpected
		if overlay := callbackEnvOverlay(calleeType, idx); len(overlay) > 0 {
			synthFn = s.withEnvOverlay(overlay).SynthFunctionTypeWithExpected
		}
		ft := synthFn(fnExpr, sc, expectedFn)
		if ft == nil {
			return nil
		}
		return ft
	}
}

func functionExprForCallbackArg(s *Synthesizer, arg ast.Expr) *ast.FunctionExpr {
	if fnExpr, ok := arg.(*ast.FunctionExpr); ok {
		return fnExpr
	}
	ident, ok := arg.(*ast.IdentExpr)
	if !ok || s == nil {
		return nil
	}
	return s.functionLiteralForIdent(ident)
}

// callbackEnvOverlay extracts the callback environment overlay for a callback at
// the given parameter index. The raw contract map is normalized immediately into
// the callbackenv domain carrier.
func callbackEnvOverlay(calleeType typ.Type, paramIdx int) callbackenv.Overlay {
	fn := intercept.ResolveSpecFunction(calleeType)
	if fn == nil || fn.Spec == nil {
		return nil
	}
	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return nil
	}
	cb := spec.GetCallback(paramIdx)
	if cb == nil || len(cb.EnvOverlay) == 0 {
		return nil
	}
	return callbackenv.OverlayFromContractMap(cb.EnvOverlay)
}

// withEnvOverlay returns a new Synthesizer with additional globals merged into
// the context through the normalized source-global overlay carrier.
func (s *Synthesizer) withEnvOverlay(overlay callbackenv.Overlay) *Synthesizer {
	overlaidCtx := s.deps.CheckCtx
	if overlaidCtx != nil {
		overlaidCtx = overlaidCtx.WithGlobalTypeOverlay(overlay)
	}
	overlaidDeps := &Deps{
		Ctx:               s.deps.Ctx,
		Types:             s.deps.Types,
		Scopes:            s.deps.Scopes,
		Manifests:         s.deps.Manifests,
		CheckCtx:          overlaidCtx,
		FunctionFacts:     s.deps.FunctionFacts,
		Graphs:            s.deps.Graphs,
		Flow:              s.deps.Flow,
		Paths:             s.deps.Paths,
		ModuleBindings:    s.deps.ModuleBindings,
		ModuleAliases:     s.deps.ModuleAliases,
		RecursiveFamilies: s.deps.RecursiveFamilies,
	}
	return NewSynthesizer(overlaidDeps, s.mode)
}
