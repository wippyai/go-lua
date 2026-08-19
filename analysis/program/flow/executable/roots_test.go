package executable

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestValidateBodyRootsRequiresExactParentAndSourceAgreement(t *testing.T) {
	counts := matrixCounts()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	fixture := openSealFixture(t, "direct-parent.lua", counts, [][]keyspace.Term{{bind, returned}}, matrixFlow(),
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}}}, nil, nil)
	if _, err := validateBodyRoots(fixture.bodies, fixture.forest, fixture.control, counts); err != nil {
		t.Fatalf("exact direct-root parent validation rejected a sealed fixture: %v", err)
	}
	if parent, ok := fixture.forest.Parent(bind); !ok || parent != body {
		t.Fatalf("forest Parent(Bind) = %v/%v, want Body %v/true", parent, ok, body)
	}
}
