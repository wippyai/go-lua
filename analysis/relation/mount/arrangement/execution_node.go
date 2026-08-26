package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Node is one opaque physical node in a sealed expression tree. Its public
// accessors expose only checked kind, children, contracts, and resolved
// layouts; callers cannot recover a logical expression or request another
// arrangement.
type Node struct{ value *executionNode }

type executionNode struct {
	kind     algebra.Kind
	children []*executionNode
	// cells is the canonical schema-level output cell coordinate for this
	// sealed physical node. It is issued during Derive from the already-bound
	// operator contracts; evaluators redeem SlotSource against it and never
	// recalculate Complete extensions or repair ordinals at runtime.
	cells algebra.CellLayout

	input         InputBinding
	select_       SelectBinding
	project       ProjectBinding
	columnProject ColumnProjectBinding
	expand        ExpandBinding
	join          JoinBinding
	merge         MergeBinding
	group         GroupBinding
	complete      CompleteBinding
	apply         ApplyBinding
	publish       PublishBinding

	logical identity.ContentID
	digest  identity.ContentID
}

// RangeBinding authenticates the sealed producer contract carried by a tuple
// range. It is issued by a sealed Node and retains the producer node itself
// rather than a caller-created range identifier or a hash of runtime values.
// The physical key layout (and, for Complete, its denominator) therefore
// remains the exact context selected during arrangement derivation. It does
// not by itself prove that a runtime tuple vector is complete: the producing
// operator and its differential laws own that semantic proof.
type RangeBinding struct {
	producer    *executionNode
	digest      identity.ContentID
	kind        algebra.Kind
	layout      Layout
	denominator model.DenominatorRef
}

func nodeRangeBinding(node *executionNode) (RangeBinding, bool) {
	if node == nil || !node.digest.Available() {
		return RangeBinding{}, false
	}
	var value RangeBinding
	switch node.kind {
	case algebra.KindInput:
		if !node.input.Available() {
			return RangeBinding{}, false
		}
		// Input ranges are relation-cofiber authority.  They deliberately use
		// the zero-column scan layout rather than the delivered row vector: a
		// payload projection changes what a row contains, never which relation
		// range the Input producer owns.
		value = RangeBinding{producer: node, digest: node.digest, kind: node.kind, layout: node.input.scan}
	case algebra.KindGroup:
		if !node.group.Available() {
			return RangeBinding{}, false
		}
		value = RangeBinding{producer: node, digest: node.digest, kind: node.kind, layout: node.group.key}
	case algebra.KindMerge:
		if !node.merge.Available() {
			return RangeBinding{}, false
		}
		value = RangeBinding{producer: node, digest: node.digest, kind: node.kind, layout: node.merge.key}
	case algebra.KindComplete:
		if !node.complete.Available() {
			return RangeBinding{}, false
		}
		value = RangeBinding{producer: node, digest: node.digest, kind: node.kind, layout: node.complete.key, denominator: node.complete.denominator}
	default:
		return RangeBinding{}, false
	}
	return value, value.Available()
}

// Available reports whether this is an owner-issued range capability. A zero
// RangeBinding cannot be made usable by copying public coordinates: the
// producer pointer, sealed node digest, and exact producer contract must
// agree. It authenticates the contract, not a claimed runtime cardinality.
func (binding RangeBinding) Available() bool {
	if binding.producer == nil || !binding.digest.Available() || binding.producer.digest != binding.digest || binding.producer.kind != binding.kind || !rangeLayoutAvailable(binding.layout) {
		return false
	}
	switch binding.kind {
	case algebra.KindInput:
		return binding.denominator == (model.DenominatorRef{}) && rangeLayoutAvailable(binding.producer.input.scan) && binding.producer.input.relation.Available() && binding.layout.Digest() == binding.producer.input.scan.Digest()
	case algebra.KindGroup:
		return binding.denominator == (model.DenominatorRef{}) && rangeLayoutAvailable(binding.producer.group.key) && binding.producer.group.cardinality.Available() && binding.layout.Digest() == binding.producer.group.key.Digest()
	case algebra.KindMerge:
		return binding.denominator == (model.DenominatorRef{}) && rangeLayoutAvailable(binding.producer.merge.key) && binding.layout.Digest() == binding.producer.merge.key.Digest()
	case algebra.KindComplete:
		return binding.producer.complete.Available() && rangeLayoutAvailable(binding.producer.complete.key) && binding.denominator == binding.producer.complete.denominator && binding.layout.Digest() == binding.producer.complete.key.Digest()
	default:
		return false
	}
}

// Producer returns the sealed producer node digest. It is provenance for a
// batch range, never a free-standing range identity.
func (binding RangeBinding) Producer() identity.ContentID {
	if !binding.Available() {
		return identity.ContentID{}
	}
	return binding.digest
}

// Kind returns the sealed node kind that issued this capability.
func (binding RangeBinding) Kind() algebra.Kind {
	if !binding.Available() {
		return algebra.KindInvalid
	}
	return binding.kind
}

// Layout returns the exact mounted layout selected for this producer.
func (binding RangeBinding) Layout() Layout {
	if !binding.Available() {
		return Layout{}
	}
	return cloneLayout(binding.layout)
}

// ValidFor redeems the node-owned range against one exact mount fence without
// copying its key vector. This is the O(1) hot-boundary check used by tuple
// Batch.ValidFor.
func (binding RangeBinding) ValidFor(fence address.Fence) bool {
	return binding.Available() && binding.layout.Handle().ValidFor(fence)
}

// rangeLayoutAvailable is the sealed-layout check used by range capabilities
// after Node issuance. Layout construction already proved the logical access
// vector and digest; redeeming that immutable witness therefore needs only
// its constant-size digest and generation-fenced handle.
func rangeLayoutAvailable(layout Layout) bool {
	handle := layout.Handle()
	return layout.Digest().Available() && handle.Available() && handle.ValidFor(handle.Fence())
}

// Denominator returns the declared denominator context. It is non-zero only
// for Complete ranges.
func (binding RangeBinding) Denominator() model.DenominatorRef {
	if !binding.Available() {
		return model.DenominatorRef{}
	}
	return binding.denominator
}

// InputBinding is the exact mounted input plan for one Input occurrence.
// Scan is the zero-column relation cofiber used to authenticate Input's range;
// Values is the occurrence's sealed source projection. The two layouts are
// intentionally separate: payload selection must not create a new range
// identity, and range ownership must not widen the declared row contract.
type InputBinding struct {
	relation model.RelationID
	scan     Layout
	values   Layout
	range_   RangeBinding
	// sealed is issued once after the exact scan/value projection has been
	// checked.  Runtime availability must redeem this scalar rather than
	// clone either logical access vector on every probe.
	sealed bool
}

func (binding InputBinding) Available() bool {
	if !binding.sealed || !binding.relation.Available() || !binding.scan.Available() || !binding.values.Available() {
		return false
	}
	scan := binding.scan.access
	values := binding.values.access
	return scan.relation == binding.relation && !scan.key.Available() && len(scan.columns) == 0 &&
		values.relation == binding.relation && !values.key.Available()
}
func (binding InputBinding) Relation() model.RelationID { return binding.relation }

// Scan returns the sealed relation-wide cofiber layout.  It is the only
// layout from which Input may redeem a range capability.
func (binding InputBinding) Scan() Layout { return cloneLayout(binding.scan) }

// Values returns the sealed occurrence projection. Operators bind a Reader to
// this layout; they must never inspect a relation schema at runtime to widen
// it or reconstruct a different vector.
func (binding InputBinding) Values() Layout { return cloneLayout(binding.values) }

func (binding InputBinding) Range() (RangeBinding, bool) {
	if !binding.Available() || !binding.range_.Available() || binding.range_.Kind() != algebra.KindInput {
		return RangeBinding{}, false
	}
	return binding.range_, true
}

// SelectBinding is the generation-fenced address of the nominal scope used
// by one Select. Scope materialization remains a mount/runtime capability;
// the binding proves the declaration was resolved by this exact mount.
type SelectBinding struct {
	scope address.Address[model.ScopeID]
}

func (binding SelectBinding) Available() bool {
	return binding.scope.Available() && binding.scope.ID().Available()
}
func (binding SelectBinding) ValidFor(fence address.Fence) bool {
	return binding.Available() && binding.scope.ValidFor(fence)
}
func (binding SelectBinding) Scope() model.ScopeID { return binding.scope.ID() }

// ProjectionMapping binds one authored source/target correspondence to the
// exact source vector layout selected at mount.
type ProjectionMapping struct {
	source model.ColumnID
	target model.ColumnID
	layout Layout
}

func (binding ProjectionMapping) Available() bool {
	if !binding.source.Available() || !binding.target.Available() || !binding.layout.Available() || binding.layout.Access().Relation() != binding.source.Relation() {
		return false
	}
	for _, column := range binding.layout.Columns() {
		if column == binding.source {
			return true
		}
	}
	return false
}
func (binding ProjectionMapping) Source() model.ColumnID { return binding.source }
func (binding ProjectionMapping) Target() model.ColumnID { return binding.target }
func (binding ProjectionMapping) Layout() Layout         { return cloneLayout(binding.layout) }

// ProjectBinding preserves authored mapping order and exact target/key
// layouts. Source layouts may repeat when mappings share a vector layout.
type ProjectBinding struct {
	target   Layout
	key      Layout
	mappings []ProjectionMapping
	// keyOrder is the sealed target-key to authored-mapping correspondence.
	// It contains indexes into mappings rather than a second copy of the
	// mapping records.  Mount proves completeness, uniqueness, and order once;
	// the evaluator only redeems these indexes.
	keyOrder []uint32
	sealed   bool
}

func (binding ProjectBinding) Available() bool {
	// All width-sensitive and uniqueness checks are performed by
	// sealProjectBinding during arrangement derivation.  This predicate is a
	// constant-time redemption check; calling it from MappingAt must not
	// allocate or rebuild a target index on the evaluator hot path.
	return binding.sealed
}

// sealProjectBinding performs the cold proof for the project correspondence.
// It is deliberately package-private: only arrangement derivation can issue
// the sealed binding consumed by the engine.
func sealProjectBinding(binding ProjectBinding) (ProjectBinding, bool) {
	if !binding.target.Available() || !binding.key.Available() || binding.target.Access().Relation() != binding.key.Access().Relation() || binding.target.Access().Key().Available() || len(binding.target.Columns()) == 0 || !binding.key.Access().Key().Available() || binding.mappings == nil || len(binding.mappings) == 0 || binding.keyOrder == nil {
		return ProjectBinding{}, false
	}
	targetColumns := make(map[model.ColumnID]struct{}, len(binding.target.Columns()))
	for _, column := range binding.target.Columns() {
		if !column.Available() || column.Relation() != binding.target.Access().Relation() {
			return ProjectBinding{}, false
		}
		if _, duplicate := targetColumns[column]; duplicate {
			return ProjectBinding{}, false
		}
		targetColumns[column] = struct{}{}
	}
	seenTargets := make(map[model.ColumnID]struct{}, len(binding.mappings))
	keyColumns := binding.key.KeyColumns()
	keyTargets := make(map[model.ColumnID]struct{}, len(keyColumns))
	for _, column := range keyColumns {
		keyTargets[column] = struct{}{}
	}
	for _, mapping := range binding.mappings {
		if !mapping.Available() || mapping.target.Relation() != binding.target.Access().Relation() {
			return ProjectBinding{}, false
		}
		if _, targetOK := targetColumns[mapping.target]; !targetOK {
			return ProjectBinding{}, false
		}
		if _, duplicate := seenTargets[mapping.target]; duplicate {
			return ProjectBinding{}, false
		}
		if _, keyTarget := keyTargets[mapping.target]; keyTarget && !sameColumnIDs(mapping.layout.KeyColumns(), mapping.layout.Columns()) {
			// A Project target key is redeemed by exact source correspondence;
			// accepting an ordinary unkeyed source vector would force the
			// evaluator back to a relation scan.
			return ProjectBinding{}, false
		}
		seenTargets[mapping.target] = struct{}{}
	}
	if len(binding.keyOrder) != len(keyColumns) {
		return ProjectBinding{}, false
	}
	seenIndexes := make(map[uint32]struct{}, len(binding.keyOrder))
	for index, mappingIndex := range binding.keyOrder {
		if int(mappingIndex) >= len(binding.mappings) {
			return ProjectBinding{}, false
		}
		if _, duplicate := seenIndexes[mappingIndex]; duplicate {
			return ProjectBinding{}, false
		}
		seenIndexes[mappingIndex] = struct{}{}
		if binding.mappings[mappingIndex].target != keyColumns[index] {
			return ProjectBinding{}, false
		}
		if _, targetOK := targetColumns[keyColumns[index]]; !targetOK {
			return ProjectBinding{}, false
		}
	}
	binding.sealed = true
	return binding, true
}
func (binding ProjectBinding) Target() Layout { return cloneLayout(binding.target) }
func (binding ProjectBinding) Key() Layout    { return cloneLayout(binding.key) }
func (binding ProjectBinding) Mappings() []ProjectionMapping {
	return append([]ProjectionMapping(nil), binding.mappings...)
}

// MappingCount reports the sealed authored projection width without exposing
// the backing slice. Runtime operators use MappingAt to redeem the vector
// without allocating a defensive copy on every invocation.
func (binding ProjectBinding) MappingCount() int {
	if !binding.Available() {
		return 0
	}
	return len(binding.mappings)
}

// MappingAt redeems one entry from the sealed authored target order.
func (binding ProjectBinding) MappingAt(index int) (ProjectionMapping, bool) {
	if !binding.Available() || index < 0 || index >= len(binding.mappings) {
		return ProjectionMapping{}, false
	}
	return binding.mappings[index], true
}

// KeyMappingCount reports the sealed key projection width.
func (binding ProjectBinding) KeyMappingCount() int {
	if !binding.Available() {
		return 0
	}
	return len(binding.keyOrder)
}

// KeyMappingAt redeems the mapping for one key component in the exact key
// vector order. The index indirection is sealed at mount; no runtime lookup
// table or target-column completeness check is needed.
func (binding ProjectBinding) KeyMappingAt(index int) (ProjectionMapping, bool) {
	if !binding.Available() || index < 0 || index >= len(binding.keyOrder) {
		return ProjectionMapping{}, false
	}
	mappingIndex := binding.keyOrder[index]
	if int(mappingIndex) >= len(binding.mappings) {
		return ProjectionMapping{}, false
	}
	return binding.mappings[mappingIndex], true
}

// ColumnProjectBinding seals a positional subset of one child row. Values is
// the exact mounted output vector; Slots preserve the corresponding child
// cell ordinals so the evaluator never searches a tuple by nominal column.
type ColumnProjectBinding struct {
	values Layout
	slots  []algebra.ColumnSlot
}

// ExpandBinding is the complete mounted physical contract for one dependent
// key-vector expansion. The evidence is already tokenized at admission;
// runtime receives no owner callback, ordinal, or coordinate conversion.
type ExpandBinding struct {
	contract  model.ExpandContract
	scope     address.Address[model.ScopeID]
	candidate Layout
	reader    Layout
	key       Layout
	evidence  expand.Evidence
	columns   []model.ColumnID
}

func (binding ExpandBinding) Available() bool {
	if !binding.contract.Available() || !binding.scope.Available() || !binding.candidate.Available() || !binding.reader.Available() || !binding.key.Available() || !binding.evidence.Available() || binding.evidence.Contract() != binding.contract || len(binding.columns) == 0 {
		return false
	}
	if binding.candidate.Access().Relation() != binding.contract.Candidate() || binding.reader.Access().Relation() != binding.contract.Reader() || binding.key.Access().Relation() != binding.contract.Reader() {
		return false
	}
	if !binding.key.Access().Key().Available() || !containsColumn(binding.key.KeyColumns(), binding.contract.Key()) {
		return false
	}
	candidateColumns := binding.candidate.Columns()
	readerColumns := binding.reader.Columns()
	if !sameRelationColumns(binding.columns, candidateColumns, readerColumns) {
		return false
	}
	return true
}

// ValidFor redeems the exact mounted join-right scope address. Expand's scope
// is not inferred from a child Select and is never reconstructed at runtime.
func (binding ExpandBinding) ValidFor(fence address.Fence) bool {
	return binding.Available() && binding.scope.ValidFor(fence)
}

func (binding ExpandBinding) Contract() model.ExpandContract {
	if !binding.Available() {
		return model.ExpandContract{}
	}
	return binding.contract
}
func (binding ExpandBinding) Scope() model.ScopeID {
	if !binding.Available() {
		return model.ScopeID{}
	}
	return binding.scope.ID()
}
func (binding ExpandBinding) Candidate() Layout {
	if !binding.Available() {
		return Layout{}
	}
	return cloneLayout(binding.candidate)
}
func (binding ExpandBinding) Reader() Layout {
	if !binding.Available() {
		return Layout{}
	}
	return cloneLayout(binding.reader)
}
func (binding ExpandBinding) Key() Layout {
	if !binding.Available() {
		return Layout{}
	}
	return cloneLayout(binding.key)
}
func (binding ExpandBinding) Evidence() expand.Evidence {
	if !binding.Available() {
		return expand.Evidence{}
	}
	return binding.evidence
}
func (binding ExpandBinding) Columns() []model.ColumnID {
	if !binding.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), binding.columns...)
}

func containsColumn(columns []model.ColumnID, wanted model.ColumnID) bool {
	for _, column := range columns {
		if column == wanted {
			return true
		}
	}
	return false
}

func sameRelationColumns(columns, candidate, reader []model.ColumnID) bool {
	if len(candidate) == 0 || len(reader) == 0 || len(columns) != len(candidate)+len(reader) {
		return false
	}
	for index, column := range candidate {
		if !column.Available() || columns[index] != column {
			return false
		}
	}
	for index, column := range reader {
		if !column.Available() || columns[len(candidate)+index] != column {
			return false
		}
	}
	return true
}

func (binding ColumnProjectBinding) Available() bool {
	if !binding.values.Available() || binding.slots == nil || len(binding.slots) == 0 {
		return false
	}
	columns := binding.values.Columns()
	if len(columns) != len(binding.slots) {
		return false
	}
	seenColumns := make(map[model.ColumnID]struct{}, len(columns))
	seenCells := make(map[uint32]struct{}, len(columns))
	for index, slot := range binding.slots {
		if !slot.Column().Available() || slot.Column() != columns[index] || slot.Column().Relation() != binding.values.Access().Relation() {
			return false
		}
		if _, duplicate := seenColumns[slot.Column()]; duplicate {
			return false
		}
		if _, duplicate := seenCells[slot.Cell()]; duplicate {
			return false
		}
		seenColumns[slot.Column()] = struct{}{}
		seenCells[slot.Cell()] = struct{}{}
	}
	return true
}

// Values returns the exact sealed output vector.
func (binding ColumnProjectBinding) Values() Layout { return cloneLayout(binding.values) }

// SlotCount returns the sealed projected width without allocating a copy.
func (binding ColumnProjectBinding) SlotCount() int {
	if !binding.Available() {
		return 0
	}
	return len(binding.slots)
}

// SlotAt redeems one exact child-cell selection in authored output order.
func (binding ColumnProjectBinding) SlotAt(index int) (algebra.ColumnSlot, bool) {
	if !binding.Available() || index < 0 || index >= len(binding.slots) {
		return algebra.ColumnSlot{}, false
	}
	return binding.slots[index], true
}

// JoinBinding names the two oriented vector layouts used by one equijoin.
type JoinBinding struct{ left, right Layout }

// NewJoinBinding seals the two already-mounted correspondence vectors.  The
// layouts are the only physical coordinate authority: no caller can supply a
// second key list or a runtime resolver.  Derive uses this same constructor,
// and tuple-native operators redeem the resulting value directly.
func NewJoinBinding(left, right Layout) (JoinBinding, bool) {
	value := JoinBinding{left: left, right: right}
	return value, value.Available()
}

func (binding JoinBinding) Available() bool {
	if !binding.left.Available() || !binding.right.Available() || !binding.left.Handle().Fence().Same(binding.right.Handle().Fence()) {
		return false
	}
	// Join correspondence is an exact vector access. These are the delivered
	// source columns sealed by bindJoin, not a relation key projection.
	leftAccess, rightAccess := binding.left.Access(), binding.right.Access()
	if leftAccess.Key().Available() || rightAccess.Key().Available() {
		return false
	}
	leftColumns, rightColumns := binding.left.Columns(), binding.right.Columns()
	if len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
		return false
	}
	// The correspondence vectors are redeemed through Reader.Lookup, so the
	// physical trie coordinate must be exactly the authored vector on both
	// sides. A plain unkeyed vector is not a usable Join binding even when its
	// logical columns happen to have the right arity.
	if !sameColumnIDs(binding.left.KeyColumns(), leftColumns) || !sameColumnIDs(binding.right.KeyColumns(), rightColumns) {
		return false
	}
	for index, leftColumn := range leftColumns {
		rightColumn := rightColumns[index]
		if !leftColumn.Available() || !rightColumn.Available() || leftColumn.Relation() != binding.left.Access().Relation() || rightColumn.Relation() != binding.right.Access().Relation() {
			return false
		}
	}
	return true
}

func sameColumnIDs(left, right []model.ColumnID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (binding JoinBinding) Left() Layout  { return cloneLayout(binding.left) }
func (binding JoinBinding) Right() Layout { return cloneLayout(binding.right) }

type MergeBinding struct {
	key       Layout
	range_    RangeBinding
	proposals []ProposalWitness
}

// ProposalWitness binds one direct Merge child occurrence to one Apply
// authority preserved by that child's closed sidecar path. Child is the
// existing physical node identity, which already commits the Apply output
// address and all intervening operators.
type ProposalWitness struct {
	child     identity.ContentID
	operation signature.Identity
}

func (witness ProposalWitness) Available() bool {
	return witness.child.Available() && witness.operation.Available()
}
func (witness ProposalWitness) Child() identity.ContentID     { return witness.child }
func (witness ProposalWitness) Operation() signature.Identity { return witness.operation }

func (binding MergeBinding) Available() bool {
	if !binding.key.Available() || !binding.key.Access().Key().Available() {
		return false
	}
	for _, proposal := range binding.proposals {
		if !proposal.Available() {
			return false
		}
	}
	return true
}
func (binding MergeBinding) Key() Layout { return cloneLayout(binding.key) }

// ProposalOperations returns the already-sealed Apply authorities whose
// result sidecars this Merge combines. An empty result means ordinary tuple
// reduction. The operation identities are references to existing Apply
// bindings, not a second execution-mode registry.
func (binding MergeBinding) ProposalOperations() []signature.Identity {
	if !binding.Available() || len(binding.proposals) == 0 {
		return nil
	}
	result := make([]signature.Identity, len(binding.proposals))
	for index, proposal := range binding.proposals {
		result[index] = proposal.operation
	}
	return result
}

func (binding MergeBinding) AcceptsProposal(child identity.ContentID, operation signature.Identity) bool {
	if !binding.Available() || !child.Available() || !operation.Available() {
		return false
	}
	for _, candidate := range binding.proposals {
		if candidate.child == child && candidate.operation == operation {
			return true
		}
	}
	return false
}
func (binding MergeBinding) Range() (RangeBinding, bool) {
	if !binding.Available() || !binding.range_.Available() || binding.range_.Kind() != algebra.KindMerge {
		return RangeBinding{}, false
	}
	return binding.range_, true
}

type GroupBinding struct {
	key         Layout
	cardinality model.Cardinality
	range_      RangeBinding
}

func (binding GroupBinding) Available() bool {
	return binding.key.Available() && binding.key.Access().Key().Available() && binding.cardinality.Available()
}
func (binding GroupBinding) Key() Layout                    { return cloneLayout(binding.key) }
func (binding GroupBinding) Cardinality() model.Cardinality { return binding.cardinality }
func (binding GroupBinding) Range() (RangeBinding, bool) {
	if !binding.Available() || !binding.range_.Available() || binding.range_.Kind() != algebra.KindGroup {
		return RangeBinding{}, false
	}
	return binding.range_, true
}

type CompleteBinding struct {
	denominator model.DenominatorRef
	key         Layout
	columns     []model.ColumnID
	range_      RangeBinding
	// sealed is set only by newCompleteBinding after the denominator, key
	// layout, and complete relation vector have been authenticated together.
	sealed bool
}

// newCompleteBinding authenticates and copies the complete relation vector
// once. No unavailable value is issued with sealed=true, so runtime probes
// can redeem the owner verdict without rebuilding a duplicate set.
func newCompleteBinding(denominator model.DenominatorRef, key Layout, columns []model.ColumnID) (CompleteBinding, bool) {
	if !denominator.Available() || !key.Available() || key.access.relation != denominator.Relation() || key.access.key != denominator.Key() || len(columns) == 0 {
		return CompleteBinding{}, false
	}
	copyOf := append([]model.ColumnID(nil), columns...)
	seen := make(map[model.ColumnID]struct{}, len(copyOf))
	for _, column := range copyOf {
		if !column.Available() || column.Relation() != denominator.Relation() {
			return CompleteBinding{}, false
		}
		if _, duplicate := seen[column]; duplicate {
			return CompleteBinding{}, false
		}
		seen[column] = struct{}{}
	}
	return CompleteBinding{denominator: denominator, key: key, columns: copyOf, sealed: true}, true
}

// Available reports the constructor's sealed complete-binding verdict and
// scalar denominator/key header. The complete row vector is validated once at
// construction; this probe never rebuilds its duplicate set.
func (binding CompleteBinding) Available() bool {
	return binding.sealed && binding.denominator.Available() && binding.key.Available() && binding.key.access.relation == binding.denominator.Relation() && binding.key.access.key == binding.denominator.Key() && len(binding.columns) != 0
}
func (binding CompleteBinding) Denominator() model.DenominatorRef { return binding.denominator }
func (binding CompleteBinding) Key() Layout                       { return cloneLayout(binding.key) }

// Columns returns the exact certificate-declared row vector for the closed
// denominator relation, in its typed relation-contract order. It is mounted
// once by Derive and is never reconstructed from runtime state.
func (binding CompleteBinding) Columns() []model.ColumnID {
	if !binding.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), binding.columns...)
}
func (binding CompleteBinding) Range() (RangeBinding, bool) {
	if !binding.Available() || !binding.range_.Available() || binding.range_.Kind() != algebra.KindComplete {
		return RangeBinding{}, false
	}
	return binding.range_, true
}

// DeliveryBinding is one sealed Apply input associated with exact mounted
// data and (when applicable) its order layout. It reuses signature.Input.
type DeliveryBinding struct {
	requirement DeliveryRequirement
	layout      Layout
	order       Layout
}

func (binding DeliveryBinding) Available() bool {
	if !binding.requirement.Available() || !binding.layout.Available() {
		return false
	}
	access, ok := binding.requirement.Access()
	if !ok || !binding.layout.Access().Equal(access) {
		return false
	}
	if binding.requirement.Delivery().IsSpan() {
		return binding.order.Available() && binding.order.Access().Key() == binding.requirement.Delivery().OrderKey()
	}
	return !binding.order.Available()
}
func (binding DeliveryBinding) Requirement() DeliveryRequirement { return binding.requirement }
func (binding DeliveryBinding) Layout() Layout                   { return cloneLayout(binding.layout) }
func (binding DeliveryBinding) Order() (Layout, bool) {
	if !binding.Available() || !binding.requirement.Delivery().IsSpan() {
		return Layout{}, false
	}
	return cloneLayout(binding.order), true
}

type ApplyBinding struct {
	operation  signature.Identity
	deliveries []DeliveryBinding
	slotSource []algebra.SlotSource
	// cells is Apply's exact semantic output row, sealed from the registered
	// signature while mount still owns the declaration catalogue. Keeping it
	// here lets a nested Apply contribute one canonical child layout without
	// reopening a signature registry at runtime.
	cells       algebra.CellLayout
	output      algebra.OutputAddress
	outputSlot  int
	childCount  uint32
	correlation algebra.ApplyCorrelation
	replay      ApplyReplay
}

func (binding ApplyBinding) Available() bool {
	if !binding.operation.Available() || !binding.cells.Available() || !binding.output.Available() || binding.deliveries == nil || len(binding.slotSource) != len(binding.deliveries) {
		return false
	}
	if binding.output.IsOwnerNamed() {
		if binding.outputSlot != -1 {
			return false
		}
	} else if binding.outputSlot < 0 || binding.outputSlot >= len(binding.deliveries) {
		return false
	} else {
		input := binding.deliveries[binding.outputSlot].Requirement().Input()
		if binding.output.IsScalarSource() && !input.Delivery.IsScalar() {
			return false
		}
		if binding.output.IsSpanSource() && !input.Delivery.IsComplete() {
			return false
		}
	}
	if binding.correlation.Specified() {
		if !binding.correlation.Available() || binding.correlation.ProjectionCount() != int(binding.childCount) || !binding.replay.Available() || !binding.replay.Correlation().Equal(binding.correlation) || binding.replay.ChildCount() != int(binding.childCount) {
			return false
		}
		for index := 0; index < binding.replay.ChildCount(); index++ {
			child, childOK := binding.replay.ChildAt(index)
			projection, projectionOK := binding.correlation.ProjectionAt(index)
			if !childOK || !projectionOK || len(projection) > 1 || child.Ordinal() != uint32(index) || !child.Root().Available() {
				return false
			}
			driverSource, driverChild := child.driverSource()
			directory, partitioned := child.carrierDirectory()
			if driverChild {
				_, slot, sourceOK := driverSource.PopulationDriver()
				if !sourceOK || len(projection) != 1 || partitioned || directory.Available() || slot.Child() != uint32(index) {
					return false
				}
				continue
			}
			if _, scopeOK := child.EmptyScope(); !scopeOK {
				return false
			}
			if len(projection) == 0 {
				if partitioned || directory.Available() {
					return false
				}
				continue
			}
			if !partitioned || !directory.Available() || directory.Population() != binding.correlation.Population() {
				return false
			}
		}
	} else if binding.replay.Available() {
		return false
	}
	// A childless binding is retained only for a certificate-checked,
	// direct-Publish zero-input seed. It has no tuple delivery surface;
	// Execute still refuses it because it cannot derive scope or lineage from
	// relational children.
	if len(binding.deliveries) == 0 {
		return len(binding.slotSource) == 0 && binding.childCount == 0
	}
	if binding.childCount == 0 {
		return false
	}
	for index, delivery := range binding.deliveries {
		if !delivery.Available() || delivery.requirement.Operation() != binding.operation || int(delivery.requirement.Index()) != index {
			return false
		}
		if binding.slotSource[index].Child() >= binding.childCount {
			return false
		}
	}
	for expected := uint32(0); expected < binding.childCount; expected++ {
		found := false
		for _, group := range binding.slotSource {
			if group.Child() == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (binding ApplyBinding) Operation() signature.Identity { return binding.operation }
func (binding ApplyBinding) Deliveries() []DeliveryBinding {
	return append([]DeliveryBinding(nil), binding.deliveries...)
}

// SlotSource returns the sealed positional tuple source for each semantic
// input slot.
func (binding ApplyBinding) SlotSource() []algebra.SlotSource {
	return append([]algebra.SlotSource(nil), binding.slotSource...)
}

// OutputCells returns the exact semantic row layout emitted by this Apply.
// It is mounted declaration data, not a runtime signature lookup.
func (binding ApplyBinding) OutputCells() algebra.CellLayout {
	if !binding.Available() {
		return algebra.CellLayout{}
	}
	return binding.cells
}

// Correlation returns the sealed heterogeneous Apply declaration. The zero
// value is the ordinary independent-child form.
func (binding ApplyBinding) Correlation() algebra.ApplyCorrelation {
	if !binding.Available() || !binding.correlation.Specified() {
		return algebra.ApplyCorrelation{}
	}
	return binding.correlation
}

// OutputAddress returns the exact plan-owned destination geometry redeemed at
// mount. Generated semantic bindings do not carry or restate this value.
func (binding ApplyBinding) OutputAddress() algebra.OutputAddress {
	if !binding.Available() {
		return algebra.OutputAddress{}
	}
	return binding.output
}

// DestinationSlot returns the operation input slot whose mounted frame is the
// destination source. OwnerNamed has no slot and returns false.
func (binding ApplyBinding) DestinationSlot() (int, bool) {
	if !binding.Available() || binding.output.IsOwnerNamed() {
		return 0, false
	}
	return binding.outputSlot, true
}

// Replay returns the complete immutable child replay witness. It is the only
// physical coordinate authority for correlated Apply execution.
func (binding ApplyBinding) Replay() (ApplyReplay, bool) {
	if !binding.Available() || !binding.correlation.Specified() || !binding.replay.Available() {
		return ApplyReplay{}, false
	}
	return binding.replay, true
}

// ChildCount is the number of ordered child nodes required by Execute.
func (binding ApplyBinding) ChildCount() int {
	if binding.childCount == 0 {
		return 0
	}
	return int(binding.childCount)
}

type PublishBinding struct {
	destination Layout
	key         Layout
	columns     Layout
}

func (binding PublishBinding) Available() bool {
	return binding.destination.Available() && binding.key.Available() && binding.columns.Available() && binding.destination.Access().Relation() == binding.key.Access().Relation() && binding.destination.Access().Relation() == binding.columns.Access().Relation() && binding.key.Access().Key().Available() && len(binding.columns.Columns()) != 0
}
func (binding PublishBinding) Destination() Layout { return cloneLayout(binding.destination) }
func (binding PublishBinding) Key() Layout         { return cloneLayout(binding.key) }

// Columns returns the exact writable child/output vector committed by Publish.
func (binding PublishBinding) Columns() Layout { return cloneLayout(binding.columns) }

func (node Node) Available() bool {
	return node.value != nil && node.value.digest.Available() && node.value.cells.Available()
}
func (node Node) Kind() algebra.Kind {
	if !node.Available() {
		return algebra.KindInvalid
	}
	return node.value.kind
}
func (node Node) Digest() identity.ContentID {
	if !node.Available() {
		return identity.ContentID{}
	}
	return node.value.digest
}

// CellLayout returns the sealed physical output coordinate for this node.
// It is useful to cold consumers which must validate a SlotSource; runtime
// operators only redeem the immutable ordinals already carried by bindings.
func (node Node) CellLayout() algebra.CellLayout {
	if !node.Available() {
		return algebra.CellLayout{}
	}
	return node.value.cells
}
func (node Node) Children() []Node {
	if !node.Available() {
		return nil
	}
	result := make([]Node, len(node.value.children))
	for index, child := range node.value.children {
		result[index] = Node{value: child}
	}
	return result
}

// Range issues the producer-owned range authority for nodes whose declared
// semantics establish a range. Input, Group, Merge, and Complete are the
// range producers; Select and Project preserve a child's authority and Join
// preserves its oriented left range. Apply/Publish carry no tuple range.
func (node Node) Range() (RangeBinding, bool) {
	if !node.Available() {
		return RangeBinding{}, false
	}
	return nodeRangeBinding(node.value)
}

func (node Node) Input() (InputBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindInput || !node.value.input.Available() {
		return InputBinding{}, false
	}
	binding := node.value.input
	binding.range_, _ = node.Range()
	return binding, true
}
func (node Node) Select() (SelectBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindSelect || !node.value.select_.Available() {
		return SelectBinding{}, false
	}
	return node.value.select_, true
}
func (node Node) Project() (ProjectBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindProject || !node.value.project.Available() {
		return ProjectBinding{}, false
	}
	return node.value.project, true
}
func (node Node) ColumnProject() (ColumnProjectBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindColumnProject || !node.value.columnProject.Available() {
		return ColumnProjectBinding{}, false
	}
	return node.value.columnProject, true
}
func (node Node) Expand() (ExpandBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindExpand || !node.value.expand.Available() {
		return ExpandBinding{}, false
	}
	return node.value.expand, true
}
func (node Node) Join() (JoinBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindJoin || !node.value.join.Available() {
		return JoinBinding{}, false
	}
	return node.value.join, true
}
func (node Node) Merge() (MergeBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindMerge || !node.value.merge.Available() {
		return MergeBinding{}, false
	}
	binding := node.value.merge
	binding.range_, _ = node.Range()
	return binding, true
}
func (node Node) Group() (GroupBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindGroup || !node.value.group.Available() {
		return GroupBinding{}, false
	}
	binding := node.value.group
	binding.range_, _ = node.Range()
	return binding, true
}
func (node Node) Complete() (CompleteBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindComplete || !node.value.complete.Available() {
		return CompleteBinding{}, false
	}
	binding := node.value.complete
	binding.range_, _ = node.Range()
	return binding, true
}
func (node Node) Apply() (ApplyBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindApply || !node.value.apply.Available() {
		return ApplyBinding{}, false
	}
	return node.value.apply, true
}
func (node Node) Publish() (PublishBinding, bool) {
	if !node.Available() || node.value.kind != algebra.KindPublish || !node.value.publish.Available() {
		return PublishBinding{}, false
	}
	return node.value.publish, true
}
