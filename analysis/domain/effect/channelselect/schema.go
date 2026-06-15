// Package channelselect owns pure channel-select result and payload type schema.
package channelselect

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
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

// ResultCase describes one internal select result union member.
type ResultCase struct {
	Index   int
	Payload typ.Type
}

// ResultCaseType builds one member of the internal select result union:
// { channel = marker, value = payload }.
func ResultCaseType(selectID string, index int, payload typ.Type) typ.Type {
	return typetable.NewRecord().
		Field(ResultChannelField, CaseMarkerType(selectID, index)).
		Field(ResultValueField, payload).
		Build()
}

// ResultValueType builds the internal select result union from case payloads.
func ResultValueType(selectID string, cases []ResultCase) (typ.Type, bool) {
	if len(cases) == 0 {
		return nil, false
	}
	caseTypes := make([]typ.Type, 0, len(cases))
	for _, c := range cases {
		caseTypes = append(caseTypes, ResultCaseType(selectID, c.Index, c.Payload))
	}
	return normalize.UnionForEvidence(caseTypes...), true
}

// ChannelPayloadType returns the payload carried by the ambient Channel<T> type.
func ChannelPayloadType(t typ.Type) (typ.Type, bool) {
	return ambient.ChannelPayloadType(t)
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

// ResultCaseTypeFromValue returns the matching select result case type, if any.
func ResultCaseTypeFromValue(resultType typ.Type, selectID string, index int) (typ.Type, bool) {
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		for _, member := range union.Members {
			if CaseTypeMatches(member, selectID, index) {
				return member, true
			}
		}
		return nil, false
	}
	if CaseTypeMatches(resultType, selectID, index) {
		return resultType, true
	}
	return nil, false
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

// ResultPathFromChannel returns the select result path for result.channel.
func ResultPathFromChannel(p pathdom.Path) (pathdom.Path, bool) {
	seg, ok := p.LastSegment()
	if !ok || seg.Kind != segment.SegmentField || seg.Name != ResultChannelField {
		return pathdom.Path{}, false
	}
	return p.Parent(), true
}
