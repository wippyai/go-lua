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
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	return c.prepare(bindings, built, sem), nil
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
	)
}

func (c *checker) prepare(bindings *bind.Result, built *cfgbuild.Result, sem *semantics.Result) *Static {
	config := c.config
	modules := moduleidentity.New(bindings, built.Graph, sem)
	moduleTypes := newRequireAliasTypeResolver(modules, config.ModuleTypes)
	typeResolver := typeresolve.NewWithExternal(bindings, moduleTypes)
	facts := transferfacts.Lower(sem, built.Graph, transferfacts.Config{
		Registry:     config.Registry,
		Bindings:     bindings,
		TypeResolver: typeResolver,
		TypeValues:   config.TypeValues,
	})
	signatureID := newSignatureIdentityResolver(bindings, built.Graph, modules)
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
		resolver = defaultVisibilityResolver(bindings, built, facts)
	}
	userExpressionValue := config.ExpressionValue
	expressionValue := userExpressionValue
	if expressionValue == nil {
		expressionValue = readexpr.Provider(readexpr.Config{
			Registry:   config.Registry,
			Facts:      facts,
			Visibility: resolver,
			TypeValues: config.TypeValues,
		})
	}
	expressionValues := config.ExpressionValues
	var expressionPaths map[factflow.ExprRef]struct{}
	if userExpressionValue == nil {
		expressionValues = mergeExpressionValues(facts.ExpressionValues(), config.ExpressionValues)
		expressionPaths = exprRefSet(facts.ExpressionPaths())
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:              config.Registry,
		ExpressionValues:      expressionValues,
		ExpressionPaths:       expressionPaths,
		ObjectLiteralView:     facts.ObjectLiteralView,
		ObjectLiteralFromView: objectLiteralViewEvaluator(config.Registry, config.TypeValues),
		ExpressionOps:         facts.ExpressionOperations(),
		ExpressionOp:          expressionOperationEvaluator(config.Registry, config.TypeValues),
		ExpressionValue:       expressionValue,
		VarargValue:           config.VarargValue,
	})
	calleeValue := calleeValueProvider(config.Registry, facts, resolver, sources, config.TypeValues)
	signatureID.indexCallSites(facts)
	callOutcomeSupplement := preparedCallOutcomeSupplement(config.Registry, config.ModuleExports, signatureID, facts, resolver, sources, config.TypeValues, calleeValue)
	return &Static{
		registry:              config.Registry,
		bindings:              bindings,
		cfg:                   built,
		semantics:             sem,
		signatures:            config.Signatures,
		moduleTypes:           config.ModuleTypes,
		moduleLoads:           config.ModuleExports,
		modules:               modules,
		signatureID:           signatureID,
		facts:                 facts,
		visibility:            resolver,
		sources:               sources,
		calleeValue:           calleeValue,
		typeNS:                typeResolver,
		typeValues:            config.TypeValues,
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
	typeValues := config.TypeValues
	if typeValues == nil {
		typeValues = s.typeValues
	}
	if typeValues == nil {
		typeValues = typevalue.NewCache()
	}
	callOutcome := s.callOutcomeProvider(config)
	entryState, initial := parameterEntryState(
		s.registry,
		typeValues,
		s.cfg.Graph,
		s.bindings,
		s.semantics.Function(),
		s.moduleLoads,
		s.typeNS,
		config.EntryState,
		config.Initial,
	)
	nodeTransfer := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts:          s.facts,
		Sources:        s.sources,
		CallOutcome:    callOutcome,
		Visibility:     s.visibility,
		ProjectPath:    luaPathTypeProjector,
		CovariantWiden: luaCovariantWiden,
		TypeValues:     typeValues,
	})
	nodeTransfer = genericForNodeTransfer(nodeTransfer, s.semantics, s.facts, s.sources, s.signatures, s.signatureID, s.typeNS, typeValues, callOutcome, s.visibility.KeySpace())
	flow := transfer.Run(transfer.Config{
		Graph:        s.cfg.Graph,
		Registry:     s.registry,
		EntryState:   entryState,
		Initial:      initial,
		NodeTransfer: nodeTransfer,
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{
			Facts:       s.facts,
			CallOutcome: callOutcome,
			Visibility:  s.visibility,
			ProjectPath: luaPathTypeProjector,
			TypeValues:  typeValues,
		}),
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
		Stats:      transferStats(config.Stats),
	})
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
		exprRefinements: s.facts.ExpressionRefinements(),
		flow:            flow,
		boundaryXfer:    nodeTransfer,
		visibility:      s.visibility,
		sources:         s.sources,
		callOutcome:     callOutcome,
		typeValues:      typeValues,
	}
}

func transferStats(stats *Stats) *transfer.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Transfer
}

func (s *Static) callOutcomeProvider(config SolveConfig) callpayload.CallOutcomeProvider {
	signatureArgumentType := config.SignatureArgumentType
	if config.SignatureArgumentTypeFactory != nil {
		factoryArgumentType := config.SignatureArgumentTypeFactory(s.callOutcomeContext())
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
	callOutcome := config.CallOutcome
	if config.CallOutcomeFactory != nil {
		callOutcome = calloutcome.WithSupplemental(
			config.CallOutcomeFactory(s.callOutcomeContext()),
			callOutcome,
		)
	}
	callOutcome = calloutcome.WithSupplemental(callOutcome, s.callOutcomeSupplement)
	if hasSignatures(s.signatures) {
		// A declared signature is the authoritative result for the names it
		// covers, so it leads the merge: its concrete return slots and
		// postconditions take precedence over the generic callable-value and
		// base-outcome fallbacks, which then supplement uncovered slots.
		callOutcome = calloutcome.WithSupplemental(effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:    s.signatures,
			NameFor:       s.signatureID.nameForCall,
			NameForSite:   s.signatureID.nameForCallSiteView,
			ReturnTypeOps: s.signatureReturnOps,
			Facts:         s.facts,
			Sources:       s.sources,
			ArgumentType:  effectlowering.SignatureArgumentTypeFunc(signatureArgumentType),
			KeySpace:      s.visibility.KeySpace(),
		}), callOutcome)
	}
	return callOutcome
}

func (s *Static) callOutcomeContext() CallOutcomeContext {
	if s == nil {
		return CallOutcomeContext{}
	}
	return CallOutcomeContext{
		Facts:                       s.facts,
		Sources:                     s.sources,
		CalleeValue:                 s.calleeValue,
		ReturnPresenceRelationsPath: s.returnPresenceRelationsForPath,
		KeySpace:                    s.visibility.KeySpace(),
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
	var out callpayload.CallOutcomeProvider
	expressionRefinements := facts.ExpressionRefinements()
	if hasModuleExports(moduleLoads) {
		out = calloutcome.WithSupplemental(out, effectlowering.ModuleLoadOutcomeProvider(effectlowering.ModuleLoadOutcomeProviderConfig{
			Exports:               moduleLoads,
			NameFor:               signatureID.nameForCall,
			NameForSite:           signatureID.nameForCallSiteView,
			Sources:               sources,
			ExpressionRefinements: expressionRefinements,
		}))
	}
	out = calloutcome.WithSupplemental(out, effectlowering.AmbientChannelSendOutcomeProvider(effectlowering.AmbientChannelSendOutcomeProviderConfig{
		ReceiverType: channelMethodReceiverTypeProvider(reg, facts, resolver, sources, typeValues),
	}))
	return calloutcome.WithSupplemental(out, effectlowering.CallableValueOutcomeProvider(effectlowering.CallableValueOutcomeProviderConfig{
		CalleeValue: effectlowering.CalleeValueFunc(calleeValue),
		Callable:    typecall.Callable,
	}))
}

func luaPathTypeProjector(root typ.Type, p pathdom.Path) (typ.Type, bool) {
	return luatypeprojection.ApplySegments(root, p.Segments)
}
