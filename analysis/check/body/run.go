package body

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourceprojection"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (c *checker) prepareChunk(stmts []ast.Stmt) (*Static, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundChunk(stmts, bindings)
}

func (c *checker) prepareBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Static, error) {
	return c.prepareBound(bindings, "chunk",
		func() { c.config.Stats.StaticChunkPrepares++ },
		func() *cfgbuild.Result { return cfgbuild.BuildChunk(stmts, bindings) },
		func(built *cfgbuild.Result) (*semantics.Result, error) {
			return semantics.ExtractChunk(stmts, bindings, built)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerWithResolver("chunk", stmts, bindings, built, resolver)
		},
	)
}

// prepareBound builds the CFG, extracts semantics, and prepares static state for
// a chunk or function. incStat bumps the matching prepare counter, build builds
// the CFG, and extract derives semantics; what labels the kind in errors.
func (c *checker) prepareBound(
	bindings *bind.Result,
	what string,
	incStat func(),
	build func() *cfgbuild.Result,
	extract func(*cfgbuild.Result) (*semantics.Result, error),
	lowerWIR func(*cfgbuild.Result, *typeresolve.Resolver) *wir.Body,
) (*Static, error) {
	if c.config.Stats != nil {
		incStat()
	}
	built := build()
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := extract(built)
	if err != nil {
		return nil, fmt.Errorf("check: extract %s semantics: %w", what, err)
	}
	modules := moduleidentity.New(bindings, built.Graph, sem)
	moduleTypes := newRequireAliasTypeResolver(modules, c.config.ModuleTypes)
	typeResolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	wirBody := lowerWIR(built, typeResolver)
	return c.prepare(bindings, built, sem, wirBody, modules, typeResolver), nil
}

func (c *checker) prepareFunction(fn *ast.FunctionExpr) (*Static, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundFunction(fn, bindings)
}

func (c *checker) prepareBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Static, error) {
	return c.prepareBound(bindings, "function",
		func() { c.config.Stats.StaticFunctionPrepares++ },
		func() *cfgbuild.Result { return cfgbuild.BuildFunction(fn, bindings) },
		func(built *cfgbuild.Result) (*semantics.Result, error) {
			return semantics.ExtractFunction(fn, bindings, built)
		},
		func(built *cfgbuild.Result, resolver *typeresolve.Resolver) *wir.Body {
			return wirlower.LowerWithResolver("function", fn.Stmts, bindings, built, resolver)
		},
	)
}

func (c *checker) prepare(
	bindings *bind.Result,
	built *cfgbuild.Result,
	sem *semantics.Result,
	wirBody *wir.Body,
	modules moduleidentity.Projection,
	typeResolver *typeresolve.Resolver,
) *Static {
	config := c.config
	globals := configGlobals(config)
	lowered := transferfacts.LowerWithSidecars(sem, built.Graph, transferfacts.Config{
		Registry:      config.Registry,
		Bindings:      bindings,
		TypeResolver:  typeResolver,
		TypeValues:    config.TypeValues,
		ModuleExports: config.ModuleExports,
		WIR:           wirBody,
	})
	facts := lowered.Facts
	signatureID := newSignatureIdentityResolver(bindings, built.Graph, modules, config.Signatures)
	signatureNameForCall := signatureID.nameForCall
	if hasSignatures(config.Signatures) {
		facts = effectlowering.WithSignatureNoNormalReturns(effectlowering.SignatureNoNormalReturnConfig{
			Graph:      built.Graph,
			Registry:   config.Registry,
			Signatures: config.Signatures,
			NameFor:    signatureNameForCall,
			Facts:      facts,
		})
	}
	resolver := config.Visibility
	if resolver == nil {
		resolver = defaultVisibilityResolver(bindings, built, sem, facts)
	}
	userExpressionValue := config.ExpressionValue
	expressionValue := userExpressionValue
	if expressionValue == nil {
		expressionValue = readexpr.Provider(readexpr.Config{
			Registry:        config.Registry,
			Facts:           facts,
			Visibility:      resolver.Before(),
			TypeValues:      config.TypeValues,
			ProofVisibility: resolver,
		})
	}
	expressionValues := config.ExpressionValues
	var expressionPaths map[factflow.ExprRef]struct{}
	if userExpressionValue == nil {
		expressionValues = mergeExpressionValues(facts.ExpressionValues(), config.ExpressionValues)
		expressionPaths = exprRefSet(facts.ExpressionPaths())
		expressionPaths = addDynamicIndexExprRefs(expressionPaths, facts.DynamicIndexExpressions())
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:              config.Registry,
		TypeValues:            config.TypeValues,
		KeySpace:              resolver.KeySpace(),
		ExpressionValues:      expressionValues,
		ExpressionPaths:       expressionPaths,
		ObjectLiteralView:     facts.ObjectLiteralView,
		ObjectLiteralFromView: objectLiteralViewEvaluator(config.Registry, config.TypeValues),
		ExpressionOps:         facts.ExpressionOperations(),
		ExpressionConditions:  facts.ExpressionConditions(),
		DynamicIndexExprs:     facts.DynamicIndexExpressions(),
		ExpressionOp:          expressionOperationEvaluator(config.Registry, config.TypeValues),
		ExpressionCondition: func(point cfg.Point, in state.State, selected factflow.ExpressionConditionFacts) state.State {
			return factapply.ApplyExpressionConditionFacts(config.Registry, resolver, luaPathTypeProjector, point, in, selected)
		},
		ExpressionValue: expressionValue,
		VarargValue:     config.VarargValue,
	})
	refinedSources := sourcevalue.NewExpressionRefinements(facts.ExpressionRefinements()).Bind(config.Registry, sources)
	calleeValue := calleeValueProvider(config.Registry, facts, resolver, refinedSources, config.TypeValues, bindings, typeResolver)
	receiverFn := declaredReceiverCallableProvider(facts, bindings, typeResolver)
	signatureID.indexCallSites(facts)
	callOutcomeSupplement := preparedCallOutcomeSupplement(config.Registry, config.ModuleExports, signatureID, facts, resolver, refinedSources, config.TypeValues, calleeValue)
	entrySeeds := entrySeedPlan(config.Registry, config.TypeValues, bindings, sem.Function(), globals, config.GlobalTypes, config.ModuleExports, typeResolver)
	return &Static{
		registry:              config.Registry,
		bindings:              bindings,
		cfg:                   built,
		semantics:             sem,
		signatures:            config.Signatures,
		moduleTypes:           config.ModuleTypes,
		moduleLoads:           config.ModuleExports,
		globals:               globals,
		globalTypes:           config.GlobalTypes,
		modules:               modules,
		signatureID:           signatureID,
		facts:                 facts,
		symbolTypes:           lowered.SymbolTypes,
		visibility:            resolver,
		sources:               sources,
		calleeValue:           calleeValue,
		receiverFn:            receiverFn,
		typeNS:                typeResolver,
		typeValues:            config.TypeValues,
		entrySeeds:            entrySeeds,
		entrySeedsPrepared:    true,
		callOutcomeSupplement: callOutcomeSupplement,
		signatureReturnOps:    signatureReturnTypeOps(),
	}
}

func (s *Static) Solve(config SolveConfig) *Result {
	if s == nil {
		return nil
	}
	if config.Stats != nil {
		config.Stats.BodySolves++
	}
	typeValues := s.solveTypeValues(config)
	signatureArgumentType := s.signatureArgumentTypeProvider(config, typeValues)
	callOutcome := s.callOutcomeProvider(config, typeValues, signatureArgumentType)
	entryState, initial := s.solveEntryState(typeValues, config.EntryState, config.Initial)
	nodeTransfer := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts:                  s.facts,
		Sources:                s.sources,
		CallOutcome:            callOutcome,
		Visibility:             s.visibility,
		ProjectPath:            luaPathTypeProjector,
		CovariantWiden:         luaCovariantWiden,
		TypeValues:             typeValues,
		ClosedDynamicAllValues: config.ClosedDynamicAllValues,
	})
	nodeTransfer = genericForNodeTransfer(nodeTransfer, s.semantics, s.facts, s.sources, s.symbolTypes, s.signatures, s.signatureID, s.typeNS, typeValues, callOutcome, s.visibility.KeySpace(), s.visibility)
	edgeTransfer := factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{
		Facts:       s.facts,
		Sources:     s.sources,
		CallOutcome: callOutcome,
		Visibility:  s.visibility,
		ProjectPath: luaPathTypeProjector,
		TypeValues:  typeValues,
	})
	flow := transfer.Run(transfer.Config{
		Graph:        s.cfg.Graph,
		Registry:     s.registry,
		StateLanes:   config.StateLanes,
		EntryState:   entryState,
		Initial:      initial,
		NodeTransfer: nodeTransfer,
		EdgeTransfer: edgeTransfer,
		WidenAt:      config.WidenAt,
		WidenDelay:   config.WidenDelay,
		Stats:        transferStats(config.Stats),
	})
	flow = s.finalizeReturnSlotHeapWitnesses(flow, typeValues)
	return &Result{
		registry:        s.registry,
		bindings:        s.bindings,
		cfg:             s.cfg,
		semantics:       s.semantics,
		signatures:      s.signatures,
		moduleTypes:     s.moduleTypes,
		modules:         s.modules,
		signatureID:     s.signatureID,
		facts:           s.facts,
		symbolTypes:     s.symbolTypes,
		exprRefinements: sourcevalue.NewExpressionRefinements(s.facts.ExpressionRefinements()),
		typeNS:          s.typeNS,
		flow:            flow,
		boundaryXfer:    nodeTransfer,
		edgeXfer:        edgeTransfer,
		visibility:      s.visibility,
		sources:         s.sources,
		callOutcome:     callOutcome,
		signatureArg:    signatureArgumentType,
		typeValues:      typeValues,
		stateLanes:      append([]state.LaneID(nil), config.StateLanes...),
		queries:         newResultQueryCache(s.facts),
	}
}

func (s *Static) finalizeReturnSlotHeapWitnesses(flow transfer.Result, typeValues *typevalue.Cache) transfer.Result {
	if s == nil || s.cfg.Graph == nil || s.registry == nil || len(flow) == 0 {
		return flow
	}
	exit := s.cfg.Graph.Exit()
	exitState, ok := flow[exit]
	if !ok {
		return flow
	}
	slots := make(map[int]struct{})
	for _, point := range s.cfg.Graph.RPO() {
		fact, ok := s.facts.Return(point)
		if !ok {
			continue
		}
		for i, source := range fact.Sources() {
			index := source.TargetIndex
			if index < 0 {
				index = i
			}
			slots[index] = struct{}{}
		}
	}
	for index := range slots {
		value := exitState.ReadReturnSlot(s.registry, index)
		projected, ok := sourceprojection.HeapObjectContainerType(s.registry, typeValues, exitState, value)
		if !ok {
			continue
		}
		exitState = exitState.WriteReturnSlot(s.registry, index, typevalue.WithWitness(s.registry, value, projected))
	}
	flow[exit] = exitState
	return flow
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
		s.semantics.Function(),
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
			ReturnValue:   stdlibSignatureReturnValue(s.registry, typeValues, s.facts, s.sources, sourcevalue.NewExpressionRefinements(s.facts.ExpressionRefinements()), s.visibility),
			KeySpace:      s.visibility.KeySpace(),
		})
		providers = append([]callpayload.CallOutcomeProvider{signatureProvider}, providers...)
	}
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
		Facts:                       s.facts,
		Sources:                     s.sources,
		CalleeValue:                 s.calleeValue,
		ReceiverCallable:            s.receiverFn,
		ReturnPresenceRelationsPath: s.returnPresenceRelationsForPath,
		KeySpace:                    s.visibility.KeySpace(),
		TypeValues:                  typeValues,
	}
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
	expressionRefinements := facts.ExpressionRefinements()
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
