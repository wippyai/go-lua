package authored

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestDenseFlowAuthoredContentChangesIdentity(t *testing.T) {
	authored, terms := flowFixture()
	first := buildFlowForTest(t, authored)
	changed := authored
	changed.Values.Terms = append([]keyspace.Term(nil), authored.Values.Terms...)
	changed.Values.Terms[0] = terms.boolean
	third := buildFlowForTest(t, changed)
	if first.Cold().ContentID() == third.Cold().ContentID() {
		t.Fatal("authored structure did not change ContentID")
	}
}

func TestDenseFlowRejectsBadAuthoredAndDerivedRelations(t *testing.T) {
	authored, terms := flowFixture()
	authored.Values.Terms[0] = terms.values
	if _, err := Build(authored); err == nil {
		t.Fatal("Values accepted a Pack relation as scalar member")
	}

	authored, terms = flowFixture()
	authored.Tables.Fields[0].Table = keyspace.MakeTerm(keyspace.FamilyTable, 2)
	if _, err := Build(authored); err == nil {
		t.Fatal("TableField accepted a foreign Table")
	}

	authored, _ = flowFixture()
	authored.Tables.Fields[0].Kind = 0
	if _, err := Build(authored); err == nil {
		t.Fatal("invalid FieldKind accepted")
	}

	authored, _ = flowFixture()
	authored.Counts[keyspace.FamilyOutcome] = 1
	if _, err := Build(authored); err == nil {
		t.Fatal("authored Outcome universe accepted")
	}

	authored, terms = flowFixture()
	authored.Values.Terms[0] = terms.outcome
	if _, err := Build(authored); err == nil {
		t.Fatal("authored Values accepted derived Outcome")
	}

	authored, _ = flowFixture()
	authored.Tables.Fields[0].Kind = kind.FieldName
	authored.Tables.Fields[0].Key = keyspace.MakeTerm(keyspace.FamilyNil, 1)
	if _, err := Build(authored); err == nil {
		t.Fatal("FieldName accepted non-Key source")
	}

	authored, terms = flowFixture()
	authored.Counts[keyspace.FamilyTableField] = 2
	authored.Tables.Fields = append(authored.Tables.Fields, Field{Table: terms.table, Key: terms.key, Values: terms.values, Kind: kind.FieldName})
	if _, err := Build(authored); err == nil {
		t.Fatal("orphan TableField accepted")
	}

}

func TestDenseFlowAllowsInterleavedGlobalFieldOrdinals(t *testing.T) {
	input := interleavedFieldsFixture()
	component := buildFlowForTest(t, input)
	firstTable := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	if got, ok := component.Tables().FieldAt(firstTable, 1); !ok || got != keyspace.MakeTerm(keyspace.FamilyTableField, 3) {
		t.Fatalf("nested field order = %08x, %v", uint32(got), ok)
	}
}

func TestDenseFlowRejectsScalarOpenTail(t *testing.T) {
	authored, terms := flowFixture()
	authored.Values.Rows[0] = Value{Owner: terms.body, Tail: terms.nil}
	authored.Values.Terms = nil
	authored.Tables.Fields[0].Kind = kind.FieldList
	if _, err := Build(authored); err == nil {
		t.Fatal("Values accepted scalar Nil as an open tail")
	}
}

func TestDenseFlowAllowsOnlyCallOrVarargOpenTail(t *testing.T) {
	authored, terms := functionCallFixture()
	authored.Values.Rows[0] = Value{Owner: terms.owner, Tail: terms.plainCall}
	authored.Values.Terms = nil
	if _, err := Build(authored); err != nil {
		t.Fatalf("Values rejected Call open tail: %v", err)
	}
	authored.Values.Rows[0].Tail = terms.vararg
	if _, err := Build(authored); err != nil {
		t.Fatalf("Values rejected Vararg open tail: %v", err)
	}
}

func TestDenseFlowAcceptsDynamicKeyField(t *testing.T) {
	for _, fieldKind := range []kind.FieldKind{kind.FieldKey} {
		authored, terms := flowFixture()
		authored.Tables.Fields[0].Kind = fieldKind
		authored.Tables.Fields[0].Key = terms.boolean
		component := buildFlowForTest(t, authored)
		if _, key, _, gotKind, ok := component.Fields().Get(terms.field); !ok || key != terms.boolean || gotKind != fieldKind {
			t.Fatalf("dynamic field = key %08x kind %v ok %v", uint32(key), gotKind, ok)
		}
	}
}

func TestDenseFlowFieldExactAndDraftAreNotOverbroadOrCopyable(t *testing.T) {
	authored, terms := flowFixture()
	authored.Tables.Fields[0].Kind = kind.FieldExact
	authored.Tables.Fields[0].Key = terms.boolean
	if component := buildFlowForTest(t, authored); component.Values().Count() == 0 {
		t.Fatal("exact literal field rejected")
	}

	authored, terms = flowFixture()
	authored.Tables.Fields[0].Kind = kind.FieldExact
	authored.Tables.Fields[0].Key = terms.function
	if _, err := Build(authored); err == nil {
		t.Fatal("FieldExact accepted dynamic Function")
	}

	authored, _ = flowFixture()
	draft, err := Build(authored)
	if err != nil {
		t.Fatal(err)
	}
	copyDraft := *draft
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := copyDraft.Finalizer(); err == nil {
		t.Fatal("copied Draft taken twice")
	}
}

func TestDenseFlowRejectsOpenAndHugeBoundaries(t *testing.T) {
	authored, _ := flowFixture()
	authored.Values.Rows[0].Tail = keyspace.MakeTerm(keyspace.FamilyNil, 1)
	if _, err := Build(authored); err == nil {
		t.Fatal("non-final scalar TableField accepted open Values")
	}

	authored, _ = flowFixture()
	authored.Counts[keyspace.FamilyValues] = keyspace.MaxTermOrdinal
	if _, err := Build(authored); err == nil {
		t.Fatal("phantom Values count accepted")
	}

	authored, _ = flowFixture()
	authored.Values.Rows[0].Fixed.End = keyspace.MaxTermOrdinal
	if _, err := Build(authored); err == nil {
		t.Fatal("huge Values member range accepted")
	}

	authored, _ = flowFixture()
	authored.Counts[keyspace.FamilyRead] = keyspace.MaxTermOrdinal + 1
	if _, err := Build(authored); err == nil {
		t.Fatal("count beyond Term ordinal capacity accepted")
	}
}

func TestDenseFlowBuildCopiesAndDraftTakeFences(t *testing.T) {
	authored, terms := flowFixture()
	draft, err := Build(authored)
	if err != nil {
		t.Fatal(err)
	}
	authored.Values.Terms[0] = terms.boolean
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := component.Values().Member(terms.values, 0); got != terms.nil {
		t.Fatal("Build retained caller storage")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("draft take fence reopened")
	}
}

func TestAccessStorageAuthoredLawsAndQueries(t *testing.T) {
	input, terms := accessStorageFixture()
	component := buildFlowForTest(t, input)

	exact := component.Access().Exact()
	if got, ok := exact.At(0); !ok || got != terms.exact {
		t.Fatalf("ExactLens At = %08x, %v", uint32(got), ok)
	}
	if owner, base, source, fieldKind, ok := exact.Get(terms.exact); !ok || owner != terms.body || base != terms.nil || source != terms.key || fieldKind != kind.FieldName {
		t.Fatalf("ExactLens = %08x %08x %08x %v %v", uint32(owner), uint32(base), uint32(source), fieldKind, ok)
	}
	if owner, base, key, ok := component.Access().Dynamic().Get(terms.dynamic); !ok || owner != terms.body || base != terms.nil || key != terms.nil {
		t.Fatalf("DynamicLens = %08x %08x %08x %v", uint32(owner), uint32(base), uint32(key), ok)
	}

	storage := component.Storage()
	if kind, body, key, ok := storage.Cells().Get(terms.local); !ok || kind != CellLocal || body != terms.body || key != 0 {
		t.Fatalf("local Cell = %v %08x %d %v", kind, uint32(body), key, ok)
	}
	if kind, body, key, ok := storage.Cells().Get(terms.global); !ok || kind != CellGlobal || body != 0 || key != 1 {
		t.Fatalf("global Cell = %v %08x %d %v", kind, uint32(body), key, ok)
	}
	if owner, source, implicit, ok := storage.Reads().Get(terms.read); !ok || owner != terms.body || source != terms.global || !implicit {
		t.Fatalf("Read = %08x %08x %v %v", uint32(owner), uint32(source), implicit, ok)
	}
	if got, ok := storage.Reads().ImplicitAt(0); !ok || got != terms.read || storage.Reads().ImplicitCount() != 1 {
		t.Fatalf("implicit Read index = %08x %v (%d)", uint32(got), ok, storage.Reads().ImplicitCount())
	}
	if owner, cell, ok := storage.Varargs().Get(terms.vararg); !ok || owner != terms.body || cell != terms.local {
		t.Fatalf("Vararg = %08x %08x %v", uint32(owner), uint32(cell), ok)
	}
	if owner, values, ok := storage.Binds().Get(terms.bind); !ok || owner != terms.body || values != terms.values {
		t.Fatalf("Bind = %08x %08x %v", uint32(owner), uint32(values), ok)
	}
	if writes, ok := storage.Assigns().WriteCount(terms.assign); !ok || writes != 2 {
		t.Fatalf("Assign WriteCount = %d, %v", writes, ok)
	}
	if got, ok := storage.Assigns().WriteAt(terms.assign, 1); !ok || got != terms.write2 {
		t.Fatalf("Assign WriteAt = %08x, %v", uint32(got), ok)
	}
}

func TestAccessStorageRejectsInvalidAuthoredRows(t *testing.T) {
	input, terms := accessStorageFixture()
	input.Storage.Cells[0] = Cell{Kind: CellGlobal, Key: 1}
	if _, err := Build(input); err == nil {
		t.Fatal("duplicate global Cell accepted")
	}

	input, terms = accessStorageFixture()
	input.Storage.Reads[0].Source = terms.local
	if _, err := Build(input); err == nil {
		t.Fatal("implicit local Read accepted")
	}

	input, terms = accessStorageFixture()
	input.Storage.Varargs[0].Cell = terms.global
	if _, err := Build(input); err == nil {
		t.Fatal("global Vararg Cell accepted")
	}

	input, _ = accessStorageFixture()
	input.Storage.Writes = input.Storage.Writes[:1]
	if _, err := Build(input); err == nil {
		t.Fatal("Assign with incomplete Write range accepted")
	}

	input, _ = accessStorageFixture()
	input.Storage.Writes[1].Assign = 0
	if _, err := Build(input); err == nil {
		t.Fatal("orphan Write accepted")
	}
}

func TestGlobalCellUsesSourceAtomHandleWithoutKeyTerm(t *testing.T) {
	input, _ := accessStorageFixture()
	input.Access.Exact = nil
	input.Counts[keyspace.FamilyLensExact] = 0
	input.Counts[keyspace.FamilyKey] = 0
	if _, err := Build(input); err != nil {
		t.Fatalf("global Cell with atom handle but no Key Term rejected: %v", err)
	}
}

func TestFunctionCallAuthoredLawsQueriesCopiesAndContent(t *testing.T) {
	input, terms := functionCallFixture()
	first := buildFlowForTest(t, input)
	functions := first.Functions()
	if got, ok := functions.At(0); !ok || got != terms.function {
		t.Fatalf("Function At = %08x, %v", uint32(got), ok)
	}
	if owner, body, vararg, ok := functions.Get(terms.function); !ok || owner != terms.owner || body != terms.functionBody || vararg != terms.varargCell {
		t.Fatalf("Function = %08x %08x %08x %v", uint32(owner), uint32(body), uint32(vararg), ok)
	}
	if count, ok := functions.CaptureCount(terms.function); !ok || count != 1 {
		t.Fatalf("Function CaptureCount = %d, %v", count, ok)
	}
	if inner, outer, ok := functions.CaptureAt(terms.function, 0); !ok || inner != terms.inner || outer != terms.outer {
		t.Fatalf("Function CaptureAt = %08x %08x %v", uint32(inner), uint32(outer), ok)
	}
	calls := first.Calls()
	if got, ok := calls.At(0); !ok || got != terms.methodCall {
		t.Fatalf("Call At = %08x, %v", uint32(got), ok)
	}
	if owner, callee, receiver, actuals, ok := calls.Get(terms.methodCall); !ok || owner != terms.owner || callee != terms.read || receiver != terms.nil || actuals != terms.values {
		t.Fatalf("method Call = %08x %08x %08x %08x %v", uint32(owner), uint32(callee), uint32(receiver), uint32(actuals), ok)
	}
	if got, ok := first.Values().Member(terms.values, 0); !ok || got != terms.vararg {
		t.Fatalf("Values Vararg member = %08x, %v", uint32(got), ok)
	}
	if got, ok := first.Values().Member(terms.values, 1); !ok || got != terms.methodCall {
		t.Fatalf("Values Call member = %08x, %v", uint32(got), ok)
	}

	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Functions.Rows[0].Vararg = 0
	input.Functions.Captures[0].Outer = terms.inner
	input.Calls[0].Receiver = 0
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	copyComponent, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, vararg, ok := copyComponent.Functions().Get(terms.function); !ok || vararg != terms.varargCell {
		t.Fatal("Build retained caller Function storage")
	}
	if _, outer, ok := copyComponent.Functions().CaptureAt(terms.function, 0); !ok || outer != terms.outer {
		t.Fatal("Build retained caller Capture storage")
	}
	if _, _, receiver, _, ok := copyComponent.Calls().Get(terms.methodCall); !ok || receiver != terms.nil {
		t.Fatal("Build retained caller Call storage")
	}

	unchanged, unchangedTerms := functionCallFixture()
	second := buildFlowForTest(t, unchanged)
	if first.Cold().ContentID() != second.Cold().ContentID() || unchangedTerms.function != terms.function {
		t.Fatal("equal Function/Call authored content has unstable identity")
	}
	changed, _ := functionCallFixture()
	changed.Functions.Rows[0].Vararg = 0
	third := buildFlowForTest(t, changed)
	if first.Cold().ContentID() == third.Cold().ContentID() {
		t.Fatal("Function authored content did not change identity")
	}
	changed, _ = functionCallFixture()
	changed.Functions.Captures[0].Outer = terms.outer2
	fourth := buildFlowForTest(t, changed)
	if first.Cold().ContentID() == fourth.Cold().ContentID() {
		t.Fatal("Capture authored content did not change identity")
	}
	changed, _ = functionCallFixture()
	changed.Calls[1].Callee = terms.nil
	fifth := buildFlowForTest(t, changed)
	if first.Cold().ContentID() == fifth.Cold().ContentID() {
		t.Fatal("Call authored content did not change identity")
	}
}

func TestAuthoredControlLawsQueriesCopiesAndContent(t *testing.T) {
	input, terms := controlFixture()
	first := buildFlowForTest(t, input)
	control := first.Control()
	if got, ok := control.Returns().At(0); !ok || got != terms.returned {
		t.Fatalf("Return At = %08x, %v", uint32(got), ok)
	}
	if owner, values, ok := control.Returns().Get(terms.returned); !ok || owner != terms.branchTrue || values != terms.numericValues {
		t.Fatalf("Return = %08x %08x %v", uint32(owner), uint32(values), ok)
	}
	if owner, ok := control.Breaks().Get(terms.broken); !ok || owner != terms.loopBody {
		t.Fatalf("Break = %08x %v", uint32(owner), ok)
	}
	if owner, ok := control.Labels().Get(terms.label); !ok || owner != terms.owner {
		t.Fatalf("Label = %08x %v", uint32(owner), ok)
	}
	if owner, target, ok := control.Gotos().Get(terms.gotoTerm); !ok || owner != terms.owner || target != terms.label {
		t.Fatalf("Goto = %08x %08x %v", uint32(owner), uint32(target), ok)
	}
	if owner, condition, yes, no, ok := control.Branches().Get(terms.branch); !ok || owner != terms.owner || condition != terms.nil || yes != terms.branchTrue || no != terms.branchFalse {
		t.Fatalf("Branch = %08x %08x %08x %08x %v", uint32(owner), uint32(condition), uint32(yes), uint32(no), ok)
	}
	if owner, body, loopKind, header, ok := control.Loops().Get(terms.numericLoop); !ok || owner != terms.owner || body != terms.loopBody || loopKind != kind.LoopNumericFor || header != terms.numericValues {
		t.Fatalf("numeric Loop = %08x %08x %v %08x %v", uint32(owner), uint32(body), loopKind, uint32(header), ok)
	}
	if cells, ok := control.Loops().CellCount(terms.genericLoop); !ok || cells != 2 {
		t.Fatalf("generic Loop CellCount = %d, %v", cells, ok)
	}
	if cell, ok := control.Loops().CellAt(terms.genericLoop, 1); !ok || cell != terms.genericCell2 {
		t.Fatalf("generic Loop CellAt = %08x, %v", uint32(cell), ok)
	}

	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Control.Returns[0].Owner = terms.owner
	input.Control.Cells[0] = terms.genericCell1
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	copied, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if owner, _, ok := copied.Control().Returns().Get(terms.returned); !ok || owner != terms.branchTrue {
		t.Fatal("Build retained caller control rows")
	}
	if cell, ok := copied.Control().Loops().CellAt(terms.numericLoop, 0); !ok || cell != terms.numericCell {
		t.Fatal("Build retained caller Loop Cell pool")
	}

	changed, _ := controlFixture()
	changed.Control.Gotos[0].Owner = terms.branchFalse
	second := buildFlowForTest(t, changed)
	if first.Cold().ContentID() == second.Cold().ContentID() {
		t.Fatal("authored control did not change ContentID")
	}
}

func TestAuthoredControlContentSensitivity(t *testing.T) {
	baseline, _ := controlFixture()
	want := buildFlowForTest(t, baseline).Cold().ContentID()
	tests := []struct {
		name   string
		mutate func(*Input, controlTerms)
	}{
		{"Return", func(input *Input, terms controlTerms) { input.Control.Returns[0].Values = terms.genericValues }},
		{"Break", func(input *Input, terms controlTerms) { input.Control.Breaks[0].Owner = terms.branchFalse }},
		{"Label", func(input *Input, terms controlTerms) { input.Control.Labels[1].Owner = terms.branchFalse }},
		{"Goto", func(input *Input, terms controlTerms) { input.Control.Gotos[0].Target = terms.label2 }},
		{"Branch", func(input *Input, terms controlTerms) { input.Control.Branches[0].Condition = terms.boolean }},
		{"Loop", func(input *Input, terms controlTerms) { input.Control.Loops[1].Owner = terms.branchFalse }},
		{"LoopCells", func(input *Input, _ controlTerms) {
			input.Control.Cells[1], input.Control.Cells[2] = input.Control.Cells[2], input.Control.Cells[1]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, terms := controlFixture()
			test.mutate(&input, terms)
			got := buildFlowForTest(t, input).Cold().ContentID()
			if got == want {
				t.Fatal("authored control mutation did not change ContentID")
			}
		})
	}
}

func TestAuthoredControlQueryBounds(t *testing.T) {
	input, terms := controlFixture()
	control := buildFlowForTest(t, input).Control()
	if _, ok := control.Returns().At(-1); ok {
		t.Fatal("Return At accepted negative index")
	}
	if _, ok := control.Returns().At(control.Returns().Count()); ok {
		t.Fatal("Return At accepted end index")
	}
	if _, _, ok := control.Returns().Get(terms.nil); ok {
		t.Fatal("Return Get accepted wrong family")
	}
	if _, ok := control.Breaks().At(-1); ok {
		t.Fatal("Break At accepted negative index")
	}
	if _, ok := control.Breaks().At(control.Breaks().Count()); ok {
		t.Fatal("Break At accepted end index")
	}
	if _, ok := control.Breaks().Get(terms.nil); ok {
		t.Fatal("Break Get accepted wrong family")
	}
	if _, ok := control.Labels().At(-1); ok {
		t.Fatal("Label At accepted negative index")
	}
	if _, ok := control.Labels().At(control.Labels().Count()); ok {
		t.Fatal("Label At accepted end index")
	}
	if _, ok := control.Labels().Get(terms.nil); ok {
		t.Fatal("Label Get accepted wrong family")
	}
	if _, ok := control.Gotos().At(-1); ok {
		t.Fatal("Goto At accepted negative index")
	}
	if _, ok := control.Gotos().At(control.Gotos().Count()); ok {
		t.Fatal("Goto At accepted end index")
	}
	if _, _, ok := control.Gotos().Get(terms.nil); ok {
		t.Fatal("Goto Get accepted wrong family")
	}
	if _, ok := control.Branches().At(-1); ok {
		t.Fatal("Branch At accepted negative index")
	}
	if _, ok := control.Branches().At(control.Branches().Count()); ok {
		t.Fatal("Branch At accepted end index")
	}
	if _, _, _, _, ok := control.Branches().Get(terms.nil); ok {
		t.Fatal("Branch Get accepted wrong family")
	}
	if _, ok := control.Loops().At(-1); ok {
		t.Fatal("Loop At accepted negative index")
	}
	if _, ok := control.Loops().At(control.Loops().Count()); ok {
		t.Fatal("Loop At accepted end index")
	}
	if _, _, _, _, ok := control.Loops().Get(terms.nil); ok {
		t.Fatal("Loop Get accepted wrong family")
	}
	if _, ok := control.Loops().CellAt(terms.genericLoop, -1); ok {
		t.Fatal("Loop CellAt accepted negative index")
	}
	if _, ok := control.Loops().CellAt(terms.genericLoop, 2); ok {
		t.Fatal("Loop CellAt accepted end index")
	}
	if _, ok := control.Loops().CellCount(terms.nil); ok {
		t.Fatal("Loop CellCount accepted wrong family")
	}

	var zero Control
	if zero.Returns().Count() != 0 || zero.Loops().Count() != 0 {
		t.Fatal("nil Control view reported rows")
	}
}

func TestScalarLoopShapes(t *testing.T) {
	for _, loopKind := range []kind.LoopKind{kind.LoopWhile, kind.LoopRepeat} {
		t.Run(kindName(loopKind), func(t *testing.T) {
			input := scalarLoopFixture(loopKind)
			if _, err := Build(input); err != nil {
				t.Fatalf("valid scalar loop rejected: %v", err)
			}

			withCells := scalarLoopFixture(loopKind)
			withCells.Counts[keyspace.FamilyCell] = 1
			withCells.Storage.Cells = []Cell{{Kind: CellLocal, Body: keyspace.MakeTerm(keyspace.FamilyBody, 2)}}
			withCells.Control.Cells = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}
			withCells.Control.Loops[0].Cells.End = 1
			if _, err := Build(withCells); err == nil {
				t.Fatal("scalar loop accepted iteration Cell")
			}

			nonscalar := scalarLoopFixture(loopKind)
			nonscalar.Control.Loops[0].Control = keyspace.MakeTerm(keyspace.FamilyLoop, 1)
			if _, err := Build(nonscalar); err == nil {
				t.Fatal("scalar loop accepted non-scalar control")
			}
		})
	}
}

func kindName(loopKind kind.LoopKind) string {
	if loopKind == kind.LoopWhile {
		return "while"
	}
	return "repeat"
}

func scalarLoopFixture(loopKind kind.LoopKind) Input {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyLoop] = 1
	return Input{Counts: counts, Control: ControlInput{Loops: []Loop{{
		Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Body: keyspace.MakeTerm(keyspace.FamilyBody, 2),
		Kind: loopKind, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1),
	}}}}
}

func TestAuthoredControlRejectsHostileShapes(t *testing.T) {
	for _, family := range []keyspace.Family{
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyLabel,
		keyspace.FamilyGoto, keyspace.FamilyBranch, keyspace.FamilyLoop,
	} {
		input, _ := controlFixture()
		input.Counts[family] = 0
		if _, err := Build(input); err == nil {
			t.Fatalf("%v count mismatch accepted", family)
		}
	}

	ownerTests := []struct {
		name   string
		mutate func(*Input)
	}{
		{"Return", func(input *Input) { input.Control.Returns[0].Owner = 0 }},
		{"Break", func(input *Input) { input.Control.Breaks[0].Owner = 0 }},
		{"Label", func(input *Input) { input.Control.Labels[0].Owner = 0 }},
		{"Goto", func(input *Input) { input.Control.Gotos[0].Owner = 0 }},
		{"Branch", func(input *Input) { input.Control.Branches[0].Owner = 0 }},
		{"Loop", func(input *Input) { input.Control.Loops[0].Owner = 0 }},
	}
	for _, test := range ownerTests {
		t.Run(test.name+"Owner", func(t *testing.T) {
			input, _ := controlFixture()
			test.mutate(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("invalid owner accepted")
			}
		})
	}

	input, terms := controlFixture()
	input.Control.Returns[0].Values = 0
	if _, err := Build(input); err == nil {
		t.Fatal("Return accepted absent Values")
	}

	input, terms = controlFixture()
	input.Control.Returns[0].Values = terms.nil
	if _, err := Build(input); err == nil {
		t.Fatal("Return accepted scalar in place of Values")
	}

	input, terms = controlFixture()
	input.Control.Gotos[0].Target = terms.branch
	if _, err := Build(input); err == nil {
		t.Fatal("Goto accepted non-Label target")
	}

	input, terms = controlFixture()
	input.Control.Branches[0].Condition = terms.numericValues
	if _, err := Build(input); err == nil {
		t.Fatal("Branch accepted Values condition")
	}

	input, terms = controlFixture()
	input.Control.Branches[0].WhenTrue = 0
	if _, err := Build(input); err == nil {
		t.Fatal("Branch accepted absent arm")
	}

	input, terms = controlFixture()
	input.Control.Branches[0].WhenTrue = terms.owner
	if _, err := Build(input); err == nil {
		t.Fatal("Branch accepted self arm")
	}

	input, terms = controlFixture()
	input.Control.Branches[0].WhenFalse = terms.branchTrue
	if _, err := Build(input); err == nil {
		t.Fatal("Branch accepted duplicate arms")
	}

	input, terms = controlFixture()
	input.Control.Loops[0].Cells.End = 0
	if _, err := Build(input); err == nil {
		t.Fatal("NumericFor accepted no local Cell")
	}

	input, terms = controlFixture()
	input.Control.Loops[0].Control = terms.genericValues
	if _, err := Build(input); err == nil {
		t.Fatal("NumericFor accepted non-2/3 Values header")
	}

	input, terms = controlFixture()
	input.Control.Loops[1].Control = terms.emptyValues
	if _, err := Build(input); err == nil {
		t.Fatal("GenericFor accepted empty Values header")
	}

	input, terms = controlFixture()
	input.Control.Cells[2] = terms.genericCell1
	if _, err := Build(input); err == nil {
		t.Fatal("GenericFor accepted duplicate Cells")
	}

	input, terms = controlFixture()
	input.Control.Cells[0] = terms.genericCell1
	if _, err := Build(input); err == nil {
		t.Fatal("Loop accepted Cell from another Body")
	}

	input, terms = controlFixture()
	input.Control.Loops[1].Cells.Start = 0
	if _, err := Build(input); err == nil {
		t.Fatal("Loop Cell CSR overlap accepted")
	}

	input, terms = controlFixture()
	input.Functions.Rows[0].Body = terms.branchTrue
	if _, err := Build(input); err == nil {
		t.Fatal("shared Function/Branch child Body accepted")
	}

	input, terms = controlFixture()
	input.Functions.Rows[0].Body = terms.loopBody
	if _, err := Build(input); err == nil {
		t.Fatal("shared Function/Loop child Body accepted")
	}

	input, terms = controlFixture()
	input.Control.Branches[0].WhenTrue = terms.loopBody
	if _, err := Build(input); err == nil {
		t.Fatal("shared Branch/Loop child Body accepted")
	}

	input, _ = controlFixture()
	input.Counts[keyspace.FamilyControlFault] = 1
	if _, err := Build(input); err != nil {
		t.Fatalf("Source-owned ControlFault count rejected: %v", err)
	}
}

func TestFunctionCallRejectsInvalidAuthoredRelations(t *testing.T) {
	input, terms := functionCallFixture()
	input.Functions.Rows[0].Body = terms.owner
	if _, err := Build(input); err == nil {
		t.Fatal("self Function Body accepted")
	}

	input, terms = functionCallFixture()
	input.Counts[keyspace.FamilyFunction] = 2
	input.Functions.Rows = append(input.Functions.Rows, Function{Owner: terms.owner, Body: terms.functionBody, Captures: Range{Start: 1, End: 1}})
	if _, err := Build(input); err == nil {
		t.Fatal("duplicate Function Body accepted")
	}

	input, terms = functionCallFixture()
	input.Functions.Rows[0].Vararg = terms.outer
	if _, err := Build(input); err == nil {
		t.Fatal("Function accepted vararg Cell from the wrong body")
	}

	input, terms = functionCallFixture()
	input.Functions.Captures[0].Inner = terms.outer
	if _, err := Build(input); err == nil {
		t.Fatal("Function accepted Capture Inner from the wrong body")
	}

	input, terms = functionCallFixture()
	input.Functions.Captures[0].Outer = terms.inner
	if _, err := Build(input); err == nil {
		t.Fatal("Function accepted self Capture Outer")
	}

	input, terms = functionCallFixture()
	input.Functions.Captures[0].Outer = terms.varargCell
	if _, err := Build(input); err == nil {
		t.Fatal("Function accepted same-body Capture Outer")
	}

	input, terms = functionCallFixture()
	input.Functions.Captures = append(input.Functions.Captures, Capture{Inner: terms.inner, Outer: terms.outer})
	input.Functions.Rows[0].Captures.End = 2
	if _, err := Build(input); err == nil {
		t.Fatal("duplicate Capture Inner accepted")
	}

	input, terms = functionCallFixture()
	input.Functions.Captures = append(input.Functions.Captures, Capture{Inner: terms.varargCell, Outer: terms.outer})
	input.Functions.Rows[0].Captures.End = 2
	if _, err := Build(input); err == nil {
		t.Fatal("duplicate Function Capture Outer accepted")
	}

	input, terms = functionCallFixture()
	input.Calls[0].Receiver = terms.vararg
	if _, err := Build(input); err == nil {
		t.Fatal("method Call receiver mismatching ExactLens base accepted")
	}

	input, terms = functionCallFixture()
	input.Calls[0].Callee = terms.vararg
	if _, err := Build(input); err == nil {
		t.Fatal("receiver-bearing plain Call accepted")
	}

	input, terms = functionCallFixture()
	input.Calls[0].Owner = terms.functionBody
	if _, err := Build(input); err == nil {
		t.Fatal("method Call with disagreeing Flow owners accepted")
	}

	input, _ = functionCallFixture()
	input.Functions.Rows[0].Captures.Start = 1
	if _, err := Build(input); err == nil {
		t.Fatal("Function Capture ranges accepted a gap")
	}
}

type fixtureTerms struct{ body, functionBody, nil, boolean, values, table, key, field, outcome, function keyspace.Term }

type accessStorageTerms struct {
	body, nil, values, key, exact, dynamic, local, global, read, vararg, bind, assign, write1, write2 keyspace.Term
}

type functionCallTerms struct {
	owner, functionBody, nil, values, key, exact, read, outer, outer2, inner, varargCell, vararg, function, methodCall, plainCall keyspace.Term
}

type controlTerms struct {
	owner, branchTrue, branchFalse, loopBody, genericBody, functionBody                   keyspace.Term
	nil, boolean, numericValues, genericValues, emptyValues                               keyspace.Term
	numericCell, genericCell1, genericCell2                                               keyspace.Term
	returned, broken, label, label2, gotoTerm, branch, numericLoop, genericLoop, function keyspace.Term
}

func controlFixture() (Input, controlTerms) {
	terms := controlTerms{
		owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), branchTrue: keyspace.MakeTerm(keyspace.FamilyBody, 2), branchFalse: keyspace.MakeTerm(keyspace.FamilyBody, 3),
		loopBody: keyspace.MakeTerm(keyspace.FamilyBody, 4), genericBody: keyspace.MakeTerm(keyspace.FamilyBody, 5), functionBody: keyspace.MakeTerm(keyspace.FamilyBody, 6),
		nil: keyspace.MakeTerm(keyspace.FamilyNil, 1), boolean: keyspace.MakeTerm(keyspace.FamilyBool, 1), numericValues: keyspace.MakeTerm(keyspace.FamilyValues, 1), genericValues: keyspace.MakeTerm(keyspace.FamilyValues, 2), emptyValues: keyspace.MakeTerm(keyspace.FamilyValues, 3),
		numericCell: keyspace.MakeTerm(keyspace.FamilyCell, 1), genericCell1: keyspace.MakeTerm(keyspace.FamilyCell, 2), genericCell2: keyspace.MakeTerm(keyspace.FamilyCell, 3),
		returned: keyspace.MakeTerm(keyspace.FamilyReturn, 1), broken: keyspace.MakeTerm(keyspace.FamilyBreak, 1), label: keyspace.MakeTerm(keyspace.FamilyLabel, 1), label2: keyspace.MakeTerm(keyspace.FamilyLabel, 2), gotoTerm: keyspace.MakeTerm(keyspace.FamilyGoto, 1),
		branch: keyspace.MakeTerm(keyspace.FamilyBranch, 1), numericLoop: keyspace.MakeTerm(keyspace.FamilyLoop, 1), genericLoop: keyspace.MakeTerm(keyspace.FamilyLoop, 2), function: keyspace.MakeTerm(keyspace.FamilyFunction, 1),
	}
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{terms.owner, terms.branchTrue, terms.branchFalse, terms.loopBody, terms.genericBody, terms.functionBody, terms.nil, terms.boolean, terms.numericValues, terms.genericValues, terms.emptyValues, terms.numericCell, terms.genericCell1, terms.genericCell2, terms.returned, terms.broken, terms.label, terms.label2, terms.gotoTerm, terms.branch, terms.numericLoop, terms.genericLoop, terms.function} {
		counts[keyspace.TermFamily(term)]++
	}
	return Input{Counts: counts,
		Values: ValuesInput{Rows: []Value{
			{Owner: terms.branchTrue, Fixed: Range{Start: 0, End: 2}}, {Owner: terms.owner, Fixed: Range{Start: 2, End: 3}}, {Owner: terms.owner, Fixed: Range{Start: 3, End: 3}},
		}, Terms: []keyspace.Term{terms.nil, terms.nil, terms.nil}},
		Storage:   StorageInput{Cells: []Cell{{Kind: CellLocal, Body: terms.loopBody}, {Kind: CellLocal, Body: terms.genericBody}, {Kind: CellLocal, Body: terms.genericBody}}},
		Functions: FunctionsInput{Rows: []Function{{Owner: terms.owner, Body: terms.functionBody}}},
		Control: ControlInput{
			Returns: []Return{{Owner: terms.branchTrue, Values: terms.numericValues}}, Breaks: []Break{{Owner: terms.loopBody}}, Labels: []Label{{Owner: terms.owner}, {Owner: terms.owner}}, Gotos: []Goto{{Owner: terms.owner, Target: terms.label}},
			Branches: []Branch{{Owner: terms.owner, Condition: terms.nil, WhenTrue: terms.branchTrue, WhenFalse: terms.branchFalse}},
			Loops:    []Loop{{Owner: terms.owner, Body: terms.loopBody, Kind: kind.LoopNumericFor, Control: terms.numericValues, Cells: Range{Start: 0, End: 1}}, {Owner: terms.owner, Body: terms.genericBody, Kind: kind.LoopGenericFor, Control: terms.genericValues, Cells: Range{Start: 1, End: 3}}},
			Cells:    []keyspace.Term{terms.numericCell, terms.genericCell1, terms.genericCell2},
		},
	}, terms
}

func functionCallFixture() (Input, functionCallTerms) {
	terms := functionCallTerms{
		owner:        keyspace.MakeTerm(keyspace.FamilyBody, 1),
		functionBody: keyspace.MakeTerm(keyspace.FamilyBody, 2),
		nil:          keyspace.MakeTerm(keyspace.FamilyNil, 1),
		values:       keyspace.MakeTerm(keyspace.FamilyValues, 1),
		key:          keyspace.MakeTerm(keyspace.FamilyKey, 1),
		exact:        keyspace.MakeTerm(keyspace.FamilyLensExact, 1),
		read:         keyspace.MakeTerm(keyspace.FamilyRead, 1),
		outer:        keyspace.MakeTerm(keyspace.FamilyCell, 1),
		outer2:       keyspace.MakeTerm(keyspace.FamilyCell, 2),
		inner:        keyspace.MakeTerm(keyspace.FamilyCell, 3),
		varargCell:   keyspace.MakeTerm(keyspace.FamilyCell, 4),
		vararg:       keyspace.MakeTerm(keyspace.FamilyVararg, 1),
		function:     keyspace.MakeTerm(keyspace.FamilyFunction, 1),
		methodCall:   keyspace.MakeTerm(keyspace.FamilyCall, 1),
		plainCall:    keyspace.MakeTerm(keyspace.FamilyCall, 2),
	}
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{
		terms.owner, terms.functionBody, terms.nil, terms.values, terms.key, terms.exact, terms.read,
		terms.outer, terms.outer2, terms.inner, terms.varargCell, terms.vararg, terms.function, terms.methodCall, terms.plainCall,
	} {
		counts[keyspace.TermFamily(term)]++
	}
	return Input{Counts: counts,
		Values: ValuesInput{Rows: []Value{{Owner: terms.owner, Fixed: Range{End: 2}}}, Terms: []keyspace.Term{terms.vararg, terms.methodCall}},
		Access: AccessInput{Exact: []ExactLens{{Owner: terms.owner, Base: terms.nil, Source: terms.key, Kind: kind.FieldName}}},
		Storage: StorageInput{
			Cells:   []Cell{{Kind: CellLocal, Body: terms.owner}, {Kind: CellLocal, Body: terms.owner}, {Kind: CellLocal, Body: terms.functionBody}, {Kind: CellLocal, Body: terms.functionBody}},
			Reads:   []Read{{Owner: terms.owner, Source: terms.exact}},
			Varargs: []Vararg{{Owner: terms.functionBody, Cell: terms.varargCell}},
		},
		Functions: FunctionsInput{Rows: []Function{{Owner: terms.owner, Body: terms.functionBody, Vararg: terms.varargCell, Captures: Range{End: 1}}}, Captures: []Capture{{Inner: terms.inner, Outer: terms.outer}}},
		Calls:     []Call{{Owner: terms.owner, Callee: terms.read, Receiver: terms.nil, Actuals: terms.values}, {Owner: terms.owner, Callee: terms.vararg, Actuals: terms.values}},
	}, terms
}

func accessStorageFixture() (Input, accessStorageTerms) {
	terms := accessStorageTerms{
		body: keyspace.MakeTerm(keyspace.FamilyBody, 1), nil: keyspace.MakeTerm(keyspace.FamilyNil, 1),
		values: keyspace.MakeTerm(keyspace.FamilyValues, 1), key: keyspace.MakeTerm(keyspace.FamilyKey, 1),
		exact: keyspace.MakeTerm(keyspace.FamilyLensExact, 1), dynamic: keyspace.MakeTerm(keyspace.FamilyLensKey, 1),
		local: keyspace.MakeTerm(keyspace.FamilyCell, 1), global: keyspace.MakeTerm(keyspace.FamilyCell, 2),
		read: keyspace.MakeTerm(keyspace.FamilyRead, 1), vararg: keyspace.MakeTerm(keyspace.FamilyVararg, 1),
		bind: keyspace.MakeTerm(keyspace.FamilyBind, 1), assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1),
		write1: keyspace.MakeTerm(keyspace.FamilyWrite, 1), write2: keyspace.MakeTerm(keyspace.FamilyWrite, 2),
	}
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{terms.body, terms.nil, terms.values, terms.key, terms.exact, terms.dynamic, terms.local, terms.global, terms.read, terms.vararg, terms.bind, terms.assign, terms.write1, terms.write2} {
		counts[keyspace.TermFamily(term)]++
	}
	input := Input{Counts: counts,
		Values: ValuesInput{Rows: []Value{{Owner: terms.body, Fixed: Range{End: 1}}}, Terms: []keyspace.Term{terms.nil}},
		Access: AccessInput{Exact: []ExactLens{{Owner: terms.body, Base: terms.nil, Source: terms.key, Kind: kind.FieldName}}, Dynamic: []DynamicLens{{Owner: terms.body, Base: terms.nil, Key: terms.nil}}},
		Storage: StorageInput{
			Cells: []Cell{{Kind: CellLocal, Body: terms.body}, {Kind: CellGlobal, Key: 1}},
			Reads: []Read{{Owner: terms.body, Source: terms.global, Implicit: true}}, Varargs: []Vararg{{Owner: terms.body, Cell: terms.local}},
			Binds: []Bind{{Owner: terms.body, Values: terms.values}}, Assigns: []Assign{{Owner: terms.body, Values: terms.values}},
			Writes: []Write{{Assign: terms.assign, Target: terms.local}, {Assign: terms.assign, Target: terms.dynamic}},
		},
	}
	return input, terms
}

func flowFixture() (Input, fixtureTerms) {
	terms := fixtureTerms{
		body: keyspace.MakeTerm(keyspace.FamilyBody, 1), nil: keyspace.MakeTerm(keyspace.FamilyNil, 1),
		boolean: keyspace.MakeTerm(keyspace.FamilyBool, 1), values: keyspace.MakeTerm(keyspace.FamilyValues, 1),
		table: keyspace.MakeTerm(keyspace.FamilyTable, 1), key: keyspace.MakeTerm(keyspace.FamilyKey, 1),
		field: keyspace.MakeTerm(keyspace.FamilyTableField, 1), outcome: keyspace.MakeTerm(keyspace.FamilyOutcome, 1),
		function: keyspace.MakeTerm(keyspace.FamilyFunction, 1),
	}
	terms.functionBody = keyspace.MakeTerm(keyspace.FamilyBody, 2)
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{terms.body, terms.functionBody, terms.nil, terms.boolean, terms.values, terms.table, terms.key, terms.field, terms.function} {
		counts[keyspace.TermFamily(term)]++
	}
	return Input{
		Counts: counts,
		Values: ValuesInput{Rows: []Value{{Owner: terms.body, Fixed: Range{End: 1}}}, Terms: []keyspace.Term{terms.nil}},
		Tables: TablesInput{
			Rows:   []Table{{Owner: terms.body, Fields: Range{End: 1}}},
			Fields: []Field{{Table: terms.table, Key: terms.key, Values: terms.values, Kind: kind.FieldName}},
			Order:  []keyspace.Term{terms.field},
		},
		Functions: FunctionsInput{Rows: []Function{{Owner: terms.body, Body: terms.functionBody}}},
	}, terms
}

func buildFlowForTest(t *testing.T, input Input) View {
	t.Helper()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func interleavedFieldsFixture() Input {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody], counts[keyspace.FamilyNil], counts[keyspace.FamilyValues] = 1, 1, 3
	counts[keyspace.FamilyTable], counts[keyspace.FamilyKey], counts[keyspace.FamilyTableField] = 2, 3, 3
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2), keyspace.MakeTerm(keyspace.FamilyValues, 3)}
	tables := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTable, 1), keyspace.MakeTerm(keyspace.FamilyTable, 2)}
	keys := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyKey, 1), keyspace.MakeTerm(keyspace.FamilyKey, 2), keyspace.MakeTerm(keyspace.FamilyKey, 3)}
	fields := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTableField, 1), keyspace.MakeTerm(keyspace.FamilyTableField, 2), keyspace.MakeTerm(keyspace.FamilyTableField, 3)}
	input := Input{Counts: counts,
		Values: ValuesInput{Rows: []Value{{Owner: body, Fixed: Range{Start: 0, End: 1}}, {Owner: body, Fixed: Range{Start: 1, End: 2}}, {Owner: body, Fixed: Range{Start: 2, End: 3}}}, Terms: []keyspace.Term{nilTerm, nilTerm, nilTerm}},
		Tables: TablesInput{Rows: []Table{{Owner: body, Fields: Range{Start: 0, End: 2}}, {Owner: body, Fields: Range{Start: 2, End: 3}}},
			Fields: []Field{{Table: tables[0], Key: keys[0], Values: values[0], Kind: kind.FieldName}, {Table: tables[1], Key: keys[1], Values: values[1], Kind: kind.FieldName}, {Table: tables[0], Key: keys[2], Values: values[2], Kind: kind.FieldName}},
			Order:  []keyspace.Term{fields[0], fields[2], fields[1]}},
	}
	return input
}
func TestFinalizerClaimsCopiedDraftAndCapturedViewExpiresAfterCommit(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copyDraft := *draft
	finalizer, err := copyDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("copied Draft acquired a second Finalizer")
	}
	captured := finalizer.View()
	if captured.Values().Count() == 0 {
		t.Fatal("claimed View did not expose authored rows")
	}
	committed, err := finalizer.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if captured.Values().Count() != 0 || captured.Cold().ContentID().Available() {
		t.Fatal("captured View remained live after Commit")
	}
	if committed.Values().Count() == 0 || !committed.Cold().ContentID().Available() {
		t.Fatal("committed View did not survive terminal transition")
	}
	if _, err := finalizer.Commit(); err == nil {
		t.Fatal("terminal Finalizer accepted a second Commit")
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("terminal Finalizer accepted Abort after Commit")
	}
}

func TestCapturedViewExpiresAfterAbort(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	captured := finalizer.View()
	if captured.Tables().Count() == 0 {
		t.Fatal("claimed View did not expose authored tables")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if captured.Tables().Count() != 0 || captured.Cold().ContentID().Available() {
		t.Fatal("captured View remained live after Abort")
	}
	if _, err := finalizer.Commit(); err == nil {
		t.Fatal("terminal Finalizer accepted Commit after Abort")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("Draft reopened after Abort")
	}
}

func TestCopiedFinalizersConcurrentCommitHaveOneConsumer(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	const callers = 32
	start := make(chan struct{})
	results := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		candidate := finalizer
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, commitErr := candidate.Commit()
			results <- commitErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for commitErr := range results {
		if commitErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful copied Finalizer commits = %d, want 1", successes)
	}
}

func TestConcurrentReadsAndTerminalCommitDoNotPanic(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	captured := finalizer.View()
	const readers = 32
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(readers + 1)
	for index := 0; index < readers; index++ {
		go func() {
			defer group.Done()
			<-start
			for repeat := 0; repeat < 128; repeat++ {
				captured.Values().Count()
				captured.Tables().At(0)
				captured.Operators().Unaries().Get(0)
			}
		}()
	}
	var committed View
	var commitErr error
	go func() {
		defer group.Done()
		<-start
		committed, commitErr = finalizer.Commit()
	}()
	close(start)
	group.Wait()
	if commitErr != nil {
		t.Fatalf("concurrent Commit: %v", commitErr)
	}
	if committed.Values().Count() == 0 {
		t.Fatal("committed View lost rows after concurrent reads")
	}
	if captured.Values().Count() != 0 {
		t.Fatal("captured View remained live after concurrent Commit")
	}
}
