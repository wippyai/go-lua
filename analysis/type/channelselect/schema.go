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
	ResultOKField      = "ok"
	ResultDefaultField = "default"
	DefaultCaseIndex   = -1
	selectIDField      = "__channel_select_id"
	caseIndexField     = "__channel_select_case_index"
)

// ResultCase describes one internal select result union member.
type ResultCase struct {
	Index   int
	Payload typ.Type
}

// ResultCaseType builds one member of the internal select result union. The
// channel field carries an internal marker for path-sensitive case narrowing;
// the public runtime fields mirror channel.select's result table shape.
func ResultCaseType(selectID string, index int, payload typ.Type) typ.Type {
	return typetable.NewRecord().
		Field(ResultChannelField, caseMarkerType(selectID, index)).
		Field(ResultValueField, payload).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
}

func resultDefaultType(selectID string) typ.Type {
	return typetable.NewRecord().
		Field(ResultChannelField, caseMarkerType(selectID, DefaultCaseIndex)).
		Field(ResultValueField, typ.Nil).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.LiteralBool(true)).
		Build()
}

// ResultValueTypeWithDefault builds the internal select result union from case
// payloads plus an optional default arm.
func ResultValueTypeWithDefault(selectID string, cases []ResultCase, hasDefault bool) (typ.Type, bool) {
	if len(cases) == 0 && !hasDefault {
		return nil, false
	}
	caseTypes := make([]typ.Type, 0, len(cases)+1)
	for _, c := range cases {
		if c.Index == DefaultCaseIndex {
			continue
		}
		caseTypes = append(caseTypes, ResultCaseType(selectID, c.Index, c.Payload))
	}
	if hasDefault {
		caseTypes = append(caseTypes, resultDefaultType(selectID))
	}
	if len(caseTypes) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(caseTypes...), true
}

func caseMarkerType(selectID string, index int) typ.Type {
	return typetable.NewRecord().
		Field(selectIDField, typ.LiteralString(selectID)).
		Field(caseIndexField, typ.LiteralInt(int64(index))).
		Build()
}

// CaseTypeMatches reports whether caseType carries the marker for selectID/index.
func CaseTypeMatches(caseType typ.Type, selectID string, index int) bool {
	channelType, ok := resultChannelFieldType(caseType)
	if !ok {
		return false
	}
	gotSelectID, gotIndex, ok := caseMarker(channelType)
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
	channelType, ok := resultChannelFieldType(caseType)
	if !ok {
		return false
	}
	got, _, ok := caseMarker(channelType)
	return ok && got == selectID
}

// ResultWithoutCase removes one explicit receive case from resultType. Default
// members are preserved, so a default-capable select does not collapse to never
// just because all explicit receive cases were excluded.
func ResultWithoutCase(resultType typ.Type, selectID string, index int) (typ.Type, bool) {
	if index == DefaultCaseIndex {
		return nil, false
	}
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

func resultChannelFieldType(caseType typ.Type) (typ.Type, bool) {
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

// ResultCasePayloadType returns the public payload field type from a select
// result union member.
func ResultCasePayloadType(caseType typ.Type) (typ.Type, bool) {
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		return nil, false
	}
	field := record.GetField(ResultValueField)
	if field == nil {
		return nil, false
	}
	return field.Type, true
}

func caseMarker(t typ.Type) (string, int, bool) {
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
	if !ok || idLiteral.Base() != kind.String {
		return "", 0, false
	}
	indexLiteral, ok := unwrap.Annotations(indexField.Type).(*typ.Literal)
	if !ok || indexLiteral.Base() != kind.Integer {
		return "", 0, false
	}
	id, ok := idLiteral.Value().(string)
	if !ok {
		return "", 0, false
	}
	index, ok := indexLiteral.Value().(int64)
	if !ok {
		return "", 0, false
	}
	return id, int(index), true
}
