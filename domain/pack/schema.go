package pack

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
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
	if !row.moduleKey.Available() || !row.occurrenceID.Available() || !row.port.valid() || row.port.owner != values.schema.owner || uint64(row.root) >= uint64(len(values.schema.roots)) {
		return false
	}
	root := values.schema.roots[row.root]
	return root.kind == rootValues && root.sourceIndex == values.index && root.port == row.port && root.id == mountedArtifactRootID(rootValues, row.moduleKey, row.occurrenceID)
}

type rootKind uint8

const (
	rootInvalid rootKind = iota
	rootValues
	rootCall
	rootTail
	// The following roots are the small P0 boundary plane.  They are still
	// Pack roots (and consequently carry exactly one whole-Pack equation); the
	// source terms which identify them remain cold in the descriptor rows.
	rootBind
	rootBody
	rootOutcome
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
	port         Port
	fixed        []Endpoint
	tail         Port
}

type inputSelectorKey struct {
	operation vocabulary.Operation
	source    vocabulary.InputSource
}

type callRow struct {
	root         uint32
	mountedID    identity.ContentID
	occurrenceID identity.ContentID
	valuesID     identity.ContentID
	receiverID   identity.ContentID
	typesID      identity.ContentID
	form         programartifact.CallForm
	moduleKey    identity.ContentID
	formalID     identity.ContentID
	typeFormal   FormalCallTypeArguments
	resultTail   uint32
	hasResult    bool
	actualTailID identity.ContentID
	tailContext  identity.ContentID
	port         Port
	fixed        []Endpoint
	tail         Port
}

type tailRow struct {
	root      uint32
	moduleKey identity.ContentID
	valueID   identity.ContentID
	port      Port
	kind      TailProducerKind
	sealed    bool
}

// bindRow is the exact Source.BindOrder/Flow Bind occurrence descriptor.  A
// bind does not own Cell values; it only names the existing Cell endpoints
// which a Pack adjustment may expose.  The Value source remains a separate
// root and Value owns the Cell transfer itself.
type bindRow struct {
	root      uint32
	moduleKey identity.ContentID
	bindID    identity.ContentID
	bodyID    identity.ContentID
	values    Values
	port      Port
	cells     []Endpoint
}

// bodyRow is one executable Program Body.  It deliberately has no caller or
// Application dimension: Call's Body capability is the only interprocedural
// selector.  Formal Cells and the Flow entry port are cold descriptors used
// by the body-boundary rules.
type bodyRow struct {
	root          uint32
	bodyID        identity.ContentID
	context       identity.ContentID
	moduleKey     identity.ContentID
	port          Port
	formals       []Endpoint
	formalIDs     []identity.ContentID
	normal        uint32
	returnOutcome uint32
	hasReturn     bool
	sealed        bool
}

// outcomeRow names one Flow Body Outcome. Normal completion has no Values;
// an explicit return keeps every executable authored Values alternative in
// order. Outcome is a Flow-owned derived term; Pack stores only the exact
// occurrence descriptor and the Pack port issued for it.
type outcomeRow struct {
	root       uint32
	bodyIndex  uint32
	moduleKey  identity.ContentID
	bodyID     identity.ContentID
	outcomeID  identity.ContentID
	kind       flowkind.OutcomeKind
	valueRoots []uint32
	port       Port
	sealed     bool
}

type schema struct {
	linkOwner link.OwnerCapability
	owner     *algebra

	roots     []rootRow
	relations []*relation
	values    []valuesRow
	calls     []callRow
	tails     []tailRow
	binds     []bindRow
	bodies    []bodyRow
	outcomes  []outcomeRow
	results   []sourceResultRow
	// sourceOccurrences is the compact mounted Program occurrence inverse
	// sealed beside results; no raw Program proof survives publication.
	sourceOccurrences []sourceOccurrenceRow

	relationIndex map[*relation]uint32
	// endpointSources is Pack-owned replay data.  The old Boundary values
	// were a retained Link -> Project -> Program reachability edge; they are
	// intentionally absent after this seal boundary.
	endpointSources []SemanticSource
	endpointIndex   map[SemanticSource]Endpoint
	// semanticEndpoints is construction-complete mounted substitution data.
	// Its keys are opaque ModuleKey/semantic IDs, never Shard/Term pairs.
	semanticEndpoints map[artifactValuesKey]Endpoint
	artifactValues    map[artifactValuesKey]uint32
	artifactTails     map[artifactValuesKey]uint32
	artifactCalls     map[artifactCallKey]uint32
	artifactBodies    map[artifactBodyKey]uint32
	artifactOutcomes  map[artifactOutcomeKey]uint32
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
// Summary reads canonicalize their ClosedRefs by this order; a transformation
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
	port := schema.state.values[values.index].port
	return port, port.valid()
}

// EndpointForMountedSemantic returns Pack's scalar subject for an exact
// mounted Program semantic value. This replaces the former Boundary.Value
// carrier: construction authenticates the source once, and post-seal callers
// retain no Link-owned proof object.
func (schema *Schema) EndpointForMountedSemantic(module, id identity.ContentID) (Endpoint, bool) {
	if schema == nil || schema.state == nil {
		return Endpoint{}, false
	}
	source, sourceOK := newSemanticSource(module, id)
	if !sourceOK {
		return Endpoint{}, false
	}
	endpoint, ok := schema.state.endpointIndex[source]
	return endpoint, ok && endpoint.valid()
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
	return ok && endpoint.valid() && endpoint.owner == schema.state.owner
}

// EndpointTag exposes the owner-issued selector identity for an exact Pack
// endpoint.  It is meaningful only to this Schema and never decodes Link.
func (schema *Schema) EndpointTag(endpoint Endpoint) (uint64, bool) {
	if schema == nil || schema.state == nil || !endpoint.valid() || endpoint.owner != schema.state.owner {
		return 0, false
	}
	if endpoint.index == 0 || uint64(endpoint.index) > uint64(len(schema.state.endpointSources)) {
		return 0, false
	}
	return uint64(endpoint.index), true
}

// ScalarSource projects an exact Pack endpoint to its detached mounted
// semantic receipt. Head and class-only scalars deliberately have no
// fabricated source.
func (schema *Schema) ScalarSource(scalar Scalar) (SemanticSource, bool) {
	if schema == nil || schema.state == nil || !scalar.valid() || scalar.owner != schema.state.owner {
		return SemanticSource{}, false
	}
	endpoint, ok := scalar.Endpoint()
	if !ok || endpoint.index == 0 || uint64(endpoint.index) > uint64(len(schema.state.endpointSources)) {
		return SemanticSource{}, false
	}
	source := schema.state.endpointSources[endpoint.index-1]
	_, valid := schema.state.endpointIndex[source]
	return source, valid && source.Available()
}

// ScalarSourceRoute is the exact endpoint projection used by staged Value
// selectors.  The tag is Pack's schema-issued endpoint identity, not a Boundary
// ordinal or an application/formal coordinate.
func (schema *Schema) ScalarSourceRoute(scalar Scalar) (SemanticSource, uint64, bool) {
	if schema == nil || schema.state == nil {
		return SemanticSource{}, 0, false
	}
	value, ok := schema.ScalarSource(scalar)
	endpoint, endpointOK := scalar.Endpoint()
	if !ok || !endpointOK || endpoint.owner != schema.state.owner {
		return SemanticSource{}, 0, false
	}
	return value, uint64(endpoint.index), endpoint.index != 0
}

func (schema *Schema) relation(root Root) (*relation, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return nil, false
	}
	relation := schema.state.relations[root.index]
	return relation, relation != nil && relation.valid()
}

// RootForValue recovers the exact occurrence root carried by one non-extreme
// Pack value in O(1).  Bottom and Top intentionally name no occurrence.
func (schema *Schema) RootForValue(value Value) (Root, bool) {
	if schema == nil || schema.state == nil || !value.valid() || value.owner != schema.state.owner || value.relation == nil || value.bottom || value.top {
		return Root{}, false
	}
	index, ok := schema.state.relationIndex[value.relation]
	if !ok || uint64(index) >= uint64(len(schema.state.roots)) {
		return Root{}, false
	}
	root := Root{schema.state, index}
	return root, root.valid() && schema.state.relations[index] == value.relation
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
			return row.fixed, row.port, row.tail, true
		}
	case rootCall:
		if uint64(root.sourceIndex) >= uint64(len(item.schema.calls)) {
			return nil, Port{}, Port{}, false
		}
		row := item.schema.calls[root.sourceIndex]
		if row.root == item.root.index {
			return row.fixed, row.port, row.tail, true
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

// RootKind reports the cold occurrence family which issued root.  It is a
// diagnostic selector only; callers still need the typed descriptor accessors
// below before constructing a Rule operand.
type RootKind uint8

const (
	RootInvalid RootKind = iota
	RootValues
	RootCall
	RootTail
	RootBind
	RootBody
	RootOutcome
)

func (schema *Schema) RootKind(root Root) (RootKind, bool) {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return RootInvalid, false
	}
	switch schema.state.roots[root.index].kind {
	case rootValues:
		return RootValues, true
	case rootCall:
		return RootCall, true
	case rootTail:
		return RootTail, true
	case rootBind:
		return RootBind, true
	case rootBody:
		return RootBody, true
	case rootOutcome:
		return RootOutcome, true
	default:
		return RootInvalid, false
	}
}

// PackOnly reports whether a root's complete relation consists of exactly one
// whole-Pack target.  Scalar-adjustment and splice rules require this shape;
// Bind/body-entry rules intentionally use the richer scalar-plus-Pack shape.
func (schema *Schema) PackOnly(root Root) bool {
	if schema == nil || schema.state == nil || !root.valid() || root.schema != schema.state {
		return false
	}
	relation, ok := schema.relation(root)
	return ok && len(relation.targets) == 1 && relation.targets[0].kind == EquationPack
}

// CellTag is the local opaque selector associated with a Cell endpoint. It is
// not a Source/Link ordinal and cannot be used to mint another endpoint.
func (schema *Schema) CellTag(endpoint Endpoint) (uint64, bool) {
	if schema == nil || schema.state == nil || !endpoint.valid() || endpoint.owner != schema.state.owner {
		return 0, false
	}
	for _, candidate := range schema.state.semanticEndpoints {
		if candidate == endpoint {
			return uint64(endpoint.index), true
		}
	}
	return 0, false
}

// Bind is one executable fixed Cell-binding occurrence. It carries no Value
// fact and no copied authored row; all source order is reissued from the
// sealed descriptor.
type Bind struct {
	schema *schema
	index  uint32
}

func (bind Bind) valid() bool {
	if bind.schema == nil || uint64(bind.index) >= uint64(len(bind.schema.binds)) {
		return false
	}
	row := bind.schema.binds[bind.index]
	if !row.moduleKey.Available() || !row.bindID.Available() || !row.bodyID.Available() || !row.port.valid() || row.port.owner != bind.schema.owner || uint64(row.root) >= uint64(len(bind.schema.roots)) {
		return false
	}
	root := bind.schema.roots[row.root]
	if root.kind != rootBind || root.sourceIndex != bind.index || root.port != row.port || root.id != mountedArtifactRootID(rootBind, row.moduleKey, row.bindID) || !row.values.valid() {
		return false
	}
	for _, cell := range row.cells {
		if !cell.valid() || cell.owner != bind.schema.owner {
			return false
		}
	}
	return true
}
func (bind Bind) Root() (Root, bool) {
	if !bind.valid() {
		return Root{}, false
	}
	root := Root{schema: bind.schema, index: bind.schema.binds[bind.index].root}
	return root, root.valid()
}
func (bind Bind) Input() (Values, bool) {
	if !bind.valid() {
		return Values{}, false
	}
	values := bind.schema.binds[bind.index].values
	return values, values.valid()
}
func (bind Bind) InputRoot() (Root, bool) {
	if !bind.valid() {
		return Root{}, false
	}
	values := bind.schema.binds[bind.index].values
	return Root{schema: bind.schema, index: values.schema.values[values.index].root}, true
}
func (bind Bind) Port() (Port, bool) {
	if !bind.valid() {
		return Port{}, false
	}
	port := bind.schema.binds[bind.index].port
	return port, port.valid()
}
func (bind Bind) CellCount() int {
	if !bind.valid() {
		return 0
	}
	return len(bind.schema.binds[bind.index].cells)
}
func (bind Bind) CellAt(index int) (Endpoint, bool) {
	if !bind.valid() || index < 0 || index >= len(bind.schema.binds[bind.index].cells) {
		return Endpoint{}, false
	}
	endpoint := bind.schema.binds[bind.index].cells[index]
	return endpoint, endpoint.valid()
}

// Body is one Call-owned executable function Body projected into Pack's
// boundary schema. The capability itself is issued by Call; this descriptor
// merely adds the exact Pack entry/normal-exit ports.
type Body struct {
	schema *schema
	index  uint32
}

func (body Body) valid() bool {
	if body.schema == nil || uint64(body.index) >= uint64(len(body.schema.bodies)) {
		return false
	}
	row := body.schema.bodies[body.index]
	if !row.sealed || !row.bodyID.Available() || !row.context.Available() || !row.moduleKey.Available() || !row.port.valid() || row.port.owner != body.schema.owner || len(row.formals) != len(row.formalIDs) || uint64(row.root) >= uint64(len(body.schema.roots)) {
		return false
	}
	root := body.schema.roots[row.root]
	if root.kind != rootBody || root.sourceIndex != body.index || root.port != row.port || root.id != mountedArtifactRootID(rootBody, row.moduleKey, row.bodyID) {
		return false
	}
	if row.normal >= uint32(len(body.schema.outcomes)) {
		return false
	}
	normal := Outcome{schema: body.schema, index: row.normal}
	if !normal.valid() || body.schema.outcomes[row.normal].bodyIndex != body.index || body.schema.outcomes[row.normal].kind != flowkind.OutcomeNormal {
		return false
	}
	if row.hasReturn {
		if row.returnOutcome >= uint32(len(body.schema.outcomes)) {
			return false
		}
		returned := Outcome{schema: body.schema, index: row.returnOutcome}
		if !returned.valid() || body.schema.outcomes[row.returnOutcome].bodyIndex != body.index || body.schema.outcomes[row.returnOutcome].kind != flowkind.OutcomeReturn {
			return false
		}
	} else if row.returnOutcome != 0 {
		return false
	}
	return true
}

// Outcome is one Body Flow Outcome projected into a Pack port. Outcome
// values remain Flow-owned; Pack only carries their whole-Pack expressions.
type Outcome struct {
	schema *schema
	index  uint32
}

func (outcome Outcome) valid() bool {
	if outcome.schema == nil || uint64(outcome.index) >= uint64(len(outcome.schema.outcomes)) {
		return false
	}
	row := outcome.schema.outcomes[outcome.index]
	if !row.sealed || !row.moduleKey.Available() || !row.bodyID.Available() || !row.outcomeID.Available() || row.kind != flowkind.OutcomeNormal && row.kind != flowkind.OutcomeReturn || !row.port.valid() || row.port.owner != outcome.schema.owner || uint64(row.root) >= uint64(len(outcome.schema.roots)) || uint64(row.bodyIndex) >= uint64(len(outcome.schema.bodies)) {
		return false
	}
	root := outcome.schema.roots[row.root]
	body := outcome.schema.bodies[row.bodyIndex]
	if root.kind != rootOutcome || root.sourceIndex != outcome.index || root.port != row.port || root.id != mountedArtifactRootID(rootOutcome, row.moduleKey, row.outcomeID) || !body.sealed || body.moduleKey != row.moduleKey || body.bodyID != row.bodyID {
		return false
	}
	return row.kind == flowkind.OutcomeNormal && len(row.valueRoots) == 0 || row.kind == flowkind.OutcomeReturn && len(row.valueRoots) > 0
}

func (outcome Outcome) Root() (Root, bool) {
	if !outcome.valid() {
		return Root{}, false
	}
	root := Root{schema: outcome.schema, index: outcome.schema.outcomes[outcome.index].root}
	return root, root.valid() && outcome.schema.roots[root.index].sourceIndex == outcome.index
}
func (outcome Outcome) Kind() flowkind.OutcomeKind {
	if !outcome.valid() {
		return 0
	}
	return outcome.schema.outcomes[outcome.index].kind
}
func (outcome Outcome) Values() (Values, bool) {
	if !outcome.valid() {
		return Values{}, false
	}
	row := outcome.schema.outcomes[outcome.index]
	if len(row.valueRoots) != 1 || uint64(row.valueRoots[0]) >= uint64(len(outcome.schema.roots)) {
		return Values{}, false
	}
	root := outcome.schema.roots[row.valueRoots[0]]
	if root.kind != rootValues || uint64(root.sourceIndex) >= uint64(len(outcome.schema.values)) || outcome.schema.values[root.sourceIndex].root != row.valueRoots[0] {
		return Values{}, false
	}
	values := Values{schema: outcome.schema, index: root.sourceIndex}
	return values, values.valid()
}
func (outcome Outcome) ValuesCount() int {
	if !outcome.valid() {
		return 0
	}
	return len(outcome.schema.outcomes[outcome.index].valueRoots)
}
func (outcome Outcome) ValuesRootAt(index int) (Root, bool) {
	if !outcome.valid() || index < 0 || index >= len(outcome.schema.outcomes[outcome.index].valueRoots) {
		return Root{}, false
	}
	row := outcome.schema.outcomes[outcome.index]
	rootIndex := row.valueRoots[index]
	if uint64(rootIndex) >= uint64(len(outcome.schema.roots)) {
		return Root{}, false
	}
	rootRow := outcome.schema.roots[rootIndex]
	if rootRow.kind != rootValues || uint64(rootRow.sourceIndex) >= uint64(len(outcome.schema.values)) {
		return Root{}, false
	}
	valueRow := outcome.schema.values[rootRow.sourceIndex]
	if valueRow.root != rootIndex || valueRow.moduleKey != row.moduleKey {
		return Root{}, false
	}
	root := Root{schema: outcome.schema, index: rootIndex}
	return root, root.valid()
}
func (outcome Outcome) Port() (Port, bool) {
	if !outcome.valid() {
		return Port{}, false
	}
	port := outcome.schema.outcomes[outcome.index].port
	return port, port.valid()
}
func (body Body) Root() (Root, bool) {
	if !body.valid() {
		return Root{}, false
	}
	root := Root{schema: body.schema, index: body.schema.bodies[body.index].root}
	return root, root.valid()
}
func (body Body) Port() (Port, bool) {
	if !body.valid() {
		return Port{}, false
	}
	port := body.schema.bodies[body.index].port
	return port, port.valid()
}
func (body Body) FormalCount() int {
	if !body.valid() {
		return 0
	}
	return len(body.schema.bodies[body.index].formals)
}
func (body Body) FormalAt(index int) (Endpoint, bool) {
	if !body.valid() || index < 0 || index >= len(body.schema.bodies[body.index].formals) {
		return Endpoint{}, false
	}
	endpoint := body.schema.bodies[body.index].formals[index]
	return endpoint, endpoint.valid()
}
func (body Body) Normal() (Outcome, bool) {
	if !body.valid() {
		return Outcome{}, false
	}
	outcome := Outcome{schema: body.schema, index: body.schema.bodies[body.index].normal}
	return outcome, outcome.valid() && outcome.schema.outcomes[outcome.index].bodyID == body.schema.bodies[body.index].bodyID && outcome.schema.outcomes[outcome.index].moduleKey == body.schema.bodies[body.index].moduleKey
}
func (body Body) Return() (Outcome, bool) {
	if !body.valid() || !body.schema.bodies[body.index].hasReturn {
		return Outcome{}, false
	}
	outcome := Outcome{schema: body.schema, index: body.schema.bodies[body.index].returnOutcome}
	return outcome, outcome.valid() && outcome.Kind() == flowkind.OutcomeReturn && outcome.schema.outcomes[outcome.index].bodyID == body.schema.bodies[body.index].bodyID && outcome.schema.outcomes[outcome.index].moduleKey == body.schema.bodies[body.index].moduleKey
}

// BelongsTo proves that this Pack Outcome was issued for this exact Pack Body
// descriptor. It exposes no raw Program Body or Outcome coordinate.
func (outcome Outcome) BelongsTo(body Body) bool {
	if !outcome.valid() || !body.valid() || outcome.schema != body.schema {
		return false
	}
	outcomeRow := outcome.schema.outcomes[outcome.index]
	bodyRow := body.schema.bodies[body.index]
	return outcomeRow.bodyID == bodyRow.bodyID && outcomeRow.moduleKey == bodyRow.moduleKey
}

// Same is an owner-fenced capability comparison. Invalid or foreign
// descriptors are never considered equal merely because their coordinates
// happen to match.
func (body Body) Same(other Body) bool {
	return body.valid() && other.valid() && body.schema == other.schema && body.index == other.index
}
func (outcome Outcome) Same(other Outcome) bool {
	return outcome.valid() && other.valid() && outcome.schema == other.schema && outcome.index == other.index
}

func (state *schema) validMountedCall(row callRow) bool {
	return state != nil && row.moduleKey.Available() && row.mountedID.Available() && row.occurrenceID.Available() && row.valuesID.Available() && row.typesID.Available() && row.formalID.Available() && row.typeFormal.Available() && row.form.Valid() && row.port.valid() && row.port.owner == state.owner
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
	if !row.sealed || !row.moduleKey.Available() || !row.valueID.Available() || row.kind != TailProducerCall && row.kind != TailProducerVararg || !row.port.valid() || row.port.owner != producer.schema.owner || uint64(row.root) >= uint64(len(producer.schema.roots)) {
		return false
	}
	root := producer.schema.roots[row.root]
	return root.kind == rootTail && root.sourceIndex == producer.index && root.port == row.port && root.id == mountedArtifactRootID(rootTail, row.moduleKey, row.valueID)
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
	port := producer.schema.tails[producer.index].port
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
type PayloadRequest struct {
	Values Values
	Index  int
}
type Payload struct {
	schema    *schema
	root      Root
	values    Values
	selection ScalarSelection
	sources   []SemanticSource
}

func (schema *Schema) Payloads(requests []PayloadRequest) ([]Payload, bool) {
	if schema == nil || schema.state == nil {
		return nil, false
	}
	out := make([]Payload, len(requests))
	for i, request := range requests {
		if request.Index < 0 || !request.Values.valid() || request.Values.schema != schema.state {
			return nil, false
		}
		row := schema.state.values[request.Values.index]
		index, ok := schema.TableIndex(int64(request.Index))
		if !ok {
			return nil, false
		}
		selection := ScalarSelection{schema: schema.state, values: request.Values, kind: scalarSelectionTableIndex, tableIndex: index, sealed: true}
		sources := make([]SemanticSource, 0, len(row.fixed))
		for _, endpoint := range row.fixed {
			value, valueOK := schema.ScalarSource(Scalar{owner: endpoint.owner, kind: ScalarEndpoint, endpoint: endpoint, class: endpoint.class, sealed: true})
			if !valueOK {
				return nil, false
			}
			sources = append(sources, value)
		}
		out[i] = Payload{schema: schema.state, root: Root{schema.state, row.root}, values: request.Values, selection: selection, sources: sources}
	}
	return out, true
}
func (payload Payload) Root() (Root, bool) {
	return payload.root, payload.schema != nil && payload.root.valid()
}

// Values returns the opaque mounted Values handle associated with this exact
// Pack payload. It carries no Shard or Program Term and is required to apply
// the owner-fenced scalar observation at Heap's selected offset.
func (payload Payload) Values() (Values, bool) {
	return payload.values, payload.schema != nil && payload.values.valid() && payload.values.schema == payload.schema
}
func (payload Payload) Selection() (ScalarSelection, bool) {
	return payload.selection, payload.selection.valid()
}
func (payload Payload) SourceCount() int {
	if payload.schema == nil {
		return 0
	}
	return len(payload.sources)
}
func (payload Payload) SourceAt(index int) (SemanticSource, bool) {
	if payload.schema == nil || index < 0 || index >= len(payload.sources) {
		return SemanticSource{}, false
	}
	return payload.sources[index], true
}
