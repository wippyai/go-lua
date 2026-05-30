package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestDomain_JoinAssociative_WidthRecordsWithNonRecord pins the join-associativity
// law on the one shape the product's general LawSuite sample never co-locates: two
// records of DIFFERENT width joined alongside a non-record. A join is the least
// upper bound, so it must be associative — (a⊔b)⊔c = a⊔(b⊔c) — independent of the
// order the abstract interpreter merges incoming values at a CFG confluence or a
// parameter contract cell.
//
// The hazard is the shape-axis record width-join: joining {id} with {id,name} must
// yield a sound upper bound of both (the wider record with its non-shared field
// made optional, {id, name?}), not the narrower {id}. If width-join instead drops
// the non-shared field while record-into-union keeps it, the two disagree and the
// law breaks — a foundational defect, since envDomain, ContractDomain and
// PointState all lift this domain.
func TestDomain_JoinAssociative_WidthRecordsWithNonRecord(t *testing.T) {
	r1 := FromType(typ.NewRecord().Field("id", typ.String).Build())
	r2 := FromType(typ.NewRecord().Field("id", typ.String).Field("name", typ.String).Build())
	other := FromType(typ.NewOptional(typ.Integer))

	left := Join(Join(r1, r2), other)
	right := Join(r1, Join(r2, other))

	if !Equal(left, right) {
		t.Fatalf("Join is not associative on (record{id}, record{id,name}, optional integer):\n  (r1 join r2) join other = %s\n  r1 join (r2 join other) = %s",
			left.ProjectValue(), right.ProjectValue())
	}

	// Both groupings must over-approximate every operand: a true LUB covers each.
	for _, v := range []AbstractValue{r1, r2, other} {
		if !Domain.LessOrEq(v, left) {
			t.Fatalf("left grouping is not an upper bound of %s: %s", v.ProjectValue(), left.ProjectValue())
		}
		if !Domain.LessOrEq(v, right) {
			t.Fatalf("right grouping is not an upper bound of %s: %s", v.ProjectValue(), right.ProjectValue())
		}
	}
}
