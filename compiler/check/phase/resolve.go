// resolve.go implements Phase A (type resolution) of the analysis pipeline.
// This phase resolves type annotation expressions from AST nodes into concrete
// typ.Type values, handling @type, @param, @return annotations and type aliases.
//
// OUTPUT: A TypeResolver that subsequent phases use to resolve
// type expressions in their specific contexts (with different scope states).
package phase

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// RunResolve executes Phase A (type resolution) and returns a type expression resolver.
// The resolver is a closure that captures the configured synthesis engine and can
// resolve type expressions in any scope context.
//
// This phase:
//  1. Builds initial symbol types from globals and parameters
//  2. Creates a declared-phase synthesis engine
//  3. Returns a resolver function for use by subsequent phases
func RunResolve(input ResolveInput) ResolveOutput {
	if input.Graph == nil {
		return ResolveOutput{}
	}

	globalCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         input.Graph,
		Bindings:      input.Bindings,
		DeclaredTypes: BuildDeclaredTypesForResolve(input.Graph, input.GlobalTypes, nil),
		BaseScope:     input.BaseScope,
		GlobalTypes:   input.GlobalTypes,
	})

	env := input.PhaseEnv
	env.Env = globalCtx
	env.Phase = api.PhaseTypeResolution
	env.ModuleBindings = firstNonNilBindings(input.ModuleBindings, input.Bindings)
	env.ModuleAliases = firstNonNilAliases(input.ModuleAliases, modules.CollectAliases(input.Graph))
	engine := synth.New(env)

	return ResolveOutput{
		TypeResolver: engine,
	}
}

// CreateTypeResolutionEngine creates an engine for type resolution with param types.
// moduleAliases carries the module aliases visible from enclosing graphs (the
// chunk's local x = require("m")); they resolve qualified type names such as
// x.T in the annotations of graph, together with the aliases graph declares.
func CreateTypeResolutionEngine(
	env PhaseEnv,
	paramTypes map[cfg.SymbolID]typ.Type,
	base *scope.State,
) *synth.Engine {
	graph := env.Graph
	if graph == nil {
		env.Phase = api.PhaseTypeResolution
		return synth.New(env)
	}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      graph.Bindings(),
		DeclaredTypes: BuildDeclaredTypesForResolve(graph, env.GlobalTypes, paramTypes),
		BaseScope:     base,
		GlobalTypes:   env.GlobalTypes,
	})
	env.Env = checkCtx
	env.Phase = api.PhaseTypeResolution
	env.ModuleBindings = graph.Bindings()
	env.ModuleAliases = modules.MergeAliases(env.ModuleAliases, modules.CollectAliases(graph))
	return synth.New(env)
}

func firstNonNilBindings(primary, fallback *bind.BindingTable) *bind.BindingTable {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstNonNilAliases(primary, fallback map[cfg.SymbolID]string) map[cfg.SymbolID]string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

// BuildDeclaredTypesForResolve collects global and parameter declarations by
// binding identity for the resolve phase.
func BuildDeclaredTypesForResolve(graph *cfg.Graph, globalTypes map[string]typ.Type, paramTypes map[cfg.SymbolID]typ.Type) flow.DeclaredTypes {
	if graph == nil || (len(globalTypes) == 0 && len(paramTypes) == 0) {
		return nil
	}

	out := make(flow.DeclaredTypes, len(globalTypes)+len(paramTypes))

	for sym, t := range paramTypes {
		if t != nil {
			out[sym] = t
		}
	}

	for name, t := range globalTypes {
		if t == nil {
			continue
		}
		if sym, ok := graph.GlobalSymbol(name); ok && sym != 0 {
			out[sym] = t
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
