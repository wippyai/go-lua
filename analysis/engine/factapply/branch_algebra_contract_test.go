package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// TestBranchAlgebraConditionSourceComparisonMatrix pins BranchAlgebra's
// ConditionSource/Condition contract at the seam behind the "branch: missing
// condition source" census family (32 external errors; minimal reproducer
// `local a, b; if a ~= b then end`): given a factflow.Facts with a registered
// BranchConditionSources entry, ConditionSource must return the lowered
// condition source unchanged and independent of edge polarity, across the
// comparison-operator matrix and both local-vs-local and local-vs-literal
// operand shapes.
func TestBranchAlgebraConditionSourceComparisonMatrix(t *testing.T) {
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("value source shape")
	}
	leftLocal, ok := factflow.NewExpressionValueSource(101, 0, 0, 0, shape)
	if !ok {
		t.Fatal("left local source")
	}
	rightLocal, ok := factflow.NewExpressionValueSource(102, 0, 0, 0, shape)
	if !ok {
		t.Fatal("right local source")
	}
	rightLiteral, ok := factflow.NewIntegerLiteralValueSource(7, 0, 0, 0, shape)
	if !ok {
		t.Fatal("right literal source")
	}

	operators := []string{"==", "~=", "<", "<="}
	operands := []struct {
		name  string
		right factflow.ValueSource
	}{
		{"local-vs-local", rightLocal},
		{"local-vs-literal", rightLiteral},
	}
	polarities := []struct {
		name             string
		truthyOnTrueEdge bool
	}{
		{"direct", true},
		{"negated", false},
	}

	exprID := factflow.ExprRef(200)
	pointID := cfg.Point(1)
	for _, op := range operators {
		for _, operand := range operands {
			for _, polarity := range polarities {
				name := op + "/" + operand.name + "/" + polarity.name
				t.Run(name, func(t *testing.T) {
					exprID++
					pointID++
					conditionSource, ok := factflow.NewExpressionValueSource(exprID, 0, 0, 0, shape)
					if !ok {
						t.Fatal("condition source")
					}
					operation, ok := factflow.NewBinaryExpressionOperation(op, leftLocal, operand.right)
					if !ok {
						t.Fatal("binary operation")
					}
					condition, ok := factflow.NewBranchCondition(conditionSource, polarity.truthyOnTrueEdge)
					if !ok {
						t.Fatal("branch condition")
					}
					facts := factflow.NewFacts(factflow.FactsInput{
						BranchConditionSources: map[cfg.Point]factflow.BranchCondition{pointID: condition},
						ExpressionOperations:   map[factflow.ExprRef]factflow.ExpressionOperation{exprID: operation},
					})
					algebra := NewBranchAlgebra(facts, pointID)

					if got := algebra.Point(); got != pointID {
						t.Fatalf("Point() = %d, want %d", got, pointID)
					}
					gotSource, ok := algebra.ConditionSource()
					if !ok || gotSource != conditionSource {
						t.Fatalf("ConditionSource() = %#v/%v, want %#v/true", gotSource, ok, conditionSource)
					}
					gotOp, ok := facts.ExpressionOperation(gotSource.ExprRef)
					if !ok || gotOp.Op() != op || gotOp.Left() != leftLocal || gotOp.Right() != operand.right {
						t.Fatalf("ExpressionOperation(%d) = %#v/%v, want op %q left %#v right %#v",
							gotSource.ExprRef, gotOp, ok, op, leftLocal, operand.right)
					}
					gotCondition, ok := algebra.Condition()
					if !ok || gotCondition.Source() != conditionSource || gotCondition.TruthyOnEdge(true) != polarity.truthyOnTrueEdge {
						t.Fatalf("Condition() = %#v/%v, want source %#v truthyOnTrueEdge=%v",
							gotCondition, ok, conditionSource, polarity.truthyOnTrueEdge)
					}
				})
			}
		}
	}
}

// TestBranchAlgebraConditionSourceAbsentReportsMissingConditionBlocker pins
// the negative half of the same seam: a branch point with no registered
// BranchConditionSources fact is exactly what GuardOnlyBlockers names
// "branch:missing-condition-source" for. This is the census family's blocker
// string origin, not a bug in ConditionSource itself: the delegate correctly
// reports absence, and upstream lowering (structural_freezer.go/compiler.go)
// is what must populate the fact for `a ~= b` over locals.
func TestBranchAlgebraConditionSourceAbsentReportsMissingConditionBlocker(t *testing.T) {
	point := cfg.Point(9)
	facts := factflow.NewFacts(factflow.FactsInput{})
	algebra := NewBranchAlgebra(facts, point)

	if gotSource, ok := algebra.ConditionSource(); ok || gotSource != (factflow.ValueSource{}) {
		t.Fatalf("ConditionSource() = %#v/%v, want zero-value/false", gotSource, ok)
	}
	guardOnly, reason := algebra.GuardOnly()
	if guardOnly || reason != "branch:missing-condition-source" {
		t.Fatalf("GuardOnly() = %v/%q, want false/\"branch:missing-condition-source\"", guardOnly, reason)
	}
	blockers := algebra.GuardOnlyBlockers()
	if len(blockers) == 0 || blockers[0] != "branch:missing-condition-source" {
		t.Fatalf("GuardOnlyBlockers() = %v, want first entry \"branch:missing-condition-source\"", blockers)
	}
}

// TestBranchAlgebraPresenceRelations pins BranchAlgebra.PresenceRelations as a
// borrowed, order-preserving view of the branch-triggered presence
// implications registered at one point, and confirms a non-empty set is
// exactly what GuardOnlyBlockers names "branch:presence-relation" for.
func TestBranchAlgebraPresenceRelations(t *testing.T) {
	point := cfg.Point(5)
	trigger := pathdom.NewPath(symbol.ID(1), "trigger")
	target := pathdom.NewPath(symbol.ID(2), "target")
	relation := factflow.NewBranchPresenceRelation(trigger, presence.Present(), target, presence.Absent())
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchPresenceRelations: map[cfg.Point]factflow.BranchPresenceRelationSet{
			point: factflow.NewBranchPresenceRelationSet(relation),
		},
	})
	algebra := NewBranchAlgebra(facts, point)

	got := algebra.PresenceRelations()
	if len(got) != 1 {
		t.Fatalf("PresenceRelations() = %#v, want one relation", got)
	}
	if !got[0].TriggerPathRef().Equal(trigger) || !presence.Equal(got[0].TriggerPresence(), presence.Present()) {
		t.Fatalf("trigger = %#v/%s, want %#v/present", got[0].TriggerPathRef(), got[0].TriggerPresence(), trigger)
	}
	if !got[0].TargetPathRef().Equal(target) || !presence.Equal(got[0].TargetPresence(), presence.Absent()) {
		t.Fatalf("target = %#v/%s, want %#v/absent", got[0].TargetPathRef(), got[0].TargetPresence(), target)
	}

	blockers := algebra.GuardOnlyBlockers()
	found := false
	for _, reason := range blockers {
		if reason == "branch:presence-relation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("GuardOnlyBlockers() = %v, want \"branch:presence-relation\"", blockers)
	}
}
