// Package effectops provides operations for function effect propagation and export.
//
// This package handles:
//   - Effect propagation through call chains
//   - Enriching exported types with computed effect information
//   - Looking up effects for functions by symbol
//
// Effect propagation computes the combined effects of a function by examining
// all call sites and merging callee effects. Effects include termination
// guarantees, IO markers, and type predicates.
//
// Export enrichment attaches effect information to module export types so
// that importers see function refinements (like "never returns" or "type guard").
package effects

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// EnrichExportWithEffects attaches known function refinements to exported values.
// This is used when exporting module records or interfaces so method refinements
// (like termination or guard effects) are preserved at module boundaries.
func EnrichExportWithEffects(export typ.Type, rootName string, effectsBySym map[cfg.SymbolID]*constraint.FunctionEffect, graph *cfg.Graph) typ.Type {
	if export == nil || graph == nil || len(effectsBySym) == 0 {
		return export
	}

	fieldEffects := make(map[string]*constraint.FunctionEffect)
	for _, sym := range cfg.SortedSymbolIDs(effectsBySym) {
		eff := effectsBySym[sym]
		if eff == nil {
			continue
		}
		name := graph.NameOf(sym)
		if name == "" {
			continue
		}
		if rootName != "" && strings.HasPrefix(name, rootName+".") {
			field := strings.TrimPrefix(name, rootName+".")
			if field != "" {
				fieldEffects[field] = eff
			}
		} else {
			field := name
			if idx := strings.LastIndex(name, "."); idx != -1 && idx+1 < len(name) {
				field = name[idx+1:]
			}
			fieldEffects[field] = eff
		}
	}
	if len(fieldEffects) == 0 {
		return export
	}

	switch v := export.(type) {
	case *typ.Record:
		changed := false
		fields := make([]typ.Field, len(v.Fields))
		for i, f := range v.Fields {
			fields[i] = f
			if fn, ok := f.Type.(*typ.Function); ok {
				if eff := fieldEffects[f.Name]; eff != nil {
					if enriched := applyFunctionRefinement(fn, eff); enriched != nil && enriched != fn {
						fields[i].Type = enriched
						changed = true
					}
				}
			}
		}
		if !changed {
			return export
		}
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, v.MapValue)
		}
		for _, f := range fields {
			builder.Field(f.Name, f.Type)
		}
		return builder.Build()
	case *typ.Interface:
		changed := false
		methods := make([]typ.Method, len(v.Methods))
		for i, m := range v.Methods {
			methods[i] = m
			fn := m.Type
			if eff := fieldEffects[m.Name]; eff != nil {
				if enriched := applyFunctionRefinement(fn, eff); enriched != nil && enriched != fn {
					methods[i].Type = enriched
					changed = true
				}
			}
		}
		if !changed {
			return export
		}
		return typ.NewInterface(v.Name, methods)
	default:
		return export
	}
}

func applyFunctionRefinement(fn *typ.Function, eff *constraint.FunctionEffect) *typ.Function {
	if fn == nil || eff == nil {
		return fn
	}
	if fn.Refinement != nil {
		return fn
	}
	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range fn.Params {
		if p.Optional {
			builder.OptParam(p.Name, p.Type)
		} else {
			builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	builder.Returns(fn.Returns...)
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder.Spec(fn.Spec)
	}
	builder.WithRefinement(eff)
	return builder.Build()
}
