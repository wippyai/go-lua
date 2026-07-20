package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestFrozenTableDomainExactMeetLaws(t *testing.T) {
	domain := frozenTableDomain()
	a := identity.ID{Kind: "table", Site: "meet-a", Index: 1}
	b := identity.ID{Kind: "table", Site: "meet-b", Index: 2}
	left, _ := domain.Top().freeze(a)
	right, _ := domain.Top().freeze(b)
	both, _ := left.freeze(b)
	runMustLaneLaws(t, "state.frozen-tables", domain, []frozenTableLane{
		domain.Bottom(), domain.Top(), left, right, both,
	})
	if got := domain.Meet(left, right); !domain.Equal(got, both) {
		t.Fatal("frozen-table Meet did not union required identities")
	}
}

func TestStoreRelationDomainExactMeetLaws(t *testing.T) {
	domain := storeRelationDomain()
	a := StoreRelation{Source: mustTestStateKey(pathdom.PathKey("store-meet-a")), Into: mustTestStateKey(pathdom.PathKey("store-meet-out"))}
	b := StoreRelation{Source: mustTestStateKey(pathdom.PathKey("store-meet-b")), Into: a.Into}
	left, _ := domain.Top().add(a)
	right, _ := domain.Top().add(b)
	both, _ := left.add(b)
	runMustLaneLaws(t, "state.store-relations", domain, []storeRelationLane{
		domain.Bottom(), domain.Top(), left, right, both,
	})
	if got := domain.Meet(left, right); !domain.Equal(got, both) {
		t.Fatal("store-relation Meet did not union required relations")
	}
}

func TestDiffRelationDomainExactMeetLaws(t *testing.T) {
	domain := diffRelationDomain()
	aKey := mustTestStateKey(pathdom.PathKey("diff-meet-a"))
	bKey := mustTestStateKey(pathdom.PathKey("diff-meet-b"))
	cKey := mustTestStateKey(pathdom.PathKey("diff-meet-c"))
	a := RelConstraint{CoA: 1, A: RelValueOperand(aKey), C: RelValueOperand(cKey), K: 1}
	b := RelConstraint{CoA: 1, A: RelValueOperand(bKey), C: RelValueOperand(cKey), K: 2}
	left, _ := domain.Top().add(a)
	right, _ := domain.Top().add(b)
	both, _ := left.add(b)
	runMustLaneLaws(t, "state.diff-relations", domain, []diffRelationLane{
		domain.Bottom(), domain.Top(), left, right, both,
	})
	if got := domain.Meet(left, right); !domain.Equal(got, both) {
		t.Fatal("diff-relation Meet did not union required constraints")
	}
}

func runMustLaneLaws[T any](t *testing.T, name string, domain lattice.Lattice[T], sample []T) {
	t.Helper()
	if domain.Meet == nil {
		t.Fatalf("%s domain has no exact Meet", name)
	}
	latticelaws.LawSuite[T]{Name: name, Domain: domain, Sample: sample}.Run(t)
}
