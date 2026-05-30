package modules

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExportType computes the module's exported type from return statements.
// Pass refinementsBySym to enrich exported functions with effect summaries.
func ExportType(result *api.FuncResult, refinementsBySym map[cfg.SymbolID]*constraint.FunctionRefinement) typ.Type {
	if result == nil || result.Graph == nil || result.NarrowSynth == nil {
		return typ.Nil
	}

	synth := result.NarrowSynth

	var export typ.Type
	var exportRootName string
	var exportRootSet bool

	for _, ret := range result.Evidence.Returns {
		p := ret.Point
		info := ret.Info
		if info == nil {
			continue
		}
		if result.FlowInputs != nil && result.FlowInputs.DeadPoints[p] {
			continue
		}
		if len(info.Exprs) == 0 {
			if export == nil {
				export = typ.Nil
			} else {
				export = typ.NewUnion(export, typ.Nil)
			}
			continue
		}

		valueType := synth.TypeOf(info.Exprs[0], p)
		if valueType == nil {
			valueType = typ.Nil
		}

		// Track the root name for export path-based summaries (e.g., return M).
		if ident, ok := info.Exprs[0].(*ast.IdentExpr); ok && ident != nil {
			if !exportRootSet {
				exportRootName = ident.Value
				exportRootSet = true
			} else if exportRootName != ident.Value {
				exportRootName = ""
			}
		} else if exportRootSet {
			exportRootName = ""
		}

		if export == nil {
			export = valueType
		} else if !typ.TypeEquals(export, valueType) {
			export = typ.NewUnion(export, valueType)
		}
	}

	if export != nil && len(refinementsBySym) > 0 && result.Graph != nil {
		export = effects.EnrichExportWithEffects(export, exportRootName, refinementsBySym, result.Graph)
	}

	if export == nil {
		return typ.Nil
	}
	return export
}

// ExportTypes extracts module-local type definitions for manifest generation.
func ExportTypes(result *api.FuncResult) map[string]typ.Type {
	if result == nil || result.Graph == nil {
		return nil
	}

	types := make(map[string]typ.Type)
	result.Graph.EachTypeDef(func(p cfg.Point, info *cfg.TypeDefInfo) {
		if info.Name == "" {
			return
		}

		sc := result.Scopes[p]
		if sc == nil {
			return
		}

		if resolved, ok := sc.LookupType(info.Name); ok {
			types[info.Name] = resolved
		}
	})
	return types
}

// CopyRefinementsForExport returns a defensive copy of refinements for manifest export.
func CopyRefinementsForExport(refinementsBySym map[cfg.SymbolID]*constraint.FunctionRefinement) map[cfg.SymbolID]*constraint.FunctionRefinement {
	if len(refinementsBySym) == 0 {
		return nil
	}
	refinements := make(map[cfg.SymbolID]*constraint.FunctionRefinement, len(refinementsBySym))
	for sym, refinement := range refinementsBySym {
		if refinement != nil {
			refinements[sym] = refinement
		}
	}
	return refinements
}

// ExportFunctionResult pairs an exported function's definition name with the
// solved analysis result of its body, supplying the source needed to recover the
// function's inferred return vector and proven (value, err) correlation.
type ExportFunctionResult struct {
	// Name is the function definition's source name (e.g. "M.request").
	Name string
	// Result is the solved per-function analysis result.
	Result *api.FuncResult
}

// EnrichExportFunctions overlays each exported function field with its
// body-proven return vector and ErrorReturn correlation, recovered from the
// function's own solved analysis result.
//
// The overlay is additive: a function field that already carries a concrete
// return vector or an ErrorReturn label keeps it, so a flow whose export already
// publishes the inferred summary (the legacy interproc projection) is unchanged.
// A flow whose export exposes only the bare declared signature gains the inferred
// return and the proven (value, err) inverse pattern the importer needs to type
// the call result and correlate sibling slots.
//
// Each per-function result is consumed through its own NarrowSynth observer; the
// same return-summary and ErrorReturn machinery the interproc signature projection
// uses derives the overlay, so no body inference is re-run here.
func EnrichExportFunctions(export typ.Type, results []ExportFunctionResult) typ.Type {
	if export == nil || len(results) == 0 {
		return export
	}
	rec, ok := unwrap.Alias(export).(*typ.Record)
	if !ok {
		return export
	}

	overlays := make(map[string]*typ.Function, len(results))
	for _, fr := range results {
		field, ok := exportFieldNameFromSymbolName(fr.Name)
		if !ok {
			continue
		}
		existing := rec.GetField(field)
		if existing == nil {
			continue
		}
		base := unwrap.Function(existing.Type)
		if base == nil {
			continue
		}
		if enriched := enrichExportFunctionType(base, fr.Result); enriched != nil && enriched != base {
			overlays[field] = enriched
		}
	}
	if len(overlays) == 0 {
		return export
	}
	return applyExportFunctionOverlays(export, overlays, 0)
}

// enrichExportFunctionType grafts the body-observed return vector and proven
// ErrorReturn correlation onto base when base lacks them. It returns base
// unchanged when the result carries no recoverable summary.
func enrichExportFunctionType(base *typ.Function, result *api.FuncResult) *typ.Function {
	if base == nil || result == nil || result.NarrowSynth == nil {
		return base
	}

	enriched := base
	if len(result.Evidence.Returns) > 0 {
		observed := abstractreturns.ObservedSummary(
			result.Graph,
			result.Evidence.Returns,
			deadPointFlow(result),
			result.NarrowSynth,
		)
		if withReturns := join.WithReturnsOrUnknown(enriched, observed); withReturns != nil {
			enriched = withReturns
		}
	}

	if !erreffect.HasErrorReturnLabel(enriched) {
		enriched = erreffect.AttachInferredErrorReturnSpec(
			enriched,
			result.Evidence,
			result.FlowSolution,
			result.NarrowSynth,
		)
	}
	return enriched
}

func deadPointFlow(result *api.FuncResult) api.FlowOps {
	if result == nil || result.FlowSolution == nil {
		return nil
	}
	return result.FlowSolution
}

// applyExportFunctionOverlays replaces the named function fields with their
// enriched signatures throughout the export type structure.
func applyExportFunctionOverlays(t typ.Type, overlays map[string]*typ.Function, depth int) typ.Type {
	if t == nil || len(overlays) == 0 || typ.DepthExceeded(depth) {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Record:
		return overlayExportRecord(v, overlays, depth)
	case *typ.Interface:
		return overlayExportInterface(v, overlays, depth)
	default:
		return t
	}
}

func overlayExportRecord(rec *typ.Record, overlays map[string]*typ.Function, depth int) typ.Type {
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		overlay, ok := overlays[field.Name]
		if !ok || overlay == nil {
			continue
		}
		base := unwrap.Function(field.Type)
		if base == nil || base == overlay {
			continue
		}
		fields[i].Type = overlay
		changed = true
	}
	if !changed {
		return rec
	}
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	for _, field := range fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	return builder.Build()
}

func overlayExportInterface(iface *typ.Interface, overlays map[string]*typ.Function, depth int) typ.Type {
	changed := false
	methods := make([]typ.Method, len(iface.Methods))
	for i, method := range iface.Methods {
		methods[i] = method
		overlay, ok := overlays[method.Name]
		if !ok || overlay == nil {
			continue
		}
		base := unwrap.Function(method.Type)
		if base == nil || base == overlay {
			continue
		}
		methods[i].Type = overlay
		changed = true
	}
	if !changed {
		return iface
	}
	return typ.NewInterface(iface.Name, methods)
}

// ResolveExportTypeNames resolves type names to concrete types for a given scope.
// This is exposed for tests that validate exported type definitions.
