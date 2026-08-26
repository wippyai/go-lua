package compiler_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/domain/composite"
)

func summaryProgram(t testing.TB, artifact *programartifact.Artifact) programschema.Program {
	t.Helper()
	program := artifact.Program()
	if !program.Available() {
		t.Fatal("summary cold program unavailable")
	}
	return program
}

func TestProgramArtifactExactScalarSummaryCrossesLocalStorageOnce(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-exact-scalar-summary.lua", Text: []byte(`
local function total(): integer
    local n = 10
    local m = n + 5
    return m
end
return total
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile scalar summary fixture: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	if !exactPublished || exactCount != 3 {
		t.Fatalf("exact scalar summary count = %d/%v, want 3/true", exactCount, exactPublished)
	}
	var occurrenceID identity.ContentID
	values := make(map[programschema.ExactScalarSummaryRole]int64, 3)
	for index := 0; index < exactCount; index++ {
		summary, summaryOK := program.ExactScalarSummaryAt(index)
		literal, literalOK := summary.Literal()
		if !summaryOK || !literalOK || literal.Kind != uint8(keyspace.LiteralInteger) || !summary.SubjectID().Available() || !summary.Role().Valid() {
			t.Fatalf("scalar summary[%d]=%+v/%v literal=%+v/%v", index, summary, summaryOK, literal, literalOK)
		}
		if !occurrenceID.Available() {
			occurrenceID = summary.OccurrenceID()
		} else if summary.OccurrenceID() != occurrenceID {
			t.Fatal("one arithmetic expression issued summaries for different occurrences")
		}
		if _, duplicate := values[summary.Role()]; duplicate {
			t.Fatalf("duplicate scalar summary role %d", summary.Role())
		}
		values[summary.Role()] = literal.Integer
	}
	if values[programschema.ExactScalarSummaryLeft] != 10 || values[programschema.ExactScalarSummaryRight] != 5 || values[programschema.ExactScalarSummaryResult] != 15 {
		t.Fatalf("exact scalar use summary=%v, want left=10 right=5 result=15", values)
	}
	arithmetic, arithmeticOK := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, occurrenceID)
	body, bodyOK := arithmetic.BodyID()
	arithmeticOrdinal, arithmeticOrdinalOK := program.OccurrenceOrdinalForID(programschema.OccurrenceBinaryArithmetic, occurrenceID)
	left, leftOK := program.OccurrenceInputID(arithmeticOrdinal, 0)
	right, rightOK := program.OccurrenceInputID(arithmeticOrdinal, 1)
	op := flowkind.BinaryOp(arithmetic.Code())
	endpointsOK := arithmeticOrdinalOK && leftOK && rightOK
	rule, ruleOK := ruleForOccurrence(artifact, "value-binary-arithmetic", occurrenceID)
	point := rule.PointID()
	input, inputOK := rule.InputPointAt(0)
	if !arithmeticOK || !bodyOK || !endpointsOK || !left.Available() || !right.Available() || op != flowkind.BinaryAdd ||
		!ruleOK || !point.Available() || !inputOK || point == input || rule.Stage() != programissuance.StageComputation {
		t.Fatalf("scalar arithmetic=%+v/%v rule=%+v/%v", arithmetic, arithmeticOK, rule, ruleOK)
	}
	for index := 0; index < exactCount; index++ {
		summary, _ := program.ExactScalarSummaryAt(index)
		if summary.BodyPathID() != body {
			t.Fatalf("scalar summary[%d] body=%s, want %s", index, summary.BodyPathID(), body)
		}
	}
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("arithmetic summary count=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	numeric, numericOK := program.ArithmeticSummaryAt(0)
	numericLeft, numericRight, numericResult, representationsOK := numeric.Representations()
	if !numericOK || !representationsOK || numeric.OccurrenceID() != occurrenceID || numeric.BodyPathID() != body ||
		numeric.Operator() != programschema.SummaryOperator(flowkind.BinaryAdd) || numericLeft != programschema.NumericRepresentationInteger ||
		numericRight != programschema.NumericRepresentationInteger || numericResult != programschema.NumericRepresentationInteger {
		t.Fatalf("arithmetic summary=%+v/%v representations=%d/%d/%d/%v", numeric, numericOK, numericLeft, numericRight, numericResult, representationsOK)
	}
	if _, ok := program.ExactScalarSummaryAt(-1); ok {
		t.Fatal("negative scalar summary index accepted")
	}
}

func TestProgramArtifactExactScalarSummaryEnumeratesMergedFiniteMutation(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-exact-scalar-merge.lua", Text: []byte(`
local function pick(flag: boolean): number
    local v: number = 0
    if flag then
        v = 1
    else
        v = 0.5
    end
    return v + v
end
return pick
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile scalar merge fixture: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	if !exactPublished {
		t.Fatal("exact scalar summary column unpublished")
	}
	if exactCount != 12 {
		t.Fatalf("merged mutation exact scalar summary count=%d/%v, want 12/true", exactCount, exactPublished)
	}
	byRole := make(map[programschema.ExactScalarSummaryRole]map[keyspace.LiteralValue]struct{}, 3)
	for index := 0; index < exactCount; index++ {
		row, rowOK := program.ExactScalarSummaryAt(index)
		coldLiteral, literalOK := row.Literal()
		if !rowOK || !literalOK {
			t.Fatalf("merged mutation summary[%d] unavailable: %+v/%v literal=%+v/%v", index, row, rowOK, coldLiteral, literalOK)
		}
		literal := keyspace.LiteralValue{Kind: keyspace.LiteralKind(coldLiteral.Kind), Integer: coldLiteral.Integer, FloatBits: coldLiteral.FloatBits}
		values := byRole[row.Role()]
		if values == nil {
			values = make(map[keyspace.LiteralValue]struct{})
			byRole[row.Role()] = values
		}
		values[literal] = struct{}{}
	}
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	wantLeft := map[keyspace.LiteralValue]struct{}{integer(0): {}, float(0.5): {}, integer(1): {}}
	wantResult := map[keyspace.LiteralValue]struct{}{integer(0): {}, float(0.5): {}, integer(1): {}, float(1): {}, float(1.5): {}, integer(2): {}}
	for _, want := range []struct {
		role   programschema.ExactScalarSummaryRole
		values map[keyspace.LiteralValue]struct{}
	}{
		{role: programschema.ExactScalarSummaryLeft, values: wantLeft},
		{role: programschema.ExactScalarSummaryRight, values: wantLeft},
		{role: programschema.ExactScalarSummaryResult, values: wantResult},
	} {
		if len(byRole[want.role]) != len(want.values) {
			t.Fatalf("merged mutation role %d values=%v, want %v", want.role, byRole[want.role], want.values)
		}
		for literal := range want.values {
			if _, found := byRole[want.role][literal]; !found {
				t.Fatalf("merged mutation role %d missing literal %+v: %v", want.role, literal, byRole[want.role])
			}
		}
	}
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("merged mutation arithmetic summary count=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	summary, summaryOK := program.ArithmeticSummaryAt(0)
	left, right, result, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || summary.Operator() != programschema.SummaryOperator(flowkind.BinaryAdd) ||
		left != programschema.NumericRepresentationNumber || right != programschema.NumericRepresentationNumber || result != programschema.NumericRepresentationNumber {
		t.Fatalf("merged mutation arithmetic=%+v/%v representations=%d/%d/%d/%v", summary, summaryOK, left, right, result, representationsOK)
	}
}

func TestProgramArtifactArithmeticSummaryAuthenticatesGuardedDivisor(t *testing.T) {
	compile := func(name, text string) *programartifact.Artifact {
		t.Helper()
		published, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
		if err != nil {
			t.Fatal(err)
		}
		compilation, compilationOK := composite.Build()
		if !compilationOK {
			t.Fatal("Program artifact grammar unavailable")
		}
		artifact, failure := compileArtifactForTest(t, published, compilation)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile guarded divisor fixture: %s", failure.Error())
		}
		return artifact
	}
	for _, test := range []struct {
		name, guard string
		want        programschema.ArithmeticDivisorProperty
	}{
		{name: "both", guard: "b ~= 0 and b ~= -1", want: programschema.ArithmeticDivisorNonzeroNotMinusOne},
		{name: "zero", guard: "b ~= 0", want: programschema.ArithmeticDivisorNonzero},
		{name: "disjunction", guard: "b ~= 0 or b ~= -1", want: programschema.ArithmeticDivisorNone},
	} {
		artifact := compile("artifact-divisor-"+test.name+".lua", `
local function idiv(a: integer, b: integer): integer
    if `+test.guard+` then
        return a // b
    end
    return 0
end
return idiv
`)
		program := summaryProgram(t, artifact)
		arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
		if !arithmeticPublished || arithmeticCount != 1 {
			t.Fatalf("%s arithmetic summary count=%d/%v, want 1/true", test.name, arithmeticCount, arithmeticPublished)
		}
		summary, summaryOK := program.ArithmeticSummaryAt(0)
		left, right, result, representationsOK := summary.Representations()
		if !summaryOK || !representationsOK || summary.Operator() != programschema.SummaryOperator(flowkind.BinaryIDiv) ||
			left != programschema.NumericRepresentationInteger || right != programschema.NumericRepresentationInteger || result != programschema.NumericRepresentationInteger ||
			summary.DivisorProperty() != test.want {
			t.Fatalf("%s summary=%+v/%v representations=%d/%d/%d/%v divisor=%d, want %d", test.name, summary, summaryOK,
				left, right, result, representationsOK, summary.DivisorProperty(), test.want)
		}
	}
	unguarded := compile("artifact-divisor-unguarded.lua", `
local function idiv(a: integer, b: integer): integer
    return a // b
end
return idiv
`)
	program := summaryProgram(t, unguarded)
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("unguarded arithmetic summary count=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	summary, summaryOK := program.ArithmeticSummaryAt(0)
	if !summaryOK || summary.DivisorProperty() != programschema.ArithmeticDivisorNone {
		t.Fatalf("unguarded summary=%+v/%v divisor=%d, want none", summary, summaryOK, summary.DivisorProperty())
	}
}

func TestProgramArtifactUnarySummaryNamesExactOutputPoint(t *testing.T) {
	compile := func(name, text string) *programartifact.Artifact {
		t.Helper()
		published, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
		if err != nil {
			t.Fatal(err)
		}
		compilation, compilationOK := composite.Build()
		if !compilationOK {
			t.Fatal("Program artifact grammar unavailable")
		}
		artifact, failure := compileArtifactForTest(t, published, compilation)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile unary summary fixture: %s", failure.Error())
		}
		return artifact
	}
	artifact := compile("artifact-unary-summary.lua", `
local function invert(n: integer): integer
    return -n
end
return invert
`)
	program := summaryProgram(t, artifact)
	unaryCount, unaryPublished := program.UnarySummaryCount()
	if !unaryPublished || unaryCount != 1 {
		t.Fatalf("unary summary count=%d/%v, want 1/true", unaryCount, unaryPublished)
	}
	summary, summaryOK := program.UnarySummaryAt(0)
	operand, result, representationsOK := summary.Representations()
	occurrence, occurrenceOK := program.OccurrenceForID(programschema.OccurrenceUnary, summary.OccurrenceID())
	occurrenceOrdinal, occurrenceOrdinalOK := program.OccurrenceOrdinalForID(programschema.OccurrenceUnary, summary.OccurrenceID())
	outputOwned := false
	_, pointCount, pointSpanOK := occurrence.PointSpan()
	for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
		point, pointOK := program.OccurrencePointID(occurrenceOrdinal, int(pointIndex))
		outputOwned = outputOwned || pointSpanOK && pointOK && point == summary.OutputPointID()
	}
	if !summaryOK || !representationsOK || !occurrenceOK || !occurrenceOrdinalOK || !outputOwned ||
		summary.Operator() != programschema.SummaryOperator(flowkind.UnaryNeg) || operand != programschema.NumericRepresentationInteger || result != programschema.NumericRepresentationInteger {
		t.Fatalf("unary summary=%+v/%v occurrence=%+v/%v output-owned=%v representations=%d/%d/%v", summary, summaryOK,
			occurrence, occurrenceOK, outputOwned, operand, result, representationsOK)
	}
	untyped := compile("artifact-unary-summary-untyped.lua", `
local function invert(n)
    return -n
end
return invert
`)
	untypedProgram := summaryProgram(t, untyped)
	untypedCount, untypedPublished := untypedProgram.UnarySummaryCount()
	if !untypedPublished || untypedCount != 0 {
		t.Fatalf("untyped unary summaries=%d/%v, want 0/true", untypedCount, untypedPublished)
	}
}

// TestProgramArtifactComputationStagesFollowLocalOperandProducers pins the
// reusable Program-owned causal cut between a storage result and a primitive
// which consumes that result. Link sees only the resulting immutable points
// and full-environment transfers; it never reconstructs expression order.
func TestProgramArtifactComputationStagesFollowLocalOperandProducers(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-computation-stage.lua", Text: []byte(`
local function guard(): integer
    local cap = 3
    if cap > 5 then
        return 0
    end
    return cap
end
return guard
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile computation fixture: %s", failure.Error())
	}

	program := summaryProgram(t, artifact)
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	var order programschema.Occurrence
	var orderIndex int
	for index := 0; index < occurrenceCount; index++ {
		row, ok := program.OccurrenceAt(index)
		if ok && row.Kind() == programschema.OccurrenceBinaryOrder {
			if order.Available() {
				t.Fatal("fixture issued more than one order occurrence")
			}
			order = row
			orderIndex = index
		}
	}
	left, leftOK := program.OccurrenceInputID(orderIndex, 0)
	if !order.Available() || !leftOK {
		t.Fatal("fixture did not issue one order occurrence")
	}

	var storage programschema.Occurrence
	for index := 0; index < occurrenceCount; index++ {
		row, ok := program.OccurrenceAt(index)
		if !ok || row.Kind() != programschema.OccurrenceStorageRead {
			continue
		}
		span, spanOK := program.OccurrenceInputID(index, 1)
		if spanOK && span == left {
			if storage.Available() {
				t.Fatal("order operand has duplicate storage origins")
			}
			storage = row
		}
	}
	if !storage.Available() {
		t.Fatal("order left operand did not retain its exact storage origin")
	}

	storageRule, storageOK := ruleForOccurrence(artifact, "value-transfer", storage.ID())
	orderRule, orderRuleOK := ruleForOccurrence(artifact, "value-binary-order", order.ID())
	storagePoint := storageRule.PointID()
	storageInput, storageInputOK := storageRule.InputPointAt(0)
	orderPoint := orderRule.PointID()
	orderInput, orderInputOK := orderRule.InputPointAt(0)
	if !storageOK || !orderRuleOK || !storagePoint.Available() || !storageInputOK || !orderPoint.Available() || !orderInputOK ||
		storagePoint == storageInput || orderPoint == storagePoint || orderInput != storagePoint ||
		orderRule.Stage() != programissuance.StageComputation || orderRule.InputSpec() != programissuance.InputPreviousStage {
		t.Fatalf("local computation chain storage=%+v/%t order=%+v/%t", storageRule, storageOK, orderRule, orderRuleOK)
	}

	transferCount, transfersPublished := program.LocalTransferCount()
	if !transfersPublished {
		t.Fatal("local-transfer family is unpublished")
	}
	baseToStorage, storageToOrder := false, false
	for index := 0; index < transferCount; index++ {
		edge, ok := program.LocalTransferAt(index)
		if !ok || !edge.Full() {
			continue
		}
		baseToStorage = baseToStorage || edge.From() == storageInput && edge.To() == storagePoint
		storageToOrder = storageToOrder || edge.From() == storagePoint && edge.To() == orderPoint
	}
	if !baseToStorage || !storageToOrder {
		t.Fatalf("full local chain base->storage/order = %v/%v", baseToStorage, storageToOrder)
	}
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	edgeCount, edgesPublished := programschema.EnvironmentEdgeFamily().Count(&program.Frozen, catalog)
	if !catalogOK || !edgesPublished {
		t.Fatal("environment-edge family is unpublished")
	}
	continuation := false
	for index := 0; index < edgeCount; index++ {
		edge, ok := programschema.EnvironmentEdgeFamily().At(&program.Frozen, catalog, index)
		// From names where the route came from and never moves; Departure is
		// where its state actually leaves, which is the source's terminal
		// stage once that point has been staged.
		continuation = continuation || ok && edge.Departure() == orderPoint
	}
	if !continuation {
		t.Fatal("Program continuation did not depart the terminal computation stage")
	}
}

func TestProgramArtifactNestedComputationsFollowSemanticDependencies(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-nested-computation-stage.lua", Text: []byte(`
local function guard(): boolean
    local cap = 3
    return (cap > 5) == false
end
return guard
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile nested computation fixture: %s", failure.Error())
	}

	program := summaryProgram(t, artifact)
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	var order, equality programschema.Occurrence
	for index := 0; index < occurrenceCount; index++ {
		row, ok := program.OccurrenceAt(index)
		if !ok {
			continue
		}
		switch row.Kind() {
		case programschema.OccurrenceBinaryOrder:
			order = row
		case programschema.OccurrenceBinaryEquality:
			left, leftOK := program.OccurrenceInputID(index, 0)
			right, rightOK := program.OccurrenceInputID(index, 1)
			if leftOK && rightOK && (left == order.ID() || right == order.ID()) {
				equality = row
			}
		}
	}
	// Equality rows are copied before order rows in the generic occurrence
	// catalog, so resolve the dependency in a second pass rather than relying
	// on that storage order.
	if order.Available() && !equality.Available() {
		for index := 0; index < occurrenceCount; index++ {
			row, ok := program.OccurrenceAt(index)
			if !ok || row.Kind() != programschema.OccurrenceBinaryEquality {
				continue
			}
			left, leftOK := program.OccurrenceInputID(index, 0)
			right, rightOK := program.OccurrenceInputID(index, 1)
			if leftOK && rightOK && (left == order.ID() || right == order.ID()) {
				equality = row
				break
			}
		}
	}
	orderRule, orderOK := ruleForOccurrence(artifact, "value-binary-order", order.ID())
	equalityRule, equalityOK := ruleForOccurrence(artifact, "value-binary-equality", equality.ID())
	orderPoint := orderRule.PointID()
	equalityPoint := equalityRule.PointID()
	equalityInput, equalityInputOK := equalityRule.InputPointAt(0)
	if !order.Available() || !equality.Available() || !orderOK || !equalityOK || !orderPoint.Available() || !equalityPoint.Available() || !equalityInputOK ||
		equalityInput != orderPoint || equalityPoint == orderPoint || equalityRule.InputSpec() != programissuance.InputPreviousStage {
		t.Fatalf("nested computation dependency order=%+v/%t equality=%+v/%t", orderRule, orderOK, equalityRule, equalityOK)
	}
	transferCount, transfersPublished := program.LocalTransferCount()
	if !transfersPublished {
		t.Fatal("local-transfer family is unpublished")
	}
	linked := false
	for index := 0; index < transferCount; index++ {
		edge, ok := program.LocalTransferAt(index)
		linked = linked || ok && edge.Full() && edge.From() == orderPoint && edge.To() == equalityPoint
	}
	if !linked {
		t.Fatal("nested computation dependency lacks an exact full local transfer")
	}
}

func ruleForOccurrence(artifact *programartifact.Artifact, key schema.Key, occurrence identity.ContentID) (programschema.RuleOccurrence, bool) {
	program := artifact.Program()
	count, published := program.RuleOccurrenceCountForKey(string(key))
	if !published {
		return programschema.RuleOccurrence{}, false
	}
	for index := 0; index < count; index++ {
		row, ok := program.RuleOccurrenceForKeyAt(string(key), index)
		if !ok {
			continue
		}
		ordinal, ordinalOK := row.Occurrence()
		parent, parentOK := program.OccurrenceAt(int(ordinal))
		if ordinalOK && parentOK && parent.ID() == occurrence {
			return row, true
		}
	}
	return programschema.RuleOccurrence{}, false
}
