// resolve.go implements Phase A (type resolution) of the analysis pipeline.
// This phase resolves type annotation expressions from AST nodes into concrete
// typ.Type values, handling @type, @param, @return annotations and type aliases.
//
// OUTPUT: A TypeResolver that subsequent phases use to resolve
// type expressions in their specific contexts (with different scope states).
package phase

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/resolveutil"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
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

	initialSymbolTypes := BuildInitialSymbolTypes(input.Graph, input.GlobalTypes, nil)
	globalCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         input.Graph,
		Bindings:      input.Bindings,
		DeclaredTypes: BuildDeclaredTypesFromSymbolTypes(input.Graph, initialSymbolTypes),
		BaseScope:     input.BaseScope,
		GlobalTypes:   input.GlobalTypes,
	})

	engine := synth.New(synth.Config{
		Ctx:       input.Ctx,
		Types:     input.Types,
		Manifests: input.Manifests,
		Env:       globalCtx,
		Phase:     api.PhaseTypeResolution,
	})

	return ResolveOutput{
		TypeResolver: engine,
	}
}

// CreateTypeResolutionEngine creates an engine for type resolution with param types.
func CreateTypeResolutionEngine(
	ctx *db.QueryContext,
	graph *cfg.Graph,
	globalTypes map[string]typ.Type,
	paramTypes map[cfg.SymbolID]typ.Type,
	base *scope.State,
	types core.TypeOps,
	manifests io.ManifestQuerier,
) *synth.Engine {
	return resolveutil.CreateTypeResolutionEngine(ctx, graph, globalTypes, paramTypes, base, types, manifests)
}

// BuildInitialSymbolTypes creates SymbolTypes for globals and parameters at all CFG points.
func BuildInitialSymbolTypes(graph *cfg.Graph, globalTypes map[string]typ.Type, paramTypes map[cfg.SymbolID]typ.Type) flow.SymbolTypes {
	return resolveutil.BuildInitialSymbolTypes(graph, globalTypes, paramTypes)
}

// BuildDeclaredTypesFromSymbolTypes extracts declared types from symbolTypes.
func BuildDeclaredTypesFromSymbolTypes(graph basecfg.VersionedGraph, symbolTypes flow.SymbolTypes) flow.DeclaredTypes {
	return resolveutil.BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
}
