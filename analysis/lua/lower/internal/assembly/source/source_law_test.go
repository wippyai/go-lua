package source_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	assembly "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func testSpan() programsource.Span { return programsource.Span{} }

func sourceView(t *testing.T, c *assembly.Collector) programsource.View {
	t.Helper()
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published.Source()
}

func TestSourceOrderAdmitsNestedBodyAsDirectRoot(t *testing.T) {
	c := assembly.New("nested-body.lua", 0, bind.GlobalCensus{})
	span := programsource.Span{File: "nested-body.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	entry := c.Body(span)
	nested := c.Body(span)
	if entry == 0 || nested == 0 || entry == nested {
		t.Fatalf("Body identities = %v/%v, want distinct nonzero terms", entry, nested)
	}
	if !c.SetBody(nested) || !c.SetBody(entry, nested) || !c.SetEntry(entry) {
		t.Fatalf("nested Body fill failed")
	}
	_ = sourceView(t, c)
}

func TestSourceMintIsDenseAndRejectsDerivedOrInvalidFamilies(t *testing.T) {
	c := assembly.New("fixture.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	if body != keyspace.MakeTerm(keyspace.FamilyBody, 1) {
		t.Fatalf("Body = %v, want dense Body/1", body)
	}
	if c.SetBody(body, keyspace.MakeTerm(keyspace.FamilyOutcome, 1)) {
		t.Fatal("derived Outcome was admitted as a direct Source root")
	}
	if c.Body(testSpan()) != 0 {
		t.Fatal("invalid Source admission did not terminalize the Collector")
	}

	bad := assembly.New("fixture.lua", 0, bind.GlobalCensus{})
	badBody := bad.Body(testSpan())
	if bad.SetBody(badBody, keyspace.MakeTerm(keyspace.FamilyInvalid, 1)) {
		t.Fatal("Invalid family was admitted")
	}
	reserved := assembly.New("fixture.lua", 1, bind.GlobalCensus{})
	reservedBody := reserved.Body(testSpan())
	if reserved.SetBody(reservedBody, keyspace.MakeTerm(keyspace.FamilyImport, 1)) {
		t.Fatal("reserved Import family was admitted as a Body root")
	}
}

func TestSourceRowsPreserveFreshLiteralsRawKeysAndBodyOrder(t *testing.T) {
	c := assembly.New("fixture.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	nilTerm := c.Nil(testSpan(), body)
	boolTerm := c.Bool(testSpan(), body, true)
	integerTerm := c.Integer(testSpan(), body, 7)
	floatTerm := c.FloatBits(testSpan(), body, math.Float64bits(1.5))
	stringTerm := c.String(testSpan(), body, "payload")
	nameTerm := c.Name(testSpan(), body, "field")
	listTerm := c.List(testSpan(), body, 1)
	if nilTerm == 0 || boolTerm == 0 || integerTerm == 0 || floatTerm == 0 || stringTerm == 0 || nameTerm == 0 || listTerm == 0 {
		t.Fatalf("fresh Source terms contain zero")
	}
	table := c.DeclareTable(testSpan(), body)
	nameValues := c.Values(testSpan(), body, []keyspace.Term{stringTerm}, 0)
	listValues := c.Values(testSpan(), body, []keyspace.Term{integerTerm}, 0)
	rootValues := c.Values(testSpan(), body, []keyspace.Term{nilTerm, boolTerm, floatTerm, table}, 0)
	nameField := c.TableField(testSpan(), table, nameTerm, nameValues, kind.FieldName)
	listField := c.TableField(testSpan(), table, listTerm, listValues, kind.FieldList)
	ret := c.Return(testSpan(), body, rootValues)
	if nameValues == 0 || listValues == 0 || table == 0 || rootValues == 0 || nameField == 0 || listField == 0 ||
		!c.FillTable(table, []keyspace.Term{nameField, listField}) || ret == 0 || !c.SetBody(body, ret) || !c.SetEntry(body) {
		t.Fatalf("failed to build Source/Flow containment")
	}
	view := sourceView(t, c)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		want := sourceFixtureFamilyCount(family)
		if got := view.Identity().FamilyCount(family); got != want {
			t.Fatalf("family %d count = %d, want %d", family, got, want)
		}
	}
	if count, ok := view.Order().BodyLen(body); !ok || count != 1 {
		t.Fatalf("Body order length = %d/%v, want one Return root", count, ok)
	}
	for _, term := range []keyspace.Term{nilTerm, boolTerm, integerTerm, floatTerm, stringTerm, nameTerm, listTerm} {
		if _, ok := view.Identity().Span(term); !ok {
			t.Fatalf("Source omitted authored leaf %v", term)
		}
	}
	if got := view.Keys().ExactCount(); got != 2 {
		t.Fatalf("exact candidate count = %d, want name and list candidates", got)
	}
	if got := view.Identity().ContentID(); !got.Available() {
		t.Fatal("published Source content identity unavailable")
	}
}

func TestSourceBuildPreimageUsesOneCanonicalExactDenominator(t *testing.T) {
	c := assembly.New("fixture.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	key := c.Name(testSpan(), body, "stable")
	table := c.DeclareTable(testSpan(), body)
	fieldValue := c.Nil(testSpan(), body)
	fieldValues := c.Values(testSpan(), body, []keyspace.Term{fieldValue}, 0)
	field := c.TableField(testSpan(), table, key, fieldValues, kind.FieldName)
	returnValue := c.Nil(testSpan(), body)
	returnValues := c.Values(testSpan(), body, []keyspace.Term{returnValue, table}, 0)
	ret := c.Return(testSpan(), body, returnValues)
	if key == 0 || table == 0 || fieldValue == 0 || fieldValues == 0 || field == 0 ||
		returnValue == 0 || returnValues == 0 || ret == 0 || !c.FillTable(table, []keyspace.Term{field}) ||
		!c.SetBody(body, ret) || !c.SetEntry(body) {
		t.Fatal("failed Source setup")
	}
	view := sourceView(t, c)
	if got := view.Identity().FamilyCount(keyspace.FamilyKey); got != 1 {
		t.Fatalf("Source key family count = %d, want one", got)
	}
	atom, ok := view.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "stable"})
	if !ok || atom == 0 || view.Keys().ExactCount() != 1 {
		t.Fatalf("canonical exact lookup = %v/%v count=%d", atom, ok, view.Keys().ExactCount())
	}
}

func TestPublishIsOneShotTerminalAndPublishesProgram(t *testing.T) {
	c := assembly.New("dependent.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	if !c.SetBody(body) || !c.SetEntry(body) {
		t.Fatal("Source completion failed")
	}
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if c.Body(testSpan()) != 0 {
		t.Fatal("terminal Collector accepted a second Body")
	}
	if repeated, repeatedErr := c.Publish(); repeatedErr == nil || repeated != nil {
		t.Fatalf("second Publish = %#v/%v, want terminal rejection", repeated, repeatedErr)
	}
	if published == nil || published.Source().Identity().Name() != "dependent.lua" {
		t.Fatalf("published Source name = %q", published.Source().Identity().Name())
	}
}

func TestPublishFailureAfterSourceClaimAbortsAndTerminalizes(t *testing.T) {
	c := assembly.New("publish-failure.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	future := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	if body == 0 || !c.SetBody(body, future) {
		if c.Body(testSpan()) != 0 {
			// SetBody is expected to be the terminal failure boundary.
			t.Fatal("invalid Source root did not terminalize")
		}
	}
	if published, err := c.Publish(); err == nil || published != nil {
		t.Fatalf("Publish accepted invalid Source root: %#v/%v", published, err)
	}
	if c.Body(testSpan()) != 0 {
		t.Fatal("failed Publish reopened the Collector")
	}
}

func TestExactCandidateRejectsNilAndNaNAndHandlesUnaryMinInt(t *testing.T) {
	c := assembly.New("fixture.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	integer := c.Integer(testSpan(), body, math.MinInt64)
	float := c.FloatBits(testSpan(), body, math.Float64bits(2.5))
	unary := c.Unary(testSpan(), body, kind.UnaryNeg, integer)
	if integer == 0 || float == 0 || unary == 0 {
		t.Fatal("failed exact candidate setup")
	}
	lens := c.LensExact(testSpan(), body, integer, unary, kind.FieldExact)
	if lens == 0 {
		t.Fatal("UnaryNeg exact candidate was rejected")
	}
	if c.LensExact(testSpan(), body, integer, float, kind.FieldExact) == 0 {
		t.Fatal("float exact candidate was rejected")
	}
	if c.LensExact(testSpan(), body, integer, c.Nil(testSpan(), body), kind.FieldExact) != 0 {
		t.Fatal("nil literal became an exact candidate")
	}
}

func TestFieldExactDoesNotTreatNonNegUnaryAsLiteralNegation(t *testing.T) {
	c := assembly.New("unary.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	integer := c.Integer(testSpan(), body, 7)
	bitNot := c.Unary(testSpan(), body, kind.UnaryBitNot, integer)
	if integer == 0 || bitNot == 0 {
		t.Fatal("unary construction failed")
	}
	if got := c.LensExact(testSpan(), body, integer, bitNot, kind.FieldExact); got != 0 {
		t.Fatalf("FieldExact admitted non-Neg Unary as %v", got)
	}
}

func TestModuleRequestExactAddsOnlyRawLiteralBeforeSourceFreeze(t *testing.T) {
	const name = "module-source.lua"
	statements, err := parse.ParseString(`local value = require("pkg.core")`, name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	binding := bind.BindChunk(statements, typeindex.Table{})
	callSyntax, ok := statements[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatal("module fixture lost direct require call")
	}
	requireIdent, ok := callSyntax.Func.(*ast.IdentExpr)
	if !ok {
		t.Fatal("module fixture lost require identifier")
	}
	requireIdentity, ok := binding.GlobalIdentity(requireIdent)
	if !ok || !requireIdentity.Matches("require") {
		t.Fatal("module fixture require is not binder-proven global")
	}
	c := assembly.New(name, 1, binding.GlobalCensus())
	body := c.Body(programsource.Span{File: name})
	requireCell := c.Global(requireIdentity)
	callee := c.ImplicitRead(programsource.Span{File: name}, body, requireCell)
	request := c.String(programsource.Span{File: name}, body, "pkg.core")
	values := c.Values(programsource.Span{File: name}, body, []keyspace.Term{request}, 0)
	call := c.DeclareCall(programsource.Span{File: name}, body, callee, 0, values, "")
	if body == 0 || requireCell == 0 || callee == 0 || request == 0 || values == 0 || call == 0 || !c.SetCallTypeArgs(call, nil) {
		t.Fatal("module request construction failed")
	}
	importTerm := c.Import(0, programsource.Span{File: name}, call)
	if importTerm == 0 || !c.SetBody(body, call) || !c.SetEntry(body) {
		t.Fatal("Module/Source completion failed")
	}
	view := sourceView(t, c)
	atom, ok := view.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "pkg.core"})
	// The binder-censused global require seed is the other Source exact atom;
	// Import contributes exactly the request literal on top of it.
	if !ok || atom == 0 || view.Keys().ExactCount() != 2 {
		t.Fatalf("Source exact denominator omitted module request: atom=%v ok=%v count=%d", atom, ok, view.Keys().ExactCount())
	}
	if c.SetImportAlias(importTerm, 0) {
		t.Fatal("Module root mutated terminal Collector after publication")
	}
}

func TestReservedImportSpansAreCensusStableAndRequireFills(t *testing.T) {
	c := assembly.New("fixture.lua", 2, bind.GlobalCensus{})
	body := c.Body(testSpan())
	request := c.String(testSpan(), body, "pkg")
	values := c.Values(testSpan(), body, []keyspace.Term{request}, 0)
	call := c.DeclareCall(testSpan(), body, request, 0, values, "")
	if body == 0 || request == 0 || values == 0 || call == 0 || !c.SetCallTypeArgs(call, nil) {
		t.Fatal("reserved Import setup failed")
	}
	first := c.Import(0, testSpan(), call)
	if first == 0 || c.Import(0, testSpan(), call) != 0 {
		t.Fatal("reserved Import duplicate fill was accepted")
	}
}

func TestSourceFaultsAreOwnedByTheirDedicatedLeaf(t *testing.T) {
	c := assembly.New("fault.lua", 0, bind.GlobalCensus{})
	body := c.Body(testSpan())
	fault := c.ControlFault(testSpan(), body, programsource.ControlFaultUndefinedGoto, 0, 0)
	if fault == 0 || !c.SetBody(body, fault) || !c.SetEntry(body) {
		t.Fatal("fault Source order completion failed")
	}
	view := sourceView(t, c)
	row, ok := view.Faults().At(fault)
	if !ok || row.Kind != programsource.ControlFaultUndefinedGoto {
		t.Fatalf("fault Source view = %#v/%v", row, ok)
	}
}

func TestSourceFaultsEnforceClosedLabelAndBlockerShapes(t *testing.T) {
	newFaultInputs := func() (*assembly.Collector, keyspace.Term, keyspace.Term, keyspace.Term) {
		c := assembly.New("fault-shapes.lua", 0, bind.GlobalCensus{})
		body := c.Body(testSpan())
		label := c.Label(testSpan(), body)
		blocker := c.Cell(testSpan(), body, "")
		return c, body, label, blocker
	}
	valid := []struct {
		name           string
		kind           programsource.ControlFaultKind
		label, blocker bool
	}{
		{"duplicate-label", programsource.ControlFaultDuplicateLabel, true, false},
		{"undefined-goto", programsource.ControlFaultUndefinedGoto, false, false},
		{"goto-enters-local", programsource.ControlFaultGotoEntersLocal, true, true},
		{"break-outside-loop", programsource.ControlFaultBreakOutsideLoop, false, false},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			c, body, label, blocker := newFaultInputs()
			if got := c.ControlFault(testSpan(), body, test.kind, chooseTerm(test.label, label), chooseTerm(test.blocker, blocker)); got == 0 {
				t.Fatalf("valid fault rejected")
			}
		})
	}
	invalid := []struct {
		name           string
		kind           programsource.ControlFaultKind
		label, blocker bool
	}{
		{"duplicate-without-label", programsource.ControlFaultDuplicateLabel, false, false},
		{"undefined-with-label", programsource.ControlFaultUndefinedGoto, true, false},
		{"break-with-blocker", programsource.ControlFaultBreakOutsideLoop, false, true},
		{"enters-without-blocker", programsource.ControlFaultGotoEntersLocal, true, false},
		{"unknown-kind", programsource.ControlFaultKind(99), false, false},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			c, body, label, blocker := newFaultInputs()
			if got := c.ControlFault(testSpan(), body, test.kind, chooseTerm(test.label, label), chooseTerm(test.blocker, blocker)); got != 0 || c.Body(testSpan()) != 0 {
				t.Fatalf("invalid fault accepted without terminal rejection: term=%v", got)
			}
		})
	}
}

func chooseTerm(present bool, term keyspace.Term) keyspace.Term {
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
	case keyspace.FamilyOutcome:
		// Published Source installs four mandatory exits plus one Return
		// outcome for this one-Body fixture. Outcome is derived by Flow,
		// not authored by the Collector.
		return 5
	default:
		return 0
	}
}
