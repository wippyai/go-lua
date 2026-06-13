// Package channelselect owns the internal select-result record schema.
package channelselect

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const (
	ResultChannelField = "channel"
	ResultValueField   = "value"
	selectIDField      = "__channel_select_id"
	caseIndexField     = "__channel_select_case_index"
)

// ResultCaseType builds one member of the internal select result union:
// { channel = marker, value = payload }.
func ResultCaseType(selectID string, index int, payload typ.Type) typ.Type {
	return typetable.NewRecord().
		Field(ResultChannelField, CaseMarkerType(selectID, index)).
		Field(ResultValueField, payload).
		Build()
}

// CaseMarkerType builds the opaque channel identity marker stored in a select
// result case's channel field.
func CaseMarkerType(selectID string, index int) typ.Type {
	return typetable.NewRecord().
		Field(selectIDField, typ.LiteralString(selectID)).
		Field(caseIndexField, typ.LiteralInt(int64(index))).
		Build()
}

// CaseTypeMatches reports whether caseType carries the marker for selectID/index.
func CaseTypeMatches(caseType typ.Type, selectID string, index int) bool {
	channelType, ok := ResultChannelFieldType(caseType)
	if !ok {
		return false
	}
	gotSelectID, gotIndex, ok := CaseMarker(channelType)
	return ok && gotSelectID == selectID && gotIndex == index
}

// ResultChannelFieldType returns the internal result case channel field type.
func ResultChannelFieldType(caseType typ.Type) (typ.Type, bool) {
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		return nil, false
	}
	field := record.GetField(ResultChannelField)
	if field == nil {
		return nil, false
	}
	return field.Type, true
}

// CaseMarker decodes a select result channel marker.
func CaseMarker(t typ.Type) (string, int, bool) {
	marker, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Record)
	if !ok {
		return "", 0, false
	}
	idField := marker.GetField(selectIDField)
	indexField := marker.GetField(caseIndexField)
	if idField == nil || indexField == nil {
		return "", 0, false
	}
	idLiteral, ok := unwrap.Annotations(idField.Type).(*typ.Literal)
	if !ok || idLiteral.Base != kind.String {
		return "", 0, false
	}
	indexLiteral, ok := unwrap.Annotations(indexField.Type).(*typ.Literal)
	if !ok || indexLiteral.Base != kind.Integer {
		return "", 0, false
	}
	id, ok := idLiteral.Value.(string)
	if !ok {
		return "", 0, false
	}
	index, ok := indexLiteral.Value.(int64)
	if !ok {
		return "", 0, false
	}
	return id, int(index), true
}

// ResultPathFromChannelField returns the select result path for result.channel.
func ResultPathFromChannelField(p pathdom.Path) (pathdom.Path, bool) {
	seg, ok := p.LastSegment()
	if !ok || seg.Kind != segment.SegmentField || seg.Name != ResultChannelField {
		return pathdom.Path{}, false
	}
	return p.Parent(), true
}
