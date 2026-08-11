package typestate

// RetainAt adds one may holder count at an exact Link-backed holder
// coordinate.  It changes neither resource protocol state nor cleanup duty.
// Zero becomes one, one becomes many, and many remains many.
func (a Algebra) RetainAt(fact Fact, holder HolderOrigin) (Fact, bool) {
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
	for index := range entries {
		if entries[index].Holder == holder {
			entries[index].Count = retainedCount(entries[index].Count)
		}
	}
	value, ok := a.Of(fact.Key, entries...)
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: fact.Key, Value: value}, true
}

// Handoff transfers the complete selected holder multiplicity to another
// exact holder of the same Link resource.  It is the reduction used by
// sealed callback/suspension/external handoff rows: the source becomes zero,
// the destination receives the same possible positive multiplicity, and the
// protocol state and cleanup duty are preserved.  A caller cannot invent a
// destination by pairing otherwise valid lifecycle coordinates.
func (a Algebra) Handoff(fact Fact, from, to HolderOrigin) (Fact, bool) {
	if !a.validFact(fact) || !from.validFor(a.schema.universe.source) || !to.validFor(a.schema.universe.source) || from == to {
		return Fact{}, false
	}
	if _, supported := a.schema.holderIndex(fact.Key, from); !supported {
		return Fact{}, false
	}
	if _, supported := a.schema.holderIndex(fact.Key, to); !supported {
		return Fact{}, false
	}
	if fact.Value.IsTop() {
		return fact, true
	}
	entries := fact.Value.Entries()
	transferred := make([]Entry, 0, len(entries))
	for index := range entries {
		if entries[index].Holder != from {
			continue
		}
		positive := entries[index].Count & (CountOne | CountMany)
		entries[index].Count = CountZero
		if positive != 0 {
			entry := entries[index]
			entry.Holder = to
			entry.Count = positive
			transferred = append(transferred, entry)
		}
	}
	entries = append(entries, transferred...)
	value, ok := a.Of(fact.Key, entries...)
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: fact.Key, Value: value}, true
}

func retainedCount(count Multiplicity) Multiplicity {
	if !count.Valid() {
		return 0
	}
	var next Multiplicity
	if count.Has(CountZero) {
		next |= CountOne
	}
	if count.Has(CountOne) || count.Has(CountMany) {
		next |= CountMany
	}
	return next
}
