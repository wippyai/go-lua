package accessgeometry

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
)

type accessGeometryFixture struct {
	sourceView source.View
	flowView   authored.View
	candidates *candidates.Result
	bodies     *flowbody.Result
	bindings   binding.Result
	staticView static.View
	moduleView imports.View
	staticID   identity.ContentID
	moduleID   identity.ContentID

	sourceFinal source.Finalizer
	staticFinal static.Finalizer
	flowFinal   authored.Finalizer
	moduleFinal imports.Finalizer
}

func openAccessGeometryFixture(t *testing.T) *accessGeometryFixture {
	return openAccessGeometryFixtureNamed(t, "access-geometry.lua")
}

func openAccessGeometryFixtureNamed(t *testing.T, name string) *accessGeometryFixture {
	return openAccessGeometryFixtureWithStaticTypeOfs(t, name, []staticoperators.TypeOf{{
		Scope:   accessTerm(keyspace.FamilyCell, 1),
		Operand: accessTerm(keyspace.FamilyRead, 2),
	}})
}

func openAccessGeometryFixtureWithStaticTypeOfs(t *testing.T, name string, typeOfs []staticoperators.TypeOf) *accessGeometryFixture {
	t.Helper()
	counts := accessGeometryCounts()
	counts[keyspace.FamilyTypeOf] = uint32(len(typeOfs))
	body := accessTerm(keyspace.FamilyBody, 1)
	input := accessFlowInput(body)
	input.Counts = counts

	sourceDraft, err := source.Build(accessSourceInput(counts, body, name))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinal, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinal.Preimage()

	staticInput := static.Input{Counts: counts}
	staticInput.Operators.TypeOf = typeOfs
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinal.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinal, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinal.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinal.View()

	flowDraft, err := authored.Build(input)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinal, err := flowDraft.Finalizer()
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinal.View()

	entry := body
	bodies, err := flowbody.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinal, err := moduleDraft.Finalizer()
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinal.View()
	staticID, moduleID := staticView.ContentID(), moduleView.ContentID()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(sourceFinal, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinal.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		closeAccessGeometryFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlProof, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	paths, err := semanticpath.Seal(issuance, sourceView.CellRoles(), sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("semanticpath.Seal: %v", err)
	}
	proof, err := executable.Seal(sourceView, flowView, forest, controlProof, staticID, moduleID, paths)
	if err != nil {
		closeAccessGeometryFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("executable.Seal: %v", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), flowView, proof, staticID, moduleID)
	if err != nil {
		closeAccessGeometryFinalizers(source.Finalizer{}, staticFinal, flowFinal, moduleFinal)
		t.Fatalf("candidates.Seal: %v", err)
	}
	fixture := &accessGeometryFixture{
		sourceView:  sourceView,
		flowView:    flowView,
		candidates:  candidateResult,
		bodies:      bodies,
		bindings:    bindingResult,
		staticView:  staticView,
		moduleView:  moduleView,
		staticID:    staticID,
		moduleID:    moduleID,
		sourceFinal: source.Finalizer{},
		staticFinal: staticFinal,
		flowFinal:   flowFinal,
		moduleFinal: moduleFinal,
	}
	t.Cleanup(func() {
		closeAccessGeometryFinalizers(fixture.sourceFinal, fixture.staticFinal, fixture.flowFinal, fixture.moduleFinal)
	})
	return fixture
}

func closeAccessGeometryFinalizers(sourceFinal source.Finalizer, staticFinal static.Finalizer, flowFinal authored.Finalizer, moduleFinal imports.Finalizer) {
	_ = moduleFinal.Abort()
	_ = flowFinal.Abort()
	_ = staticFinal.Abort()
	_ = sourceFinal.Abort()
}

func accessTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	term := keyspace.MakeTerm(family, ordinal)
	if term == 0 {
		panic("access geometry fixture Term overflow")
	}
	return term
}

func accessGeometryCounts() (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 30
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyInteger] = 6
	counts[keyspace.FamilyFloat] = 3
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyKey] = 3
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyValues] = 16
	counts[keyspace.FamilyLensExact] = 7
	counts[keyspace.FamilyLensKey] = 2
	counts[keyspace.FamilyRead] = 4
	counts[keyspace.FamilyAssign] = 3
	counts[keyspace.FamilyBind] = 1
	counts[keyspace.FamilyWrite] = 8
	counts[keyspace.FamilyUnary] = 5
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyTable] = 1
	counts[keyspace.FamilyTableField] = 11
	counts[keyspace.FamilyTypeOf] = 1
	return counts
}

func accessSourceInput(counts [keyspace.FamilyCount]uint32, body keyspace.Term, name string) source.Input {
	input := source.Input{
		Name: name,
		ExactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralBool, Bool: true},
			{Kind: keyspace.LiteralInteger, Integer: 1},
			{Kind: keyspace.LiteralInteger, Integer: 2},
			{Kind: keyspace.LiteralInteger, Integer: 7},
			{Kind: keyspace.LiteralString, String: "field"},
			{Kind: keyspace.LiteralString, String: "lens"},
			{Kind: keyspace.LiteralString, String: "direct"},
			{Kind: keyspace.LiteralInteger, Integer: -1},
			{Kind: keyspace.LiteralInteger, Integer: -9},
			{Kind: keyspace.LiteralInteger, Integer: math.MinInt64},
			{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Ldexp(1, 63))},
		},
		Keys: []source.KeyInput{
			source.ListKey(body, 1),
			source.NameKey(body, "field"),
			source.NameKey(body, "lens"),
		},
		Bodies: []source.BodySource{{Body: body, Terms: []keyspace.Term{
			accessTerm(keyspace.FamilyAssign, 1),
			accessTerm(keyspace.FamilyAssign, 2),
			accessTerm(keyspace.FamilyAssign, 3),
			accessTerm(keyspace.FamilyBind, 1),
			accessTerm(keyspace.FamilyReturn, 1),
		}}},
		Binds: []source.BindCells{{
			Bind:  accessTerm(keyspace.FamilyBind, 1),
			Cells: []keyspace.Term{accessTerm(keyspace.FamilyCell, 1)},
		}},
		Integer: []source.IntegerLiteral{
			{Owner: body, Value: 1},
			{Owner: body, Value: math.MinInt64},
			{Owner: body, Value: 1},
			{Owner: body, Value: math.MinInt64},
			{Owner: body, Value: 7},
			{Owner: body, Value: 9},
		},
		Float: []source.FloatLiteral{
			{Owner: body, Bits: math.Float64bits(math.NaN())},
			{Owner: body, Bits: math.Float64bits(math.NaN())},
			{Owner: body, Bits: math.Float64bits(2)},
		},
		Bool:   []source.BoolLiteral{{Owner: body, Value: true}},
		String: []source.StringLiteral{{Owner: body, Value: "direct"}},
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyNil]; ordinal++ {
		input.Nil = append(input.Nil, source.NilLiteral{Owner: body})
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	return input
}

func accessFlowInput(body keyspace.Term) authored.Input {
	t := func(family keyspace.Family, ordinal uint32) keyspace.Term { return accessTerm(family, ordinal) }
	// Every Values row owns a distinct scalar occurrence. This keeps the real
	// containment proof honest while still making Read2 static-only and leaving
	// sparse candidate ordinals around it.
	valueTerms := []keyspace.Term{
		t(keyspace.FamilyRead, 1), t(keyspace.FamilyRead, 3), t(keyspace.FamilyRead, 4),
		t(keyspace.FamilyNil, 13), t(keyspace.FamilyNil, 14), t(keyspace.FamilyNil, 15),
		t(keyspace.FamilyNil, 16), t(keyspace.FamilyNil, 17), t(keyspace.FamilyNil, 18),
		t(keyspace.FamilyNil, 19), t(keyspace.FamilyNil, 20),
		t(keyspace.FamilyNil, 21), t(keyspace.FamilyNil, 25), t(keyspace.FamilyNil, 26),
		t(keyspace.FamilyNil, 22), t(keyspace.FamilyNil, 23),
		t(keyspace.FamilyNil, 24), t(keyspace.FamilyNil, 30), t(keyspace.FamilyTable, 1),
	}
	values := make([]authored.Value, 16)
	for ordinal := 0; ordinal < 11; ordinal++ {
		values[ordinal] = authored.Value{Owner: body, Fixed: authored.Range{Start: uint32(ordinal), End: uint32(ordinal + 1)}}
	}
	values[11] = authored.Value{Owner: body, Fixed: authored.Range{Start: 11, End: 14}}
	values[12] = authored.Value{Owner: body, Fixed: authored.Range{Start: 14, End: 15}}
	values[13] = authored.Value{Owner: body, Fixed: authored.Range{Start: 15, End: 16}}
	values[14] = authored.Value{Owner: body, Fixed: authored.Range{Start: 16, End: 18}}
	values[15] = authored.Value{Owner: body, Fixed: authored.Range{Start: 18, End: 19}}
	return authored.Input{
		Values: authored.ValuesInput{Rows: values, Terms: valueTerms},
		Access: authored.AccessInput{
			Exact: []authored.ExactLens{
				{Owner: body, Base: t(keyspace.FamilyNil, 1), Source: t(keyspace.FamilyKey, 3), Kind: kind.FieldName},
				{Owner: body, Base: t(keyspace.FamilyNil, 2), Source: t(keyspace.FamilyNil, 12), Kind: kind.FieldExact},
				{Owner: body, Base: t(keyspace.FamilyNil, 3), Source: t(keyspace.FamilyFloat, 1), Kind: kind.FieldExact},
				{Owner: body, Base: t(keyspace.FamilyNil, 4), Source: t(keyspace.FamilyUnary, 1), Kind: kind.FieldExact},
				{Owner: body, Base: t(keyspace.FamilyNil, 5), Source: t(keyspace.FamilyUnary, 2), Kind: kind.FieldExact},
				{Owner: body, Base: t(keyspace.FamilyNil, 27), Source: t(keyspace.FamilyUnary, 5), Kind: kind.FieldExact},
				{Owner: body, Base: t(keyspace.FamilyNil, 28), Source: t(keyspace.FamilyNil, 29), Kind: kind.FieldExact},
			},
			Dynamic: []authored.DynamicLens{
				{Owner: body, Base: t(keyspace.FamilyNil, 6), Key: t(keyspace.FamilyNil, 7)},
				{Owner: body, Base: t(keyspace.FamilyNil, 8), Key: t(keyspace.FamilyNil, 9)},
			},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Reads: []authored.Read{
				{Owner: body, Source: t(keyspace.FamilyLensExact, 1)},
				{Owner: body, Source: t(keyspace.FamilyCell, 1)},
				{Owner: body, Source: t(keyspace.FamilyLensExact, 3)},
				{Owner: body, Source: t(keyspace.FamilyLensKey, 2)},
			},
			Assigns: []authored.Assign{
				{Owner: body, Values: t(keyspace.FamilyValues, 12)},
				{Owner: body, Values: t(keyspace.FamilyValues, 13)},
				{Owner: body, Values: t(keyspace.FamilyValues, 14)},
			},
			Binds: []authored.Bind{{Owner: body, Values: t(keyspace.FamilyValues, 15)}},
			Writes: []authored.Write{
				{Assign: t(keyspace.FamilyAssign, 1), Target: t(keyspace.FamilyLensKey, 1)},
				{Assign: t(keyspace.FamilyAssign, 1), Target: t(keyspace.FamilyCell, 1)},
				{Assign: t(keyspace.FamilyAssign, 1), Target: t(keyspace.FamilyLensExact, 2)},
				{Assign: t(keyspace.FamilyAssign, 2), Target: t(keyspace.FamilyCell, 1)},
				{Assign: t(keyspace.FamilyAssign, 2), Target: t(keyspace.FamilyLensExact, 4)},
				{Assign: t(keyspace.FamilyAssign, 2), Target: t(keyspace.FamilyLensExact, 5)},
				{Assign: t(keyspace.FamilyAssign, 3), Target: t(keyspace.FamilyLensExact, 6)},
				{Assign: t(keyspace.FamilyAssign, 3), Target: t(keyspace.FamilyLensExact, 7)},
			},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 11}}},
			Fields: []authored.Field{
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyKey, 1), Values: t(keyspace.FamilyValues, 1), Kind: kind.FieldList},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyKey, 2), Values: t(keyspace.FamilyValues, 2), Kind: kind.FieldName},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyNil, 10), Values: t(keyspace.FamilyValues, 3), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyFloat, 2), Values: t(keyspace.FamilyValues, 4), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyUnary, 3), Values: t(keyspace.FamilyValues, 5), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyUnary, 4), Values: t(keyspace.FamilyValues, 6), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyNil, 11), Values: t(keyspace.FamilyValues, 7), Kind: kind.FieldKey},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyBool, 1), Values: t(keyspace.FamilyValues, 8), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyInteger, 5), Values: t(keyspace.FamilyValues, 9), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyFloat, 3), Values: t(keyspace.FamilyValues, 10), Kind: kind.FieldExact},
				{Table: t(keyspace.FamilyTable, 1), Key: t(keyspace.FamilyString, 1), Values: t(keyspace.FamilyValues, 11), Kind: kind.FieldExact},
			},
			Order: []keyspace.Term{
				t(keyspace.FamilyTableField, 1), t(keyspace.FamilyTableField, 2), t(keyspace.FamilyTableField, 3), t(keyspace.FamilyTableField, 4),
				t(keyspace.FamilyTableField, 5), t(keyspace.FamilyTableField, 6), t(keyspace.FamilyTableField, 7), t(keyspace.FamilyTableField, 8),
				t(keyspace.FamilyTableField, 9), t(keyspace.FamilyTableField, 10), t(keyspace.FamilyTableField, 11),
			},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{
			{Owner: body, Op: kind.UnaryNeg, Operand: t(keyspace.FamilyInteger, 1)},
			{Owner: body, Op: kind.UnaryNeg, Operand: t(keyspace.FamilyInteger, 2)},
			{Owner: body, Op: kind.UnaryNeg, Operand: t(keyspace.FamilyInteger, 3)},
			{Owner: body, Op: kind.UnaryNeg, Operand: t(keyspace.FamilyInteger, 4)},
			{Owner: body, Op: kind.UnaryNeg, Operand: t(keyspace.FamilyInteger, 6)},
		}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: t(keyspace.FamilyValues, 16)}}},
	}
}

func TestAccessGeometrySealHonestFixture(t *testing.T) {
	fixture := openAccessGeometryFixture(t)
	result, err := Seal(fixture.sourceView, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView)
	if err != nil {
		t.Fatalf("accessgeometry.Seal: %v", err)
	}
	if !Matches(result, fixture.sourceView.Identity().ContentID(), fixture.flowView.Cold().ContentID(), fixture.staticID, fixture.moduleID) {
		t.Fatal("sealed access geometry provenance did not match its four owners")
	}
	fields := result.TableFields()
	if fields.Count() != 11 {
		t.Fatalf("TableField denominator = %d, want 11", fields.Count())
	}
	listOwner, _, listKey, ok := fixture.sourceView.Keys().List(accessTerm(keyspace.FamilyKey, 1))
	if !ok || listOwner != accessTerm(keyspace.FamilyBody, 1) {
		t.Fatal("fixture ListKey was unavailable")
	}
	nameOwner, _, nameKey, ok := fixture.sourceView.Keys().Name(accessTerm(keyspace.FamilyKey, 2))
	if !ok || nameOwner != accessTerm(keyspace.FamilyBody, 1) {
		t.Fatal("fixture NameKey was unavailable")
	}
	lensOwner, _, lensKey, ok := fixture.sourceView.Keys().Name(accessTerm(keyspace.FamilyKey, 3))
	if !ok || lensOwner != accessTerm(keyspace.FamilyBody, 1) {
		t.Fatal("fixture Lens NameKey was unavailable")
	}
	if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, 1)); !ok || key != listKey {
		t.Fatalf("FieldList key = %d/%v, want %d/true", key, ok, listKey)
	}
	if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, 2)); !ok || key != nameKey {
		t.Fatalf("FieldName key = %d/%v, want %d/true", key, ok, nameKey)
	}
	for _, ordinal := range []uint32{3, 4} {
		if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, ordinal)); !ok || key != 0 {
			t.Fatalf("FieldExact%v key = %d/%v, want zero/true", ordinal, key, ok)
		}
	}
	keys := fixture.sourceView.Keys()
	negOne, found := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: -1})
	if !found {
		t.Fatal("fixture normalized -1 atom was unavailable")
	}
	minInt, found := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Ldexp(1, 63))})
	if !found {
		t.Fatal("fixture normalized MinInt64 atom was unavailable")
	}
	if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, 5)); !ok || key != negOne {
		t.Fatalf("UnaryNeg integer key = %d/%v, want %d/true", key, ok, negOne)
	}
	if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, 6)); !ok || key != minInt {
		t.Fatalf("UnaryNeg MinInt64 key = %d/%v, want %d/true", key, ok, minInt)
	}
	if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, 7)); !ok || key != 0 {
		t.Fatalf("FieldKey key = %d/%v, want zero/true", key, ok)
	}
	boolKey, boolFound := fixture.sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true})
	intKey, intFound := fixture.sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7})
	floatKey, floatFound := fixture.sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 2})
	stringKey, stringFound := fixture.sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "direct"})
	if !boolFound || !intFound || !floatFound || !stringFound {
		t.Fatal("fixture direct bool/int/integral-float/string keys were unavailable")
	}
	for _, check := range []struct {
		ordinal uint32
		want    keyspace.Key
	}{
		{8, boolKey}, {9, intKey}, {10, floatKey}, {11, stringKey},
	} {
		if key, ok := fields.Get(accessTerm(keyspace.FamilyTableField, check.ordinal)); !ok || key != check.want {
			t.Fatalf("direct FieldExact%v key = %d/%v, want %d/true", check.ordinal, key, ok, check.want)
		}
	}

	exact, dynamic := result.ExactLenses(), result.DynamicLenses()
	if exact.Count() != 7 || dynamic.Count() != 2 {
		t.Fatalf("Lens denominators = exact %d, dynamic %d; want 7, 2", exact.Count(), dynamic.Count())
	}
	for _, ordinal := range []uint32{2, 3} {
		if key, ok := exact.Get(accessTerm(keyspace.FamilyLensExact, ordinal)); !ok || key != 0 {
			t.Fatalf("non-storable exact Lens%v key = %d/%v, want zero/true", ordinal, key, ok)
		}
	}
	if key, ok := exact.Get(accessTerm(keyspace.FamilyLensExact, 4)); !ok || key != negOne {
		t.Fatalf("UnaryNeg exact Lens key = %d/%v, want %d/true", key, ok, negOne)
	}
	if key, ok := exact.Get(accessTerm(keyspace.FamilyLensExact, 5)); !ok || key != minInt {
		t.Fatalf("UnaryNeg MinInt64 exact Lens key = %d/%v, want %d/true", key, ok, minInt)
	}
	minusNine, found := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: -9})
	if !found {
		t.Fatal("fixture normalized -9 atom was unavailable")
	}
	if key, ok := exact.Get(accessTerm(keyspace.FamilyLensExact, 6)); !ok || key != minusNine {
		t.Fatalf("static UnaryNeg exact Lens key = %d/%v, want %d/true", key, ok, minusNine)
	}
	if key, ok := exact.Get(accessTerm(keyspace.FamilyLensExact, 7)); !ok || key != 0 {
		t.Fatalf("nil exact Lens key = %d/%v, want zero/true", key, ok)
	}
	for ordinal := uint32(1); ordinal <= 2; ordinal++ {
		if key, ok := dynamic.Get(accessTerm(keyspace.FamilyLensKey, ordinal)); !ok || key != 0 {
			t.Fatalf("Dynamic Lens%v key = %d/%v, want zero/true", ordinal, key, ok)
		}
	}

	accesses := result.IndexAccesses()
	reads, writes := accesses.Reads(), accesses.Writes()
	if reads.Count() != 3 || writes.Count() != 6 {
		t.Fatalf("IndexAccess candidate counts = reads %d, writes %d; want 3, 6", reads.Count(), writes.Count())
	}
	wantReads := []keyspace.Term{
		accessTerm(keyspace.FamilyRead, 1), accessTerm(keyspace.FamilyRead, 3),
		accessTerm(keyspace.FamilyRead, 4),
	}
	wantWrites := []keyspace.Term{
		accessTerm(keyspace.FamilyWrite, 1), accessTerm(keyspace.FamilyWrite, 3),
		accessTerm(keyspace.FamilyWrite, 5), accessTerm(keyspace.FamilyWrite, 6),
		accessTerm(keyspace.FamilyWrite, 7), accessTerm(keyspace.FamilyWrite, 8),
	}
	for index, wantTerm := range wantReads {
		term, ok := reads.At(index)
		if !ok || term != wantTerm || !reads.Contains(term) {
			t.Fatalf("IndexGet At(%d) = %v/%v, want %v/true", index, term, ok, wantTerm)
		}
	}
	for index, wantTerm := range wantWrites {
		term, ok := writes.At(index)
		if !ok || term != wantTerm || !writes.Contains(term) {
			t.Fatalf("IndexSet At(%d) = %v/%v, want %v/true", index, term, ok, wantTerm)
		}
	}
	base, keyTerm, lens, ok := reads.Get(accessTerm(keyspace.FamilyRead, 1))
	if !ok || base != accessTerm(keyspace.FamilyNil, 1) || keyTerm != accessTerm(keyspace.FamilyKey, 3) || lens != accessTerm(keyspace.FamilyLensExact, 1) {
		t.Fatalf("IndexGet row = %v,%v,%v,%v", base, keyTerm, lens, ok)
	}
	if exactKey, ok := exact.Get(lens); !ok || exactKey != lensKey {
		t.Fatalf("IndexGet exact key plane = %d/%v, want %d/true", exactKey, ok, lensKey)
	}
	base, keyTerm, lens, ok = reads.Get(accessTerm(keyspace.FamilyRead, 3))
	if !ok || base != accessTerm(keyspace.FamilyNil, 3) || keyTerm != accessTerm(keyspace.FamilyFloat, 1) || lens != accessTerm(keyspace.FamilyLensExact, 3) {
		t.Fatalf("IndexGet later exact row = %v,%v,%v,%v", base, keyTerm, lens, ok)
	}
	base, keyTerm, lens, ok = reads.Get(accessTerm(keyspace.FamilyRead, 4))
	if !ok || base != accessTerm(keyspace.FamilyNil, 8) || keyTerm != accessTerm(keyspace.FamilyNil, 9) || lens != accessTerm(keyspace.FamilyLensKey, 2) {
		t.Fatalf("IndexGet later dynamic row = %v,%v,%v,%v", base, keyTerm, lens, ok)
	}
	base, keyTerm, values, position, lens, ok := writes.Get(accessTerm(keyspace.FamilyWrite, 1))
	if !ok || base != accessTerm(keyspace.FamilyNil, 6) || keyTerm != accessTerm(keyspace.FamilyNil, 7) || values != accessTerm(keyspace.FamilyValues, 12) || position != 0 || lens != accessTerm(keyspace.FamilyLensKey, 1) {
		t.Fatalf("IndexSet dynamic row = %v,%v,%v,%d,%v,%v", base, keyTerm, values, position, lens, ok)
	}
	base, keyTerm, values, position, lens, ok = writes.Get(accessTerm(keyspace.FamilyWrite, 3))
	if !ok || base != accessTerm(keyspace.FamilyNil, 2) || keyTerm != accessTerm(keyspace.FamilyNil, 12) || values != accessTerm(keyspace.FamilyValues, 12) || position != 2 || lens != accessTerm(keyspace.FamilyLensExact, 2) {
		t.Fatalf("IndexSet multi-write position row = %v,%v,%v,%d,%v,%v", base, keyTerm, values, position, lens, ok)
	}
	base, keyTerm, values, position, lens, ok = writes.Get(accessTerm(keyspace.FamilyWrite, 8))
	if !ok || base != accessTerm(keyspace.FamilyNil, 28) || keyTerm != accessTerm(keyspace.FamilyNil, 29) || values != accessTerm(keyspace.FamilyValues, 14) || position != 1 || lens != accessTerm(keyspace.FamilyLensExact, 7) {
		t.Fatalf("IndexSet later exact row = %v,%v,%v,%d,%v,%v", base, keyTerm, values, position, lens, ok)
	}
	if reads.Contains(accessTerm(keyspace.FamilyRead, 2)) || writes.Contains(accessTerm(keyspace.FamilyWrite, 2)) || writes.Contains(accessTerm(keyspace.FamilyWrite, 4)) {
		t.Fatal("dead/static Read or Write was retained as a candidate")
	}
	for _, deadRead := range []keyspace.Term{accessTerm(keyspace.FamilyRead, 2)} {
		if _, ok := reads.Slot(deadRead); ok {
			t.Fatalf("dead/static Read %v has a candidate slot", deadRead)
		}
	}
	for _, deadWrite := range []keyspace.Term{accessTerm(keyspace.FamilyWrite, 2), accessTerm(keyspace.FamilyWrite, 4)} {
		if _, ok := writes.Slot(deadWrite); ok {
			t.Fatalf("dead/static Write %v has a candidate slot", deadWrite)
		}
	}
	if _, ok := reads.At(reads.Count()); ok {
		t.Fatal("IndexGet dense At accepted an out-of-range ordinal")
	}
	if _, ok := reads.At(-1); ok {
		t.Fatal("IndexGet dense At accepted a negative ordinal")
	}
	if _, ok := writes.At(writes.Count()); ok {
		t.Fatal("IndexSet dense At accepted an out-of-range ordinal")
	}
	if slot, ok := reads.Slot(accessTerm(keyspace.FamilyRead, 3)); !ok || slot != 1 {
		t.Fatalf("Read candidate slot = %d/%v, want 1/true", slot, ok)
	}
	if slot, ok := reads.Slot(accessTerm(keyspace.FamilyRead, 4)); !ok || slot != 2 {
		t.Fatalf("later Read candidate slot = %d/%v, want 2/true", slot, ok)
	}
	if slot, ok := writes.Slot(accessTerm(keyspace.FamilyWrite, 3)); !ok || slot != 1 {
		t.Fatalf("multi-write candidate slot = %d/%v, want 1/true", slot, ok)
	}
	if slot, ok := writes.Slot(accessTerm(keyspace.FamilyWrite, 8)); !ok || slot != 5 {
		t.Fatalf("later Write candidate slot = %d/%v, want 5/true", slot, ok)
	}
}

func TestAccessGeometrySealRejectsUnavailableForeignAndMalformedOwners(t *testing.T) {
	fixture := openAccessGeometryFixture(t)
	if _, err := Seal(source.View{}, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView); err == nil {
		t.Fatal("Seal accepted an unavailable Source owner")
	}
	if _, err := Seal(fixture.sourceView, authored.View{}, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView); err == nil {
		t.Fatal("Seal accepted an unavailable authored Flow owner")
	}
	if _, err := Seal(fixture.sourceView, fixture.flowView, nil, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView); err == nil {
		t.Fatal("Seal accepted a nil candidate provenance fence")
	}
	foreign := openAccessGeometryFixtureWithStaticTypeOfs(t, "foreign-access-geometry.lua", []staticoperators.TypeOf{
		{Scope: accessTerm(keyspace.FamilyCell, 1), Operand: accessTerm(keyspace.FamilyRead, 2)},
		{Scope: accessTerm(keyspace.FamilyCell, 1), Operand: accessTerm(keyspace.FamilyRead, 2)},
	})
	if foreign.staticView.ContentID() == fixture.staticView.ContentID() {
		t.Fatal("foreign fixture did not produce a distinct Static identity")
	}
	if _, err := Seal(fixture.sourceView, fixture.flowView, foreign.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView); err == nil {
		t.Fatal("Seal accepted a candidate Result from a foreign Source/Flow")
	}
	if _, err := Seal(fixture.sourceView, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, foreign.staticView, fixture.moduleView); err == nil {
		t.Fatal("Seal accepted a foreign Static identity")
	}

	sealed, err := Seal(fixture.sourceView, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView)
	if err != nil {
		t.Fatalf("honest Seal: %v", err)
	}
	foreignBody := accessTerm(keyspace.FamilyBody, 99)
	if _, _, ok := lensGeometry(fixture.flowView, sealed, accessTerm(keyspace.FamilyLensExact, 1), foreignBody); ok {
		t.Fatal("exact Lens geometry accepted a foreign Read/Assign owner")
	}
	if _, _, ok := lensGeometry(fixture.flowView, sealed, accessTerm(keyspace.FamilyLensKey, 1), foreignBody); ok {
		t.Fatal("dynamic Lens geometry accepted a foreign Read/Assign owner")
	}

	counts, err := validateDenominators(fixture.sourceView, fixture.flowView)
	if err != nil || counts.tableFields == 0 || counts.exactLenses == 0 || counts.reads == 0 || counts.writes == 0 {
		t.Fatalf("honest denominator validation = %#v/%v", counts, err)
	}
	if _, err := validateDenominators(fixture.sourceView, authored.View{}); err == nil {
		t.Fatal("denominator validation accepted an unavailable/malformed Flow view")
	}
	if _, _, err := normalizedFieldKey(fixture.sourceView, fixture.flowView, accessTerm(keyspace.FamilyKey, 2), kind.FieldName, foreignBody); err == nil {
		t.Fatal("normalized FieldName accepted a foreign owner")
	}
	if _, _, err := normalizedFieldKey(fixture.sourceView, fixture.flowView, accessTerm(keyspace.FamilyInteger, 5), kind.FieldExact, foreignBody); err == nil {
		t.Fatal("normalized FieldExact accepted a foreign table/lens owner")
	}
}

func TestAccessGeometrySealScalesThroughDenseSeal(t *testing.T) {
	fixture := openAccessGeometryFixture(t)
	sealed, err := Seal(fixture.sourceView, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView)
	if err != nil {
		t.Fatalf("initial Seal: %v", err)
	}
	if cap(sealed.tableFields.keys) != sealed.TableFields().Count()+1 ||
		cap(sealed.exactLenses.keys) != sealed.ExactLenses().Count()+1 ||
		cap(sealed.dynamicLenses.keys) != sealed.DynamicLenses().Count()+1 ||
		cap(sealed.indexAccesses.accesses) != sealed.IndexAccesses().Reads().Count()+sealed.IndexAccesses().Writes().Count() {
		t.Fatal("Seal retained capacity beyond its dense denominators")
	}
	var sealErr error
	allocations := testing.AllocsPerRun(50, func() {
		if _, err := Seal(fixture.sourceView, fixture.flowView, fixture.candidates, fixture.bodies, fixture.bindings, fixture.staticView, fixture.moduleView); err != nil {
			sealErr = err
		}
	})
	if sealErr != nil {
		t.Fatalf("repeated Seal: %v", sealErr)
	}
	if allocations <= 0 {
		t.Fatal("Seal unexpectedly allocated no derived storage")
	}
}
