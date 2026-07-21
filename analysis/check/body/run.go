package body

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
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
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
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
	sealedLuaTypeChecks := luaTypePredicateChecksSealedForLowering(bindings, c.config.Signatures, c.config.GlobalTypes)
	return c.prepareBound(bindings, "chunk",
		stmts,
		func() { c.config.Stats.StaticChunkPrepares++ },
		func() *cfgbuild.Result {
			return cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: sealedLuaTypeChecks})
		},
		nil,
		func() moduleidentity.Projection {
			return moduleidentity.NewRequireAliases(bindings, stmts, nil)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerWithResolverAndOptions("chunk", stmts, bindings, built, resolver, wirlower.Options{
				MethodReceiverTypes: c.config.MethodReceiverTypes,
				SealedLuaTypeChecks: sealedLuaTypeChecks,
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
	if bindings == nil {
		return nil, ErrBindingsRequired
	}
	if c.config.Stats != nil {
		incStat()
	}
	built := build()
	if built == nil || built.Graph == nil {
		return nil, ErrCFGRequired
	}
	moduleTypes := newRequireAliasTypeResolver(requireAliases(), c.config.ModuleTypes)
	typeResolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	wirBody := lowerWIR(built, typeResolver)
	return c.prepare(bindings, built, fn, wirBody, typeResolver, sourceStmts)
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
	sealedLuaTypeChecks := luaTypePredicateChecksSealedForLowering(bindings, c.config.Signatures, c.config.GlobalTypes)
	return c.prepareBound(bindings, "function",
		functionSourceStmts(fn),
		func() { c.config.Stats.StaticFunctionPrepares++ },
		func() *cfgbuild.Result {
			return cfgbuild.BuildFunctionWithOptions(fn, bindings, cfgbuild.Options{SealedLuaTypeChecks: sealedLuaTypeChecks})
		},
		fn,
		func() moduleidentity.Projection {
			return moduleidentity.NewRequireAliases(bindings, fn.Stmts, fn)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerFunctionWithResolverAndOptions("function", fn, bindings, built, resolver, wirlower.Options{
				MethodReceiverTypes: c.config.MethodReceiverTypes,
				SealedLuaTypeChecks: sealedLuaTypeChecks,
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
) (*Static, error) {
	config := c.config
	unitNamespace := config.UnitNamespace
	if unitNamespace == (lexicalidentity.UnitNamespace{}) {
		unitNamespace = standaloneLexicalUnitNamespace(bindings, built, wirBody)
	}
	functionSymbol, hasFunctionSymbol := bindings.FunctionSymbol(fn)
	lexicalBodyID := lexicalidentity.RootBody(unitNamespace)
	if fn != nil && hasFunctionSymbol {
		lexicalBodyID = lexicalidentity.FunctionBody(unitNamespace, uint64(functionSymbol))
	}
	tableLiteralSite := identity.TableLiteralSiteForBody(lexicalBodyID)
	globals := configGlobals(config)
	modules := moduleidentity.NewFromWIR(bindings, built.Graph, wirBody, fn)
	signatureID := newSignatureIdentityResolver(bindings, built.Graph, wirBody, modules, config.Signatures, config.GlobalTypes, config.ModuleExports)
	sealedLuaTypeChecks := signatureID.luaTypePredicateChecksSealed()
	signatureNameForCall := signatureID.nameForCall
	noNormalReturnCall := effectlowering.SignatureNoNormalReturnPredicate(effectlowering.SignatureNoNormalReturnConfig{
		Graph:      built.Graph,
		Registry:   config.Registry,
		Signatures: config.Signatures,
		NameFor:    signatureNameForCall,
	})
	lowered := transferfacts.LowerDetailed(built.Graph, transferfacts.Config{
		Registry:            config.Registry,
		LexicalBodyID:       lexicalBodyID,
		TableLiteralSite:    tableLiteralSite,
		TypeResolver:        typeResolver,
		TypeValues:          config.TypeValues,
		ModuleExports:       config.ModuleExports,
		WIR:                 wirBody,
		SealedLuaTypeChecks: sealedLuaTypeChecks,
		NoNormalReturnCall:  noNormalReturnCall,
	})
	facts := lowered.Facts
	assignments := assignmentFactsFromSource(bindings, built, sourceStmts)
	declarations := declarationFactsFromSource(bindings, built, sourceStmts)
	genericFors := genericForFactsFromSource(bindings, built, sourceStmts)
	resolver := config.Visibility
	if resolver == nil {
		resolver = defaultVisibilityResolver(bindings, built, wirBody, genericFors)
	}
	signatureProducer := effectlowering.PrepareSignatureProducer(effectlowering.SignatureOutcomeProviderConfig{
		Signatures: config.Signatures, NameForSite: signatureID.nameForCallSiteView,
		IntrinsicForSite: signatureID.intrinsicForCallSiteView,
	})
	directCaptures := bindings.DirectCaptures(fn)
	boundaryCaptures := symbolicBoundaryCaptureSymbols(wirBody, directCaptures, bindings)
	boundaryGlobals := bindings.DirectGlobalReads(fn)
	if fn == nil {
		boundaryGlobals = bindings.ChunkGlobalReads()
	}
	boundaryGlobalContracts := materializeBoundaryGlobalTypeValues(config.Registry, config.TypeValues, bindings, boundaryGlobals, config.GlobalTypes)
	boundaryGlobalPairs := make([]operationplan.BoundaryGlobal, len(boundaryGlobals))
	for index, global := range boundaryGlobals {
		boundaryGlobalPairs[index] = operationplan.BoundaryGlobal{Symbol: global, Contract: boundaryGlobalContracts[index]}
	}
	operationPlan := lowered.Plan.
		WithBoundaryParams(bindings.ParamSymbols(fn)).
		WithBoundaryCaptures(boundaryCaptures).
		WithBoundaryGlobals(boundaryGlobalPairs).
		WithBoundaryReturns(materializeDeclaredReturnTypeValues(config.Registry, config.TypeValues, typeResolver, fn))
	entrySeeds := entrySeedPlan(config.Registry, config.TypeValues, bindings, fn, globals, config.GlobalTypes, config.ModuleExports, typeResolver)
	entrySeeds = appendBoundaryGlobalContractEntrySeeds(entrySeeds, operationPlan)
	entrySeeds = applyMethodReceiverEntrySeed(config.Registry, config.TypeValues, bindings, fn, config.MethodReceiverTypes, entrySeeds)
	paramSlots := make([]statekey.Value, len(operationPlan.BoundaryParams()))
	for index, param := range operationPlan.BoundaryParams() {
		paramSlots[index] = statekey.SymbolValue(param)
	}
	paramContracts, ok := state.NewEntrySeedPlan(entrySeeds).ValuesForSlots(paramSlots)
	if !ok {
		return nil, errors.New("check: prepared parameter tuple has no finalized entry-seed authority")
	}
	operationPlan = operationPlan.WithBoundaryParamContracts(paramContracts)
	signatureCalls := signatureCallOperations(config.Registry, bindings, built.Graph, facts, operationPlan, signatureProducer)
	genericForOperations := compileGenericForOperations(genericFors, typeResolver, func(expr ast.Expr) (pathdom.Path, bool) {
		return pathexpr.Resolve(expr, bindings)
	}, signatureCalls, func(point cfg.Point, ref effect.ParamRef) (int, typ.Type, bool) {
		site, ok := facts.CallSiteView(point)
		if !ok {
			return 0, nil, false
		}
		index, ok := effect.ResolveParamIndex(ref, site.ArgumentSourceCount())
		if !ok {
			return 0, nil, false
		}
		source, ok := site.ArgumentSourceAt(index)
		if !ok {
			return 0, nil, false
		}
		declared, ok := genericForDeclaredPathIteratorSourceType(source, facts, resolver, lowered.SymbolTypes)
		return index, declared, ok
	})
	moduleLoadCalls := moduleLoadOperations(config.Registry, built.Graph, facts, signatureID, config.ModuleExports, config.TypeValues)
	attachMetatables := attachMetatableOperations(built.Graph, facts, signatureID)
	operationPlan = operationPlan.
		WithCallSurface(sealPreparedCallSurface(bindings, wirBody, facts, signatureCalls, moduleLoadCalls, lexicalBodyID, unitNamespace, built.Graph.Size())).
		WithSignatureCalls(signatureCalls).
		WithModuleLoads(moduleLoadCalls).
		WithAttachMetatables(attachMetatables)
	operationPlan = operationPlan.
		WithSignatureAllocations(signatureAllocationOperations(operationPlan, uint64(functionSymbol))).
		WithExtensions(genericForOperationExtensions(genericForOperations))
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
		ExpressionValue:       expressionValue,
		PreferExpressionValue: userExpressionValue != nil,
		VarargValue:           varargValue,
	})
	expressionRefinements := sourcevalue.NewExpressionRefinementsFromReader(facts)
	refinedSources := expressionRefinements.Bind(config.Registry, sources)
	rootDeclarations := preparedRootDeclarationQuery(facts, built.Graph)
	calleeValue := calleeValueProvider(config.Registry, facts, resolver, refinedSources, config.TypeValues, bindings, typeResolver, rootDeclarations)
	receiverFn := declaredReceiverCallableProvider(facts, bindings, typeResolver, rootDeclarations)
	callOutcomeSupplement := preparedCallOutcomeSupplement(config.Registry, config.ModuleExports, signatureID, facts, resolver, refinedSources, config.TypeValues, calleeValue)
	prepared := &Static{
		lexicalBodyID:                 lexicalBodyID,
		tableLiteralSite:              tableLiteralSite,
		registry:                      config.Registry,
		bindings:                      bindings,
		cfg:                           built,
		function:                      fn,
		wir:                           wirBody,
		sourceStmts:                   append([]ast.Stmt(nil), sourceStmts...),
		signatures:                    config.Signatures,
		moduleTypes:                   config.ModuleTypes,
		moduleLoads:                   config.ModuleExports,
		globals:                       globals,
		globalTypes:                   config.GlobalTypes,
		modules:                       modules,
		signatureID:                   signatureID,
		sealedLuaTypeChecks:           sealedLuaTypeChecks,
		facts:                         facts,
		operationPlan:                 operationPlan,
		symbolTypes:                   lowered.SymbolTypes,
		assignments:                   assignments,
		declarations:                  declarations,
		genericFors:                   genericFors,
		visibility:                    resolver,
		sources:                       sources,
		readExpressionConfig:          readExpressionConfig,
		customExpressionValue:         userExpressionValue != nil,
		customExpressionValueProvider: userExpressionValue,
		calleeValue:                   calleeValue,
		receiverFn:                    receiverFn,
		typeNS:                        typeResolver,
		typeValues:                    config.TypeValues,
		entrySeeds:                    entrySeeds,
		entrySeedsPrepared:            true,
		callOutcomeSupplement:         callOutcomeSupplement,
		signatureReturnOps:            signatureReturnTypeOps(),
	}
	prepared.readGraph = compileReadGraph(prepared)
	return prepared, nil
}

// symbolicBoundaryCaptureSymbols retains every capture that participates in
// the function's value relation. WIR value paths intentionally omit callee-only
// reads, so such a capture may be erased only when the binder proves it is one
// stable lexical function whose complete use set consists of direct calls.
// Parameters, ambient captures, and other dynamic callables remain ordinary
// capture roots even when their sole syntax use is as a callee.
func symbolicBoundaryCaptureSymbols(body *wir.Body, captures []bind.Capture, bindings *bind.Result) []symbol.ID {
	if body == nil || len(captures) == 0 {
		return nil
	}
	used := make(map[symbol.ID]bool, len(captures))
	isCapture := make(map[symbol.ID]bool, len(captures))
	for _, capture := range captures {
		if capture.Captured != 0 {
			isCapture[capture.Captured] = true
		}
	}
	observe := func(p pathdom.Path) bool {
		if isCapture[p.Symbol] {
			used[p.Symbol] = true
		}
		return true
	}
	body.ForEachValuePath(observe)
	out := make([]symbol.ID, 0, len(captures))
	for _, capture := range captures {
		_, discharged := bindings.StableDirectCallFunctionIdentity(capture.Captured)
		if used[capture.Captured] || !discharged {
			out = append(out, capture.Captured)
		}
	}
	return out
}

// newResult binds immutable body/query ownership to one execution factory.
// Stabilized relation publication retains no point transfers or provider-backed
// fallback semantics.
func (f *ExecutionFactory) newResult(flow transfer.Result, observationPlan ObservationPlan) *Result {
	if f == nil || f.prepared == nil {
		return nil
	}
	s := f.prepared
	return &Result{
		lexicalBodyID:         s.lexicalBodyID,
		tableLiteralSite:      s.tableLiteralSite,
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
		sealedLuaTypeChecks:   s.sealedLuaTypeChecks,
		facts:                 s.facts,
		operationPlan:         s.operationPlan,
		symbolTypes:           s.symbolTypes,
		assignments:           s.assignments,
		declarations:          s.declarations,
		genericFors:           s.genericFors,
		exprRefinements:       sourcevalue.NewExpressionRefinementsFromReader(s.facts),
		typeNS:                s.typeNS,
		flow:                  flow,
		visibility:            s.visibility,
		sources:               s.sources,
		customExpressionValue: s.customExpressionValue,
		calleeValue:           s.calleeValue,
		signatureArg:          f.signatureArgumentType,
		typeValues:            f.typeValues,
		stateLanes:            f.StateLanes(),
		observationPlan:       observationPlan,
		queries:               newResultQueryCache(),
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

func addObservationStats(dst *ObservationStats, src ObservationStats) {
	if dst == nil {
		return
	}
	dst.PlannedNodeOutputs += src.PlannedNodeOutputs
	dst.PlannedBoundaryOutputs += src.PlannedBoundaryOutputs
	dst.PlannedEdgeReachability += src.PlannedEdgeReachability
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

func (s *Static) signatureArgumentTypeProvider(config SolveConfig, typeValues *typevalue.Cache, input effectlowering.SignatureOutcomeInputProgram) effectlowering.SignatureArgumentTypeProgram {
	programs := []effectlowering.SignatureArgumentTypeProgram{config.SignatureArgumentType, s.stateSignatureArgumentType(input)}
	if config.SignatureArgumentTypeFactory != nil {
		programs = append([]effectlowering.SignatureArgumentTypeProgram{config.SignatureArgumentTypeFactory(s.callOutcomeContext(typeValues), input)}, programs...)
	}
	return effectlowering.ComposeSignatureArgumentTypePrograms(programs...)
}

func (s *Static) callOutcomeProvider(config SolveConfig, typeValues *typevalue.Cache, signatureArgumentType effectlowering.SignatureArgumentTypeProgram, productDomain state.ProductDomain) callpayload.CallOutcomeProgram {
	var providers []callpayload.CallOutcomeProgram
	if config.CallOutcomeFactory != nil {
		providers = append(providers, config.CallOutcomeFactory(s.callOutcomeContext(typeValues)))
	}
	providers = append(providers, config.CallOutcome, s.callOutcomeSupplement)
	frontProviders := []callpayload.CallOutcomeProgram{
		effectlowering.AmbientTypestateEscapeOutcomeProvider(effectlowering.AmbientTypestateEscapeOutcomeProviderConfig{
			NameForSite: s.signatureID.nameForCallSiteView,
			Signatures:  s.signatures,
			Facts:       s.facts,
			KeySpace:    s.visibility.KeySpace(),
			Resolver:    s.visibility,
			Domain:      productDomain,
		}),
		effectlowering.AmbientChannelLifecycleOutcomeProvider(effectlowering.AmbientChannelLifecycleOutcomeProviderConfig{
			NameForSite: s.signatureID.nameForCallSiteView,
			KeySpace:    s.visibility.KeySpace(),
			Resolver:    s.visibility,
			Domain:      productDomain,
		}),
	}
	if hasSignatures(s.signatures) {
		signatureInputs, err := effectlowering.SealSignatureOutcomeOperands(productDomain, s.visibility.KeySpace())
		if err != nil {
			panic(err)
		}
		signatureProvider := effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:    s.signatures,
			NameFor:       s.signatureID.nameForCall,
			NameForSite:   s.signatureID.nameForCallSiteView,
			ReturnTypeOps: s.signatureReturnOps,
			TypeValues:    typeValues,
			Facts:         s.facts,
			ArgumentTypes: signatureArgumentType,
			ReturnValues:  stdlibSignatureReturnValue(s.registry, typeValues, signatureInputs),
			KeySpace:      s.visibility.KeySpace(),
			InputProgram:  signatureInputs,
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
	return s.callOutcomeContextWithSources(typeValues, s.sources)
}

// callOutcomeContextWithSources is the sole body-owned call adapter. Static
// solves use prepared sources; replacement application factories supply their
// session-bound source set through the same constructor.
func (s *Static) callOutcomeContextWithSources(typeValues *typevalue.Cache, sources sourcevalue.SourceValues) CallOutcomeContext {
	if s == nil {
		return CallOutcomeContext{}
	}
	if typeValues == nil {
		typeValues = s.typeValues
	}
	return CallOutcomeContext{
		LexicalBodyID: s.lexicalBodyID,
		Facts:         s.facts,
		OperationPlan: s.operationPlan,
		Sources:       sources,
		PathValue: func(ctx transfer.NodeContext, path pathdom.Path, in state.State) (product.Value, bool) {
			return readexpr.Project(readexpr.Config{
				Registry: s.registry, Facts: s.facts, Visibility: s.visibility, TypeValues: typeValues,
			}, ctx.Point, path, in)
		},
		DynamicRead: func(ctx transfer.NodeContext, tablePath pathdom.Path, owner, key product.Value, in state.State) (product.Value, bool) {
			return sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
				Registry: s.registry, TypeValues: typeValues, KeySpace: s.visibility.KeySpace(), Visibility: s.visibility,
				Point: ctx.Point, TablePath: tablePath, TableValue: owner, KeyValue: key, ValueInput: in, ProjectPath: true,
			})
		},
		DynamicTableRead: func(ctx transfer.NodeContext, tablePath pathdom.Path, table, key product.Value, in state.State) (product.Value, bool) {
			return sourcevalue.ReadBoundDynamicValue(sourcevalue.BoundDynamicRead{
				Registry: s.registry, TypeValues: typeValues, KeySpace: s.visibility.KeySpace(), Visibility: s.visibility,
				Point: ctx.Point, TablePath: tablePath, TableValue: table, KeyValue: key, ValueInput: in,
			})
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
) callpayload.CallOutcomeProgram {
	var providers []callpayload.CallOutcomeProgram
	if hasModuleExports(moduleLoads) {
		providers = append(providers, effectlowering.ModuleLoadOutcomeProvider(effectlowering.ModuleLoadOutcomeProviderConfig{
			Exports:     moduleLoads,
			NameFor:     signatureID.nameForCall,
			NameForSite: signatureID.nameForCallSiteView,
			TypeValues:  typeValues,
		}))
	}
	providers = append(providers, effectlowering.AmbientChannelSendOutcomeProvider(effectlowering.AmbientChannelSendOutcomeProviderConfig{
		KeySpace: resolver.KeySpace(),
	}))
	providers = append(providers, effectlowering.CallableValueOutcomeProvider(effectlowering.CallableValueOutcomeProviderConfig{
		Callable:   typecall.Callable,
		TypeValues: typeValues,
	}))
	providers = append(providers, explicitAnyReceiverMethodOutcomeProvider(reg, typeValues))
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
			if rootType, ok := typeValues.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
				if value, ok := luaProjectValueFromType(reg, typeValues, rootType, segments); ok {
					if family, cases, ok := variant.ProjectOriginView(origin.Family(), origin.CasesView(), segments); ok {
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
