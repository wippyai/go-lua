package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func TestBoundaryTransportRebasesReturnPresenceCorrelationToCallerResults(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	returnValue := from.FromPath(pathdom.Path{Root: "ret[0]"})
	returnError := from.FromPath(pathdom.Path{Root: "ret[1]"})
	callerValue := to.FromPath(pathdom.NewPath(101, "value"))
	callerError := to.FromPath(pathdom.NewPath(102, "err"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	source := Domain(reg).Bottom().
		WriteReturnSlot(reg, 0, present).
		WriteReturnSlot(reg, 1, absent).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			returnError, presence.Absent(), returnValue, presence.Present(),
		))

	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Slot: key.ReturnSlot(0), Path: returnValue, Value: present},
		{Slot: key.ReturnSlot(1), Path: returnError, Value: absent},
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceImplication := pathevidence.NewPathPresenceImplication(
		returnError, presence.Absent(), returnValue, presence.Present(),
	)
	if !artifact.world.HasPathPresenceImplication(sourceImplication) {
		t.Fatal("boundary projection dropped return-slot presence correlation")
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: callerValue, ToSlot: key.SymbolValue(101)},
		{FromRoot: 1, ToRoot: 1, To: callerError, ToSlot: key.SymbolValue(102)},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	want := pathevidence.NewPathPresenceImplication(
		callerError, presence.Absent(), callerValue, presence.Present(),
	)
	if !rebased.world.HasPathPresenceImplication(want) {
		t.Fatal("boundary rebase dropped return-slot presence correlation")
	}
	got, err := ApplyBoundary(reg, to, Reachable(Domain(reg).Bottom()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasPathPresenceImplication(want) {
		t.Fatal("return-slot presence correlation did not rebase to caller result paths")
	}
}
