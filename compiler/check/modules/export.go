package modules

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
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
	export = preservePopulatedMapKeys(export, result)
	return export
}

// preservePopulatedMapKeys recovers the provably-populated literal string keys of
// a module field whose value is a local declared as a string-keyed map ({[string]:
// V}) and populated by literal-string index writes. The declared map annotation
// erases those keys at export, collapsing the field to the bare map; a known-key
// read across the module boundary then observes the optional map value (V?) even
// though the key is statically present.
//
// The recovery rebuilds such a field as a record carrying the populated keys as
// concrete fields plus the declared map component (Record{key: V, ..., [string]:
// V}). A literal-key read then resolves to the present field (non-optional) while
// an absent/unknown key still falls to the map component (V?), so the read of a
// key the producer never wrote stays soundly optional.
func preservePopulatedMapKeys(export typ.Type, result *api.FuncResult) typ.Type {
	if export == nil || result == nil || result.NarrowSynth == nil || result.Graph == nil {
		return export
	}
	rec, ok := unwrap.Alias(export).(*typ.Record)
	if !ok || rec == nil {
		return export
	}
	if len(result.Evidence.Assignments) == 0 {
		return export
	}

	fieldSources := exportFieldSourceSymbols(result, rec)
	if len(fieldSources) == 0 {
		return export
	}

	overlays := make(map[string]typ.Type)
	for fieldName, sourceSym := range fieldSources {
		field := rec.GetField(fieldName)
		if field == nil {
			continue
		}
		declaredMap, ok := stringKeyedMapDecl(field.Type)
		if !ok {
			continue
		}
		populated := populatedStringKeyWrites(result, sourceSym, declaredMap)
		if len(populated) == 0 {
			continue
		}
		overlays[fieldName] = buildPopulatedMapRecord(field.Type, declaredMap, populated)
	}
	if len(overlays) == 0 {
		return export
	}
	return overlayExportFieldsRecord(export, rec, overlays)
}

// exportFieldSourceSymbols maps an exported record field name to the local symbol
// whose value was assigned into it (root.field = local). Only a direct identifier
// source feeds the map; a computed or call source is not a recoverable populated
// literal.
func exportFieldSourceSymbols(result *api.FuncResult, rec *typ.Record) map[string]cfg.SymbolID {
	out := make(map[string]cfg.SymbolID)
	for _, asg := range result.Evidence.Assignments {
		info := asg.Info
		if info == nil || len(info.Targets) != 1 || len(info.Sources) != 1 {
			continue
		}
		target := info.Targets[0]
		if target.Kind != cfg.TargetField || len(target.FieldPath) != 1 || target.BaseSymbol == 0 {
			continue
		}
		fieldName := target.FieldPath[0]
		if rec.GetField(fieldName) == nil {
			continue
		}
		if len(info.SourceSymbols) != 1 {
			continue
		}
		sourceSym := info.SourceSymbols[0]
		if sourceSym == 0 {
			continue
		}
		out[fieldName] = sourceSym
	}
	return out
}

// stringKeyedMapDecl returns the declared map when t is a map (directly or through
// an alias) whose key admits literal string keys. The literal keys recovered below
// must be subtypes of this key for the rebuild to be sound.
func stringKeyedMapDecl(t typ.Type) (*typ.Map, bool) {
	m, ok := unwrap.Alias(t).(*typ.Map)
	if !ok || m == nil || m.Key == nil || m.Value == nil {
		return nil, false
	}
	if !subtype.IsSubtype(typ.LiteralString("k"), m.Key) {
		return nil, false
	}
	return m, true
}

// populatedStringKeyWrites observes the value type of every literal-string index
// write into the source local (local["key"] = value), keyed by the literal key.
// A write whose key is not a sound literal string key for the declared map, or
// whose observed value is not a subtype of the declared map value, is skipped so
// the rebuilt record never widens the declared map contract.
func populatedStringKeyWrites(result *api.FuncResult, sourceSym cfg.SymbolID, declaredMap *typ.Map) map[string]typ.Type {
	populated := make(map[string]typ.Type)
	for _, asg := range result.Evidence.Assignments {
		info := asg.Info
		if info == nil || len(info.Targets) != 1 || len(info.Sources) != 1 {
			continue
		}
		target := info.Targets[0]
		if target.Kind != cfg.TargetField || target.BaseSymbol != sourceSym || len(target.FieldPath) != 1 {
			continue
		}
		key := target.FieldPath[0]
		if !subtype.IsSubtype(typ.LiteralString(key), declaredMap.Key) {
			continue
		}
		value := result.NarrowSynth.TypeOf(info.Sources[0], asg.Point)
		if value == nil || typ.IsAbsentOrUnknown(value) {
			continue
		}
		if !subtype.IsSubtype(value, declaredMap.Value) {
			continue
		}
		if existing, ok := populated[key]; ok {
			populated[key] = typ.NewUnion(existing, value)
			continue
		}
		populated[key] = value
	}
	return populated
}

// buildPopulatedMapRecord rebuilds a declared string-keyed map field as a record
// carrying the populated literal keys as concrete fields plus the declared map
// component. When the declared field is an alias of the map, the rebuilt record is
// re-wrapped in the same alias so the importer still sees the named type.
func buildPopulatedMapRecord(declared typ.Type, declaredMap *typ.Map, populated map[string]typ.Type) typ.Type {
	builder := typ.NewRecord().MapComponent(declaredMap.Key, declaredMap.Value)
	for _, key := range sortedKeys(populated) {
		builder.Field(key, populated[key])
	}
	record := builder.Build()
	if alias, ok := declared.(*typ.Alias); ok {
		return typ.NewAlias(alias.Name, record)
	}
	return record
}

func sortedKeys(m map[string]typ.Type) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// overlayExportFieldsRecord replaces the named fields of the module export record
// with their rebuilt populated-map records, re-wrapping the original export alias
// when the export type was an alias of the record.
func overlayExportFieldsRecord(export typ.Type, rec *typ.Record, overlays map[string]typ.Type) typ.Type {
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		overlay, ok := overlays[field.Name]
		if !ok || overlay == nil {
			continue
		}
		fields[i].Type = overlay
		changed = true
	}
	if !changed {
		return export
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
	rebuilt := builder.Build()
	if alias, ok := export.(*typ.Alias); ok {
		return typ.NewAlias(alias.Name, rebuilt)
	}
	return rebuilt
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
