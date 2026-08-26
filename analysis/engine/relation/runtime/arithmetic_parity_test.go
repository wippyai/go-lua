package runtime_test

import (
	"testing"

	deltaeval "github.com/wippyai/go-lua/analysis/engine/relation/eval/delta"
	stepeval "github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestArithmeticSolvePublishesPhysicalRows(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("arithmetic physical solve")
	}
	if result.Evaluations() != 1 {
		t.Fatalf("arithmetic schedule evaluations: got=%d want=1", result.Evaluations())
	}
	if evaluations := fixture.ArithmeticEvaluations(); evaluations != 2 {
		t.Fatalf("arithmetic applications: got=%d want=2 solve-evaluations=%d publications=%d", evaluations, result.Evaluations(), result.Publications())
	}
	reader, ok := fixture.Output(result.Root())
	if !ok || !reader.Available() {
		t.Fatal("arithmetic output reader")
	}
	rows := 0
	completed, valid := reader.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() {
			return false
		}
		rows++
		return true
	})
	if !completed || !valid || rows != 2 {
		t.Fatalf("arithmetic output rows: completed=%v valid=%v rows=%d", completed, valid, rows)
	}
	payload, ok := fixture.OutputPayload(result.Root())
	if !ok || !payload.Available() {
		t.Fatal("arithmetic output payload reader")
	}
	want := fixture.Expected()
	seen := make(map[model.RowID]identity.ContentID, len(want))
	completed, valid = payload.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() || len(row.Cells()) != 1 {
			return false
		}
		cells := row.Cells()
		if cells[0].Column() != fixture.IDs().OutputWrite || !cells[0].Value().Available() {
			return false
		}
		seen[row.ID()] = cells[0].Value().Opaque()
		return true
	})
	if !completed || !valid || len(seen) != len(want) {
		t.Fatalf("arithmetic output payload: completed=%v valid=%v rows=%d want=%d", completed, valid, len(seen), len(want))
	}
	for row, expected := range want {
		if got, ok := seen[row]; !ok || got != expected {
			t.Fatalf("arithmetic output row %v: got=%v want=%v present=%v", row, got, expected, ok)
		}
	}
}

// TestArithmeticFullLaterParityPublishesIdenticalRows proves the semantic
// parity boundary for the canonical arithmetic target. The source transition
// is issued by the fixture's existing SeedSource worker and committed through
// Apply/Publish; Full evaluates the sealed dependency at the successor root,
// while Later redeems the same dependency from the authenticated delta.
// Their final committed root and output row order, payload, scope, and lineage
// must agree exactly; their publication cardinality intentionally differs
// because Later settles only the affected row.
func TestArithmeticFullLaterParityPublishesIdenticalRows(t *testing.T) {
	fixture := arithmetic.New(t, 0xA2)
	seed, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !seed.Available() || fixture.ArithmeticEvaluations() != 2 {
		t.Fatal("arithmetic predecessor solve")
	}
	inputDelta, ok := fixture.SourceAChangedDelta(seed.Root())
	if !ok || !inputDelta.Available() || !inputDelta.Base().Same(seed.Root()) || !inputDelta.Next().SuccessorOf(inputDelta.Base()) {
		t.Fatal("arithmetic owner-issued source delta")
	}
	laterRoot, ok := fixpoint.Later(inputDelta)
	if !ok || !laterRoot.Available() {
		t.Fatal("arithmetic Later root")
	}
	execution := fixture.Mounted().Arrangement().Execution()
	schedules := execution.Schedules()
	if len(schedules) != 1 || schedules[0].Dependency() != fixture.IDs().Dependency {
		t.Fatalf("arithmetic schedule count/dependency: count=%d", len(schedules))
	}
	entry := schedules[0]

	fullSession, ok := stepeval.New(fixture.Mounted(), inputDelta.Next(), fixture.View())
	if !ok || !fullSession.Available() {
		t.Fatal("arithmetic Full evaluator session")
	}
	full, ok := fullSession.Evaluate(entry)
	if !ok || !full.Available() || full.Kind() != algebra.KindPublish {
		t.Fatal("arithmetic Full evaluation")
	}

	laterSession, ok := deltaeval.New(fixture.Mounted(), laterRoot, fixture.View())
	if !ok || !laterSession.Available() {
		t.Fatal("arithmetic Later evaluator session")
	}
	later, ok := laterSession.Evaluate(entry)
	if !ok || !later.Available() || later.Kind() != algebra.KindPublish {
		t.Fatal("arithmetic Later evaluation")
	}

	if full.Dependency() != later.Dependency() || full.Expression() != later.Expression() || full.Node() != later.Node() || full.Kind() != later.Kind() {
		t.Fatal("arithmetic Full/Later result identity diverged")
	}
	fullSettlements := full.Settlements()
	laterSettlements := later.Settlements()
	if len(fullSettlements) != 2 || len(laterSettlements) != 1 || !fullSettlements[0].Available() || !fullSettlements[1].Available() || !laterSettlements[0].Available() {
		t.Fatalf("arithmetic publication cardinality Full=%d Later=%d", len(fullSettlements), len(laterSettlements))
	}
	if !fullSettlements[0].Changed() || fullSettlements[1].Changed() || !laterSettlements[0].Changed() {
		t.Fatal("arithmetic Full/Later changed-publication cardinality")
	}
	children := entry.Node().Children()
	if len(children) != 1 {
		t.Fatalf("arithmetic publish child count=%d", len(children))
	}
	applyNode, applyNodeOK := children[0].Apply()
	if !applyNodeOK || !applyNode.Available() {
		t.Fatal("arithmetic child Apply binding")
	}
	fullApplications := full.Applications()
	laterApplications := later.Applications()
	if len(fullApplications) != 1 || len(laterApplications) != 1 || !fullApplications[0].Available() || !laterApplications[0].Available() || fullApplications[0].Operation() != applyNode.Operation() || laterApplications[0].Operation() != applyNode.Operation() {
		t.Fatalf("arithmetic Full/Later lost child Apply operation: full=%d later=%d", len(fullApplications), len(laterApplications))
	}
	if fullApplications[0].Len() != 2 || laterApplications[0].Len() != 1 {
		t.Fatalf("arithmetic Full/Later child Apply extent: full=%d later=%d", fullApplications[0].Len(), laterApplications[0].Len())
	}
	if evaluations := fixture.ArithmeticEvaluations(); evaluations != 5 {
		t.Fatalf("arithmetic invocation count Full=2 Later=1: got=%d", evaluations)
	}
	fullRoot := arithmeticFullRoot(t, fullSettlements)
	laterRootAfter := later.Next()
	if !laterRootAfter.Available() {
		t.Fatal("arithmetic Later successor")
	}
	fullPayload, ok := fixture.OutputPayload(fullRoot)
	if !ok {
		t.Fatal("arithmetic Full output payload")
	}
	laterPayload, ok := fixture.OutputPayload(laterRootAfter)
	if !ok {
		t.Fatal("arithmetic Later output payload")
	}
	fullRows := arithmeticPayload(t, fullPayload)
	laterRows := arithmeticPayload(t, laterPayload)
	if len(fullRows) != 2 || len(laterRows) != len(fullRows) {
		t.Fatalf("arithmetic output row count Full=%d Later=%d", len(fullRows), len(laterRows))
	}
	for index := range fullRows {
		if !sameArithmeticRow(fullRows[index], laterRows[index]) {
			t.Fatalf("arithmetic output row %d diverged", index)
		}
	}
}

type arithmeticPayloadRow struct {
	id      model.RowID
	key     uint64
	scope   witness.Scope
	lineage model.LineageRef
	cells   []arithmeticPayloadCell
}

type arithmeticPayloadCell struct {
	column   model.ColumnID
	typeID   model.TypeID
	value    binding.ValueToken
	presence model.Presence
	scope    witness.Scope
	lineage  model.LineageRef
}

func arithmeticPayload(t *testing.T, reader read.Reader) []arithmeticPayloadRow {
	t.Helper()
	if !reader.Available() {
		t.Fatal("arithmetic payload reader")
	}
	rows := make([]arithmeticPayloadRow, 0, 2)
	completed, valid := reader.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() {
			return false
		}
		cells := row.Cells()
		if len(cells) != 1 {
			return false
		}
		cell := cells[0]
		if !cell.Available() {
			return false
		}
		rows = append(rows, arithmeticPayloadRow{
			id: row.ID(), key: uint64(row.Key()), scope: row.Scope(), lineage: row.Lineage(),
			cells: []arithmeticPayloadCell{{column: cell.Column(), typeID: cell.Type(), value: cell.Value(), presence: cell.Presence(), scope: cell.Scope(), lineage: cell.Lineage()}},
		})
		return true
	})
	if !completed || !valid {
		t.Fatal("arithmetic payload scan")
	}
	return rows
}

func sameArithmeticRow(left, right arithmeticPayloadRow) bool {
	if left.id != right.id || left.key != right.key || !left.scope.Same(right.scope) || left.lineage != right.lineage || len(left.cells) != len(right.cells) {
		return false
	}
	for index := range left.cells {
		first, second := left.cells[index], right.cells[index]
		if first.column != second.column || first.typeID != second.typeID || first.presence != second.presence || !first.scope.Same(second.scope) || first.lineage != second.lineage || first.value.Available() != second.value.Available() {
			return false
		}
		if first.value.Available() && !first.value.Same(second.value) {
			return false
		}
	}
	return true
}

func arithmeticFullRoot(t *testing.T, settlements []publish.Settlement) database.Version {
	t.Helper()
	if len(settlements) == 0 {
		t.Fatal("arithmetic Full settlements")
	}
	current := settlements[0].Base()
	if !current.Available() {
		t.Fatal("arithmetic Full settlement base")
	}
	for index, settlement := range settlements {
		if !settlement.Available() || !settlement.Base().Same(current) {
			t.Fatalf("arithmetic Full settlement chain at %d", index)
		}
		current = settlement.Next()
	}
	return current
}
