// Package channelselect owns pure channel-select result type schema.
package channelselect

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const (
	ResultChannelField = "channel"
	ResultValueField   = "value"
	DefaultCaseIndex   = -1
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

// ResultDefaultType builds the residual result member for a select default arm.
func ResultDefaultType(selectID string) typ.Type {
	return ResultCaseType(selectID, DefaultCaseIndex, typ.Nil)
}

// ResultValueTypeWithDefault builds the internal select result union from case
// payloads plus an optional default arm.
func ResultValueTypeWithDefault(selectID string, cases []ResultCase, hasDefault bool) (typ.Type, bool) {
	if len(cases) == 0 && !hasDefault {
		return nil, false
	}
	caseTypes := make([]typ.Type, 0, len(cases)+1)
	for _, c := range cases {
		caseTypes = append(caseTypes, ResultCaseType(selectID, c.Index, c.Payload))
	}
	if hasDefault {
		caseTypes = append(caseTypes, ResultDefaultType(selectID))
	}
	return normalize.UnionForEvidence(caseTypes...), true
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

// ResultHasSelectID reports whether resultType contains any member for selectID.
func ResultHasSelectID(resultType typ.Type, selectID string) bool {
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		for _, member := range union.Members {
			if resultCaseHasSelectID(member, selectID) {
				return true
			}
		}
		return false
	}
	return resultCaseHasSelectID(resultType, selectID)
}

func resultCaseHasSelectID(caseType typ.Type, selectID string) bool {
	channelType, ok := ResultChannelFieldType(caseType)
	if !ok {
		return false
	}
	got, _, ok := CaseMarker(channelType)
	return ok && got == selectID
}

// ResultWithoutCase removes one explicit receive case from resultType. Default
// members are preserved, so a default-capable select does not collapse to never
// just because all explicit receive cases were excluded.
func ResultWithoutCase(resultType typ.Type, selectID string, index int) (typ.Type, bool) {
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		kept := make([]typ.Type, 0, len(union.Members))
		removed := false
		for _, member := range union.Members {
			if CaseTypeMatches(member, selectID, index) {
				removed = true
				continue
			}
			kept = append(kept, member)
		}
		if !removed {
			return nil, false
		}
		if len(kept) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(kept...), true
	}
	if CaseTypeMatches(resultType, selectID, index) {
		return typ.Never, true
	}
	return nil, false
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
