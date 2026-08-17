package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealRejectsUnavailableOwnerProofsBeforeBuildingGeometry(t *testing.T) {
	staticID := identity.ContentID{3}
	moduleID := identity.ContentID{4}
	if result, err := Seal(zeroSourceControlSourceView(), authored.View{}, nil, nil, nil, keyspace.MakeTerm(keyspace.FamilyBody, 1), staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted unavailable owners: result=%v err=%v", result, err)
	}
	if result, err := Seal(zeroSourceControlSourceView(), authored.View{}, &body.Result{}, &containment.Result{}, &control.Shape{}, keyspace.MakeTerm(keyspace.FamilyBody, 1), staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted malformed owner projections: result=%v err=%v", result, err)
	}
}

func zeroSourceControlSourceView() source.View { return source.View{} }
func zeroSourceControlFlowView() authored.View { return authored.View{} }
