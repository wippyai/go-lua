package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// proofFixture is deliberately assembled through the live owner capabilities.
// It does not construct Result values or bypass any owner finalizer. Keeping
// the capabilities open is also important: LocalContainment and Preimage are
// lifecycle-bound inputs to Prove.
type proofFixture struct {
	entry keyspace.Term

	preimage       source.Preimage
	sourceFinalize source.Finalizer

	flowView     authored.View
	flowFinalize authored.Finalizer

	staticView     static.View
	staticFinalize static.Finalizer

	moduleView     imports.View
	moduleFinalize imports.Finalizer

	bodies   *body.Result
	binding  binding.Result
	finalize func()
}

type proofSpec struct {
	counts    [keyspace.FamilyCount]uint32
	rows      [][]keyspace.Term
	flow      authored.Input
	static    static.Input
	module    imports.Input
	exacts    []keyspace.LiteralValue
	keys      []source.KeyInput
	binds     []source.BindCells
	nilOwners []keyspace.Term
	formals   []source.FunctionFormals
	entry     keyspace.Term
}

func newProofFixture(t *testing.T, spec proofSpec) *proofFixture {
	t.Helper()
	counts := spec.counts
	if counts[keyspace.FamilyBody] == 0 {
		t.Fatal("fixture requires an Entry Body")
	}
	entry := spec.entry
	if entry == 0 {
		entry = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	}

	flowInput := spec.flow
	flowInput.Counts = counts
	staticInput := spec.static
	staticInput.Counts = counts

	sourceInput := makeSourceInput(counts, spec.rows, spec.exacts, spec.keys, spec.binds, spec.nilOwners)
	for _, row := range spec.formals {
		if keyspace.TermFamily(row.Function) != keyspace.FamilyFunction || keyspace.TermOrdinal(row.Function) == 0 ||
			uint64(keyspace.TermOrdinal(row.Function)) > uint64(len(sourceInput.Functions)) {
			t.Fatalf("invalid proof Function formal owner %v", row.Function)
		}
		sourceInput.Functions[keyspace.TermOrdinal(row.Function)-1] = row
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(spec.module)
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		_ = flowFinalize.Abort()
		_ = staticFinalize.Abort()
		_ = sourceFinalize.Abort()
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	fixture := &proofFixture{
		entry:          entry,
		preimage:       preimage,
		sourceFinalize: sourceFinalize,
		flowView:       flowView,
		flowFinalize:   flowFinalize,
		staticView:     staticView,
		staticFinalize: staticFinalize,
		moduleView:     moduleView,
		moduleFinalize: moduleFinalize,
		bodies:         bodies,
		binding:        bindings,
	}
	fixture.finalize = func() {
		_ = fixture.moduleFinalize.Abort()
		_ = fixture.flowFinalize.Abort()
		_ = fixture.staticFinalize.Abort()
		_ = fixture.sourceFinalize.Abort()
	}
	t.Cleanup(fixture.finalize)
	return fixture
}

func (fixture *proofFixture) prove() (*Result, error) {
	result, _, err := fixture.proveWithScope()
	return result, err
}

func (fixture *proofFixture) proveWithScope() (*Result, *StaticScopeProof, error) {
	return Prove(
		fixture.preimage,
		fixture.staticView,
		fixture.flowView,
		fixture.bodies,
		fixture.binding,
		fixture.moduleView,
		fixture.entry,
	)
}

func makeSourceInput(
	counts [keyspace.FamilyCount]uint32,
	rows [][]keyspace.Term,
	exacts []keyspace.LiteralValue,
	keys []source.KeyInput,
	binds []source.BindCells,
	nilOwners []keyspace.Term,
) source.Input {
	input := source.Input{Name: "containment-law.lua", ExactAtoms: append([]keyspace.LiteralValue(nil), exacts...)}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{
				File: input.Name, StartLine: line, StartCol: 1,
				EndLine: line, EndCol: 1,
			}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, counts[keyspace.FamilyBody])
	for ordinal := range input.Bodies {
		var terms []keyspace.Term
		if ordinal < len(rows) {
			terms = append(terms, rows[ordinal]...)
		}
		input.Bodies[ordinal] = source.BodySource{
			Body:  keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal+1)),
			Terms: terms,
		}
	}
	input.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for ordinal := range input.Binds {
		if ordinal < len(binds) {
			input.Binds[ordinal] = binds[ordinal]
		}
		input.Binds[ordinal].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal+1))
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for ordinal := range input.Functions {
		input.Functions[ordinal].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal+1))
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyNil]; ordinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(nilOwners) {
			owner = nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBool]; ordinal++ {
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: ordinal%2 == 1})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyInteger]; ordinal++ {
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: int64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFloat]; ordinal++ {
		input.Float = append(input.Float, source.FloatLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bits: uint64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyString]; ordinal++ {
		input.String = append(input.String, source.StringLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: "literal"})
	}
	input.Keys = append([]source.KeyInput(nil), keys...)
	return input
}

func countsFor(families ...struct {
	family keyspace.Family
	count  uint32
}) (counts [keyspace.FamilyCount]uint32) {
	for _, item := range families {
		counts[item.family] = item.count
	}
	return counts
}

func c(family keyspace.Family, count uint32) struct {
	family keyspace.Family
	count  uint32
} {
	return struct {
		family keyspace.Family
		count  uint32
	}{family: family, count: count}
}

func emptyModule(t *testing.T) imports.Input {
	t.Helper()
	return imports.Input{}
}

func TestProveMinimalEntryRoot(t *testing.T) {
	counts := countsFor(c(keyspace.FamilyBody, 1))
	fixture := newProofFixture(t, proofSpec{counts: counts, static: static.Input{}, module: emptyModule(t)})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if result.Count() != 1 {
		t.Fatalf("Count = %d, want one Entry Body", result.Count())
	}
	if got, ok := result.At(0); !ok || got != fixture.entry {
		t.Fatalf("At(0) = %v/%v, want Entry %v", got, ok, fixture.entry)
	}
	if parent, ok := result.Parent(fixture.entry); ok || parent != 0 {
		t.Fatalf("Entry parent = %v/%v, want zero/false", parent, ok)
	}
	if !result.Contains(fixture.entry, fixture.entry) {
		t.Fatal("Entry does not contain itself")
	}
	if result.Static(fixture.entry) {
		t.Fatal("Entry was marked as a static expression")
	}
}

func TestProveGlobalCellRootAndChunkCellReachesEntry(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyCell, 2),
		c(keyspace.FamilyVararg, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	global := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	chunk := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	flow := authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: vararg}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: 1},
				{Kind: authored.CellLocal, Body: body},
			},
			Varargs: []authored.Vararg{{Owner: body, Cell: chunk}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		flow:   flow,
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(global); ok || parent != 0 {
		t.Fatalf("global Cell parent = %v/%v, want root", parent, ok)
	}
	if parent, ok := result.Parent(chunk); !ok || parent != body {
		t.Fatalf("chunk Cell parent = %v/%v, want Entry %v", parent, ok, body)
	}
	if !result.Contains(body, chunk) || result.Contains(global, chunk) {
		t.Fatal("Cell containment intervals are not root/lexical exact")
	}
}

func TestProveUsesLexicalBodyParentNotConstructHost(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 2),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{function},
		},
		Functions: authored.FunctionsInput{
			Rows: []authored.Function{{Owner: body, Body: child}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
	staticInput := static.Input{
		Contracts: static.ContractsInput{Function: []static.FunctionContract{{}}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}, nil},
		flow:   flow,
		static: staticInput,
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(child); !ok || parent != body {
		t.Fatalf("child Body parent = %v/%v, want lexical Body %v", parent, ok, body)
	}
	if parent, ok := result.Parent(function); !ok || parent != keyspace.MakeTerm(keyspace.FamilyValues, 1) {
		t.Fatalf("Function parent = %v/%v, want Values", parent, ok)
	}
	if result.Contains(function, child) {
		t.Fatal("Function construct became a parent of its executable Body")
	}
}

func TestProveDirectSourceStatementBelongsToBody(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyNil, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyReturn, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(returned); !ok || parent != body {
		t.Fatalf("Return parent = %v/%v, want Body %v", parent, ok, body)
	}
	if parent, ok := result.Parent(values); !ok || parent != returned {
		t.Fatalf("Values parent = %v/%v, want Return %v", parent, ok, returned)
	}
}

func TestProveRepeatedTypeOfUsesSameBodyFallback(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyRead, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyTypeOf, 2),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	flow := authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Reads: []authored.Read{{Owner: body, Source: cell}},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
	}
	staticInput := static.Input{Operators: static.OperatorsInput{TypeOf: []static.TypeOf{
		{Scope: cell, Operand: read},
		{Scope: cell, Operand: read},
	}}}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind}},
		flow:   flow,
		static: staticInput,
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(read); !ok || parent != body {
		t.Fatalf("Read parent = %v/%v, want Body %v", parent, ok, body)
	}
	for ordinal := uint32(1); ordinal <= 2; ordinal++ {
		typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, ordinal)
		if parent, ok := result.Parent(typeOf); !ok || parent != body {
			t.Fatalf("TypeOf%d parent = %v/%v, want Body %v", ordinal, parent, ok, body)
		}
	}
}

func TestProveRejectsTypeOfScopeFromDifferentBody(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 2),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyRead, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyTypeOf, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind}, nil},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: child}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Reads: []authored.Read{{Owner: child, Source: cell}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
		static: static.Input{
			Contracts: static.ContractsInput{Function: []static.FunctionContract{{}}},
			Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{Scope: cell, Operand: read}}},
		},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		module: emptyModule(t),
	})
	if _, err := fixture.prove(); err == nil {
		t.Fatal("Prove accepted TypeOf whose operand belongs to a different Body")
	}
}

func TestProveTypeOfLocalParentSuppressesScopeFallback(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyNil, 1),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyRead, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyValueClaim, 1),
		c(keyspace.FamilyTypeOf, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{claim},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Reads: []authored.Read{{Owner: body, Source: cell}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			Claims: []authored.ValueClaim{{Owner: body, Operand: keyspace.MakeTerm(keyspace.FamilyNil, 1), Kind: kind.ValueClaimTypeAs}},
		},
		static: static.Input{
			Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{Scope: cell, Operand: read}}},
			Operands:  static.OperandsInput{Claim: []static.ClaimTarget{{Claim: claim, Target: typeOf}}},
		},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(typeOf); !ok || parent != claim {
		t.Fatalf("TypeOf parent = %v/%v, want local Static parent %v", parent, ok, claim)
	}
	if parent, ok := result.Parent(read); !ok || parent != body {
		t.Fatalf("unconsumed TypeOf operand parent = %v/%v, want Body fallback %v", parent, ok, body)
	}
	if result.Contains(typeOf, read) {
		t.Fatal("TypeOf operand reference became containment")
	}
	if !result.Static(typeOf) || !result.Static(read) {
		t.Fatal("TypeOf node and operand closure were not classified static")
	}
}

func TestProveSharedAnnotationValuesFallbackToOneBody(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyValues, 2),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyTypePrimitive, 1),
		c(keyspace.FamilyDeclaredType, 1),
		c(keyspace.FamilyAnnotation, 2),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	initializer := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	declared := keyspace.MakeTerm(keyspace.FamilyDeclaredType, 1)
	annotation1 := keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)
	annotation2 := keyspace.MakeTerm(keyspace.FamilyAnnotation, 2)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: initializer}},
			},
		},
		static: static.Input{
			Types: static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
			Declarations: static.DeclarationsInput{DeclaredType: []static.DeclaredType{{
				Cell: cell, Target: primitive,
			}}},
			Operands: static.OperandsInput{Annotation: []static.Annotation{
				{Scope: cell, Target: primitive, Name: 1, Values: values},
				{Scope: cell, Target: primitive, Name: 1, Values: values},
			}},
		},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "annotation"}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(values); !ok || parent != body {
		t.Fatalf("Annotation Values parent = %v/%v, want Body %v", parent, ok, body)
	}
	if parent, ok := result.Parent(declared); !ok || parent != cell {
		t.Fatalf("DeclaredType parent = %v/%v, want Cell %v", parent, ok, cell)
	}
	if parent, ok := result.Parent(primitive); !ok || parent != declared {
		t.Fatalf("Annotation target parent = %v/%v, want DeclaredType %v", parent, ok, declared)
	}
	for _, annotation := range []keyspace.Term{annotation1, annotation2} {
		if parent, ok := result.Parent(annotation); !ok || parent != primitive {
			t.Fatalf("Annotation %v parent = %v/%v, want target %v", annotation, parent, ok, primitive)
		}
	}
	if result.Contains(annotation1, values) || result.Contains(annotation2, values) {
		t.Fatal("Annotation Values reference became containment")
	}
	if !result.Static(annotation1) || !result.Static(annotation2) || !result.Static(values) {
		t.Fatal("Annotation nodes and Values closure were not classified static")
	}
	if result.Static(primitive) {
		t.Fatal("Annotation target reference leaked into static expression closure")
	}
}

func TestProveRejectsStandaloneUnconsumedTerm(t *testing.T) {
	counts := countsFor(c(keyspace.FamilyBody, 1), c(keyspace.FamilyTypePrimitive, 1))
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		static: static.Input{Types: static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}}},
		module: emptyModule(t),
	})
	if _, err := fixture.prove(); err == nil {
		t.Fatal("Prove accepted a static term with no structural consumer or fallback")
	}
}

func TestProveRejectsWrongModuleImportCallForeignKey(t *testing.T) {
	input := imports.Input{Imports: []imports.Import{{
		Term: keyspace.MakeTerm(keyspace.FamilyImport, 1),
		Call: keyspace.MakeTerm(keyspace.FamilyFunction, 1),
	}}}
	if _, err := imports.Build(input); err == nil {
		t.Fatal("imports.Build accepted an Import whose Call foreign key is not a Call")
	}
}

func TestProveRejectsExpiredOwners(t *testing.T) {
	tests := []struct {
		name   string
		expire func(*proofFixture)
	}{
		{name: "Source", expire: func(fixture *proofFixture) { _ = fixture.sourceFinalize.Abort() }},
		{name: "Authored", expire: func(fixture *proofFixture) { _, _ = fixture.flowFinalize.Commit() }},
		{name: "Static", expire: func(fixture *proofFixture) { _, _ = fixture.staticFinalize.Commit(static.CommitInput{}) }},
		{name: "Module", expire: func(fixture *proofFixture) { _ = fixture.moduleFinalize.Abort() }},
		{name: "zero Static", expire: func(fixture *proofFixture) {
			fixture.staticView = static.View{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counts := countsFor(c(keyspace.FamilyBody, 1))
			fixture := newProofFixture(t, proofSpec{counts: counts, module: emptyModule(t)})
			test.expire(fixture)
			result, err := fixture.prove()
			if err == nil {
				t.Fatalf("Prove accepted expired %s owner", test.name)
			}
			if result != nil {
				t.Fatalf("Prove returned a proof with expired %s owner", test.name)
			}
		})
	}
}

func TestProveResultQueriesAreDenseDeterministicAndAllocationFree(t *testing.T) {
	counts := countsFor(c(keyspace.FamilyBody, 1))
	fixture := newProofFixture(t, proofSpec{counts: counts, module: emptyModule(t)})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if result.Count() != 1 {
		t.Fatalf("Count = %d, want 1", result.Count())
	}
	if _, ok := result.At(-1); ok {
		t.Fatal("At(-1) returned a term")
	}
	if _, ok := result.At(result.Count()); ok {
		t.Fatal("At(Count) returned a term")
	}
	if _, ok := result.Parent(keyspace.MakeTerm(keyspace.FamilyCell, 1)); ok {
		t.Fatal("Parent accepted a foreign term")
	}
	if result.Contains(keyspace.MakeTerm(keyspace.FamilyCell, 1), fixture.entry) {
		t.Fatal("Contains accepted a foreign outer term")
	}
	if testing.AllocsPerRun(100, func() {
		result.Count()
		result.At(0)
		result.Parent(fixture.entry)
		result.Contains(fixture.entry, fixture.entry)
		result.Static(fixture.entry)
	}) != 0 {
		t.Fatal("Result queries allocate")
	}
}
