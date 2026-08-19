package rows

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

const lawFamily = keyspace.FamilyTypePrimitive

func sealedPool(t *testing.T, values ...int) (Pool[int], Span) {
	t.Helper()
	var builder PoolBuilder[int]
	span, ok := builder.Append(values)
	if !ok {
		t.Fatalf("Append(%v) refused", values)
	}
	return builder.Seal(), span
}

// TestSealedStorageIgnoresLaterCallerMutation proves every constructor takes a
// copy: a caller that keeps its input slice has no write path into the seal.
func TestSealedStorageIgnoresLaterCallerMutation(t *testing.T) {
	source := []int{1, 2, 3}

	pool := NewPool(source)
	list := NewRows(source)
	table, ok := NewTable(lawFamily, source)
	if !ok {
		t.Fatal("NewTable refused a well-formed table")
	}
	var builder PoolBuilder[int]
	span, ok := builder.Append(source)
	if !ok {
		t.Fatal("Append refused a well-formed column")
	}
	appended := builder.Seal()

	for index := range source {
		source[index] = 99
	}

	for _, probe := range []struct {
		name  string
		value func(int) (int, bool)
	}{
		{name: "NewPool", value: func(index int) (int, bool) { return pool.At(Span{start: 0, end: 3}, index) }},
		{name: "NewRows", value: list.At},
		{name: "NewTable", value: table.At},
		{name: "PoolBuilder", value: func(index int) (int, bool) { return appended.At(span, index) }},
	} {
		for index, want := range []int{1, 2, 3} {
			got, ok := probe.value(index)
			if !ok || got != want {
				t.Fatalf("%s sealed value at %d = %d/%v, want %d", probe.name, index, got, ok, want)
			}
		}
	}
}

// TestSealedValueCopiesShareNoWritePath proves a copied sealed value observes
// the same rows, so passing a Table or Pool by value is a total handoff.
func TestSealedValueCopiesShareNoWritePath(t *testing.T) {
	table, ok := NewTable(lawFamily, []int{7, 8})
	if !ok {
		t.Fatal("NewTable refused a well-formed table")
	}
	copied := table
	for index := range 2 {
		original, firstOK := table.At(index)
		duplicate, secondOK := copied.At(index)
		if !firstOK || !secondOK || original != duplicate {
			t.Fatalf("copied table at %d = %d/%v, want %d/%v", index, duplicate, secondOK, original, firstOK)
		}
	}
	if copied.Family() != table.Family() || copied.Count() != table.Count() {
		t.Fatal("copied table lost its family or denominator")
	}
}

// TestPoolReadsAreTotalOnMalformedSpans proves every span read fails closed
// rather than panicking, clamping, or reaching outside its own pool.
func TestPoolReadsAreTotalOnMalformedSpans(t *testing.T) {
	pool, span := sealedPool(t, 4, 5, 6)
	short, _ := sealedPool(t, 1)

	for _, test := range []struct {
		name string
		pool Pool[int]
		span Span
	}{
		{name: "inverted", pool: pool, span: Span{start: 2, end: 1}},
		{name: "past end", pool: pool, span: Span{start: 1, end: 9}},
		{name: "wholly outside", pool: pool, span: Span{start: 7, end: 9}},
		{name: "foreign pool", pool: short, span: span},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.pool.Count(test.span); got != 0 {
				t.Fatalf("Count = %d, want 0", got)
			}
			for _, index := range []int{-1, 0, 1} {
				if value, ok := test.pool.At(test.span, index); ok || value != 0 {
					t.Fatalf("At(%d) = %d/%v, want fail closed", index, value, ok)
				}
			}
			for range test.pool.All(test.span) {
				t.Fatal("All yielded an element outside the pool")
			}
		})
	}

	if value, ok := pool.At(span, -1); ok || value != 0 {
		t.Fatalf("At(-1) = %d/%v, want fail closed", value, ok)
	}
	if value, ok := pool.At(span, span.Len()); ok || value != 0 {
		t.Fatalf("At(len) = %d/%v, want fail closed", value, ok)
	}
}

// TestTableReadsAreTotalOnForeignTerms proves a table refuses every term it
// does not name instead of indexing by a bare ordinal.
func TestTableReadsAreTotalOnForeignTerms(t *testing.T) {
	table, ok := NewTable(lawFamily, []int{11, 12})
	if !ok {
		t.Fatal("NewTable refused a well-formed table")
	}
	for _, test := range []struct {
		name string
		term keyspace.Term
	}{
		{name: "zero term", term: 0},
		{name: "foreign family", term: keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)},
		{name: "past count", term: keyspace.MakeTerm(lawFamily, 3)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if row, ok := table.Row(test.term); ok || row != 0 {
				t.Fatalf("Row(%d) = %d/%v, want fail closed", test.term, row, ok)
			}
		})
	}
	for _, index := range []int{-1, 2} {
		if row, ok := table.At(index); ok || row != 0 {
			t.Fatalf("At(%d) = %d/%v, want fail closed", index, row, ok)
		}
		if term, ok := table.Term(index); ok || term != 0 {
			t.Fatalf("Term(%d) = %d/%v, want fail closed", index, term, ok)
		}
	}
}

// TestTableTermAndIndexReadsAgree proves the ordinal contract: the term of an
// index names exactly the row that index holds, in both iteration orders.
func TestTableTermAndIndexReadsAgree(t *testing.T) {
	values := []int{21, 22, 23}
	table, ok := NewTable(lawFamily, values)
	if !ok {
		t.Fatal("NewTable refused a well-formed table")
	}
	seen := 0
	for term, row := range table.Terms() {
		index := int(keyspace.TermOrdinal(term)) - 1
		byIndex, indexOK := table.At(index)
		byTerm, termOK := table.Row(term)
		reverse, reverseOK := table.Term(index)
		if !indexOK || !termOK || !reverseOK || byIndex != row || byTerm != row || reverse != term {
			t.Fatalf("term %d disagreed: row=%d index=%d/%v term=%d/%v reverse=%d/%v",
				term, row, byIndex, indexOK, byTerm, termOK, reverse, reverseOK)
		}
		seen++
	}
	if seen != len(values) {
		t.Fatalf("Terms visited %d rows, want %d", seen, len(values))
	}
}

// TestZeroSealedValuesAreEmptyNotBroken proves the zero Pool, Rows, Table and
// Span are usable sealed empties rather than states a reader must guard.
func TestZeroSealedValuesAreEmptyNotBroken(t *testing.T) {
	var pool Pool[int]
	var list Rows[int]
	var table Table[int]
	var span Span

	if pool.Len() != 0 || pool.Count(span) != 0 || span.Len() != 0 || !span.Empty() {
		t.Fatal("zero pool or span was not an empty sealed value")
	}
	if value, ok := pool.At(span, 0); ok || value != 0 {
		t.Fatalf("zero pool At = %d/%v, want fail closed", value, ok)
	}
	if list.Count() != 0 || table.Count() != 0 || table.Family() != keyspace.FamilyInvalid {
		t.Fatal("zero rows or table carried a denominator or family")
	}
	if row, ok := table.Row(keyspace.MakeTerm(lawFamily, 1)); ok || row != 0 {
		t.Fatalf("zero table Row = %d/%v, want fail closed", row, ok)
	}
	for range pool.All(span) {
		t.Fatal("zero pool yielded an element")
	}
	for range table.Terms() {
		t.Fatal("zero table yielded a row")
	}
}

// TestBuildersAreOneShotSealers proves a sealed builder issues no further
// span or row, so no span can name storage a reader has already sealed.
func TestBuildersAreOneShotSealers(t *testing.T) {
	var pool PoolBuilder[int]
	if _, ok := pool.Append([]int{1, 2}); !ok {
		t.Fatal("Append refused a well-formed column")
	}
	if pool.Len() != 2 {
		t.Fatalf("PoolBuilder.Len = %d, want 2", pool.Len())
	}
	sealed := pool.Seal()
	if _, ok := pool.Append([]int{3}); ok {
		t.Fatal("sealed PoolBuilder issued another span")
	}
	if sealed.Len() != 2 {
		t.Fatalf("sealed pool width = %d, want 2", sealed.Len())
	}

	var list RowsBuilder[int]
	index, ok := list.Append(5)
	if !ok || index != 0 {
		t.Fatalf("RowsBuilder.Append returned %d/%v, want the first dense index", index, ok)
	}
	if row, ok := list.At(0); !ok || row != 5 {
		t.Fatalf("RowsBuilder.At(0) = %d/%v, want the placed row", row, ok)
	}
	if _, ok := list.At(1); ok {
		t.Fatal("RowsBuilder.At read past the rows it holds")
	}
	sealedList := list.Seal()
	if _, ok := list.Append(6); ok {
		t.Fatal("sealed RowsBuilder accepted another row")
	}
	if sealedList.Count() != 1 {
		t.Fatalf("sealed rows count = %d, want 1", sealedList.Count())
	}

	table := NewTableBuilder[int](lawFamily)
	term, ok := table.Append(9)
	if !ok || term != keyspace.MakeTerm(lawFamily, 1) {
		t.Fatalf("Append returned %d/%v, want the first canonical term", term, ok)
	}
	sealedTable := table.Seal()
	if _, ok := table.Append(10); ok {
		t.Fatal("sealed TableBuilder accepted another row")
	}
	if sealedTable.Count() != 1 {
		t.Fatalf("sealed table count = %d, want 1", sealedTable.Count())
	}
}

// TestBuildersRefuseFamiliesOutsideTheClosedInventory proves an unnamed family
// yields no table rather than a table numbering an invalid keyspace.
func TestBuildersRefuseFamiliesOutsideTheClosedInventory(t *testing.T) {
	for _, family := range []keyspace.Family{keyspace.FamilyInvalid, keyspace.FamilyCount} {
		if _, ok := NewTable(family, []int{1}); ok {
			t.Fatalf("NewTable admitted family %d", family)
		}
		builder := NewTableBuilder[int](family)
		if _, ok := builder.Append(1); ok {
			t.Fatalf("NewTableBuilder(%d) accepted a row", family)
		}
		if builder.Seal().Count() != 0 {
			t.Fatalf("NewTableBuilder(%d) sealed a nonempty table", family)
		}
	}
}

// TestTableRefusesUnaddressableRowCounts proves the canonical ordinal ceiling
// is the table's admission bound, not a truncation performed at read time.
func TestTableRefusesUnaddressableRowCounts(t *testing.T) {
	if _, ok := NewTable(lawFamily, make([]struct{}, keyspace.MaxTermOrdinal)); !ok {
		t.Fatal("NewTable refused the largest addressable table")
	}
	if _, ok := NewTable(lawFamily, make([]struct{}, int(keyspace.MaxTermOrdinal)+1)); ok {
		t.Fatal("NewTable admitted a row count no canonical ordinal can name")
	}
}

// TestIterationStopsWhenTheReaderStops proves both sequences honour an early
// exit, so a fail-closed reader never walks storage it has rejected.
func TestIterationStopsWhenTheReaderStops(t *testing.T) {
	pool, span := sealedPool(t, 1, 2, 3)
	visited := 0
	for range pool.All(span) {
		visited++
		break
	}
	if visited != 1 {
		t.Fatalf("Pool.All visited %d elements after break, want 1", visited)
	}

	table, ok := NewTable(lawFamily, []int{1, 2, 3})
	if !ok {
		t.Fatal("NewTable refused a well-formed table")
	}
	visited = 0
	for range table.Terms() {
		visited++
		break
	}
	if visited != 1 {
		t.Fatalf("Table.Terms visited %d rows after break, want 1", visited)
	}
	visited = 0
	for range table.All() {
		visited++
		break
	}
	if visited != 1 {
		t.Fatalf("Rows.All visited %d rows after break, want 1", visited)
	}
}
