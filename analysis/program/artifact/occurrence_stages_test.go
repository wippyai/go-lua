package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/domain/composite"
)

func summaryProgram(t testing.TB, artifact *programartifact.Artifact) cold.Program {
	t.Helper()
	frozen, catalog, published := artifact.ColdPublication()
	if !published {
		t.Fatal("summary cold publication unavailable")
	}
	artifactID := artifact.ID()
	module, moduleOK := identity.DeriveContentID("test/numeric-summary-module", artifactID[:])
	if !moduleOK {
		t.Fatal("summary test module identity unavailable")
	}
	program := cold.Program{
		Frozen: frozen, ModuleKey: module, ArtifactID: artifact.ID(), ProgramID: artifact.CompileKey().ProgramID(), SchemaID: artifact.CompileKey().SchemaDigest(),
	}
	if !program.Available() || !catalog.Available() {
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile scalar summary fixture: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	if !exactPublished || exactCount != 3 {
		t.Fatalf("exact scalar summary count = %d/%v, want 3/true", exactCount, exactPublished)
	}
	var occurrenceID identity.ContentID
	values := make(map[cold.ExactScalarSummaryRole]int64, 3)
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
	if values[cold.ExactScalarSummaryLeft] != 10 || values[cold.ExactScalarSummaryRight] != 5 || values[cold.ExactScalarSummaryResult] != 15 {
		t.Fatalf("exact scalar use summary=%v, want left=10 right=5 result=15", values)
	}
	arithmetic, arithmeticOK := artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, occurrenceID)
	body, bodyOK := arithmetic.BodyID()
	left, right, op, endpointsOK := arithmetic.BinaryArithmetic()
	rule, ruleOK := ruleForOccurrence(artifact, "value-binary-arithmetic", occurrenceID)
	point, pointOK := rule.PointAt(0)
	input, inputOK := rule.InputPoint()
	if !arithmeticOK || !bodyOK || !endpointsOK || !left.Available() || !right.Available() || op != flowkind.BinaryAdd ||
		!ruleOK || !pointOK || !inputOK || point == input || rule.Stage() != programartifact.RuleStageLocal {
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
		numeric.Operator() != cold.SummaryOperator(flowkind.BinaryAdd) || numericLeft != cold.NumericRepresentationInteger ||
		numericRight != cold.NumericRepresentationInteger || numericResult != cold.NumericRepresentationInteger {
		t.Fatalf("arithmetic summary=%+v/%v representations=%d/%d/%d/%v", numeric, numericOK, numericLeft, numericRight, numericResult, representationsOK)
	}
	if _, ok := program.ExactScalarSummaryAt(-1); ok {
		t.Fatal("negative scalar summary index accepted")
	}
}

func TestProgramArtifactExactScalarSummaryWithholdsMergedMutation(t *testing.T) {
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile scalar merge fixture: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	exactCount, exactPublished := program.ExactScalarSummaryCount()
	if !exactPublished {
		t.Fatal("exact scalar summary column unpublished")
	}
	if exactCount != 0 {
		rows := make([]struct {
			role    cold.ExactScalarSummaryRole
			subject identity.ContentID
			literal keyspace.LiteralValue
		}, 0, exactCount)
		for index := 0; index < exactCount; index++ {
			row, _ := program.ExactScalarSummaryAt(index)
			coldLiteral, _ := row.Literal()
			literal := keyspace.LiteralValue{Kind: keyspace.LiteralKind(coldLiteral.Kind), Integer: coldLiteral.Integer, FloatBits: coldLiteral.FloatBits}
			rows = append(rows, struct {
				role    cold.ExactScalarSummaryRole
				subject identity.ContentID
				literal keyspace.LiteralValue
			}{role: row.Role(), subject: row.SubjectID(), literal: literal})
		}
		t.Fatalf("merged mutation issued %d exact scalar summaries: %+v", exactCount, rows)
	}
	arithmeticCount, arithmeticPublished := program.ArithmeticSummaryCount()
	if !arithmeticPublished || arithmeticCount != 1 {
		t.Fatalf("merged mutation arithmetic summary count=%d/%v, want 1/true", arithmeticCount, arithmeticPublished)
	}
	summary, summaryOK := program.ArithmeticSummaryAt(0)
	left, right, result, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || summary.Operator() != cold.SummaryOperator(flowkind.BinaryAdd) ||
		left != cold.NumericRepresentationNumber || right != cold.NumericRepresentationNumber || result != cold.NumericRepresentationNumber {
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
		compilation, compilationOK := composite.Global()
		if !compilationOK {
			t.Fatal("Program artifact grammar unavailable")
		}
		artifact, failure := composite.CompileArtifactDetailed(published, compilation)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile guarded divisor fixture: %s", failure.Error())
		}
		return artifact
	}
	for _, test := range []struct {
		name, guard string
		want        cold.ArithmeticDivisorProperty
	}{
		{name: "both", guard: "b ~= 0 and b ~= -1", want: cold.ArithmeticDivisorNonzeroNotMinusOne},
		{name: "zero", guard: "b ~= 0", want: cold.ArithmeticDivisorNonzero},
		{name: "disjunction", guard: "b ~= 0 or b ~= -1", want: cold.ArithmeticDivisorNone},
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
		if !summaryOK || !representationsOK || summary.Operator() != cold.SummaryOperator(flowkind.BinaryIDiv) ||
			left != cold.NumericRepresentationInteger || right != cold.NumericRepresentationInteger || result != cold.NumericRepresentationInteger ||
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
	if !summaryOK || summary.DivisorProperty() != cold.ArithmeticDivisorNone {
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
		compilation, compilationOK := composite.Global()
		if !compilationOK {
			t.Fatal("Program artifact grammar unavailable")
		}
		artifact, failure := composite.CompileArtifactDetailed(published, compilation)
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
	occurrence, occurrenceOK := artifact.OccurrenceForID(programartifact.OccurrenceUnary, summary.OccurrenceID())
	outputOwned := false
	for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
		point, pointOK := occurrence.PointAt(pointIndex)
		outputOwned = outputOwned || pointOK && point == summary.OutputPointID()
	}
	if !summaryOK || !representationsOK || !occurrenceOK || !outputOwned ||
		summary.Operator() != cold.SummaryOperator(flowkind.UnaryNeg) || operand != cold.NumericRepresentationInteger || result != cold.NumericRepresentationInteger {
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile computation fixture: %s", failure.Error())
	}

	var order programartifact.OccurrenceRow
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, ok := artifact.OccurrenceAt(index)
		if ok && row.Kind() == programartifact.OccurrenceBinaryOrder {
			if order.Available() {
				t.Fatal("fixture issued more than one order occurrence")
			}
			order = row
		}
	}
	left, _, _, orderOK := order.BinaryOrder()
	if !orderOK {
		t.Fatal("fixture did not issue one order occurrence")
	}

	var storage programartifact.OccurrenceRow
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, ok := artifact.OccurrenceAt(index)
		if !ok || row.Kind() != programartifact.OccurrenceStorageRead {
			continue
		}
		_, span, readOK := row.StorageRead()
		if readOK && span == left {
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
	storagePoint, storagePointOK := storageRule.PointAt(0)
	storageInput, storageInputOK := storageRule.InputPoint()
	orderPoint, orderPointOK := orderRule.PointAt(0)
	orderInput, orderInputOK := orderRule.InputPoint()
	if !storageOK || !orderRuleOK || !storagePointOK || !storageInputOK || !orderPointOK || !orderInputOK ||
		storagePoint == storageInput || orderPoint == storagePoint || orderInput != storagePoint ||
		orderRule.Stage() != programartifact.RuleStageLocal || orderRule.InputKind() != programartifact.RuleInputFinish {
		t.Fatalf("local computation chain storage=%+v/%t order=%+v/%t", storageRule, storageOK, orderRule, orderRuleOK)
	}

	baseToStorage, storageToOrder := false, false
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		edge, ok := artifact.LocalTransferAt(index)
		if !ok || !edge.FullEnvironment() {
			continue
		}
		baseToStorage = baseToStorage || edge.From() == storageInput && edge.To() == storagePoint
		storageToOrder = storageToOrder || edge.From() == storagePoint && edge.To() == orderPoint
	}
	if !baseToStorage || !storageToOrder {
		t.Fatalf("full local chain base->storage/order = %v/%v", baseToStorage, storageToOrder)
	}
	continuation := false
	for index := 0; index < artifact.EnvironmentEdgeCount(); index++ {
		edge, ok := artifact.EnvironmentEdgeAt(index)
		continuation = continuation || ok && edge.From() == orderPoint
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile nested computation fixture: %s", failure.Error())
	}

	var order, equality programartifact.OccurrenceRow
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, ok := artifact.OccurrenceAt(index)
		if !ok {
			continue
		}
		switch row.Kind() {
		case programartifact.OccurrenceBinaryOrder:
			order = row
		case programartifact.OccurrenceBinaryEquality:
			left, right, _, binaryOK := row.BinaryEquality()
			if binaryOK && (left == order.ID() || right == order.ID()) {
				equality = row
			}
		}
	}
	// Equality rows are copied before order rows in the generic occurrence
	// catalog, so resolve the dependency in a second pass rather than relying
	// on that storage order.
	if order.Available() && !equality.Available() {
		for index := 0; index < artifact.OccurrenceCount(); index++ {
			row, ok := artifact.OccurrenceAt(index)
			if !ok || row.Kind() != programartifact.OccurrenceBinaryEquality {
				continue
			}
			left, right, _, binaryOK := row.BinaryEquality()
			if binaryOK && (left == order.ID() || right == order.ID()) {
				equality = row
				break
			}
		}
	}
	orderRule, orderOK := ruleForOccurrence(artifact, "value-binary-order", order.ID())
	equalityRule, equalityOK := ruleForOccurrence(artifact, "value-binary-equality", equality.ID())
	orderPoint, orderPointOK := orderRule.PointAt(0)
	equalityPoint, equalityPointOK := equalityRule.PointAt(0)
	equalityInput, equalityInputOK := equalityRule.InputPoint()
	if !order.Available() || !equality.Available() || !orderOK || !equalityOK || !orderPointOK || !equalityPointOK || !equalityInputOK ||
		equalityInput != orderPoint || equalityPoint == orderPoint || equalityRule.InputKind() != programartifact.RuleInputFinish {
		t.Fatalf("nested computation dependency order=%+v/%t equality=%+v/%t", orderRule, orderOK, equalityRule, equalityOK)
	}
	linked := false
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		edge, ok := artifact.LocalTransferAt(index)
		linked = linked || ok && edge.FullEnvironment() && edge.From() == orderPoint && edge.To() == equalityPoint
	}
	if !linked {
		t.Fatal("nested computation dependency lacks an exact full local transfer")
	}
}

func ruleForOccurrence(artifact *programartifact.Artifact, key schema.Key, occurrence identity.ContentID) (programartifact.RuleOccurrenceRow, bool) {
	for index := 0; index < artifact.RulePlacementCountForKey(key); index++ {
		row, ok := artifact.RulePlacementForKeyAt(key, index)
		if ok && row.ID() == occurrence {
			return row, true
		}
	}
	return programartifact.RuleOccurrenceRow{}, false
}
