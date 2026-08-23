package typeauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/transform"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// RuntimeInner is a dense, Runtime-owner-fenced exact structural type. Its
// fields remain private: a caller cannot turn a portable type byte string into
// a hot handle or combine an inner from another Runtime authority.
type RuntimeInner struct {
	owner *Runtime
	index uint32 // one-based into Runtime.rows
}

// RuntimeField is the sealed scalar projection of one direct record field.
// The child is an owner-fenced Runtime row; optionality and readonly are the
// only authored metadata retained beside it. The source typ.Type is consumed
// during sealing and is never reachable through this value.
//
// Inner is intentionally exported as a value rather than exposing the dense
// row index. Runtime methods still authenticate it against their owner before
// using it as a handle.
type RuntimeField struct {
	Inner    RuntimeInner
	Optional bool
	Readonly bool
}

// Child returns the projected field child. It is an alias for Inner for
// consumers that describe a field projection in terms of its child row.
func (field RuntimeField) Child() RuntimeInner { return field.Inner }

// Type returns the projected field child. Runtime rows are the sealed type
// representation, so no typ.Type is materialized by this accessor.
func (field RuntimeField) Type() RuntimeInner { return field.Inner }

// RuntimeInput is a cold, ownership-isolated canonical graph receipt. It can
// only be minted by the Authority which owns the Link it will be sealed
// against. The receipt's source plane is consumed during sealing; Runtime
// retains no typ graph or receipt after publication.
type RuntimeInput struct {
	authority    *Authority
	graph        typ.CanonicalGraphReceipt
	prefix       *familyPrefix
	prefixMember int
}

type runtimeChild struct {
	inner   RuntimeInner
	present bool
}

// A range is intentionally retained only for the one structural plane that
// has a production consumer: direct Union variants. All other source planes
// are construction-only and are not copied into Runtime.
type runtimeRange struct{ start, end uint32 }

type runtimeRow struct {
	form         kind.Kind
	runtimeKinds runtimekind.Set
	canonicalID  identity.ContentID
	scopedID     identity.ContentID

	inner    runtimeChild
	variants runtimeRange
	fields   map[string]RuntimeField
	atoms    []uint32
	rank     uint32
}

// Runtime is the immutable finite structural authority consumed by Runtime
// reflection. Its only retained dense structural feed is the direct Union
// variant plane and the direct Optional inner edge used by descriptor
// formation; every other authored child is named by the canonical
// construction value the closed universe keeps as its relation source.
type Runtime struct {
	sourceID              identity.ContentID
	id                    identity.ContentID
	runtimeKindsPublished bool

	rows     []runtimeRow
	variants []runtimeChild

	identities []identity.ContentID
	canonical  []uint32

	// The closed universe of the sealed rows. closedPositions maps a one-based
	// dense row onto its universe position (-1 for an open row) and closedRows
	// is the inverse. sources holds the canonical construction value of every
	// row: it is the relation's only proof source, and it is linear in the
	// admitted node count. relation answers one ordered pair at a time.
	closedPositions []int32
	closedRows      []uint32
	sources         []typ.Type
	relation        runtimeRelation

	// Primitive rows are part of the same dense Runtime universe. Their
	// handles are discovered from the same graph receipt source plane as every
	// input row; no synthetic Runtime row is installed.
	nilRow     uint32
	booleanRow uint32
	numberRow  uint32
	integerRow uint32
	stringRow  uint32
	anyRow     uint32
	unknownRow uint32
	neverRow   uint32
}

// runtimeCanonicalInput records every caller position represented by one
// canonical input identity. Runtime's dense universe is a function of the set
// of receipts, not incidental traversal order; positions are restored only
// after receipt source ordinals have been coalesced.
type runtimeCanonicalInput struct {
	input     RuntimeInput
	identity  identity.ContentID
	positions []int
}

// Primitive Runtime rows are a fixed vocabulary, not a per-seal discovery
// pass. Their owner-issued receipts are consumed once into immutable singleton
// seed rows reused by every Runtime seal.
type runtimePrimitiveSourceTable struct {
	sources []runtimePrimitiveSource
	err     error
}

type runtimePrimitiveSource struct {
	node  typ.CanonicalGraphNode
	value typ.Type
}

var runtimePrimitiveSources = makeRuntimePrimitiveSources()

func makeRuntimePrimitiveSources() runtimePrimitiveSourceTable {
	values := []typ.Type{typ.Nil, typ.Boolean, typ.Number, typ.Integer, typ.String, typ.Any, typ.Unknown, typ.Never}
	sources := make([]runtimePrimitiveSource, len(values))
	for index, value := range values {
		graph, err := typ.EncodeCanonicalGraph(context.Background(), value)
		if err != nil || !graph.Valid() {
			if err == nil {
				err = errors.New("typeauthority: primitive Runtime graph unavailable")
			}
			return runtimePrimitiveSourceTable{err: err}
		}
		nodes := graph.Nodes()
		plane, planeOK := graph.TakeSourcePlane()
		if !planeOK || len(nodes) != 1 || len(plane) != 1 || plane[0] == nil {
			return runtimePrimitiveSourceTable{err: errors.New("typeauthority: primitive Runtime source unavailable")}
		}
		sources[index] = runtimePrimitiveSource{node: nodes[0], value: plane[0]}
	}
	return runtimePrimitiveSourceTable{sources: sources}
}

// RuntimeInputForType clones and admits one graph at the authority boundary.
// The root disposition in the owner-issued receipt is authoritative: closed
// roots may be sealed into Runtime, while roots retaining external lexical
// formals remain open and are rejected as Runtime roots.
func (a *Authority) RuntimeInputForType(value typ.Type) (RuntimeInput, bool) {
	if a == nil || !a.LinkID().Available() || value == nil {
		return RuntimeInput{}, false
	}
	owned := transform.Clone(value)
	graph, err := typ.EncodeCanonicalGraph(context.Background(), owned)
	if err != nil || !graph.Valid() {
		return RuntimeInput{}, false
	}
	root, rootOK := graph.Root()
	if !rootOK || !root.Closed {
		return RuntimeInput{}, false
	}
	if _, digestOK := graph.Digest(); !digestOK {
		return RuntimeInput{}, false
	}
	return RuntimeInput{authority: a, graph: graph}, true
}

// CanonicalIdentity is the owner-neutral identity of this admitted input.
func (input RuntimeInput) CanonicalIdentity() (identity.ContentID, bool) {
	if input.authority == nil || !input.graph.Valid() {
		return identity.ContentID{}, false
	}
	digest, available := input.graph.Digest()
	return identity.ContentID(digest), available && identity.ContentID(digest).Available()
}

// RootKind reports the root structural form admitted by this construction
// token without exposing its detached graph.
func (input RuntimeInput) RootKind() (kind.Kind, bool) {
	if input.authority == nil || !input.graph.Valid() {
		return 0, false
	}
	root, available := input.graph.Root()
	return root.Kind, available
}

// SealRuntime closes the receipt source planes of Static's finite canonical
// input set. Source nodes are sorted by their stable lexical identity and
// coalesced linearly into dense rows. Direct authored Union children and the
// direct Optional inner are the only structural feeds copied to Runtime.
func SealRuntime(types *Authority, inputs []RuntimeInput) (*Runtime, []RuntimeInner, error) {
	if types == nil || types.artifact == nil || !types.LinkID().Available() {
		return nil, nil, errors.New("typeauthority: Runtime requires sealed artifact authority")
	}
	runtime := &Runtime{sourceID: types.LinkID()}
	builder := runtimeBuilder{runtime: runtime}
	canonicalInputs, err := canonicalRuntimeInputs(types, inputs)
	if err != nil {
		return nil, nil, err
	}
	canonicalInners, err := builder.ingest(canonicalInputs)
	if err != nil {
		return nil, nil, err
	}
	inners := make([]RuntimeInner, len(inputs))
	for index, input := range canonicalInputs {
		for _, position := range input.positions {
			inners[position] = canonicalInners[index]
		}
	}
	if err := builder.sealRuntimeKinds(); err != nil {
		return nil, nil, err
	}
	if err := builder.sealCanonical(); err != nil {
		return nil, nil, err
	}
	if err := builder.sealDescriptors(); err != nil {
		return nil, nil, err
	}
	// Direct field projections are derived from the live construction graph
	// before the relation source is released. Runtime retains only dense child
	// handles and scalar metadata.
	if err := builder.sealFields(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealRanks(); err != nil {
		return nil, nil, err
	}
	// The closed universe is published last: it is the relation's index over
	// the construction values the seal keeps as its proof source.
	if err := builder.sealClosedUniverse(); err != nil {
		return nil, nil, err
	}
	if err := runtime.sealIdentity(); err != nil {
		return nil, nil, err
	}
	// Reference projections are persistent scalar rows. Their canonical graph
	// receipts are a one-shot construction handoff and die once this Runtime
	// has installed the complete input denominator.
	types.releaseRuntimeInputs()
	// No receipt and no source ordinal map crosses the seal. The canonical
	// construction values do: one value per row is the sealed relation's proof
	// source, and it is the whole cost the seal pays for the judgment.
	builder.sourceMaps = nil
	return runtime, inners, nil
}

func canonicalRuntimeInputs(types *Authority, inputs []RuntimeInput) ([]runtimeCanonicalInput, error) {
	if types == nil {
		return nil, errors.New("typeauthority: nil Runtime selector authority")
	}
	ordered := make([]runtimeCanonicalInput, 0, len(inputs))
	for position, input := range inputs {
		if input.authority != types || !input.graph.Valid() {
			return nil, errors.New("typeauthority: foreign Runtime input")
		}
		root, rootOK := input.graph.Root()
		if !rootOK || !root.Closed {
			return nil, errors.New("typeauthority: Runtime input root is open")
		}
		digest, digestOK := input.graph.Digest()
		canonicalID := identity.ContentID(digest)
		if !digestOK || !canonicalID.Available() {
			return nil, errors.New("typeauthority: Runtime input identity mismatch")
		}
		ordered = append(ordered, runtimeCanonicalInput{
			input: input, identity: canonicalID, positions: []int{position},
		})
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].identity[:], ordered[right].identity[:]) < 0
	})
	unique := ordered[:0]
	for _, input := range ordered {
		if len(unique) != 0 && unique[len(unique)-1].identity == input.identity {
			unique[len(unique)-1].positions = append(unique[len(unique)-1].positions, input.positions...)
			continue
		}
		unique = append(unique, input)
	}
	return unique, nil
}

type runtimeSourceKey struct {
	identity identity.ContentID
	kind     kind.Kind
	closed   bool
	scope    identity.ContentID
	formals  uint32
	bound    bool
	owner    identity.ContentID
	ordinal  uint32
}

type runtimeSource struct {
	input   int
	ordinal uint32
	seed    int
	node    typ.CanonicalGraphNode
	value   typ.Type
	key     runtimeSourceKey
	row     uint32
}

// runtimeBuilder is deliberately short-lived. It owns only the detached
// source values needed by the relation prover and the receipt ordinal maps
// needed to remap direct authored edges into dense handles.
type runtimeBuilder struct {
	runtime      *Runtime
	construction []typ.Type
	sourceMaps   [][]uint32
	keys         []runtimeSourceKey
}

func (b *runtimeBuilder) ingest(inputs []runtimeCanonicalInput) ([]RuntimeInner, error) {
	prefix, members := sharedFamilyPrefix(inputs)
	if prefix == nil {
		return b.ingestFresh(inputs)
	}
	if familyPrefixHasLocal(members) {
		return b.mergeFamilyPrefix(prefix, inputs, members)
	}
	return b.installFamilyPrefix(prefix, inputs, members)
}

func (b *runtimeBuilder) ingestFresh(inputs []runtimeCanonicalInput) ([]RuntimeInner, error) {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != 0 || len(b.construction) != 0 {
		return nil, errors.New("typeauthority: invalid Runtime receipt builder")
	}
	b.sourceMaps = make([][]uint32, len(inputs))
	occurrences := make([]runtimeSource, 0)
	if runtimePrimitiveSources.err != nil {
		return nil, runtimePrimitiveSources.err
	}
	for seedIndex, source := range runtimePrimitiveSources.sources {
		nodes := []typ.CanonicalGraphNode{source.node}
		if err := validateRuntimeSourceNode(nodes, source.node); err != nil {
			return nil, err
		}
		key, err := runtimeSourceKeyForNode(nodes, source.node)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, runtimeSource{input: -1, seed: seedIndex, node: source.node, value: runtimeSourceValue(source.value), key: key})
	}
	for inputIndex, input := range inputs {
		nodes := input.input.graph.Nodes()
		if len(nodes) == 0 {
			return nil, errors.New("typeauthority: Runtime receipt source empty")
		}
		// A sealed family receipt lends its shared read-only plane to every
		// Runtime seal; an ordinary receipt transfers its linear plane once.
		plane, planeOK := input.input.graph.SourcePlane()
		if !planeOK || len(plane) != len(nodes) {
			return nil, errors.New("typeauthority: Runtime receipt source unavailable")
		}
		b.sourceMaps[inputIndex] = make([]uint32, len(nodes))
		for ordinal, node := range nodes {
			if err := validateRuntimeSourceNode(nodes, node); err != nil {
				return nil, err
			}
			value := plane[ordinal]
			if value == nil {
				return nil, errors.New("typeauthority: Runtime receipt source unavailable")
			}
			key, err := runtimeSourceKeyForNode(nodes, node)
			if err != nil {
				return nil, err
			}
			occurrences = append(occurrences, runtimeSource{input: inputIndex, ordinal: uint32(ordinal), seed: -1, node: node, value: runtimeSourceValue(value), key: key})
		}
		root, rootOK := input.input.graph.RootOrdinal()
		if !rootOK || uint64(root) >= uint64(len(nodes)) {
			return nil, errors.New("typeauthority: Runtime receipt root ordinal")
		}
	}
	if len(occurrences) == 0 {
		return nil, errors.New("typeauthority: Runtime receipt source unavailable")
	}
	order := make([]int, len(occurrences))
	for index := range occurrences {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		return runtimeSourceKeyCompare(occurrences[order[left]].key, occurrences[order[right]].key) < 0 ||
			(runtimeSourceKeyCompare(occurrences[order[left]].key, occurrences[order[right]].key) == 0 && runtimeSourceTieLess(occurrences[order[left]], occurrences[order[right]]))
	})
	representatives := make([]runtimeSource, 0)
	for cursor := 0; cursor < len(order); {
		first := occurrences[order[cursor]]
		rowOrdinal, err := runtimeDenseOrdinal(len(b.runtime.rows))
		if err != nil {
			return nil, err
		}
		representatives = append(representatives, first)
		row := runtimeRow{form: first.node.Kind, canonicalID: identity.ContentID(first.node.Identity)}
		if !first.node.Closed {
			row.scopedID = runtimeSourceIdentity(first.key)
		}
		if !row.canonicalID.Available() || (!first.node.Closed && !row.scopedID.Available()) {
			return nil, errors.New("typeauthority: Runtime source identity unavailable")
		}
		b.runtime.rows = append(b.runtime.rows, row)
		b.construction = append(b.construction, runtimeSourceValue(first.value))
		for cursor < len(order) && runtimeSourceKeyCompare(first.key, occurrences[order[cursor]].key) == 0 {
			occurrence := &occurrences[order[cursor]]
			occurrence.row = rowOrdinal
			if occurrence.input >= 0 {
				b.sourceMaps[occurrence.input][occurrence.ordinal] = rowOrdinal
			}
			cursor++
		}
	}
	seedRows := make([]uint32, len(runtimePrimitiveSources.sources))
	for _, occurrence := range occurrences {
		if occurrence.seed >= 0 && occurrence.seed < len(seedRows) {
			seedRows[occurrence.seed] = occurrence.row
		}
	}
	if len(seedRows) != len(runtimePrimitiveSources.sources) {
		return nil, errors.New("typeauthority: primitive Runtime row count")
	}
	b.runtime.nilRow, b.runtime.booleanRow, b.runtime.numberRow, b.runtime.integerRow = seedRows[0], seedRows[1], seedRows[2], seedRows[3]
	b.runtime.stringRow, b.runtime.anyRow, b.runtime.unknownRow, b.runtime.neverRow = seedRows[4], seedRows[5], seedRows[6], seedRows[7]
	for index, representative := range representatives {
		if err := b.installReceiptEdges(index, representative); err != nil {
			return nil, err
		}
	}
	canonicalInners := make([]RuntimeInner, len(inputs))
	for inputIndex, input := range inputs {
		root, rootOK := input.input.graph.RootOrdinal()
		if !rootOK || uint64(root) >= uint64(len(b.sourceMaps[inputIndex])) {
			return nil, errors.New("typeauthority: Runtime receipt root mapping")
		}
		row := b.sourceMaps[inputIndex][root]
		if row == 0 {
			return nil, errors.New("typeauthority: Runtime receipt root unmapped")
		}
		canonicalInners[inputIndex] = RuntimeInner{owner: b.runtime, index: row}
	}
	b.keys = make([]runtimeSourceKey, len(representatives))
	for index, representative := range representatives {
		b.keys[index] = representative.key
	}
	return canonicalInners, nil
}

func runtimeSourceValue(value typ.Type) typ.Type {
	return typ.UnwrapStructuralWrappers(value)
}

func validateRuntimeSourceNode(nodes []typ.CanonicalGraphNode, node typ.CanonicalGraphNode) error {
	if !identity.ContentID(node.Identity).Available() {
		return errors.New("typeauthority: Runtime source node identity unavailable")
	}
	for _, child := range node.Children {
		if uint64(child) >= uint64(len(nodes)) {
			return errors.New("typeauthority: Runtime source child ordinal")
		}
	}
	if !node.Closed {
		if !identity.ContentID(node.Scope.Token).Available() || node.Scope.Formals == 0 {
			return errors.New("typeauthority: Runtime open source scope unavailable")
		}
	}
	// Binding.Ordinal is local to Binding.Owner. Its validity is established
	// once by the owner-issued immutable graph receipt; Scope.Formals belongs
	// to this occurrence and is not that binder's local arity. Runtime retains
	// only the owner bound needed before it indexes the source plane.
	if node.Bound && uint64(node.Binding.Owner) >= uint64(len(nodes)) {
		return errors.New("typeauthority: Runtime source binding owner")
	}
	return nil
}

func runtimeSourceKeyForNode(nodes []typ.CanonicalGraphNode, node typ.CanonicalGraphNode) (runtimeSourceKey, error) {
	key := runtimeSourceKey{
		identity: identity.ContentID(node.Identity), kind: node.Kind, closed: node.Closed,
	}
	if node.Closed {
		return key, nil
	}
	key.scope = identity.ContentID(node.Scope.Token)
	key.formals = node.Scope.Formals
	key.bound = node.Bound
	if node.Bound {
		owner := nodes[node.Binding.Owner]
		key.owner = identity.ContentID(owner.Identity)
		key.ordinal = node.Binding.Ordinal
	}
	return key, nil
}

func runtimeSourceTieLess(left, right runtimeSource) bool {
	if left.input != right.input {
		if left.input < 0 {
			return true
		}
		if right.input < 0 {
			return false
		}
		return left.input < right.input
	}
	return left.ordinal < right.ordinal
}

func runtimeSourceKeyCompare(left, right runtimeSourceKey) int {
	if result := bytes.Compare(left.identity[:], right.identity[:]); result != 0 {
		return result
	}
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	if left.closed != right.closed {
		if !left.closed {
			return -1
		}
		return 1
	}
	if result := bytes.Compare(left.scope[:], right.scope[:]); result != 0 {
		return result
	}
	if left.formals != right.formals {
		if left.formals < right.formals {
			return -1
		}
		return 1
	}
	if left.bound != right.bound {
		if !left.bound {
			return -1
		}
		return 1
	}
	if result := bytes.Compare(left.owner[:], right.owner[:]); result != 0 {
		return result
	}
	if left.ordinal < right.ordinal {
		return -1
	}
	if left.ordinal > right.ordinal {
		return 1
	}
	return 0
}

func runtimeSourceIdentity(key runtimeSourceKey) identity.ContentID {
	if key.closed {
		return key.identity
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime/scoped-row\x00\x02"))
	_, _ = hash.Write(key.identity[:])
	_, _ = hash.Write(key.scope[:])
	writeRuntimeWord(hash, uint64(key.formals))
	if key.bound {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write(key.owner[:])
	writeRuntimeWord(hash, uint64(key.ordinal))
	var result identity.ContentID
	copy(result[:], hash.Sum(nil))
	return result
}

func (b *runtimeBuilder) installReceiptEdges(index int, source runtimeSource) error {
	if b == nil || b.runtime == nil || index < 0 || index >= len(b.runtime.rows) {
		return errors.New("typeauthority: Runtime receipt edge row")
	}
	row := &b.runtime.rows[index]
	if source.node.Kind != row.form {
		return errors.New("typeauthority: Runtime receipt row kind")
	}
	child := func(ordinal uint32) (runtimeChild, error) {
		if source.input < 0 || source.input >= len(b.sourceMaps) || uint64(ordinal) >= uint64(len(b.sourceMaps[source.input])) {
			return runtimeChild{}, errors.New("typeauthority: Runtime receipt edge mapping")
		}
		index := b.sourceMaps[source.input][ordinal]
		if index == 0 {
			return runtimeChild{}, errors.New("typeauthority: Runtime receipt edge unmapped")
		}
		return runtimeChild{inner: RuntimeInner{owner: b.runtime, index: index}, present: true}, nil
	}
	switch row.form {
	case kind.Record:
		// Canonical record children begin with the direct named fields. Static
		// members, map components, and metatables follow that authored prefix
		// and intentionally do not enter this direct-field plane.
		record, recordOK := runtimeSourceValue(source.value).(*typ.Record)
		if !recordOK || record == nil {
			return errors.New("typeauthority: Runtime record source unavailable")
		}
		if uint64(len(record.Fields)) > uint64(len(source.node.Children)) {
			return errors.New("typeauthority: Runtime record field source arity")
		}
		if len(record.Fields) == 0 {
			return nil
		}
		fields := make(map[string]RuntimeField, len(record.Fields))
		for fieldIndex, field := range record.Fields {
			entry, err := child(source.node.Children[fieldIndex])
			if err != nil {
				return err
			}
			fields[field.Name] = RuntimeField{
				Inner: entry.inner, Optional: field.Optional, Readonly: field.Readonly,
			}
		}
		row.fields = fields
	case kind.Union:
		row.variants.start = uint32(len(b.runtime.variants))
		for _, ordinal := range source.node.Children {
			entry, err := child(ordinal)
			if err != nil {
				return err
			}
			b.runtime.variants = append(b.runtime.variants, entry)
		}
		row.variants.end = uint32(len(b.runtime.variants))
	case kind.Optional:
		if len(source.node.Children) != 1 {
			return errors.New("typeauthority: Runtime Optional source arity")
		}
		entry, err := child(source.node.Children[0])
		if err != nil {
			return err
		}
		row.inner = entry
	}
	return nil
}

func runtimeDenseOrdinal(length int) (uint32, error) {
	if length < 0 || uint64(length) >= uint64(math.MaxUint32) {
		return 0, errors.New("typeauthority: Runtime dense handle overflow")
	}
	return uint32(length + 1), nil
}

// sealRuntimeKinds publishes the owner-issued runtime-vocabulary column while
// the receipt construction sources are still live.
func (b *runtimeBuilder) sealRuntimeKinds() error {
	if b != nil && b.runtime != nil && b.runtime.runtimeKindsPublished {
		return nil
	}
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime kind source")
	}
	for index, value := range b.construction {
		kinds := runtimekind.All
		if !b.runtime.rows[index].scopedID.Available() {
			kinds = typ.MayRuntimeKinds(value)
		}
		if !kinds.Valid() {
			return errors.New("typeauthority: invalid Runtime kind projection")
		}
		b.runtime.rows[index].runtimeKinds = kinds
	}
	b.runtime.runtimeKindsPublished = true
	return nil
}

// sealCanonical publishes exact structural identity. Rows are already
// coalesced by owner-issued source key; no assignability relation rewrites the
// vector.
func (b *runtimeBuilder) sealCanonical() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime canonical source")
	}
	b.runtime.canonical = make([]uint32, len(b.runtime.rows)+1)
	for index := range b.runtime.rows {
		b.runtime.canonical[index+1] = uint32(index + 1)
		if !b.runtime.rows[index].canonicalID.Available() {
			return errors.New("typeauthority: closed Runtime row lacks canonical structural identity")
		}
	}
	return nil
}

// sealDescriptors derives the semantic union descriptor from only the direct
// Union and Optional feeds installed from receipt source ordinals.
func (b *runtimeBuilder) sealDescriptors() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) || len(b.runtime.canonical) != len(b.runtime.rows)+1 {
		return errors.New("typeauthority: malformed Runtime descriptor source")
	}
	runtime := b.runtime
	state := make([]uint8, len(runtime.rows))
	var visit func(int) ([]uint32, error)
	visit = func(index int) ([]uint32, error) {
		if index < 0 || index >= len(runtime.rows) {
			return nil, errors.New("typeauthority: Runtime descriptor index")
		}
		if runtime.rows[index].atoms != nil {
			return runtime.rows[index].atoms, nil
		}
		if state[index] == 1 {
			return []uint32{runtime.canonical[index+1]}, nil
		}
		state[index] = 1
		row := runtime.rows[index]
		atoms := make([]uint32, 0, 1)
		switch row.form {
		case kind.Union:
			if row.variants.start > row.variants.end || uint64(row.variants.end) > uint64(len(runtime.variants)) {
				return nil, errors.New("typeauthority: malformed Runtime union descriptor range")
			}
			for _, child := range runtime.variants[row.variants.start:row.variants.end] {
				if !child.present || !runtime.owns(child.inner) {
					return nil, errors.New("typeauthority: malformed Runtime union descriptor child")
				}
				childAtoms, err := visit(int(child.inner.index - 1))
				if err != nil {
					return nil, err
				}
				atoms = append(atoms, childAtoms...)
			}
		case kind.Optional:
			if runtime.nilRow == 0 {
				return nil, errors.New("typeauthority: Runtime Optional lacks nil row")
			}
			atoms = append(atoms, runtime.canonical[runtime.nilRow])
			if row.inner.present {
				if !runtime.owns(row.inner.inner) {
					return nil, errors.New("typeauthority: malformed Runtime Optional descriptor child")
				}
				childAtoms, err := visit(int(row.inner.inner.index - 1))
				if err != nil {
					return nil, err
				}
				atoms = append(atoms, childAtoms...)
			}
		}
		if len(atoms) == 0 {
			atoms = append(atoms, runtime.canonical[index+1])
		}
		for atomIndex, atom := range atoms {
			if atom == 0 || uint64(atom) > uint64(len(runtime.rows)) {
				return nil, errors.New("typeauthority: malformed Runtime semantic atom")
			}
			atoms[atomIndex] = runtime.canonical[atom]
		}
		sort.Slice(atoms, func(left, right int) bool { return atoms[left] < atoms[right] })
		unique := atoms[:0]
		for _, atom := range atoms {
			if len(unique) == 0 || unique[len(unique)-1] != atom {
				unique = append(unique, atom)
			}
		}
		if len(unique) == 0 {
			unique = append(unique, runtime.canonical[index+1])
		}
		runtime.rows[index].atoms = append([]uint32(nil), unique...)
		state[index] = 2
		return runtime.rows[index].atoms, nil
	}
	for index := range runtime.rows {
		if _, err := visit(index); err != nil {
			return err
		}
	}
	return nil
}

// sealFields validates the already-installed scalar direct-field plane while
// the builder still owns the construction graphs. Fresh and local rows have
// their fields installed by installReceiptEdges; rows copied from a Family
// prefix carry the immutable scalar plane forward and only need owner-fence
// validation here. This pass deliberately does not inspect or retain a typ
// graph in Runtime.
func (b *runtimeBuilder) sealFields() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime field source")
	}
	for index, row := range b.runtime.rows {
		for _, field := range row.fields {
			if !b.runtime.owns(field.Inner) {
				return errors.New("typeauthority: malformed Runtime field projection")
			}
		}
		if row.form == kind.Record && row.fields == nil {
			// A record with no named fields has no map to retain. A non-empty
			// record must have been installed from its source plane above.
			value := runtimeSourceValue(b.construction[index])
			if record, ok := value.(*typ.Record); ok && record != nil && len(record.Fields) != 0 {
				return errors.New("typeauthority: missing Runtime record field projection")
			}
		}
	}
	return nil
}

// sealRanks assigns the exact-singleton finite-set measure. Every closed row
// is one distinct atom, so each singleton has rank |P|.
func (r *Runtime) sealRanks() error {
	if r == nil || len(r.canonical) != len(r.rows)+1 {
		return errors.New("typeauthority: malformed Runtime rank source")
	}
	closed := uint64(0)
	for _, row := range r.rows {
		if !row.scopedID.Available() {
			closed++
		}
	}
	if closed == 0 || closed >= uint64(math.MaxUint32) {
		return errors.New("typeauthority: Runtime rank universe overflow")
	}
	for index := range r.rows {
		if !r.rows[index].scopedID.Available() {
			r.rows[index].rank = uint32(closed)
		}
	}
	return nil
}

func (r *Runtime) sealIdentity() error {
	if r == nil || !r.sourceID.Available() {
		return errors.New("typeauthority: unavailable Runtime identity source")
	}
	hash := sha256.New()
	// v7 adds the sealed direct-field projection plane. Structural source
	// graphs remain construction-only; only sorted field keys, dense child
	// handles, and optional/readonly bits enter Runtime identity.
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime\x00\x07"))
	_, _ = hash.Write(r.sourceID[:])
	writeRuntimeWord(hash, uint64(len(r.rows)))
	for _, row := range r.rows {
		writeRuntimeWord(hash, uint64(row.form))
		if row.scopedID.Available() {
			_, _ = hash.Write([]byte{0})
		} else {
			_, _ = hash.Write([]byte{1})
		}
		writeRuntimeWord(hash, uint64(row.runtimeKinds))
		if !row.canonicalID.Available() {
			return errors.New("typeauthority: Runtime row canonical identity unavailable")
		}
		_, _ = hash.Write(row.canonicalID[:])
		if row.scopedID.Available() {
			_, _ = hash.Write([]byte{1})
			_, _ = hash.Write(row.scopedID[:])
		} else {
			_, _ = hash.Write([]byte{0})
		}
		writeRuntimeChild(hash, row.inner)
		writeRuntimeWord(hash, uint64(row.variants.start))
		writeRuntimeWord(hash, uint64(row.variants.end))
		if err := writeRuntimeFields(hash, row.fields); err != nil {
			return err
		}
	}
	writeRuntimeWord(hash, uint64(len(r.variants)))
	for _, child := range r.variants {
		writeRuntimeChild(hash, child)
	}
	writeRuntimeWord(hash, uint64(len(r.canonical)))
	for _, canonical := range r.canonical {
		writeRuntimeWord(hash, uint64(canonical))
	}
	for _, row := range r.rows {
		writeRuntimeWord(hash, uint64(len(row.atoms)))
		for _, atom := range row.atoms {
			writeRuntimeWord(hash, uint64(atom))
		}
		writeRuntimeWord(hash, uint64(row.rank))
	}
	copy(r.id[:], hash.Sum(nil))
	if !r.id.Available() {
		return errors.New("typeauthority: unavailable Runtime content identity")
	}
	r.identities = make([]identity.ContentID, len(r.rows))
	for index := range r.rows {
		innerID := sha256.New()
		_, _ = innerID.Write([]byte("wippy.analysis.typeauthority.runtime/inner\x00\x01"))
		_, _ = innerID.Write(r.id[:])
		writeRuntimeWord(innerID, uint64(index+1))
		copy(r.identities[index][:], innerID.Sum(nil))
		if !r.identities[index].Available() {
			return errors.New("typeauthority: unavailable Runtime inner identity")
		}
	}
	return nil
}

func writeRuntimeWord(hash interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = hash.Write(word[:])
}

func writeRuntimeChild(hash interface{ Write([]byte) (int, error) }, child runtimeChild) {
	writeRuntimeWord(hash, uint64(child.inner.index))
	if child.present {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
}

func writeRuntimeFields(hash interface{ Write([]byte) (int, error) }, fields map[string]RuntimeField) error {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writeRuntimeWord(hash, uint64(len(keys)))
	for _, key := range keys {
		field := fields[key]
		if field.Inner.index == 0 {
			return errors.New("typeauthority: unavailable Runtime field identity")
		}
		writeRuntimeWord(hash, uint64(len(key)))
		_, _ = hash.Write([]byte(key))
		writeRuntimeWord(hash, uint64(field.Inner.index))
		if field.Optional {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		if field.Readonly {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
	}
	return nil
}

// LinkID identifies the exact sealed Link Runtime is fenced to.
func (r *Runtime) LinkID() identity.ContentID {
	if r == nil {
		return identity.ContentID{}
	}
	return r.sourceID
}

func (r *Runtime) ContentID() identity.ContentID {
	if r == nil {
		return identity.ContentID{}
	}
	return r.id
}

func (r *Runtime) Count() int {
	if r == nil {
		return 0
	}
	return len(r.rows)
}

// InnerAtIndex authenticates a Runtime-local one-based atom index.
func (r *Runtime) InnerAtIndex(index uint32) (RuntimeInner, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.rows)) {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: index}, true
}

func (r *Runtime) Index(inner RuntimeInner) (uint32, bool) {
	if !r.owns(inner) {
		return 0, false
	}
	return inner.index, true
}

// Closed reports whether inner belongs to Runtime's complete structural
// universe. Scoped/formal rows remain addressable for their own owners but are
// not admissible as facts in a total hot lattice.
func (r *Runtime) Closed(inner RuntimeInner) bool {
	return r.owns(inner) && len(r.closedPositions) == len(r.rows) && r.closedPositions[inner.index-1] >= 0
}

func (r *Runtime) owns(inner RuntimeInner) bool {
	return r != nil && inner.owner == r && inner.index != 0 && uint64(inner.index) <= uint64(len(r.rows))
}

func (r *Runtime) Equal(left, right RuntimeInner) bool {
	return r.owns(left) && r.owns(right) && left.index == right.index
}

func (r *Runtime) Identity(inner RuntimeInner) (identity.ContentID, bool) {
	if !r.owns(inner) || int(inner.index) > len(r.identities) {
		return identity.ContentID{}, false
	}
	id := r.identities[inner.index-1]
	return id, id.Available()
}

// Kind reports the structural category of one dense row.
func (r *Runtime) Kind(inner RuntimeInner) (kind.Kind, bool) {
	if !r.owns(inner) {
		return 0, false
	}
	return r.rows[inner.index-1].form, true
}

// RuntimeKinds reports the owner-issued runtime-vocabulary projection.
func (r *Runtime) RuntimeKinds(inner RuntimeInner) (runtimekind.Set, bool) {
	if !r.owns(inner) || !r.runtimeKindsPublished {
		return 0, false
	}
	kinds := r.rows[inner.index-1].runtimeKinds
	return kinds, kinds.Valid()
}

// CanonicalIdentity is the owner-neutral identity of one closed structural
// row. Runtime issues it once so consumers never re-encode a source graph.
func (r *Runtime) CanonicalIdentity(inner RuntimeInner) (identity.ContentID, bool) {
	if !r.owns(inner) || r.rows[inner.index-1].scopedID.Available() || !r.rows[inner.index-1].canonicalID.Available() {
		return identity.ContentID{}, false
	}
	id := r.rows[inner.index-1].canonicalID
	return id, id.Available()
}

// StructuralEqual is the sealed structural judgment. Open rows are exact only
// against their own owner-local identity; unlike closed rows they do not claim
// an independent portable equality decision.
func (r *Runtime) StructuralEqual(left, right RuntimeInner) (answer, decided bool) {
	if !r.owns(left) || !r.owns(right) {
		return false, false
	}
	if left.index == right.index {
		return true, true
	}
	return false, !r.rows[left.index-1].scopedID.Available() && !r.rows[right.index-1].scopedID.Available()
}

// Subtype is the sealed owner-local subtype judgment. The answer for one
// ordered pair is decided by the canonical checker the first time it is asked
// for and read from the relation's memory afterwards.
func (r *Runtime) Subtype(left, right RuntimeInner) (answer, decided bool) {
	if !r.owns(left) || !r.owns(right) {
		return false, false
	}
	if left.index == right.index {
		return true, true
	}
	if len(r.closedPositions) != len(r.rows) || len(r.sources) != len(r.rows) {
		return false, false
	}
	if r.closedPositions[left.index-1] < 0 || r.closedPositions[right.index-1] < 0 {
		return false, false
	}
	return r.relation.decide(left.index, right.index, r.sources[left.index-1], r.sources[right.index-1])
}

// Canonical returns the semantic-equivalence representative used by Runtime
// descriptors. Exact Runtime identities remain distinct.
func (r *Runtime) Canonical(inner RuntimeInner) (RuntimeInner, bool) {
	if !r.owns(inner) || len(r.canonical) != len(r.rows)+1 || r.canonical[inner.index] == 0 {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: r.canonical[inner.index]}, true
}

// DescriptorCount and DescriptorAt expose Runtime's immutable semantic-union
// descriptor without exposing dense row identities or construction graphs.
func (r *Runtime) DescriptorCount(inner RuntimeInner) int {
	if !r.owns(inner) {
		return 0
	}
	return len(r.rows[inner.index-1].atoms)
}

func (r *Runtime) DescriptorAt(inner RuntimeInner, index int) (RuntimeInner, bool) {
	if !r.owns(inner) || index < 0 || index >= len(r.rows[inner.index-1].atoms) {
		return RuntimeInner{}, false
	}
	atom := r.rows[inner.index-1].atoms[index]
	if atom == 0 || uint64(atom) > uint64(len(r.rows)) {
		return RuntimeInner{}, false
	}
	return RuntimeInner{owner: r, index: atom}, true
}

// Field returns one exact direct record field in constant time. The lookup is
// owner-fenced on both the receiver and the projected child; foreign or zero
// inners are absent. A sealed map read returns the scalar projection without
// allocation and without consulting any construction graph.
func (r *Runtime) Field(inner RuntimeInner, key string) (RuntimeField, bool) {
	if !r.owns(inner) {
		return RuntimeField{}, false
	}
	field, ok := r.rows[inner.index-1].fields[key]
	if !ok || !r.owns(field.Inner) {
		return RuntimeField{}, false
	}
	return field, true
}
