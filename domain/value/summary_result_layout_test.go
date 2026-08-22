package value

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
)

// summaryResultPin is the layout digest every value-summary payload carries.
// It is stated here and again beside the composition, so the layout this
// package's codec laws write under and the layout the analyzer publishes under
// are two independent statements of one identity rather than one statement
// read twice.
const summaryResultPin = "58ee06e07da27d3cb74d5e15cff584861e77628bcd21c65d34e81e4eb01b3976"

// summaryResultTestLayout seals this family's publication layout the way the
// composition seals it: the shape derived from the fold the family's query
// registration declares, the row state vocabulary read from a sealed
// structural table, and the columns this package publishes.
var summaryResultTestLayout = sealSummaryResultTestLayout()

func sealSummaryResultTestLayout() *plane.Sealed {
	table, tableOK := structuretest.Table(structure.PublicationPlaneSpecs())
	if !tableOK {
		return nil
	}
	shape, shapeOK := query.NewShape(SummaryResultFamily, query.FoldDistributive)
	if !shapeOK {
		return nil
	}
	sealed, _ := plane.Seal(shape, table, SummaryResultStates, SummaryResultColumns())
	return sealed
}

// TestSummaryResultLayoutSeals states that the family's declaration is
// admissible and that it is the declaration the wire is pinned to: an unsealed
// layout would refuse every answer at publication, and a drifted one would
// refuse every payload written under the one it replaced.
func TestSummaryResultLayoutSeals(t *testing.T) {
	if !summaryResultTestLayout.Available() || summaryResultTestLayout.Family() != SummaryResultFamily {
		t.Fatal("the value-summary layout did not seal")
	}
	digest := summaryResultTestLayout.Digest()
	if got := hex.EncodeToString(digest[:]); got != summaryResultPin {
		t.Fatalf("layout digest = %s, want the pinned declaration %s", got, summaryResultPin)
	}
	if summaryResultTestLayout.RowWidth() != 2 {
		t.Fatalf("row width = %d, want the state byte plus the top flag", summaryResultTestLayout.RowWidth())
	}
	variable, declared := summaryResultTestLayout.Variable()
	if !declared || variable != SummaryColumnImage {
		t.Fatalf("variable column = %d/%v, want the declared compact image", variable, declared)
	}
	states := summaryResultTestLayout.States()
	if len(states) != 1 || states[0] != structure.PublicationClassHeld {
		t.Fatalf("row states = %v, want the sealed publication class vocabulary", states)
	}
}
