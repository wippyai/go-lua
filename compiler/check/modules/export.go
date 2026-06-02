package modules

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/exportkey"
	interprocfields "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type exportFieldTypes map[interprocfields.FieldKey]typ.Type
type exportFieldSymbols map[interprocfields.FieldKey]cfg.SymbolID
type exportFunctionOverlays map[exportkey.MemberPathKey]exportFunctionOverlay

type exportFunctionOverlay struct {
	path exportkey.MemberPath
	fn   *typ.Function
}

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
// exact static bracket members plus the declared map component
// (Record{["key"]: V, ..., [string]: V}). A literal-key read then resolves to the
// present static member (non-optional) while an absent/unknown key still falls to
// the map component (V?), so the read of a key the producer never wrote stays
// soundly optional.
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

	overlays := make(exportFieldTypes)
	for _, fieldKey := range interprocfields.SortedFieldKeys(fieldSources) {
		fieldName, ok := interprocfields.FieldKeyStringKey(fieldKey)
		if !ok {
			continue
		}
		sourceSym := fieldSources[fieldKey]
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
		overlays[fieldKey] = buildPopulatedMapRecord(field.Type, declaredMap, populated)
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
func exportFieldSourceSymbols(result *api.FuncResult, rec *typ.Record) exportFieldSymbols {
	out := make(exportFieldSymbols)
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
		fieldKey, ok := interprocfields.FieldKeyFromName(fieldName)
		if !ok {
			continue
		}
		if len(info.SourceSymbols) != 1 {
			continue
		}
		sourceSym := info.SourceSymbols[0]
		if sourceSym == 0 {
			continue
		}
		out[fieldKey] = sourceSym
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
func populatedStringKeyWrites(result *api.FuncResult, sourceSym cfg.SymbolID, declaredMap *typ.Map) exportFieldTypes {
	populated := make(exportFieldTypes)
	for _, asg := range result.Evidence.Assignments {
		info := asg.Info
		if info == nil || len(info.Targets) != 1 || len(info.Sources) != 1 {
			continue
		}
		target := info.Targets[0]
		if target.Kind != cfg.TargetIndex || target.BaseSymbol != sourceSym || len(target.FieldPath) != 0 {
			continue
		}
		keyExpr, ok := target.Key.(*ast.StringExpr)
		if !ok {
			continue
		}
		key := keyExpr.Value
		if !subtype.IsSubtype(typ.LiteralString(key), declaredMap.Key) {
			continue
		}
		fieldKey, ok := interprocfields.FieldKeyFromName(key)
		if !ok {
			continue
		}
		value := result.NarrowSynth.TypeOf(info.Sources[0], asg.Point)
		if value == nil || typ.IsAbsentOrUnknown(value) {
			continue
		}
		if !subtype.IsSubtype(value, declaredMap.Value) {
			continue
		}
		if existing, ok := populated[fieldKey]; ok {
			populated[fieldKey] = typ.NewUnion(existing, value)
			continue
		}
		populated[fieldKey] = value
	}
	return populated
}

// buildPopulatedMapRecord rebuilds a declared string-keyed map field as a record
// carrying the populated literal keys as exact bracket members plus the declared
// map component. When the declared field is an alias of the map, the rebuilt
// record is re-wrapped in the same alias so the importer still sees the named type.
func buildPopulatedMapRecord(declared typ.Type, declaredMap *typ.Map, populated exportFieldTypes) typ.Type {
	builder := typ.NewRecord().MapComponent(declaredMap.Key, declaredMap.Value)
	for _, fieldKey := range interprocfields.SortedFieldKeys(populated) {
		name, ok := interprocfields.FieldKeyStringKey(fieldKey)
		if !ok {
			continue
		}
		builder.StaticStringIndex(name, populated[fieldKey])
	}
	record := builder.Build()
	if alias, ok := declared.(*typ.Alias); ok {
		return typ.NewAlias(alias.Name, record)
	}
	return record
}

// overlayExportFieldsRecord replaces the named fields of the module export record
// with their rebuilt populated-map records, re-wrapping the original export alias
// when the export type was an alias of the record.
func overlayExportFieldsRecord(export typ.Type, rec *typ.Record, overlays exportFieldTypes) typ.Type {
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		fieldKey, keyOK := interprocfields.FieldKeyFromName(field.Name)
		if !keyOK {
			continue
		}
		overlay, ok := overlays[fieldKey]
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
	for _, member := range rec.StaticMembers {
		builder.AddStaticMember(member)
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
	// TargetPath is the CFG's structural function-definition path.
	TargetPath constraint.Path
	// Name is the function definition's source name (e.g. "M.request").
	// It is a compatibility fallback for direct local/global function names only.
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

	overlays := make(exportFunctionOverlays, len(results))
	for _, fr := range results {
		path, ok := exportFunctionResultPath(fr)
		if !ok {
			continue
		}
		base := exportFunctionMemberType(rec, path)
		if base == nil {
			continue
		}
		if enriched := enrichExportFunctionType(base, fr.Result); enriched != nil && enriched != base {
			overlays[path.Key()] = exportFunctionOverlay{path: path, fn: enriched}
		}
	}
	if len(overlays) == 0 {
		return export
	}
	return applyExportFunctionOverlays(export, overlays)
}

func exportFunctionResultPath(fr ExportFunctionResult) (exportkey.MemberPath, bool) {
	if path, ok := exportkey.MemberPathFromTargetPath("", fr.TargetPath); ok {
		return path, true
	}
	if fr.Name == "" || strings.Contains(fr.Name, ".") {
		return exportkey.MemberPath{}, false
	}
	key, ok := interprocfields.FieldKeyFromName(fr.Name)
	if !ok {
		return exportkey.MemberPath{}, false
	}
	return exportkey.NewMemberPath([]interprocfields.FieldKey{key})
}

func exportFunctionMemberType(t typ.Type, path exportkey.MemberPath) *typ.Function {
	member, ok := exportMemberTypeAtPath(t, path.Segments(), 0)
	if !ok {
		return nil
	}
	return unwrap.Function(member)
}

func exportMemberTypeAtPath(t typ.Type, path []interprocfields.FieldKey, depth int) (typ.Type, bool) {
	if t == nil || len(path) == 0 || typ.DepthExceeded(depth) {
		return nil, false
	}
	base := unwrap.Alias(t)
	var member typ.Type
	var ok bool
	switch v := base.(type) {
	case *typ.Record:
		member, ok = exportRecordMemberType(v, path[0])
	case *typ.Interface:
		member, ok = exportInterfaceMemberType(v, path[0])
	default:
		return nil, false
	}
	if !ok || member == nil {
		return nil, false
	}
	if len(path) == 1 {
		return member, true
	}
	return exportMemberTypeAtPath(member, path[1:], depth+1)
}

func exportRecordMemberType(rec *typ.Record, key interprocfields.FieldKey) (typ.Type, bool) {
	if rec == nil {
		return nil, false
	}
	switch key.Kind {
	case constraint.SegmentField:
		field := rec.GetField(key.Name)
		if field == nil {
			return nil, false
		}
		return field.Type, true
	case constraint.SegmentIndexString:
		member := rec.GetStaticStringIndex(key.Name)
		if member == nil {
			return nil, false
		}
		return member.Type, true
	case constraint.SegmentIndexInt:
		member := rec.GetStaticIntIndex(int64(key.Index))
		if member == nil {
			return nil, false
		}
		return member.Type, true
	default:
		return nil, false
	}
}

func exportInterfaceMemberType(iface *typ.Interface, key interprocfields.FieldKey) (typ.Type, bool) {
	if iface == nil {
		return nil, false
	}
	name, ok := interprocfields.FieldKeyStringKey(key)
	if !ok {
		return nil, false
	}
	for _, method := range iface.Methods {
		if method.Name == name {
			return method.Type, true
		}
	}
	return nil, false
}

// enrichExportFunctionType grafts the body-observed return vector and proven
// ErrorReturn correlation onto base when base lacks them. It returns base
// unchanged when the result carries no recoverable summary.
func enrichExportFunctionType(base *typ.Function, result *api.FuncResult) *typ.Function {
	if base == nil || result == nil {
		return base
	}

	enriched := base
	if result.NarrowSynth != nil && len(result.Evidence.Returns) > 0 {
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
		enriched = attachExportReturnRelations(enriched, result.ReturnRelations)
	}
	if result.NarrowSynth != nil && !erreffect.HasErrorReturnLabel(enriched) {
		enriched = erreffect.AttachInferredErrorReturnSpec(
			enriched,
			result.Evidence,
			deadPointFlow(result),
			result.NarrowSynth,
		)
	}
	return enriched
}

func attachExportReturnRelations(fn *typ.Function, rels flow.ReturnRelations) *typ.Function {
	if fn == nil {
		return fn
	}
	for _, rel := range rels.ErrorReturns() {
		return erreffect.AttachErrorReturnSpec(fn, rel.ValueIndex, rel.ErrorIndex)
	}
	return fn
}

func deadPointFlow(result *api.FuncResult) api.FlowOps {
	if result == nil {
		return nil
	}
	return result.SolvedFlow()
}

// applyExportFunctionOverlays replaces exactly the exported function members
// identified by their structural export paths. It intentionally does not recurse
// by leaf name: `M.run` and `M.api.run` are distinct paths.
func applyExportFunctionOverlays(t typ.Type, overlays exportFunctionOverlays) typ.Type {
	if t == nil || len(overlays) == 0 {
		return t
	}
	out := t
	for _, overlay := range sortedExportFunctionOverlays(overlays) {
		out = applyExportFunctionOverlay(out, overlay.path.Segments(), overlay.fn, 0)
	}
	return out
}

func sortedExportFunctionOverlays(overlays exportFunctionOverlays) []exportFunctionOverlay {
	keys := make([]exportkey.MemberPathKey, 0, len(overlays))
	for key := range overlays {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i]) < string(keys[j])
	})
	out := make([]exportFunctionOverlay, 0, len(keys))
	for _, key := range keys {
		out = append(out, overlays[key])
	}
	return out
}

func applyExportFunctionOverlay(t typ.Type, path []interprocfields.FieldKey, fn *typ.Function, depth int) typ.Type {
	if t == nil || len(path) == 0 || fn == nil || typ.DepthExceeded(depth) {
		return t
	}
	base := unwrap.Alias(t)
	var out typ.Type
	switch v := base.(type) {
	case *typ.Record:
		out = overlayExportRecordPath(v, path, fn, depth+1)
	case *typ.Interface:
		out = overlayExportInterfacePath(v, path, fn)
	default:
		return t
	}
	if out == base {
		return t
	}
	if alias, ok := t.(*typ.Alias); ok {
		return typ.NewAlias(alias.Name, out)
	}
	return out
}

func overlayExportRecordPath(rec *typ.Record, path []interprocfields.FieldKey, fn *typ.Function, depth int) typ.Type {
	if rec == nil || len(path) == 0 || fn == nil {
		return rec
	}
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		fieldKey, keyOK := interprocfields.FieldKeyFromName(field.Name)
		if !keyOK || fieldKey != path[0] {
			continue
		}
		next := overlayExportMemberType(field.Type, path[1:], fn, depth+1)
		if exportOverlayTypeUnchanged(next, field.Type) {
			continue
		}
		fields[i].Type = next
		changed = true
	}
	staticMembers := make([]typ.StaticMember, len(rec.StaticMembers))
	for i, member := range rec.StaticMembers {
		staticMembers[i] = member
		memberKey, keyOK := exportStaticMemberKey(member)
		if !keyOK || memberKey != path[0] {
			continue
		}
		next := overlayExportMemberType(member.Type, path[1:], fn, depth+1)
		if exportOverlayTypeUnchanged(next, member.Type) {
			continue
		}
		staticMembers[i].Type = next
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
	for _, member := range staticMembers {
		builder.AddStaticMember(member)
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

func exportOverlayTypeUnchanged(next, current typ.Type) bool {
	if next == current {
		return true
	}
	if unwrap.Function(next) != nil || unwrap.Function(current) != nil {
		return false
	}
	return typ.TypeEquals(next, current)
}

func overlayExportMemberType(current typ.Type, rest []interprocfields.FieldKey, fn *typ.Function, depth int) typ.Type {
	if len(rest) == 0 {
		if unwrap.Function(current) == nil {
			return current
		}
		return fn
	}
	return applyExportFunctionOverlay(current, rest, fn, depth+1)
}

func exportStaticMemberKey(member typ.StaticMember) (interprocfields.FieldKey, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return interprocfields.FieldKey{Kind: constraint.SegmentIndexString, Name: member.Name}, true
	case typ.StaticMemberIntIndex:
		return interprocfields.FieldKey{Kind: constraint.SegmentIndexInt, Index: int(member.Index)}, true
	default:
		return interprocfields.FieldKey{}, false
	}
}

func overlayExportInterfacePath(iface *typ.Interface, path []interprocfields.FieldKey, fn *typ.Function) typ.Type {
	if iface == nil || len(path) != 1 || fn == nil {
		return iface
	}
	name, ok := interprocfields.FieldKeyStringKey(path[0])
	if !ok {
		return iface
	}
	changed := false
	methods := make([]typ.Method, len(iface.Methods))
	for i, method := range iface.Methods {
		methods[i] = method
		if method.Name != name || unwrap.Function(method.Type) == nil || method.Type == fn {
			continue
		}
		methods[i].Type = fn
		changed = true
	}
	if !changed {
		return iface
	}
	return typ.NewInterface(iface.Name, methods)
}

// ResolveExportTypeNames resolves type names to concrete types for a given scope.
// This is exposed for tests that validate exported type definitions.
