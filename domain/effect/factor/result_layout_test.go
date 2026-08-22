package factor

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
)

// exactResultPin is the layout digest every effect-exact payload carries. It
// is stated here and again beside the composition, so the layout this
// package's codec laws write under and the layout the analyzer publishes under
// are two independent statements of one identity rather than one statement
// read twice.
const exactResultPin = "5dfd6657e139f6ce6e55a18677ec2c7cfcb9d1fc315bbef05b7218f54dbf3a54"

// exactResultTestLayout seals this family's publication layout the way the
// composition seals it: the shape derived from the fold the family's query
// registration declares, the row state vocabulary read from a sealed
// structural table, and the columns this package publishes.
var exactResultTestLayout = sealExactResultTestLayout()

func sealExactResultTestLayout() *plane.Sealed {
	table, tableOK := structuretest.Table(structure.PublicationPlaneSpecs())
	if !tableOK {
		return nil
	}
	shape, shapeOK := query.NewShape(ExactResultFamily, query.FoldGeneral)
	if !shapeOK {
		return nil
	}
	sealed, _ := plane.Seal(shape, table, ExactResultStates, ExactResultColumns())
	return sealed
}

// TestExactResultLayoutSeals states that the family's declaration is
// admissible and that it is the declaration the wire is pinned to: an unsealed
// layout would refuse every answer at publication, and a drifted one would
// refuse every payload written under the one it replaced.
func TestExactResultLayoutSeals(t *testing.T) {
	if !exactResultTestLayout.Available() || exactResultTestLayout.Family() != ExactResultFamily {
		t.Fatal("the effect-exact layout did not seal")
	}
	digest := exactResultTestLayout.Digest()
	if got := hex.EncodeToString(digest[:]); got != exactResultPin {
		t.Fatalf("layout digest = %s, want the pinned declaration %s", got, exactResultPin)
	}
	if exactResultTestLayout.RowWidth() != 2 {
		t.Fatalf("row width = %d, want the state byte plus the top flag", exactResultTestLayout.RowWidth())
	}
	variable, declared := exactResultTestLayout.Variable()
	if !declared || variable != ExactColumnAtoms {
		t.Fatalf("variable column = %d/%v, want the declared atom vector", variable, declared)
	}
	states := exactResultTestLayout.States()
	if len(states) != 1 || states[0] != structure.PublicationClassHeld {
		t.Fatalf("row states = %v, want the sealed publication class vocabulary", states)
	}
}
