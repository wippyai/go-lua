package pathevidence

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestApplyCoordinatePresenceImplicationsEqualsCanonicalLanePublication(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	trigger := ks.FromPath(pathdom.NewPath(symbol.ID(11), "value"))
	target := ks.FromPath(pathdom.NewPath(symbol.ID(12), "err"))
	implications := []PathPresenceImplication{
		NewPathPresenceImplication(trigger, presence.Present(), target, presence.Absent()),
		NewPathPresenceImplication(trigger, presence.Absent(), target, presence.Present()),
	}
	implications, ok := CanonicalPathPresenceImplications(reg, ks, implications)
	if !ok {
		t.Fatal("implication fixture did not canonicalize")
	}

	input := Domain(reg).Bottom()
	skeleton, entries, ok := DecomposeCoordinates(input, ks)
	if !ok {
		t.Fatal("Bottom coordinate decomposition failed")
	}
	skeleton, entries, ok = ApplyCoordinatePresenceImplications(skeleton, entries, reg, ks, implications)
	if !ok {
		t.Fatal("factorwise implication publication failed")
	}
	got, ok := ComposeCoordinates(skeleton, entries, reg, ks)
	if !ok {
		t.Fatal("factorwise implication result did not compose")
	}

	want := input
	for _, implication := range implications {
		want, _ = want.AddPathPresenceImplication(implication)
	}
	if !Domain(reg).Equal(got, want) {
		t.Fatal("factorwise implication publication differs from canonical Lane publication")
	}
	if got.RefinementsBottom() || got.StaticMembersBottom() || got.ProofsBottom() || got.PathPresenceImplicationsBottom() {
		t.Fatal("implication publication did not establish coupled lane reachability")
	}
}

func TestImplicationCoordinateSeparatesAddressFromClauseLattice(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	trigger := ks.FromPath(pathdom.NewPath(symbol.ID(21), "ok"))
	target := ks.FromPath(pathdom.NewPath(symbol.ID(22), "payload"))
	value := func(name string) product.Value {
		literal := typ.LiteralString(name)
		return typevalue.WithWitness(reg, typevalue.FromType(reg, literal), literal)
	}
	leftKey, left := implicationCoordinateParts(NewPathTruthinessValueRefinementImplication(trigger, true, target, value("left")))
	rightKey, right := implicationCoordinateParts(NewPathTruthinessValueRefinementImplication(trigger, true, target, value("right")))
	if leftKey != rightKey {
		t.Fatal("dynamic target values changed the structural implication address")
	}
	if joined := CoordinateScalarJoin(reg, left, right); joined.present {
		t.Fatalf("distinct must clauses survived join: %#v", joined)
	}
	meet := CoordinateScalarMeet(reg, left, right)
	if !meet.present || len(meet.clauses) != 2 {
		t.Fatalf("meet clauses = %#v, want canonical union", meet)
	}
	bottom := CoordinateDefault(CoordinateBottom(), leftKey, reg)
	if !bottom.clauseBottom || !CoordinateScalarEqual(reg, CoordinateScalarJoin(reg, bottom, left), left) {
		t.Fatalf("clause Bottom join law failed: bottom=%#v", bottom)
	}
}
