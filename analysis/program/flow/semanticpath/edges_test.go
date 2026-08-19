package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestMergeContainmentRolesRequiresTheOwningForestAndPlane(t *testing.T) {
	var paths [keyspace.FamilyCount][]edgeDescriptor
	if err := mergeContainmentRoles(source.View{}, nil, &paths); err == nil {
		t.Fatal("nil containment result was accepted")
	}
	if err := mergeContainmentRoles(source.View{}, &containment.Result{}, nil); err == nil {
		t.Fatal("nil edge plane was accepted")
	}
}
