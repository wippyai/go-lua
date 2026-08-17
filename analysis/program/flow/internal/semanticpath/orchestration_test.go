package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDeriveRequiresTheExactOwnerQuartetBeforeConstructingPlanes(t *testing.T) {
	ids := []identity.ContentID{{1}, {2}, {3}, {4}}
	if planes, err := derive(source.View{}, source.CellRoles{}, authored.View{}, &body.Result{}, binding.Result{}, &containment.Result{}, &outcome.Result{}, ids[0], ids[1], ids[2], ids[3]); err == nil || planes.body != nil {
		t.Fatalf("derive accepted views whose Source/Flow owners disagree: planes=%#v err=%v", planes, err)
	}
}
