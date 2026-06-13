package check

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/effectlowering"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/readexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (c *Checker) CheckChunk(stmts []ast.Stmt) (*Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.CheckBoundChunk(stmts, bindings)
}

func (c *Checker) CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Result, error) {
	summaries, err := c.functionSummaries(stmts, bindings)
	if err != nil {
		return nil, err
	}
	checker := c.withSummaryApplication(summaries)
	return checker.checkBoundChunk(stmts, bindings, summaries)
}

func (c *Checker) checkBoundChunk(stmts []ast.Stmt, bindings *bind.Result, summaries summaryApplication) (*Result, error) {
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract chunk semantics: %w", err)
	}
	result := c.run(bindings, built, sem)
	c.attachFunctionResults(result, bindings, nil, summaries)
	return result, nil
}

func (c *Checker) CheckFunction(fn *ast.FunctionExpr) (*Result, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.CheckBoundFunction(fn, bindings)
}

func (c *Checker) CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
	var stmts []ast.Stmt
	if fn != nil {
		stmts = fn.Stmts
	}
	summaries, err := c.functionSummaries(stmts, bindings)
	if err != nil {
		return nil, err
	}
	checker := c.withSummaryApplication(summaries)
	result, err := checker.checkBoundFunctionBody(fn, bindings)
	if err != nil {
		return nil, err
	}
	checker.attachFunctionResults(result, bindings, fn, summaries)
	return result, nil
}

func (c *Checker) checkBoundFunctionBody(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
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

func (c *Checker) run(bindings *bind.Result, built *cfgbuild.Result, sem *semantics.Result) *Result {
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
	if callOutcome == nil && config.SummaryResults != nil && config.SummaryKeyFor != nil {
		callOutcome = callresult.OutcomeProvider(config.SummaryResults, config.SummaryKeyFor)
	}
	if hasSignatures(config.Signatures) {
		callOutcome = callresult.WithSupplementalResults(callOutcome, effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
			Sources:    sources,
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

func (c *Checker) attachFunctionResults(parent *Result, bindings *bind.Result, fn *ast.FunctionExpr, summaries summaryApplication) {
	if parent == nil || bindings == nil {
		return
	}
	for _, nested := range bindings.NestedFunctions(fn) {
		child, ok := c.checkNestedFunction(nested, bindings, summaries)
		if !ok {
			continue
		}
		c.attachFunctionResults(child, bindings, nested, summaries)
		parent.functions = append(parent.functions, child)
	}
}

func (c *Checker) checkNestedFunction(fn *ast.FunctionExpr, bindings *bind.Result, summaries summaryApplication) (*Result, bool) {
	checker := c.withSummaryApplication(summaries)
	result, err := checker.checkBoundFunctionBody(fn, bindings)
	if err != nil {
		return nil, false
	}
	return result, true
}
