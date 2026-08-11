package typedomain

import "github.com/wippyai/go-lua/analysis/domain/type/internal/sequence"

// Mode is one immutable alternative of a Lua result pack. It contains only
// dense Table-local handles. Rules normally use the compact Closed, Open, and
// Opaque constructors below; Modes is exposed solely for an immutable query
// view and for the few rules that genuinely construct a finite disjunction.
type Mode = sequence.Mode
type ModeKind = sequence.ModeKind

const (
	ModeClosed = sequence.ModeClosed
	ModeKnown  = sequence.ModeKnown
	ModeOpaque = sequence.ModeOpaque
)

func ClosedMode(values ...Handle) Mode { return sequence.ClosedMode(values...) }
func KnownMode(prefix []Handle, element Handle, suffix []Handle) Mode {
	return sequence.KnownMode(prefix, element, suffix)
}
func OpaqueMode(prefix, suffix []Handle) Mode { return sequence.OpaqueMode(prefix, suffix) }

// WidenRank is the exact lexicographic descent witness for Pack widening.
// ShapeClass descends Bottom (2) -> finite alternatives (1) -> PackTop (0).
// With a fixed skeleton set, ExactLabels strictly descends whenever widening
// erases a varying exact label to TypeTop.
type WidenRank = sequence.Rank

// Pack is the single result-sequence carrier. It is Bottom, PackTop, or an
// immutable finite set of correlated one-tail Modes, all owned by one Table.
// It has no raw-type accessor and no compatibility single-form projection.
type Pack struct {
	table *Table
	value sequence.Value
}

// packInputs gives sequence.Assemble its fixed operands without building a
// transient []sequence.Value for every Rule evaluation.  Pack remains the
// owner-fenced public carrier; sequence sees only a synchronous immutable
// indexed view and cannot retain it.
type packInputs []Pack

func (input packInputs) Len() int { return len(input) }
func (input packInputs) At(index int) sequence.Value {
	return input[index].value
}

// groupedPackInputs is the private synchronous owner-fenced input view for
// the sequence-domain grouped Values law. The integer layout is supplied by a
// caller's cold source equation; Pack deliberately never learns that source
// identity or retains the layout.
type groupedPackInputs struct {
	groups      []Pack
	fixedGroups []uint32
	tailGroup   uint32
	hasTail     bool
}

func (input groupedPackInputs) GroupCount() int                  { return len(input.groups) }
func (input groupedPackInputs) GroupAt(index int) sequence.Value { return input.groups[index].value }
func (input groupedPackInputs) FixedCount() int                  { return len(input.fixedGroups) }
func (input groupedPackInputs) FixedGroupAt(index int) uint32    { return input.fixedGroups[index] }
func (input groupedPackInputs) TailGroup() (uint32, bool)        { return input.tailGroup, input.hasTail }

func (table *Table) Bottom() Pack { return Pack{table: table, value: sequence.Bottom()} }
func (table *Table) Top() Pack    { return Pack{table: table, value: sequence.Top()} }

func (table *Table) Closed(values ...Handle) (Pack, bool) {
	return table.Pack(ClosedMode(values...))
}

func (table *Table) Open(prefix []Handle, element Handle, suffix []Handle) (Pack, bool) {
	return table.Pack(KnownMode(prefix, element, suffix))
}

func (table *Table) Opaque(prefix, suffix []Handle) (Pack, bool) {
	return table.Pack(OpaqueMode(prefix, suffix))
}

// Pack builds the finite alternative carrier in one path. It rejects foreign,
// forged, and malformed modes before normalization, so sequence never needs a
// public type graph, string key, global registry, codec, or subtype relation.
func (table *Table) Pack(modes ...Mode) (Pack, bool) {
	// A finite alternative set is recurrent state. It must never observe a
	// partly admitted local type universe: all labels are frozen before the
	// first fact exists. Bottom and PackTop remain available through their
	// dedicated constructors, but any component-bearing Pack requires Seal.
	if table == nil || !table.Sealed() {
		return Pack{}, false
	}
	for _, mode := range modes {
		if !sequence.ValidMode(mode, table.Valid) {
			return Pack{}, false
		}
	}
	return Pack{table: table, value: sequence.FromModes(table, modes...)}, true
}

func (pack Pack) IsBottom() bool { return pack.value.IsBottom() }
func (pack Pack) IsTop() bool    { return pack.value.IsTop() }

// Modes returns a deep-copy immutable view. Its order is non-semantic.
func (pack Pack) Modes() []Mode { return pack.value.Modes() }

func (pack Pack) WidenRank() WidenRank {
	if pack.table == nil || !pack.table.Sealed() {
		return WidenRank{}
	}
	return sequence.WidenRank(pack.table, pack.value)
}

func (pack Pack) Hash() uint64 {
	if pack.table == nil || !pack.table.Sealed() {
		return 0
	}
	return sequence.Hash(pack.table, pack.value)
}

func same(left, right Pack) (*Table, bool) {
	return left.table, left.table != nil && left.table == right.table && left.table.Sealed()
}

func Equal(left, right Pack) bool {
	table, ok := same(left, right)
	return ok && sequence.Equal(table, left.value, right.value)
}

func LessEqual(left, right Pack) bool {
	table, ok := same(left, right)
	return ok && sequence.LessEqual(table, left.value, right.value)
}

// Join is exact finite alternative union. It is used on acyclic flow and may
// retain arbitrarily many alternatives; it never invokes a cardinality cap.
func Join(left, right Pack) (Pack, bool) {
	table, ok := same(left, right)
	if !ok {
		return Pack{}, false
	}
	return Pack{table: table, value: sequence.Join(table, left.value, right.value)}, true
}

// Widen is legal only when the sealed Solver executes an explicit Mu/SCC
// recurrence boundary. Acyclic Rules use Join. The carrier itself enforces the
// mathematical widening transform but cannot infer a caller's topology.
func Widen(previous, next Pack) (Pack, bool) {
	table, ok := same(previous, next)
	if !ok {
		return Pack{}, false
	}
	return Pack{table: table, value: sequence.Widen(table, previous.value, next.value)}, true
}

// Scalar applies Lua's non-final expression-list rule.
func Scalar(value Pack) (Pack, bool) {
	if value.table == nil || !value.table.Sealed() {
		return Pack{}, false
	}
	return Pack{table: value.table, value: sequence.Scalar(value.table, value.value)}, true
}

// Assemble is the sole Program Values construction law. It scalar-adjusts
// every fixed expression Pack and forwards final unchanged. Callers represent
// an absent final Values tail with the Table's closed-empty Pack; Assemble
// deliberately has no nil-tail, residual, splice, or general concatenation
// form.
//
// It is exact for the Factor's finite-alternative carrier: every feasible
// scalar alignment of each fixed input is paired with every feasible final
// alternative. Rules invoke it per compatible engine tuple, before any
// cross-guard join, so the carrier retains both pack and control correlation.
// As with every exact finite disjunction operation, independent alternatives
// can require a Cartesian number of retained modes. No cardinality cap or
// label join is legal here.
func Assemble(fixed []Pack, final Pack) (Pack, bool) {
	if final.table == nil || !final.table.Sealed() {
		return Pack{}, false
	}
	for _, value := range fixed {
		if value.table != final.table || !value.table.Sealed() {
			return Pack{}, false
		}
	}
	return Pack{table: final.table, value: sequence.AssembleInputs(final.table, packInputs(fixed), final.value)}, true
}

// AssembleGrouped is the correlation-preserving Program Values law. groups
// are distinct fact inputs, while fixedGroups and tailGroup name those groups
// by compact cold indices. Repeated indices mean one source realization must
// be used in every named slot. It is therefore deliberately separate from
// Assemble, whose operands are independent occurrences.
//
// empty is the closed-empty Pack used when no final source expression exists.
// It supplies the Table authority even for an empty expression list.
func AssembleGrouped(groups []Pack, fixedGroups []uint32, tailGroup uint32, hasTail bool, empty Pack) (Pack, bool) {
	if empty.table == nil || !empty.table.Sealed() || empty.value.IsBottom() {
		return Pack{}, false
	}
	closedEmpty := sequence.FromModes(empty.table, sequence.ClosedMode())
	if !sequence.Equal(empty.table, empty.value, closedEmpty) {
		return Pack{}, false
	}
	for _, group := range groups {
		if group.table != empty.table || !group.table.Sealed() {
			return Pack{}, false
		}
	}
	if hasTail {
		index := int(tailGroup)
		if index < 0 || index >= len(groups) || uint32(index) != tailGroup {
			return Pack{}, false
		}
	}
	for _, raw := range fixedGroups {
		index := int(raw)
		if index < 0 || index >= len(groups) || uint32(index) != raw {
			return Pack{}, false
		}
	}
	input := groupedPackInputs{
		groups:      groups,
		fixedGroups: fixedGroups,
		tailGroup:   tailGroup,
		hasTail:     hasTail,
	}
	return Pack{table: empty.table, value: sequence.AssembleGrouped(empty.table, input)}, true
}

// FixedAt projects one demanded assignment/parameter/result position. The
// returned Pack is a scalar marginal (closed one-label alternatives); the
// original Pack remains the only representation that carries correlations
// between destination positions. width is a destination-boundary check, not
// an instruction to manufacture a width-sized intermediate Pack.
func FixedAt(value Pack, width, index int) (Pack, bool) {
	if value.table == nil || !value.table.Sealed() || width < 0 || index < 0 || index >= width {
		return Pack{}, false
	}
	return Pack{table: value.table, value: sequence.FixedAt(value.table, value.value, index)}, true
}
