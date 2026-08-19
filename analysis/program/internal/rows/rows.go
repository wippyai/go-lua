// Package rows is the shared sealed row-representation vocabulary for the
// canonical Program owners. It carries no domain meaning: a Pool is a dense
// column, a Table is a dense row list addressed by canonical family ordinal,
// and a Span selects one contiguous window of a Pool.
//
// The vocabulary exists so a family owner states its representation once
// instead of restating pool ranges, ordinal bounds, and fail-closed accessors
// per relation. Two disciplines are structural rather than documented:
//
//   - Seal immutability. Row storage is unexported and every constructor
//     copies. A sealed Pool or Table cannot be written through, and a caller
//     that keeps its input slice cannot reach the sealed copy. A Span can only
//     be minted by the PoolBuilder that placed the values it selects.
//   - Bounds discipline. Every read is total. A malformed ordinal, a foreign
//     family, an inverted Span, or a Span that does not lie inside its Pool
//     yields the zero value and false; no accessor panics and none clamps.
//
// The zero Pool, Span, Table, and Rows are valid sealed empty values.
package rows

import (
	"iter"
	"math"
)

// Span selects a contiguous window inside one sealed Pool. Its bounds are
// unexported so a Span can only originate from the builder that appended the
// values, never from arithmetic performed by a reader.
type Span struct{ start, end uint32 }

// Len is the window's width. An end that precedes its start is an empty
// window rather than a wrapped one.
func (span Span) Len() int {
	if span.end < span.start {
		return 0
	}
	return int(span.end - span.start)
}

// Empty reports whether the window selects no element.
func (span Span) Empty() bool { return span.Len() == 0 }

// Pool is a sealed dense column shared by the rows of one table.
type Pool[Element any] struct{ values []Element }

// NewPool seals a copy of values. The caller keeps no write path into it.
func NewPool[Element any](values []Element) Pool[Element] {
	if len(values) == 0 {
		return Pool[Element]{}
	}
	return Pool[Element]{values: append(make([]Element, 0, len(values)), values...)}
}

// Len is the whole column's width, which is also the sealed denominator of
// every Span the pool's builder issued.
func (pool Pool[Element]) Len() int { return len(pool.values) }

// Count is the width of span within this pool. A span that does not lie
// inside the pool counts zero rather than reporting a width it cannot serve.
func (pool Pool[Element]) Count(span Span) int {
	if !pool.holds(span) {
		return 0
	}
	return span.Len()
}

// At returns one element of span's window. It is the only indexed read of a
// pool: callers never reconstruct a start plus an offset.
func (pool Pool[Element]) At(span Span, index int) (value Element, ok bool) {
	if !pool.holds(span) || index < 0 || index >= span.Len() {
		return value, false
	}
	return pool.values[int(span.start)+index], true
}

// All iterates span's window in place. The sequence yields values, so no
// window slice escapes the seal.
func (pool Pool[Element]) All(span Span) iter.Seq2[int, Element] {
	return func(yield func(int, Element) bool) {
		if !pool.holds(span) {
			return
		}
		for index, value := range pool.values[span.start:span.end] {
			if !yield(index, value) {
				return
			}
		}
	}
}

func (pool Pool[Element]) holds(span Span) bool {
	return span.end >= span.start && uint64(span.end) <= uint64(len(pool.values))
}

// PoolBuilder accumulates one column and issues the spans that select it. It
// is the only source of a Span. Seal is one-shot: a sealed builder appends no
// more, so a span cannot be issued against a pool that has already been read.
type PoolBuilder[Element any] struct {
	values []Element
	sealed bool
}

// Append places values contiguously and returns the span selecting them. It
// fails closed once the column would exceed the Span bound or after Seal.
func (builder *PoolBuilder[Element]) Append(values []Element) (Span, bool) {
	if builder == nil || builder.sealed {
		return Span{}, false
	}
	start := len(builder.values)
	if uint64(start)+uint64(len(values)) > uint64(math.MaxUint32) {
		return Span{}, false
	}
	builder.values = append(builder.values, values...)
	return Span{start: uint32(start), end: uint32(len(builder.values))}, true
}

// Len is the width accumulated so far. It is the denominator a family owner
// seals when a segment width is itself an authored number.
func (builder *PoolBuilder[Element]) Len() int {
	if builder == nil {
		return 0
	}
	return len(builder.values)
}

// Seal hands the accumulated column over as an immutable Pool and closes the
// builder. The builder retains no write path into the sealed values.
func (builder *PoolBuilder[Element]) Seal() Pool[Element] {
	if builder == nil {
		return Pool[Element]{}
	}
	builder.sealed = true
	values := builder.values
	builder.values = nil
	if len(values) == 0 {
		return Pool[Element]{}
	}
	return Pool[Element]{values: values}
}
