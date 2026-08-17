package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// TestSealedQueryColumnMembershipMatchesDeclaredSolveUniverse fixes the
// publication boundary for real corpus solves. Every query the solve declares
// is covered by the sealed column, while a foreign key is not covered and
// remains a miss.
func TestSealedQueryColumnMembershipMatchesDeclaredSolveUniverse(t *testing.T) {
	for _, fixture := range []string{
		"core/query-zero-row",
		"core/empty-table-map-assign",
	} {
		t.Run(fixture, func(t *testing.T) {
			solve := solveThroughReceipts(t, fixtureLink(t, fixture))
			declared := declaredSolveQueryUniverse(t, solve)
			assertQueryColumnMembership(t, solve.published, solve.queryFamily, declared)
		})
	}
}

// TestSealedQueryColumnMembershipMutationIsVisible is the red-first check on
// the law above. A delta clone drops one covered, unanswered subject from its
// denominator and turns its proven absence into a miss. The original snapshot
// remains unchanged, then a widened clone turns a foreign miss into a proven
// absence. Both directions make a membership swap observable.
func TestSealedQueryColumnMembershipMutationIsVisible(t *testing.T) {
	solve := solveThroughReceipts(t, fixtureLink(t, "core/query-zero-row"))
	declared := declaredSolveQueryUniverse(t, solve)
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.queryFamily)
	if !opens {
		t.Fatal("the fixture publishes no query column")
	}

	var absent identity.ContentID
	for _, summary := range solve.summaries {
		if summary.answer.Rows == 0 {
			absent = summary.subject
			break
		}
	}
	if !absent.Available() {
		t.Fatal("the fixture declares no unanswered value-summary subject")
	}
	if _, status := snapshot.Query(&solve.published, plan, absent); status != snapshot.ReadProvenAbsent {
		t.Fatalf("baseline covered absence = %s, want proven-absent", status)
	}

	droppedMembers := withoutDeclaredMember(t, declared, absent)
	dropped := cloneQueryColumnWithMembers(t, solve.published, solve.queryFamily, declared, droppedMembers, identity.Generation(solve.published.Generation().Next()))
	droppedPlan, opened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&dropped, solve.queryFamily)
	if !opened {
		t.Fatal("the narrowed clone publishes no query column")
	}
	if _, status := snapshot.Query(&dropped, droppedPlan, absent); status != snapshot.ReadMiss {
		t.Fatalf("narrowed covered absence = %s, want miss", status)
	}
	if _, status := snapshot.Query(&solve.published, plan, absent); status != snapshot.ReadProvenAbsent {
		t.Fatalf("the clone changed the original covered absence to %s", status)
	}

	foreign := identity.ContentID{0xD3, 0xE1}
	if _, status := snapshot.Query(&solve.published, plan, foreign); status != snapshot.ReadMiss {
		t.Fatalf("baseline foreign key = %s, want miss", status)
	}
	widenedMembers := append(append([]identity.ContentID(nil), declared...), foreign)
	widened := cloneQueryColumnWithMembers(t, solve.published, solve.queryFamily, declared, widenedMembers, solve.published.Generation().Next().Next())
	widenedPlan, opened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&widened, solve.queryFamily)
	if !opened {
		t.Fatal("the widened clone publishes no query column")
	}
	if _, status := snapshot.Query(&widened, widenedPlan, foreign); status != snapshot.ReadProvenAbsent {
		t.Fatalf("widened foreign key = %s, want proven-absent", status)
	}
}

func declaredSolveQueryUniverse(t testing.TB, solve *receiptSolve) []identity.ContentID {
	t.Helper()
	if solve == nil {
		t.Fatal("the solve is unavailable")
	}
	declared := make([]identity.ContentID, 0, len(solve.summaries)+len(solve.effects))
	seen := make(map[identity.ContentID]struct{}, cap(declared))
	for _, summary := range solve.summaries {
		appendDeclaredQueryMember(t, &declared, seen, summary.subject)
	}
	for _, effect := range solve.effects {
		appendDeclaredQueryMember(t, &declared, seen, effect.subject)
	}
	if len(declared) == 0 {
		t.Fatal("the solve declared no query members")
	}
	return declared
}

func appendDeclaredQueryMember(t testing.TB, declared *[]identity.ContentID, seen map[identity.ContentID]struct{}, member identity.ContentID) {
	t.Helper()
	if !member.Available() {
		t.Fatal("the solve declared an unavailable query member")
	}
	if _, duplicate := seen[member]; duplicate {
		t.Fatalf("the solve declared query member %x twice", member[:4])
	}
	seen[member] = struct{}{}
	*declared = append(*declared, member)
}

func assertQueryColumnMembership(t testing.TB, published snapshot.Snapshot, family identity.ContentID, declared []identity.ContentID) {
	t.Helper()
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, family)
	if !opens {
		t.Fatal("the solve publishes no query column")
	}
	for _, member := range declared {
		if _, status := snapshot.Query(&published, plan, member); status != snapshot.ReadHit && status != snapshot.ReadProvenAbsent {
			t.Fatalf("declared query member %x = %s, want hit or proven-absent", member[:4], status)
		}
	}
	foreign := identity.ContentID{0xD3, 0xE1}
	if _, status := snapshot.Query(&published, plan, foreign); status != snapshot.ReadMiss {
		t.Fatalf("foreign query member = %s, want miss", status)
	}
}

func withoutDeclaredMember(t testing.TB, members []identity.ContentID, removed identity.ContentID) []identity.ContentID {
	t.Helper()
	kept := make([]identity.ContentID, 0, len(members)-1)
	found := false
	for _, member := range members {
		if member == removed {
			found = true
			continue
		}
		kept = append(kept, member)
	}
	if !found {
		t.Fatalf("the declared universe does not contain %x", removed[:4])
	}
	return kept
}

func cloneQueryColumnWithMembers(t testing.TB, base snapshot.Snapshot, family identity.ContentID, declared, members []identity.ContentID, generation identity.Generation) snapshot.Snapshot {
	t.Helper()
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&base, family)
	if !opens {
		t.Fatal("the source snapshot publishes no query column")
	}
	rows := make(map[identity.ContentID]engine.Answer, len(declared))
	for _, member := range declared {
		answer, status := snapshot.Query(&base, plan, member)
		switch status {
		case snapshot.ReadHit:
			rows[member] = answer
		case snapshot.ReadProvenAbsent:
		default:
			t.Fatalf("source query member %x = %s", member[:4], status)
		}
	}
	clone := snapshot.NewDelta(base, generation)
	denominator := identity.ContentID{0xD3, 0xE0, byte(generation)}
	if _, err := snapshot.DeclareQuery(&clone, family, plan.Axis().Slot, snapshot.Content[identity.ContentID, engine.Answer]{
		Rows:        rows,
		Denominator: denominator,
		Members:     members,
	}); err != nil {
		t.Fatalf("clone query column: %v", err)
	}
	sealed, err := clone.Seal()
	if err != nil {
		t.Fatalf("seal query-column clone: %v", err)
	}
	return sealed
}
