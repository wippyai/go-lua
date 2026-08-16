package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

type moduleEntryImportTerms struct {
	body, foreignBody      keyspace.Term
	request                keyspace.Term
	actuals, assigned      keyspace.Term
	requireCell, localCell keyspace.Term
	globalCell             keyspace.Term
	readRequire, readLocal keyspace.Term
	readLens, readGlobal   keyspace.Term
	readNonImplicit        keyspace.Term
	requireLens            keyspace.Term
	call, imported         keyspace.Term
	requireKey, globalKey  keyspace.Key
}

type moduleEntryImportFixture struct {
	source source.View
	flow   authored.View
	module imports.View
	terms  moduleEntryImportTerms
}

func TestModuleEntryImportRequiresCanonicalGlobalRequireRead(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, nil)
	terms := fixture.terms

	// Assignment to the global is ordinary Heap mutation. It does not erase the
	// authored Module observation. The static-only and runtime paths both use
	// the same canonical global Read shape; only the Read's implicit provenance
	// differs.
	assign, target, ok := fixture.flow.Storage().Writes().Get(keyspace.MakeTerm(keyspace.FamilyWrite, 1))
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || target != terms.requireCell {
		t.Fatalf("require Write = %v/%v/%v, want authored global mutation", assign, target, ok)
	}
	if length, ok := fixture.flow.Values().Len(terms.actuals); !ok || length != 1 {
		t.Fatalf("require actual width = %d/%v, want one fixed argument", length, ok)
	}

	resolutions, err := sealModuleImportResolutions(fixture.source, fixture.flow, fixture.module)
	if err != nil {
		t.Fatalf("sealModuleImportResolutions: %v", err)
	}
	if len(resolutions) != 1 || resolutions[0].Request != terms.request || resolutions[0].Key == 0 {
		t.Fatalf("literal require resolution = %#v, want first Source string argument", resolutions)
	}
	if atom, ok := fixture.source.Keys().Exact(resolutions[0].Key); !ok || atom != (keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "dep"}) {
		t.Fatalf("resolved request atom = %#v/%v, want dep", atom, ok)
	}
}

func TestModuleEntryImportAcceptsStaticNonImplicitRequireRead(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, func(input *authored.Input, terms moduleEntryImportTerms) {
		input.Calls[0].Callee = terms.readNonImplicit
	})
	resolutions, err := sealModuleImportResolutions(fixture.source, fixture.flow, fixture.module)
	if err != nil {
		t.Fatalf("static non-implicit require Read: %v", err)
	}
	if len(resolutions) != 1 || resolutions[0].Request != fixture.terms.request || resolutions[0].Key == 0 {
		t.Fatalf("static require resolution = %#v, want authored Request and derived Key", resolutions)
	}
}

func TestModuleEntryImportRejectsEmptySourceStringRequest(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, nil, "")
	request := fixture.terms.request
	observed, owner, text, ok := fixture.source.Literals().Strings().At(0)
	if !ok || observed != request || owner != fixture.terms.body || text != "" {
		t.Fatalf("empty Request fixture is not an authored Source String row: %v/%v/%q/%v", observed, owner, text, ok)
	}
	if key, keyOK := fixture.source.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: ""}); !keyOK || key == 0 {
		t.Fatalf("empty Request fixture is not an exact Source key: %v/%v", key, keyOK)
	}
	if _, err := sealModuleImportResolutions(fixture.source, fixture.flow, fixture.module); err == nil {
		t.Fatal("authored-admissible empty Source String Request was accepted by final Flow resolution")
	}
}

func TestModuleEntryImportRejectsCrossOwnerActuals(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, func(input *authored.Input, terms moduleEntryImportTerms) {
		input.Values.Rows[0].Owner = terms.foreignBody
	})
	if _, err := sealModuleImportResolutions(fixture.source, fixture.flow, fixture.module); err == nil {
		t.Fatal("Import whose actual Values have a foreign Body owner was accepted")
	}
}

func TestModuleEntryImportRejectsDuplicateCallClaim(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, nil)
	duplicate := keyspace.MakeTerm(keyspace.FamilyImport, 2)
	moduleView := moduleEntryModule(t, imports.Input{Imports: []imports.Import{
		{Term: fixture.terms.imported, Call: fixture.terms.call, Request: fixture.terms.request},
		{Term: duplicate, Call: fixture.terms.call, Request: fixture.terms.request},
	}})
	if _, err := sealModuleImportResolutions(fixture.source, fixture.flow, moduleView); err == nil || err.Error() != "program/flow: duplicate Module Import Call" {
		t.Fatalf("duplicate Call claim error = %v, want exact duplicate-ownership rejection", err)
	}
}

func TestModuleEntryImportRejectsNonCanonicalCalleeForms(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*authored.Input, moduleEntryImportTerms)
	}{
		{
			name: "shadowed local or alias",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Calls[0].Callee = terms.readLocal
			},
		},
		{
			name: "_G.require lens",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Calls[0].Callee = terms.readLens
			},
		},
		{
			name: "literal or arbitrary callee",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Calls[0].Callee = terms.request
			},
		},
		{
			name: "other global",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Calls[0].Callee = terms.readGlobal
			},
		},
		{
			name: "receiver-bearing method form",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Calls[0].Callee = terms.readLens
				input.Calls[0].Receiver = terms.readGlobal
			},
		},
		{
			name: "missing canonical global require Cell",
			mutate: func(input *authored.Input, terms moduleEntryImportTerms) {
				input.Storage.Cells[0] = authored.Cell{Kind: authored.CellLocal, Body: terms.body}
				input.Storage.Reads[0].Source = terms.globalCell
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newModuleEntryImportFixture(t, test.mutate)
			if _, err := sealModuleImportResolutions(fixture.source, fixture.flow, fixture.module); err == nil {
				t.Fatal("non-canonical require callee was accepted")
			}
		})
	}
}

func TestModuleEntryAuthoredOwnerRejectsDuplicateGlobalRequireCells(t *testing.T) {
	fixture := newModuleEntryImportFixture(t, nil)
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 2
	if _, err := authored.Build(authored.Input{
		Counts: counts,
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellGlobal, Key: fixture.terms.requireKey},
			{Kind: authored.CellGlobal, Key: fixture.terms.requireKey},
		}},
	}); err == nil {
		t.Fatal("authored Flow accepted duplicate global require Cells")
	}
}

func newModuleEntryImportFixture(
	t *testing.T,
	mutate func(*authored.Input, moduleEntryImportTerms),
	requestText ...string,
) moduleEntryImportFixture {
	t.Helper()
	requestValue := "dep"
	if len(requestText) != 0 {
		requestValue = requestText[0]
	}
	terms := moduleEntryImportTerms{
		body:            keyspace.MakeTerm(keyspace.FamilyBody, 1),
		foreignBody:     keyspace.MakeTerm(keyspace.FamilyBody, 2),
		request:         keyspace.MakeTerm(keyspace.FamilyString, 1),
		actuals:         keyspace.MakeTerm(keyspace.FamilyValues, 1),
		assigned:        keyspace.MakeTerm(keyspace.FamilyValues, 2),
		requireCell:     keyspace.MakeTerm(keyspace.FamilyCell, 1),
		localCell:       keyspace.MakeTerm(keyspace.FamilyCell, 2),
		globalCell:      keyspace.MakeTerm(keyspace.FamilyCell, 3),
		readRequire:     keyspace.MakeTerm(keyspace.FamilyRead, 1),
		readLocal:       keyspace.MakeTerm(keyspace.FamilyRead, 2),
		readLens:        keyspace.MakeTerm(keyspace.FamilyRead, 3),
		readGlobal:      keyspace.MakeTerm(keyspace.FamilyRead, 4),
		readNonImplicit: keyspace.MakeTerm(keyspace.FamilyRead, 5),
		requireLens:     keyspace.MakeTerm(keyspace.FamilyLensExact, 1),
		call:            keyspace.MakeTerm(keyspace.FamilyCall, 1),
		imported:        keyspace.MakeTerm(keyspace.FamilyImport, 1),
	}
	keyRequire := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)

	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyLensExact] = 1
	counts[keyspace.FamilyCell] = 3
	counts[keyspace.FamilyRead] = 5
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 1
	counts[keyspace.FamilyCall] = 1
	counts[keyspace.FamilyKey] = 2
	counts[keyspace.FamilyImport] = 1

	name := "module-entry-import.lua"
	sourceView := moduleEntrySource(t, source.Input{
		Name:     name,
		Families: familySpansNamed(name, counts),
		String:   []source.StringLiteral{{Owner: terms.body, Value: requestValue}},
		ExactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: requestValue},
			{Kind: keyspace.LiteralString, String: "require"},
			{Kind: keyspace.LiteralString, String: "_G"},
		},
		Keys: []source.KeyInput{
			source.NameKey(terms.body, "require"),
			source.NameKey(terms.body, "_G"),
		},
		Bodies: []source.BodySource{{Body: terms.body}, {Body: terms.foreignBody}},
	})
	var ok bool
	terms.requireKey, ok = sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"})
	if !ok || terms.requireKey == 0 {
		t.Fatal("fixture Source has no canonical require key")
	}
	terms.globalKey, ok = sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"})
	if !ok || terms.globalKey == 0 || terms.globalKey == terms.requireKey {
		t.Fatal("fixture Source has no distinct canonical _G key")
	}

	flowInput := authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: terms.body, Fixed: authored.Range{End: 1}},
				{Owner: terms.body, Fixed: authored.Range{Start: 1, End: 2}},
			},
			Terms: []keyspace.Term{terms.request, terms.request},
		},
		Access: authored.AccessInput{Exact: []authored.ExactLens{{
			Owner: terms.body, Base: terms.readGlobal, Source: keyRequire, Kind: kind.FieldName,
		}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: terms.requireKey},
				{Kind: authored.CellLocal, Body: terms.body},
				{Kind: authored.CellGlobal, Key: terms.globalKey},
			},
			Reads: []authored.Read{
				{Owner: terms.body, Source: terms.requireCell, Implicit: true},
				{Owner: terms.body, Source: terms.localCell},
				{Owner: terms.body, Source: terms.requireLens},
				{Owner: terms.body, Source: terms.globalCell},
				{Owner: terms.body, Source: terms.requireCell},
			},
			Assigns: []authored.Assign{{Owner: terms.body, Values: terms.assigned}},
			Writes:  []authored.Write{{Assign: assign, Target: terms.requireCell}},
		},
		Calls: []authored.Call{{Owner: terms.body, Callee: terms.readRequire, Actuals: terms.actuals}},
	}
	if mutate != nil {
		mutate(&flowInput, terms)
	}
	return moduleEntryImportFixture{
		source: sourceView,
		flow:   moduleEntryFlow(t, flowInput),
		module: moduleEntryModule(t, imports.Input{Imports: []imports.Import{{Term: terms.imported, Call: terms.call, Request: terms.request}}}),
		terms:  terms,
	}
}
