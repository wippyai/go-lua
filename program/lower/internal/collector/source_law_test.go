package collector

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func testSpan() source.Span { return source.Span{} }

func TestSourceMintIsDenseAndRejectsDerivedOrInvalidFamilies(t *testing.T) {
	c := New("fixture.lua", 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(testSpan())
	if body != keyspace.MakeTerm(keyspace.FamilyBody, 1) {
		t.Fatalf("Body = %v, want dense Body/1", body)
	}
	if got := c.mint(keyspace.FamilyOutcome, testSpan()); got != 0 || c.err == nil {
		t.Fatalf("Outcome mint = %v/%v, want poisoned rejection", got, c.err)
	}
	if got := c.mint(keyspace.FamilyString, testSpan()); got != 0 {
		t.Fatalf("mint after poison = %v, want zero", got)
	}

	bad := New("fixture.lua", 0, bind.GlobalCensus{})
	if got := bad.mint(keyspace.FamilyInvalid, testSpan()); got != 0 || bad.err == nil {
		t.Fatalf("Invalid mint = %v/%v, want poisoned rejection", got, bad.err)
	}
	reserved := New("fixture.lua", 1, bind.GlobalCensus{})
	if got := reserved.mint(keyspace.FamilyImport, testSpan()); got != 0 || reserved.err == nil {
		t.Fatalf("reserved Import mint = %v/%v, want census-only rejection", got, reserved.err)
	}
}

func TestSourceRowsPreserveFreshLiteralsRawKeysAndBodyOrder(t *testing.T) {
	c := New("fixture.lua", 0, bind.GlobalCensus{})
	literals := c.Source().Literals()
	order := c.Source().Order()
	keys := c.Source().Keys()
	body := order.Body(testSpan())
	nilTerm := literals.Nil(testSpan(), body)
	boolTerm := literals.Bool(testSpan(), body, true)
	integerTerm := literals.Integer(testSpan(), body, 7)
	floatTerm := literals.FloatBits(testSpan(), body, math.Float64bits(1.5))
	stringTerm := literals.String(testSpan(), body, "payload")
	nameTerm := keys.Name(testSpan(), body, "field")
	listTerm := keys.List(testSpan(), body, 1)
	if nilTerm == 0 || boolTerm == 0 || integerTerm == 0 || floatTerm == 0 || stringTerm == 0 || nameTerm == 0 || listTerm == 0 {
		t.Fatalf("fresh source terms contain zero: nil=%v bool=%v int=%v float=%v string=%v name=%v list=%v", nilTerm, boolTerm, integerTerm, floatTerm, stringTerm, nameTerm, listTerm)
	}
	// Scalar literals and Keys are Source-owned leaves, not direct Body roots.
	// Give them honest Flow containment: the table fields retain the key
	// leaves, while Return is the sole direct Body statement root.
	table := c.Flow().Tables().DeclareTable(testSpan(), body)
	nameValues := c.Flow().Values().Values(testSpan(), body, []Term{stringTerm}, 0)
	listValues := c.Flow().Values().Values(testSpan(), body, []Term{integerTerm}, 0)
	rootValues := c.Flow().Values().Values(testSpan(), body,
		[]Term{nilTerm, boolTerm, floatTerm, table}, 0)
	nameField := c.Flow().Tables().TableField(testSpan(), table, nameTerm, nameValues, kind.FieldName)
	listField := c.Flow().Tables().TableField(testSpan(), table, listTerm, listValues, kind.FieldList)
	ret := c.Flow().Control().Return(testSpan(), body, rootValues)
	if nameValues == 0 || listValues == 0 || table == 0 || rootValues == 0 || nameField == 0 || listField == 0 ||
		!c.Flow().Tables().FillTable(table, []Term{nameField, listField}) || ret == 0 {
		t.Fatalf("failed to build Flow containment: %v", c.err)
	}
	if !order.SetBody(body, ret) || !order.SetEntry(body) {
		t.Fatalf("failed to complete Body/Entry: %v", c.err)
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	preimage := preparedSourcePreimage(t, prepared)
	if entry := preparedEntryForTest(t, prepared); entry != body {
		t.Fatalf("Entry = %v, want %v", entry, body)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		want := sourceFixtureFamilyCount(family)
		if got := preimage.Identity().FamilyCount(family); got != want {
			t.Fatalf("family %d count = %d, want %d", family, got, want)
		}
	}
	if count, ok := preimage.Order().BodyLen(body); !ok || count != 1 {
		t.Fatalf("Body order length = %d/%v, want one Return root", count, ok)
	}
	for _, term := range []Term{nilTerm, boolTerm, integerTerm, floatTerm, stringTerm, nameTerm, listTerm} {
		if _, ok := preimage.Identity().Span(term); !ok {
			t.Fatalf("Source omitted authored leaf %v", term)
		}
	}
	if preimage.Keys().ExactCount() != 2 {
		t.Fatalf("exact candidate count = %d, want name and list candidates", preimage.Keys().ExactCount())
	}
	wantContent := preimage.Identity().ContentID()
	if !collectorScratchCleared(c) {
		t.Fatal("Prepare retained Collector construction scratch")
	}
	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("Assemble = %v/%v", assembly, err)
	}
	// Collector cleanup proves only that the borrowed construction cursor was
	// released. The Source Build ownership law (program/source) proves that
	// the published component owns its rows after this synchronous handoff.
	sourceComponent, _, _, _, err := assembly.Take()
	if err != nil || sourceComponent == nil {
		t.Fatalf("Assembly.Take = %v/%v", sourceComponent, err)
	}
	if got := sourceComponent.View().Identity().ContentID(); got != wantContent {
		t.Fatalf("published Source changed after Collector terminal cleanup: %x != %x", got, wantContent)
	}
	if span, ok := sourceComponent.View().Identity().Span(stringTerm); !ok || span.File != "fixture.lua" {
		t.Fatalf("published Source span after Collector cleanup = %#v/%v", span, ok)
	}
	if preimage.Identity().Name() != "" {
		t.Fatal("Source Preimage remained live after Assemble")
	}
}

func TestSourceBuildPreimageUsesOneCanonicalExactDenominator(t *testing.T) {
	c := New("fixture.lua", 0, bind.GlobalCensus{})
	order := c.Source().Order()
	body := order.Body(testSpan())
	key := c.Source().Keys().Name(testSpan(), body, "stable")
	if key == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatalf("failed source setup: %v", c.err)
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	preimage := preparedSourcePreimage(t, prepared)
	staticView, moduleView := preparedSiblingViewsForTest(t, prepared)
	if got := preimage.Identity().FamilyCount(keyspace.FamilyKey); got != 1 {
		t.Fatalf("Source key family count = %d, want one", got)
	}
	atom, ok := preimage.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "stable"})
	if !ok || atom == 0 || preimage.Keys().ExactCount() != 1 {
		t.Fatalf("canonical exact lookup = %v/%v count=%d", atom, ok, preimage.Keys().ExactCount())
	}
	if err := abortPreparedSourceForTest(t, prepared); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if preimage.Identity().Name() != "" || preimage.Keys().ExactCount() != 0 {
		t.Fatal("Source Preimage remained live after terminal Abort")
	}
	// Source is already terminal; Assemble must fail and consume the remaining
	// Static, Module, and Flow construction owners rather than reopening it.
	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("Assemble after Source Abort = %v/%v, want terminal failure", assembly, err)
	}
	if staticView.Available() || moduleView.ContentID().Available() {
		t.Fatal("failed Prepared.Assemble left a sibling owner live")
	}
}

func TestPrepareIsOneShotTerminalAndExpiresPreimageAfterAssemble(t *testing.T) {
	c := New("dependent.lua", 0, bind.GlobalCensus{})
	sourceRoot := c.Source()
	flowRoot := c.Flow()
	staticRoot := c.Static()
	body := c.Source().Order().Body(testSpan())
	if !c.Source().Order().SetBody(body) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("Source completion failed: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if entry := preparedEntryForTest(t, prepared); entry != body || preparedSourcePreimage(t, prepared).Identity().Name() != "dependent.lua" {
		t.Fatalf("Prepare result is incomplete: entry=%v source=%q", entry, preparedSourcePreimage(t, prepared).Identity().Name())
	}
	if !errors.Is(failure(c), errCollectorTerminal) || !collectorScratchCleared(c) {
		t.Fatalf("collector did not terminalize and clear scratch: %v", failure(c))
	}
	if got := sourceRoot.Order().Body(testSpan()); got != 0 {
		t.Fatalf("captured Source root mutated terminal Collector with %v", got)
	}
	if got := flowRoot.Storage().Cell(testSpan(), body); got != 0 {
		t.Fatalf("captured Flow root mutated terminal Collector with %v", got)
	}
	if got := staticRoot.Types().Primitive(testSpan(), programstatic.PrimitiveString); got != 0 {
		t.Fatalf("captured Static root mutated terminal Collector with %v", got)
	}
	if repeated, repeatedErr := c.Prepare(); repeatedErr == nil || repeated.state != nil {
		t.Fatalf("second Prepare = %#v/%v, want terminal rejection", repeated, repeatedErr)
	}

	preimage := preparedSourcePreimage(t, prepared)
	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("Assemble = %v/%v", assembly, err)
	}
	if preimage.Identity().Name() != "" || preimage.Identity().TermCount() != 0 || preimage.Keys().ExactCount() != 0 {
		t.Fatal("Source Preimage remained live after terminal Assemble")
	}
}

func TestPrepareFailureAfterSourceClaimAbortsAndTerminalizes(t *testing.T) {
	c := New("prepare-failure.lua", 0, bind.GlobalCensus{})
	order := c.Source().Order()
	body := order.Body(testSpan())
	// Mint a Values identity without its Flow-owned row. Source can build the
	// complete family census, then the private dependent freeze must reject the
	// missing Flow row after Source has already been claimed.
	orphan := c.mint(keyspace.FamilyValues, testSpan())
	if body == 0 || orphan == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatalf("failure fixture setup: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err == nil || prepared.state != nil {
		t.Fatalf("Prepare accepted orphan Flow row: %#v/%v", prepared, err)
	}
	// A failed Source Build/freeze must discard its borrowed view together with
	// every Collector construction row; no retry can observe it. This is a
	// lifecycle law; Source's separate copy law proves published-row ownership.
	if !collectorScratchCleared(c) || !c.terminal || failure(c) == nil {
		t.Fatalf("failed Prepare retained scratch, lost its cause, or remained writable: %v", failure(c))
	}
	if got := c.Source().Order().Body(testSpan()); got != 0 {
		t.Fatalf("failed Prepare allowed later mutation with Body %v", got)
	}
	if repeated, repeatedErr := c.Prepare(); repeatedErr == nil || repeated.state != nil {
		t.Fatalf("failed Prepare reopened on retry: %#v/%v", repeated, repeatedErr)
	}
}

func TestExactCandidateRejectsNilAndNaNAndHandlesUnaryMinInt(t *testing.T) {
	c := New("fixture.lua", 0, bind.GlobalCensus{})
	literals := c.Source().Literals()
	body := c.Source().Order().Body(testSpan())
	nilTerm := literals.Nil(testSpan(), body)
	integer := literals.Integer(testSpan(), body, math.MinInt64)
	float := literals.FloatBits(testSpan(), body, math.Float64bits(2.5))
	unary := c.mint(keyspace.FamilyUnary, testSpan())
	if nilTerm == 0 || integer == 0 || float == 0 || unary == 0 {
		t.Fatalf("failed exact candidate setup: %v", c.err)
	}
	if _, ok := literals.exactLiteral(nilTerm); ok {
		t.Fatal("nil literal became a storable exact candidate")
	}
	negated, ok := literals.unaryNegExact(unary, integer)
	if !ok || negated.Kind != keyspace.LiteralFloat {
		t.Fatalf("minimum integer negation = %#v/%v, want exact float", negated, ok)
	}
	if _, ok := literals.unaryNegExact(unary, float); !ok {
		t.Fatal("float UnaryNeg exact candidate rejected")
	}
	if c.addExact(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}) {
		t.Fatal("NaN exact candidate accepted")
	}
}

func TestFieldExactDoesNotTreatNonNegUnaryAsLiteralNegation(t *testing.T) {
	c := New("unary.lua", 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(testSpan())
	integer := c.Source().Literals().Integer(testSpan(), body, 7)
	bitNot := c.Flow().Operators().Unary(testSpan(), body, kind.UnaryBitNot, integer)
	if integer == 0 || bitNot == 0 {
		t.Fatalf("unary construction failed: %v", failure(c))
	}
	if _, ok := c.Flow().Access().exactCandidate(bitNot); ok {
		t.Fatal("FieldExact admitted non-Neg Unary as arithmetic negation")
	}
}

func TestModuleRequestExactAddsOnlyRawLiteralBeforeSourceFreeze(t *testing.T) {
	const name = "module-source.lua"
	c := New(name, 1, bind.GlobalCensus{})
	literals := c.Source().Literals()
	order := c.Source().Order()
	body := order.Body(source.Span{File: name})
	if body == 0 || !order.SetEntry(body) {
		t.Fatalf("Body/Entry construction failed: %v", failure(c))
	}
	request := literals.String(source.Span{File: name}, body, "pkg.core")
	values := c.Flow().Values().Values(source.Span{File: name}, body, []Term{request}, 0)
	call := c.Flow().Calls().DeclareCall(source.Span{File: name}, body, request, 0, values)
	if request == 0 || values == 0 || call == 0 {
		t.Fatalf("module request construction failed: %v", failure(c))
	}
	if !c.Flow().Calls().SetCallTypeArgs(call, nil) {
		t.Fatalf("Call type-argument sidecar construction failed: %v", failure(c))
	}
	// The Module row remains unresolved authored state; this operation cannot
	// smuggle a Request or Key handle into it.
	importTerm := c.Module().Import(0, source.Span{File: name}, call)
	if importTerm == 0 {
		t.Fatalf("Import failed: %v", failure(c))
	}
	if got := c.module.imports[0]; got.Request != request || got.Call != call || got.Term != importTerm || got.Alias != 0 || got.Key != 0 {
		t.Fatalf("authored Module row = %#v, want Request/Call/Term and zero Alias/Key", got)
	}
	if got := c.source.exact; len(got) != 1 || got[0] != (keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "pkg.core"}) {
		t.Fatalf("raw module exact rows = %#v, want one request atom", got)
	}
	if !order.SetBody(body, call) {
		t.Fatalf("Body fill failed: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	preimage := preparedSourcePreimage(t, prepared)
	atom, ok := preimage.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "pkg.core"})
	if !ok || atom == 0 || preimage.Keys().ExactCount() != 1 {
		t.Fatalf("Source exact denominator omitted module request: atom=%v ok=%v count=%d", atom, ok, preimage.Keys().ExactCount())
	}
	if c.Module().SetImportAlias(importTerm, 0) {
		t.Fatal("Module root mutated terminal Collector after Prepare")
	}
	if err := abortPreparedSourceForTest(t, prepared); err != nil {
		t.Fatalf("Source abort: %v", err)
	}
	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("cleanup Assemble after Source abort = %v/%v", assembly, err)
	}
}

func TestReservedImportSpansAreCensusStableAndRequireFills(t *testing.T) {
	c := New("fixture.lua", 2, bind.GlobalCensus{})
	if got := keyspace.TermFamily(keyspace.MakeTerm(keyspace.FamilyImport, 1)); got != keyspace.FamilyImport || c.counts[keyspace.FamilyImport] != 2 {
		t.Fatalf("reserved Import denominator = %d/%v", c.counts[keyspace.FamilyImport], got)
	}
	if c.Module().fillReservedImport(1, testSpan()) == 0 || c.Module().fillReservedImport(1, testSpan()) != 0 {
		t.Fatal("reserved Import duplicate fill was accepted")
	}
}

func TestSourceFaultsAreOwnedByTheirDedicatedLeaf(t *testing.T) {
	c := New("fault.lua", 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(testSpan())
	fault := c.Source().Faults().ControlFault(testSpan(), body, source.ControlFaultUndefinedGoto, 0, 0)
	if fault == 0 {
		t.Fatalf("ControlFault = zero: %v", failure(c))
	}
	if !c.Source().Order().SetBody(body, fault) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("fault Source order completion failed: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	row, ok := preparedSourcePreimage(t, prepared).Faults().At(fault)
	if !ok || row.Kind != source.ControlFaultUndefinedGoto {
		t.Fatalf("fault Source view = %#v/%v", row, ok)
	}
	if err := abortPreparedSourceForTest(t, prepared); err != nil {
		t.Fatalf("Source abort: %v", err)
	}
	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("cleanup Assemble after Source abort = %v/%v", assembly, err)
	}
}

func TestSourceFaultsEnforceClosedLabelAndBlockerShapes(t *testing.T) {
	newFaultInputs := func() (*Collector, Term, Term, Term) {
		c := New("fault-shapes.lua", 0, bind.GlobalCensus{})
		body := c.Source().Order().Body(testSpan())
		label := c.Flow().Control().Label(testSpan(), body)
		blocker := c.Flow().Storage().Cell(testSpan(), body)
		return c, body, label, blocker
	}

	valid := []struct {
		name           string
		kind           source.ControlFaultKind
		label, blocker bool
	}{
		{"duplicate-label", source.ControlFaultDuplicateLabel, true, false},
		{"undefined-goto", source.ControlFaultUndefinedGoto, false, false},
		{"goto-enters-local", source.ControlFaultGotoEntersLocal, true, true},
		{"break-outside-loop", source.ControlFaultBreakOutsideLoop, false, false},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			c, body, label, blocker := newFaultInputs()
			if got := c.Source().Faults().ControlFault(testSpan(), body, test.kind, chooseTerm(test.label, label), chooseTerm(test.blocker, blocker)); got == 0 {
				t.Fatalf("valid fault rejected: %v", failure(c))
			}
		})
	}

	invalid := []struct {
		name           string
		kind           source.ControlFaultKind
		label, blocker bool
	}{
		{"duplicate-without-label", source.ControlFaultDuplicateLabel, false, false},
		{"undefined-with-label", source.ControlFaultUndefinedGoto, true, false},
		{"break-with-blocker", source.ControlFaultBreakOutsideLoop, false, true},
		{"enters-without-blocker", source.ControlFaultGotoEntersLocal, true, false},
		{"unknown-kind", source.ControlFaultKind(99), false, false},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			c, body, label, blocker := newFaultInputs()
			if got := c.Source().Faults().ControlFault(testSpan(), body, test.kind, chooseTerm(test.label, label), chooseTerm(test.blocker, blocker)); got != 0 || failure(c) == nil {
				t.Fatalf("invalid fault accepted without poisoning: term=%v err=%v", got, failure(c))
			}
		})
	}
	t.Run("future-label", func(t *testing.T) {
		c := New("fault-future-label.lua", 0, bind.GlobalCensus{})
		body := c.Source().Order().Body(testSpan())
		future := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
		if got := c.Source().Faults().ControlFault(testSpan(), body, source.ControlFaultDuplicateLabel, future, 0); got != 0 || failure(c) == nil {
			t.Fatalf("future Label accepted: term=%v err=%v", got, failure(c))
		}
	})
	t.Run("future-blocker", func(t *testing.T) {
		c := New("fault-future-blocker.lua", 0, bind.GlobalCensus{})
		body := c.Source().Order().Body(testSpan())
		label := c.Flow().Control().Label(testSpan(), body)
		future := keyspace.MakeTerm(keyspace.FamilyCell, 1)
		if got := c.Source().Faults().ControlFault(testSpan(), body, source.ControlFaultGotoEntersLocal, label, future); got != 0 || failure(c) == nil {
			t.Fatalf("future blocker Cell accepted: term=%v err=%v", got, failure(c))
		}
	})
}

func chooseTerm(present bool, term Term) Term {
	if present {
		return term
	}
	return 0
}

func sourceFixtureFamilyCount(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyBody,
		keyspace.FamilyTable, keyspace.FamilyReturn:
		return 1
	case keyspace.FamilyValues:
		return 3
	case keyspace.FamilyKey:
		return 2
	case keyspace.FamilyTableField:
		return 2
	default:
		return 0
	}
}

func collectorScratchCleared(c *Collector) bool {
	if c == nil || c.name != "" {
		return false
	}
	return reflect.DeepEqual(c.counts, [keyspace.FamilyCount]uint32{}) &&
		reflect.DeepEqual(c.spans, [keyspace.FamilyCount][]source.Span{}) &&
		reflect.DeepEqual(c.source, sourceRows{}) &&
		reflect.DeepEqual(c.flow, flowRows{}) &&
		reflect.DeepEqual(c.static, staticRows{}) &&
		reflect.DeepEqual(c.module, moduleRows{})
}
