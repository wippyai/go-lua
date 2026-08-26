package column_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/eval/delta"
	expandfixture "github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// A semantic R payload replacement is a real Later source for Expand.  The
// law exercises the complete owner path: exact R ChangeReader frontier,
// frozen key evidence, C RowID inverse, and replay below/through the Expand
// zipper. No C or P publication is involved.
func TestLaterExpandReaderReplacementReplaysFromExactRChange(t *testing.T) {
	fixture := expandfixture.New(t)
	world := fixture.World()
	entry, ok := world.Mounted().Arrangement().Execution().Dependency(fixture.Dependency())
	if !ok || !entry.Available() || entry.Node().Kind() != algebra.KindExpand {
		t.Fatal("Expand dependency entry")
	}
	expandBinding, ok := entry.Node().Expand()
	if !ok || !expandBinding.Available() {
		t.Fatal("Expand binding")
	}

	base := world.Base()
	scratch := store.NewReadScratch(world.View().Manager())
	if scratch == nil || !scratch.Available() {
		t.Fatal("read scratch")
	}
	rReader, ok := read.Bind(base, expandBinding.Reader(), world.View(), scratch)
	if !ok || !rReader.Available() {
		t.Fatal("R reader")
	}

	var oldCell read.Cell
	var rowID model.RowID
	scope, scopeOK := world.Mounted().Scope(fixture.MainScope())
	if !scopeOK || !scope.Available() {
		t.Fatal("R scope")
	}
	completed, valid := rReader.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() || row.ID() != fixture.ReaderRowFor(fixture.Key1()) {
			return true
		}
		rowID = row.ID()
		scope = row.Scope()
		for _, cell := range row.Cells() {
			if cell.Column() == fixture.ReaderPayload1Column() {
				oldCell = cell
				break
			}
		}
		return false
	})
	if completed || !valid || !rowID.Available() || !oldCell.Available() {
		t.Fatalf("R seed lookup completed=%t valid=%t row=%v cell=%t", completed, valid, rowID, oldCell.Available())
	}

	readerDenominator := model.DenominatorRef{}
	for _, candidate := range world.Mounted().Denominators() {
		if candidate.Relation() == fixture.Contract().Reader() {
			if readerDenominator.Available() {
				t.Fatal("ambiguous R denominator")
			}
			readerDenominator = candidate
		}
	}
	if !readerDenominator.Available() {
		t.Fatal("R denominator")
	}
	readerWitness, ok := world.Mounted().Denominator(readerDenominator)
	if !ok {
		t.Fatal("R denominator witness")
	}
	cellToken, ok := world.Mounted().IssueCell(readerWitness, scope, fixture.ReaderPayload1Column(), rowID)
	if !ok {
		t.Fatal("R payload cell token")
	}
	coordinate, ok := world.View().Resolve(cellToken)
	if !ok || !coordinate.Available() {
		t.Fatal("R payload coordinate")
	}
	newOpaque, ok := identity.DeriveContentID("analysis/engine/relation/state/internal/column/expand-delta-law/v1", []byte("reader-payload-1-next"))
	if !ok {
		t.Fatal("new R payload identity")
	}
	newValue, ok := world.Mounted().IssueValue(fixture.TypeID(), newOpaque)
	if !ok {
		t.Fatal("new R payload value")
	}
	newCell, ok := column.NewCell(newValue, oldCell.Presence())
	if !ok {
		t.Fatal("new R payload cell")
	}
	update, ok := column.NewUpdate(coordinate.Dense(), coordinate.Mask(), newCell, oldCell.Lineage())
	if !ok {
		t.Fatal("R payload update")
	}
	columnVersion, ok := base.Store().Column(fixture.ReaderPayload1Column())
	if !ok {
		t.Fatal("R payload column")
	}
	_, columnDelta, ok := columnVersion.Next(update)
	if !ok || !columnDelta.Available() {
		t.Fatal("R payload column delta")
	}
	preparedStore, ok := store.Prepare(base.Store(), columnDelta)
	if !ok || !preparedStore.Available() {
		t.Fatal("store successor")
	}
	preparedDatabase, ok := database.Prepare(base, preparedStore, scratch, base.ContributionDirectory(), base.ContributionState(), nil)
	if !ok || !preparedDatabase.Available() {
		t.Fatal("database successor")
	}
	_, databaseDelta, ok := database.Commit(preparedDatabase)
	if !ok || !databaseDelta.Available() {
		t.Fatal("database delta")
	}
	later, ok := fixpoint.Later(databaseDelta)
	if !ok {
		t.Fatal("Later root")
	}
	session, ok := delta.New(world.Mounted(), later, world.View())
	if !ok || !session.Available() {
		t.Fatal("Later session")
	}
	result, ok := session.Evaluate(entry)
	if !ok || !result.Available() || result.Kind() != algebra.KindExpand {
		t.Fatalf("R-only Expand result ok=%t available=%t kind=%v", ok, result.Available(), result.Kind())
	}
	seen := 0
	for _, batch := range result.Batches() {
		for index := 0; index < batch.Len(); index++ {
			value, valueOK := batch.At(index)
			if !valueOK || !value.Available() {
				t.Fatal("Expand tuple")
			}
			for _, cell := range value.Cells() {
				if cell.Column() == fixture.ReaderPayload1Column() && cell.Value().Available() && cell.Value().Opaque() == newOpaque {
					seen++
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("R-only Expand replay omitted replacement payload")
	}
}
