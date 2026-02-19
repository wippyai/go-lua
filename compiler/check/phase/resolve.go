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
	namesToCheck := make(map[string]struct{}, len(globalTypes)+len(paramTypes))
	for name := range globalTypes {
		namesToCheck[name] = struct{}{}
	}

	paramNameTypes := make(map[string]typ.Type, len(paramTypes))
	for sym, t := range paramTypes {
		if name := graph.NameOf(sym); name != "" {
			paramNameTypes[name] = t
			namesToCheck[name] = struct{}{}
		}
	}

	if len(namesToCheck) == 0 {
		return nil
	}

	names := cfg.SortedFieldNames(namesToCheck)
	type nameMeta struct {
		name      string
		paramName typ.Type
		global    typ.Type
		globalSym cfg.SymbolID
	}

	meta := make([]nameMeta, 0, len(names))
	for _, name := range names {
		item := nameMeta{name: name}
		if t := paramNameTypes[name]; t != nil {
			item.paramName = t
		}
		if t := globalTypes[name]; t != nil {
			item.global = t
			if sym, ok := graph.GlobalSymbol(name); ok {
				item.globalSym = sym
			}
		}
		meta = append(meta, item)
	}

	paramSymbolTypes := make(map[cfg.SymbolID]flow.TypedValue, len(paramTypes))
	for sym, t := range paramTypes {
		if t != nil {
			paramSymbolTypes[sym] = flow.TypedValue{Type: t, State: flow.StateResolved}
		}
	}
	bindings := graph.Bindings()

	out := make(flow.SymbolTypes)
	// Compute types once at entry and reuse if symbols don't change
	var prevTypesAt map[cfg.SymbolID]flow.TypedValue

	for _, p := range graph.RPO() {
		locals := graph.LocalSymbolsAt(p)
		var typesAt map[cfg.SymbolID]flow.TypedValue

		for _, item := range meta {
			sym := locals[item.name]
			if sym == 0 {
				sym = item.globalSym
			}
			if sym == 0 {
				continue
			}

			tv, ok := paramSymbolTypes[sym]
			if !ok {
				switch {
				case item.paramName != nil:
					tv = flow.TypedValue{Type: item.paramName, State: flow.StateResolved}
				case item.global != nil && sym == item.globalSym:
					tv = flow.TypedValue{Type: item.global, State: flow.StateResolved}
				case item.global != nil && item.globalSym == 0 && bindings != nil:
					if kind, ok := bindings.Kind(sym); ok && kind == basecfg.SymbolGlobal {
						tv = flow.TypedValue{Type: item.global, State: flow.StateResolved}
						break
					}
					continue
				default:
					continue
				}
			}

			if typesAt == nil {
				typesAt = make(map[cfg.SymbolID]flow.TypedValue, len(meta))
			}
			typesAt[sym] = tv
		}

		if len(typesAt) == 0 {
			continue
		}

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

	entry := graph.Entry()
	bestPoint := make(map[cfg.SymbolID]cfg.Point, len(symbolTypes))
	for p, typesAt := range symbolTypes {
		if p == entry {
			continue
		}
		for sym, tv := range typesAt {
			if tv.State != flow.StateResolved || tv.Type == nil {
				continue
			}
			if prev, ok := bestPoint[sym]; !ok || p < prev {
				bestPoint[sym] = p
				out[sym] = tv.Type
			}
		}
	}

	if typesAt := symbolTypes[entry]; typesAt != nil {
		for sym, tv := range typesAt {
			if tv.State != flow.StateResolved || tv.Type == nil {
				continue
			}
			out[sym] = tv.Type
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
