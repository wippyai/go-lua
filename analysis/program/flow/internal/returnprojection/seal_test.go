package returnprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealRejectsUnavailableOrForeignOwnerInputs(t *testing.T) {
	staticID := identity.ContentID{3}
	moduleID := identity.ContentID{4}
	if result, err := Seal(source.View{}, authored.View{}, nil, nil, nil, staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted unavailable owners: result=%v err=%v", result, err)
	}
	if result, err := Seal(source.View{}, authored.View{}, &body.Result{}, &outcome.Result{}, &executable.Result{}, staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted malformed owner projections: result=%v err=%v", result, err)
	}
}
