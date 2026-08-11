package static

import "github.com/wippyai/go-lua/program/keyspace"

// RowTail is the closed, parser-authored tail vocabulary for a Static
// function effect row.  A row variable is local to its owning Function; it is
// not a global term, string label, or inferred effect identity.
type RowTail uint8

const (
	RowClosed RowTail = iota + 1
	RowVariable
)

// RowVar is a bounded, zero-based local row-variable coordinate.  Static
// currently has no effect-operation syntax or operation schema, so authored
// rows admit no occurrence labels yet; the tail coordinate is nevertheless
// retained as part of the closed RowSpec vocabulary for exact replay.
type RowVar uint32

// MaxRowVar is the largest representable local row coordinate.  It uses the
// same finite ordinal ceiling as canonical Program terms without turning the
// coordinate into a Program identity.
const MaxRowVar RowVar = RowVar(keyspace.MaxTermOrdinal)

// MaxRowFormals is the largest representable local row-formal denominator.
// The denominator is source-authored and local to one Function; it is not a
// dense Program family or a second effect identity.
const MaxRowFormals uint32 = keyspace.MaxTermOrdinal

// EffectOccurrence is intentionally an empty, closed placeholder.  The
// current parser/collector has no effect-operation syntax or schema identity,
// so Build accepts only rows with zero occurrences.  Keeping the occurrence
// position in RowSpec makes that absence explicit and leaves no string/Target/
// Link label authority hidden in Static.
type EffectOccurrence struct{}

// RowSpec is one authored Koka-style effect row.  Omitted Function rows and
// present rows whose Tail is RowClosed are distinct: presence belongs to the
// sparse EffectRows relation, while RowSpec carries only the row vocabulary.
// Occurrences is currently required to be empty; a future operation owner may
// extend the vocabulary only through an explicit source-schema cut.
type RowSpec struct {
	Occurrences []EffectOccurrence
	RowFormals  uint32
	Tail        RowTail
	Var         RowVar
}

// EffectRow is one sparse authored row owned by an existing canonical
// FamilyFunction term.  It does not mint a FamilyEffect and does not proxy a
// FunctionContract return/type sidecar.
type EffectRow struct {
	Function keyspace.Term
	Row      RowSpec
}

// EffectRowsInput is the parser/collector boundary for Static's authored
// effect-row relation.  Rows are sparse by Function and are canonicalized by
// Build; an omitted Function has no relation row.
type EffectRowsInput struct {
	Rows []EffectRow
}

type effectRowsStore struct {
	rows        []effectRow
	occurrences []EffectOccurrence
	// byFunction is a derived dense ordinal lookup. Zero means that the
	// sparse relation omits that Function; nonzero is row index plus one.
	byFunction []uint32
}

type effectRow struct {
	function    keyspace.Term
	occurrences poolRange
	rowFormals  uint32
	tail        RowTail
	variable    RowVar
}

// EffectRows is the immutable query view of the sparse authored relation.
// It carries only the owning Component/lifecycle capability and never a
// second row map or copied effect representation.
type EffectRows struct {
	component *Component
	state     *draftState
}
