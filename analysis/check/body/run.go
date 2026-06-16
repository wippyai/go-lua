package body

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (c *checker) prepareChunk(stmts []ast.Stmt) (*Static, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundChunk(stmts, bindings)
}

func (c *checker) prepareBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Static, error) {
	if c.config.Stats != nil {
		c.config.Stats.StaticChunkPrepares++
	}
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract chunk semantics: %w", err)
	}
	return c.prepare(bindings, built, sem), nil
}

func (c *checker) prepareFunction(fn *ast.FunctionExpr) (*Static, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.prepareBoundFunction(fn, bindings)
}

func (c *checker) prepareBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Static, error) {
	if c.config.Stats != nil {
		c.config.Stats.StaticFunctionPrepares++
	}
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractFunction(fn, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract function semantics: %w", err)
	}
	return c.prepare(bindings, built, sem), nil
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
		})
	}
	expressionValues := config.ExpressionValues
	var expressionPaths map[factflow.ExprRef]struct{}
	if userExpressionValue == nil {
		expressionValues = mergeExpressionValues(facts.ExpressionValues(), config.ExpressionValues)
		expressionPaths = exprRefSet(facts.ExpressionPaths())
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:         config.Registry,
		ExpressionValues: expressionValues,
		ExpressionPaths:  expressionPaths,
		ObjectLiterals:   facts.ObjectLiterals(),
		ObjectLiteral:    objectLiteralEvaluator(config.Registry),
		ExpressionOps:    facts.ExpressionOperations(),
		ExpressionOp:     expressionOperationEvaluator(config.Registry),
		ExpressionValue:  expressionValue,
		VarargValue:      config.VarargValue,
	})
	calleeValue := calleeValueProvider(config.Registry, facts, resolver, sources)
	signatureID.indexCallSites(facts)
	return &Static{
		registry:    config.Registry,
		bindings:    bindings,
		cfg:         built,
		semantics:   sem,
		signatures:  config.Signatures,
		moduleTypes: config.ModuleTypes,
		moduleLoads: config.ModuleExports,
		modules:     modules,
		signatureID: signatureID,
		facts:       facts,
		visibility:  resolver,
		sources:     sources,
		calleeValue: calleeValue,
		typeNS:      typeResolver,
	}
}

func (s *Static) Solve(config SolveConfig) *Result {
	if s == nil {
		return nil
	}
	if config.Stats != nil {
		config.Stats.BodySolves++
	}
	callOutcome := s.callOutcomeProvider(config)
	entryState, initial := parameterEntryState(
		s.registry,
		s.cfg.Graph,
		s.bindings,
		s.semantics.Function(),
		s.moduleLoads,
		s.typeNS,
		config.EntryState,
		config.Initial,
	)
	nodeTransfer := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts:       s.facts,
		Sources:     s.sources,
		CallOutcome: callOutcome,
		Visibility:  s.visibility,
		ProjectPath: luaPathTypeProjector,
	})
	nodeTransfer = genericForNodeTransfer(nodeTransfer, s.semantics, s.facts, s.sources, s.signatures, s.signatureID, s.typeNS)
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
		}),
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
		Stats:      transferStats(config.Stats),
	})
	return &Result{
		registry:     s.registry,
		bindings:     s.bindings,
		cfg:          s.cfg,
		semantics:    s.semantics,
		signatures:   s.signatures,
		moduleTypes:  s.moduleTypes,
		modules:      s.modules,
		signatureID:  s.signatureID,
		facts:        s.facts,
		flow:         flow,
		boundaryXfer: nodeTransfer,
		visibility:   s.visibility,
		sources:      s.sources,
		callOutcome:  callOutcome,
	}
}

func transferStats(stats *Stats) *transfer.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Transfer
}

func (s *Static) callOutcomeProvider(config SolveConfig) factapply.CallOutcomeProvider {
	signatureArgumentType := config.SignatureArgumentType
	if config.SignatureArgumentTypeFactory != nil {
		factoryArgumentType := config.SignatureArgumentTypeFactory(CallOutcomeContext{Facts: s.facts, Sources: s.sources, CalleeValue: s.calleeValue})
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
			config.CallOutcomeFactory(CallOutcomeContext{Facts: s.facts, Sources: s.sources, CalleeValue: s.calleeValue}),
			callOutcome,
		)
	}
	if hasModuleExports(s.moduleLoads) {
		callOutcome = calloutcome.WithSupplemental(callOutcome, effectlowering.ModuleLoadOutcomeProvider(effectlowering.ModuleLoadOutcomeProviderConfig{
			Exports: s.moduleLoads,
			NameFor: s.signatureID.nameForCall,
			Facts:   s.facts,
			Sources: s.sources,
		}))
	}
	callOutcome = calloutcome.WithSupplemental(callOutcome, effectlowering.CallableValueOutcomeProvider(effectlowering.CallableValueOutcomeProviderConfig{
		CalleeValue: effectlowering.CalleeValueFunc(s.calleeValue),
		Callable:    typecall.Callable,
	}))
	if hasSignatures(s.signatures) {
		// A declared signature is the authoritative result for the names it
		// covers, so it leads the merge: its concrete return slots and
		// postconditions take precedence over the generic callable-value and
		// base-outcome fallbacks, which then supplement uncovered slots.
		callOutcome = calloutcome.WithSupplemental(effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:    s.signatures,
			NameFor:       s.signatureID.nameForCall,
			ReturnTypeOps: signatureReturnTypeOps(),
			Facts:         s.facts,
			Sources:       s.sources,
			ArgumentType:  effectlowering.SignatureArgumentTypeFunc(signatureArgumentType),
		}), callOutcome)
	}
	return callOutcome
}

func luaPathTypeProjector(root typ.Type, p pathdom.Path) (typ.Type, bool) {
	return luatypeprojection.ApplySegments(root, p.Segments)
}
