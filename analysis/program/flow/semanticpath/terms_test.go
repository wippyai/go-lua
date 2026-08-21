package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDeriveTermPathsDoesNotInventTermsWithoutSourceDenominators(t *testing.T) {
	paths, err := deriveTermPaths(source.View{}, source.CellRoles{}, authored.View{}, binding.Result{}, &body.Result{}, &containment.Result{}, &outcome.Result{}, nil, nil)
	if err == nil {
		t.Fatal("term derivation accepted unavailable Cell role owners")
	}
	_ = paths
}
