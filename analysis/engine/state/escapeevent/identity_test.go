package escapeevent

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestDomainJoinPreservesSharedFactSetIdentity(t *testing.T) {
	lane, _ := Top().Add(Fact{Target: pathaddr.StateKey("sym1@1.value"), Kind: KindBorrow})
	domain := Domain()
	if domain.Same == nil || !domain.Same(domain.Join(lane, lane), lane) {
		t.Fatal("escape-event domain did not preserve the shared persistent fact set")
	}
}

func TestDomainExactMeetLaws(t *testing.T) {
	a := Fact{Target: pathaddr.StateKey("sym1@1.value"), Kind: KindBorrow}
	b := Fact{Target: pathaddr.StateKey("sym2@1.value"), Kind: KindSend}
	left, _ := Top().Add(a)
	right, _ := Top().Add(b)
	both, _ := left.Add(b)
	domain := Domain()
	if domain.Meet == nil {
		t.Fatal("escape-event domain has no exact Meet")
	}
	latticelaws.LawSuite[Lane]{
		Name:   "escapeevent.Lane",
		Domain: domain,
		Sample: []Lane{domain.Bottom(), domain.Top(), left, right, both},
	}.Run(t)
	if got := domain.Meet(left, right); !domain.Equal(got, both) {
		t.Fatalf("Meet(singleton a, singleton b) = %#v, want union %#v", got.Snapshot(), both.Snapshot())
	}
}
