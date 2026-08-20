package value

import "github.com/wippyai/go-lua/analysis/engine"

// ValueSummaryObservation is the detached result of the Value summary query.
// It is declared beside the schema that folds it: a fold's output shape
// belongs to the domain that reduces into it. Fields are exported so the hot
// projector populates the exact type the cold schema owner declared its query
// slot with.
type ValueSummaryObservation struct {
	Values  []Value
	Present []bool
	Rows    uint32
	Valid   bool
	// owner is the exact schema that opened this transient engine answer.
	// It is deliberately not part of the detached wire result: the encoded
	// answer carries only the owner's Link identity and coordinate identities.
	owner *Schema
}

// BeginValueSummary starts the fold state for the summary Value query. The
// coordinate width is the schema's, so the accumulator is shaped by the domain
// that reduces into it rather than by the caller that opens the query.
func BeginValueSummary(schema *Schema) ValueSummaryObservation {
	if schema == nil || schema.CoordinateCount() == 0 {
		return ValueSummaryObservation{}
	}
	return ValueSummaryObservation{
		Values:  make([]Value, schema.CoordinateCount()),
		Present: make([]bool, schema.CoordinateCount()),
		Valid:   true,
		owner:   schema,
	}
}

// AccumulateValueSummary joins one observed coordinate vector into the
// detached result. The fold is coordinatewise: an absent cell leaves its
// coordinate untouched, the first present cell writes it, and a later present
// cell joins under the schema's own order.
func AccumulateValueSummary(schema *Schema, result ValueSummaryObservation, cells engine.OrderedCells[Value]) (ValueSummaryObservation, bool) {
	return AccumulateValueSummaryRows(schema, result, cells.Count(), cells.At)
}

// AccumulateValueSummaryRows is that fold stated over the coordinate vector
// itself rather than over the engine's observation of it. A solve reaches the
// fold through the cells it observed; a reader that holds the same vector as
// published rows reaches it directly. One fold law serves both, so the answer a
// published column folds to is the answer the solve folded.
func AccumulateValueSummaryRows(schema *Schema, result ValueSummaryObservation, count int, at func(index int) (Value, bool, bool)) (ValueSummaryObservation, bool) {
	if schema == nil || result.owner != schema || !summaryObservationOwned(schema, result) || at == nil || count == 0 || count != schema.CoordinateCount() || len(result.Values) != count || len(result.Present) != count {
		return ValueSummaryObservation{}, false
	}
	for index := range result.Values {
		value, present, ok := at(index)
		if !ok {
			return ValueSummaryObservation{}, false
		}
		if !present {
			continue
		}
		if !schema.owns(value) {
			return ValueSummaryObservation{}, false
		}
		if !result.Present[index] {
			result.Values[index], result.Present[index] = value, true
			continue
		}
		joined, ok := schema.Join(result.Values[index], value)
		if !ok {
			return ValueSummaryObservation{}, false
		}
		result.Values[index] = joined
	}
	// Correlated observations are folded into one detached summary row only
	// when the vector contains at least one present coordinate. An all-absent
	// vector is a covered zero-row observation, not a fabricated summary row.
	result.Rows = 0
	for _, present := range result.Present {
		if present {
			result.Rows = 1
			break
		}
	}
	return result, true
}

func CloneValueSummary(input ValueSummaryObservation) ValueSummaryObservation {
	input.Values = append([]Value(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	return input
}

func EqualValueSummary(schema *Schema, left, right ValueSummaryObservation) bool {
	if schema == nil || left.owner != schema || right.owner != schema || !summaryObservationOwned(schema, left) || !summaryObservationOwned(schema, right) || left.Valid != right.Valid || left.Rows != right.Rows || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) {
		return false
	}
	for index := range left.Values {
		if left.Present[index] != right.Present[index] || left.Present[index] && !schema.Equal(left.Values[index], right.Values[index]) {
			return false
		}
	}
	return true
}

func FingerprintValueSummary(schema *Schema, value ValueSummaryObservation) uint64 {
	if schema == nil || value.owner != schema || !summaryObservationOwned(schema, value) {
		return 0
	}
	result := uint64(value.Rows) << 32
	for index := range value.Values {
		result ^= uint64(index+1) * 0x9e3779b97f4a7c15
		if index < len(value.Present) && value.Present[index] {
			result ^= schema.Fingerprint(value.Values[index])
		}
	}
	if value.Valid {
		result ^= 1 << 63
	}
	return result
}

// summaryObservationOwned checks the canonical in-memory fold invariant.
// The query engine supplies the owner separately from its coordinate cells;
// retaining that exact pointer prevents an equal-content foreign Schema from
// entering a running fold or a frozen result.
func summaryObservationOwned(schema *Schema, observation ValueSummaryObservation) bool {
	if schema == nil || observation.owner != schema || !observation.Valid || len(observation.Values) == 0 || len(observation.Values) != len(observation.Present) || len(observation.Values) != schema.CoordinateCount() || observation.Rows > 1 {
		return false
	}
	any := false
	for index, present := range observation.Present {
		if !present {
			continue
		}
		if !schema.owns(observation.Values[index]) {
			return false
		}
		any = true
	}
	return observation.Rows == summaryRowsForPresence(any)
}

func summaryRowsForPresence(present bool) uint32 {
	if present {
		return 1
	}
	return 0
}
