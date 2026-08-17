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
	}
}

// AccumulateValueSummary joins one observed coordinate vector into the
// detached result. The fold is coordinatewise: an absent cell leaves its
// coordinate untouched, the first present cell writes it, and a later present
// cell joins under the schema's own order.
func AccumulateValueSummary(schema *Schema, result ValueSummaryObservation, cells engine.OrderedCells[Value]) (ValueSummaryObservation, bool) {
	if schema == nil || !result.Valid || cells.Count() == 0 || len(result.Values) != cells.Count() || len(result.Present) != cells.Count() {
		return ValueSummaryObservation{}, false
	}
	for index := range result.Values {
		value, present, ok := cells.At(index)
		if !ok {
			return ValueSummaryObservation{}, false
		}
		if !present {
			continue
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
	// Correlated observations are folded into one detached summary row.
	result.Rows = 1
	return result, true
}

func CloneValueSummary(input ValueSummaryObservation) ValueSummaryObservation {
	input.Values = append([]Value(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	return input
}

func EqualValueSummary(schema *Schema, left, right ValueSummaryObservation) bool {
	if schema == nil || left.Valid != right.Valid || left.Rows != right.Rows || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) {
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
	if schema == nil {
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
