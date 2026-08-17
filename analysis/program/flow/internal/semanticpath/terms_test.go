package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDeriveTermPathsDoesNotInventTermsWithoutSourceDenominators(t *testing.T) {
	var edges [keyspace.FamilyCount][]edgeDescriptor
	var descriptors [keyspace.FamilyCount][]identity.ContentID
	var roots [keyspace.FamilyCount][]identity.ContentID
	paths, err := deriveTermPaths(source.View{}, source.CellRoles{}, authored.View{}, binding.Result{}, &body.Result{}, &containment.Result{}, &outcome.Result{}, edges, nil, descriptors, roots, nil)
	if err == nil {
		t.Fatal("term derivation accepted unavailable Cell role owners")
	}
	_ = paths
}
