package typestate

// Acquire is Typestate's single-output acquisition reduction. Its authority
// is an already-validated Typestate acquisition declaration derived from a
// ResourceOrigin and a sealed Contract row. Value identity, reachability, and
// result transport remain deliberate prerequisites of their owning domains.
func (a Algebra) Acquire(acquisition Acquisition) (Fact, bool) {
	if !a.valid() || !a.schema.validAcquisition(acquisition) {
		return Fact{}, false
	}
	holder, ok := acquisition.resource.defaultHolder()
	if !ok {
		return Fact{}, false
	}
	value, ok := a.Of(acquisition.key, Entry{State: acquisition.state, Duty: DutyLocal, Holder: holder, Count: CountOne})
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: acquisition.key, Value: value}, true
}

// Transition applies one exact Contract transition outcome to one resource
// fact. The declaration was derived from the exact formal origin and existing
// Application × operation coordinate, so matching a protocol name or an
// application alone is insufficient. A final target state discharges cleanup.
//
// The result remains a Typestate fact.  This is not a multi-owner operation:
// Call/Value outcome propagation and all non-resource ownership conclusions
// are separate Rules with the same structural predecessor.
func (a Algebra) Transition(fact Fact, transition Transition) (Fact, bool) {
	if !a.validFact(fact) || !a.schema.validTransition(transition) {
		return Fact{}, false
	}
	if fact.Key != transition.key {
		return Fact{}, false
	}
	if fact.Value.IsTop() {
		// Top remains the conservative result: it may contain an unmatched
		// state, and the carrier has no per-state complement representation.
		return fact, true
	}
	entries := fact.Value.Entries()
	updated := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.State != transition.from {
			// A relation is a may-set. This outcome only rewrites the matching
			// predecessor alternative; alternatives for other predecessor states
			// remain possible after the reduction.
			updated = append(updated, entry)
			continue
		}
		entry.State = transition.to
		if transition.final {
			entry.Duty = DutyDischarged
		}
		updated = append(updated, entry)
	}
	value, ok := a.Of(fact.Key, updated...)
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: fact.Key, Value: value}, true
}

// ReleaseOneWhenHeld is the positive-holder branch of an exact release rule.
// It changes neither protocol state nor cleanup duty. In the may-set count
// algebra, one contributes zero, many contributes {one,many}, and an existing
// zero alternative is preserved. Thus {zero,one} becomes {zero}, while
// {zero,many} becomes {zero,one,many}.
//
// holder must be an existing exact holder coordinate for this fact's key.
func (a Algebra) ReleaseOneWhenHeld(fact Fact, holder HolderOrigin) (Fact, bool) {
	return a.releaseWhenHeld(fact, holder, false)
}

// ReleaseAllWhenHeld is ReleaseOneWhenHeld's all-holder counterpart. Every
// positive multiplicity becomes zero and an existing zero alternative remains
// zero, so its count result is always exactly {zero} for a valid selected
// holder coordinate.
func (a Algebra) ReleaseAllWhenHeld(fact Fact, holder HolderOrigin) (Fact, bool) {
	return a.releaseWhenHeld(fact, holder, true)
}

func (a Algebra) releaseWhenHeld(fact Fact, holder HolderOrigin, all bool) (Fact, bool) {
	if !a.validFact(fact) || !holder.validFor(a.schema.universe.source) {
		return Fact{}, false
	}
	if _, supported := a.schema.holderIndex(fact.Key, holder); !supported {
		return Fact{}, false
	}
	if fact.Value.IsTop() {
		return fact, true
	}
	entries := fact.Value.Entries()
	updated := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Holder != holder {
			updated = append(updated, entry)
			continue
		}
		next := releasedCount(entry.Count, all)
		if !next.Valid() {
			return Fact{}, false
		}
		entry.Count = next
		updated = append(updated, entry)
	}
	value, ok := a.Of(fact.Key, updated...)
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: fact.Key, Value: value}, true
}

func releasedCount(count Multiplicity, all bool) Multiplicity {
	if !count.Valid() {
		return 0
	}
	next := count & CountZero
	if all {
		if count.Has(CountOne) || count.Has(CountMany) {
			next |= CountZero
		}
		return next
	}
	if count.Has(CountOne) {
		next |= CountZero
	}
	if count.Has(CountMany) {
		next |= CountOne | CountMany
	}
	return next
}

func (a Algebra) validFact(fact Fact) bool {
	if !a.valid() || !a.accepts(fact.Value) {
		return false
	}
	key, ok := a.schema.Admit(fact.Key.Resource)
	if !ok || key != fact.Key || fact.Value.IsTop() {
		return ok && key == fact.Key
	}
	resource := fact.Key.Resource.ContentID()
	for _, cell := range fact.Value.cells {
		coordinateKey, _, coordinateOK := a.schema.CoordinateAt(int(cell.coordinate - 1))
		if !coordinateOK || coordinateKey != fact.Key || cell.resource != resource {
			return false
		}
	}
	return true
}
