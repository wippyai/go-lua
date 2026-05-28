package phase

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// RunNarrow executes the narrowing phase.
func RunNarrow(input NarrowInput) NarrowOutput {
	var declaredTypes flow.DeclaredTypes
	var annotatedVars map[cfg.SymbolID]bool
	if input.Extract.Inputs != nil {
		declaredTypes = input.Extract.Inputs.DeclaredTypes
		annotatedVars = input.Extract.Inputs.AnnotatedVars
	}
	if declaredTypes == nil {
		declaredTypes = input.Scope.DeclaredTypes
	}
	if len(input.Scope.AnnotatedVars) > 0 {
		if annotatedVars == nil {
			annotatedVars = make(map[cfg.SymbolID]bool, len(input.Scope.AnnotatedVars))
			for sym, v := range input.Scope.AnnotatedVars {
				annotatedVars[sym] = v
			}
		} else {
			for sym, v := range input.Scope.AnnotatedVars {
				if v {
					annotatedVars[sym] = true
				}
			}
		}
	}

	// Prefer graph bindings for local symbol resolution, then module bindings.
	bindings := input.Graph.Bindings()
	if bindings == nil {
		bindings = input.ModuleBindings
	}

	narrowingCtx := NewContextBuilder(input.PhaseEnv).
		WithScope(input.Scope).
		WithBindings(bindings).
		WithDeclaredTypes(declaredTypes).
		WithAnnotatedVars(annotatedVars).
		WithFunctionFacts(input.FunctionFacts).
		WithLiteralTypes(input.LiteralTypes).
		WithSolution(input.Solve.Solution).
		BuildNarrow()

	engine := createNarrowedEngine(
		input.Ctx,
		input.Types,
		input.Manifests,
		input.Graph,
		input.Scope.Scopes,
		input.Solve.Solution,
		input.Extract.Inputs,
		narrowingCtx,
		input.FunctionFacts,
		input.ModuleBindings,
		input.ModuleAliases,
		input.Extract.Evidence,
		input.RecursiveFamilies,
	)

	fnEffect := InferRefinement(input.Graph, input.Solve.Solution, input.Extract.Params, input.Extract.ReturnType)
	fnEffect = EnrichWithKeysCollector(fnEffect, input.Graph, input.Extract.Evidence)

	return NarrowOutput{
		Facts:        narrowingCtx.Types(),
		Refinement:   fnEffect,
		Synth:        engine,
		QueryContext: input.Ctx,
		TypeOps:      engine.CallQuery(),
	}
}

func createNarrowedEngine(
	ctx *db.QueryContext,
	types core.TypeOps,
	manifests io.ManifestQuerier,
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	solution *flow.Solution,
	inputs *flow.Inputs,
	checkCtx api.NarrowEnv,
	functionFacts api.FunctionFacts,
	moduleBindings *bind.BindingTable,
	moduleAliases map[cfg.SymbolID]string,
	evidence api.FlowEvidence,
	recursiveFamilies *typ.RecursiveFamilyInterner,
) *synth.Engine {
	var bindings *bind.BindingTable
	if checkCtx != nil {
		bindings = checkCtx.Bindings()
	}
	if bindings == nil {
		bindings = moduleBindings
	}
	return synth.New(synth.Config{
		Ctx:            ctx,
		Types:          types,
		Scopes:         scopes,
		Flow:           solution,
		Inputs:         inputs,
		Paths:          newPathFromExprFunc(solution, bindings, graph),
		Manifests:      manifests,
		Env:            checkCtx,
		FunctionFacts:  functionFacts,
		Phase:             api.PhaseNarrowing,
		Evidence:          evidence,
		ModuleBindings:    moduleBindings,
		ModuleAliases:     moduleAliases,
		RecursiveFamilies: recursiveFamilies,
	})
}

// newPathFromExprFunc returns a PathFromExprFunc using bindings-based,
// SSA-versioned path extraction.
func newPathFromExprFunc(solution *flow.Solution, bindings *bind.BindingTable, graph *cfg.Graph) api.PathFromExprFunc {
	return func(p cfg.Point, expr ast.Expr, _ *scope.State) constraint.Path {
		if solution == nil {
			return constraint.Path{}
		}
		constResolver := func(name string) *flow.ConstValue {
			if bindings == nil {
				return nil
			}
			return solution.ConstValueAt(p, name)
		}
		return path.FromExprWithBindingsAt(expr, constResolver, bindings, graph, p)
	}
}
