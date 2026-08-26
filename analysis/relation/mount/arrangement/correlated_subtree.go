package arrangement

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// correlationExtentKind is private on purpose. Callers redeem the closed
// source vocabulary only through the typed capability accessors below; a
// public tag would turn the certificate back into a child-shape classifier.
type correlationExtentKind uint8

const (
	correlationExtentInvalid correlationExtentKind = iota
	correlationExtentPopulationDriver
	correlationExtentPartition
	correlationExtentMountedDenominator
)

func (kind correlationExtentKind) available() bool {
	return kind >= correlationExtentPopulationDriver && kind <= correlationExtentMountedDenominator
}

// CorrelationExtentSource is the opaque, sealed source of one Input or
// Complete extent.  It contains no reader, planner, callback, or expression:
// runtime can only redeem the exact source selected at cold mount.
//
// PopulationDriver carries the authored Apply SlotSource whose exact cell is
// read from the population row.  Partition carries the certificate-issued
// Q-row posting directory for the carrier denominator.  MountedDenominator
// carries one exact non-Q source denominator.  There is deliberately no
// relation-name fallback and no Direct/Joined discriminator.
type CorrelationExtentSource struct {
	kind        correlationExtentKind
	driver      Layout
	slot        algebra.SlotSource
	directory   binding.PartitionDirectory
	denominator model.DenominatorRef
	digest      identity.ContentID
	sealed      bool
}

func newPopulationDriverExtent(driver Layout, denominator model.DenominatorRef, slot algebra.SlotSource) (CorrelationExtentSource, bool) {
	if !driver.Available() || !denominator.Available() || driver.Access().Relation() != denominator.Relation() || driver.Access().Key().Available() || driver.CoordinateClass() != CoordinateClassNone {
		return CorrelationExtentSource{}, false
	}
	value := CorrelationExtentSource{kind: correlationExtentPopulationDriver, driver: cloneLayout(driver), slot: slot, denominator: denominator}
	return sealCorrelationExtentSource(value)
}

func newPartitionExtent(directory binding.PartitionDirectory, denominator model.DenominatorRef, fence address.Fence) (CorrelationExtentSource, bool) {
	runtime, runtimeOK := correlatedBindingFenceForAddress(fence)
	if !runtimeOK || !directory.Available() || !directory.ValidFor(runtime) || !denominator.Available() || directory.Child() != denominator {
		return CorrelationExtentSource{}, false
	}
	value := CorrelationExtentSource{kind: correlationExtentPartition, directory: directory, denominator: denominator}
	return sealCorrelationExtentSource(value)
}

func newMountedDenominatorExtent(denominator model.DenominatorRef) (CorrelationExtentSource, bool) {
	if !denominator.Available() {
		return CorrelationExtentSource{}, false
	}
	value := CorrelationExtentSource{kind: correlationExtentMountedDenominator, denominator: denominator}
	return sealCorrelationExtentSource(value)
}

func sealCorrelationExtentSource(value CorrelationExtentSource) (CorrelationExtentSource, bool) {
	if !validCorrelationExtentSource(value) {
		return CorrelationExtentSource{}, false
	}
	parts := correlationExtentSourceDigestParts(value)
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree/extent-source/v1", parts...)
	if !ok {
		return CorrelationExtentSource{}, false
	}
	value.digest, value.sealed = digest, true
	return value, true
}

func validCorrelationExtentSource(value CorrelationExtentSource) bool {
	if !value.kind.available() || !value.denominator.Available() {
		return false
	}
	switch value.kind {
	case correlationExtentPopulationDriver:
		return value.driver.Available() && value.driver.Access().Relation() == value.denominator.Relation() && !value.driver.Access().Key().Available() && value.driver.CoordinateClass() == CoordinateClassNone && !value.directory.Available()
	case correlationExtentPartition:
		return !value.driver.Available() && value.directory.Available() && value.directory.Child() == value.denominator
	case correlationExtentMountedDenominator:
		return !value.driver.Available() && !value.directory.Available()
	default:
		return false
	}
}

func correlationExtentSourceDigestParts(value CorrelationExtentSource) [][]byte {
	parts := [][]byte{[]byte{byte(value.kind)}, denominatorBytes(value.denominator)}
	switch value.kind {
	case correlationExtentPopulationDriver:
		parts = append(parts, contentBytes(value.driver.Digest()), correlatedUint32Bytes(value.slot.Child()), correlatedUint32Bytes(value.slot.Cell()))
	case correlationExtentPartition:
		fence := value.directory.Fence()
		parts = append(parts,
			contentBytes(value.directory.Digest()),
			nominalBytes(fence.Schema().Owner().Content(), fence.Schema().Content()),
			correlatedIdentityMountBytes(fence.Mount()),
			correlatedIdentityGenerationBytes(fence.Generation()),
		)
	}
	return parts
}

// Available is the O(1) redemption of the constructor-issued source seal.
func (value CorrelationExtentSource) Available() bool {
	return value.sealed && value.digest.Available() && value.kind.available() && value.denominator.Available()
}

// PopulationDriver returns the population vector and exact Apply slot only
// for a driver-row source.  The slot is provenance, not an invitation to
// resolve a child by relation name.
func (value CorrelationExtentSource) PopulationDriver() (Layout, algebra.SlotSource, bool) {
	if !value.Available() || value.kind != correlationExtentPopulationDriver {
		return Layout{}, algebra.SlotSource{}, false
	}
	return cloneLayout(value.driver), value.slot, true
}

// Partition returns the one certificate-backed Q posting directory for a
// carrier extent.  It never exposes a relation scan or directory enumeration.
func (value CorrelationExtentSource) Partition() (binding.PartitionDirectory, bool) {
	if !value.Available() || value.kind != correlationExtentPartition {
		return binding.PartitionDirectory{}, false
	}
	return value.directory, true
}

// Denominator returns the exact source population named by this extent.
func (value CorrelationExtentSource) Denominator() (model.DenominatorRef, bool) {
	if !value.Available() {
		return model.DenominatorRef{}, false
	}
	return value.denominator, true
}

func (value CorrelationExtentSource) digestValue() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	return value.digest, true
}

// CorrelatedOccurrence is an exact root-relative execution occurrence.  Node
// digests are useful integrity facts but are intentionally insufficient as an
// occurrence selector: the same mounted node may occur twice under a Join.
// The private path is therefore part of the sealed identity and all subtree
// lookups match both node pointer and path.
type CorrelatedOccurrence struct {
	node   *executionNode
	path   []uint32
	digest identity.ContentID
	sealed bool
}

func newCorrelatedOccurrence(node *executionNode, path []uint32) (CorrelatedOccurrence, bool) {
	if node == nil || !node.digest.Available() || path == nil {
		return CorrelatedOccurrence{}, false
	}
	// Preserve an allocated zero-length root path. A root occurrence is exact
	// and valid; collapsing it to nil would make it indistinguishable from the
	// unavailable-path sentinel used by public lookup accessors.
	copyPath := make([]uint32, len(path))
	copy(copyPath, path)
	parts := make([][]byte, 0, len(copyPath)+1)
	parts = append(parts, contentBytes(node.digest))
	for _, step := range copyPath {
		parts = append(parts, correlatedUint32Bytes(step))
	}
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree/occurrence/v1", parts...)
	if !ok {
		return CorrelatedOccurrence{}, false
	}
	return CorrelatedOccurrence{node: node, path: copyPath, digest: digest, sealed: true}, true
}

func (value CorrelatedOccurrence) Available() bool {
	return value.sealed && value.node != nil && value.node.digest.Available() && value.path != nil && value.digest.Available()
}

func (value CorrelatedOccurrence) Node() Node {
	if !value.Available() {
		return Node{}
	}
	return Node{value: value.node}
}

// Path returns the root-relative child indexes identifying this occurrence.
// It is diagnostic/dispatch provenance only; callers must pair it with Node.
func (value CorrelatedOccurrence) Path() []uint32 {
	if !value.Available() {
		return nil
	}
	result := make([]uint32, len(value.path))
	copy(result, value.path)
	return result
}

func (value CorrelatedOccurrence) Digest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.digest
}

func (value CorrelatedOccurrence) matches(node *executionNode, path []uint32) bool {
	if !value.Available() || node == nil || value.node != node || len(value.path) != len(path) {
		return false
	}
	for index := range path {
		if value.path[index] != path[index] {
			return false
		}
	}
	return true
}

// InputExtent binds exactly one mounted Input occurrence to one closed row
// source.  The source is selected before the subtree is sealed; runtime never
// rediscoveres it by relation, logical digest, or child shape.
type InputExtent struct {
	occurrence CorrelatedOccurrence
	binding    InputBinding
	source     CorrelationExtentSource
	digest     identity.ContentID
	sealed     bool
}

func newInputExtent(occurrence CorrelatedOccurrence, bindingValue InputBinding, source CorrelationExtentSource) (InputExtent, bool) {
	if !occurrence.Available() || occurrence.node.kind != algebra.KindInput || !sameCorrelatedInputBinding(occurrence.node, bindingValue) || !source.Available() {
		return InputExtent{}, false
	}
	denominator, denominatorOK := source.Denominator()
	if !denominatorOK || denominator.Relation() != bindingValue.Relation() {
		return InputExtent{}, false
	}
	parts := [][]byte{contentBytes(occurrence.Digest()), contentBytes(bindingValue.Scan().Digest()), contentBytes(bindingValue.Values().Digest()), contentBytes(bindingValue.range_.Producer())}
	sourceDigest, sourceOK := source.digestValue()
	if !sourceOK {
		return InputExtent{}, false
	}
	parts = append(parts, contentBytes(sourceDigest))
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree/input-extent/v1", parts...)
	if !ok {
		return InputExtent{}, false
	}
	return InputExtent{occurrence: occurrence, binding: bindingValue, source: source, digest: digest, sealed: true}, true
}

func sameCorrelatedInputBinding(node *executionNode, value InputBinding) bool {
	if node == nil || node.kind != algebra.KindInput || !value.Available() || !node.input.Available() || value.Relation() != node.input.Relation() || !value.Scan().Equal(node.input.Scan()) || !value.Values().Equal(node.input.Values()) {
		return false
	}
	rangeValue, rangeOK := value.Range()
	nodeRange, nodeRangeOK := nodeRangeBinding(node)
	return rangeOK && nodeRangeOK && rangeValue.Producer() == nodeRange.Producer() && rangeValue.Kind() == nodeRange.Kind() && rangeValue.Layout().Equal(nodeRange.Layout())
}

func (value InputExtent) Available() bool {
	return value.sealed && value.digest.Available() && value.occurrence.Available() && value.binding.Available() && value.source.Available()
}

func (value InputExtent) Occurrence() CorrelatedOccurrence {
	if !value.Available() {
		return CorrelatedOccurrence{}
	}
	return value.occurrence
}

func (value InputExtent) Node() Node { return value.Occurrence().Node() }

func (value InputExtent) Binding() InputBinding {
	if !value.Available() {
		return InputBinding{}
	}
	return value.binding
}

func (value InputExtent) Source() CorrelationExtentSource {
	if !value.Available() {
		return CorrelationExtentSource{}
	}
	return value.source
}

func (value InputExtent) digestValue() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	return value.digest, true
}

// DenominatorExtent binds exactly one mounted Complete occurrence to its
// closed range source.  It is deliberately separate from InputExtent because
// a joined input's source and its carrier range can be different authorities.
type DenominatorExtent struct {
	occurrence CorrelatedOccurrence
	binding    CompleteBinding
	source     CorrelationExtentSource
	digest     identity.ContentID
	sealed     bool
}

func newDenominatorExtent(occurrence CorrelatedOccurrence, bindingValue CompleteBinding, source CorrelationExtentSource) (DenominatorExtent, bool) {
	if !occurrence.Available() || occurrence.node.kind != algebra.KindComplete || !sameCorrelatedCompleteBinding(occurrence.node, bindingValue) || !source.Available() {
		return DenominatorExtent{}, false
	}
	denominator, denominatorOK := source.Denominator()
	if !denominatorOK || denominator != bindingValue.Denominator() {
		return DenominatorExtent{}, false
	}
	parts := [][]byte{contentBytes(occurrence.Digest()), contentBytes(bindingValue.Key().Digest()), denominatorBytes(bindingValue.Denominator())}
	sourceDigest, sourceOK := source.digestValue()
	if !sourceOK {
		return DenominatorExtent{}, false
	}
	parts = append(parts, contentBytes(sourceDigest))
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree/denominator-extent/v1", parts...)
	if !ok {
		return DenominatorExtent{}, false
	}
	return DenominatorExtent{occurrence: occurrence, binding: bindingValue, source: source, digest: digest, sealed: true}, true
}

func sameCorrelatedCompleteBinding(node *executionNode, value CompleteBinding) bool {
	if node == nil || node.kind != algebra.KindComplete || !value.Available() || !node.complete.Available() || value.Denominator() != node.complete.Denominator() || !value.Key().Equal(node.complete.Key()) {
		return false
	}
	rangeValue, rangeOK := value.Range()
	nodeRange, nodeRangeOK := nodeRangeBinding(node)
	return rangeOK && nodeRangeOK && rangeValue.Producer() == nodeRange.Producer() && rangeValue.Kind() == nodeRange.Kind() && rangeValue.Layout().Equal(nodeRange.Layout()) && rangeValue.Denominator() == nodeRange.Denominator()
}

func (value DenominatorExtent) Available() bool {
	return value.sealed && value.digest.Available() && value.occurrence.Available() && value.binding.Available() && value.source.Available()
}

func (value DenominatorExtent) Occurrence() CorrelatedOccurrence {
	if !value.Available() {
		return CorrelatedOccurrence{}
	}
	return value.occurrence
}

func (value DenominatorExtent) Node() Node { return value.Occurrence().Node() }

func (value DenominatorExtent) Binding() CompleteBinding {
	if !value.Available() {
		return CompleteBinding{}
	}
	return value.binding
}

func (value DenominatorExtent) Source() CorrelationExtentSource {
	if !value.Available() {
		return CorrelationExtentSource{}
	}
	return value.source
}

func (value DenominatorExtent) digestValue() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	return value.digest, true
}

// CorrelatedSubtree is one generic, sealed correlated Apply child.  It owns
// the exact physical root, its empty Select scope, and complete occurrence
// maps for every Input and Complete below that root.  It has no shape tag:
// direct, shared, scalar, and joined cases are all represented solely by the
// extent sources selected by the cold walker.
type CorrelatedSubtree struct {
	ordinal    uint32
	root       *executionNode
	emptyScope SelectBinding
	inputs     []InputExtent
	completes  []DenominatorExtent
	digest     identity.ContentID
	sealed     bool
}

func (value CorrelatedSubtree) Available() bool {
	return value.sealed && value.root != nil && value.root.digest.Available() && value.digest.Available()
}

func (value CorrelatedSubtree) Ordinal() uint32 {
	if !value.Available() {
		return 0
	}
	return value.ordinal
}

func (value CorrelatedSubtree) Root() Node {
	if !value.Available() {
		return Node{}
	}
	return Node{value: value.root}
}

// EmptyScope returns the one Select scope used to represent an authenticated
// empty Complete subtree.  A driver-row scalar leaf has no possible empty
// child range, so it deliberately returns false rather than minting a scope.
func (value CorrelatedSubtree) EmptyScope() (SelectBinding, bool) {
	if !value.Available() || !value.emptyScope.Available() {
		return SelectBinding{}, false
	}
	return value.emptyScope, true
}

func (value CorrelatedSubtree) InputCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.inputs)
}

func (value CorrelatedSubtree) InputAt(index int) (InputExtent, bool) {
	if !value.Available() || index < 0 || index >= len(value.inputs) || !value.inputs[index].Available() {
		return InputExtent{}, false
	}
	return value.inputs[index], true
}

// InputFor redeems an exact Input extent only when both physical node and
// root-relative path agree.  In particular, a repeated same-relation Input
// or an equal node digest cannot select a sibling extent by itself.
func (value CorrelatedSubtree) InputFor(node Node, path []uint32) (InputExtent, bool) {
	if !value.Available() || !node.Available() || path == nil {
		return InputExtent{}, false
	}
	for _, candidate := range value.inputs {
		if candidate.Available() && candidate.occurrence.matches(node.value, path) {
			return candidate, true
		}
	}
	return InputExtent{}, false
}

func (value CorrelatedSubtree) CompleteCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.completes)
}

func (value CorrelatedSubtree) CompleteAt(index int) (DenominatorExtent, bool) {
	if !value.Available() || index < 0 || index >= len(value.completes) || !value.completes[index].Available() {
		return DenominatorExtent{}, false
	}
	return value.completes[index], true
}

// CompleteFor is the Complete counterpart of InputFor.  It never resolves a
// denominator by relation name or structural digest alone.
func (value CorrelatedSubtree) CompleteFor(node Node, path []uint32) (DenominatorExtent, bool) {
	if !value.Available() || !node.Available() || path == nil {
		return DenominatorExtent{}, false
	}
	for _, candidate := range value.completes {
		if candidate.Available() && candidate.occurrence.matches(node.value, path) {
			return candidate, true
		}
	}
	return DenominatorExtent{}, false
}

func (value CorrelatedSubtree) Digest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.digest
}

// carrierDirectory is intentionally package-private: the generic evaluator
// resolves extents through Source(), while this narrow helper keeps legacy
// replay callers from reopening a relation directory during transition.
func (value CorrelatedSubtree) carrierDirectory() (binding.PartitionDirectory, bool) {
	for _, candidate := range value.completes {
		if !candidate.Available() {
			continue
		}
		if directory, ok := candidate.source.Partition(); ok {
			return directory, true
		}
	}
	return binding.PartitionDirectory{}, false
}

type correlatedInputRecord struct {
	occurrence CorrelatedOccurrence
	binding    InputBinding
}

type correlatedCompleteRecord struct {
	occurrence CorrelatedOccurrence
	binding    CompleteBinding
}

type correlatedCellProvenance struct {
	input int
	cell  uint32
}

type correlatedWalk struct {
	inputs    []correlatedInputRecord
	completes []correlatedCompleteRecord
	selects   []SelectBinding
	output    []correlatedCellProvenance
}

// sealCorrelatedSubtree is the one cold walker for every correlated child.
// It follows the oriented Complete/Select/Join-left range spine only to find
// the carrier; all remaining leaves are resolved from exact Apply cell
// provenance.  No runtime callback or planner survives this function.
func sealCorrelatedSubtree(
	ordinal uint32,
	root *executionNode,
	correlation algebra.ApplyCorrelation,
	driver Layout,
	slots []algebra.SlotSource,
	deliveries []DeliveryBinding,
	partition certificate.CorrelationPartition,
	directory binding.PartitionDirectory,
) (CorrelatedSubtree, bool) {
	if root == nil || !root.digest.Available() || !correlation.Available() || !driver.Available() || len(slots) != len(deliveries) {
		return CorrelatedSubtree{}, false
	}
	projection, projectionOK := correlation.ProjectionAt(int(ordinal))
	if !projectionOK || len(projection) > 1 {
		return CorrelatedSubtree{}, false
	}
	walk, walkOK := walkCorrelatedSubtree(root)
	if !walkOK || len(walk.inputs) == 0 {
		return CorrelatedSubtree{}, false
	}

	carrierInput, carrierComplete, spineScope, completeRoot := correlatedCarrierSpine(root, walk)
	driverInput := correlatedDriverInput(root, walk, driver, correlation)
	partitioned := len(projection) == 1
	if partitioned {
		if driverInput >= 0 {
			// A driver row is a scalar leaf. It deliberately has no Q posting
			// directory and cannot also claim a Complete carrier range.
			if partition.Available() || directory.Available() || carrierComplete >= 0 {
				return CorrelatedSubtree{}, false
			}
		} else {
			if !completeRoot || carrierInput < 0 || carrierComplete < 0 || !partition.Available() || !directory.Available() {
				return CorrelatedSubtree{}, false
			}
			carrier := walk.completes[carrierComplete].binding.Denominator()
			runtime, runtimeOK := correlatedBindingFenceForAddress(driver.Handle().Fence())
			if !runtimeOK || partition.Population() != correlation.Population() || partition.Child() != carrier || partition.Projection() != projection[0] || directory.Seal() != partition.Digest() || !directory.ValidFor(runtime) || directory.Population() != correlation.Population() || directory.Child() != carrier {
				return CorrelatedSubtree{}, false
			}
		}
	} else {
		if partition.Available() || directory.Available() || !completeRoot || carrierInput < 0 || carrierComplete < 0 {
			return CorrelatedSubtree{}, false
		}
	}
	if completeRoot {
		if !spineScope.Available() || len(walk.selects) != 1 {
			// One exact Select scope is the only authenticated empty cofiber
			// anchor. Multiple scopes would make empty result selection
			// ambiguous; no scope is inferred from a denominator or key.
			return CorrelatedSubtree{}, false
		}
	} else if len(walk.selects) != 0 || len(walk.completes) != 0 {
		return CorrelatedSubtree{}, false
	}

	candidates, candidatesOK := correlatedSourceCandidates(walk, ordinal, slots, deliveries)
	if !candidatesOK {
		return CorrelatedSubtree{}, false
	}

	inputExtents := make([]InputExtent, len(walk.inputs))
	carrierDenominator := model.DenominatorRef{}
	if carrierComplete >= 0 {
		carrierDenominator = walk.completes[carrierComplete].binding.Denominator()
	}
	for index, input := range walk.inputs {
		var source CorrelationExtentSource
		var sourceOK bool
		switch {
		case index == driverInput:
			slot, scalarOK := correlatedDriverSlot(candidates[index], correlation, input.binding, ordinal)
			if !scalarOK {
				return CorrelatedSubtree{}, false
			}
			source, sourceOK = newPopulationDriverExtent(driver, correlation.Population(), slot)
		case partitioned && index == carrierInput:
			if !allCandidateDenominators(candidates[index], carrierDenominator) {
				return CorrelatedSubtree{}, false
			}
			source, sourceOK = newPartitionExtent(directory, carrierDenominator, driver.Handle().Fence())
		case !partitioned && index == carrierInput:
			if !allCandidateDenominators(candidates[index], carrierDenominator) {
				return CorrelatedSubtree{}, false
			}
			source, sourceOK = newMountedDenominatorExtent(carrierDenominator)
		default:
			denominator, denominatorOK := uniqueCandidateDenominator(candidates[index])
			if !denominatorOK || (partitioned && denominator == carrierDenominator) {
				// A non-carrier leaf cannot silently consume a Q-local carrier
				// range through a global mounted witness.
				return CorrelatedSubtree{}, false
			}
			source, sourceOK = newMountedDenominatorExtent(denominator)
		}
		if !sourceOK {
			return CorrelatedSubtree{}, false
		}
		inputExtents[index], sourceOK = newInputExtent(input.occurrence, input.binding, source)
		if !sourceOK {
			return CorrelatedSubtree{}, false
		}
	}

	completeExtents := make([]DenominatorExtent, len(walk.completes))
	for index, complete := range walk.completes {
		denominator := complete.binding.Denominator()
		var source CorrelationExtentSource
		var sourceOK bool
		// The certificate partition authorizes only the one exact Complete
		// occurrence selected by the oriented carrier spine. A nested or
		// sibling Complete over the same denominator remains a mounted source;
		// matching a denominator or a path prefix would broaden Q authority.
		if partitioned && index == carrierComplete && denominator == carrierDenominator {
			source, sourceOK = newPartitionExtent(directory, denominator, driver.Handle().Fence())
		} else {
			source, sourceOK = newMountedDenominatorExtent(denominator)
		}
		if !sourceOK {
			return CorrelatedSubtree{}, false
		}
		completeExtents[index], sourceOK = newDenominatorExtent(complete.occurrence, complete.binding, source)
		if !sourceOK {
			return CorrelatedSubtree{}, false
		}
	}

	value := CorrelatedSubtree{ordinal: ordinal, root: root, emptyScope: spineScope, inputs: inputExtents, completes: completeExtents}
	if !validCorrelatedSubtree(value) {
		return CorrelatedSubtree{}, false
	}
	parts := correlatedSubtreeDigestParts(value)
	digest, digestOK := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree/v1", parts...)
	if !digestOK {
		return CorrelatedSubtree{}, false
	}
	value.digest, value.sealed = digest, true
	return value, true
}

func walkCorrelatedSubtree(root *executionNode) (correlatedWalk, bool) {
	if root == nil {
		return correlatedWalk{}, false
	}
	value := correlatedWalk{}
	stack := make(map[*executionNode]bool)
	var walk func(*executionNode, []uint32) ([]correlatedCellProvenance, bool)
	walk = func(node *executionNode, path []uint32) ([]correlatedCellProvenance, bool) {
		if node == nil || !node.digest.Available() || stack[node] {
			return nil, false
		}
		stack[node] = true
		defer delete(stack, node)
		switch node.kind {
		case algebra.KindInput:
			if len(node.children) != 0 || !node.input.Available() {
				return nil, false
			}
			occurrence, occurrenceOK := newCorrelatedOccurrence(node, path)
			if !occurrenceOK {
				return nil, false
			}
			bindingValue := node.input
			bindingValue.range_, _ = nodeRangeBinding(node)
			if !bindingValue.Available() {
				return nil, false
			}
			index := len(value.inputs)
			value.inputs = append(value.inputs, correlatedInputRecord{occurrence: occurrence, binding: bindingValue})
			columns := bindingValue.Values().Columns()
			output := make([]correlatedCellProvenance, len(columns))
			for cell := range columns {
				output[cell] = correlatedCellProvenance{input: index, cell: uint32(cell)}
			}
			return output, true
		case algebra.KindSelect:
			if len(node.children) != 1 || !node.select_.Available() {
				return nil, false
			}
			value.selects = append(value.selects, node.select_)
			return walk(node.children[0], appendCorrelatedPath(path, 0))
		case algebra.KindJoin:
			if len(node.children) != 2 || !node.join.Available() {
				return nil, false
			}
			left, leftOK := walk(node.children[0], appendCorrelatedPath(path, 0))
			if !leftOK {
				return nil, false
			}
			right, rightOK := walk(node.children[1], appendCorrelatedPath(path, 1))
			if !rightOK {
				return nil, false
			}
			return append(left, right...), true
		case algebra.KindComplete:
			if len(node.children) != 1 || !node.complete.Available() {
				return nil, false
			}
			occurrence, occurrenceOK := newCorrelatedOccurrence(node, path)
			if !occurrenceOK {
				return nil, false
			}
			bindingValue := node.complete
			bindingValue.range_, _ = nodeRangeBinding(node)
			if !bindingValue.Available() {
				return nil, false
			}
			value.completes = append(value.completes, correlatedCompleteRecord{occurrence: occurrence, binding: bindingValue})
			return walk(node.children[0], appendCorrelatedPath(path, 0))
		default:
			// The generic evaluator intentionally begins with the physical
			// vocabulary it can execute without planning: Input, Select, Join,
			// Complete. A future operator must add its own sealed extent law.
			return nil, false
		}
	}
	output, ok := walk(root, []uint32{})
	if !ok {
		return correlatedWalk{}, false
	}
	value.output = output
	return value, true
}

func appendCorrelatedPath(path []uint32, step uint32) []uint32 {
	result := make([]uint32, len(path)+1)
	copy(result, path)
	result[len(path)] = step
	return result
}

// correlatedCarrierSpine follows the only range-preserving orientation. It
// never searches by relation or logical digest: Complete and Select preserve
// their only child, while Join preserves its left child. The returned indexes
// name records in the full occurrence walk, not a relation-keyed cache.
func correlatedCarrierSpine(root *executionNode, walk correlatedWalk) (inputIndex, completeIndex int, scope SelectBinding, completeRoot bool) {
	inputIndex, completeIndex = -1, -1
	if root == nil || root.kind != algebra.KindComplete {
		return inputIndex, completeIndex, SelectBinding{}, false
	}
	completeRoot = true
	node, path := root, []uint32{}
	for {
		switch node.kind {
		case algebra.KindComplete:
			for index, candidate := range walk.completes {
				if candidate.occurrence.matches(node, path) {
					if completeIndex < 0 {
						completeIndex = index
					}
					break
				}
			}
			if len(node.children) != 1 {
				return -1, -1, SelectBinding{}, false
			}
			node, path = node.children[0], appendCorrelatedPath(path, 0)
		case algebra.KindSelect:
			if scope.Available() || len(node.children) != 1 || !node.select_.Available() {
				return -1, -1, SelectBinding{}, false
			}
			scope = node.select_
			node, path = node.children[0], appendCorrelatedPath(path, 0)
		case algebra.KindJoin:
			if len(node.children) != 2 || !node.join.Available() {
				return -1, -1, SelectBinding{}, false
			}
			node, path = node.children[0], appendCorrelatedPath(path, 0)
		case algebra.KindInput:
			for index, candidate := range walk.inputs {
				if candidate.occurrence.matches(node, path) {
					return index, completeIndex, scope, true
				}
			}
			return -1, -1, SelectBinding{}, false
		default:
			return -1, -1, SelectBinding{}, false
		}
	}
}

// correlatedCarrierDenominator is the cold partition join point. It follows
// the oriented spine, never a relation name, and returns only the exact
// Complete carrier which certificate.CorrelationPartition is allowed to name.
func correlatedCarrierDenominator(root *executionNode) (model.DenominatorRef, bool) {
	walk, ok := walkCorrelatedSubtree(root)
	if !ok {
		return model.DenominatorRef{}, false
	}
	_, complete, _, completeRoot := correlatedCarrierSpine(root, walk)
	if !completeRoot || complete < 0 || complete >= len(walk.completes) {
		return model.DenominatorRef{}, false
	}
	denominator := walk.completes[complete].binding.Denominator()
	return denominator, denominator.Available()
}

func correlatedDriverInput(root *executionNode, walk correlatedWalk, driver Layout, correlation algebra.ApplyCorrelation) int {
	if root == nil || root.kind != algebra.KindInput || len(walk.inputs) != 1 || !root.input.Available() || !driver.Available() || !correlation.Available() || root.input.Relation() != driver.Access().Relation() || root.input.Relation() != correlation.Population().Relation() {
		return -1
	}
	// A direct population child may intentionally project only the correlation
	// coordinate (for example Q.site-id) even though the mounted population
	// driver is the full authored row vector. Driver identity is therefore
	// relation/coordinate authority, not equality of the two payload vectors;
	// correlatedDriverSlot performs the exact semantic slot check below.
	if !containsColumn(root.input.Values().Columns(), correlation.Coordinate()) {
		return -1
	}
	if !walk.inputs[0].occurrence.matches(root, []uint32{}) {
		return -1
	}
	return 0
}

type correlatedSourceCandidate struct {
	denominator model.DenominatorRef
	slot        algebra.SlotSource
	input       model.ColumnID
	delivery    DeliveryBinding
}

func correlatedSourceCandidates(walk correlatedWalk, ordinal uint32, slots []algebra.SlotSource, deliveries []DeliveryBinding) ([][]correlatedSourceCandidate, bool) {
	result := make([][]correlatedSourceCandidate, len(walk.inputs))
	if len(slots) != len(deliveries) {
		return nil, false
	}
	for index, slot := range slots {
		if slot.Child() != ordinal {
			continue
		}
		if index >= len(deliveries) || !deliveries[index].Available() || deliveries[index].Requirement().Index() != uint32(index) || int(slot.Cell()) >= len(walk.output) {
			return nil, false
		}
		provenance := walk.output[slot.Cell()]
		if provenance.input < 0 || provenance.input >= len(walk.inputs) {
			return nil, false
		}
		bindingValue := walk.inputs[provenance.input].binding
		columns := bindingValue.Values().Columns()
		semantic := deliveries[index].Requirement().Input()
		source, sourceOK := semantic.SourceDenominator()
		if !sourceOK || !semantic.Available() || int(provenance.cell) >= len(columns) || columns[provenance.cell] != semantic.Column || bindingValue.Relation() != semantic.Relation || source.Relation() != bindingValue.Relation() {
			return nil, false
		}
		result[provenance.input] = append(result[provenance.input], correlatedSourceCandidate{denominator: source, slot: slot, input: semantic.Column, delivery: deliveries[index]})
	}
	return result, true
}

func correlatedDriverSlot(candidates []correlatedSourceCandidate, correlation algebra.ApplyCorrelation, input InputBinding, ordinal uint32) (algebra.SlotSource, bool) {
	if len(candidates) != 1 || !input.Available() {
		return algebra.SlotSource{}, false
	}
	candidate := candidates[0]
	semantic := candidate.delivery.Requirement().Input()
	columns := input.Values().Columns()
	if candidate.slot.Child() != ordinal || !semantic.Delivery.IsScalar() || candidate.denominator != correlation.Population() || semantic.Relation != correlation.Population().Relation() || semantic.Column != correlation.Coordinate() || semantic.Type != correlation.Type() || int(candidate.slot.Cell()) >= len(columns) || columns[candidate.slot.Cell()] != correlation.Coordinate() {
		return algebra.SlotSource{}, false
	}
	return candidate.slot, true
}

func allCandidateDenominators(candidates []correlatedSourceCandidate, wanted model.DenominatorRef) bool {
	if len(candidates) == 0 || !wanted.Available() {
		return false
	}
	for _, candidate := range candidates {
		if candidate.denominator != wanted {
			return false
		}
	}
	return true
}

func uniqueCandidateDenominator(candidates []correlatedSourceCandidate) (model.DenominatorRef, bool) {
	if len(candidates) == 0 {
		return model.DenominatorRef{}, false
	}
	value := candidates[0].denominator
	if !value.Available() {
		return model.DenominatorRef{}, false
	}
	for _, candidate := range candidates[1:] {
		if candidate.denominator != value {
			return model.DenominatorRef{}, false
		}
	}
	return value, true
}

func validCorrelatedSubtree(value CorrelatedSubtree) bool {
	if value.root == nil || !value.root.digest.Available() || value.inputs == nil || len(value.inputs) == 0 || value.completes == nil {
		return false
	}
	walk, walkOK := walkCorrelatedSubtree(value.root)
	if !walkOK || len(walk.inputs) != len(value.inputs) || len(walk.completes) != len(value.completes) {
		return false
	}
	for index, input := range value.inputs {
		if !input.Available() || !input.occurrence.matches(walk.inputs[index].occurrence.node, walk.inputs[index].occurrence.path) || !sameCorrelatedInputBinding(walk.inputs[index].occurrence.node, input.binding) {
			return false
		}
	}
	for index, complete := range value.completes {
		if !complete.Available() || !complete.occurrence.matches(walk.completes[index].occurrence.node, walk.completes[index].occurrence.path) || !sameCorrelatedCompleteBinding(walk.completes[index].occurrence.node, complete.binding) {
			return false
		}
	}
	carrierInput, carrierComplete, scope, completeRoot := correlatedCarrierSpine(value.root, walk)
	if completeRoot {
		if carrierInput < 0 || carrierComplete < 0 || !scope.Available() || !value.emptyScope.Available() || value.emptyScope.Scope() != scope.Scope() || len(walk.selects) != 1 {
			return false
		}
	}
	// The direct-driver case has no Complete and no empty scope. Avoid a
	// synthetic scope merely to make the standard shape look uniform.
	if !completeRoot {
		if len(walk.completes) != 0 || value.emptyScope.Available() {
			return false
		}
		foundDriver := false
		for _, input := range value.inputs {
			if _, _, driver := input.source.PopulationDriver(); driver {
				if foundDriver {
					return false
				}
				foundDriver = true
			}
		}
		if !foundDriver {
			return false
		}
	}
	return true
}

func correlatedSubtreeDigestParts(value CorrelatedSubtree) [][]byte {
	parts := [][]byte{contentBytes(value.root.digest), correlatedUint32Bytes(value.ordinal)}
	if value.emptyScope.Available() {
		scope := value.emptyScope.Scope()
		parts = append(parts, []byte{1}, nominalBytes(scope.Owner().Content(), scope.Content()))
	} else {
		parts = append(parts, []byte{0})
	}
	for _, input := range value.inputs {
		digest, ok := input.digestValue()
		if !ok {
			return nil
		}
		parts = append(parts, contentBytes(digest))
	}
	for _, complete := range value.completes {
		digest, ok := complete.digestValue()
		if !ok {
			return nil
		}
		parts = append(parts, contentBytes(digest))
	}
	return parts
}

func denominatorBytes(value model.DenominatorRef) []byte {
	if !value.Available() {
		return nil
	}
	result := nominalBytes(value.Relation().Owner().Content(), value.Relation().Content())
	return append(result, nominalBytes(value.Key().Relation().Owner().Content(), value.Key().Content())...)
}

func correlatedUint32Bytes(value uint32) []byte {
	encoded := make([]byte, 4)
	binary.BigEndian.PutUint32(encoded, value)
	return encoded
}

func correlatedBindingFenceForAddress(fence address.Fence) (binding.Fence, bool) {
	if !fence.Available() {
		return binding.Fence{}, false
	}
	return binding.NewFence(fence.SchemaID(), fence.MountID(), fence.Generation())
}

func correlatedIdentityMountBytes(value identity.MountID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func correlatedIdentityGenerationBytes(value identity.Generation) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint64(result, uint64(value))
	return result
}
