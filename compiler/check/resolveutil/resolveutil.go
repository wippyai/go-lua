package resolveutil

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

	names := make([]string, 0, len(namesToCheck))
	for name := range namesToCheck {
		names = append(names, name)
	}
	sort.Strings(names)

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
