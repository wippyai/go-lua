package pack

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/static"
)

// Root and Values are Pack-private selectors over direct Program topology.
// Every root is one pack-valued occurrence: a Program Values expression, an
// ordinary-call boundary, or an open-tail producer.  In particular,
// two calls in one body never share a Pack factor coordinate.
type Root struct {
	schema *schema
	index  uint32
}
type Values struct {
	schema *schema
	index  uint32
}

func (root Root) valid() bool {
	return root.schema != nil && uint64(root.index) < uint64(len(root.schema.relations))
}
func (values Values) valid() bool {
	if values.schema == nil || uint64(values.index) >= uint64(len(values.schema.values)) {
		return false
	}
	row := values.schema.values[values.index]
	if !row.moduleKey.Available() || !row.occurrenceID.Available() || uint64(row.root) >= uint64(len(values.schema.roots)) {
		return false
	}
	root := values.schema.roots[row.root]
	if root.kind != rootValues || root.sourceIndex != values.index || !root.port.valid() || root.port.owner != values.schema.owner || root.id != mountedArtifactRootID(rootValues, row.moduleKey, row.occurrenceID) {
		return false
	}
	_, tailOK := values.schema.tailPort(row.tailRoot)
	return row.hasTail && tailOK || !row.hasTail && row.tailRoot == 0
}

func (state *schema) tailPort(rootIndex uint32) (Port, bool) {
	if state == nil || uint64(rootIndex) >= uint64(len(state.roots)) {
		return Port{}, false
	}
	root := state.roots[rootIndex]
	if root.kind != rootTail || !root.port.valid() || root.port.owner != state.owner || uint64(root.sourceIndex) >= uint64(len(state.tails)) || state.tails[root.sourceIndex].root != rootIndex {
		return Port{}, false
	}
	return root.port, true
}

type rootKind uint8

const (
	rootInvalid rootKind = iota
	rootValues
	rootCall
	rootTail
)

type rootRow struct {
	kind        rootKind
	port        Port
	sourceIndex uint32
	id          identity.ContentID
}

type valuesRow struct {
	root         uint32
	moduleKey    identity.ContentID
	occurrenceID identity.ContentID
	fixed        []Endpoint
	tailRoot     uint32
	hasTail      bool
}

type inputSelectorKey struct {
	operation vocabulary.Operation
	source    vocabulary.InputSource
}

type callRow struct {
	root          uint32
	occurrenceID  identity.ContentID
	moduleKey     identity.ContentID
	formalID      identity.ContentID
	typeArguments static.TypeArgumentSequence
	tailRoot      uint32
	hasTail       bool
	fixed         []Endpoint
}

type tailRow struct {
	root      uint32
	moduleKey identity.ContentID
	valueID   identity.ContentID
	kind      TailProducerKind
}

type schema struct {
	linkOwner link.OwnerCapability
	owner     *algebra

	roots     []rootRow
	relations []*relation
	values    []valuesRow
	calls     []callRow
	tails     []tailRow
	// sourceValues is the sealed Pack value column for each source-producing
	// root. Source and Root remain the canonical descriptors in roots; this
	// column stores the Value directly and never wraps it in a transport object.
	sourceValues []Value

	// endpointSources is Pack-owned replay data.  The old Boundary values
	// were a retained Link -> Project -> Program reachability edge; they are
	// intentionally absent after this seal boundary.
	endpointSources []SemanticSource
	endpointIndex   map[SemanticSource]Endpoint
	artifactValues  map[artifactValuesKey]uint32
	artifactCalls   map[artifactCallKey]uint32
	// inputSelectors is the sealed Target-ABI projection consumed by Effect.
	// It contains only Pack-owned interpretation templates: neither Target nor
	// any Link/Boundary handle survives construction.
	inputSelectors map[inputSelectorKey]InputSelector
}

type Schema struct{ state *schema }

// LinkOwner returns the exact detached Link owner witness admitted by this
// schema. The capability is the non-forgeable fence used when composing
// independently sealed schemas.
func (schema *Schema) LinkOwner() link.OwnerCapability {
	if schema == nil || schema.state == nil {
		return link.OwnerCapability{}
	}
	return schema.state.linkOwner
}

// Owner is the concise alias used by cross-domain binders.
func (schema *Schema) Owner() link.OwnerCapability { return schema.LinkOwner() }

func (schema *Schema) RootCount() int {
	if schema == nil || schema.state == nil {
		return 0
	}
	return len(schema.state.roots)
}
func (schema *Schema) RootAt(index int) (Root, bool) {
	if schema == nil || schema.state == nil || index < 0 || index >= len(schema.state.roots) {
		return Root{}, false
	}
	root := Root{schema.state, uint32(index)}
	return root, root.valid()
}

// RootOrder returns the sealed Factor-coordinate order of one Pack root.
// Summary reads canonicalize their dense keys by this order; a transformation
// which also has authored order can therefore carry only this small Pack
// permutation, never an engine coordinate or copied source row.
func (schema *Schema) RootOrder(root Root) (int, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return 0, false
	}
	return int(root.index), true
}

// RootID is the canonical cold identity for a Pack source/root descriptor.
func (schema *Schema) RootID(root Root) (identity.ContentID, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return identity.ContentID{}, false
	}
	id := schema.state.roots[root.index].id
	return id, id.Available()
}

func (schema *Schema) Port(values Values) (Port, bool) {
	if schema == nil || schema.state == nil || !values.valid() || values.schema != schema.state {
		return Port{}, false
	}
	port := schema.state.roots[schema.state.values[values.index].root].port
	return port, port.valid()
}

// OwnsSemanticSource proves that source was emitted by this exact sealed Pack
// directory. SemanticSource intentionally carries only detached scalar
// identities, so a later owner join must not accept a copied source from an
// equal-content or foreign Pack merely because its module and semantic IDs
// compare equal.
func (schema *Schema) OwnsSemanticSource(source SemanticSource) bool {
	if schema == nil || schema.state == nil || !source.Available() {
		return false
	}
	endpoint, ok := schema.state.endpointIndex[source]
	issued, issuedOK := schema.state.sourceForEndpoint(endpoint)
	return ok && issuedOK && issued == source
}

// ScalarSource projects an exact Pack endpoint to its detached mounted
// semantic receipt. Head and class-only scalars deliberately have no
// fabricated source.
func (schema *Schema) ScalarSource(scalar Scalar) (SemanticSource, bool) {
	if schema == nil || schema.state == nil || !scalar.valid() || scalar.owner != schema.state.owner {
		return SemanticSource{}, false
	}
	endpoint, ok := scalar.Endpoint()
	if !ok {
		return SemanticSource{}, false
	}
	return schema.state.sourceForEndpoint(endpoint)
}

func (state *schema) sourceForEndpoint(endpoint Endpoint) (SemanticSource, bool) {
	if state == nil || !endpoint.valid() || endpoint.owner != state.owner || endpoint.index == 0 || uint64(endpoint.index) > uint64(len(state.endpointSources)) {
		return SemanticSource{}, false
	}
	source := state.endpointSources[endpoint.index-1]
	indexed, ok := state.endpointIndex[source]
	return source, ok && source.Available() && indexed == endpoint
}

func (schema *Schema) relation(root Root) (*relation, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return nil, false
	}
	relation := schema.state.relations[root.index]
	return relation, relation != nil && relation.valid()
}

func (schema *Schema) Admit(root Root, value Value) bool {
	relation, ok := schema.relation(root)
	return ok && value.valid() && value.owner == schema.state.owner && (value.IsBottom() || value.IsTop() || value.relation == relation)
}
func (schema *Schema) Bottom() Value {
	if schema == nil || schema.state == nil {
		return Value{}
	}
	return bottomValue(schema.state.owner)
}
func (schema *Schema) Top() Value {
	if schema == nil || schema.state == nil {
		return Value{}
	}
	return topValue(schema.state.owner)
}
func (schema *Schema) Lattice() lattice.Lattice[Value] {
	owner := schema.state.owner
	return lattice.Lattice[Value]{
		Bottom: func() Value { return bottomValue(owner) }, Top: func() Value { return topValue(owner) },
		Equal: equalValue, Same: sameValueRepresentation, LessOrEq: lessOrEqualValue,
		Join: func(left, right Value) Value {
			value, ok := joinValue(left, right)
			if !ok {
				return topValue(owner)
			}
			return value
		},
		Widen: func(left, right Value) Value {
			value, ok := widenValue(left, right)
			if !ok {
				return topValue(owner)
			}
			return value
		},
	}
}
func (schema *Schema) Fingerprint(value Value) uint64 {
	if schema == nil || schema.state == nil || !value.valid() || value.owner != schema.state.owner {
		return 0
	}
	return value.hash
}
func (schema *Schema) At(root Root, value Value, component int) uint64 {
	if !schema.Admit(root, value) || component < 0 || component >= len(value.rank) {
		return 0
	}
	return value.rank[component]
}

// Source is the sealed authored construction descriptor for exactly one
// source-producing Root.  Tail roots intentionally have no Source: a later
// Pack producer Rule owns Call/Vararg result construction.
type Source struct {
	schema *schema
	root   Root
}
type SourceItem struct {
	schema *schema
	root   Root
}

func (schema *Schema) Source(root Root) (Source, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return Source{}, false
	}
	kind := schema.state.roots[root.index].kind
	if kind != rootValues && kind != rootCall {
		return Source{}, false
	}
	return Source{schema: schema.state, root: root}, true
}
func (source Source) valid() bool {
	return source.schema != nil && source.root.valid() && source.root.schema == source.schema && (source.schema.roots[source.root.index].kind == rootValues || source.schema.roots[source.root.index].kind == rootCall)
}
func (source Source) Root() (Root, bool) { return source.root, source.valid() }
func (source Source) ContentID() (identity.ContentID, bool) {
	if !source.valid() {
		return identity.ContentID{}, false
	}
	id := source.schema.roots[source.root.index].id
	return id, id.Available()
}

func (source Source) Count() int {
	if !source.valid() {
		return 0
	}
	return 1
}
func (source Source) At(index int) (SourceItem, bool) {
	if !source.valid() || index != 0 {
		return SourceItem{}, false
	}
	return SourceItem{schema: source.schema, root: source.root}, true
}
func (item SourceItem) valid() bool {
	return item.schema != nil && item.root.valid() && item.root.schema == item.schema && (item.schema.roots[item.root.index].kind == rootValues || item.schema.roots[item.root.index].kind == rootCall)
}
func (item SourceItem) row() ([]Endpoint, Port, Port, bool) {
	if !item.valid() {
		return nil, Port{}, Port{}, false
	}
	root := item.schema.roots[item.root.index]
	switch root.kind {
	case rootValues:
		if uint64(root.sourceIndex) >= uint64(len(item.schema.values)) {
			return nil, Port{}, Port{}, false
		}
		row := item.schema.values[root.sourceIndex]
		if row.root == item.root.index {
			if !row.hasTail {
				return row.fixed, root.port, Port{}, true
			}
			tail, tailOK := item.schema.tailPort(row.tailRoot)
			return row.fixed, root.port, tail, tailOK
		}
	case rootCall:
		if uint64(root.sourceIndex) >= uint64(len(item.schema.calls)) {
			return nil, Port{}, Port{}, false
		}
		row := item.schema.calls[root.sourceIndex]
		if row.root == item.root.index {
			if !row.hasTail {
				return row.fixed, root.port, Port{}, true
			}
			tail, tailOK := item.schema.tailPort(row.tailRoot)
			return row.fixed, root.port, tail, tailOK
		}
	}
	return nil, Port{}, Port{}, false
}
func (item SourceItem) Port() (Port, bool) { _, port, _, ok := item.row(); return port, ok }
func (item SourceItem) FixedCount() int {
	fixed, _, _, ok := item.row()
	if !ok {
		return 0
	}
	return len(fixed)
}
func (item SourceItem) FixedAt(index int) (Endpoint, bool) {
	fixed, _, _, ok := item.row()
	if !ok || index < 0 || index >= len(fixed) {
		return Endpoint{}, false
	}
	return fixed[index], true
}
func (item SourceItem) Tail() (Port, Offset, bool) {
	_, _, tail, ok := item.row()
	if !ok || !tail.valid() {
		return Port{}, Offset{}, false
	}
	offset, offsetOK := zeroOffset(item.schema.owner)
	return tail, offset, offsetOK
}

func (state *schema) validMountedCall(index uint32, row callRow) bool {
	if state == nil || !row.moduleKey.Available() || !row.occurrenceID.Available() || !row.formalID.Available() || !row.typeArguments.Available() || uint64(row.root) >= uint64(len(state.roots)) {
		return false
	}
	root := state.roots[row.root]
	_, tailOK := state.tailPort(row.tailRoot)
	return root.kind == rootCall && root.sourceIndex == index && root.port.valid() && root.port.owner == state.owner && root.id.Available() && (row.hasTail && tailOK || !row.hasTail && row.tailRoot == 0)
}

type inputSelectionKind uint8

const (
	inputSelectionInvalid inputSelectionKind = iota
	inputSelectionScalar
	inputSelectionTail
	inputSelectionWhole
)

// InputSelector is one cold, Pack-owned interpretation template for a Target
// input source. It is independent of any Application: the same
// sealed template is applied to each selected call later.  Scalar selectors
// retain their already-sealed TableIndex, so solving never searches or interns
// an offset from a Target formal ordinal.
type InputSelector struct {
	schema *schema
	kind   inputSelectionKind
	table  TableIndex
	start  int
	sealed bool
}

func (selector InputSelector) valid() bool {
	if !selector.sealed || selector.schema == nil || selector.start < 0 {
		return false
	}
	switch selector.kind {
	case inputSelectionScalar:
		return selector.table.valid() && selector.table.offset.owner == selector.schema.owner &&
			uint64(selector.start) == selector.table.value
	case inputSelectionTail, inputSelectionWhole:
		return !selector.table.sealed
	default:
		return false
	}
}

// InputSelector returns the one reusable cold template compiled from a sealed
// Target input source. Link already sealed every Call root before Pack
// construction. Selection of an operation for an application remains
// activation authority, not this lookup.
func (schema *Schema) InputSelector(operation vocabulary.Operation, source vocabulary.InputSource) (InputSelector, bool) {
	if schema == nil || schema.state == nil || operation == 0 {
		return InputSelector{}, false
	}
	selector, ok := schema.state.inputSelectors[inputSelectorKey{operation: operation, source: source}]
	return selector, ok && selector.valid() && selector.schema == schema.state
}

// Start returns the call-row fixed-member offset carried by this exact
// Pack-owned selector. Consumers use it to project a mounted call's authored
// fixed suffix without retaining Pack's private selector representation.
func (selector InputSelector) Start() (int, bool) {
	return selector.start, selector.valid()
}

// OwnsInputSelector proves that this exact Pack schema issued the detached
// input-selection capability. Effect uses this fence before applying a
// selector to a mounted call, so a same-shaped selector from another seal
// cannot be spliced into its operand proof.
func (schema *Schema) OwnsInputSelector(selector InputSelector) bool {
	return schema != nil && schema.state != nil && selector.valid() && selector.schema == schema.state
}

// TailProducerKind distinguishes the two Program occurrences that produce a
// complete open Values tail.  It is not a type/class fallback: Call requires
// a selected-return Pack transfer and Vararg requires its incoming Pack
// boundary.  Neither is an authored zero-read Source.
type TailProducerKind uint8

const (
	TailProducerInvalid TailProducerKind = iota
	TailProducerCall
	TailProducerVararg
)

// TailProducer is Pack's capability for a Call/Vararg whole-Pack occurrence.
// A producer Rule must bind its Root and Port from this descriptor; callers
// cannot reinterpret a raw Program tail as an arbitrary free Port.
type TailProducer struct {
	schema *schema
	index  uint32
}

func (producer TailProducer) valid() bool {
	if producer.schema == nil || uint64(producer.index) >= uint64(len(producer.schema.tails)) {
		return false
	}
	row := producer.schema.tails[producer.index]
	if !row.moduleKey.Available() || !row.valueID.Available() || row.kind != TailProducerCall && row.kind != TailProducerVararg || uint64(row.root) >= uint64(len(producer.schema.roots)) {
		return false
	}
	root := producer.schema.roots[row.root]
	return root.kind == rootTail && root.sourceIndex == producer.index && root.port.valid() && root.port.owner == producer.schema.owner && root.id == mountedArtifactRootID(rootTail, row.moduleKey, row.valueID)
}
func (schema *Schema) TailProducer(root Root) (TailProducer, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state || schema.state.roots[root.index].kind != rootTail {
		return TailProducer{}, false
	}
	index := schema.state.roots[root.index].sourceIndex
	producer := TailProducer{schema: schema.state, index: index}
	return producer, producer.valid() && producer.schema.tails[index].root == root.index
}
func (producer TailProducer) Root() (Root, bool) {
	if !producer.valid() {
		return Root{}, false
	}
	root := Root{producer.schema, producer.schema.tails[producer.index].root}
	return root, root.valid()
}
func (producer TailProducer) Port() (Port, bool) {
	if !producer.valid() {
		return Port{}, false
	}
	port := producer.schema.roots[producer.schema.tails[producer.index].root].port
	return port, port.valid()
}
func (producer TailProducer) ContentID() (identity.ContentID, bool) {
	root, rootOK := producer.Root()
	if !rootOK {
		return identity.ContentID{}, false
	}
	id := producer.schema.roots[root.index].id
	return id, id.Available()
}
func (producer TailProducer) Kind() TailProducerKind {
	if !producer.valid() {
		return TailProducerInvalid
	}
	return producer.schema.tails[producer.index].kind
}

// Payload is the direct Program-Values marginal consumed by Heap.
type Payload struct {
	selection ScalarSelection
}

func (payload Payload) valid() bool { return payload.selection.valid() }
func (payload Payload) Root() (Root, bool) {
	if !payload.valid() {
		return Root{}, false
	}
	row := payload.selection.schema.values[payload.selection.values.index]
	root := Root{payload.selection.schema, row.root}
	return root, root.valid()
}

// Values returns the opaque mounted Values handle associated with this exact
// Pack payload. It carries no Shard or Program Term and is required to apply
// the owner-fenced scalar observation at Heap's selected offset.
func (payload Payload) Values() (Values, bool) {
	if !payload.valid() {
		return Values{}, false
	}
	return payload.selection.values, true
}
func (payload Payload) Selection() (ScalarSelection, bool) {
	return payload.selection, payload.valid()
}
func (payload Payload) SourceCount() int {
	if !payload.valid() {
		return 0
	}
	return len(payload.selection.schema.values[payload.selection.values.index].fixed)
}
func (payload Payload) SourceAt(index int) (SemanticSource, bool) {
	if !payload.valid() {
		return SemanticSource{}, false
	}
	row := payload.selection.schema.values[payload.selection.values.index]
	if index < 0 || index >= len(row.fixed) {
		return SemanticSource{}, false
	}
	return payload.selection.schema.sourceForEndpoint(row.fixed[index])
}
