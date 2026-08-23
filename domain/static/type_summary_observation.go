package static

import "github.com/wippyai/go-lua/analysis/engine"

// TypeSummaryObservation is the detached answer of Static's summary query.
//
// Static deliberately does not own the coordinate denominator.  The Value
// domain issues that denominator, so a running summary receives its fixed
// width explicitly and retains only the ClassSet that owns the TypeFacts it
// folds.  The owner is transient query state; it is not part of the detached
// result image.
type TypeSummaryObservation struct {
	Values  []TypeFact
	Present []bool
	Rows    uint32
	Valid   bool

	owner *ClassSet
	width uint32
}

// BeginTypeSummary opens one Static summary fold over an already-issued Value
// coordinate range.  The width is intentionally supplied by the caller: a
// Static ClassSet must not import or reconstruct Value's coordinate identity.
// A non-positive width is unavailable rather than an invented empty result.
func BeginTypeSummary(classes *ClassSet, width int) TypeSummaryObservation {
	if classes == nil || width <= 0 || uint64(width) > uint64(^uint32(0)) {
		return TypeSummaryObservation{}
	}
	return TypeSummaryObservation{
		Values:  make([]TypeFact, width),
		Present: make([]bool, width),
		Valid:   true,
		owner:   classes,
		width:   uint32(width),
	}
}

// AccumulateTypeSummary joins one engine-issued coordinate vector into the
// detached result.  The join is always delegated to ClassSet.JoinTypeFact;
// Static never introduces a second type or class relation for the fold.
func AccumulateTypeSummary(classes *ClassSet, result TypeSummaryObservation, cells engine.OrderedCells[TypeFact]) (TypeSummaryObservation, bool) {
	return AccumulateTypeSummaryRows(classes, result, cells.Count(), cells.At)
}

// AccumulateTypeSummaryRows is the same fold over an explicit dense vector.
// It is useful to the sealed reader and to package laws, while preserving the
// exact callback shape issued by the engine.  The hot path mutates the two
// already-owned planes and allocates no temporary row or map.
func AccumulateTypeSummaryRows(classes *ClassSet, result TypeSummaryObservation, count int, at func(index int) (TypeFact, bool, bool)) (TypeSummaryObservation, bool) {
	if classes == nil || result.owner != classes || !typeSummaryObservationOwned(classes, result) || at == nil || count == 0 || count != int(result.width) || len(result.Values) != count || len(result.Present) != count {
		return TypeSummaryObservation{}, false
	}
	for index := 0; index < count; index++ {
		fact, present, available := at(index)
		if !available {
			return TypeSummaryObservation{}, false
		}
		// An absent cell contributes no type fact and must not be widened to
		// Top.  A non-zero absent carrier may still be checked when it names
		// an owner, which keeps a foreign TypeFact from crossing the fence
		// while accepting the engine's sparse zero value.
		if !present {
			if fact.owner != nil && !classes.OwnsTypeFact(fact) {
				return TypeSummaryObservation{}, false
			}
			continue
		}
		if !classes.OwnsTypeFact(fact) || !fact.present {
			return TypeSummaryObservation{}, false
		}
		if !result.Present[index] {
			result.Values[index] = fact
			result.Present[index] = true
			result.Rows = 1
			continue
		}
		joined := classes.JoinTypeFact(result.Values[index], fact)
		result.Values[index] = joined
		result.Rows = 1
	}
	return result, true
}

// CloneTypeSummary detaches both mutable fold planes while retaining the
// exact ClassSet owner fence. The ClassSet and each TypeFact remain immutable
// owner-issued values and therefore need no deep copy.
func CloneTypeSummary(input TypeSummaryObservation) TypeSummaryObservation {
	input.Values = append([]TypeFact(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	return input
}

// OwnsTypeSummary reports whether an observation is a complete, owner-local
// summary value. It is the only in-memory admission fence used by the result
// codec and by consumers of a frozen query answer.
func OwnsTypeSummary(classes *ClassSet, observation TypeSummaryObservation) bool {
	return classes != nil && typeSummaryObservationOwned(classes, observation)
}

// EqualTypeSummary compares two detached Static summaries under one exact
// ClassSet owner. Presence is part of the result; absent cells do not acquire
// a fabricated bottom, Top, or language-level Unknown value.
func EqualTypeSummary(classes *ClassSet, left, right TypeSummaryObservation) bool {
	if classes == nil || left.owner != classes || right.owner != classes || !typeSummaryObservationOwned(classes, left) || !typeSummaryObservationOwned(classes, right) || left.Valid != right.Valid || left.Rows != right.Rows || left.width != right.width || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) {
		return false
	}
	for index := range left.Values {
		if left.Present[index] != right.Present[index] {
			return false
		}
		if left.Present[index] && !classes.EqualTypeFact(left.Values[index], right.Values[index]) {
			return false
		}
	}
	return true
}

// FingerprintTypeSummary is the allocation-free identity of an owned
// observation. It includes the coordinate position, presence plane, row
// cardinality, and each ClassSet-owned TypeFact's canonical fingerprint.
func FingerprintTypeSummary(classes *ClassSet, observation TypeSummaryObservation) uint64 {
	if classes == nil || observation.owner != classes || !typeSummaryObservationOwned(classes, observation) {
		return 0
	}
	hash := uint64(0xcbf29ce484222325)
	mix := func(value uint64) {
		hash ^= value
		hash *= 0x100000001b3
	}
	mix(uint64(observation.width))
	mix(uint64(observation.Rows))
	if observation.Valid {
		mix(1)
	}
	for index, present := range observation.Present {
		mix(uint64(index+1) * 0x9e3779b97f4a7c15)
		if present {
			mix(classes.TypeFactFingerprint(observation.Values[index]))
			mix(1)
		}
	}
	return hash
}

func typeSummaryObservationOwned(classes *ClassSet, observation TypeSummaryObservation) bool {
	if classes == nil || observation.owner != classes || !observation.Valid || observation.width == 0 || uint64(observation.width) != uint64(len(observation.Values)) || len(observation.Values) != len(observation.Present) || observation.Rows > 1 {
		return false
	}
	any := false
	for index, present := range observation.Present {
		fact := observation.Values[index]
		if present {
			if !classes.OwnsTypeFact(fact) || !fact.present {
				return false
			}
			any = true
			continue
		}
		// The initial accumulator uses the zero TypeFact for an unwritten
		// coordinate. If a caller supplies a non-zero carrier there, it may
		// only be one issued by this same ClassSet; foreign facts are never
		// silently hidden by the presence bit.
		if fact.owner != nil && !classes.OwnsTypeFact(fact) {
			return false
		}
	}
	return observation.Rows == summaryTypeRowsForPresence(any)
}

func summaryTypeRowsForPresence(present bool) uint32 {
	if present {
		return 1
	}
	return 0
}
