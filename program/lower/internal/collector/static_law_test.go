package collector

import (
	"math"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestStaticFreezeResolvesOnlyThroughSourcePreimage(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:          1,
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyTypeLiteral:   1,
		keyspace.FamilyTypeRecord:    1,
		keyspace.FamilyTypeField:     1,
	}
	owner := New("static-law.lua", 0, bind.GlobalCensus{})
	rows := &owner.static
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	literal := keyspace.MakeTerm(keyspace.FamilyTypeLiteral, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	record := keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)
	if err := rows.Primitive(primitive, programstatic.PrimitiveString); err != nil {
		t.Fatal(err)
	}
	if !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "literal"}) {
		t.Fatal("literal exact admission failed")
	}
	if err := rows.LiteralString(literal, "literal"); err != nil {
		t.Fatal(err)
	}
	if !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "field"}) {
		t.Fatal("field exact admission failed")
	}
	if err := rows.Field(field, "field", primitive, false); err != nil {
		t.Fatal(err)
	}
	if err := rows.Record(record, []keyspace.Term{field}, false); err != nil {
		t.Fatal(err)
	}
	if len(owner.source.exact) != 2 {
		t.Fatalf("exact Source admissions = %d, want 2", len(owner.source.exact))
	}
	preimage := testStaticPreimage(t, counts, owner.source.exact)
	input, err := rows.freeze(preimage, counts)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if input.Types.Field[0].Key == 0 || input.Types.Literal[0].Exact == 0 {
		t.Fatalf("freeze did not resolve exact keys: field=%v literal=%v", input.Types.Field[0].Key, input.Types.Literal[0].Exact)
	}
	if _, err := programstatic.Build(input); err != nil {
		t.Fatalf("frozen Static input does not Build: %v", err)
	}
	// The input is detached from row-owned slices. This matters because the
	// lowerer reuses its scratch ranges after Source is claimed.
	input.Types.Record[0].Fields[0] = 0
	if rows.record[0].Fields[0] == 0 {
		t.Fatal("freeze retained a shared Record field slice")
	}
}

func TestStaticFreezeRejectsMissingPayloadAndNaN(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:        1,
		keyspace.FamilyTypeLiteral: 1,
	}
	owner := New("static-law.lua", 0, bind.GlobalCensus{})
	rows := &owner.static
	if err := rows.LiteralString(keyspace.MakeTerm(keyspace.FamilyTypeLiteral, 1), "missing"); err != nil {
		t.Fatal(err)
	}
	preimage := testStaticPreimage(t, counts, nil)
	if _, err := rows.freeze(preimage, counts); err == nil {
		t.Fatal("freeze accepted an exact payload absent from Source")
	}
	if err := rows.LiteralFloat(keyspace.MakeTerm(keyspace.FamilyTypeLiteral, 1), math.Float64bits(math.NaN())); err == nil {
		t.Fatal("LiteralFloat accepted NaN")
	}
}

func TestStaticRowsFillsAreOneShotAndClaimsCanonical(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:          1,
		keyspace.FamilyTypeAlias:     1,
		keyspace.FamilyTypeParam:     1,
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyValueClaim:    3,
	}
	owner := New("static-law.lua", 0, bind.GlobalCensus{})
	rows := &owner.static
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if err := rows.Primitive(primitive, programstatic.PrimitiveString); err != nil {
		t.Fatal(err)
	}
	if !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "A"}) {
		t.Fatal("alias exact admission failed")
	}
	if err := rows.AliasDeclare(alias, body, "A", mustCoordinate(t)); err != nil {
		t.Fatal(err)
	}
	if !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "T"}) {
		t.Fatal("type parameter exact admission failed")
	}
	if err := rows.TypeParamDeclare(param, alias, "T"); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeParamFill(param, primitive); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasParams(alias, []keyspace.Term{param}); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasParams(alias, []keyspace.Term{param}); err == nil {
		t.Fatal("AliasParams accepted duplicate fill")
	}
	if err := rows.AliasTarget(alias, primitive); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasTarget(alias, primitive); err == nil {
		t.Fatal("AliasTarget accepted duplicate fill")
	}
	// Sparse rows are not sorted by append convenience. Freeze must reject the
	// resulting noncanonical order instead of silently sorting a semantic row.
	rows.claims = []programstatic.ClaimTarget{
		{Claim: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: primitive},
		{Claim: keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), Target: primitive},
	}
	preimage := testStaticPreimage(t, counts, owner.source.exact)
	if _, err := rows.freeze(preimage, counts); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatal("freeze accepted noncanonical sparse Claim order")
	}
}

func TestStaticRowsDoNotOwnSourceAdmission(t *testing.T) {
	rows := &staticRows{}
	if err := rows.FieldRaw(
		keyspace.MakeTerm(keyspace.FamilyTypeField, 1),
		keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "A"},
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
		false,
	); err != nil {
		t.Fatalf("pure Static row rejected valid raw payload: %v", err)
	}
	if len(rows.field) != 1 {
		t.Fatalf("Static row count = %d, want 1", len(rows.field))
	}
}

func TestStaticClaimStateMachineSeparatesOneShotAndFill(t *testing.T) {
	claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	target := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	rows := &staticRows{}
	if err := rows.FillClaimTarget(claim, claim, target); err == nil {
		t.Fatal("Claim fill without declaration was accepted")
	}
	if err := rows.ClaimOneShot(claim, claim, target); err != nil {
		t.Fatalf("one-shot Claim failed: %v", err)
	}
	if err := rows.ClaimOneShot(claim, claim, target); err == nil {
		t.Fatal("duplicate one-shot Claim was accepted")
	}
	if err := rows.FillClaimTarget(claim, claim, target); err == nil {
		t.Fatal("fill overwrote complete one-shot Claim")
	}

	declared := keyspace.MakeTerm(keyspace.FamilyValueClaim, 2)
	if err := rows.ClaimDeclare(declared, declared); err != nil {
		t.Fatalf("Claim declaration failed: %v", err)
	}
	if err := rows.FillClaimTarget(declared, declared, target); err != nil {
		t.Fatalf("declared Claim fill failed: %v", err)
	}
	if err := rows.FillClaimTarget(declared, declared, target); err == nil {
		t.Fatal("duplicate declared Claim fill was accepted")
	}
}

func TestStaticDeclarationAndAssertionKeepSeparateCoordinates(t *testing.T) {
	c := New("static-coordinates.lua", 0, bind.GlobalCensus{})
	declSpan := source.Span{File: "static-coordinates.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}
	nameSpan := source.Span{File: "static-coordinates.lua", StartLine: 1, StartCol: 6, EndLine: 1, EndCol: 11}
	paramSpan := source.Span{File: "static-coordinates.lua", StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 8}
	body := c.Source().Order().Body(declSpan)
	if body == 0 {
		t.Fatal("Body construction failed")
	}
	alias := c.Static().Declarations().Alias(declSpan, nameSpan, body, "Alias")
	if alias == 0 {
		t.Fatalf("Alias construction failed: %v", c.err)
	}
	iface := c.Static().Declarations().Interface(declSpan, nameSpan, body, "Interface")
	if iface == 0 {
		t.Fatalf("Interface construction failed: %v", c.err)
	}
	assertion := c.Static().Signatures().TypeAsserts(declSpan, paramSpan, "T", false, 0, 0)
	if assertion == 0 {
		t.Fatalf("TypeAsserts construction failed: %v", c.err)
	}
	wantName, ok := source.CoordinateFromParts(nameSpan.StartLine, nameSpan.StartCol, nameSpan.EndLine, nameSpan.EndCol)
	if !ok {
		t.Fatal("invalid name coordinate fixture")
	}
	wantParam, ok := source.CoordinateFromParts(paramSpan.StartLine, paramSpan.StartCol, paramSpan.EndLine, paramSpan.EndCol)
	if !ok {
		t.Fatal("invalid parameter coordinate fixture")
	}
	if got := c.spans[keyspace.FamilyTypeAlias][0]; got != declSpan {
		t.Fatalf("Alias declaration span = %#v, want %#v", got, declSpan)
	}
	if got := c.static.aliases[0].coordinate; got != wantName {
		t.Fatalf("Alias name coordinate = %#v, want %#v", got, wantName)
	}
	if got := c.spans[keyspace.FamilyTypeInterface][0]; got != declSpan {
		t.Fatalf("Interface declaration span = %#v, want %#v", got, declSpan)
	}
	if got := c.static.interfaces[0].coordinate; got != wantName {
		t.Fatalf("Interface name coordinate = %#v, want %#v", got, wantName)
	}
	if got := c.spans[keyspace.FamilyTypeAsserts][0]; got != declSpan {
		t.Fatalf("TypeAsserts expression span = %#v, want %#v", got, declSpan)
	}
	if got := c.static.assertions[0].coordinate; got != wantParam {
		t.Fatalf("TypeAsserts parameter coordinate = %#v, want %#v", got, wantParam)
	}
}

func TestStaticPublicationDuplicateIsDelegatedToStaticBuild(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:            1,
		keyspace.FamilyAssign:          1,
		keyspace.FamilyTypePrimitive:   1,
		keyspace.FamilyTypeAlias:       1,
		keyspace.FamilyTypeRef:         2,
		keyspace.FamilyTypePublication: 2,
	}
	owner := New("static-publication.lua", 0, bind.GlobalCensus{})
	rows := &owner.static
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	refOne := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	refTwo := keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)
	publicationOne := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	publicationTwo := keyspace.MakeTerm(keyspace.FamilyTypePublication, 2)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	if err := rows.Primitive(primitive, programstatic.PrimitiveString); err != nil {
		t.Fatal(err)
	}
	if !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "Alias"}) || !owner.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "Type"}) {
		t.Fatal("publication exact admission failed")
	}
	if err := rows.AliasDeclare(alias, body, "Alias", mustCoordinate(t)); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasParams(alias, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.AliasTarget(alias, primitive); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeRefDeclaration(refOne, 0, alias, []string{"Type"}); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeRefDeclaration(refTwo, 0, alias, []string{"Type"}); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypePublication(publicationOne, assign, 0, refOne); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypePublication(publicationTwo, assign, 0, refTwo); err != nil {
		t.Fatal(err)
	}
	preimage := testStaticPreimage(t, counts, owner.source.exact)
	input, err := rows.freeze(preimage, counts)
	if err != nil {
		t.Fatalf("Collector Static freeze rejected duplicate before Static Build: %v", err)
	}
	if _, err := programstatic.Build(input); err == nil {
		t.Fatal("Static Build accepted duplicate Assign/pair publication")
	}
}

func TestStaticClaimDeclarationRequiresTargetBeforeFreeze(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:          1,
		keyspace.FamilyValueClaim:    1,
		keyspace.FamilyTypePrimitive: 1,
	}
	claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	owner := New("static-law.lua", 0, bind.GlobalCensus{})
	rows := &owner.static
	rows.claims = []programstatic.ClaimTarget{{Claim: claim}}
	preimage := testStaticPreimage(t, counts, nil)
	if _, err := rows.freeze(preimage, counts); err == nil {
		t.Fatal("Static freeze accepted an unfilled sparse Claim declaration")
	}
}

func mustCoordinate(t *testing.T) source.Coordinate {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("coordinate fixture is invalid")
	}
	return coordinate
}

func testStaticPreimage(t *testing.T, counts [keyspace.FamilyCount]uint32, atoms []keyspace.LiteralValue) source.Preimage {
	t.Helper()
	families := make([]source.FamilySpans, keyspace.FamilyCount-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			spans[index] = source.Span{File: "static-law.lua", StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 2}
		}
		families[family-1] = source.FamilySpans{Family: family, Spans: spans}
	}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	draft, err := source.Build(source.Input{
		Name:       "static-law.lua",
		Families:   families,
		Bodies:     []source.BodySource{{Body: body, Terms: []keyspace.Term{body}}},
		ExactAtoms: atoms,
	})
	if err != nil {
		t.Fatalf("source build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("source finalizer: %v", err)
	}
	return finalizer.Preimage()
}
