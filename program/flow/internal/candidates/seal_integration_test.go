package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/control"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/position"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

type candidateFixture struct {
	sourceView source.View
	flowView   authored.View
	proof      *executable.Result

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize module.Finalizer
}

type candidateSpec struct {
	counts     [keyspace.FamilyCount]uint32
	rows       [][]keyspace.Term
	flow       authored.Input
	static     static.Input
	nilOwners  []keyspace.Term
	intOwners  []keyspace.Term
	keys       []source.KeyInput
	exactAtoms []keyspace.LiteralValue
}

func openCandidateFixture(t *testing.T, spec candidateSpec) *candidateFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("candidate fixture requires one Source row per Body")
	}

	sourceDraft, err := source.Build(candidateSourceInput(spec))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := spec.static
	staticInput.Counts = [keyspace.FamilyCount]uint32{}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		// Cross-owner TypeOf validation needs the Flow-owned endpoint
		// denominators (Cell, Read, and its expression closure) even though
		// those families are not Static-owned cardinalities.
		staticInput.Counts[family] = spec.counts[family]
	}
	staticInput.Counts[keyspace.FamilyBody] = spec.counts[keyspace.FamilyBody]
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	staticInput.Counts[keyspace.FamilyTypeAlias] = uint32(len(staticInput.Declarations.Alias))
	staticInput.Counts[keyspace.FamilyTypeOf] = uint32(len(staticInput.Operators.TypeOf))
	if len(staticInput.Contracts.Function) == 0 && spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if len(staticInput.Contracts.Call) == 0 && spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]static.CallContract, spec.counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
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

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, module.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := candidateTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, module.Finalizer{})
		t.Fatalf("module.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeCandidateFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(indexInput)
	if err != nil {
		closeCandidateFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()

	controlProof, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeCandidateFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	proof, err := executable.Seal(sourceView, flowView, forest, controlProof,
		staticView.ContentID(), moduleView.ContentID())
	if err != nil {
		closeCandidateFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	fixture := &candidateFixture{
		sourceView:     sourceView,
		flowView:       flowView,
		proof:          proof,
		staticFinalize: staticFinalize,
		flowFinalize:   flowFinalize,
		moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		closeCandidateFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func closeCandidateFinalizers(sourceFinalize source.Finalizer, staticFinalize static.Finalizer, flowFinalize authored.Finalizer, moduleFinalize module.Finalizer) {
	_ = moduleFinalize.Abort()
	_ = flowFinalize.Abort()
	_ = staticFinalize.Abort()
	_ = sourceFinalize.Abort()
}

func candidateSourceInput(spec candidateSpec) source.Input {
	input := source.Input{Name: "candidates-integration.lua", ExactAtoms: append([]keyspace.LiteralValue(nil), spec.exactAtoms...), Keys: append([]source.KeyInput(nil), spec.keys...)}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, spec.counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, rows := range spec.rows {
		input.Bodies[index] = source.BodySource{Body: candidateTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), rows...)}
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyNil]; ordinal++ {
		owner := candidateTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.nilOwners) {
			owner = spec.nilOwners[ordinal-1]
		}
		input.Nil = append(input.Nil, source.NilLiteral{Owner: owner})
	}
	for ordinal := uint32(1); ordinal <= spec.counts[keyspace.FamilyInteger]; ordinal++ {
		owner := candidateTerm(keyspace.FamilyBody, 1)
		if int(ordinal) <= len(spec.intOwners) {
			owner = spec.intOwners[ordinal-1]
		}
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: owner, Value: int64(ordinal)})
	}
	input.Binds = make([]source.BindCells, spec.counts[keyspace.FamilyBind])
	for ordinal := range input.Binds {
		input.Binds[ordinal] = source.BindCells{
			Bind:  candidateTerm(keyspace.FamilyBind, uint32(ordinal+1)),
			Cells: []keyspace.Term{candidateTerm(keyspace.FamilyCell, uint32(ordinal+1))},
		}
	}
	return input
}

func candidateTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	term := keyspace.MakeTerm(family, ordinal)
	if term == 0 {
		panic("candidate fixture Term overflow")
	}
	return term
}

func candidateIntegrationSpec() candidateSpec {
	const (
		body = keyspace.FamilyBody
	)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:      1,
		keyspace.FamilyNil:       57,
		keyspace.FamilyInteger:   1,
		keyspace.FamilyKey:       2,
		keyspace.FamilyValues:    5,
		keyspace.FamilyLensExact: 3,
		keyspace.FamilyLensKey:   2,
		keyspace.FamilyCell:      2,
		keyspace.FamilyRead:      4,
		keyspace.FamilyBind:      1,
		keyspace.FamilyAssign:    3,
		keyspace.FamilyWrite:     3,
		keyspace.FamilyUnary:     5,
		keyspace.FamilyBinary:    19,
		keyspace.FamilySelect:    2,
		keyspace.FamilyReturn:    1,
		keyspace.FamilyTypeOf:    1,
	}
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term { return candidateTerm(family, ordinal) }
	bodyTerm := term(body, 1)
	integerTerm := term(keyspace.FamilyInteger, 1)
	keyTerms := []keyspace.Term{term(keyspace.FamilyKey, 1), term(keyspace.FamilyKey, 2)}
	values := []keyspace.Term{term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2), term(keyspace.FamilyValues, 3), term(keyspace.FamilyValues, 4), term(keyspace.FamilyValues, 5)}
	exact := []keyspace.Term{term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyLensExact, 2), term(keyspace.FamilyLensExact, 3)}
	dynamic := []keyspace.Term{term(keyspace.FamilyLensKey, 1), term(keyspace.FamilyLensKey, 2)}
	localCell := term(keyspace.FamilyCell, 1)
	globalCell := term(keyspace.FamilyCell, 2)
	reads := []keyspace.Term{term(keyspace.FamilyRead, 1), term(keyspace.FamilyRead, 2), term(keyspace.FamilyRead, 3), term(keyspace.FamilyRead, 4)}
	assigns := []keyspace.Term{term(keyspace.FamilyAssign, 1), term(keyspace.FamilyAssign, 2), term(keyspace.FamilyAssign, 3)}
	bind := term(keyspace.FamilyBind, 1)
	unaries := []keyspace.Term{term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 2), term(keyspace.FamilyUnary, 3), term(keyspace.FamilyUnary, 4), term(keyspace.FamilyUnary, 5)}
	selects := []keyspace.Term{term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)}
	binaries := make([]keyspace.Term, 19)
	for ordinal := range binaries {
		binaries[ordinal] = term(keyspace.FamilyBinary, uint32(ordinal+1))
	}
	nilOrdinal := uint32(0)
	nextNil := func() keyspace.Term {
		nilOrdinal++
		return term(keyspace.FamilyNil, nilOrdinal)
	}
	valueMembers := []keyspace.Term{nextNil(), nextNil(), nextNil(), nextNil()}
	exactBases := []keyspace.Term{nextNil(), nextNil(), nextNil()}
	dynamicBases := []keyspace.Term{nextNil(), nextNil()}
	dynamicKeys := []keyspace.Term{nextNil(), nextNil()}
	for index := 1; index < len(unaries); index++ {
		// The static negative-key Unary consumes the sole Integer. Runtime
		// Unary rows each receive a private Nil operand.
		_ = nextNil()
	}
	binaryOperands := make([][2]keyspace.Term, len(binaries))
	for index := range binaryOperands {
		binaryOperands[index] = [2]keyspace.Term{nextNil(), nextNil()}
	}
	selectOperands := make([][2]keyspace.Term, len(selects))
	for index := range selectOperands {
		selectOperands[index] = [2]keyspace.Term{nextNil(), nextNil()}
	}
	if nilOrdinal != counts[keyspace.FamilyNil] {
		panic("candidate fixture literal allocation mismatch")
	}
	returnTerms := make([]keyspace.Term, 0, len(unaries)+len(binaries)+len(selects)+len(reads))
	returnTerms = append(returnTerms, unaries[1:4]...)
	returnTerms = append(returnTerms, binaries...)
	returnTerms = append(returnTerms, selects...)
	returnTerms = append(returnTerms, reads[0], reads[2], reads[3])
	valueTerms := []keyspace.Term{valueMembers[0], valueMembers[1], valueMembers[2], unaries[4], valueMembers[3]}
	valueTerms = append(valueTerms, returnTerms...)
	nilOwners := make([]keyspace.Term, counts[keyspace.FamilyNil])
	for index := range nilOwners {
		nilOwners[index] = bodyTerm
	}
	return candidateSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, assigns[0], assigns[1], term(keyspace.FamilyReturn, 1), assigns[2]}},
		exactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "field"},
			{Kind: keyspace.LiteralString, String: "write"},
		},
		keys:      []source.KeyInput{source.NameKey(bodyTerm, "field"), source.NameKey(bodyTerm, "write")},
		intOwners: []keyspace.Term{bodyTerm},
		nilOwners: nilOwners,
		static: static.Input{Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{
			Scope: localCell, Operand: reads[1],
		}}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: bodyTerm, Fixed: authored.Range{End: 1}},
					{Owner: bodyTerm, Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: bodyTerm, Fixed: authored.Range{Start: 2, End: 3}},
					{Owner: bodyTerm, Fixed: authored.Range{Start: 3, End: 5}},
					{Owner: bodyTerm, Fixed: authored.Range{Start: 5, End: uint32(len(valueTerms))}},
				},
				Terms: valueTerms,
			},
			Access: authored.AccessInput{
				Exact: []authored.ExactLens{
					{Owner: bodyTerm, Base: exactBases[0], Source: keyTerms[0], Kind: kind.FieldName},
					{Owner: bodyTerm, Base: exactBases[1], Source: unaries[0], Kind: kind.FieldExact},
					{Owner: bodyTerm, Base: exactBases[2], Source: keyTerms[1], Kind: kind.FieldName},
				},
				Dynamic: []authored.DynamicLens{
					{Owner: bodyTerm, Base: dynamicBases[0], Key: dynamicKeys[0]},
					{Owner: bodyTerm, Base: dynamicBases[1], Key: dynamicKeys[1]},
				},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: bodyTerm},
					{Kind: authored.CellGlobal, Key: 1},
				},
				Reads: []authored.Read{
					{Owner: bodyTerm, Source: exact[0]},
					{Owner: bodyTerm, Source: exact[1]},
					{Owner: bodyTerm, Source: dynamic[0]},
					{Owner: bodyTerm, Source: globalCell},
				},
				Binds: []authored.Bind{{Owner: bodyTerm, Values: values[0]}},
				Assigns: []authored.Assign{
					{Owner: bodyTerm, Values: values[1]},
					{Owner: bodyTerm, Values: values[2]},
					{Owner: bodyTerm, Values: values[3]},
				},
				Writes: []authored.Write{
					{Assign: assigns[0], Target: exact[2]},
					{Assign: assigns[1], Target: dynamic[1]},
					{Assign: assigns[2], Target: globalCell},
				},
			},
			Operators: authored.OperatorsInput{
				Unaries: []authored.Unary{
					{Owner: bodyTerm, Op: kind.UnaryNeg, Operand: integerTerm},
					{Owner: bodyTerm, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyNil, 12)},
					{Owner: bodyTerm, Op: kind.UnaryNot, Operand: term(keyspace.FamilyNil, 13)},
					{Owner: bodyTerm, Op: kind.UnaryLen, Operand: term(keyspace.FamilyNil, 14)},
					{Owner: bodyTerm, Op: kind.UnaryBitNot, Operand: term(keyspace.FamilyNil, 15)},
				},
				Binaries: candidateBinaryRows(bodyTerm, binaryOperands),
				Selects: []authored.Select{
					{Owner: bodyTerm, Op: kind.SelectAnd, Left: selectOperands[0][0], Right: selectOperands[0][1]},
					{Owner: bodyTerm, Op: kind.SelectOr, Left: selectOperands[1][0], Right: selectOperands[1][1]},
				},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: values[4]}}},
		},
	}
}

func candidateBinaryRows(owner keyspace.Term, operands [][2]keyspace.Term) []authored.Binary {
	rows := make([]authored.Binary, len(operands))
	for index := range rows {
		rows[index] = authored.Binary{Owner: owner, Op: kind.BinaryOp(index + 1), Left: operands[index][0], Right: operands[index][1]}
	}
	return rows
}

func TestCandidateSealHonestFixture(t *testing.T) {
	fixture := openCandidateFixture(t, candidateIntegrationSpec())
	result, err := Seal(fixture.sourceView.Identity(), fixture.flowView, fixture.proof,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	assertCandidateTerms(t, "UnaryNumeric", result.UnaryNumeric(), []keyspace.Term{
		candidateTerm(keyspace.FamilyUnary, 2),
	})
	assertCandidateTerms(t, "Length", result.Length(), []keyspace.Term{candidateTerm(keyspace.FamilyUnary, 4)})
	if result.UnaryNumeric().Contains(candidateTerm(keyspace.FamilyUnary, 1)) {
		t.Fatal("static negative exact-key Unary entered UnaryNumeric")
	}
	if result.UnaryNumeric().Contains(candidateTerm(keyspace.FamilyUnary, 3)) {
		t.Fatal("UnaryNot entered UnaryNumeric")
	}
	assertCandidateTerms(t, "Arithmetic", result.Arithmetic(), termsInFamily(keyspace.FamilyBinary, 1, 7))
	assertCandidateTerms(t, "Concat", result.Concat(), termsInFamily(keyspace.FamilyBinary, 8, 8))
	assertCandidateTerms(t, "Bitwise", result.Bitwise(), termsInFamily(keyspace.FamilyBinary, 9, 13))
	assertCandidateTerms(t, "Equality", result.Equality(), termsInFamily(keyspace.FamilyBinary, 14, 15))
	assertCandidateTerms(t, "Order", result.Order(), termsInFamily(keyspace.FamilyBinary, 16, 19))
	if result.IndexGet().Count() != 2 || !result.IndexGet().Contains(candidateTerm(keyspace.FamilyRead, 1)) || !result.IndexGet().Contains(candidateTerm(keyspace.FamilyRead, 3)) || result.IndexGet().Contains(candidateTerm(keyspace.FamilyRead, 2)) || result.IndexGet().Contains(candidateTerm(keyspace.FamilyRead, 4)) {
		t.Fatal("IndexGet did not distinguish exact/dynamic, static, and Cell reads")
	}
	if result.IndexSet().Count() != 2 || !result.IndexSet().Contains(candidateTerm(keyspace.FamilyWrite, 1)) || !result.IndexSet().Contains(candidateTerm(keyspace.FamilyWrite, 2)) || result.IndexSet().Contains(candidateTerm(keyspace.FamilyWrite, 3)) {
		t.Fatal("IndexSet did not distinguish exact/dynamic and Cell writes")
	}
	if fixture.proof.Executable(candidateTerm(keyspace.FamilyUnary, 1)) || fixture.proof.Executable(candidateTerm(keyspace.FamilyRead, 2)) {
		t.Fatal("honest executable proof retained static negative-key closure")
	}
	if fixture.proof.Executable(candidateTerm(keyspace.FamilyUnary, 5)) || result.UnaryNumeric().Contains(candidateTerm(keyspace.FamilyUnary, 5)) {
		t.Fatal("honest executable proof retained a post-Return dead Unary")
	}
	if fixture.proof.Executable(candidateTerm(keyspace.FamilySelect, 1)) == false || fixture.proof.Executable(candidateTerm(keyspace.FamilySelect, 2)) == false {
		t.Fatal("honest executable proof dropped authored Select rows")
	}
}

func TestCandidateSealPermutationResealAndCapacity(t *testing.T) {
	first := openCandidateFixture(t, candidateIntegrationSpec())
	second := openCandidateFixture(t, candidateIntegrationSpec())
	left, err := Seal(first.sourceView.Identity(), first.flowView, first.proof,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("first Seal: %v", err)
	}
	right, err := Seal(second.sourceView.Identity(), second.flowView, second.proof,
		second.staticFinalize.View().ContentID(), second.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("second Seal: %v", err)
	}
	foreignSpec := candidateIntegrationSpec()
	foreignSpec.flow.Operators.Binaries[0].Op = kind.BinarySub
	foreign := openCandidateFixture(t, foreignSpec)
	if _, err := Seal(first.sourceView.Identity(), foreign.flowView, first.proof,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("Seal accepted a proof from the first Flow with a foreign Flow identity")
	}
	permutedSpec := candidateIntegrationSpec()
	permutedSpec.keys[0], permutedSpec.keys[1] = permutedSpec.keys[1], permutedSpec.keys[0]
	permuted := openCandidateFixture(t, permutedSpec)
	permutedResult, err := Seal(permuted.sourceView.Identity(), permuted.flowView, permuted.proof,
		permuted.staticFinalize.View().ContentID(), permuted.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("permuted reseal: %v", err)
	}
	for _, check := range []struct {
		name  string
		left  func() (int, func(int) (keyspace.Term, bool))
		right func() (int, func(int) (keyspace.Term, bool))
	}{
		{"UnaryNumeric", func() (int, func(int) (keyspace.Term, bool)) { v := left.UnaryNumeric(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) {
			v := permutedResult.UnaryNumeric()
			return v.Count(), v.At
		}},
		{"Length", func() (int, func(int) (keyspace.Term, bool)) { v := left.Length(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.Length(); return v.Count(), v.At }},
		{"Arithmetic", func() (int, func(int) (keyspace.Term, bool)) { v := left.Arithmetic(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) {
			v := permutedResult.Arithmetic()
			return v.Count(), v.At
		}},
		{"Bitwise", func() (int, func(int) (keyspace.Term, bool)) { v := left.Bitwise(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.Bitwise(); return v.Count(), v.At }},
		{"Concat", func() (int, func(int) (keyspace.Term, bool)) { v := left.Concat(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.Concat(); return v.Count(), v.At }},
		{"Equality", func() (int, func(int) (keyspace.Term, bool)) { v := left.Equality(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.Equality(); return v.Count(), v.At }},
		{"Order", func() (int, func(int) (keyspace.Term, bool)) { v := left.Order(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.Order(); return v.Count(), v.At }},
		{"IndexGet", func() (int, func(int) (keyspace.Term, bool)) { v := left.IndexGet(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.IndexGet(); return v.Count(), v.At }},
		{"IndexSet", func() (int, func(int) (keyspace.Term, bool)) { v := left.IndexSet(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := permutedResult.IndexSet(); return v.Count(), v.At }},
	} {
		leftCount, leftAt := check.left()
		rightCount, rightAt := check.right()
		if leftCount != rightCount {
			t.Fatalf("permuted %s count = %d/%d", check.name, leftCount, rightCount)
		}
		for index := 0; index < leftCount; index++ {
			leftTerm, leftOK := leftAt(index)
			rightTerm, rightOK := rightAt(index)
			if !leftOK || !rightOK || leftTerm != rightTerm {
				t.Fatalf("permuted %s At(%d) = %08x/%v and %08x/%v", check.name, index, uint32(leftTerm), leftOK, uint32(rightTerm), rightOK)
			}
		}
	}
	for _, check := range []struct {
		name  string
		left  func() (int, func(int) (keyspace.Term, bool))
		right func() (int, func(int) (keyspace.Term, bool))
	}{
		{"UnaryNumeric", func() (int, func(int) (keyspace.Term, bool)) { v := left.UnaryNumeric(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.UnaryNumeric(); return v.Count(), v.At }},
		{"Length", func() (int, func(int) (keyspace.Term, bool)) { v := left.Length(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Length(); return v.Count(), v.At }},
		{"Arithmetic", func() (int, func(int) (keyspace.Term, bool)) { v := left.Arithmetic(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Arithmetic(); return v.Count(), v.At }},
		{"Bitwise", func() (int, func(int) (keyspace.Term, bool)) { v := left.Bitwise(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Bitwise(); return v.Count(), v.At }},
		{"Concat", func() (int, func(int) (keyspace.Term, bool)) { v := left.Concat(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Concat(); return v.Count(), v.At }},
		{"Equality", func() (int, func(int) (keyspace.Term, bool)) { v := left.Equality(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Equality(); return v.Count(), v.At }},
		{"Order", func() (int, func(int) (keyspace.Term, bool)) { v := left.Order(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.Order(); return v.Count(), v.At }},
		{"IndexGet", func() (int, func(int) (keyspace.Term, bool)) { v := left.IndexGet(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.IndexGet(); return v.Count(), v.At }},
		{"IndexSet", func() (int, func(int) (keyspace.Term, bool)) { v := left.IndexSet(); return v.Count(), v.At }, func() (int, func(int) (keyspace.Term, bool)) { v := right.IndexSet(); return v.Count(), v.At }},
	} {
		t.Run(check.name, func(t *testing.T) {
			leftCount, leftAt := check.left()
			rightCount, rightAt := check.right()
			if leftCount != rightCount {
				t.Fatalf("reseal count = %d/%d", leftCount, rightCount)
			}
			for index := 0; index < leftCount; index++ {
				leftTerm, leftOK := leftAt(index)
				rightTerm, rightOK := rightAt(index)
				if !leftOK || !rightOK || leftTerm != rightTerm {
					t.Fatalf("reseal At(%d) = %08x/%v and %08x/%v", index, uint32(leftTerm), leftOK, uint32(rightTerm), rightOK)
				}
			}
		})
	}
	for _, view := range []struct {
		name  string
		count int
		cap   int
	}{
		{"UnaryNumeric", left.UnaryNumeric().Count(), cap(left.buckets.unaryNumeric)},
		{"Length", left.Length().Count(), cap(left.buckets.length)},
		{"Arithmetic", left.Arithmetic().Count(), cap(left.buckets.arithmetic)},
		{"Bitwise", left.Bitwise().Count(), cap(left.buckets.bitwise)},
		{"Concat", left.Concat().Count(), cap(left.buckets.concat)},
		{"Equality", left.Equality().Count(), cap(left.buckets.equality)},
		{"Order", left.Order().Count(), cap(left.buckets.order)},
		{"IndexGet", left.IndexGet().Count(), cap(left.buckets.indexGet)},
		{"IndexSet", left.IndexSet().Count(), cap(left.buckets.indexSet)},
	} {
		if view.cap > view.count*2+1 {
			t.Fatalf("%s retained cap %d for %d members", view.name, view.cap, view.count)
		}
	}
}

func assertCandidateTerms[T interface {
	Count() int
	At(int) (keyspace.Term, bool)
}](t *testing.T, name string, view T, want []keyspace.Term) {
	t.Helper()
	if view.Count() != len(want) {
		t.Fatalf("%s Count() = %d, want %d", name, view.Count(), len(want))
	}
	for index, wantTerm := range want {
		got, ok := view.At(index)
		if !ok || got != wantTerm {
			t.Fatalf("%s At(%d) = %08x/%v, want %08x/true", name, index, uint32(got), ok, uint32(wantTerm))
		}
	}
}

func termsInFamily(family keyspace.Family, first, last uint32) []keyspace.Term {
	terms := make([]keyspace.Term, 0, last-first+1)
	for ordinal := first; ordinal <= last; ordinal++ {
		terms = append(terms, candidateTerm(family, ordinal))
	}
	return terms
}
