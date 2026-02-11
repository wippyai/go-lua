// resolve.go implements Phase A (type resolution) of the analysis pipeline.
// This phase resolves type annotation expressions from AST nodes into concrete
// typ.Type values, handling @type, @param, @return annotations and type aliases.
//
// OUTPUT: A TypeResolver that subsequent phases use to resolve
// type expressions in their specific contexts (with different scope states).
package phase

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
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
	initialSymbolTypes := BuildInitialSymbolTypes(graph, globalTypes, paramTypes)
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      graph.Bindings(),
		DeclaredTypes: BuildDeclaredTypesFromSymbolTypes(graph, initialSymbolTypes),
		BaseScope:     base,
		GlobalTypes:   globalTypes,
	})
	return synth.New(synth.Config{
		Ctx:       ctx,
		Types:     types,
		Manifests: manifests,
		Env:       checkCtx,
		Phase:     api.PhaseTypeResolution,
	})
}

// BuildInitialSymbolTypes creates SymbolTypes for globals and parameters at all CFG points.
func BuildInitialSymbolTypes(graph *cfg.Graph, globalTypes map[string]typ.Type, paramTypes map[cfg.SymbolID]typ.Type) flow.SymbolTypes {
	if graph == nil {
		return nil
	}

	// Collect all names we need to look up
	namesToCheck := make(map[string]typ.Type, len(globalTypes)+len(paramTypes))
	for name, t := range globalTypes {
		namesToCheck[name] = t
	}

	paramNameTypes := make(map[string]typ.Type, len(paramTypes))
	for sym, t := range paramTypes {
		if name := graph.NameOf(sym); name != "" {
			paramNameTypes[name] = t
			namesToCheck[name] = t
		}
	}

	if len(namesToCheck) == 0 {
		return nil
	}

	names := cfg.SortedFieldNames(namesToCheck)

	bindings := graph.Bindings()
	out := make(flow.SymbolTypes)

	// Compute types once at entry and reuse if symbols don't change
	var prevTypesAt map[cfg.SymbolID]flow.TypedValue

	for _, p := range graph.RPO() {
		var typesAt map[cfg.SymbolID]flow.TypedValue

		// Check each name we care about
		for _, name := range names {
			sym, ok := graph.SymbolAt(p, name)
			if !ok || sym == 0 {
				continue
			}

			var tv flow.TypedValue
			if t, ok := paramTypes[sym]; ok && t != nil {
				tv = flow.TypedValue{Type: t, State: flow.StateResolved}
			} else if t, ok := paramNameTypes[name]; ok && t != nil {
				tv = flow.TypedValue{Type: t, State: flow.StateResolved}
			} else if t, ok := globalTypes[name]; ok && t != nil {
				if bindings != nil {
					if kind, ok := bindings.Kind(sym); ok && kind == basecfg.SymbolGlobal {
						tv = flow.TypedValue{Type: t, State: flow.StateResolved}
					}
				}
			}

			if tv.Type != nil {
				if typesAt == nil {
					typesAt = make(map[cfg.SymbolID]flow.TypedValue, len(namesToCheck))
				}
				typesAt[sym] = tv
			}
		}

		if len(typesAt) > 0 {
			// Reuse previous map if identical content
			if prevTypesAt != nil && len(typesAt) == len(prevTypesAt) {
				identical := true
				for sym, tv := range typesAt {
					if prev, ok := prevTypesAt[sym]; !ok || prev != tv {
						identical = false
						break
					}
				}
				if identical {
					out[p] = prevTypesAt
					continue
				}
			}
			out[p] = typesAt
			prevTypesAt = typesAt
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildDeclaredTypesFromSymbolTypes extracts declared types from symbolTypes.
func BuildDeclaredTypesFromSymbolTypes(graph basecfg.VersionedGraph, symbolTypes flow.SymbolTypes) flow.DeclaredTypes {
	if symbolTypes == nil {
		return nil
	}
	out := make(flow.DeclaredTypes)
	points := make([]cfg.Point, 0, len(symbolTypes))
	for p := range symbolTypes {
		points = append(points, p)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })

	entry := graph.Entry()
	for _, p := range points {
		typesAt := symbolTypes[p]
		for sym, tv := range typesAt {
			if tv.State != flow.StateResolved || tv.Type == nil {
				continue
			}
			if p == entry || out[sym] == nil {
				out[sym] = tv.Type
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
