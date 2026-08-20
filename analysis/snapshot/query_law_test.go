package snapshot

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The query laws publish one materialized result column under a family
// identity, alongside an ordinary axis column, so the difference between an
// answer a consumer may open and a column that was never published as one is
// observable.
var (
	querySchema      = identity.ContentID{0x1A, 0xA1}
	queryStore       = identity.StoreID(23)
	queryFamily      = identity.ContentID{0x1B, 0xB1}
	queryDenominator = identity.ContentID{0x1C, 0xC1}
	queryColumnID    = identity.ContentID{0x1D, 0xD1}
	queryUnknown     = identity.ContentID{0x1E, 0xE1}
	queryUnanswered  = identity.ContentID{0x1F, 0xF1}

	queryColumnAxis = Axis[string, int]{SchemaID: querySchema, Slot: 1}
)

// querySnapshot seals a result column for queryFamily at slot 0 and an
// ordinary published column at slot 1, and registers a family that no result
// column answers.
func querySnapshot(t testing.TB, generation identity.Generation) (Snapshot, QueryPlan[string, int]) {
	t.Helper()
	builder := NewBuilder(querySchema, queryStore, generation)
	plan, err := DeclareQuery(&builder, queryFamily, 0, Content[string, int]{
		Rows:        map[string]int{"present": 11},
		Denominator: queryDenominator,
		Members:     []string{"present", "absent"},
	})
	if err != nil {
		t.Fatalf("declare query: %v", err)
	}
	if err := PutColumn(&builder, queryColumnAxis, Content[string, int]{Rows: map[string]int{"present": 22}}); err != nil {
		t.Fatalf("put column: %v", err)
	}
	if err := builder.Publish(queryColumnID, queryColumnAxis.Slot); err != nil {
		t.Fatalf("publish column: %v", err)
	}
	if err := builder.RegisterQuery(queryUnanswered); err != nil {
		t.Fatalf("register unanswered family: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal query snapshot: %v", err)
	}
	return sealed, plan
}

// TestQueryAnswersItsPublishedFamily fixes what a published answer is. A
// query family is answered by a result column the snapshot addresses under
// the family identity and registers as answerable, and reading it reports the
// same four outcomes a column read reports: a materialized answer, a
// materialized absence, ignorance, and a rejection.
func TestQueryAnswersItsPublishedFamily(t *testing.T) {
	sealed, declared := querySnapshot(t, identity.Generation(1))
	opened, published := OpenQuery[string, int](&sealed, queryFamily)
	if !published {
		t.Fatal("a declared family does not open")
	}
	if opened != declared || !opened.Available() {
		t.Fatalf("opened plan = %+v, want the declared plan %+v", opened, declared)
	}
	cases := []struct {
		name  string
		key   string
		value int
		want  ReadStatus
	}{
		{name: "materialized answer", key: "present", value: 11, want: ReadHit},
		{name: "materialized absence", key: "absent", want: ReadProvenAbsent},
		{name: "unanswered key", key: "unknown", want: ReadMiss},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, status := Query(&sealed, opened, testCase.key)
			if status != testCase.want || value != testCase.value {
				t.Fatalf("query = (%d, %v), want (%d, %v)", value, status, testCase.value, testCase.want)
			}
		})
	}
	if value, status := Read(&sealed, opened.Axis(), "present"); value != 11 || status != ReadHit {
		t.Fatalf("result column read = (%d, %v), want (11, hit)", value, status)
	}
	if !sealed.Queries().Published(queryFamily) {
		t.Fatal("a declared family is not registered as answerable")
	}
}

// TestQueryFailsClosed fixes every rejection. A plan a snapshot did not issue
// answers nothing, and a family that is not registered, not addressed, or
// answered by a column of other types opens nothing: an answer is only what a
// snapshot published as one.
func TestQueryFailsClosed(t *testing.T) {
	sealed, plan := querySnapshot(t, identity.Generation(1))
	zero := Snapshot{}

	t.Run("zero plan", func(t *testing.T) {
		value, status := Query(&sealed, QueryPlan[string, int]{}, "present")
		assertInvalid(t, value, status)
		if (QueryPlan[string, int]{}).Available() {
			t.Fatal("the zero plan names a result column")
		}
	})
	t.Run("plan of another schema", func(t *testing.T) {
		foreign := QueryPlan[string, int]{SchemaID: fixtureSchema, Slot: plan.Slot}
		value, status := Query(&sealed, foreign, "present")
		assertInvalid(t, value, status)
	})
	t.Run("plan of another answer type", func(t *testing.T) {
		crossed := QueryPlan[string, record]{SchemaID: querySchema, Slot: plan.Slot}
		value, status := Query(&sealed, crossed, "present")
		if status != ReadInvalid || value != (record{}) {
			t.Fatalf("query = (%+v, %v), want (zero, invalid)", value, status)
		}
	})
	t.Run("plan against another snapshot", func(t *testing.T) {
		value, status := Query(&zero, plan, "present")
		assertInvalid(t, value, status)
		if value, status := Query[string, int](nil, plan, "present"); status != ReadInvalid || value != 0 {
			t.Fatalf("nil snapshot query = (%d, %v), want (0, invalid)", value, status)
		}
	})
	t.Run("unknown family", func(t *testing.T) {
		if _, published := OpenQuery[string, int](&sealed, queryUnknown); published {
			t.Fatal("an unpublished family opens")
		}
		if _, published := OpenQuery[string, int](&sealed, identity.ContentID{}); published {
			t.Fatal("an unavailable family opens")
		}
	})
	t.Run("addressed column that is not a family", func(t *testing.T) {
		if _, published := OpenQuery[string, int](&sealed, queryColumnID); published {
			t.Fatal("an ordinary column opens as a query family")
		}
	})
	t.Run("registered family without a result column", func(t *testing.T) {
		if _, published := OpenQuery[string, int](&sealed, queryUnanswered); published {
			t.Fatal("a family no column answers opens")
		}
	})
	t.Run("family of another key or answer type", func(t *testing.T) {
		if _, published := OpenQuery[int, int](&sealed, queryFamily); published {
			t.Fatal("a family opens under another key type")
		}
		if _, published := OpenQuery[string, record](&sealed, queryFamily); published {
			t.Fatal("a family opens under another answer type")
		}
	})
	t.Run("unpublished snapshot", func(t *testing.T) {
		if _, published := OpenQuery[string, int](&zero, queryFamily); published {
			t.Fatal("an unpublished snapshot opens a family")
		}
		if _, published := OpenQuery[string, int](nil, queryFamily); published {
			t.Fatal("a nil snapshot opens a family")
		}
	})
	t.Run("family declared without an identity", func(t *testing.T) {
		builder := NewBuilder(querySchema, queryStore, identity.Generation(1))
		_, err := DeclareQuery(&builder, identity.ContentID{}, 0, Content[string, int]{})
		if !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("declare error = %v, want %v", err, ErrUnavailableIdentity)
		}
		if _, err := DeclareQuery[string, int](nil, queryFamily, 0, Content[string, int]{}); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("nil builder declare error = %v, want %v", err, ErrUnavailableIdentity)
		}
	})
	t.Run("family declared twice", func(t *testing.T) {
		builder := NewBuilder(querySchema, queryStore, identity.Generation(1))
		if _, err := DeclareQuery(&builder, queryFamily, 0, Content[string, int]{}); err != nil {
			t.Fatalf("declare query: %v", err)
		}
		if _, err := DeclareQuery(&builder, queryFamily, 1, Content[string, int]{}); !errors.Is(err, ErrDuplicatePublication) {
			t.Fatalf("second declaration error = %v, want %v", err, ErrDuplicatePublication)
		}
	})
}

// TestQueryReadsAllocateNothing is the cost law of a published answer. Every
// query outcome and every plan opening runs in zero allocations, so reading a
// registered answer is never priced above recomputing one locally.
func TestQueryReadsAllocateNothing(t *testing.T) {
	sealed, plan := querySnapshot(t, identity.Generation(1))
	rejected := QueryPlan[string, int]{SchemaID: fixtureSchema, Slot: plan.Slot}
	cases := []struct {
		name string
		want ReadStatus
		read func()
	}{
		{name: "hit", want: ReadHit, read: func() { sinkInt, sinkStatus = Query(&sealed, plan, "present") }},
		{name: "proven absence", want: ReadProvenAbsent, read: func() { sinkInt, sinkStatus = Query(&sealed, plan, "absent") }},
		{name: "miss", want: ReadMiss, read: func() { sinkInt, sinkStatus = Query(&sealed, plan, "unknown") }},
		{name: "invalid", want: ReadInvalid, read: func() { sinkInt, sinkStatus = Query(&sealed, rejected, "present") }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(1000, testCase.read); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
			if sinkStatus != testCase.want {
				t.Fatalf("status = %v, want %v", sinkStatus, testCase.want)
			}
		})
	}
	t.Run("open", func(t *testing.T) {
		open := func() { sinkPlan, sinkOpened = OpenQuery[string, int](&sealed, queryFamily) }
		if allocations := testing.AllocsPerRun(1000, open); allocations != 0 {
			t.Fatalf("allocations = %v, want 0", allocations)
		}
		if !sinkOpened || sinkPlan != plan {
			t.Fatalf("opened = (%+v, %t), want the declared plan", sinkPlan, sinkOpened)
		}
	})
}

// TestQueryRowsArePublishedLikeAnyColumn fixes that a materialized answer is
// stored as a column and nothing else. A derived publication sets and
// withdraws answers with the ordinary row edits, at the ordinary cost, the
// base keeps answering what it answered, and a withdrawn answer the family's
// denominator covers becomes a materialized absence.
func TestQueryRowsArePublishedLikeAnyColumn(t *testing.T) {
	base, plan := querySnapshot(t, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := SetRow(&delta, plan.Axis(), "materialized", 5); err != nil {
		t.Fatalf("set answer: %v", err)
	}
	if err := RemoveRow(&delta, plan.Axis(), "present"); err != nil {
		t.Fatalf("withdraw answer: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}
	opened, published := OpenQuery[string, int](&sealed, queryFamily)
	if !published {
		t.Fatal("a derived publication lost its query family")
	}
	if value, status := Query(&sealed, opened, "materialized"); value != 5 || status != ReadHit {
		t.Fatalf("materialized answer = (%d, %v), want (5, hit)", value, status)
	}
	if _, status := Query(&sealed, opened, "present"); status != ReadProvenAbsent {
		t.Fatalf("withdrawn answer = %v, want proven-absent", status)
	}
	if value, status := Query(&base, plan, "present"); value != 11 || status != ReadHit {
		t.Fatalf("base answer after a derived withdrawal = (%d, %v), want (11, hit)", value, status)
	}
	if _, status := Query(&base, plan, "materialized"); status != ReadMiss {
		t.Fatalf("base answer for a derived row = %v, want miss", status)
	}
	if !sealed.Denominators().Proves(queryDenominator, opened.Slot) {
		t.Fatal("a derived publication lost the family's denominator")
	}
}

var (
	queryMembershipSchema = identity.ContentID{0xD1, 0x01}
	queryMembershipFamily = identity.ContentID{0xD1, 0x02}
	queryMembershipBase   = identity.ContentID{0xD1, 0x03}
	queryMembershipAxis   = Axis[identity.ContentID, int]{SchemaID: queryMembershipSchema, Slot: 0}
)

func queryMembershipID(value byte) identity.ContentID {
	return identity.ContentID{0xD1, value}
}

func queryMembershipSnapshot(t testing.TB, generation identity.Generation, members []identity.ContentID, rows map[identity.ContentID]int, denominator identity.ContentID) (Snapshot, QueryPlan[identity.ContentID, int]) {
	t.Helper()
	builder := NewBuilder(queryMembershipSchema, identity.StoreID(31), generation)
	plan, err := DeclareQuery(&builder, queryMembershipFamily, queryMembershipAxis.Slot, Content[identity.ContentID, int]{
		Rows: rows, Denominator: denominator, Members: members,
	})
	if err != nil {
		t.Fatalf("declare query: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal query snapshot: %v", err)
	}
	return sealed, plan
}

func queryMembershipRows(t testing.TB, base Snapshot, plan QueryPlan[identity.ContentID, int], members []identity.ContentID) map[identity.ContentID]int {
	t.Helper()
	rows := make(map[identity.ContentID]int, len(members))
	for _, member := range members {
		value, status := Query(&base, plan, member)
		switch status {
		case ReadHit:
			rows[member] = value
		case ReadProvenAbsent, ReadMiss:
		default:
			t.Fatalf("source query member %x = %s", member[:4], status)
		}
	}
	return rows
}

func reissueQueryMembership(t testing.TB, base Snapshot, plan QueryPlan[identity.ContentID, int], members []identity.ContentID, generation identity.Generation) Snapshot {
	t.Helper()
	delta := NewDelta(base, generation)
	if _, err := DeclareQuery(&delta, queryMembershipFamily, plan.Axis().Slot, Content[identity.ContentID, int]{
		Rows:        queryMembershipRows(t, base, plan, members),
		Denominator: identity.ContentID{0xD2, byte(generation)},
		Members:     members,
	}); err != nil {
		t.Fatalf("reissue query membership: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal reissued query membership: %v", err)
	}
	return sealed
}

// TestQueryMembershipIsClosedOverItsDeclaredUniverse fixes the three-way
// owner contract: stored subjects hit, covered missing subjects are proven
// absent, and subjects outside the declared universe remain unknown.
func TestQueryMembershipIsClosedOverItsDeclaredUniverse(t *testing.T) {
	present := queryMembershipID(0x11)
	absent := queryMembershipID(0x12)
	foreign := queryMembershipID(0x13)
	sealed, plan := queryMembershipSnapshot(t, identity.Generation(1), []identity.ContentID{present, absent}, map[identity.ContentID]int{present: 17}, queryMembershipBase)
	opened, opens := OpenQuery[identity.ContentID, int](&sealed, queryMembershipFamily)
	if !opens || opened != plan {
		t.Fatal("the published query family did not reopen its own plan")
	}
	if value, status := Query(&sealed, opened, present); value != 17 || status != ReadHit {
		t.Fatalf("stored subject = (%d, %s), want (17, hit)", value, status)
	}
	if _, status := Query(&sealed, opened, absent); status != ReadProvenAbsent {
		t.Fatalf("declared zero-row subject = %s, want proven-absent", status)
	}
	if _, status := Query(&sealed, opened, foreign); status != ReadMiss {
		t.Fatalf("foreign subject = %s, want miss", status)
	}
}

// TestQueryMembershipMutationChangesCoverageWithoutMutatingTheBase proves
// that a derived publication may narrow or widen its closed universe without
// mutating the base snapshot.
func TestQueryMembershipMutationChangesCoverageWithoutMutatingTheBase(t *testing.T) {
	present := queryMembershipID(0x21)
	absent := queryMembershipID(0x22)
	foreign := queryMembershipID(0x23)
	base, plan := queryMembershipSnapshot(t, identity.Generation(1), []identity.ContentID{present, absent}, map[identity.ContentID]int{present: 29}, queryMembershipBase)

	narrowed := reissueQueryMembership(t, base, plan, []identity.ContentID{present}, identity.Generation(2))
	narrowedPlan, opens := OpenQuery[identity.ContentID, int](&narrowed, queryMembershipFamily)
	if !opens {
		t.Fatal("narrowed publication did not reopen its query family")
	}
	if _, status := Query(&narrowed, narrowedPlan, absent); status != ReadMiss {
		t.Fatalf("narrowed absent subject = %s, want miss", status)
	}
	if _, status := Query(&base, plan, absent); status != ReadProvenAbsent {
		t.Fatalf("narrowing changed the base covered absence to %s", status)
	}

	widened := reissueQueryMembership(t, base, plan, []identity.ContentID{present, absent, foreign}, identity.Generation(3))
	widenedPlan, opens := OpenQuery[identity.ContentID, int](&widened, queryMembershipFamily)
	if !opens {
		t.Fatal("widened publication did not reopen its query family")
	}
	if _, status := Query(&widened, widenedPlan, foreign); status != ReadProvenAbsent {
		t.Fatalf("widened foreign subject = %s, want proven-absent", status)
	}
	if _, status := Query(&base, plan, foreign); status != ReadMiss {
		t.Fatalf("widening changed the base foreign miss to %s", status)
	}
}

// BenchmarkQuery reports the cost of reading a published answer, which the
// law above fixes at zero allocations on every outcome.
func BenchmarkQuery(b *testing.B) {
	sealed, plan := querySnapshot(b, identity.Generation(1))
	rejected := QueryPlan[string, int]{SchemaID: fixtureSchema, Slot: plan.Slot}
	b.Run("hit", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Query(&sealed, plan, "present")
		}
	})
	b.Run("miss", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Query(&sealed, plan, "unknown")
		}
	})
	b.Run("proven-absent", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Query(&sealed, plan, "absent")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Query(&sealed, rejected, "present")
		}
	})
	b.Run("open", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkPlan, sinkOpened = OpenQuery[string, int](&sealed, queryFamily)
		}
	})
}
