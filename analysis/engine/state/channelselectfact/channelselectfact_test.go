package channelselectfact

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestSnapshotsBottomTopAndReachableEmpty(t *testing.T) {
	bottom := Bottom().Snapshot()
	if !bottom.Bottom || bottom.Top || len(bottom.Facts) != 0 {
		t.Fatalf("bottom snapshot = %#v, want explicit bottom", bottom)
	}

	top := Top().Snapshot()
	if top.Bottom || !top.Top || len(top.Facts) != 0 {
		t.Fatalf("top snapshot = %#v, want empty top", top)
	}

	reachable := Bottom().Reachable().Snapshot()
	if reachable.Bottom || !reachable.Top || len(reachable.Facts) != 0 {
		t.Fatalf("reachable empty snapshot = %#v, want empty top", reachable)
	}
}

func TestMustSetJoinIntersectionAndWiden(t *testing.T) {
	domain := Domain()
	common := Fact{Select: "select-1", Kind: FactSelect, Result: testStateKey("sym1@1.result")}
	leftOnly := Fact{Select: "select-1", Kind: FactReceive, Case: testStateKey("sym1@1.left"), Index: 0}
	rightOnly := Fact{Select: "select-1", Kind: FactCase, Case: testStateKey("sym1@1.right"), Index: 1}

	left := Top().Add(common).Add(leftOnly)
	right := Top().Add(common).Add(rightOnly)
	joined := domain.Join(left, right)

	if !joined.Has(common) {
		t.Fatalf("common fact was dropped")
	}
	if joined.Has(leftOnly) || joined.Has(rightOnly) {
		t.Fatalf("left/right-only facts survived join: %#v", joined.Snapshot())
	}
	if widened := domain.Widen(left, right); !domain.Equal(widened, joined) {
		t.Fatalf("widen = %#v, want join %#v", widened.Snapshot(), joined.Snapshot())
	}
	if !domain.Equal(domain.Join(domain.Bottom(), left), left) {
		t.Fatalf("bottom should be join identity")
	}
}

func TestDomainExactMeetLaws(t *testing.T) {
	a := Fact{Select: "select-a", Kind: FactSelect, Result: testStateKey("sym1@1.result")}
	b := Fact{Select: "select-b", Kind: FactCase, Case: testStateKey("sym2@1.case")}
	left := Top().Add(a)
	right := Top().Add(b)
	both := left.Add(b)
	domain := Domain()
	if domain.Meet == nil {
		t.Fatal("channel-select domain has no exact Meet")
	}
	latticelaws.LawSuite[Lane]{
		Name:   "channelselectfact.Lane",
		Domain: domain,
		Sample: []Lane{domain.Bottom(), domain.Top(), left, right, both},
	}.Run(t)
	if got := domain.Meet(left, right); !domain.Equal(got, both) {
		t.Fatalf("Meet(singleton a, singleton b) = %#v, want union %#v", got.Snapshot(), both.Snapshot())
	}
}

func TestOrderLawsWhenFactsDrop(t *testing.T) {
	domain := Domain()
	common := Fact{Select: "select-1", Kind: FactSelect, Result: testStateKey("sym1@1.result")}
	leftOnly := Fact{Select: "select-1", Kind: FactReceive, Case: testStateKey("sym1@1.left"), Index: 0}

	left := Top().Add(common).Add(leftOnly)
	joined := domain.Join(left, Top().Add(common))

	if !domain.LessOrEq(left, joined) {
		t.Fatalf("left should be <= joined after dropping must facts")
	}
	if domain.LessOrEq(joined, left) {
		t.Fatalf("joined should not be <= left when left has more must facts")
	}
}

func TestCloneAddIsolation(t *testing.T) {
	common := Fact{Select: "select-1", Kind: FactSelect, Result: testStateKey("sym1@1.result")}
	extra := Fact{Select: "select-1", Kind: FactReceive, Case: testStateKey("sym1@1.case"), Index: 0}

	original := Top().Add(common)
	clone := original.Clone().Add(extra)

	if original.Has(extra) || !clone.Has(extra) {
		t.Fatalf("clone add mutated original or missed clone fact")
	}
	if !original.Has(common) || !clone.Has(common) {
		t.Fatalf("clone did not preserve original facts")
	}
}

func TestSnapshotStableOrdering(t *testing.T) {
	facts := []Fact{
		{Select: "b", Kind: FactSelect, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactCase, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactReceive, Result: testStateKey("b"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("b"), Index: 0},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("a"), Index: 1},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactSelect, Result: testStateKey("z"), Case: testStateKey("z"), Index: 9},
	}
	want := []Fact{
		{Select: "a", Kind: FactSelect, Result: testStateKey("z"), Case: testStateKey("z"), Index: 9},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("a"), Index: 1},
		{Select: "a", Kind: FactReceive, Result: testStateKey("a"), Case: testStateKey("b"), Index: 0},
		{Select: "a", Kind: FactReceive, Result: testStateKey("b"), Case: testStateKey("a"), Index: 0},
		{Select: "a", Kind: FactCase, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
		{Select: "b", Kind: FactSelect, Result: testStateKey("a"), Case: testStateKey("a"), Index: 0},
	}

	lane := Top()
	for _, fact := range facts {
		lane = lane.Add(fact)
	}
	if got := lane.Snapshot().Facts; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot facts = %#v, want %#v", got, want)
	}
}

func TestValidationIgnoresEmptySelectAndKeepsNegativeIndex(t *testing.T) {
	emptySelect := Fact{Select: "", Kind: FactSelect, Result: testStateKey("sym1@1.result")}
	if got := Bottom().Add(emptySelect).Snapshot(); !got.Bottom {
		t.Fatalf("empty select add changed bottom lane: %#v", got)
	}

	negativeIndex := Fact{Select: "select-1", Kind: FactReceive, Case: testStateKey("sym1@1.case"), Index: -1}
	lane := Top().Add(negativeIndex)
	if !lane.Has(negativeIndex) {
		t.Fatalf("negative index fact should be retained")
	}
	if got := lane.Snapshot().Facts; len(got) != 1 || got[0] != negativeIndex {
		t.Fatalf("negative index snapshot = %#v, want retained fact", got)
	}
}

func testStateKey(raw string) pathaddr.StateKey {
	key, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(raw))
	if !ok {
		panic("invalid test state key: " + raw)
	}
	return key
}
