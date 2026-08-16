package channelselect

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func TestResultValueTypeBuildsSelectUnion(t *testing.T) {
	got, ok := ResultValueTypeWithDefault("select-1", []ResultCase{
		{Index: 0, Payload: typ.String},
		{Index: 1, Payload: typ.Number},
	}, false)
	if !ok {
		t.Fatal("ResultValueType returned no type")
	}
	union, ok := unwrap.Alias(unwrap.Annotations(got)).(*typ.Union)
	if !ok {
		t.Fatalf("ResultValueType = %T, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("union members = %d, want 2", len(union.Members))
	}
	if !CaseTypeMatches(union.Members[0], "select-1", 0) && !CaseTypeMatches(union.Members[1], "select-1", 0) {
		t.Fatalf("union missing select-1 case 0 marker: %v", union.Members)
	}
}

func TestResultValueTypeWithDefaultIncludesDefaultMember(t *testing.T) {
	got, ok := ResultValueTypeWithDefault("select-default", []ResultCase{
		{Index: 0, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}
	union, ok := unwrap.Alias(unwrap.Annotations(got)).(*typ.Union)
	if !ok {
		t.Fatalf("ResultValueTypeWithDefault = %T, want union", got)
	}
	if len(union.Members) != 2 {
		t.Fatalf("union members = %d, want explicit case plus default", len(union.Members))
	}
	if !ResultHasSelectID(got, "select-default") {
		t.Fatalf("result type missing select-default marker: %v", got)
	}
	if _, ok := ResultCaseTypeFromValue(got, "select-default", DefaultCaseIndex); !ok {
		t.Fatalf("result type missing default case marker: %v", got)
	}
}

func TestResultValueTypeIgnoresExplicitDefaultSentinelCase(t *testing.T) {
	if got, ok := ResultValueTypeWithDefault("select-sentinel", []ResultCase{
		{Index: DefaultCaseIndex, Payload: typ.String},
	}, false); ok || got != nil {
		t.Fatalf("ResultValueTypeWithDefault(explicit sentinel only) = %v, %v; want nil, false", got, ok)
	}

	got, ok := ResultValueTypeWithDefault("select-sentinel", []ResultCase{
		{Index: DefaultCaseIndex, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault(default plus sentinel) returned no type")
	}
	caseType, ok := ResultCaseTypeFromValue(got, "select-sentinel", DefaultCaseIndex)
	if !ok {
		t.Fatalf("result type missing default member after explicit sentinel skip: %v", got)
	}
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		t.Fatalf("default case = %T, want record", caseType)
	}
	value := record.GetField(ResultValueField)
	if value == nil || value.Type != typ.Nil {
		t.Fatalf("default case value = %v, want nil payload", value)
	}
}

func TestResultCaseTypeIncludesRuntimeStatusFields(t *testing.T) {
	caseType := ResultCaseType("select-status", 0, typ.String)
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		t.Fatalf("case type = %T, want record", caseType)
	}
	okField := record.GetField("ok")
	if okField == nil || !typ.TypeEquals(okField.Type, typ.Boolean) {
		t.Fatalf("ok field = %v, want boolean", okField)
	}
	defaultField := record.GetField("default")
	if defaultField == nil || !typ.TypeEquals(defaultField.Type, typ.Nil) {
		t.Fatalf("non-default default field = %v, want nil", defaultField)
	}
}

func TestResultValueTypeWithDefaultIncludesRuntimeDefaultFields(t *testing.T) {
	result, ok := ResultValueTypeWithDefault("select-default-status", []ResultCase{
		{Index: 0, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}
	caseType, ok := ResultCaseTypeFromValue(result, "select-default-status", DefaultCaseIndex)
	if !ok {
		t.Fatalf("result type missing default member: %v", result)
	}
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		t.Fatalf("default case type = %T, want record", caseType)
	}
	okField := record.GetField("ok")
	if okField == nil || !typ.TypeEquals(okField.Type, typ.Boolean) {
		t.Fatalf("default ok field = %v, want boolean", okField)
	}
	defaultField := record.GetField("default")
	if defaultField == nil || !typ.TypeEquals(defaultField.Type, typ.LiteralBool(true)) {
		t.Fatalf("default field = %v, want true", defaultField)
	}
}

func TestResultCaseTypeFromValueMatchesUnionMembers(t *testing.T) {
	caseType := ResultCaseType("select-2", 7, typetable.NewRecord().Field("ok", typ.Boolean).Build())
	union := typeexpr.Union(caseType, typ.String)

	got, ok := ResultCaseTypeFromValue(union, "select-2", 7)
	if !ok {
		t.Fatal("ResultCaseTypeFromValue returned no match")
	}
	if !typ.TypeEquals(got, caseType) {
		t.Fatalf("matched type = %v, want %v", got, caseType)
	}
}

func TestResultWithoutCasePreservesDefaultMember(t *testing.T) {
	result, ok := ResultValueTypeWithDefault("select-remove", []ResultCase{
		{Index: 0, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}

	got, ok := ResultWithoutCase(result, "select-remove", 0)
	if !ok {
		t.Fatal("ResultWithoutCase returned no type")
	}
	if !CaseTypeMatches(got, "select-remove", DefaultCaseIndex) {
		t.Fatalf("ResultWithoutCase = %v, want default member", got)
	}
}

func TestResultWithoutCaseRefusesDefaultSentinel(t *testing.T) {
	result, ok := ResultValueTypeWithDefault("select-remove-default", []ResultCase{
		{Index: 0, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}

	if got, ok := ResultWithoutCase(result, "select-remove-default", DefaultCaseIndex); ok || got != nil {
		t.Fatalf("ResultWithoutCase(default sentinel) = %v, %v; want nil, false", got, ok)
	}
	if _, ok := ResultCaseTypeFromValue(result, "select-remove-default", DefaultCaseIndex); !ok {
		t.Fatalf("default member was not present before removal attempt: %v", result)
	}
}

func TestResultWithoutCaseNoDefaultCanBecomeNever(t *testing.T) {
	result, ok := ResultValueTypeWithDefault("select-remove", []ResultCase{
		{Index: 0, Payload: typ.String},
	}, false)
	if !ok {
		t.Fatal("ResultValueType returned no type")
	}

	got, ok := ResultWithoutCase(result, "select-remove", 0)
	if !ok {
		t.Fatal("ResultWithoutCase returned no type")
	}
	if !typ.IsNever(got) {
		t.Fatalf("ResultWithoutCase = %v, want never", got)
	}
}

func TestCaseTypeMatchesRejectsMalformedMarker(t *testing.T) {
	malformed := typetable.NewRecord().
		Field(ResultChannelField, typetable.NewRecord().
			Field(selectIDField, typ.LiteralString("select-bad")).
			Field(caseIndexField, typ.LiteralString("not-an-index")).
			Build()).
		Field(ResultValueField, typ.String).
		Build()

	if CaseTypeMatches(malformed, "select-bad", 0) {
		t.Fatal("CaseTypeMatches accepted marker with non-integer case index")
	}
}
