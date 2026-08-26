package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Reduce answers one owner reduction in the ABI's closed outcome vocabulary.
//
// It is the total translation between two closed vocabularies and nothing
// else. It chooses no destination - the row is the one the binding declared -
// and it decides nothing about the batch, which is the engine's to decide.
// Absence and opacity stay distinct answers, so an owner that proved nothing
// never publishes a fabricated bottom value.
func Reduce[R any](emitter *Emitter[R], fact R, reduction structure.ReductionOutcome) outcome.Code {
	switch reduction {
	case structure.Concrete:
		if !emitter.Put(fact) {
			return outcome.Refused
		}
		return outcome.Produced
	case structure.AuthenticatedOpaque:
		if !emitter.PutOpaque(fact) {
			return outcome.Refused
		}
		return outcome.Opaque
	case structure.NoCandidate:
		return outcome.NoCandidate
	case structure.NoSelection:
		return outcome.NoSelection
	default:
		return outcome.Refused
	}
}

// Carried answers one owner carry transform. A carry states its disposition as
// its own success flag rather than as a reduction vocabulary, and a transform
// that did not hold produces no row.
func Carried[R any](emitter *Emitter[R], fact R, held bool) outcome.Code {
	if !held || !emitter.Put(fact) {
		return outcome.Refused
	}
	return outcome.Produced
}

// Answer settles one owner disposition for a family that publishes no fact.
//
// It is the same total translation Reduce performs, less the publication: a
// structural rule answers whether its occurrence holds and stages nothing, so
// a concrete answer produces its disposition and no row. An owner that tried
// to publish through such a family would find an emitter opened at a capacity
// of none, so the absence of a fact is enforced and not merely intended.
func Answer(reduction structure.ReductionOutcome) outcome.Code {
	switch reduction {
	case structure.Concrete:
		return outcome.Produced
	case structure.AuthenticatedOpaque:
		return outcome.Opaque
	case structure.NoCandidate:
		return outcome.NoCandidate
	case structure.NoSelection:
		return outcome.NoSelection
	default:
		return outcome.Refused
	}
}
