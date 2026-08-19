package channelselect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/ambient"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

func TestResultValueTypeBuildsSelectUnion(t *testing.T) {
	got, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
		{Index: 1, Channel: typ.Number, Payload: typ.Number},
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
	first := ResultCaseType(typ.String, typ.String)
	second := ResultCaseType(typ.Number, typ.Number)
	if !typ.TypeEquals(union.Members[0], first) && !typ.TypeEquals(union.Members[1], first) {
		t.Fatalf("union missing first receive case: %v", union.Members)
	}
	if !typ.TypeEquals(union.Members[0], second) && !typ.TypeEquals(union.Members[1], second) {
		t.Fatalf("union missing second receive case: %v", union.Members)
	}
}

func TestResultValueTypeWithDefaultIncludesDefaultMember(t *testing.T) {
	got, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
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
	if _, ok := ResultCaseTypeFromValue(got, resultDefaultType()); !ok {
		t.Fatalf("result type missing default case: %v", got)
	}
}

func TestResultValueTypeIgnoresExplicitDefaultSentinelCase(t *testing.T) {
	if got, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: DefaultCaseIndex, Channel: typ.String, Payload: typ.String},
	}, false); ok || got != nil {
		t.Fatalf("ResultValueTypeWithDefault(explicit sentinel only) = %v, %v; want nil, false", got, ok)
	}

	got, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: DefaultCaseIndex, Channel: typ.String, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault(default plus sentinel) returned no type")
	}
	caseType, ok := ResultCaseTypeFromValue(got, resultDefaultType())
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
	caseType := ResultCaseType(typ.String, typ.String)
	record, ok := unwrap.Alias(unwrap.Annotations(caseType)).(*typ.Record)
	if !ok {
		t.Fatalf("case type = %T, want record", caseType)
	}
	if record.GetField("__channel_select_id") != nil || record.GetField("__channel_select_case_index") != nil {
		t.Fatal("result case still carries phantom marker fields")
	}
	channel := record.GetField(ResultChannelField)
	if channel == nil || !typ.TypeEquals(channel.Type, typ.String) {
		t.Fatalf("channel field = %v, want the receive channel type", channel)
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
	result, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}
	caseType, ok := ResultCaseTypeFromValue(result, resultDefaultType())
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
	caseType := ResultCaseType(typ.String, typetable.NewRecord().Field("ok", typ.Boolean).Build())
	union := typeexpr.Union(caseType, typ.String)

	got, ok := ResultCaseTypeFromValue(union, caseType)
	if !ok {
		t.Fatal("ResultCaseTypeFromValue returned no match")
	}
	if !typ.TypeEquals(got, caseType) {
		t.Fatalf("matched type = %v, want %v", got, caseType)
	}
}

func TestResultWithoutCasePreservesDefaultMember(t *testing.T) {
	receive := ResultCaseType(typ.String, typ.String)
	result, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}

	got, ok := ResultWithoutCase(result, receive)
	if !ok {
		t.Fatal("ResultWithoutCase returned no type")
	}
	if _, ok := ResultCaseTypeFromValue(got, resultDefaultType()); !ok {
		t.Fatalf("ResultWithoutCase = %v, want default member", got)
	}
}

func TestResultWithoutCaseRefusesDefaultSentinel(t *testing.T) {
	result, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
	}, true)
	if !ok {
		t.Fatal("ResultValueTypeWithDefault returned no type")
	}

	if got, ok := ResultWithoutCase(result, resultDefaultType()); ok || got != nil {
		t.Fatalf("ResultWithoutCase(default sentinel) = %v, %v; want nil, false", got, ok)
	}
	if _, ok := ResultCaseTypeFromValue(result, resultDefaultType()); !ok {
		t.Fatalf("default member was not present before removal attempt: %v", result)
	}
}

func TestResultWithoutCaseNoDefaultCanBecomeNever(t *testing.T) {
	receive := ResultCaseType(typ.String, typ.String)
	result, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
	}, false)
	if !ok {
		t.Fatal("ResultValueType returned no type")
	}

	got, ok := ResultWithoutCase(result, receive)
	if !ok {
		t.Fatal("ResultWithoutCase returned no type")
	}
	if !typ.IsNever(got) {
		t.Fatalf("ResultWithoutCase = %v, want never", got)
	}
}

func TestFamilyKeyIsTheDeclaredStructureRow(t *testing.T) {
	if FamilyKey != "semantic/fact/channel-select-case" || Role != "fact/channel-select-case" {
		t.Fatalf("FamilyKey = %q Role = %q", FamilyKey, Role)
	}
}

func TestCaseFactIsSiteAndOrdinalNotTypeMarker(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("site"))
	other, otherOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("other"))
	if !siteOK || !otherOK {
		t.Fatal("site identities unavailable")
	}
	if CaseFactAvailable(CaseFact{}) || CaseFactAvailable(CaseFact{Site: site, Ordinal: DefaultCaseIndex}) {
		t.Fatal("empty or default-sentinel fact was admitted")
	}
	first := CaseFact{Site: site, Ordinal: 0}
	if !CaseFactAvailable(first) {
		t.Fatal("receive-arm fact was rejected")
	}
	id, idOK := CaseFactID(first)
	again, againOK := CaseFactID(first)
	second, secondOK := CaseFactID(CaseFact{Site: site, Ordinal: 1})
	foreign, foreignOK := CaseFactID(CaseFact{Site: other, Ordinal: 0})
	if !idOK || !againOK || !secondOK || !foreignOK {
		t.Fatal("case fact identity unavailable")
	}
	if id != again {
		t.Fatal("identical site and ordinal minted two identities")
	}
	if id == second || id == foreign {
		t.Fatal("case fact identity ignored site or ordinal")
	}
	if _, ok := CaseFactID(CaseFact{Site: site, Ordinal: DefaultCaseIndex}); ok {
		t.Fatal("default-sentinel fact minted an identity")
	}
}

func TestExhaustivenessReadsAcceptedFactsNotTypeShape(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	var facts CaseSet
	if !facts.Admit(CaseFact{Site: site, Ordinal: 0, Channel: typ.String, Payload: typ.String}) ||
		!facts.Admit(CaseFact{Site: site, Ordinal: 1, Channel: typ.Number, Payload: typ.Number}) ||
		!facts.Admit(CaseFact{Site: site, Ordinal: 2, Channel: typ.Boolean, Payload: typ.Boolean}) {
		t.Fatal("accepted facts refused")
	}
	if facts.Admit(CaseFact{Site: site, Ordinal: 0, Channel: typ.String, Payload: typ.Never}) {
		t.Fatal("duplicate site and ordinal was admitted")
	}
	missing := facts.MissingArms(site, []int{0, 1}, false)
	if len(missing) != 1 || missing[0].Ordinal != 2 {
		t.Fatalf("missing arms = %+v, want ordinal 2", missing)
	}
	if leftover := facts.MissingArms(site, []int{0, 1}, true); leftover != nil {
		t.Fatalf("default arm left missing %+v", leftover)
	}
	if _, ok := facts.Lookup(site, 2); !ok {
		t.Fatal("accepted ordinal 2 was not readable")
	}
}

func TestLookalikeTypeIsNotAnAcceptedSelectArm(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	var facts CaseSet
	if !facts.Admit(CaseFact{Site: site, Ordinal: 0, Channel: typ.String, Payload: typ.String}) {
		t.Fatal("accepted fact refused")
	}
	forged := typetable.NewRecord().
		Field(ResultChannelField, typetable.NewRecord().
			Field("__channel_select_id", typ.LiteralString("select")).
			Field("__channel_select_case_index", typ.LiteralInt(0)).
			Build()).
		Field(ResultValueField, typ.Boolean).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	live, ok := ResultValueTypeWithDefault([]ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
		{Index: 1, Channel: typ.Number, Payload: typ.Number},
	}, false)
	if !ok {
		t.Fatal("live select result unavailable")
	}
	mixed := typeexpr.Union(live, forged)
	fact, factOK := facts.Lookup(site, 0)
	got, removed := ResultWithoutFact(mixed, fact)
	if !factOK || !removed || typ.IsNever(got) {
		t.Fatal("accepted fact removed more than the live arm")
	}
	if _, ok := ResultCaseTypeFromValue(got, ResultCaseType(typ.Number, typ.Number)); !ok {
		t.Fatalf("live arm 1 was erased: %v", got)
	}
	if _, ok := ResultCaseTypeFromValue(got, forged); !ok {
		t.Fatalf("lookalike was treated as an accepted arm: %v", got)
	}
}

func TestUserAuthoredLookalikeRecordDoesNotEraseLiveSelectArm(t *testing.T) {
	live := ResultCaseType(typ.String, typ.String)
	forged := typetable.NewRecord().
		Field(ResultChannelField, typetable.NewRecord().
			Field("__channel_select_id", typ.LiteralString("site")).
			Field("__channel_select_case_index", typ.LiteralInt(0)).
			Build()).
		Field(ResultValueField, typ.Boolean).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	mixed := typeexpr.Union(live, forged)
	got, removed := ResultWithoutCase(mixed, live)
	if !removed {
		t.Fatal("ResultWithoutCase refused the live member")
	}
	if typ.IsNever(got) {
		t.Fatal("forged lookalike record erased the live select arm")
	}
	if !typ.TypeEquals(got, forged) {
		t.Fatalf("remaining type = %v, want the user-authored lookalike", got)
	}
}

func TestUserRecordIsNotAReceiveCase(t *testing.T) {
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	lookalike := typetable.NewRecord().
		Field(ResultChannelField, channel).
		Field(ResultValueField, typ.String).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	if _, ok := CaseFromType(lookalike); ok {
		t.Fatal("user record was decoded as a select case")
	}
	got, ok := CaseFromType(ReceiveCaseType(channel, typ.String))
	if !ok || !typ.TypeEquals(got.Channel, channel) || !typ.TypeEquals(got.Payload, typ.String) {
		t.Fatalf("CaseFromType(receive case) = %+v, %v", got, ok)
	}
	if _, ok := CaseFromType(typ.Instantiate(ambient.ChannelGeneric(), typ.String)); ok {
		t.Fatal("Channel instantiation was decoded as a select case")
	}
}

func TestSelectFromCasesKeepsSameTypeArmsAsDistinctFacts(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	result, facts, ok := SelectFromCases(site, []ResultCase{
		{Index: 0, Channel: channel, Payload: typ.String},
		{Index: 1, Channel: channel, Payload: typ.String},
	}, false)
	if !ok {
		t.Fatal("SelectFromCases refused same-type arms")
	}
	if record, isRecord := unwrap.Alias(unwrap.Annotations(result)).(*typ.Record); isRecord {
		if record.GetField("__channel_select_id") != nil || record.GetField("__channel_select_case_index") != nil {
			t.Fatal("select result carried marker fields")
		}
	}
	if _, ok := facts.Lookup(site, 0); !ok {
		t.Fatal("accepted ordinal 0 was not readable")
	}
	if _, ok := facts.Lookup(site, 1); !ok {
		t.Fatal("accepted ordinal 1 was not readable")
	}
	missing := facts.MissingArms(site, []int{0}, false)
	if len(missing) != 1 || missing[0].Ordinal != 1 {
		t.Fatalf("missing arms = %+v, want ordinal 1", missing)
	}
}

func TestCasesFromTableReadsConstructorTuple(t *testing.T) {
	events := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	timeout := typ.Instantiate(ambient.ChannelGeneric(), typ.Number)
	lookalike := typetable.NewRecord().
		Field(ResultChannelField, events).
		Field(ResultValueField, typ.String).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	table, ok := typetable.ConstructorType([]typetable.ConstructorEntry{
		{Path: []typetable.ConstructorKey{{Kind: typetable.ConstructorIntIndex, Index: 1}}, Type: ReceiveCaseType(events, typ.String)},
		{Path: []typetable.ConstructorKey{{Kind: typetable.ConstructorIntIndex, Index: 2}}, Type: lookalike},
		{Path: []typetable.ConstructorKey{{Kind: typetable.ConstructorIntIndex, Index: 3}}, Type: ReceiveCaseType(timeout, typ.Number)},
	})
	if !ok {
		t.Fatal("constructor table unavailable")
	}
	if _, isTuple := unwrap.Alias(table).(*typ.Tuple); !isTuple {
		t.Fatalf("array-only select argument = %T, want tuple", table)
	}
	cases, hasDefault, ok := CasesFromTable(table)
	if !ok {
		t.Fatal("CasesFromTable refused a constructor tuple")
	}
	if hasDefault {
		t.Fatal("array-only select argument was treated as having a default")
	}
	if len(cases) != 2 || cases[0].Index != 0 || cases[1].Index != 2 {
		t.Fatalf("cases = %+v, want ordinals 0 and 2", cases)
	}
	if !typ.TypeEquals(cases[0].Channel, events) || !typ.TypeEquals(cases[1].Channel, timeout) {
		t.Fatalf("decoded channels = %+v", cases)
	}
}

func TestCasesFromTableIgnoresUserLookalike(t *testing.T) {
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	other := typ.Instantiate(ambient.ChannelGeneric(), typ.Number)
	lookalike := typetable.NewRecord().
		Field(ResultChannelField, channel).
		Field(ResultValueField, typ.String).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	table := typetable.NewRecord().
		StaticIntIndex(1, ReceiveCaseType(channel, typ.String)).
		StaticIntIndex(2, lookalike).
		StaticIntIndex(3, ReceiveCaseType(other, typ.Number)).
		Field(ResultDefaultField, typ.LiteralBool(true)).
		Build()
	cases, hasDefault, ok := CasesFromTable(table)
	if !ok {
		t.Fatal("CasesFromTable refused a select argument table")
	}
	if !hasDefault {
		t.Fatal("literal default field was not a default arm")
	}
	if len(cases) != 2 || cases[0].Index != 0 || cases[1].Index != 2 {
		t.Fatalf("cases = %+v, want ordinals 0 and 2", cases)
	}
	if !typ.TypeEquals(cases[0].Channel, channel) || !typ.TypeEquals(cases[1].Channel, other) {
		t.Fatalf("decoded channels = %+v", cases)
	}
}

func TestSpecializeSelectAdmitsNominalCasesAndIgnoresLookalike(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	events := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	timeout := typ.Instantiate(ambient.ChannelGeneric(), typ.Number)
	lookalike := typetable.NewRecord().
		Field(ResultChannelField, events).
		Field(ResultValueField, typ.String).
		Field(ResultOKField, typ.Boolean).
		Field(ResultDefaultField, typ.Nil).
		Build()
	table := typetable.NewRecord().
		StaticIntIndex(1, ReceiveCaseType(events, typ.String)).
		StaticIntIndex(2, lookalike).
		StaticIntIndex(3, ReceiveCaseType(timeout, typ.Number)).
		Build()
	result, facts, ok := SpecializeSelect(site, []typ.Type{table})
	if !ok {
		t.Fatal("SpecializeSelect refused a table of nominal cases")
	}
	assertNoSelectMarkers(t, result)
	if _, ok := facts.Lookup(site, 0); !ok {
		t.Fatal("accepted ordinal 0 was not readable")
	}
	if _, ok := facts.Lookup(site, 1); ok {
		t.Fatal("lookalike table member was admitted as a case")
	}
	if _, ok := facts.Lookup(site, 2); !ok {
		t.Fatal("accepted ordinal 2 was not readable")
	}
	missing := facts.MissingArms(site, []int{0}, false)
	if len(missing) != 1 || missing[0].Ordinal != 2 {
		t.Fatalf("missing arms = %+v, want ordinal 2", missing)
	}
}

func TestSpecializeSelectRequiresSiteAndCases(t *testing.T) {
	if _, _, ok := SpecializeSelect(identity.ContentID{}, []typ.Type{typetable.NewRecord().Build()}); ok {
		t.Fatal("SpecializeSelect admitted an unavailable site")
	}
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	if _, _, ok := SpecializeSelect(site, nil); ok {
		t.Fatal("SpecializeSelect admitted a missing argument")
	}
	if _, _, ok := SpecializeSelect(site, []typ.Type{typ.String}); ok {
		t.Fatal("SpecializeSelect admitted a non-table argument")
	}
}

func TestIsSelectFunctionMatchesOnlyTheModuleSignature(t *testing.T) {
	if !IsSelectFunction(SelectFunction()) {
		t.Fatal("SelectFunction was not recognized")
	}
	other := typ.Func().Param("cases", typ.Any).Returns(typ.Any).Build()
	if IsSelectFunction(other) {
		t.Fatal("unrelated function was treated as channel.select")
	}
}

func TestModuleTypeIsNotAUserRecord(t *testing.T) {
	if !IsModuleType(ModuleType()) {
		t.Fatal("ModuleType was not recognized")
	}
	if IsModuleType(typetable.NewRecord().Field("select", typ.Any).Build()) {
		t.Fatal("user record was treated as the channel module")
	}
	if IsModuleType(typ.Instantiate(ambient.ChannelGeneric(), typ.String)) {
		t.Fatal("Channel instance was treated as the channel module")
	}
}

func TestAdmitCasesRefusesDuplicateOrdinal(t *testing.T) {
	site, siteOK := identity.DeriveContentID("test/channel-select-site/v1", []byte("select"))
	if !siteOK {
		t.Fatal("site identity unavailable")
	}
	var facts CaseSet
	if AdmitCases(&facts, site, []ResultCase{
		{Index: 0, Channel: typ.String, Payload: typ.String},
		{Index: 0, Channel: typ.Number, Payload: typ.Number},
	}) {
		t.Fatal("duplicate ordinal was admitted")
	}
}

func assertNoSelectMarkers(t *testing.T, result typ.Type) {
	t.Helper()
	switch typed := unwrap.Alias(unwrap.Annotations(result)).(type) {
	case *typ.Union:
		for _, member := range typed.Members {
			assertNoSelectMarkers(t, member)
		}
	case *typ.Record:
		if typed.GetField("__channel_select_id") != nil || typed.GetField("__channel_select_case_index") != nil {
			t.Fatalf("result %v carried marker fields", result)
		}
	}
}
