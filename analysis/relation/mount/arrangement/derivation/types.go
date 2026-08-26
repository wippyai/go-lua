// Package derivation owns the cold, mount-time physical derivative of one
// logical expression. It is deliberately a child of arrangement: the parent
// package owns the sealed execution handle, while this package owns the
// bounded immutable path data that the handle redeems directly.
package derivation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const pathDigestDomain = "analysis/relation/mount/arrangement/derivation/path/v1"

// Orientation records which side of an oriented Join supplied the path's
// child.  It is not an unordered Join membership flag: swapping the sides
// changes the derivative.
type Orientation uint8

const (
	OrientationNone Orientation = iota
	OrientationLeft
	OrientationRight
)

func (value Orientation) Available() bool {
	return value == OrientationNone || value == OrientationLeft || value == OrientationRight
}

// Access is the logical part of one physical arrangement binding.  The
// physical digest is kept beside it in sealedAccess and is never synthesized
// while redeeming a path.
type Access struct {
	relation model.RelationID
	key      model.KeyID
	columns  []model.ColumnID
}

func newAccess(relation model.RelationID, key model.KeyID, columns []model.ColumnID) (Access, bool) {
	if !relation.Available() {
		return Access{}, false
	}
	if key.Available() && key.Relation() != relation {
		return Access{}, false
	}
	copyOf := append([]model.ColumnID(nil), columns...)
	seen := make(map[model.ColumnID]struct{}, len(copyOf))
	for _, column := range copyOf {
		if !column.Available() || column.Relation() != relation {
			return Access{}, false
		}
		if _, duplicate := seen[column]; duplicate {
			return Access{}, false
		}
		seen[column] = struct{}{}
	}
	return Access{relation: relation, key: key, columns: copyOf}, true
}

func NewAccess(relation model.RelationID, key model.KeyID, columns []model.ColumnID) (Access, bool) {
	return newAccess(relation, key, columns)
}

func (value Access) Available() bool {
	if !value.relation.Available() || value.key.Available() && value.key.Relation() != value.relation {
		return false
	}
	for index, column := range value.columns {
		if !column.Available() || column.Relation() != value.relation {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if value.columns[prior] == column {
				return false
			}
		}
	}
	return true
}

func (value Access) Relation() model.RelationID { return value.relation }
func (value Access) Key() model.KeyID           { return value.key }
func (value Access) Columns() []model.ColumnID {
	return append([]model.ColumnID(nil), value.columns...)
}

func (value Access) equal(other Access) bool {
	if value.relation != other.relation || value.key != other.key || len(value.columns) != len(other.columns) {
		return false
	}
	for index := range value.columns {
		if value.columns[index] != other.columns[index] {
			return false
		}
	}
	return true
}

// Equal compares the complete immutable logical access, including ordered
// columns. Physical identity is intentionally compared separately through
// SiblingAccess.Physical.
func (value Access) Equal(other Access) bool { return value.equal(other) }

// Binding is the physical evidence selected by arrangement before path
// derivation.  Derivation may match it, but cannot resolve another layout or
// retain a resolver/callback for later use.
type Binding struct {
	access   Access
	physical identity.ContentID
}

func NewBinding(relation model.RelationID, key model.KeyID, columns []model.ColumnID, physical identity.ContentID) (Binding, bool) {
	access, ok := newAccess(relation, key, columns)
	if !ok || !physical.Available() {
		return Binding{}, false
	}
	return Binding{access: access, physical: physical}, true
}

func (value Binding) Available() bool {
	return value.access.Available() && value.physical.Available()
}

func (value Binding) Access() Access               { return value.access }
func (value Binding) Physical() identity.ContentID { return value.physical }

// InputBinding is the exact mounted row vector for one sealed Input
// occurrence. The Input digest, rather than RelationID, is the binding key:
// two occurrences of one relation may carry different projections and must
// never alias in a derivation plan.
type InputBinding struct {
	input    identity.ContentID
	relation model.RelationID
	columns  []model.ColumnID
	physical identity.ContentID
	// sealed records the constructor's complete vector proof.  Available is a
	// hot redemption predicate; it must not rebuild the duplicate-detection
	// map that NewInputBinding already evaluated at mount.
	sealed bool
}

func NewInputBinding(input identity.ContentID, relation model.RelationID, columns []model.ColumnID, physical identity.ContentID) (InputBinding, bool) {
	access, ok := newAccess(relation, model.KeyID{}, columns)
	if !ok || !input.Available() || !physical.Available() {
		return InputBinding{}, false
	}
	return InputBinding{input: input, relation: relation, columns: access.columns, physical: physical, sealed: true}, true
}

func (value InputBinding) Available() bool {
	return value.sealed && value.relation.Available() && value.input.Available() && value.physical.Available()
}
func (value InputBinding) Input() identity.ContentID  { return value.input }
func (value InputBinding) Relation() model.RelationID { return value.relation }
func (value InputBinding) Columns() []model.ColumnID {
	return append([]model.ColumnID(nil), value.columns...)
}
func (value InputBinding) Physical() identity.ContentID { return value.physical }

type sealedAccess struct {
	access   Access
	physical identity.ContentID
}

func (value sealedAccess) available() bool {
	return value.access.Available() && value.physical.Available()
}

// SiblingAccess is the sealed physical access carried by a Frame. It exposes
// only copied logical IDs and the already-issued physical digest; no resolver
// or schema catalogue can be recovered from it.
type SiblingAccess struct{ value sealedAccess }

func (value SiblingAccess) Available() bool              { return value.value.available() }
func (value SiblingAccess) Access() Access               { return value.value.access }
func (value SiblingAccess) Physical() identity.ContentID { return value.value.physical }
func (value SiblingAccess) Equal(other SiblingAccess) bool {
	return value.Available() && other.Available() && value.value.physical == other.value.physical && value.value.access.equal(other.value.access)
}

// ChildWitness is the complete sealed evidence for one authored child of a
// Merge frame. Unlike SiblingAccess, which intentionally omits the active
// zipper child, this witness retains the child node identity and closed
// algebra kind beside its exact physical access.
type ChildWitness struct {
	ordinal uint32
	value   sealedAccess
	node    identity.ContentID
	kind    algebra.Kind
}

// newChildWitness seals one authored child identity beside its physical
// access. The derivation builder is the owner of this constructor; callers
// only redeem the immutable witness through the frame's child accessors.
func newChildWitness(ordinal uint32, access sealedAccess, node identity.ContentID, kind algebra.Kind) (ChildWitness, bool) {
	if !access.available() || !node.Available() || !validKind(kind) {
		return ChildWitness{}, false
	}
	access = sealedAccess{access: Access{relation: access.access.relation, key: access.access.key, columns: append([]model.ColumnID(nil), access.access.columns...)}, physical: access.physical}
	value := ChildWitness{ordinal: ordinal, value: access, node: node, kind: kind}
	return value, value.Available()
}

func (value ChildWitness) Available() bool {
	return value.value.available() && value.node.Available() && validKind(value.kind)
}
func (value ChildWitness) Ordinal() uint32              { return value.ordinal }
func (value ChildWitness) Access() Access               { return value.value.access }
func (value ChildWitness) Physical() identity.ContentID { return value.value.physical }
func (value ChildWitness) Node() identity.ContentID     { return value.node }
func (value ChildWitness) Kind() algebra.Kind           { return value.kind }
func (value ChildWitness) Equal(other ChildWitness) bool {
	return value.Available() && other.Available() && value.ordinal == other.ordinal && value.node == other.node && value.kind == other.kind && value.value.physical == other.value.physical && value.value.access.equal(other.value.access)
}

func validKind(value algebra.Kind) bool {
	switch value {
	case algebra.KindInput,
		algebra.KindSelect,
		algebra.KindProject,
		algebra.KindJoin,
		algebra.KindMerge,
		algebra.KindGroup,
		algebra.KindComplete,
		algebra.KindApply,
		algebra.KindPublish,
		algebra.KindColumnProject,
		algebra.KindExpand:
		return true
	default:
		return false
	}
}

func (value sealedAccess) equal(other sealedAccess) bool {
	return value.available() && other.available() && value.physical == other.physical && value.access.equal(other.access)
}

// CompleteReplay is the mount-issued witness for the only Complete child
// shape that the differential evaluator can replay without reopening algebra:
// Complete(Select(Input)).  The child Input's full row vector and relation
// cofiber are retained as sealed physical accesses, while the Select and
// Input node identities let runtime redeem their already-mounted bindings.
//
// It intentionally carries no Reader, denominator rows, cache, callback, or
// logical expression.  The denominator remains the Complete frame's owner;
// runtime obtains its ordered RowIDs from that mounted witness and uses this
// value only to bind the exact unkeyed child vector and reapply the sealed
// Select.
type CompleteReplay struct {
	parentNode  identity.ContentID
	occurrence  uint32
	inputNode   identity.ContentID
	selectNode  identity.ContentID
	values      sealedAccess
	range_      sealedAccess
	denominator model.DenominatorRef
	order       sealedAccess
	scope       model.ScopeID
	digest      identity.ContentID
	sealed      bool
}

func newCompleteReplay(parentNode identity.ContentID, occurrence uint32, inputNode, selectNode identity.ContentID, values, range_, order sealedAccess, denominator model.DenominatorRef, scope model.ScopeID) (CompleteReplay, bool) {
	if !parentNode.Available() || !inputNode.Available() || !selectNode.Available() || !values.available() || !range_.available() || !order.available() || !denominator.Available() || !scope.Available() {
		return CompleteReplay{}, false
	}
	if values.access.relation != range_.access.relation || values.access.relation != denominator.Relation() || values.access.key.Available() || range_.access.key.Available() || len(values.access.columns) == 0 || len(range_.access.columns) != 0 {
		return CompleteReplay{}, false
	}
	if order.access.relation != denominator.Relation() || order.access.key != denominator.Key() || len(order.access.columns) != 0 {
		return CompleteReplay{}, false
	}
	value := CompleteReplay{
		parentNode:  parentNode,
		occurrence:  occurrence,
		inputNode:   inputNode,
		selectNode:  selectNode,
		values:      sealedAccess{access: Access{relation: values.access.relation, key: values.access.key, columns: append([]model.ColumnID(nil), values.access.columns...)}, physical: values.physical},
		range_:      sealedAccess{access: Access{relation: range_.access.relation, key: range_.access.key, columns: append([]model.ColumnID(nil), range_.access.columns...)}, physical: range_.physical},
		denominator: denominator,
		order:       sealedAccess{access: Access{relation: order.access.relation, key: order.access.key, columns: append([]model.ColumnID(nil), order.access.columns...)}, physical: order.physical},
		scope:       scope,
		sealed:      true,
	}
	value.digest, _ = value.recomputeDigest()
	return value, value.Available()
}

func (value CompleteReplay) Available() bool {
	if !value.sealed || !value.parentNode.Available() || !value.inputNode.Available() || !value.selectNode.Available() || !value.values.available() || !value.range_.available() || !value.order.available() || !value.denominator.Available() || !value.scope.Available() || !value.digest.Available() {
		return false
	}
	if value.values.access.relation != value.range_.access.relation || value.values.access.relation != value.denominator.Relation() || value.values.access.key.Available() || value.range_.access.key.Available() || len(value.values.access.columns) == 0 || len(value.range_.access.columns) != 0 {
		return false
	}
	if value.order.access.relation != value.denominator.Relation() || value.order.access.key != value.denominator.Key() || len(value.order.access.columns) != 0 {
		return false
	}
	digest, ok := value.recomputeDigest()
	return ok && digest == value.digest
}

func (value CompleteReplay) specified() bool {
	return value.sealed || value.parentNode.Available() || value.inputNode.Available() || value.selectNode.Available() || value.values.available() || value.range_.available() || value.order.available() || value.denominator.Available() || value.scope.Available() || value.digest.Available()
}

func (value CompleteReplay) recomputeDigest() (identity.ContentID, bool) {
	if !value.parentNode.Available() || !value.inputNode.Available() || !value.selectNode.Available() || !value.values.available() || !value.range_.available() || !value.order.available() || !value.denominator.Available() || !value.scope.Available() {
		return identity.ContentID{}, false
	}
	parts := [][]byte{contentBytes(value.parentNode), contentBytes(value.inputNode), contentBytes(value.selectNode)}
	appendUint32(&parts, value.occurrence)
	parts = append(parts,
		accessDigest(value.values.access), contentBytes(value.values.physical),
		accessDigest(value.range_.access), contentBytes(value.range_.physical),
		accessDigest(value.order.access), contentBytes(value.order.physical),
		nominalBytes(value.denominator.Relation().Owner().Content(), value.denominator.Relation().Content()),
		nominalBytes(value.denominator.Key().Relation().Owner().Content(), value.denominator.Key().Content()),
		nominalBytes(value.scope.Owner().Content(), value.scope.Content()))
	return identity.DeriveContentID(pathDigestDomain+"/complete-replay/v1", parts...)
}

// withOccurrence binds the path occurrence after the leaf walk has assigned
// its stable order. The replay is otherwise immutable and remains mount data.
func (value CompleteReplay) withOccurrence(occurrence uint32) (CompleteReplay, bool) {
	if !value.sealed {
		return CompleteReplay{}, false
	}
	value.occurrence = occurrence
	value.digest, _ = value.recomputeDigest()
	return value, value.Available()
}

// ParentNode returns the exact Complete node that owns this replay.
func (value CompleteReplay) ParentNode() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.parentNode
}

// Occurrence returns the stable authored Input occurrence carrying this
// Complete frame.
func (value CompleteReplay) Occurrence() uint32 {
	if !value.Available() {
		return 0
	}
	return value.occurrence
}

func (value CompleteReplay) InputNode() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.inputNode
}

func (value CompleteReplay) SelectNode() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.selectNode
}

func (value CompleteReplay) Values() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.values}
}

func (value CompleteReplay) Range() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.range_}
}

func (value CompleteReplay) Scope() model.ScopeID {
	if !value.Available() {
		return model.ScopeID{}
	}
	return value.scope
}

// Denominator returns the exact Complete denominator identity.
func (value CompleteReplay) Denominator() model.DenominatorRef {
	if !value.Available() {
		return model.DenominatorRef{}
	}
	return value.denominator
}

// Order returns the exact mounted denominator-key access used to preserve
// order identity. It is not a request to resolve another layout.
func (value CompleteReplay) Order() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.order}
}

// Digest returns the sealed replay identity.
func (value CompleteReplay) Digest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.digest
}

// Frame is one parent context in a leaf-to-root zipper.  Siblings are ordered
// in authored order and carry their exact mounted physical digest.  A frame
// contains identities and contracts only; the logical algebra expression is
// intentionally not retained.
type Frame struct {
	kind        algebra.Kind
	orientation Orientation
	// node is the sealed logical node whose parent context this frame owns.
	// Runtime redeems it through arrangement.Execution's immutable node
	// directory; it must never rediscover a frame owner by walking the root.
	node     identity.ContentID
	ordinal  uint32
	siblings []sealedAccess
	// children is populated only for Merge. Unlike siblings, which omit the
	// active authored branch for the ordinary zipper ascent, it retains every
	// authored child witness in stable child order. The physical identity,
	// node digest, and closed kind are sealed at mount; redeeming a differential
	// Merge must never reconstruct a missing child access or widen it into a
	// relation scan.
	children       []ChildWitness
	scope          model.ScopeID
	denominator    model.DenominatorRef
	operation      signature.Identity
	destination    model.RelationID
	key            model.KeyID
	cardinality    model.Cardinality
	completeReplay CompleteReplay
	columns        []model.ColumnID
	slots          []algebra.ColumnSlot
	sources        []algebra.SlotSource
	mapTargets     []model.ColumnID
	// Expand retains the model contract and the mount-issued evidence digest.
	// The physical C/R/key accesses are carried in that fixed order. P is an
	// owner/evidence relation rather than a runtime row layout; its exact
	// correspondence remains sealed in Evidence. The active C expression is
	// the zipper child and is therefore not duplicated as a logical callback or
	// runtime tuple source.
	expandContract model.ExpandContract
	expandEvidence identity.ContentID
}

func (frame Frame) Available() bool {
	if !frame.orientation.Available() || !frame.node.Available() || frame.siblings == nil {
		return false
	}
	for _, sibling := range frame.siblings {
		if !sibling.available() {
			return false
		}
	}
	switch frame.kind {
	case algebra.KindSelect:
		return frame.scope.Available() && frame.orientation == OrientationNone && len(frame.siblings) == 0 && frame.children == nil && frame.denominator == (model.DenominatorRef{}) && !frame.operation.Available() && !frame.destination.Available() && !frame.key.Available() && frame.columns == nil && frame.slots == nil && frame.sources == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindComplete:
		return frame.denominator.Available() && frame.orientation == OrientationNone && len(frame.siblings) == 1 && frame.children == nil && frame.siblings[0].access.key == frame.denominator.Key() && frame.siblings[0].access.relation == frame.denominator.Relation() && (!frame.completeReplay.specified() || frame.completeReplay.Available()) && frame.columns == nil && frame.slots == nil && frame.sources == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindJoin:
		return (frame.orientation == OrientationLeft || frame.orientation == OrientationRight) && len(frame.siblings) == 1 && frame.children == nil && !frame.siblings[0].access.key.Available() && len(frame.columns) != 0 && frame.denominator == (model.DenominatorRef{}) && !frame.operation.Available() && !frame.destination.Available() && !frame.key.Available() && frame.slots == nil && frame.sources == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindMerge:
		if frame.orientation != OrientationNone || !frame.key.Available() || len(frame.columns) == 0 || frame.denominator != (model.DenominatorRef{}) || frame.operation.Available() || frame.destination != (model.RelationID{}) || frame.slots != nil || frame.sources != nil || frame.mapTargets != nil || frame.key.Relation() != frame.columns[0].Relation() || frame.children == nil || len(frame.children) == 0 || len(frame.siblings)+1 != len(frame.children) || frame.ordinal >= uint32(len(frame.children)) || frame.expandContract.Available() || frame.expandEvidence.Available() {
			return false
		}
		for _, sibling := range frame.siblings {
			if sibling.access.key.Available() || !sameColumns(sibling.access.columns, frame.columns) || sibling.access.relation != frame.columns[0].Relation() {
				return false
			}
		}
		for index, child := range frame.children {
			if !child.Available() || child.ordinal != uint32(index) || child.value.access.key.Available() || !sameColumns(child.value.access.columns, frame.columns) || child.value.access.relation != frame.columns[0].Relation() {
				return false
			}
		}
		// Siblings are the authored child list with exactly the active ordinal
		// removed. Comparing the complete sealed vectors here prevents a caller
		// from substituting a relation carry that happens to have the same row
		// shape as the active child.
		siblingIndex := 0
		for childIndex, child := range frame.children {
			if uint32(childIndex) == frame.ordinal {
				continue
			}
			if siblingIndex >= len(frame.siblings) || !frame.siblings[siblingIndex].equal(child.value) {
				return false
			}
			siblingIndex++
		}
		if siblingIndex != len(frame.siblings) {
			return false
		}
		return true
	case algebra.KindGroup:
		return frame.orientation == OrientationNone && frame.key.Available() && frame.cardinality.Available() && len(frame.siblings) == 1 && frame.children == nil && frame.siblings[0].access.key == frame.key && frame.siblings[0].access.relation == frame.key.Relation() && frame.denominator == (model.DenominatorRef{}) && !frame.operation.Available() && !frame.destination.Available() && frame.columns == nil && frame.slots == nil && frame.sources == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindColumnProject:
		if frame.orientation != OrientationNone || len(frame.siblings) != 1 || frame.children != nil || frame.siblings[0].access.key.Available() || frame.slots == nil || len(frame.slots) == 0 || len(frame.columns) != len(frame.slots) || frame.denominator != (model.DenominatorRef{}) || frame.operation.Available() || frame.destination != (model.RelationID{}) || frame.key.Available() || frame.sources != nil || frame.mapTargets != nil || frame.expandContract.Available() || frame.expandEvidence.Available() {
			return false
		}
		seenCells := make(map[uint32]struct{}, len(frame.slots))
		for index, slot := range frame.slots {
			if !slot.Column().Available() || slot.Column() != frame.columns[index] || slot.Column().Relation() != frame.siblings[0].access.relation {
				return false
			}
			if _, duplicate := seenCells[slot.Cell()]; duplicate {
				return false
			}
			seenCells[slot.Cell()] = struct{}{}
		}
		return sameColumns(frame.siblings[0].access.columns, frame.columns)
	case algebra.KindProject:
		if frame.orientation != OrientationNone || !frame.destination.Available() || !frame.key.Available() || len(frame.siblings) < 2 || frame.children != nil || len(frame.columns) == 0 || frame.mapTargets == nil || len(frame.mapTargets) == 0 || frame.denominator != (model.DenominatorRef{}) || frame.operation.Available() || frame.slots != nil || frame.sources != nil || frame.siblings[0].access.key.Available() || frame.siblings[1].access.key != frame.key || frame.siblings[1].access.relation != frame.destination || frame.siblings[0].access.relation != frame.destination || !sameColumns(frame.siblings[0].access.columns, frame.columns) || frame.expandContract.Available() || frame.expandEvidence.Available() {
			return false
		}
		seenTargets := make(map[model.ColumnID]struct{}, len(frame.mapTargets))
		for _, target := range frame.mapTargets {
			if !target.Available() || target.Relation() != frame.destination {
				return false
			}
			if _, duplicate := seenTargets[target]; duplicate {
				return false
			}
			seenTargets[target] = struct{}{}
		}
		for _, sibling := range frame.siblings[2:] {
			if sibling.access.key.Available() {
				return false
			}
		}
		return true
	case algebra.KindApply:
		return frame.orientation == OrientationNone && frame.operation.Available() && frame.sources != nil && len(frame.sources) != 0 && len(frame.siblings) != 0 && frame.children == nil && frame.denominator == (model.DenominatorRef{}) && !frame.destination.Available() && !frame.key.Available() && frame.columns == nil && frame.slots == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindPublish:
		return frame.orientation == OrientationNone && frame.destination.Available() && frame.key.Available() && len(frame.columns) != 0 && len(frame.siblings) == 3 && frame.children == nil && frame.siblings[0].access.relation == frame.destination && !frame.siblings[0].access.key.Available() && len(frame.siblings[0].access.columns) == 0 && frame.siblings[1].access.relation == frame.destination && frame.siblings[1].access.key == frame.key && frame.siblings[2].access.relation == frame.destination && !frame.siblings[2].access.key.Available() && sameColumns(frame.siblings[2].access.columns, frame.columns) && frame.denominator == (model.DenominatorRef{}) && !frame.operation.Available() && frame.slots == nil && frame.sources == nil && frame.mapTargets == nil && !frame.expandContract.Available() && !frame.expandEvidence.Available()
	case algebra.KindExpand:
		if frame.orientation != OrientationNone || !frame.expandContract.Available() || !frame.expandEvidence.Available() || len(frame.siblings) != 3 || frame.children != nil || frame.denominator != (model.DenominatorRef{}) || frame.operation.Available() || frame.destination != (model.RelationID{}) || frame.key.Available() || frame.cardinality.Available() || frame.columns != nil || frame.slots != nil || frame.sources != nil || frame.mapTargets != nil {
			return false
		}
		candidate, reader, key := frame.siblings[0], frame.siblings[1], frame.siblings[2]
		return candidate.access.relation == frame.expandContract.Candidate() && !candidate.access.key.Available() && reader.access.relation == frame.expandContract.Reader() && !reader.access.key.Available() && key.access.relation == frame.expandContract.Reader() && key.access.key.Available() && key.access.key.Relation() == frame.expandContract.Reader() && sameColumns(key.access.columns, nil)
	default:
		return false
	}
}

func (frame Frame) Kind() algebra.Kind       { return frame.kind }
func (frame Frame) Orientation() Orientation { return frame.orientation }

// Node returns the exact logical node that issued this parent context.
// Runtime uses the corresponding mount execution directory for O(1) binding
// redemption; a zero digest cannot be promoted by callers.
func (frame Frame) Node() identity.ContentID {
	if !frame.Available() {
		return identity.ContentID{}
	}
	return frame.node
}
func (frame Frame) Ordinal() uint32                   { return frame.ordinal }
func (frame Frame) Scope() model.ScopeID              { return frame.scope }
func (frame Frame) Denominator() model.DenominatorRef { return frame.denominator }
func (frame Frame) Operation() signature.Identity     { return frame.operation }
func (frame Frame) Destination() model.RelationID     { return frame.destination }
func (frame Frame) Key() model.KeyID                  { return frame.key }
func (frame Frame) Cardinality() model.Cardinality    { return frame.cardinality }
func (frame Frame) CompleteReplay() CompleteReplay {
	if !frame.Available() || frame.kind != algebra.KindComplete || !frame.completeReplay.Available() {
		return CompleteReplay{}
	}
	return frame.completeReplay
}
func (frame Frame) Columns() []model.ColumnID { return append([]model.ColumnID(nil), frame.columns...) }
func (frame Frame) Slots() []algebra.ColumnSlot {
	return append([]algebra.ColumnSlot(nil), frame.slots...)
}
func (frame Frame) SlotSources() []algebra.SlotSource {
	return append([]algebra.SlotSource(nil), frame.sources...)
}
func (frame Frame) MappingTargets() []model.ColumnID {
	return append([]model.ColumnID(nil), frame.mapTargets...)
}

// ExpandContract returns the sealed logical C/P/R contract carried by an
// Expand frame. It is unavailable for every other frame kind.
func (frame Frame) ExpandContract() model.ExpandContract {
	if !frame.Available() || frame.kind != algebra.KindExpand {
		return model.ExpandContract{}
	}
	return frame.expandContract
}

// ExpandEvidence returns the mount-issued evidence digest sealed into an
// Expand frame. Evidence contents remain owned by arrangement/expand.
func (frame Frame) ExpandEvidence() identity.ContentID {
	if !frame.Available() || frame.kind != algebra.KindExpand {
		return identity.ContentID{}
	}
	return frame.expandEvidence
}

func (frame Frame) SiblingCount() int {
	if !frame.Available() {
		return 0
	}
	return len(frame.siblings)
}

func (frame Frame) SiblingAt(index int) (SiblingAccess, bool) {
	if !frame.Available() || index < 0 || index >= len(frame.siblings) {
		return SiblingAccess{}, false
	}
	return SiblingAccess{value: frame.siblings[index]}, true
}

// ExpandReaderTrigger is the sealed R-change source for one Expand zipper.
// It names the exact Expand node, the concrete candidate Input path that can
// be replayed from a C RowID, and the Expand frame position in that path. The
// reader access is copied from the frame's mount-selected sibling; no runtime
// relation lookup or fabricated Input is possible from this value.
type ExpandReaderTrigger struct {
	node   identity.ContentID
	path   uint32
	frame  uint32
	reader sealedAccess
	replay ExpandReplay
}

func newExpandReaderTrigger(node identity.ContentID, path, frame uint32, reader sealedAccess, replay ExpandReplay) (ExpandReaderTrigger, bool) {
	if !node.Available() || !reader.available() || reader.access.key.Available() || reader.access.columns == nil || len(reader.access.columns) == 0 || !replay.Available() {
		return ExpandReaderTrigger{}, false
	}
	value := ExpandReaderTrigger{node: node, path: path, frame: frame, reader: reader, replay: replay}
	return value, value.Available()
}

func (trigger ExpandReaderTrigger) Available() bool {
	if !trigger.node.Available() || !trigger.reader.available() || trigger.reader.access.key.Available() || trigger.reader.access.columns == nil || len(trigger.reader.access.columns) == 0 || !trigger.reader.access.relation.Available() || !trigger.replay.Available() || trigger.replay.EmitOccurrence() != trigger.replay.Anchor().PathOccurrence() || trigger.path != trigger.replay.Anchor().PathOccurrence() {
		return false
	}
	for index := 0; index < trigger.replay.WatcherCount(); index++ {
		watcher, ok := trigger.replay.WatcherAt(index)
		if !ok || watcher.StopFrame() != trigger.frame || watcher.StopFrameDigest() != trigger.node {
			return false
		}
	}
	return true
}

// Node returns the exact logical Expand node digest.
func (trigger ExpandReaderTrigger) Node() identity.ContentID {
	if !trigger.Available() {
		return identity.ContentID{}
	}
	return trigger.node
}

// PathOccurrence returns the actual candidate Input path occurrence sealed by
// mount for this trigger.
func (trigger ExpandReaderTrigger) PathOccurrence() uint32 {
	if !trigger.Available() {
		return 0
	}
	return trigger.path
}

// FrameOrdinal returns the Expand frame position in the candidate path.
func (trigger ExpandReaderTrigger) FrameOrdinal() uint32 {
	if !trigger.Available() {
		return 0
	}
	return trigger.frame
}

// Reader returns the exact sealed R access used to bind the successor reader.
func (trigger ExpandReaderTrigger) Reader() SiblingAccess {
	if !trigger.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: trigger.reader}
}

// Replay returns the complete fixed-epoch child program sealed for this
// Expand boundary. The replay contains the canonical C anchor and every
// Input watcher below the boundary; it is not an evaluator callback or a
// second logical schema.
func (trigger ExpandReaderTrigger) Replay() ExpandReplay {
	if !trigger.Available() {
		return ExpandReplay{}
	}
	return trigger.replay
}

func (trigger ExpandReaderTrigger) digest() (identity.ContentID, bool) {
	if !trigger.Available() {
		return identity.ContentID{}, false
	}
	if !trigger.replay.Available() {
		return identity.ContentID{}, false
	}
	replayDigest := trigger.replay.Digest()
	if !replayDigest.Available() {
		return identity.ContentID{}, false
	}
	parts := [][]byte{contentBytes(trigger.node), contentBytes(trigger.reader.physical), contentBytes(replayDigest)}
	appendUint32(&parts, trigger.path)
	appendUint32(&parts, trigger.frame)
	return identity.DeriveContentID(pathDigestDomain+"/expand-reader-trigger", parts...)
}

// ChildCount reports the number of authored Merge alternatives carried by the
// frame. It is zero for non-Merge frames. Child vectors are the exact physical
// evidence selected during derivation, not a request to resolve a layout at
// runtime.
func (frame Frame) ChildCount() int {
	if !frame.Available() || frame.kind != algebra.KindMerge {
		return 0
	}
	return len(frame.children)
}

// ChildAt redeems one complete authored Merge child witness in expression
// order. The witness includes its ordinal, child node digest/kind, and exact
// physical access. The active child is included; SiblingAt intentionally
// remains the active-child-omitting view used by ordinary zipper operators.
func (frame Frame) ChildAt(index int) (ChildWitness, bool) {
	if !frame.Available() || frame.kind != algebra.KindMerge || index < 0 || index >= len(frame.children) {
		return ChildWitness{}, false
	}
	return frame.children[index], true
}

// Path is one immutable root-to-leaf derivative for one Input occurrence.
// Frames retain authored parent order: the first frame is the root context
// and the last frame is the Input's immediate parent. This is the canonical
// descent order used to redeem a zipper without rediscovering the tree.
type Path struct {
	root       model.ExpressionID
	occurrence uint32
	// node is the exact Input expression digest at the leaf. It closes the
	// occurrence identity without requiring runtime to walk the root tree.
	node         identity.ContentID
	leafRelation model.RelationID
	readColumns  []model.ColumnID
	leaf         sealedAccess
	frames       []Frame
	digest       identity.ContentID
}

func (path Path) Available() bool {
	if !path.root.Available() || !path.node.Available() || !path.leafRelation.Available() || !path.digest.Available() || path.frames == nil || !path.leaf.available() {
		return false
	}
	if path.leaf.access.relation != path.leafRelation || path.leaf.access.key.Available() || !sameColumns(path.leaf.access.columns, path.readColumns) {
		return false
	}
	for _, frame := range path.frames {
		if !frame.Available() {
			return false
		}
		if replay := frame.CompleteReplay(); replay.Available() && (replay.ParentNode() != frame.node || replay.Occurrence() != path.occurrence) {
			return false
		}
	}
	for _, column := range path.readColumns {
		if !column.Available() || column.Relation() != path.leafRelation {
			return false
		}
	}
	digest, ok := digestPath(path)
	return ok && digest == path.digest
}

func (path Path) Root() model.ExpressionID {
	if !path.Available() {
		return model.ExpressionID{}
	}
	return path.root
}

// Node returns the exact Input expression digest at this path's leaf.
func (path Path) Node() identity.ContentID {
	if !path.Available() {
		return identity.ContentID{}
	}
	return path.node
}
func (path Path) Occurrence() uint32 {
	if !path.Available() {
		return 0
	}
	return path.occurrence
}
func (path Path) LeafRelation() model.RelationID {
	if !path.Available() {
		return model.RelationID{}
	}
	return path.leafRelation
}
func (path Path) ReadColumns() []model.ColumnID {
	if !path.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), path.readColumns...)
}
func (path Path) Leaf() SiblingAccess {
	if !path.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: path.leaf}
}
func (path Path) FrameCount() int {
	if !path.Available() {
		return 0
	}
	return len(path.frames)
}
func (path Path) FrameAt(index int) (Frame, bool) {
	if !path.Available() || index < 0 || index >= len(path.frames) {
		return Frame{}, false
	}
	return path.frames[index], true
}
func (path Path) Digest() identity.ContentID {
	if !path.Available() {
		return identity.ContentID{}
	}
	return path.digest
}

// Plan is the sealed derivative set for one expression root. Path and its
// index lookup are O(1); redeeming a path performs only its bounded zipper
// depth checks. All physical matching happens in Build before sealing.
type Plan struct{ data *planData }

type planData struct {
	root      model.ExpressionID
	paths     []Path
	byPath    map[uint32]int
	triggers  []ExpandReaderTrigger
	byTrigger map[identity.ContentID]int
	digest    identity.ContentID
	sealed    bool
}

func (plan Plan) Available() bool {
	return plan.data != nil && plan.data.sealed && plan.data.root.Available() && plan.data.paths != nil && plan.data.byPath != nil && len(plan.data.paths) == len(plan.data.byPath) && plan.data.triggers != nil && plan.data.byTrigger != nil && len(plan.data.triggers) == len(plan.data.byTrigger) && plan.data.digest.Available()
}

func (plan Plan) Root() model.ExpressionID {
	if !plan.Available() {
		return model.ExpressionID{}
	}
	return plan.data.root
}
func (plan Plan) Digest() identity.ContentID {
	if !plan.Available() {
		return identity.ContentID{}
	}
	return plan.data.digest
}
func (plan Plan) Len() int {
	if !plan.Available() {
		return 0
	}
	return len(plan.data.paths)
}
func (plan Plan) PathAt(index int) (Path, bool) {
	if !plan.Available() || index < 0 || index >= len(plan.data.paths) {
		return Path{}, false
	}
	path := plan.data.paths[index]
	if plan.data.byPath[path.occurrence] != index || !path.Available() {
		return Path{}, false
	}
	return path, true
}
func (plan Plan) Path(occurrence uint32) (Path, bool) {
	if !plan.Available() {
		return Path{}, false
	}
	index, ok := plan.data.byPath[occurrence]
	if !ok {
		return Path{}, false
	}
	return plan.PathAt(index)
}

// ExpandReaderTrigger redeems the one sealed candidate path for an Expand
// node. Build refuses duplicate or missing candidate sources, so this lookup
// is exact and O(1).
func (plan Plan) ExpandReaderTrigger(node identity.ContentID) (ExpandReaderTrigger, bool) {
	if !plan.Available() || !node.Available() {
		return ExpandReaderTrigger{}, false
	}
	index, ok := plan.data.byTrigger[node]
	if !ok || index < 0 || index >= len(plan.data.triggers) {
		return ExpandReaderTrigger{}, false
	}
	trigger := plan.data.triggers[index]
	if !trigger.Available() || trigger.node != node {
		return ExpandReaderTrigger{}, false
	}
	return trigger, true
}

// ExpandReaderTriggers returns the sealed R-change sources in canonical path
// order. It is a defensive projection for mount laws; runtime uses the exact
// node lookup above.
func (plan Plan) ExpandReaderTriggers() []ExpandReaderTrigger {
	if !plan.Available() {
		return nil
	}
	return append([]ExpandReaderTrigger(nil), plan.data.triggers...)
}
