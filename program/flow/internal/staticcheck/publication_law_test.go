package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestStaticCheckPublicationUsesExactWritePairAndPathReceipt(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyKey, 1), checkCount(keyspace.FamilyLensExact, 1),
		checkCount(keyspace.FamilyRead, 1), checkCount(keyspace.FamilyValues, 1),
		checkCount(keyspace.FamilyAssign, 1), checkCount(keyspace.FamilyWrite, 1),
		checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication.lua", counts: counts,
		rows:   [][]keyspace.Term{{assign}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(body, "field")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilValue}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: cell}},
				Assigns: []authored.Assign{{Owner: body, Values: values}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: cell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 1 || receipt.Publications[0] != publication {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationAcceptsDeclarationTarget(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	fieldKey := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	coordinate, coordinateOK := source.CoordinateFromParts(1, 1, 1, 2)
	if !coordinateOK {
		t.Fatal("CoordinateFromParts rejected declaration fixture")
	}
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyKey, 1), checkCount(keyspace.FamilyLensExact, 1),
		checkCount(keyspace.FamilyRead, 1), checkCount(keyspace.FamilyValues, 1),
		checkCount(keyspace.FamilyAssign, 1), checkCount(keyspace.FamilyWrite, 1),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyTypeAlias, 1),
		checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-declaration.lua", counts: counts,
		rows:   [][]keyspace.Term{{alias, assign}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(body, "field")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilValue}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: fieldKey, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: cell}},
				Assigns: []authored.Assign{{Owner: body, Values: values}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}},
			},
		},
		static: static.Input{
			Types: static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
			References: static.ReferencesInput{TypeRef: []static.TypeRef{{
				Resolution: static.TypeRefDeclaration, Source: []keyspace.Key{1}, Target: alias,
			}}},
			Declarations: static.DeclarationsInput{Alias: []static.TypeAlias{{
				Owner: body, Target: primitive, Name: 1, NameCoordinate: coordinate,
			}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 1 || receipt.Publications[0] != publication {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationAcceptsQualifiedDeclarationTarget(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTwo := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	cellA := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cellB := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	keyA := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	keyT := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	lensOne := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	lensTwo := keyspace.MakeTerm(keyspace.FamilyLensExact, 2)
	readA := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readB := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	valuesOne := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	valuesTwo := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	assignOne := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assignTwo := keyspace.MakeTerm(keyspace.FamilyAssign, 2)
	refOne := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	refTwo := keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	publicationOne := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	publicationTwo := keyspace.MakeTerm(keyspace.FamilyTypePublication, 2)
	coordinate, coordinateOK := source.CoordinateFromParts(1, 1, 1, 2)
	if !coordinateOK {
		t.Fatal("CoordinateFromParts rejected qualified declaration fixture")
	}
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyKey, 2), checkCount(keyspace.FamilyLensExact, 2), checkCount(keyspace.FamilyRead, 2),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyAssign, 2), checkCount(keyspace.FamilyWrite, 2),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyTypeAlias, 1),
		checkCount(keyspace.FamilyTypeRef, 2), checkCount(keyspace.FamilyTypePublication, 2),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-qualified-declaration.lua", counts: counts,
		rows:   [][]keyspace.Term{{alias, assignOne, assignTwo}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "A"}, {Kind: keyspace.LiteralString, String: "T"}},
		keys:   []source.KeyInput{source.NameKey(body, "A"), source.NameKey(body, "T")},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{nilOne, nilTwo},
			},
			Access: authored.AccessInput{Exact: []authored.ExactLens{
				{Owner: body, Base: readA, Source: keyT, Kind: kind.FieldName},
				{Owner: body, Base: readB, Source: keyA, Kind: kind.FieldName},
			}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}, {Kind: authored.CellGlobal, Key: 2}},
				Reads:   []authored.Read{{Owner: body, Source: cellA}, {Owner: body, Source: cellB}},
				Assigns: []authored.Assign{{Owner: body, Values: valuesOne}, {Owner: body, Values: valuesTwo}},
				Writes:  []authored.Write{{Assign: assignOne, Target: lensOne}, {Assign: assignTwo, Target: lensTwo}},
			},
		},
		static: static.Input{
			Types: static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
			References: static.ReferencesInput{TypeRef: []static.TypeRef{
				{Resolution: static.TypeRefDeclaration, Source: []keyspace.Key{2}, Target: alias},
				{Resolution: static.TypeRefDeclaration, Source: []keyspace.Key{1, 2}, Root: cellA, Target: alias},
			}},
			Declarations: static.DeclarationsInput{Alias: []static.TypeAlias{{
				Owner: body, Target: primitive, Name: 2, NameCoordinate: coordinate,
			}}},
			Publications: static.PublicationsInput{Type: []static.Publication{
				{Assign: assignOne, Pair: 0, Target: refOne},
				{Assign: assignTwo, Pair: 0, Target: refTwo},
			}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 2 || receipt.Publications[0] != publicationOne || receipt.Publications[1] != publicationTwo {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationRejectsMismatchedCanonicalPathIdentity(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilValue2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assign2 := keyspace.MakeTerm(keyspace.FamilyAssign, 2)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyKey, 2), checkCount(keyspace.FamilyLensExact, 2), checkCount(keyspace.FamilyRead, 2),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyAssign, 2), checkCount(keyspace.FamilyWrite, 2),
		checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-foreign-key.lua", counts: counts,
		rows:   [][]keyspace.Term{{assign, assign2}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}, {Kind: keyspace.LiteralString, String: "other"}},
		keys:   []source.KeyInput{source.NameKey(body, "field"), source.NameKey(body, "other")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{nilValue, nilValue2}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}, {Owner: body, Base: read2, Source: keyspace.MakeTerm(keyspace.FamilyKey, 2), Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: cell}},
				Assigns: []authored.Assign{{Owner: body, Values: values}, {Owner: body, Values: values2}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}, {Assign: assign2, Target: keyspace.MakeTerm(keyspace.FamilyLensExact, 2)}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: cell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{2}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err == nil {
		t.Fatal("Validate accepted a publication with a foreign canonical path key")
	}
	if len(receipt.Publications) != 0 {
		t.Fatalf("invalid Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationRejectsMismatchedCanonicalPathRoot(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilValue2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	foreignRoot := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	key2 := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	lens2 := keyspace.MakeTerm(keyspace.FamilyLensExact, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assign2 := keyspace.MakeTerm(keyspace.FamilyAssign, 2)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyKey, 2), checkCount(keyspace.FamilyLensExact, 2), checkCount(keyspace.FamilyRead, 2),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyAssign, 2), checkCount(keyspace.FamilyWrite, 2),
		checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-foreign-root.lua", counts: counts,
		rows:   [][]keyspace.Term{{assign, assign2}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}, {Kind: keyspace.LiteralString, String: "other"}},
		keys:   []source.KeyInput{source.NameKey(body, "field"), source.NameKey(body, "other")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{nilValue, nilValue2}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}, {Owner: body, Base: read2, Source: key2, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}, {Kind: authored.CellGlobal, Key: 2}},
				Reads:   []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: foreignRoot}},
				Assigns: []authored.Assign{{Owner: body, Values: values}, {Owner: body, Values: values2}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}, {Assign: assign2, Target: lens2}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: foreignRoot, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	if receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	); err == nil || len(receipt.Publications) != 0 {
		t.Fatalf("Validate accepted a foreign canonical path root: %#v/%v", receipt, err)
	}
}

func TestStaticCheckPublicationAcceptsVisibleLocalRoot(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyKey, 1), checkCount(keyspace.FamilyLensExact, 1), checkCount(keyspace.FamilyRead, 1),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyAssign, 1),
		checkCount(keyspace.FamilyWrite, 1), checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-local-root.lua", counts: counts,
		rows: [][]keyspace.Term{{bind, assign}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(body, "field")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{nil1, nil2}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Reads: []authored.Read{{Owner: body, Source: cell}},
				Binds: []authored.Bind{{Owner: body, Values: values1}}, Assigns: []authored.Assign{{Owner: body, Values: values2}},
				Writes: []authored.Write{{Assign: assign, Target: lens}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: cell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 1 || receipt.Publications[0] != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationAcceptsDeepDottedPath(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	key1 := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	key2 := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	lens1 := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	lens2 := keyspace.MakeTerm(keyspace.FamilyLensExact, 2)
	read1 := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyKey, 2), checkCount(keyspace.FamilyLensExact, 2), checkCount(keyspace.FamilyRead, 2),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyAssign, 1),
		checkCount(keyspace.FamilyWrite, 1), checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-deep-path.lua", counts: counts,
		rows: [][]keyspace.Term{{bind, assign}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "first"}, {Kind: keyspace.LiteralString, String: "second"}},
		keys:   []source.KeyInput{source.NameKey(body, "first"), source.NameKey(body, "second")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{nil1, nil2}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{
				{Owner: body, Base: read1, Source: key1, Kind: kind.FieldName},
				{Owner: body, Base: read2, Source: key2, Kind: kind.FieldName},
			}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Reads: []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: lens1}},
				Binds: []authored.Bind{{Owner: body, Values: values1}}, Assigns: []authored.Assign{{Owner: body, Values: values2}},
				Writes: []authored.Write{{Assign: assign, Target: lens2}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: cell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1, 2}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 1 || receipt.Publications[0] != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}

func TestStaticCheckPublicationAcceptsAdjustedTailPair(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyKey, 1), checkCount(keyspace.FamilyLensExact, 1), checkCount(keyspace.FamilyRead, 1),
		checkCount(keyspace.FamilyVararg, 1), checkCount(keyspace.FamilyValues, 1), checkCount(keyspace.FamilyAssign, 1),
		checkCount(keyspace.FamilyWrite, 1), checkCount(keyspace.FamilyTypeRef, 1), checkCount(keyspace.FamilyTypePublication, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-publication-adjusted-tail.lua", counts: counts,
		rows:   [][]keyspace.Term{{assign}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(body, "field")},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: vararg}}, Terms: nil},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellGlobal, Key: 1}, {Kind: authored.CellLocal, Body: body}}, Reads: []authored.Read{{Owner: body, Source: cell}},
				Assigns: []authored.Assign{{Owner: body, Values: values1}}, Writes: []authored.Write{{Assign: assign, Target: lens}},
				Varargs: []authored.Vararg{{Owner: body, Cell: keyspace.MakeTerm(keyspace.FamilyCell, 2)}},
			},
		},
		static: static.Input{
			References:   static.ReferencesInput{TypeRef: []static.TypeRef{{Resolution: static.TypeRefCanonicalPath, Root: cell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1}}}},
			Publications: static.PublicationsInput{Type: []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.Publications) != 1 || receipt.Publications[0] != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publication receipt = %#v", receipt.Publications)
	}
}
