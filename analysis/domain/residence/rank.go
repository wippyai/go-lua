package residence

// WidenRank is the exact finite remaining-alternative witness for the one
// Residence family. It is a proof over sealed Link vocabulary, never a budget.
type WidenRank struct{ schema Schema }

func NewWidenRank(schema Schema) (WidenRank, bool) {
	if !schema.valid() {
		return WidenRank{}, false
	}
	return WidenRank{schema: schema}, true
}

func (rank WidenRank) Width() int {
	if !rank.schema.valid() {
		return 0
	}
	return 1
}

func (rank WidenRank) At(key Key, value Value, component int) uint64 {
	if component != 0 || !key.valid() || key.owner != rank.schema.owner || !rank.schema.owns(value) || value.top || uint64(len(value.facts)) > rank.schema.owner.potential {
		return 0
	}
	return rank.schema.owner.potential - uint64(len(value.facts)) + 1
}
