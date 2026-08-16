package value_test

import (
	"testing"

	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
)

// TestAllocationMembershipIsExactAndCoordinateFenced records the deliberately
// narrow Phase3D Value surface. Recent/Summary membership is meaningful only
// for the canonical AllocationResult and its exact Value owner; it is not an
// alias or uniqueness proof.
func TestAllocationMembershipIsExactAndCoordinateFenced(t *testing.T) {
	fixture := directAllocationSubjectFixtureFor(t, "membership")
	// Re-seal the identical input independently. Scalar schema identities must
	// match while Value/Pack owners remain distinct live authorities.
	foreign := directAllocationSubjectFixtureFor(t, "membership")
	if fixture.heaps.ContentID() != foreign.heaps.ContentID() {
		t.Fatal("equal-content foreign Heap fixture")
	}
	direct, directOK := fixture.values.DirectAllocationSubjectFor(fixture.packs, fixture.source, fixture.allocation)
	recent, recentOK := fixture.allocation.Fresh()
	summary, summaryOK := fixture.allocation.Age(recent)
	other, otherOK := fixture.other.Fresh()
	mixed, mixedOK := fixture.values.Join(recent, other)
	recentAndSummary, recentAndSummaryOK := fixture.values.Join(recent, summary)
	foreignRecent, foreignRecentOK := foreign.allocation.Fresh()
	if !directOK || !direct.Valid() || !recentOK || !summaryOK || !otherOK || !mixedOK || !recentAndSummaryOK || !foreignRecentOK {
		t.Fatal("allocation membership fixture")
	}

	cases := []struct {
		name string
		fact valuedomain.Value
		want valuedomain.AllocationMembership
	}{
		{name: "recent", fact: recent, want: valuedomain.MembershipRecent},
		{name: "summary", fact: summary, want: valuedomain.MembershipSummary},
		{name: "top", fact: fixture.values.Top(), want: valuedomain.MembershipMixedOrUnknown},
		{name: "bottom", fact: fixture.values.Bottom(), want: valuedomain.MembershipMixedOrUnknown},
		{name: "mixed", fact: mixed, want: valuedomain.MembershipMixedOrUnknown},
		{name: "same-key-recent-summary", fact: recentAndSummary, want: valuedomain.MembershipMixedOrUnknown},
		{name: "different-allocation", fact: other, want: valuedomain.MembershipMixedOrUnknown},
	}
	for _, test := range cases {
		got, classified := fixture.allocation.ClassifyMembership(test.fact)
		if !classified || got != test.want {
			t.Fatalf("membership %s got=%d classified=%t want=%d", test.name, got, classified, test.want)
		}
	}
	if got, classified := fixture.allocation.ClassifyMembership(foreign.values.Top()); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("foreign Value owner entered allocation membership classification")
	}
	if got, classified := fixture.allocation.ClassifyMembership(foreignRecent); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("equal-content foreign Value/allocation result entered local classification")
	}
	if got, classified := foreign.allocation.ClassifyMembership(recent); classified || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("local Value fact entered equal-content foreign allocation result")
	}

	matchedIndex := -1
	for index := 0; index < fixture.values.CoordinateCount(); index++ {
		if got, matched := direct.ClassifySummaryCell(index, recent); matched {
			if got != valuedomain.MembershipRecent {
				t.Fatalf("direct coordinate class=%d", got)
			}
			matchedIndex = index
			break
		}
	}
	if matchedIndex < 0 || fixture.values.CoordinateCount() < 2 {
		t.Fatal("direct coordinate fixture")
	}
	mismatchIndex := 0
	if mismatchIndex == matchedIndex {
		mismatchIndex = 1
	}
	if got, matched := direct.ClassifySummaryCell(mismatchIndex, recent); matched || got != valuedomain.AllocationMembershipInvalid {
		t.Fatal("direct receipt classified a different Value coordinate")
	}
}
