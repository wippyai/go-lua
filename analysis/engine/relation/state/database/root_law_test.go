package database_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// These refusal laws intentionally use only public doors.  A caller cannot
// manufacture a database root, candidate, or partial index vector from
// zero values; valid database construction is exercised by the transaction
// vertical laws using a checked Mounted witness.
func TestDatabaseRefusesUnsealedAndPartialInputs(t *testing.T) {
	if value, ok := database.Bootstrap(witness.Mounted{}, geometry.Geometry{}); ok || value.Available() {
		t.Fatal("zero database input was accepted")
	}
	if prepared, ok := database.Prepare(database.Version{}, store.Prepared{}, nil, contribution.Directory{}, contribution.State{}, nil); ok || prepared.Available() {
		t.Fatal("unsealed database candidate was accepted")
	}
	if next, delta, ok := database.Commit(database.Prepared{}); ok || next.Available() || delta.Available() {
		t.Fatal("unsealed database commit published a root")
	}
}

// The fresh-root door is intentionally hostile to post-commit/rebased store
// roots. Its exact function shape is the proof: only the sealed Mounted and
// Geometry authorities are accepted, so there is no store candidate to pass
// as a new database root and no alternate constructor to bypass Bootstrap.
func TestBootstrapFreshRootDoorHasNoStoreIngress(t *testing.T) {
	door := reflect.TypeOf(database.Bootstrap)
	if door.Kind() != reflect.Func || door.NumIn() != 2 || door.NumOut() != 2 {
		t.Fatalf("Bootstrap shape=%v, want func(Mounted, Geometry)(Version, bool)", door)
	}
	if door.In(0) != reflect.TypeOf(witness.Mounted{}) || door.In(1) != reflect.TypeOf(geometry.Geometry{}) {
		t.Fatalf("Bootstrap inputs=%v,%v admit a non-sealed source", door.In(0), door.In(1))
	}
	if door.Out(0) != reflect.TypeOf(database.Version{}) || door.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Bootstrap outputs=%v", []reflect.Type{door.Out(0), door.Out(1)})
	}
	if door.In(0) == reflect.TypeOf(store.Version{}) || door.In(1) == reflect.TypeOf(store.Version{}) {
		t.Fatal("Bootstrap admitted a committed store root")
	}
}
