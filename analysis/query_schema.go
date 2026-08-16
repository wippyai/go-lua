package analysis

import (
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programquery"
)

type valueSummaryObservation = programquery.ValueSummaryObservation

// Keep the production catalog's two concrete hot surfaces tied to their
// typed implementations. These assignments are compile-time checks, not a
// second registry or a runtime callback lookup.
var (
	_ valueSummaryHotSpecSignature = valueSummaryQueryHotSpec
	_ effectExactHotSpecSignature  = effectExactQueryHotSpec
)

type valueSummaryHotSpecSignature = func(*valuedomain.Schema, engine.SemanticKey) engine.HotSummaryQuerySpec[valuedomain.Value, valueSummaryObservation]
type effectExactHotSpecSignature = func(*effectfactor.Algebra, engine.SemanticKey) engine.HotExactQuerySpec[effectfactor.Value, effectObservation]

func valueSummaryQueryHotSpec(schema *valuedomain.Schema, freezer engine.SemanticKey) engine.HotSummaryQuerySpec[valuedomain.Value, valueSummaryObservation] {
	return engine.HotSummaryQuerySpec[valuedomain.Value, valueSummaryObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[valuedomain.Value], valueSummaryObservation]{
			Begin: func() valueSummaryObservation {
				if schema == nil || schema.CoordinateCount() == 0 {
					return valueSummaryObservation{}
				}
				return valueSummaryObservation{
					Values:  make([]valuedomain.Value, schema.CoordinateCount()),
					Present: make([]bool, schema.CoordinateCount()),
					Valid:   true,
				}
			},
			Accumulate: func(result valueSummaryObservation, cells engine.OrderedCells[valuedomain.Value]) (valueSummaryObservation, bool) {
				if schema == nil || !result.Valid || cells.Count() == 0 || len(result.Values) != cells.Count() || len(result.Present) != cells.Count() {
					return valueSummaryObservation{}, false
				}
				for index := range result.Values {
					value, present, ok := cells.At(index)
					if !ok {
						return valueSummaryObservation{}, false
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
						return valueSummaryObservation{}, false
					}
					result.Values[index] = joined
				}
				// Correlated observations are folded into one detached summary row.
				result.Rows = 1
				return result, true
			},
		},
		Result: engine.FrozenResult[valueSummaryObservation]{
			Semantic: freezer, Freeze: programquery.CloneValueSummary, Clone: programquery.CloneValueSummary,
			Equal: func(left, right valueSummaryObservation) bool {
				return programquery.EqualValueSummary(schema, left, right)
			},
			Fingerprint: func(value valueSummaryObservation) uint64 { return programquery.FingerprintValueSummary(schema, value) },
		},
	}
}

func effectExactQueryHotSpec(algebra *effectfactor.Algebra, freezer engine.SemanticKey) engine.HotExactQuerySpec[effectfactor.Value, effectObservation] {
	return engine.HotExactQuerySpec[effectfactor.Value, effectObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[effectfactor.Value], effectObservation]{
			Begin: func() effectObservation { return programquery.BeginEffect(algebra) },
			Accumulate: func(result effectObservation, cells engine.OrderedCells[effectfactor.Value]) (effectObservation, bool) {
				if cells.Count() != 1 {
					return effectObservation{}, false
				}
				value, present, available := cells.At(0)
				return programquery.AccumulateEffect(algebra, result, value, present, available)
			},
		},
		Result: engine.FrozenResult[effectObservation]{
			Semantic: freezer, Freeze: programquery.CloneEffect, Clone: programquery.CloneEffect,
			Equal: programquery.EqualEffect, Fingerprint: programquery.FingerprintEffect,
		},
	}
}
