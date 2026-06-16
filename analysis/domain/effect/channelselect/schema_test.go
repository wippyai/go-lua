package channelselect

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func TestResultValueTypeBuildsSelectUnion(t *testing.T) {
	got, ok := ResultValueType("select-1", []ResultCase{
		{Index: 0, Payload: typ.String},
		{Index: 1, Payload: typ.Number},
	})
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

func TestResultWithoutCaseNoDefaultCanBecomeNever(t *testing.T) {
	result, ok := ResultValueType("select-remove", []ResultCase{
		{Index: 0, Payload: typ.String},
	})
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

func TestResultPathFromChannel(t *testing.T) {
	p := pathdom.NewPath(17, "result").Field(ResultChannelField)
	got, ok := ResultPathFromChannel(p)
	if !ok {
		t.Fatal("ResultPathFromChannel returned false")
	}
	if got.Root != "result" || got.Symbol != 17 || len(got.Segments) != 0 {
		t.Fatalf("result path = %#v, want root result with no suffix", got)
	}
}
