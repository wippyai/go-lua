package body

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body/readexpr"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factapply/effectlowering"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (c *checker) checkChunk(stmts []ast.Stmt) (*Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.checkBoundChunk(stmts, bindings)
}

func (c *checker) checkBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Result, error) {
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract chunk semantics: %w", err)
	}
	return c.run(bindings, built, sem), nil
}

func (c *checker) checkFunction(fn *ast.FunctionExpr) (*Result, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.checkBoundFunction(fn, bindings)
}

func (c *checker) checkBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractFunction(fn, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract function semantics: %w", err)
	}
	return c.run(bindings, built, sem), nil
}

func (c *checker) run(bindings *bind.Result, built *cfgbuild.Result, sem *semantics.Result) *Result {
	config := c.config
	facts := transferfacts.Lower(sem, built.Graph, transferfacts.Config{Registry: config.Registry, Bindings: bindings})
	if hasSignatures(config.Signatures) {
		facts = effectlowering.WithSignatureNoNormalReturns(effectlowering.SignatureNoNormalReturnConfig{
			Graph:      built.Graph,
			Registry:   config.Registry,
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
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
	if userExpressionValue == nil {
		expressionValues = mergeExpressionValues(facts.ExpressionValues(), config.ExpressionValues)
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:         config.Registry,
		ExpressionValues: expressionValues,
		ExpressionValue:  expressionValue,
		VarargValue:      config.VarargValue,
	})
	callOutcome := config.CallOutcome
	if config.CallOutcomeFactory != nil {
		callOutcome = factapply.WithSupplementalCallOutcome(
			config.CallOutcomeFactory(CallOutcomeContext{Facts: facts, Sources: sources}),
			callOutcome,
		)
	}
	if hasSignatures(config.Signatures) {
		callOutcome = factapply.WithSupplementalCallOutcome(callOutcome, effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:    config.Signatures,
			NameFor:       c.signatureNameForCall(bindings),
			ReturnTypeOps: signatureReturnTypeOps(),
			Facts:         facts,
			Sources:       sources,
		}))
	}
	entryState, initial := parameterEntryState(
		config.Registry,
		built.Graph,
		bindings,
		sem.Function(),
		config.EntryState,
		config.Initial,
	)
	flow := transfer.Run(transfer.Config{
		Graph:      built.Graph,
		Registry:   config.Registry,
		EntryState: entryState,
		Initial:    initial,
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sources,
			CallOutcome: callOutcome,
			Visibility:  resolver,
		}),
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{
			Facts:       facts,
			CallOutcome: callOutcome,
			Visibility:  resolver,
		}),
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	})
	return &Result{
		registry:    config.Registry,
		bindings:    bindings,
		cfg:         built,
		semantics:   sem,
		signatures:  config.Signatures,
		facts:       facts,
		flow:        flow,
		visibility:  resolver,
		sources:     sources,
		callOutcome: callOutcome,
	}
}
