package runtimeentry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealRejectsUnavailableOwnerProofs(t *testing.T) {
	staticID := identity.ContentID{3}
	moduleID := identity.ContentID{4}
	if result, err := Seal(source.View{}, authored.View{}, nil, nil, nil, staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted unavailable owner proofs: result=%v err=%v", result, err)
	}
	if result, err := Seal(source.View{}, authored.View{}, &sourcecontrol.Result{}, &evaluation.Ports{}, &executable.Result{}, staticID, moduleID); err == nil || result != nil {
		t.Fatalf("Seal accepted malformed owner proofs: result=%v err=%v", result, err)
	}
}
