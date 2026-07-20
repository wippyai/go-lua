package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ObservationKind names the first immutable diagnostic-evidence tranche.
// Kinds are deliberately closed: an unrepresented observation family rejects
// whole-function authority rather than disappearing from diagnostics.
type ObservationKind = engineobservation.Kind

const (
	ObservationInvalid      = engineobservation.Invalid
	ObservationAssignment   = engineobservation.Assignment
	ObservationCallArgument = engineobservation.CallArgument
	ObservationCallResult   = engineobservation.CallResult
)

// ObservationTerm is guarded evidence captured at the exact CFG point where a
// lexical binding becomes visible. Guard is local to the observation; using an
// exit-row guard would incorrectly condition earlier evidence on later paths.
type ObservationTerm struct {
	BodyOwner lexicalidentity.StableLexicalBodyID
	Route     engineobservation.InvocationID
	Kind      ObservationKind
	Anchor    engineobservation.Occurrence
	Guard     Guard
	Slot      uint32
	Actual    ValueTerm
	Expected  ValueTerm
}

// observationObligation records the exact feasible world in which a local
// consumer occurrence must be emitted. It is deliberately separate from
// ObservationTerm: lowering can know that a consumer is owed while failing to
// construct its value, and that absence must keep whole-owner coverage false.
type observationObligation struct {
	BodyOwner lexicalidentity.StableLexicalBodyID
	Route     engineobservation.InvocationID
	Anchor    engineobservation.Occurrence
	Guard     Guard
}

// relationAnnotations carries reached-before-exit evidence independently of
// semantic return rows. A non-returning call has no successor Row, but its
// call-entry obligations and observations are still part of the lexical
// relation transaction and must publish atomically with it.
type relationAnnotations struct {
	observations []ObservationTerm
	obligations  []observationObligation
}

func unionRelationAnnotations(arena *Arena, sets ...relationAnnotations) relationAnnotations {
	var observations [][]ObservationTerm
	var obligations [][]observationObligation
	for _, set := range sets {
		observations = append(observations, set.observations)
		obligations = append(obligations, set.obligations)
	}
	return relationAnnotations{
		observations: unionObservationTerms(arena, observations...),
		obligations:  unionobservationObligations(obligations...),
	}
}

func equalRelationAnnotations(left, right relationAnnotations) bool {
	if len(left.observations) != len(right.observations) || len(left.obligations) != len(right.obligations) {
		return false
	}
	observations := make(map[ObservationTerm]struct{}, len(left.observations))
	for _, item := range left.observations {
		observations[item] = struct{}{}
	}
	for _, item := range right.observations {
		if _, ok := observations[item]; !ok {
			return false
		}
	}
	obligations := make(map[observationObligation]struct{}, len(left.obligations))
	for _, item := range left.obligations {
		obligations[item] = struct{}{}
	}
	for _, item := range right.obligations {
		if _, ok := obligations[item]; !ok {
			return false
		}
	}
	return true
}

func (o observationObligation) valid(arena *Arena, shape Shape) bool {
	return o.BodyOwner != (lexicalidentity.StableLexicalBodyID{}) && o.Anchor.Valid() && arena.validGuard(o.Guard, shape)
}

func recordobservationObligation(in []observationObligation, next observationObligation) []observationObligation {
	for _, prior := range in {
		if prior == next {
			return in
		}
	}
	return append(in, next)
}

func unionobservationObligations(sets ...[]observationObligation) []observationObligation {
	count := 0
	for _, set := range sets {
		count += len(set)
	}
	if count == 0 {
		return nil
	}
	out := make([]observationObligation, 0, count)
	for _, set := range sets {
		for _, obligation := range set {
			out = recordobservationObligation(out, obligation)
		}
	}
	return out
}

func (o ObservationTerm) valid(arena *Arena, shape Shape) bool {
	return o.Kind > ObservationInvalid && o.Kind <= ObservationCallResult &&
		o.BodyOwner != (lexicalidentity.StableLexicalBodyID{}) && o.Anchor.Valid() && o.Anchor.Kind == o.Kind && o.Anchor.Slot == o.Slot &&
		arena.validGuard(o.Guard, shape) && arena.validValue(o.Actual, shape, make(map[ValueTerm]bool)) &&
		(o.Expected == 0 || arena.validValue(o.Expected, shape, make(map[ValueTerm]bool)))
}

func (o ObservationTerm) canonical(arena *Arena) string {
	expected := "-"
	if o.Expected != 0 {
		expected = arena.canonicalValue(o.Expected)
	}
	return fmt.Sprintf("%x:%x:%d:%v:%s:%d:%s:%s", o.BodyOwner, o.Route, o.Kind, o.Anchor, arena.canonicalGuard(o.Guard), o.Slot, arena.canonicalValue(o.Actual), expected)
}

// Observation is one specialized immutable evidence cell.
type Observation struct {
	Owner       lexicalidentity.StableLexicalBodyID
	Invocation  engineobservation.InvocationID
	Kind        ObservationKind
	Anchor      engineobservation.Occurrence
	Slot        uint32
	Actual      product.Value
	Expected    product.Value
	HasExpected bool
}

// ObservationProjection is transient specialization output. It is not a
// cache/publication artifact: schema, codec, universe, and keyspace contracts
// belong to the later atomic diagnostics artifact. Before publication, Owner
// uses lexicalidentity.StableLexicalBodyID (CellRef remains internal routing),
// a tagged durable source occurrence, and abnormal
// terminal/cyclic observation rows must be projected independently of returns.
type ObservationProjection struct{ items []Observation }

func (a ObservationProjection) Items() []Observation { return append([]Observation(nil), a.items...) }

func canonicalizeObservations(reg *axis.Registry, in []Observation) ObservationProjection {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Owner != in[j].Owner {
			return in[i].Owner.String() < in[j].Owner.String()
		}
		if in[i].Invocation != in[j].Invocation {
			return fmt.Sprintf("%x", in[i].Invocation) < fmt.Sprintf("%x", in[j].Invocation)
		}
		if in[i].Kind != in[j].Kind {
			return in[i].Kind < in[j].Kind
		}
		if in[i].Anchor != in[j].Anchor {
			return in[i].Anchor.Less(in[j].Anchor)
		}
		if in[i].Slot != in[j].Slot {
			return in[i].Slot < in[j].Slot
		}
		if left, right := product.Hash(reg, in[i].Actual), product.Hash(reg, in[j].Actual); left != right {
			return left < right
		}
		if in[i].HasExpected != in[j].HasExpected {
			return !in[i].HasExpected
		}
		return product.Hash(reg, in[i].Expected) < product.Hash(reg, in[j].Expected)
	})
	out := in[:0]
	for _, item := range in {
		last := len(out) - 1
		if last >= 0 && sameObservationOccurrence(out[last], item) && out[last].HasExpected == item.HasExpected &&
			product.Equal(reg, out[last].Actual, item.Actual) && (!item.HasExpected || product.Equal(reg, out[last].Expected, item.Expected)) {
			continue
		}
		out = append(out, item)
	}
	return ObservationProjection{items: out}
}

func sameObservationOccurrence(left, right Observation) bool {
	return left.Owner == right.Owner && left.Invocation == right.Invocation && left.Kind == right.Kind && left.Anchor == right.Anchor && left.Slot == right.Slot
}

func recordObservationTerm(in []ObservationTerm, next ObservationTerm) []ObservationTerm {
	for _, prior := range in {
		if prior == next {
			return in
		}
	}
	return append(in, next)
}

// unionObservationTerms joins row annotations without changing the row's
// semantic witness. ObservationTerm keeps Actual and Expected in one record,
// so canonicalization can never destroy their correlation by independently
// joining the two values.
func unionObservationTerms(arena *Arena, sets ...[]ObservationTerm) []ObservationTerm {
	count := 0
	for _, set := range sets {
		count += len(set)
	}
	if count == 0 {
		return nil
	}
	out := make([]ObservationTerm, 0, count)
	for _, set := range sets {
		for _, term := range set {
			out = recordObservationTerm(out, term)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].canonical(arena) < out[j].canonical(arena)
	})
	return out
}
