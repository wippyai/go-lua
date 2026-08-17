package analysis

import (
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Keep the production catalog's two concrete hot surfaces tied to their
// typed implementations. These assignments are compile-time checks, not a
// second registry or a runtime callback lookup.
var (
	_ valueSummaryHotSpecSignature = valueSummaryQueryHotSpec
	_ effectExactHotSpecSignature  = effectExactQueryHotSpec
)

type valueSummaryHotSpecSignature = func(*valuedomain.Schema, identity.SemanticKey) engine.HotSummaryQuerySpec[valuedomain.Value, valuedomain.ValueSummaryObservation]
type effectExactHotSpecSignature = func(*effectfactor.Algebra, identity.SemanticKey) engine.HotExactQuerySpec[effectfactor.Value, effectfactor.EffectObservation]

func valueSummaryQueryHotSpec(schema *valuedomain.Schema, freezer identity.SemanticKey) engine.HotSummaryQuerySpec[valuedomain.Value, valuedomain.ValueSummaryObservation] {
	return engine.HotSummaryQuerySpec[valuedomain.Value, valuedomain.ValueSummaryObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[valuedomain.Value], valuedomain.ValueSummaryObservation]{
			Begin: func() valuedomain.ValueSummaryObservation { return valuedomain.BeginValueSummary(schema) },
			Accumulate: func(result valuedomain.ValueSummaryObservation, cells engine.OrderedCells[valuedomain.Value]) (valuedomain.ValueSummaryObservation, bool) {
				return valuedomain.AccumulateValueSummary(schema, result, cells)
			},
		},
		Result: engine.FrozenResult[valuedomain.ValueSummaryObservation]{
			Semantic: freezer, Freeze: valuedomain.CloneValueSummary, Clone: valuedomain.CloneValueSummary,
			Equal: func(left, right valuedomain.ValueSummaryObservation) bool {
				return valuedomain.EqualValueSummary(schema, left, right)
			},
			Fingerprint: func(value valuedomain.ValueSummaryObservation) uint64 {
				return valuedomain.FingerprintValueSummary(schema, value)
			},
		},
	}
}

func effectExactQueryHotSpec(algebra *effectfactor.Algebra, freezer identity.SemanticKey) engine.HotExactQuerySpec[effectfactor.Value, effectfactor.EffectObservation] {
	return engine.HotExactQuerySpec[effectfactor.Value, effectfactor.EffectObservation]{
		Fold: engine.QueryFold[engine.OrderedCells[effectfactor.Value], effectfactor.EffectObservation]{
			Begin: func() effectfactor.EffectObservation { return effectfactor.BeginEffect(algebra) },
			Accumulate: func(result effectfactor.EffectObservation, cells engine.OrderedCells[effectfactor.Value]) (effectfactor.EffectObservation, bool) {
				if cells.Count() != 1 {
					return effectfactor.EffectObservation{}, false
				}
				value, present, available := cells.At(0)
				return effectfactor.AccumulateEffect(algebra, result, value, present, available)
			},
		},
		Result: engine.FrozenResult[effectfactor.EffectObservation]{
			Semantic: freezer, Freeze: effectfactor.CloneEffect, Clone: effectfactor.CloneEffect,
			Equal: effectfactor.EqualEffect, Fingerprint: effectfactor.FingerprintEffect,
		},
	}
}
