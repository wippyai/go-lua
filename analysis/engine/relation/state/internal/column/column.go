package column

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// factor is intentionally private.  A Column is a one-factor diagram; the
// factor is a physical implementation coordinate and is never a logical
// identity exposed to callers.
const factor uint64 = 1

// Cell is the semantic terminal carried by the column's reduced diagram.  It
// keeps Presence independent from the opaque semantic value: explicit
// ProvenAbsent and UnprovenMissing cells carry no ValueToken, while a sparse
// undefined diagram terminal means that no cell was committed at all.
type Cell struct {
	value    binding.ValueToken
	presence model.Presence
	// sealed is set only by NewCell after the complete value/presence
	// relationship has been checked. Cell is immutable after construction, so
	// the hot availability path need not revisit the value algebra.
	sealed bool
}

// NewCell authenticates a semantic cell without attaching a physical address.
// The column later checks the value's exact schema/mount/generation fence.
func NewCell(value binding.ValueToken, presence model.Presence) (Cell, bool) {
	if !presence.Available() || presence.Is(model.Refused) || !cellValueMatches(value, presence) {
		return Cell{}, false
	}
	return Cell{value: value, presence: presence, sealed: true}, true
}

// Available reports whether cell carries a complete semantic presence/value
// pair. It does not claim a particular column fence until Version.Next
// validates the opaque value token.
func (cell Cell) Available() bool {
	if cell.sealed {
		return true
	}
	return cell.presence.Available() && !cell.presence.Is(model.Refused) && cellValueMatches(cell.value, cell.presence)
}

// Value returns the opaque semantic token.  For explicit absence this is the
// unavailable zero token, never a synthesized domain default.
func (cell Cell) Value() binding.ValueToken { return cell.value }

// Presence returns the independent logical presence status.
func (cell Cell) Presence() model.Presence { return cell.presence }

// SemanticSame compares semantic value and presence, but not lineage.
func (cell Cell) SemanticSame(other Cell) bool {
	if !cell.Available() || !other.Available() || cell.presence != other.presence {
		return false
	}
	if cell.value.Available() || other.value.Available() {
		return cell.value.Available() && other.value.Available() && cell.value.Same(other.value)
	}
	return true
}

func cellValueMatches(value binding.ValueToken, presence model.Presence) bool {
	if !presence.Available() || presence.Is(model.Refused) {
		return false
	}
	if presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque) {
		return value.Available()
	}
	return !value.Available()
}

// Update is one geometry-resolved candidate write.  Key is a private scalar
// physical coordinate and mask is its independent support region; no scope or
// logical CellToken is folded into the key.
type Update struct {
	key     geometry.Key
	mask    support.Mask
	cell    Cell
	lineage model.LineageRef
	remove  bool
}

// NewUpdate constructs a candidate payload.  Exact guard-manager and runtime
// fence admission is performed by Version.Next, where the owning column is
// available.
func NewUpdate(key geometry.Key, mask support.Mask, cell Cell, lineage model.LineageRef) (Update, bool) {
	if !mask.Valid() || !cell.Available() || !lineage.Available() {
		return Update{}, false
	}
	return Update{key: key, mask: mask, cell: cell, lineage: lineage}, true
}

// NewRemoval constructs the sole column mutation that writes sparse
// undefined over an exact support region. Removal is deliberately an
// operation bit on Update rather than a second mutation type, and carries no
// Cell or lineage terminal. Version.Next deletes both roots for this region.
func NewRemoval(key geometry.Key, mask support.Mask) (Update, bool) {
	if !mask.Valid() {
		return Update{}, false
	}
	return Update{key: key, mask: mask, remove: true}, true
}

// Key returns the private scalar coordinate carried by update.  It is a
// physical lookup key, not a logical relation/column identity.
func (update Update) Key() geometry.Key { return update.key }

// Mask returns the support region independently from the physical key.
func (update Update) Mask() support.Mask { return update.mask }

// Cell returns the semantic payload carried by update.
func (update Update) Cell() Cell { return update.cell }

// Lineage returns the independent lineage sidecar carried by update.
func (update Update) Lineage() model.LineageRef { return update.lineage }

// Removal reports whether this update writes sparse undefined rather than a
// semantic cell. The operation remains private to the existing Update shape.
func (update Update) Removal() bool { return update.remove }

// Column owns one immutable semantic diagram root and one immutable lineage
// diagram root. Both diagrams use one private factor and the same sealed guard
// manager; their roots are advanced only together by Version.Next.
type Column struct {
	schema model.ColumnSchema
	fence  binding.Fence
	guards *guard.Manager
	// sealed is immutable constructor proof. All expensive terminal/diagram
	// checks happen before this bit is set; readers only need this proof.
	sealed bool

	semanticArena  *terminal.Arena[Cell]
	lineageArena   *terminal.Arena[model.LineageRef]
	semanticGraph  *diagram.Diagram[uint64, geometry.Key, Cell]
	lineageGraph   *diagram.Diagram[uint64, geometry.Key, model.LineageRef]
	emptySemantic  diagram.Root[uint64, geometry.Key, Cell]
	emptyLineage   diagram.Root[uint64, geometry.Key, model.LineageRef]
	semanticFactor uint64
}

// NewColumn creates the exact generation-owned sparse column substrate.
// Terminals and both diagrams are sealed before the column becomes available;
// no candidate terminal or root can escape this constructor.
func NewColumn(schema model.ColumnSchema, fence binding.Fence, guards *guard.Manager) (*Column, bool) {
	if !schema.Available() || !fence.Available() || guards == nil || !guards.Valid(guards.True()) {
		return nil, false
	}
	semanticArena, ok := terminal.New(terminal.Config[Cell]{Equal: func(left, right Cell) bool {
		return left.SemanticSame(right)
	}, Fingerprint: cellFingerprint})
	if !ok || !semanticArena.Seal() {
		return nil, false
	}
	lineageArena, ok := terminal.New(terminal.Config[model.LineageRef]{Equal: func(left, right model.LineageRef) bool {
		return left == right
	}, Fingerprint: lineageFingerprint})
	if !ok || !lineageArena.Seal() {
		return nil, false
	}
	semanticGraph, ok := diagram.New(diagram.Config[uint64, geometry.Key, Cell]{Factors: []uint64{factor}, Terminals: semanticArena, Guards: guards})
	if !ok {
		return nil, false
	}
	lineageGraph, ok := diagram.New(diagram.Config[uint64, geometry.Key, model.LineageRef]{Factors: []uint64{factor}, Terminals: lineageArena, Guards: guards})
	if !ok {
		return nil, false
	}
	column := &Column{
		schema: schema, fence: fence, guards: guards,
		semanticArena: semanticArena, lineageArena: lineageArena,
		semanticGraph: semanticGraph, lineageGraph: lineageGraph,
		emptySemantic: semanticGraph.Empty(), emptyLineage: lineageGraph.Empty(),
		semanticFactor: factor,
	}
	if !column.Available() {
		return nil, false
	}
	column.sealed = true
	return column, true
}

// Available reports whether column owns complete sealed terminal and diagram
// generations under its exact runtime fence.
func (column *Column) Available() bool {
	if column == nil {
		return false
	}
	if column.sealed {
		return true
	}
	return column.schema.Available() && column.fence.Available() && column.guards != nil && column.guards.Valid(column.guards.True()) && column.semanticArena != nil && column.semanticArena.Sealed() && column.lineageArena != nil && column.lineageArena.Sealed() && column.semanticGraph != nil && column.lineageGraph != nil && column.semanticGraph.Valid(column.emptySemantic) && column.lineageGraph.Valid(column.emptyLineage)
}

// Schema returns the immutable logical column declaration.
func (column *Column) Schema() model.ColumnSchema {
	if column == nil {
		return model.ColumnSchema{}
	}
	return column.schema
}

// ID returns the owner-issued logical column identity.
func (column *Column) ID() model.ColumnID {
	if column == nil || !column.Available() {
		return model.ColumnID{}
	}
	return column.schema.ID()
}

// Type returns the owner-issued semantic type identity.
func (column *Column) Type() model.TypeID {
	if column == nil || !column.Available() {
		return model.TypeID{}
	}
	return column.schema.Type()
}

// Relation returns the logical relation owning this column.
func (column *Column) Relation() model.RelationID {
	if column == nil || !column.Available() {
		return model.RelationID{}
	}
	return column.schema.Relation()
}

// Fence returns the exact schema/mount/generation authority for all semantic
// values admitted by this column.
func (column *Column) Fence() binding.Fence {
	if column == nil {
		return binding.Fence{}
	}
	return column.fence
}

// Guards returns the sealed support manager required by candidate masks.
func (column *Column) Guards() *guard.Manager {
	if column == nil {
		return nil
	}
	return column.guards
}

// Initial returns the immutable pair of empty diagram roots. It carries no
// fallback/default terminal: an undefined sparse lookup is a genuine miss.
func (column *Column) Initial() Version {
	if !column.Available() {
		return Version{}
	}
	value := Version{column: column, state: &versionState{semantic: column.emptySemantic, lineage: column.emptyLineage, semanticRevision: 1, lineageRevision: 1}}
	return sealVersion(value)
}

func cellFingerprint(cell Cell) uint64 {
	hash := uint64(1469598103934665603)
	hash = mixByte(hash, byte(cell.presence.Kind()))
	if reason, ok := cell.presence.Reason(); ok {
		hash = mixContent(hash, reason.Owner().Content())
		hash = mixContent(hash, reason.Content())
	}
	if cell.value.Available() {
		hash = mixContent(hash, cell.value.Type().Owner().Content())
		hash = mixContent(hash, cell.value.Type().Content())
		hash = mixContent(hash, cell.value.Opaque())
	}
	return hash
}

func lineageFingerprint(lineage model.LineageRef) uint64 {
	hash := uint64(1469598103934665603)
	hash = mixContent(hash, lineage.Owner().Content())
	return mixContent(hash, lineage.Content())
}

func mixContent(hash uint64, value identity.ContentID) uint64 {
	for _, byteValue := range value {
		hash = mixByte(hash, byteValue)
	}
	return hash
}

func mixByte(hash uint64, value byte) uint64 { return (hash ^ uint64(value)) * 1099511628211 }
