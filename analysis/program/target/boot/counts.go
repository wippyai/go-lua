package boot

// Counts is the complete denominator contribution of the boot owner.
// Target publishes these values under schema-owned IDs without reopening boot
// rows or reconstructing a second relation walk.
type Counts struct {
	Roots                int
	Entries              int
	MetatableAttachments int
	Bindings             int
}

func (t *Table) Counts() Counts {
	if t == nil {
		return Counts{}
	}
	return Counts{
		Roots:                t.roots.Count(),
		Entries:              t.entries.Count(),
		MetatableAttachments: t.metatables.Count(),
		Bindings:             t.bindings.Count(),
	}
}
