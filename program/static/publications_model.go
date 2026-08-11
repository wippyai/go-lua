package static

import "github.com/wippyai/go-lua/program/keyspace"

// Publication is one authored binding from an Assign write-pair to a resolved
// type reference. Assign geometry and reference spelling remain owned by
// Flow/Source and References respectively; this relation owns neither.
type Publication struct {
	Assign keyspace.Term
	Pair   uint32
	Target keyspace.Term
}

// PublicationsInput is the complete dense TypePublication denominator.
type PublicationsInput struct{ Type []Publication }

// publicationRow is deliberately the complete retained representation. The
// duplicate-pair set exists only while building; paths, roots, selection
// frontiers, and reverse maps are owned elsewhere or derived later by Link.
type publicationRow struct {
	assign keyspace.Term
	pair   uint32
	target keyspace.Term
}

type publicationSlot struct {
	assign keyspace.Term
	pair   uint32
}

// Publications is the typed zero-allocation read view for the exact authored
// TypePublication relation.
type Publications struct {
	component *Component
	state     *draftState
}
