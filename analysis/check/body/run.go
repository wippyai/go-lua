package body

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func functionVarargValueProvider(reg *axis.Registry, fn *ast.FunctionExpr, bindings *bind.Result) sourcevalue.VarargValueProvider {
	if reg == nil || fn == nil || bindings == nil {
		return nil
	}
	vararg, ok := bindings.VarargSymbol(fn)
	if !ok || vararg == 0 {
		return nil
	}
	slot := statekey.SymbolValue(vararg)
	if slot == 0 {
		return nil
	}
	return func(_ cfg.Point, _ factflow.ValueSource, in state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
		value := in.ReadValue(reg, slot)
		if product.Equal(reg, value, product.Bottom(reg)) {
			return product.Value{}, false
		}
		return value, true
	}
}

func (c *checker) prepareChunk(stmts []ast.Stmt) (*Static, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundChunk(stmts, bindings)
}

func (c *checker) prepareBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Static, error) {
	return c.prepareBound(bindings, "chunk",
		stmts,
		func() { c.config.Stats.StaticChunkPrepares++ },
		func() *cfgbuild.Result { return cfgbuild.BuildChunk(stmts, bindings) },
		nil,
		func() moduleidentity.Projection {
			return moduleidentity.NewRequireAliases(bindings, stmts, nil)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerWithResolverAndOptions("chunk", stmts, bindings, built, resolver, wirlower.Options{
				MethodReceiverTypes: c.config.MethodReceiverTypes,
			})
		},
	)
}

// prepareBound builds the CFG and prepares static state for a chunk or function.
// incStat bumps the matching prepare counter, build builds the CFG, and fn is
// the function identity for function bodies.
func (c *checker) prepareBound(
	bindings *bind.Result,
	_ string,
	sourceStmts []ast.Stmt,
	incStat func(),
	build func() *cfgbuild.Result,
	fn *ast.FunctionExpr,
	requireAliases func() moduleidentity.Projection,
	lowerWIR func(*cfgbuild.Result, *typeresolve.Resolver) *wir.Body,
) (*Static, error) {
	if c.config.Stats != nil {
		incStat()
	}
	built := build()
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	moduleTypes := newRequireAliasTypeResolver(requireAliases(), c.config.ModuleTypes)
	typeResolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	wirBody := lowerWIR(built, typeResolver)
	return c.prepare(bindings, built, fn, wirBody, typeResolver, sourceStmts), nil
}

func (c *checker) prepareFunction(fn *ast.FunctionExpr) (*Static, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundFunction(fn, bindings)
}

func functionSourceStmts(fn *ast.FunctionExpr) []ast.Stmt {
	if fn == nil {
		return nil
	}
	return fn.Stmts
}

func (c *checker) prepareBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Static, error) {
	return c.prepareBound(bindings, "function",
		functionSourceStmts(fn),
		func() { c.config.Stats.StaticFunctionPrepares++ },
		func() *cfgbuild.Result { return cfgbuild.BuildFunction(fn, bindings) },
		fn,
		func() moduleidentity.Projection {
			return moduleidentity.NewRequireAliases(bindings, fn.Stmts, fn)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerFunctionWithResolverAndOptions("function", fn, bindings, built, resolver, wirlower.Options{
				MethodReceiverTypes: c.config.MethodReceiverTypes,
			})
		},
	)
}

func (c *checker) prepare(
	bindings *bind.Result,
	built *cfgbuild.Result,
	fn *ast.FunctionExpr,
	wirBody *wir.Body,
	typeResolver *typeresolve.Resolver,
	sourceStmts []ast.Stmt,
) *Static {
	config := c.config
	globals := configGlobals(config)
	modules := moduleidentity.NewFromWIR(bindings, built.Graph, wirBody, fn)
	signatureID := newSignatureIdentityResolver(bindings, built.Graph, wirBody, modules, config.Signatures)
	signatureNameForCall := signatureID.nameForCall
	noNormalReturnCall := effectlowering.SignatureNoNormalReturnPredicate(effectlowering.SignatureNoNormalReturnConfig{
		Graph:      built.Graph,
		Registry:   config.Registry,
		Signatures: config.Signatures,
		NameFor:    signatureNameForCall,
	})
	lowered := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{
		Registry:           config.Registry,
		TypeResolver:       typeResolver,
		TypeValues:         config.TypeValues,
		ModuleExports:      config.ModuleExports,
		WIR:                wirBody,
		NoNormalReturnCall: noNormalReturnCall,
	})
	facts := lowered.Facts
	assignments := assignmentFactsFromSource(bindings, built, sourceStmts)
	declarations := declarationFactsFromSource(bindings, built, sourceStmts)
	genericFors := genericForFactsFromSource(bindings, built, sourceStmts)
	genericForOperations := compileGenericForOperations(genericFors, typeResolver, func(expr ast.Expr) (pathdom.Path, bool) {
		return pathexpr.Resolve(expr, bindings)
	})
	operationPlan := lowered.Plan.WithExtensions(genericForOperationExtensions(genericForOperations))
	resolver := config.Visibility
	if resolver == nil {
		resolver = defaultVisibilityResolver(bindings, built, wirBody, genericFors)
	}
	userExpressionValue := config.ExpressionValue
	expressionValue := userExpressionValue
	var readExpressionConfig *readexpr.Config
	if expressionValue == nil {
		defaultConfig := readexpr.Config{
			Registry:        config.Registry,
			Facts:           facts,
			Visibility:      resolver,
			TypeValues:      config.TypeValues,
			ProofVisibility: resolver,
		}
		readExpressionConfig = &defaultConfig
		expressionValue = readexpr.Provider(defaultConfig)
	}
	expressionValues := config.ExpressionValues
	var expressionPaths map[factflow.ExprRef]struct{}
	if userExpressionValue == nil {
		expressionValues = expressionValuesFromFacts(facts, config.ExpressionValues)
		expressionPaths = expressionPathRefsFromFacts(facts)
	}
	varargValue := config.VarargValue
	if varargValue == nil {
		varargValue = functionVarargValueProvider(config.Registry, fn, bindings)
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:          config.Registry,
		TypeValues:        config.TypeValues,
		KeySpace:          resolver.KeySpace(),
		Visibility:        resolver,
		ProjectPathValue:  luaProjectValue(config.Registry, config.TypeValues),
		ExpressionValues:  expressionValues,
		ExpressionPaths:   expressionPaths,
		ObjectLiteralView: facts.ObjectLiteralView,
		ObjectLiteralFromView: func(lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (product.Value, bool) {
			return luasourcevalue.ObjectLiteralValueFromViewCached(config.Registry, config.TypeValues, lit, resolver)
		},
		ExpressionOps:        expressionOperationsFromFacts(facts),
		ExpressionConditions: expressionConditionsFromFacts(facts),
		DynamicIndexExprs:    dynamicIndexExpressionsFromFacts(facts),
		ExpressionOp: func(op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			return luasourcevalue.ExpressionOperationValue(config.Registry, config.TypeValues, op, left, right)
		},
		ExpressionCondition: func(point cfg.Point, in state.State, selected factflow.ExpressionConditionFacts) state.State {
			return factapply.ApplyExpressionConditionFacts(config.Registry, resolver, luaPathTypeProjector, point, in, selected)
		},
		StaticScalarKey:       luaStaticScalarKeySegment(config.Registry, config.TypeValues),
		ExpressionValue:       expressionValue,
		PreferExpressionValue: userExpressionValue != nil,
		VarargValue:           varargValue,
	})
	expressionRefinements := sourcevalue.NewExpressionRefinementsFromReader(facts)
	refinedSources := expressionRefinements.Bind(config.Registry, sources)
	calleeValue := calleeValueProvider(config.Registry, facts, resolver, refinedSources, config.TypeValues, bindings, typeResolver)
	receiverFn := declaredReceiverCallableProvider(facts, bindings, typeResolver)
	callOutcomeSupplement := preparedCallOutcomeSupplement(config.Registry, config.ModuleExports, signatureID, facts, resolver, refinedSources, config.TypeValues, calleeValue)
	entrySeeds := entrySeedPlan(config.Registry, config.TypeValues, bindings, fn, globals, config.GlobalTypes, config.ModuleExports, typeResolver)
	wtoPlan := compileWTOPlan(built.Graph, config.Schedule)
	var concretePlan *concreteflow.Plan
	if wtoPlan != nil {
		// Certification is intentionally fail-closed. Irreducible bodies and any
		// future operation-plan shape the dense executor does not understand keep
		// the existing generic WTO without making body preparation fail.
		concretePlan, _ = concreteflow.Compile(built.Graph, operationPlan, wtoPlan)
	}
	return &Static{
		registry:              config.Registry,
		bindings:              bindings,
		cfg:                   built,
		function:              fn,
		wir:                   wirBody,
		sourceStmts:           append([]ast.Stmt(nil), sourceStmts...),
		signatures:            config.Signatures,
		moduleTypes:           config.ModuleTypes,
		moduleLoads:           config.ModuleExports,
		globals:               globals,
		globalTypes:           config.GlobalTypes,
		modules:               modules,
		signatureID:           signatureID,
		facts:                 facts,
		operationPlan:         operationPlan,
		symbolTypes:           lowered.SymbolTypes,
		assignments:           assignments,
		declarations:          declarations,
		genericFors:           genericFors,
		genericForOperations:  genericForOperations,
		visibility:            resolver,
		sources:               sources,
		readExpressionConfig:  readExpressionConfig,
		customExpressionValue: userExpressionValue != nil,
		calleeValue:           calleeValue,
		receiverFn:            receiverFn,
		typeNS:                typeResolver,
		typeValues:            config.TypeValues,
		entrySeeds:            entrySeeds,
		entrySeedsPrepared:    true,
		callOutcomeSupplement: callOutcomeSupplement,
		signatureReturnOps:    signatureReturnTypeOps(),
		wtoPlan:               wtoPlan,
		concreteFlow:          concretePlan,
	}
}

func (s *Static) Solve(config SolveConfig) *Result {
	result, err := s.solve(config)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// solve is the error-returning counterpart of Solve used by the public
// prepared-body APIs. Keeping Solve preserves the existing convenience method
// while SolvePrepared can propagate cooperative cancellation to its caller.
func (s *Static) solve(config SolveConfig) (*Result, error) {
	return s.solveWithFlow(config, nil)
}

type bodyFlowTransaction struct {
	flow      transfer.Result
	abortFn   func()
	committed bool
}

func (t *bodyFlowTransaction) commit() {
	if t == nil || t.committed {
		return
	}
	t.committed = true
}

func (t *bodyFlowTransaction) abort() {
	if t == nil || t.committed {
		return
	}
	if t.abortFn != nil {
		t.abortFn()
	}
}

type bodyFlowRunner func(transfer.Config) (*bodyFlowTransaction, error)

func (s *Static) solveWithFlow(config SolveConfig, runFlow bodyFlowRunner) (*Result, error) {
	if s == nil {
		return nil, nil
	}
	var session *cancellation.Session
	config.Context, session = cancellation.Attach(config.Context)
	sources := sourcevalue.BindSession(s.sources, session)
	if s.readExpressionConfig != nil {
		readConfig := *s.readExpressionConfig
		readConfig.Context = &readexpr.Context{Cancel: session.Token()}
		sources = sourcevalue.WithExpressionValue(sources, readexpr.Provider(readConfig))
	}
	if config.Stats != nil {
		config.Stats.BodySolves++
	}
	typeValues := s.solveTypeValues(config)
	signatureArgumentType := s.signatureArgumentTypeProvider(config, typeValues)
	callOutcome := s.callOutcomeProvider(config, typeValues, signatureArgumentType)
	entryState, initial := s.solveEntryState(typeValues, config.EntryState, config.Initial)
	widenThresholds := wideningThresholdsFromWIR(s.wir)
	nodeTransfer := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts:                  s.facts,
		Sources:                sources,
		CallOutcome:            callOutcome,
		Visibility:             s.visibility,
		ProjectPath:            luaPathTypeProjector,
		CovariantWiden:         luaCovariantWiden,
		TypeValues:             typeValues,
		ClosedDynamicAllValues: config.ClosedDynamicAllValues,
	})
	nodeTransfer = genericForNodeTransfer(nodeTransfer, s.genericForOperations, s.facts, sources, s.symbolTypes, s.signatures, s.signatureID, typeValues, callOutcome, s.visibility.KeySpace(), s.visibility)
	edgeTransfer := factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{
		Facts:       s.facts,
		Sources:     sources,
		CallOutcome: callOutcome,
		Visibility:  s.visibility,
		ProjectPath: luaPathTypeProjector,
		TypeValues:  typeValues,
	})
	observationPlan := compileObservationPlan(s.cfg.Graph, s.facts, callOutcome != nil)
	observationCapture := newObservationCapture(observationPlan)
	var comparisonConcreteFlow *concreteflow.Plan
	if config.CompareWTO != nil {
		comparisonConcreteFlow = s.concreteFlow
	}
	transferConfig := transfer.Config{
		Context:  config.Context,
		Session:  session,
		Graph:    s.cfg.Graph,
		Registry: s.registry,
		Schedule: config.Schedule,
		WTOPlan:  s.wtoPlan,
		// The executor remains attached to Static for differential and benchmark
		// use, but production publication stays on generic WTO until a corpus gate
		// demonstrates an end-to-end win outside measurement noise.
		ConcreteFlow:                  comparisonConcreteFlow,
		CanonicalConcreteTransactions: comparisonConcreteFlow != nil,
		FuseConcreteIdentity:          comparisonConcreteFlow != nil,
		CompareWTO:                    config.CompareWTO,
		StateLanes:                    config.StateLanes,
		StateOptions:                  state.DomainOptions{WidenThresholds: widenThresholds},
		EntryState:                    entryState,
		Initial:                       initial,
		NodeTransfer:                  nodeTransfer,
		EdgeTransfer:                  edgeTransfer,
		WidenAt:                       config.WidenAt,
		WidenDelay:                    config.WidenDelay,
		Stats:                         transferStats(config.Stats),
		ObserveNode:                   observationPlan.observesNode,
		RecordNodeObservation:         observationCapture.record,
		FinalizeNodeObservations:      observationCapture.finalize,
		ResetNodeObservations:         observationCapture.reset,
		BeforePoint:                   config.BeforePoint,
		AfterPoint:                    config.AfterPoint,
		Resume:                        config.Resume,
		ResumePoints:                  config.ResumePoints,
	}
	var flow transfer.Result
	var flowTx *bodyFlowTransaction
	var err error
	if runFlow == nil {
		flow, err = transfer.TryRun(transferConfig)
	} else {
		flowTx, err = runFlow(transferConfig)
		if err == nil {
			flow = flowTx.flow
		}
	}
	if err != nil {
		return nil, err
	}
	result := &Result{
		registry:              s.registry,
		bindings:              s.bindings,
		cfg:                   s.cfg,
		function:              s.function,
		wir:                   s.wir,
		sourceStmts:           append([]ast.Stmt(nil), s.sourceStmts...),
		signatures:            s.signatures,
		moduleTypes:           s.moduleTypes,
		modules:               s.modules,
		signatureID:           s.signatureID,
		facts:                 s.facts,
		symbolTypes:           s.symbolTypes,
		assignments:           s.assignments,
		declarations:          s.declarations,
		genericFors:           s.genericFors,
		exprRefinements:       sourcevalue.NewExpressionRefinementsFromReader(s.facts),
		typeNS:                s.typeNS,
		flow:                  flow,
		boundaryXfer:          nodeTransfer,
		edgeXfer:              edgeTransfer,
		visibility:            s.visibility,
		sources:               s.sources,
		customExpressionValue: s.customExpressionValue,
		callOutcome:           callOutcome,
		calleeValue:           s.calleeValue,
		signatureArg:          signatureArgumentType,
		typeValues:            typeValues,
		stateLanes:            append([]state.LaneID(nil), config.StateLanes...),
		observationPlan:       observationPlan,
		capturedNodeOutputs:   observationCapture.valid,
		observation:           observationCapture.stats,
		queries:               newResultQueryCache(s.facts),
	}
	// Seal the immutable query surface after the solver, including narrowing,
	// has converged. This replaces lazy node/edge transfer replay in diagnostics
	// and readmodels with compact published facts.
	if err := result.sealObservationsContext(config.Context); err != nil {
		if flowTx != nil {
			flowTx.abort()
		}
		return nil, errors.Join(solve.ErrCanceled, err)
	}
	if config.Stats != nil {
		addObservationStats(&config.Stats.Observation, result.observation)
	}
	result.finalizeReturnSlotsFromBoundaryValues()
	resultVersion, err := computeResultVersion(s, config, entryState, initial)
	if err != nil {
		if flowTx != nil {
			flowTx.abort()
		}
		return nil, err
	}
	result.resultVersion = resultVersion
	if flowTx != nil {
		flowTx.commit()
	}
	return result, nil
}

func (r *Result) finalizeReturnSlotsFromBoundaryValues() {
	if r == nil || r.registry == nil || r.flow == nil {
		return
	}
	graph := r.Graph()
	if graph == nil {
		return
	}
	exit := graph.Exit()
	exitState, ok := r.flow[exit]
	if !ok {
		return
	}
	domain := product.Domain(r.registry)
	joined := map[int]product.Value{}
	seen := map[int]bool{}
	for _, point := range r.ReturnPoints() {
		if !r.PointReachable(point) {
			continue
		}
		sources, ok := r.ReturnValueSources(point)
		if !ok {
			continue
		}
		for i, source := range sources {
			index := source.TargetIndex
			if index < 0 {
				index = i
			}
			value, valueOK := r.SourceValueAtBoundary(point, source)
			if !valueOK || !returnSlotBoundaryValueAdmissible(r.registry, value) {
				value = product.Top()
			}
			if !seen[index] {
				joined[index] = value
				seen[index] = true
				continue
			}
			joined[index] = domain.Join(joined[index], value)
		}
	}
	for index, value := range joined {
		if product.Equal(r.registry, value, product.Bottom(r.registry)) {
			continue
		}
		exitState = exitState.WriteReturnSlot(r.registry, index, value)
	}
	r.flow[exit] = exitState
}

func returnSlotBoundaryValueAdmissible(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	claim := product.Get(reg, value, assertion.Key)
	if claim.Has(assertion.AnyClaim) && !claim.Has(assertion.RuntimeClaim) {
		t, ok := typevalue.TypeOf(reg, value)
		return ok && returnSlotStructuredType(t)
	}
	if claim.Has(assertion.TypeClaim) && !claim.Has(assertion.RuntimeClaim) {
		t, ok := typevalue.TypeOf(reg, value)
		return ok && returnSlotStructuredType(t)
	}
	return true
}

func returnSlotStructuredType(t typ.Type) bool {
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Tuple, *typ.Record, *typ.Function, *typ.Interface:
		return true
	case *typ.Optional:
		return returnSlotStructuredType(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if !returnSlotStructuredType(member) {
				return false
			}
		}
		return len(v.Members) != 0
	default:
		return false
	}
}

func (s *Static) solveEntryState(typeValues *typevalue.Cache, entry state.State, initial transfer.InitialState) (state.State, transfer.InitialState) {
	if s.entrySeedsPrepared {
		return applyEntrySeedPlan(s.registry, s.cfg.Graph, s.entrySeeds, entry, initial)
	}
	return parameterEntryState(
		s.registry,
		typeValues,
		s.cfg.Graph,
		s.bindings,
		s.function,
		s.globals,
		s.globalTypes,
		s.moduleLoads,
		s.typeNS,
		entry,
		initial,
	)
}

func transferStats(stats *Stats) *transfer.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Transfer
}

func addObservationStats(dst *ObservationStats, src ObservationStats) {
	if dst == nil {
		return
	}
	dst.PlannedNodeOutputs += src.PlannedNodeOutputs
	dst.CapturedNodeOutputs += src.CapturedNodeOutputs
	dst.ValidatedNodeOutputs += src.ValidatedNodeOutputs
	dst.RecomputedNodeOutputs += src.RecomputedNodeOutputs
	dst.PlannedBoundaryOutputs += src.PlannedBoundaryOutputs
	dst.PlannedEdgeReachability += src.PlannedEdgeReachability
	dst.CapturedBoundaryOutputs += src.CapturedBoundaryOutputs
	dst.ValidatedBoundaryOutputs += src.ValidatedBoundaryOutputs
	dst.RecomputedBoundaryOutputs += src.RecomputedBoundaryOutputs
	dst.ProjectedBoundaryOutputs += src.ProjectedBoundaryOutputs
	dst.ProjectedEdgeReachability += src.ProjectedEdgeReachability
}

func (s *Static) solveTypeValues(config SolveConfig) *typevalue.Cache {
	if s != nil && s.typeValues != nil {
		return s.typeValues
	}
	if config.TypeValues != nil {
		return config.TypeValues
	}
	return typevalue.NewCache()
}

func (s *Static) signatureArgumentTypeProvider(config SolveConfig, typeValues *typevalue.Cache) SignatureArgumentTypeFunc {
	signatureArgumentType := config.SignatureArgumentType
	if config.SignatureArgumentTypeFactory != nil {
		factoryArgumentType := config.SignatureArgumentTypeFactory(s.callOutcomeContext(typeValues))
		if factoryArgumentType != nil {
			baseArgumentType := signatureArgumentType
			signatureArgumentType = func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
				if t, ok := factoryArgumentType(ctx, source, in, read); ok {
					return t, true
				}
				if baseArgumentType != nil {
					return baseArgumentType(ctx, source, in, read)
				}
				return nil, false
			}
		}
	}
	return signatureArgumentType
}

func (s *Static) callOutcomeProvider(config SolveConfig, typeValues *typevalue.Cache, signatureArgumentType SignatureArgumentTypeFunc) callpayload.CallOutcomeProvider {
	var providers []callpayload.CallOutcomeProvider
	if config.CallOutcomeFactory != nil {
		providers = append(providers, config.CallOutcomeFactory(s.callOutcomeContext(typeValues)))
	}
	providers = append(providers, config.CallOutcome, s.callOutcomeSupplement)
	frontProviders := []callpayload.CallOutcomeProvider{
		effectlowering.AmbientTypestateEscapeOutcomeProvider(effectlowering.AmbientTypestateEscapeOutcomeProviderConfig{
			NameForSite: s.signatureID.nameForCallSiteView,
			Signatures:  s.signatures,
			Facts:       s.facts,
			KeySpace:    s.visibility.KeySpace(),
			Resolver:    s.visibility,
		}),
		effectlowering.AmbientChannelLifecycleOutcomeProvider(effectlowering.AmbientChannelLifecycleOutcomeProviderConfig{
			ReceiverType: channelMethodReceiverTypeProvider(s.registry, s.facts, s.visibility, s.sources, typeValues),
			NameForSite:  s.signatureID.nameForCallSiteView,
			KeySpace:     s.visibility.KeySpace(),
			Resolver:     s.visibility,
		}),
	}
	if hasSignatures(s.signatures) {
		signatureProvider := effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:    s.signatures,
			NameFor:       s.signatureID.nameForCall,
			NameForSite:   s.signatureID.nameForCallSiteView,
			ReturnTypeOps: s.signatureReturnOps,
			TypeValues:    typeValues,
			Facts:         s.facts,
			Sources:       s.sources,
			ArgumentType:  effectlowering.SignatureArgumentTypeFunc(signatureArgumentType),
			ReturnValue:   stdlibSignatureReturnValue(s.registry, typeValues, s.facts, s.sources, sourcevalue.NewExpressionRefinementsFromReader(s.facts), s.visibility),
			KeySpace:      s.visibility.KeySpace(),
		})
		frontProviders = append(frontProviders, signatureProvider)
	}
	providers = append(frontProviders, providers...)
	return calloutcome.ComposeSupplemental(providers...)
}

func (s *Static) callOutcomeContext(typeValues *typevalue.Cache) CallOutcomeContext {
	if s == nil {
		return CallOutcomeContext{}
	}
	if typeValues == nil {
		typeValues = s.typeValues
	}
	return CallOutcomeContext{
		Facts:   s.facts,
		Sources: s.sources,
		PathValue: func(ctx transfer.NodeContext, path pathdom.Path, in state.State) (product.Value, bool) {
			return readexpr.Project(readexpr.Config{
				Registry: s.registry, Facts: s.facts, Visibility: s.visibility, TypeValues: typeValues,
			}, ctx.Point, path, in)
		},
		ProtectedCall:               s.isProtectedCallSite,
		CalleeValue:                 s.calleeValue,
		ReceiverCallable:            s.receiverFn,
		ReturnPresenceRelationsPath: s.returnPresenceRelationsForPath,
		KeySpace:                    s.visibility.KeySpace(),
		TypeValues:                  typeValues,
	}
}

func (s *Static) isProtectedCallSite(ctx transfer.NodeContext, site factflow.CallSiteView) bool {
	if s == nil || s.signatureID == nil || site.MethodName() != "" {
		return false
	}
	name, ok := s.signatureID.nameForCallSiteView(ctx, site)
	return ok && (name == "pcall" || name == "xpcall")
}

func (s *Static) returnPresenceRelationsForPath(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation {
	if s == nil {
		return nil
	}
	var name string
	var ok bool
	if s.signatureID != nil {
		name, ok = s.signatureID.stableCalleeName(p.Symbol, p)
	}
	if !ok {
		name, ok = s.modules.SignatureName(point, p)
	}
	if !ok {
		return nil
	}
	sig, ok := s.signatures.Lookup(name)
	if !ok || sig.OperationalEffects == nil || len(sig.OperationalEffects.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]callpayload.CallReturnPresenceRelation, 0, len(sig.OperationalEffects.ReturnPresenceRelations))
	for _, relation := range sig.OperationalEffects.ReturnPresenceRelations {
		out = append(out, callpayload.CallReturnPresenceRelation{
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: relation.TriggerPresence,
			TargetIndex:     relation.TargetIndex,
			TargetPresence:  relation.TargetPresence,
		})
	}
	return out
}

func preparedCallOutcomeSupplement(
	reg *axis.Registry,
	moduleLoads importlookup.Source,
	signatureID *signatureIdentityResolver,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	typeValues *typevalue.Cache,
	calleeValue CalleeValueFunc,
) callpayload.CallOutcomeProvider {
	var providers []callpayload.CallOutcomeProvider
	expressionRefinements := sourcevalue.NewExpressionRefinementsFromReader(facts)
	if hasModuleExports(moduleLoads) {
		providers = append(providers, effectlowering.ModuleLoadOutcomeProvider(effectlowering.ModuleLoadOutcomeProviderConfig{
			Exports:               moduleLoads,
			NameFor:               signatureID.nameForCall,
			NameForSite:           signatureID.nameForCallSiteView,
			Sources:               sources,
			ExpressionRefinements: expressionRefinements,
			TypeValues:            typeValues,
		}))
	}
	providers = append(providers, effectlowering.AmbientChannelSendOutcomeProvider(effectlowering.AmbientChannelSendOutcomeProviderConfig{
		ReceiverType: channelMethodReceiverTypeProvider(reg, facts, resolver, sources, typeValues),
		KeySpace:     resolver.KeySpace(),
	}))
	providers = append(providers, effectlowering.CallableValueOutcomeProvider(effectlowering.CallableValueOutcomeProviderConfig{
		CalleeValue: effectlowering.CalleeValueFunc(calleeValue),
		Callable:    typecall.Callable,
		TypeValues:  typeValues,
	}))
	providers = append(providers, explicitAnyReceiverMethodOutcomeProvider(reg, sources, typeValues))
	return calloutcome.ComposeSupplemental(providers...)
}

func luaPathTypeProjector(root typ.Type, p pathdom.Path) (typ.Type, bool) {
	return luatypeprojection.ApplySegments(root, p.Segments)
}

func luaProjectSegments(root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	return luatypeprojection.ApplySegments(root, segments)
}

func luaProjectValue(reg *axis.Registry, typeValues *typevalue.Cache) func(product.Value, []segment.Segment) (product.Value, bool) {
	return func(root product.Value, segments []segment.Segment) (product.Value, bool) {
		if reg == nil || typeValues == nil {
			return product.Value{}, false
		}
		if origin := product.Get(reg, root, variantorigin.Key); !origin.IsBottom() && !origin.IsTop() {
			if rootType, ok := typeValues.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
				if value, ok := luaProjectValueFromType(reg, typeValues, rootType, segments); ok {
					if family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.CasesRef(), segments); ok {
						value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
					}
					return value, true
				}
			}
		}
		rootType, ok := typeValues.TypeOf(reg, root)
		if !ok || rootType == nil || typ.IsAny(rootType) || typ.IsUnknown(rootType) {
			return product.Value{}, false
		}
		return luaProjectValueFromType(reg, typeValues, rootType, segments)
	}
}

func luaStaticScalarKeySegment(reg *axis.Registry, typeValues *typevalue.Cache) sourcevalue.StaticScalarKeySegment {
	return func(value product.Value) (segment.Segment, bool) {
		if reg == nil || typeValues == nil {
			return segment.Segment{}, false
		}
		t, ok := typeValues.TypeOf(reg, value)
		if !ok {
			return segment.Segment{}, false
		}
		lit, ok := unwrap.Alias(t).(*typ.Literal)
		if !ok {
			return segment.Segment{}, false
		}
		switch v := lit.Value.(type) {
		case string:
			return segment.Segment{Kind: segment.SegmentIndexString, Name: v}, true
		case int64:
			if int64(int(v)) != v {
				return segment.Segment{}, false
			}
			return segment.Segment{Kind: segment.SegmentIndexInt, Index: int(v)}, true
		default:
			return segment.Segment{}, false
		}
	}
}

func luaProjectValueFromType(reg *axis.Registry, typeValues *typevalue.Cache, rootType typ.Type, segments []segment.Segment) (product.Value, bool) {
	projected, ok := luaProjectSegments(rootType, segments)
	if !ok || projected == nil {
		return product.Value{}, false
	}
	value := typevalue.WithWitness(reg, typeValues.FromType(reg, projected), projected)
	if !typevalue.ProjectionHasNil(projected) {
		value = product.WithPresence(reg, value, presence.Present())
	}
	return value, true
}
