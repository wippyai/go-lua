// Package exportmanifest publishes solved checker results into module manifests.
package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// FromProgramResult publishes the manifest evidence currently represented by
// the solved program result. It intentionally publishes only stable manifest
// sections with public read models: module export type, module-local type
// definitions, directly recovered function signatures, ambient globals, and
// framework callback protocols.
func FromProgramResult(path string, result program.Result) *manifest.Manifest {
	m := manifest.New(path)
	if export, ok := exportType(result); ok {
		m.SetExport(export)
	} else {
		m.SetExport(typ.Unknown)
	}
	publishTypeDefinitions(m, result.RootResult())
	publishFunctionSignatures(m, path, result)
	publishProvidedGlobals(m, result)
	publishCallbackProtocols(m, path, result)
	return m
}

func exportType(result program.Result) (typ.Type, bool) {
	root := result.RootResult()
	if root == nil {
		return nil, false
	}
	source, sourceOK := rootReturnType(root)
	summary, summaryOK := summaryReturnType(result, root)
	if sourceOK && hasRecordMembers(source) {
		if summaryOK {
			if merged, ok := mergeRecordMembers(summary, source); ok {
				return merged, true
			}
		}
		return source, true
	}
	if summaryOK {
		return summary, true
	}
	if sourceOK {
		return source, true
	}
	return nil, false
}

func summaryReturnType(result program.Result, root *body.Result) (typ.Type, bool) {
	summary, ok := result.Snapshot().Read(result.RootKey())
	if !ok {
		return nil, false
	}
	if len(summary.Returns) == 0 {
		return nil, false
	}
	t, ok := typevalue.TypeOf(root.Registry(), summary.Returns[0])
	if !ok || typ.IsUnknown(t) {
		return nil, false
	}
	return t, true
}

func hasRecordMembers(t typ.Type) bool {
	switch t := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return len(t.Fields) > 0 || len(t.StaticMembers) > 0
	case *typ.Union:
		for _, member := range t.Members {
			if hasRecordMembers(member) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func mergeRecordMembers(summary, source typ.Type) (typ.Type, bool) {
	summaryRecord, ok := unwrap.Annotated(summary).(*typ.Record)
	if !ok {
		return nil, false
	}
	sourceRecord, ok := unwrap.Annotated(source).(*typ.Record)
	if !ok {
		return nil, false
	}
	fields := mergeFields(summaryRecord.Fields, sourceRecord.Fields)
	staticMembers := mergeStaticMembers(summaryRecord.StaticMembers, sourceRecord.StaticMembers)
	parts := typ.RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     preferredType(sourceRecord.Metatable, summaryRecord.Metatable),
		MapKey:        preferredType(sourceRecord.MapKey, summaryRecord.MapKey),
		MapValue:      preferredType(sourceRecord.MapValue, summaryRecord.MapValue),
		Open:          summaryRecord.Open || sourceRecord.Open,
	}
	return typetable.RebuildRecord(parts), true
}

func mergeFields(summary, source []typ.Field) []typ.Field {
	if len(summary) == 0 {
		return append([]typ.Field(nil), source...)
	}
	byName := make(map[string]typ.Field, len(summary)+len(source))
	for _, field := range summary {
		byName[field.Name] = field
	}
	for _, field := range source {
		byName[field.Name] = mergeField(byName[field.Name], field)
	}
	out := make([]typ.Field, 0, len(byName))
	for _, field := range byName {
		out = append(out, field)
	}
	return out
}

func mergeField(existing, next typ.Field) typ.Field {
	if existing.Name == "" {
		return next
	}
	return typ.Field{
		Name:     existing.Name,
		Type:     mergeManifestMemberType(existing.Type, next.Type),
		Optional: existing.Optional || next.Optional,
		Readonly: existing.Readonly && next.Readonly,
	}
}

func mergeStaticMembers(summary, source []typ.StaticMember) []typ.StaticMember {
	if len(summary) == 0 {
		return append([]typ.StaticMember(nil), source...)
	}
	byKey := make(map[staticMemberKey]typ.StaticMember, len(summary)+len(source))
	for _, member := range summary {
		byKey[keyForStaticMember(member)] = member
	}
	for _, member := range source {
		key := keyForStaticMember(member)
		byKey[key] = mergeStaticMember(byKey[key], member)
	}
	out := make([]typ.StaticMember, 0, len(byKey))
	for _, member := range byKey {
		out = append(out, member)
	}
	return out
}

func mergeStaticMember(existing, next typ.StaticMember) typ.StaticMember {
	if existing.Kind == 0 {
		return next
	}
	return typ.StaticMember{
		Kind:     existing.Kind,
		Name:     existing.Name,
		Index:    existing.Index,
		Type:     mergeManifestMemberType(existing.Type, next.Type),
		Optional: existing.Optional || next.Optional,
		Readonly: existing.Readonly && next.Readonly,
	}
}

func mergeManifestMemberType(existing, next typ.Type) typ.Type {
	if existing == nil {
		return next
	}
	if next == nil || typ.TypeEquals(existing, next) {
		return existing
	}
	if subtype.IsSubtype(next, existing) {
		return next
	}
	if subtype.IsSubtype(existing, next) {
		return existing
	}
	return normalize.UnionForEvidence(existing, next)
}

type staticMemberKey struct {
	kind  typ.StaticMemberKind
	name  string
	index int64
}

func keyForStaticMember(member typ.StaticMember) staticMemberKey {
	return staticMemberKey{
		kind:  member.Kind,
		name:  member.Name,
		index: member.Index,
	}
}

func preferredType(primary, fallback typ.Type) typ.Type {
	if primary != nil {
		return primary
	}
	return fallback
}

func rootReturnType(result *body.Result) (typ.Type, bool) {
	if result == nil || result.Graph() == nil {
		return nil, false
	}
	var candidates []typ.Type
	for _, point := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(point)
		if !ok {
			continue
		}
		if len(fact.Sources) == 0 {
			continue
		}
		t, ok := sourceType(result, point, fact.Sources[0])
		if !ok {
			continue
		}
		candidates = append(candidates, t)
	}
	return normalize.UnionType(candidates)
}
