package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func TestStaticRowsRejectOutOfRangeChildQueries(t *testing.T) {
	artifact := &Artifact{}
	if program := artifact.Program(); program.Available() {
		t.Fatal("unavailable artifact exposed a Program")
	}
	state, stateOK := programstate.New(snapshot.Frozen{}, identity.ContentID{})
	if stateOK {
		t.Fatal("unpublished Program opened a ColdState")
	}
	if _, ok := staticnode.NewView(state); ok {
		t.Fatal("unavailable ColdState exposed static node rows")
	}
}
