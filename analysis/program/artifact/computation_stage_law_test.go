package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

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
	if artifact.ExactScalarSummaryCount() != 3 {
		t.Fatalf("exact scalar summary count = %d, want 3", artifact.ExactScalarSummaryCount())
	}
	var occurrenceID identity.ContentID
	values := make(map[programartifact.ExactScalarSummaryRole]int64, 3)
	for index := 0; index < artifact.ExactScalarSummaryCount(); index++ {
		summary, summaryOK := artifact.ExactScalarSummaryAt(index)
		literal, literalOK := summary.Literal()
		if !summaryOK || !literalOK || literal.Kind != keyspace.LiteralInteger || !summary.SubjectID().Available() || !summary.Role().Valid() {
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
	if values[programartifact.ExactScalarSummaryLeft] != 10 || values[programartifact.ExactScalarSummaryRight] != 5 || values[programartifact.ExactScalarSummaryResult] != 15 {
		t.Fatalf("exact scalar use summary=%v, want left=10 right=5 result=15", values)
	}
	arithmetic, arithmeticOK := artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, occurrenceID)
	body, bodyOK := arithmetic.BodyID()
	left, right, op, endpointsOK := arithmetic.BinaryArithmetic()
	rule, ruleOK := ruleForOccurrence(artifact, programartifact.RuleRoleValueBinaryArithmetic, occurrenceID)
	point, pointOK := rule.PointAt(0)
	input, inputOK := rule.InputPoint()
	if !arithmeticOK || !bodyOK || !endpointsOK || !left.Available() || !right.Available() || op != flowkind.BinaryAdd ||
		!ruleOK || !pointOK || !inputOK || point == input || rule.Stage() != programartifact.RuleStageLocal {
		t.Fatalf("scalar arithmetic=%+v/%v rule=%+v/%v", arithmetic, arithmeticOK, rule, ruleOK)
	}
	for index := 0; index < artifact.ExactScalarSummaryCount(); index++ {
		summary, _ := artifact.ExactScalarSummaryAt(index)
		if summary.BodyPathID() != body {
			t.Fatalf("scalar summary[%d] body=%s, want %s", index, summary.BodyPathID(), body)
		}
	}
	if artifact.ArithmeticSummaryCount() != 1 {
		t.Fatalf("arithmetic summary count=%d, want 1", artifact.ArithmeticSummaryCount())
	}
	numeric, numericOK := artifact.ArithmeticSummaryAt(0)
	numericLeft, numericRight, numericResult, representationsOK := numeric.Representations()
	if !numericOK || !representationsOK || numeric.OccurrenceID() != occurrenceID || numeric.BodyPathID() != body ||
		numeric.Operator() != flowkind.BinaryAdd || numericLeft != programartifact.NumericRepresentationInteger ||
		numericRight != programartifact.NumericRepresentationInteger || numericResult != programartifact.NumericRepresentationInteger {
		t.Fatalf("arithmetic summary=%+v/%v representations=%d/%d/%d/%v", numeric, numericOK, numericLeft, numericRight, numericResult, representationsOK)
	}
	if _, ok := artifact.ExactScalarSummaryAt(-1); ok {
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
	if artifact.ExactScalarSummaryCount() != 0 {
		rows := make([]struct {
			role    programartifact.ExactScalarSummaryRole
			subject identity.ContentID
			literal keyspace.LiteralValue
		}, 0, artifact.ExactScalarSummaryCount())
		for index := 0; index < artifact.ExactScalarSummaryCount(); index++ {
			row, _ := artifact.ExactScalarSummaryAt(index)
			literal, _ := row.Literal()
			rows = append(rows, struct {
				role    programartifact.ExactScalarSummaryRole
				subject identity.ContentID
				literal keyspace.LiteralValue
			}{role: row.Role(), subject: row.SubjectID(), literal: literal})
		}
		t.Fatalf("merged mutation issued %d exact scalar summaries: %+v", artifact.ExactScalarSummaryCount(), rows)
	}
	if artifact.ArithmeticSummaryCount() != 1 {
		t.Fatalf("merged mutation arithmetic summary count=%d, want 1", artifact.ArithmeticSummaryCount())
	}
	summary, summaryOK := artifact.ArithmeticSummaryAt(0)
	left, right, result, representationsOK := summary.Representations()
	if !summaryOK || !representationsOK || summary.Operator() != flowkind.BinaryAdd ||
		left != programartifact.NumericRepresentationNumber || right != programartifact.NumericRepresentationNumber || result != programartifact.NumericRepresentationNumber {
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
		want        programartifact.ArithmeticDivisorProperty
	}{
		{name: "both", guard: "b ~= 0 and b ~= -1", want: programartifact.ArithmeticDivisorNonzeroNotMinusOne},
		{name: "zero", guard: "b ~= 0", want: programartifact.ArithmeticDivisorNonzero},
		{name: "disjunction", guard: "b ~= 0 or b ~= -1", want: programartifact.ArithmeticDivisorNone},
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
		if artifact.ArithmeticSummaryCount() != 1 {
			t.Fatalf("%s arithmetic summary count=%d, want 1", test.name, artifact.ArithmeticSummaryCount())
		}
		summary, summaryOK := artifact.ArithmeticSummaryAt(0)
		left, right, result, representationsOK := summary.Representations()
		if !summaryOK || !representationsOK || summary.Operator() != flowkind.BinaryIDiv ||
			left != programartifact.NumericRepresentationInteger || right != programartifact.NumericRepresentationInteger || result != programartifact.NumericRepresentationInteger ||
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
	if unguarded.ArithmeticSummaryCount() != 1 {
		t.Fatalf("unguarded arithmetic summary count=%d, want 1", unguarded.ArithmeticSummaryCount())
	}
	summary, summaryOK := unguarded.ArithmeticSummaryAt(0)
	if !summaryOK || summary.DivisorProperty() != programartifact.ArithmeticDivisorNone {
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
	if artifact.UnarySummaryCount() != 1 {
		t.Fatalf("unary summary count=%d, want 1", artifact.UnarySummaryCount())
	}
	summary, summaryOK := artifact.UnarySummaryAt(0)
	operand, result, representationsOK := summary.Representations()
	occurrence, occurrenceOK := artifact.OccurrenceForID(programartifact.OccurrenceUnary, summary.OccurrenceID())
	outputOwned := false
	for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
		point, pointOK := occurrence.PointAt(pointIndex)
		outputOwned = outputOwned || pointOK && point == summary.OutputPointID()
	}
	if !summaryOK || !representationsOK || !occurrenceOK || !outputOwned ||
		summary.Operator() != flowkind.UnaryNeg || operand != programartifact.NumericRepresentationInteger || result != programartifact.NumericRepresentationInteger {
		t.Fatalf("unary summary=%+v/%v occurrence=%+v/%v output-owned=%v representations=%d/%d/%v", summary, summaryOK,
			occurrence, occurrenceOK, outputOwned, operand, result, representationsOK)
	}
	untyped := compile("artifact-unary-summary-untyped.lua", `
local function invert(n)
    return -n
end
return invert
`)
	if untyped.UnarySummaryCount() != 0 {
		t.Fatalf("untyped unary summaries=%d, want 0", untyped.UnarySummaryCount())
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

	storageRule, storageOK := ruleForOccurrence(artifact, programartifact.RuleRoleValueStorageTransfer, storage.ID())
	orderRule, orderRuleOK := ruleForOccurrence(artifact, programartifact.RuleRoleValueBinaryOrder, order.ID())
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
	orderRule, orderOK := ruleForOccurrence(artifact, programartifact.RuleRoleValueBinaryOrder, order.ID())
	equalityRule, equalityOK := ruleForOccurrence(artifact, programartifact.RuleRoleValueBinaryEquality, equality.ID())
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

func ruleForOccurrence(artifact *programartifact.Artifact, role programartifact.RuleRole, occurrence identity.ContentID) (programartifact.RuleOccurrenceRow, bool) {
	for index := 0; index < artifact.RuleOccurrenceCount(role); index++ {
		row, ok := artifact.RuleOccurrenceAt(role, index)
		if ok && row.ID() == occurrence {
			return row, true
		}
	}
	return programartifact.RuleOccurrenceRow{}, false
}
