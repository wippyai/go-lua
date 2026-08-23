// Package diagram owns immutable sparse fact decision diagrams.  A typed
// (factor,key) column indexes one reduced ordered MTBDD whose order is the
// shared sealed guard order and whose terminals are typed arena identities.
// It contains no lattice, rule, solver, or dependency policy.
package diagram

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Config defines one finite ordered factor schema and its sealed terminal and
// guard universes.  The supplied Factor sequence, not numeric spelling, is
// canonical column order.
type Config[F ~uint64, K scalar.Key, V any] struct {
	Factors   []F
	Terminals *terminal.Arena[V]
	Guards    *guard.Manager
}

// Value is the opaque reduced MTBDD stored in one fact column.  Its diagram
// authority is retained with the handle: an FDD from another terminal/guard
// generation may never be spliced into this one by accident.
type Value[V any] struct {
	owner *diagramOwner
	node  *node[V]
}

// Fact identifies one populated factor/key column and its decision function.
type Fact[F ~uint64, K scalar.Key, V any] struct {
	Factor F
	Key    K
	Value  Value[V]
}

// Diagram owns a frozen schema.  It owns no candidate cache: Builder owns all
// interning and drops it at the publication cut.
type Diagram[F ~uint64, K scalar.Key, V any] struct {
	owner     *diagramOwner
	ranks     map[F]uint32
	factors   []F
	terminals *terminal.Arena[V]
	guards    *guard.Manager

	// partitions lends the value partition its node tables. It is reuse
	// storage for one read, never a candidate cache: nothing a read puts back
	// survives the Seal that published its cells.
	partitions sync.Pool
}

// diagramOwner is a non-generic opaque generation token retained by Values.
// It avoids permitting a Value from another diagram with an accidentally
// compatible Go instantiation into a Builder.
type diagramOwner struct{ marker byte }

// Root is a persistent sparse collection of fact columns.  A Diagram reader
// accepts it only after Builder.Seal; candidate roots cannot escape as state.
type Root[F ~uint64, K scalar.Key, V any] struct {
	diagram *Diagram[F, K, V]
	root    *factorNode[F, K, V]
	count   int
	sealed  bool
	lease   leaseRef
}

// Builder is a single-writer construction scope.  It structurally shares
// prior columns and has one local FDD unique table across every column, which
// preserves shared correlation without a second product carrier.
type Builder[F ~uint64, K scalar.Key, V any] struct {
	diagram           *Diagram[F, K, V]
	terminalAuthority interface{ Valid(terminal.ID[V]) bool }
	terminalWork      *terminal.Work[V]
	lease             *lease
	token             leaseRef
	open              bool
	terminals         map[terminal.ID[V]]*node[V]
	decisions         map[decisionKey[V]]*node[V]
	updates           map[updateKey[V]]*node[V]
	imports           map[*node[V]]*node[V]

	// The memos below are transaction-owned reusable storage for the pointwise
	// operations. Their keys name only operands, never the operation applied,
	// so every one of them is reset when its operation begins: this is storage
	// reuse across the many columns of one transaction, never result reuse
	// across two different operations. Each nested walk keeps its own field so
	// an outer operation and the inner combine it drives cannot alias.
	zipWork        map[zipKey[V]]*node[V]
	transformWork  map[transformKey[V]]*node[V]
	transformNodes map[*node[V]]*node[V]
	existsWork     map[existsKey[V]]*node[V]
	existsZip      map[zipKey[V]]*node[V]
	reindexNodes   map[*node[V]]*node[V]
	reindexZip     map[zipKey[V]]*node[V]
	reindexITEWork map[reindexITEKey[V]]*node[V]
	// maskWork is the one memo that survives its operation. Mask carries no
	// callback at all, so (node, region) already names its result exactly and
	// a transaction that restricts many columns to the same authored region
	// proves each shared suffix once.
	maskWork map[maskKey[V]]*node[V]
}

// reuseMemo hands back caller-owned map storage emptied for one operation. A
// nil map is created on first use, so a Builder that never runs an operation
// never allocates its memo.
func reuseMemo[T comparable, R any](storage map[T]R) map[T]R {
	if storage == nil {
		return make(map[T]R)
	}
	clear(storage)
	return storage
}

// lease is the identity of one Builder's candidate scope. A Builder is
// reusable storage, so the identity is a pointer plus the generation it is
// currently issuing: a root from an earlier transaction of the same Builder
// carries an earlier generation and is refused rather than silently accepted.
type lease struct{ generation uint64 }

// leaseRef is the candidate-scope stamp a root carries. Its zero value names a
// sealed root, which belongs to the Diagram rather than to any transaction.
type leaseRef struct {
	owner      *lease
	generation uint64
}

type factorNode[F ~uint64, K scalar.Key, V any] struct {
	factor F
	rank   uint32
	keys   *keyNode[K, V]
	left   *factorNode[F, K, V]
	right  *factorNode[F, K, V]
	height int8
}

type keyNode[K scalar.Key, V any] struct {
	key    K
	value  *node[V]
	left   *keyNode[K, V]
	right  *keyNode[K, V]
	height int8
}

// node is one reduced ordered MTBDD node.  A terminal carries only a typed
// terminal identity; the zero identity is the exact undefined terminal.
type node[V any] struct {
	terminal bool
	value    terminal.ID[V]
	atom     guard.Atom
	low      *node[V]
	high     *node[V]
}

// valuePartitionFrame is operation-local traversal state for one FDD
// partition. Keeping it explicit avoids consuming the Go call stack for an
// arbitrarily deep ordered decision chain.
type valuePartitionFrame[V any] struct {
	value  *node[V]
	region support.Mask
}

type terminalCell[V any] struct {
	terminal terminal.ID[V]
	region   support.Mask
}

type decisionKey[V any] struct {
	atom      guard.Atom
	low, high *node[V]
}

type updateKey[V any] struct {
	prior *node[V]
	when  support.Mask
	value terminal.ID[V]
}

// Combine is a typed terminal operation used by one synchronized FDD
// traversal.  The zero terminal denotes undefined; semantic code decides how
// undefined participates in its lattice law.  A false result rejects the
// whole candidate operation rather than allowing an invalid terminal into a
// published diagram.
type Combine[V any] func(left, right terminal.ID[V]) (terminal.ID[V], bool)

// Transform maps terminals only where one exact support region holds.  It is
// structural: callers own terminal meaning.  Unlike Mask+Zip with a constant
// input, Transform can admit only surviving mapped terminals, avoiding cold
// terminal-page growth for an update that collapses back to sparse default.
type Transform[V any] func(terminal.ID[V]) (terminal.ID[V], bool)

// New freezes Diagram's three universes.  A changed factor order, terminal
// arena, or guard manager is a distinct fact-generation identity.
func New[F ~uint64, K scalar.Key, V any](config Config[F, K, V]) (*Diagram[F, K, V], bool) {
	if config.Terminals == nil || !config.Terminals.Sealed() || config.Guards == nil || !config.Guards.Valid(config.Guards.True()) {
		return nil, false
	}
	diagram := &Diagram[F, K, V]{
		owner: &diagramOwner{},
		ranks: make(map[F]uint32, len(config.Factors)), factors: append([]F(nil), config.Factors...),
		terminals: config.Terminals, guards: config.Guards,
	}
	for index, factor := range diagram.factors {
		if _, exists := diagram.ranks[factor]; exists {
			return nil, false
		}
		diagram.ranks[factor] = uint32(index)
	}
	return diagram, true
}

// Empty returns the canonical sealed root with no populated columns.
func (diagram *Diagram[F, K, V]) Empty() Root[F, K, V] {
	if diagram == nil {
		return Root[F, K, V]{}
	}
	return Root[F, K, V]{diagram: diagram, sealed: true}
}

// Guards returns this Diagram's sealed guard-order authority.  It is exposed
// only to adjacent internal fact algebra so support operations can prove they
// share the one valuation universe before touching any typed column.
func (diagram *Diagram[F, K, V]) Guards() *guard.Manager {
	if diagram == nil {
		return nil
	}
	return diagram.guards
}

// Terminals returns this Diagram's immutable typed terminal semantic owner.
// Adjacent fact algebra uses it only to bind a candidate terminal Work to the
// exact same diagram; terminal identities themselves remain opaque.
func (diagram *Diagram[F, K, V]) Terminals() *terminal.Arena[V] {
	if diagram == nil {
		return nil
	}
	return diagram.terminals
}

// SoleFactor returns the column family when this Diagram was deliberately
// constructed for one typed Binding.  A heterogeneous carrier composes such
// bindings through sealed physical slots instead of another diagram schema.
func (diagram *Diagram[F, K, V]) SoleFactor() (F, bool) {
	var zero F
	if diagram == nil || len(diagram.factors) != 1 {
		return zero, false
	}
	return diagram.factors[0], true
}

// Begin opens one candidate FDD unique table.  No semantic fact budget or
// cardinality ceiling is introduced by those local allocation tables.
func (diagram *Diagram[F, K, V]) Begin() *Builder[F, K, V] {
	if diagram == nil {
		return nil
	}
	return diagram.begin(diagram.terminals)
}

// BeginWithTerminals opens one candidate FDD scope tied to an exact candidate
// terminal page over this Diagram's immutable semantic owner.  The Work is
// private until its coordinated Seal; ordinary Diagram readers cannot accept
// its IDs beforehand.
func (diagram *Diagram[F, K, V]) BeginWithTerminals(values *terminal.Work[V]) *Builder[F, K, V] {
	if diagram == nil || values == nil || values.Base() != diagram.terminals {
		return nil
	}
	builder := diagram.begin(values)
	builder.terminalWork = values
	return builder
}

func (diagram *Diagram[F, K, V]) begin(values interface{ Valid(terminal.ID[V]) bool }) *Builder[F, K, V] {
	builder := &Builder[F, K, V]{}
	builder.reopen(diagram, values)
	return builder
}

// BeginWithTerminalsInto opens the same candidate scope over caller-owned
// Builder storage. The unique tables and the transaction memos are reset, not
// reallocated, so a warm write transaction opens without allocating: a Builder
// is the reusable scratch of one write, never a value that outlives it.
func (diagram *Diagram[F, K, V]) BeginWithTerminalsInto(builder *Builder[F, K, V], values *terminal.Work[V]) bool {
	if diagram == nil || builder == nil || builder.open || values == nil || values.Base() != diagram.terminals {
		return false
	}
	builder.reopen(diagram, values)
	builder.terminalWork = values
	return true
}

// reopen installs one candidate scope over this Builder's storage. Every map
// is cleared rather than dropped, and the scope's generation advances so a root
// stamped by an earlier transaction is no longer this Builder's.
func (builder *Builder[F, K, V]) reopen(diagram *Diagram[F, K, V], values interface{ Valid(terminal.ID[V]) bool }) {
	if builder.lease == nil {
		builder.lease = &lease{}
	}
	builder.lease.generation++
	builder.token = leaseRef{owner: builder.lease, generation: builder.lease.generation}
	builder.diagram = diagram
	builder.terminalAuthority = values
	builder.terminalWork = nil
	builder.open = true
	if builder.terminals == nil {
		builder.terminals = make(map[terminal.ID[V]]*node[V])
		builder.decisions = make(map[decisionKey[V]]*node[V])
		builder.updates = make(map[updateKey[V]]*node[V])
		builder.imports = make(map[*node[V]]*node[V])
		return
	}
	clear(builder.terminals)
	clear(builder.decisions)
	clear(builder.updates)
	clear(builder.imports)
	builder.releaseMemos()
}

// Valid reports whether root is a sealed root from this exact Diagram.
func (diagram *Diagram[F, K, V]) Valid(root Root[F, K, V]) bool {
	return diagram != nil && root.diagram == diagram && root.sealed && root.lease == leaseRef{}
}

// Count returns the number of populated sparse columns, not the number of
// guarded terminals within them.
func (diagram *Diagram[F, K, V]) Count(root Root[F, K, V]) (int, bool) {
	if !diagram.Valid(root) {
		return 0, false
	}
	return root.count, true
}

// Equal proves exact semantic equality of two sealed sparse roots from this
// Diagram.  It compares coordinates and reduced decision functions, using the
// terminal arena's semantic equality rather than candidate-page ID spelling.
// This is intentionally a publication-boundary operation: candidate builders
// have their own local terminal authority and must not use it for wake logic.
func (diagram *Diagram[F, K, V]) Equal(left, right Root[F, K, V]) bool {
	if !diagram.Valid(left) || !diagram.Valid(right) {
		return false
	}
	if left.root == right.root {
		return true
	}
	if left.count != right.count {
		return false
	}
	type coordinate struct {
		factor F
		key    K
	}
	columns := make(map[coordinate]*node[V], left.count)
	walk := func(root *factorNode[F, K, V], visit func(F, K, *node[V]) bool) bool {
		factors := make([]*factorNode[F, K, V], 0)
		current := root
		for current != nil || len(factors) != 0 {
			for current != nil {
				factors = append(factors, current)
				current = current.left
			}
			last := len(factors) - 1
			current = factors[last]
			factors = factors[:last]
			keys := make([]*keyNode[K, V], 0)
			key := current.keys
			for key != nil || len(keys) != 0 {
				for key != nil {
					keys = append(keys, key)
					key = key.left
				}
				keyLast := len(keys) - 1
				key = keys[keyLast]
				keys = keys[:keyLast]
				if !visit(current.factor, key.key, key.value) {
					return false
				}
				key = key.right
			}
			current = current.right
		}
		return true
	}
	if !walk(left.root, func(factor F, key K, value *node[V]) bool {
		columns[coordinate{factor: factor, key: key}] = value
		return true
	}) {
		return false
	}
	if len(columns) != left.count {
		return false
	}
	matched := 0
	if !walk(right.root, func(factor F, key K, value *node[V]) bool {
		leftValue, found := columns[coordinate{factor: factor, key: key}]
		if !found || !diagram.equalValue(leftValue, value) {
			return false
		}
		matched++
		return true
	}) {
		return false
	}
	return matched == len(columns)
}

// Valid accepts sealed predecessors and roots private to this Builder.
func (builder *Builder[F, K, V]) Valid(root Root[F, K, V]) bool {
	return builder != nil && builder.open && root.diagram == builder.diagram && (builder.diagram.Valid(root) || !root.sealed && root.lease == builder.token)
}

// Set strongly overwrites the exact support region for one fact column:
// D'[factor,key] = ite(when, value, D[factor,key]).  Thus old values cannot
// resurrect under a new guard partition.  `when` is a full BDD, not a mask
// approximation; `value` must be a terminal from this exact sealed arena.
func (builder *Builder[F, K, V]) Set(root Root[F, K, V], factor F, key K, when support.Mask, value terminal.ID[V]) (Root[F, K, V], bool) {
	if !builder.Valid(root) || !when.Valid() || when.Manager() != builder.diagram.guards || builder.terminalAuthority == nil || !builder.terminalAuthority.Valid(value) {
		return Root[F, K, V]{}, false
	}
	rank, admitted := builder.diagram.ranks[factor]
	if !admitted {
		return Root[F, K, V]{}, false
	}
	column := findFactor(root.root, rank)
	var stored *node[V]
	var prior *node[V]
	if column != nil {
		if stored = columnValue(column.keys, key); stored != nil {
			prior = builder.importNode(stored)
		}
	}
	if prior == nil {
		prior = builder.terminal(terminal.ID[V]{})
	}
	next := builder.update(prior, when, value)
	if next == prior {
		return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, lease: builder.token}, true
	}
	columns, added := setFactor(root.root, rank, factor, key, next)
	count := root.count
	if added {
		count++
	}
	return Root[F, K, V]{diagram: builder.diagram, root: columns, count: count, lease: builder.token}, true
}

// Delete strongly writes undefined under when.  The sparse column itself is
// removed only when the resulting FDD is identically undefined.
func (builder *Builder[F, K, V]) Delete(root Root[F, K, V], factor F, key K, when support.Mask) (Root[F, K, V], bool) {
	if !builder.Valid(root) || !when.Valid() || when.Manager() != builder.diagram.guards {
		return Root[F, K, V]{}, false
	}
	rank, admitted := builder.diagram.ranks[factor]
	if !admitted {
		return Root[F, K, V]{}, false
	}
	column := findFactor(root.root, rank)
	if column == nil {
		return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, lease: builder.token}, true
	}
	prior := columnValue(column.keys, key)
	if prior == nil {
		return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, lease: builder.token}, true
	}
	prior = builder.importNode(prior)
	next := builder.update(prior, when, terminal.ID[V]{})
	if next == prior {
		return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, lease: builder.token}, true
	}
	if next.terminal && next.value == (terminal.ID[V]{}) {
		columns, removed := deleteFactor(root.root, rank, key)
		count := root.count
		if removed {
			count--
		}
		return Root[F, K, V]{diagram: builder.diagram, root: columns, count: count, lease: builder.token}, true
	}
	columns, _ := setFactor(root.root, rank, factor, key, next)
	return Root[F, K, V]{diagram: builder.diagram, root: columns, count: root.count, lease: builder.token}, true
}

// Seal publishes root and drops every builder-local FDD cache.  Sealed roots
// retain only their reachable immutable nodes.
func (builder *Builder[F, K, V]) Seal(root Root[F, K, V]) (Root[F, K, V], bool) {
	if !builder.Valid(root) {
		return Root[F, K, V]{}, false
	}
	// A candidate FDD and its candidate terminal page have one publication
	// cut.  The terminal page is made immutable first; the base Arena then
	// recognizes those IDs through its stable semantic owner before this root
	// becomes externally readable.
	if builder.terminalWork != nil {
		if _, ok := builder.terminalWork.Seal(); !ok {
			return Root[F, K, V]{}, false
		}
	}
	builder.close()
	builder.terminalWork = nil
	return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, sealed: true}, true
}

// Discard revokes candidate roots and releases all candidate caches.  The
// candidate terminal page is dropped with them: its values never reach the
// arena's sealed intern generation.
func (builder *Builder[F, K, V]) Discard() {
	if builder == nil {
		return
	}
	builder.terminalWork.Discard()
	builder.close()
	builder.terminalWork = nil
}

// Get returns one opaque FDD value for a populated column.  It does not pick
// a terminal; callers must use At with a concrete guard valuation.
func (diagram *Diagram[F, K, V]) Get(root Root[F, K, V], factor F, key K) (Value[V], bool, bool) {
	if !diagram.Valid(root) {
		return Value[V]{}, false, false
	}
	return diagram.get(root, factor, key)
}

// Get is Builder's candidate counterpart to Diagram.Get.
func (builder *Builder[F, K, V]) Get(root Root[F, K, V], factor F, key K) (Value[V], bool, bool) {
	if !builder.Valid(root) {
		return Value[V]{}, false, false
	}
	return builder.diagram.get(root, factor, key)
}

// Constant returns the canonical FDD terminal for id.  The zero ID is the
// exact undefined terminal and is deliberately accepted here even though it
// cannot be written through Set.
func (builder *Builder[F, K, V]) Constant(id terminal.ID[V]) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validTerminal(id) {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: builder.terminal(id)}, true
}

// Put replaces one sparse column with an already-built FDD from this exact
// Builder.  A uniformly undefined FDD removes the physical column, preserving
// sparse shape without treating undefined as an ordinary terminal value.
func (builder *Builder[F, K, V]) Put(root Root[F, K, V], factor F, key K, value Value[V]) (Root[F, K, V], bool) {
	if !builder.Valid(root) || !builder.validValue(value) {
		return Root[F, K, V]{}, false
	}
	rank, admitted := builder.diagram.ranks[factor]
	if !admitted {
		return Root[F, K, V]{}, false
	}
	var stored *node[V]
	if column := findFactor(root.root, rank); column != nil {
		stored = columnValue(column.keys, key)
	}
	if stored == value.node {
		return Root[F, K, V]{diagram: builder.diagram, root: root.root, count: root.count, lease: builder.token}, true
	}
	if value.node.terminal && value.node.value == (terminal.ID[V]{}) {
		columns, removed := deleteFactor(root.root, rank, key)
		count := root.count
		if removed {
			count--
		}
		return Root[F, K, V]{diagram: builder.diagram, root: columns, count: count, lease: builder.token}, true
	}
	columns, added := setFactor(root.root, rank, factor, key, value.node)
	count := root.count
	if added {
		count++
	}
	return Root[F, K, V]{diagram: builder.diagram, root: columns, count: count, lease: builder.token}, true
}

// Zip applies operation pointwise to two FDDs over their common guard order.
// It is structural only: the operation owns all lattice meaning.  The walk is
// synchronized, so shared guard partitions remain correlated rather than
// being reconstructed as independent Boolean masks.
func (builder *Builder[F, K, V]) Zip(left, right Value[V], operation Combine[V]) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validValue(left) || !builder.validValue(right) || operation == nil {
		return Value[V]{}, false
	}
	builder.zipWork = reuseMemo(builder.zipWork)
	result, valid := builder.zip(left.node, right.node, operation, builder.zipWork)
	if !valid {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: result}, true
}

// Transform applies operation to every terminal selected by when and retains
// every terminal outside it exactly.  `when` is consumed directly as the one
// shared BDD region; no per-column support is constructed or retained.
func (builder *Builder[F, K, V]) Transform(value Value[V], when support.Mask, operation Transform[V]) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validValue(value) || !when.Valid() || when.Manager() != builder.diagram.guards || operation == nil {
		return Value[V]{}, false
	}
	builder.transformWork, builder.transformNodes = reuseMemo(builder.transformWork), reuseMemo(builder.transformNodes)
	result, valid := builder.transform(builder.importNode(value.node), when, operation, builder.transformWork, builder.transformNodes)
	if !valid {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: result}, true
}

type transformKey[V any] struct {
	node *node[V]
	when support.Mask
}

func (builder *Builder[F, K, V]) transform(node *node[V], when support.Mask, operation Transform[V], cache map[transformKey[V]]*node[V], all map[*node[V]]*node[V]) (*node[V], bool) {
	key := transformKey[V]{node: node, when: when}
	if cached, found := cache[key]; found {
		return cached, true
	}
	view, valid := when.Decompose()
	if !valid {
		return nil, false
	}
	if view.Terminal {
		if !view.Value {
			cache[key] = node
			return node, true
		}
		result, valid := builder.transformAll(node, operation, all)
		if valid {
			cache[key] = result
		}
		return result, valid
	}
	whenRank, valid := builder.diagram.guards.Rank(view.Atom)
	if !valid {
		return nil, false
	}
	if node.terminal {
		low, valid := builder.transform(node, view.Low, operation, cache, all)
		if !valid {
			return nil, false
		}
		high, valid := builder.transform(node, view.High, operation, cache, all)
		if !valid {
			return nil, false
		}
		result := builder.decision(view.Atom, low, high)
		cache[key] = result
		return result, true
	}
	nodeRank, valid := builder.diagram.guards.Rank(node.atom)
	if !valid {
		return nil, false
	}
	if nodeRank < whenRank {
		low, valid := builder.transform(node.low, when, operation, cache, all)
		if !valid {
			return nil, false
		}
		high, valid := builder.transform(node.high, when, operation, cache, all)
		if !valid {
			return nil, false
		}
		result := builder.decision(node.atom, low, high)
		cache[key] = result
		return result, true
	}
	if nodeRank == whenRank {
		low, valid := builder.transform(node.low, view.Low, operation, cache, all)
		if !valid {
			return nil, false
		}
		high, valid := builder.transform(node.high, view.High, operation, cache, all)
		if !valid {
			return nil, false
		}
		result := builder.decision(node.atom, low, high)
		cache[key] = result
		return result, true
	}
	low, valid := builder.transform(node, view.Low, operation, cache, all)
	if !valid {
		return nil, false
	}
	high, valid := builder.transform(node, view.High, operation, cache, all)
	if !valid {
		return nil, false
	}
	result := builder.decision(view.Atom, low, high)
	cache[key] = result
	return result, true
}

func (builder *Builder[F, K, V]) transformAll(node *node[V], operation Transform[V], cache map[*node[V]]*node[V]) (*node[V], bool) {
	if cached, found := cache[node]; found {
		return cached, true
	}
	if node.terminal {
		value, valid := operation(node.value)
		if !valid || !builder.validTerminal(value) {
			return nil, false
		}
		result := builder.terminal(value)
		cache[node] = result
		return result, true
	}
	low, valid := builder.transformAll(node.low, operation, cache)
	if !valid {
		return nil, false
	}
	high, valid := builder.transformAll(node.high, operation, cache)
	if !valid {
		return nil, false
	}
	result := builder.decision(node.atom, low, high)
	cache[node] = result
	return result, true
}

type zipKey[V any] struct {
	left, right *node[V]
}

func (builder *Builder[F, K, V]) zip(left, right *node[V], operation Combine[V], cache map[zipKey[V]]*node[V]) (*node[V], bool) {
	key := zipKey[V]{left: left, right: right}
	if cached, found := cache[key]; found {
		return cached, true
	}
	if left.terminal && right.terminal {
		result, valid := operation(left.value, right.value)
		if !valid || !builder.validTerminal(result) {
			return nil, false
		}
		node := builder.terminal(result)
		cache[key] = node
		return node, true
	}
	var atom guard.Atom
	leftLow, leftHigh := left, left
	rightLow, rightHigh := right, right
	switch {
	case left.terminal:
		atom = right.atom
		rightLow, rightHigh = right.low, right.high
	case right.terminal:
		atom = left.atom
		leftLow, leftHigh = left.low, left.high
	default:
		leftRank, leftValid := builder.diagram.guards.Rank(left.atom)
		rightRank, rightValid := builder.diagram.guards.Rank(right.atom)
		if !leftValid || !rightValid {
			return nil, false
		}
		if leftRank < rightRank {
			atom = left.atom
			leftLow, leftHigh = left.low, left.high
		} else if rightRank < leftRank {
			atom = right.atom
			rightLow, rightHigh = right.low, right.high
		} else {
			atom = left.atom
			leftLow, leftHigh = left.low, left.high
			rightLow, rightHigh = right.low, right.high
		}
	}
	low, valid := builder.zip(leftLow, rightLow, operation, cache)
	if !valid {
		return nil, false
	}
	high, valid := builder.zip(leftHigh, rightHigh, operation, cache)
	if !valid {
		return nil, false
	}
	result := builder.decisionOrExisting(atom, low, high, left, right)
	cache[key] = result
	return result, true
}

// Mask preserves value precisely where region holds and writes undefined
// everywhere else.  It is the bridge between global joint support and one
// typed column; support remains one shared Boolean carrier, never a shadow
// mask stored beside each fact.
func (builder *Builder[F, K, V]) Mask(value Value[V], region support.Mask) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validValue(value) || !region.Valid() || region.Manager() != builder.diagram.guards {
		return Value[V]{}, false
	}
	if builder.maskWork == nil {
		builder.maskWork = make(map[maskKey[V]]*node[V])
	}
	result, valid := builder.mask(value.node, region, builder.maskWork)
	if !valid {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: result}, true
}

// Exists removes one guard decision from a typed FDD.  At the discharged
// boundary the two cofactors are combined by operation; this is existential
// abstraction over fact values, not a syntactic branch deletion.  Nodes below
// the target order are shared verbatim and no valuation is enumerated.
func (builder *Builder[F, K, V]) Exists(value Value[V], atom guard.Atom, operation Combine[V]) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validValue(value) || operation == nil {
		return Value[V]{}, false
	}
	rank, admitted := builder.diagram.guards.Rank(atom)
	if !admitted {
		return Value[V]{}, false
	}
	builder.existsWork, builder.existsZip = reuseMemo(builder.existsWork), reuseMemo(builder.existsZip)
	result, valid := builder.exists(value.node, rank, operation, builder.existsWork, builder.existsZip)
	if !valid {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: result}, true
}

// Reindex transports one symbolic value through a sealed coordinate relation.
// Mapped source decisions are rebuilt through their target expressions;
// forgotten decisions combine their two reachable fibers with operation. The
// caller must first totalize the value over source support, so a zero terminal
// here means an unreachable source fiber rather than a domain Default.
func (builder *Builder[F, K, V]) Reindex(value Value[V], relation guard.Reindex, operation Combine[V]) (Value[V], bool) {
	if builder == nil || !builder.open || !builder.validValue(value) || !relation.Valid() || relation.Source().Manager() != builder.diagram.guards || relation.Target().Manager() != builder.diagram.guards || operation == nil {
		return Value[V]{}, false
	}
	if relation.Identity() {
		return value, true
	}
	builder.reindexNodes, builder.reindexZip = reuseMemo(builder.reindexNodes), reuseMemo(builder.reindexZip)
	if relation.PureProjection() {
		result, ok := builder.reindexProjection(value.node, relation, operation, builder.reindexNodes, builder.reindexZip)
		if !ok {
			return Value[V]{}, false
		}
		return Value[V]{owner: builder.diagram.owner, node: result}, true
	}
	builder.reindexITEWork = reuseMemo(builder.reindexITEWork)
	result, ok := builder.reindex(value.node, relation, operation, builder.reindexNodes, builder.reindexZip, builder.reindexITEWork)
	if !ok {
		return Value[V]{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: result}, true
}

// reindexProjection transports a pure coordinate projection without building
// target-region ITEs. Retained coordinates are rebuilt directly at the same
// atom; forgotten coordinates alone invoke the caller's existential combine.
// The input-node memo is enough to share recursive source structure, while
// the existing zip memo keeps the forgotten-fiber joins canonical.
func (builder *Builder[F, K, V]) reindexProjection(input *node[V], relation guard.Reindex, operation Combine[V], cache map[*node[V]]*node[V], zipCache map[zipKey[V]]*node[V]) (*node[V], bool) {
	if input.terminal {
		return input, true
	}
	if cached, ok := cache[input]; ok {
		return cached, true
	}
	action, ok := relation.ProjectionAction(input.atom)
	if !ok {
		return nil, false
	}
	low, ok := builder.reindexProjection(input.low, relation, operation, cache, zipCache)
	if !ok {
		return nil, false
	}
	high, ok := builder.reindexProjection(input.high, relation, operation, cache, zipCache)
	if !ok {
		return nil, false
	}
	var result *node[V]
	switch {
	case action.RetainsCoordinate():
		if low == input.low && high == input.high {
			// A separately-issued coordinate identity changes only the Scope
			// wrapper, not the Boolean payload. Preserve the exact FDD node when
			// the recursive children are unchanged.
			result = input
		} else {
			result = builder.decision(input.atom, low, high)
		}
	case action.ForgetsCoordinate():
		result, ok = builder.zip(low, high, operation, zipCache)
		if !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	cache[input] = result
	return result, true
}

type reindexITEKey[V any] struct {
	condition guard.Guard
	then      *node[V]
	otherwise *node[V]
}

func (builder *Builder[F, K, V]) reindex(input *node[V], relation guard.Reindex, operation Combine[V], cache map[*node[V]]*node[V], zipCache map[zipKey[V]]*node[V], ite map[reindexITEKey[V]]*node[V]) (*node[V], bool) {
	if input.terminal {
		return input, true
	}
	if cached, ok := cache[input]; ok {
		return cached, true
	}
	action, ok := relation.Action(input.atom)
	if !ok {
		return nil, false
	}
	low, ok := builder.reindex(input.low, relation, operation, cache, zipCache, ite)
	if !ok {
		return nil, false
	}
	high, ok := builder.reindex(input.high, relation, operation, cache, zipCache, ite)
	if !ok {
		return nil, false
	}
	lowCondition, lowValid := action.Low()
	highCondition, highValid := action.High()
	if !lowValid || !highValid {
		return nil, false
	}
	zero := builder.terminal(terminal.ID[V]{})
	low, ok = builder.reindexITE(lowCondition, low, zero, ite)
	if !ok {
		return nil, false
	}
	high, ok = builder.reindexITE(highCondition, high, zero, ite)
	if !ok {
		return nil, false
	}
	result, ok := builder.zip(low, high, operation, zipCache)
	if !ok {
		return nil, false
	}
	cache[input] = result
	return result, true
}

// reindexITE is a full ordered ITE between a target BDD condition and typed
// FDD branches. It takes the minimum decision rank from all three operands,
// so non-monotone substitutions and simultaneous swaps remain canonical.
func (builder *Builder[F, K, V]) reindexITE(condition guard.Guard, then, otherwise *node[V], cache map[reindexITEKey[V]]*node[V]) (*node[V], bool) {
	view, ok := builder.diagram.guards.Decompose(condition)
	if !ok {
		return nil, false
	}
	if view.Terminal {
		if view.Value {
			return then, true
		}
		return otherwise, true
	}
	if then == otherwise {
		return then, true
	}
	key := reindexITEKey[V]{condition: condition, then: then, otherwise: otherwise}
	if cached, ok := cache[key]; ok {
		return cached, true
	}
	conditionRank, ok := builder.diagram.guards.Rank(view.Atom)
	if !ok {
		return nil, false
	}
	rank := conditionRank
	if !then.terminal {
		thenRank, valid := builder.diagram.guards.Rank(then.atom)
		if !valid {
			return nil, false
		}
		if thenRank < rank {
			rank = thenRank
		}
	}
	if !otherwise.terminal {
		otherwiseRank, valid := builder.diagram.guards.Rank(otherwise.atom)
		if !valid {
			return nil, false
		}
		if otherwiseRank < rank {
			rank = otherwiseRank
		}
	}
	lowCondition, highCondition := condition, condition
	if conditionRank == rank {
		lowCondition, highCondition = view.Low, view.High
	}
	lowThen, highThen := then, then
	if !then.terminal {
		thenRank, _ := builder.diagram.guards.Rank(then.atom)
		if thenRank == rank {
			lowThen, highThen = then.low, then.high
		}
	}
	lowOtherwise, highOtherwise := otherwise, otherwise
	if !otherwise.terminal {
		otherwiseRank, _ := builder.diagram.guards.Rank(otherwise.atom)
		if otherwiseRank == rank {
			lowOtherwise, highOtherwise = otherwise.low, otherwise.high
		}
	}
	low, ok := builder.reindexITE(lowCondition, lowThen, lowOtherwise, cache)
	if !ok {
		return nil, false
	}
	high, ok := builder.reindexITE(highCondition, highThen, highOtherwise, cache)
	if !ok {
		return nil, false
	}
	atom, valid := builder.diagram.guards.AtomAt(rank)
	if !valid {
		return nil, false
	}
	result := builder.decisionOrExisting(atom, low, high, then, otherwise)
	cache[key] = result
	return result, true
}

type existsKey[V any] struct {
	value *node[V]
	rank  uint64
}

func (builder *Builder[F, K, V]) exists(value *node[V], target uint64, operation Combine[V], cache map[existsKey[V]]*node[V], zipCache map[zipKey[V]]*node[V]) (*node[V], bool) {
	if value.terminal {
		return value, true
	}
	key := existsKey[V]{value: value, rank: target}
	if cached, found := cache[key]; found {
		return cached, true
	}
	rank, admitted := builder.diagram.guards.Rank(value.atom)
	if !admitted {
		return nil, false
	}
	if rank > target {
		cache[key] = value
		return value, true
	}
	if rank == target {
		low, valid := builder.exists(value.low, target, operation, cache, zipCache)
		if !valid {
			return nil, false
		}
		high, valid := builder.exists(value.high, target, operation, cache, zipCache)
		if !valid {
			return nil, false
		}
		result, valid := builder.zip(low, high, operation, zipCache)
		if !valid {
			return nil, false
		}
		cache[key] = result
		return result, true
	}
	low, valid := builder.exists(value.low, target, operation, cache, zipCache)
	if !valid {
		return nil, false
	}
	high, valid := builder.exists(value.high, target, operation, cache, zipCache)
	if !valid {
		return nil, false
	}
	result := builder.decisionOrExisting(value.atom, low, high, value, nil)
	cache[key] = result
	return result, true
}

type maskKey[V any] struct {
	value *node[V]
	mask  support.Mask
}

func (builder *Builder[F, K, V]) mask(value *node[V], region support.Mask, cache map[maskKey[V]]*node[V]) (*node[V], bool) {
	key := maskKey[V]{value: value, mask: region}
	if cached, found := cache[key]; found {
		return cached, true
	}
	view, valid := region.Decompose()
	if !valid {
		return nil, false
	}
	if view.Terminal {
		if view.Value {
			cache[key] = value
			return value, true
		}
		result := builder.terminal(terminal.ID[V]{})
		cache[key] = result
		return result, true
	}
	regionRank, regionValid := builder.diagram.guards.Rank(view.Atom)
	if !regionValid {
		return nil, false
	}
	if !value.terminal {
		valueRank, valueValid := builder.diagram.guards.Rank(value.atom)
		if !valueValid {
			return nil, false
		}
		if valueRank < regionRank {
			low, valid := builder.mask(value.low, region, cache)
			if !valid {
				return nil, false
			}
			high, valid := builder.mask(value.high, region, cache)
			if !valid {
				return nil, false
			}
			result := builder.decisionOrExisting(value.atom, low, high, value, nil)
			cache[key] = result
			return result, true
		}
		if valueRank == regionRank {
			low, valid := builder.mask(value.low, view.Low, cache)
			if !valid {
				return nil, false
			}
			high, valid := builder.mask(value.high, view.High, cache)
			if !valid {
				return nil, false
			}
			result := builder.decisionOrExisting(view.Atom, low, high, value, nil)
			cache[key] = result
			return result, true
		}
	}
	low, valid := builder.mask(value, view.Low, cache)
	if !valid {
		return nil, false
	}
	high, valid := builder.mask(value, view.High, cache)
	if !valid {
		return nil, false
	}
	result := builder.decisionOrExisting(view.Atom, low, high, value, nil)
	cache[key] = result
	return result, true
}

func (diagram *Diagram[F, K, V]) get(root Root[F, K, V], factor F, key K) (Value[V], bool, bool) {
	rank, admitted := diagram.ranks[factor]
	if !admitted {
		return Value[V]{}, false, false
	}
	column := findFactor(root.root, rank)
	if column == nil {
		return Value[V]{}, false, true
	}
	value := columnValue(column.keys, key)
	if value == nil {
		return Value[V]{}, false, true
	}
	return Value[V]{owner: diagram.owner, node: value}, true, true
}

// equalValue compares two reduced FDDs by meaning rather than by the local
// builder pages that happened to intern them.  One Diagram fixes their guard
// order; semantic terminal equality handles independently sealed candidate
// pages from the same terminal owner.
func (diagram *Diagram[F, K, V]) equalValue(left, right *node[V]) bool {
	type pair struct{ left, right *node[V] }
	seen := make(map[pair]struct{})
	var equal func(*node[V], *node[V]) bool
	equal = func(left, right *node[V]) bool {
		if left == right {
			return true
		}
		if left == nil || right == nil || left.terminal != right.terminal {
			return false
		}
		key := pair{left: left, right: right}
		if _, found := seen[key]; found {
			return true
		}
		seen[key] = struct{}{}
		if left.terminal {
			return diagram.terminals.Equal(left.value, right.value)
		}
		return left.atom == right.atom && equal(left.low, right.low) && equal(left.high, right.high)
	}
	return equal(left, right)
}

// At evaluates D[factor,key](valuation).  `present` is false exactly for the
// undefined terminal or an absent column; `valid` distinguishes bad roots.
func (diagram *Diagram[F, K, V]) At(root Root[F, K, V], factor F, key K, valuation func(guard.Atom) bool) (terminal.ID[V], bool, bool) {
	value, present, valid := diagram.Get(root, factor, key)
	if !valid || !present || valuation == nil {
		return terminal.ID[V]{}, false, valid
	}
	return diagram.Evaluate(value, valuation)
}

// Evaluate reads one opaque FDD under valuation.
func (diagram *Diagram[F, K, V]) Evaluate(value Value[V], valuation func(guard.Atom) bool) (terminal.ID[V], bool, bool) {
	if diagram == nil || value.owner != diagram.owner || value.node == nil || valuation == nil {
		return terminal.ID[V]{}, false, false
	}
	current := value.node
	for !current.terminal {
		if valuation(current.atom) {
			current = current.high
		} else {
			current = current.low
		}
	}
	if !diagram.terminals.Valid(current.value) {
		return terminal.ID[V]{}, false, true
	}
	return current.value, true, true
}

// ForEachTerminal streams each reachable symbolic terminal of value once.
// It follows shared MTBDD topology, never enumerates Boolean valuations.
// includeUndefined controls whether the sparse undefined terminal is exposed
// to the caller.  The callback must not retain diagram-internal topology.
func (diagram *Diagram[F, K, V]) ForEachTerminal(value Value[V], includeUndefined bool, visit func(terminal.ID[V]) bool) (completed, valid bool) {
	if diagram == nil || value.owner != diagram.owner || value.node == nil || visit == nil {
		return false, false
	}
	seen := make(map[*node[V]]struct{})
	var walk func(*node[V]) bool
	walk = func(current *node[V]) bool {
		if _, found := seen[current]; found {
			return true
		}
		seen[current] = struct{}{}
		if current.terminal {
			if current.value == (terminal.ID[V]{}) && !includeUndefined {
				return true
			}
			return visit(current.value)
		}
		return walk(current.low) && walk(current.high)
	}
	return walk(value.node), true
}

// Partition refines region by every decision in every populated FDD column of
// root.  Each published callback cell is nonempty, disjoint from the others,
// and has one terminal tuple across the complete sparse root.  It walks
// symbolic DAG nodes and exact BDD conjunctions only; it never enumerates
// Boolean valuations or exposes FDD terminals.
func (diagram *Diagram[F, K, V]) Partition(root Root[F, K, V], region support.Mask, visit func(support.Mask) bool) (completed, valid bool) {
	if diagram == nil || !diagram.Valid(root) || !region.Valid() || region.Manager() != diagram.guards || visit == nil {
		return false, false
	}
	work := support.New(diagram.guards)
	if work == nil || !work.Valid(region) {
		return false, false
	}
	cells := []support.Mask{region}
	complete, rootsValid := diagram.ForEach(root, func(fact Fact[F, K, V]) bool {
		next := make([]support.Mask, 0, len(cells))
		for _, cell := range cells {
			if !diagram.partitionValue(work, fact.Value.node, cell, &next) {
				return false
			}
		}
		cells = next
		return true
	})
	if !complete || !rootsValid || !work.Seal() {
		work.Discard()
		return false, false
	}
	for _, cell := range cells {
		view, okay := cell.Decompose()
		if !okay {
			return false, false
		}
		if view.Terminal && !view.Value {
			continue
		}
		if !visit(cell) {
			return false, true
		}
	}
	return true, true
}

// PartitionValueTerminals refines region by one FDD value and reports the
// exact terminal selected on every resulting nonempty cell. The zero terminal
// is deliberately reported: semantic callers distinguish sparse absence from
// an ordinary stored Default. It walks this one value once and never exposes
// the FDD topology.
//
// A branched value refines region by Boolean construction, so the read needs
// one support transaction. That transaction is a cost of the read rather than
// of the Diagram, so scratch lends the caller's shell for it: a caller reading
// one key at a time owns a reusable shell and hands it in, while a caller
// without one passes nil and the read owns a private shell for this traversal.
// The lent shell must hold no candidate the caller intends to keep - each read
// opens, publishes or discards exactly one transaction inside it, so node
// identity and the seal cut are the same either way.
func (diagram *Diagram[F, K, V]) PartitionValueTerminals(value Value[V], region support.Mask, scratch *support.Work, visit func(terminal.ID[V], support.Mask) bool) (completed, valid bool) {
	if diagram == nil || value.owner != diagram.owner || value.node == nil || !region.Valid() || region.Manager() != diagram.guards || visit == nil {
		return false, false
	}
	// A constant FDD already selects one terminal everywhere in region. This
	// common observation case has no symbolic support refinement to publish,
	// so it must not allocate a disposable guard candidate transaction.
	if value.node.terminal {
		return visit(value.node.value, region), true
	}
	// Decompose exposes BDD cofactors, not subset masks: Low/High omit the
	// tested literal and therefore cannot themselves be emitted as FDD pieces.
	// The one allocation-free aligned case is an empty cofactor, which proves
	// all of region already selects the other FDD branch and keeps the region
	// unchanged. Descending that case here keeps a constant-on-region value off
	// the transactional path entirely.
	current := value.node
	for !current.terminal {
		view, decomposed := region.Decompose()
		if !decomposed {
			return false, false
		}
		if view.Terminal || view.Atom != current.atom {
			break
		}
		if support.Empty(view.Low) {
			current = current.high
			continue
		}
		if support.Empty(view.High) {
			current = current.low
			continue
		}
		break
	}
	if current.terminal {
		return visit(current.value, region), true
	}
	work := diagram.beginRefinement(scratch)
	if work == nil {
		return false, false
	}
	if !work.Valid(region) {
		work.Discard()
		return false, false
	}
	walk := diagram.takeValuePartition()
	defer diagram.putValuePartition(walk)
	if !walk.discover(current) || !walk.accumulate(work, region) || !walk.collect(work) {
		work.Discard()
		return false, false
	}
	if !work.Seal() {
		work.Discard()
		return false, false
	}
	for _, cell := range walk.cells {
		if !visit(cell.terminal, cell.region) {
			return false, true
		}
	}
	return true, true
}

// valuePartitionScratch is short-lived implementation storage for one value
// partition. It holds no fact, identity, or candidate and never survives the
// read that borrowed it; the Diagram pools it so the one-key read the engine
// runs millions of times reuses its tables instead of minting them per call.
type valuePartitionScratch[V any] struct {
	index   map[*node[V]]int32
	order   []*node[V]
	edges   [][2]int32
	pending []int32
	regions []support.Mask
	stack   []*node[V]
	queue   []int32
	cells   []terminalCell[V]
}

// valuePartitionInline is the node count below which the walk finds a node by
// scanning its own discovery order. Almost every read partitions a handful of
// nodes, where a linear scan of pointers costs less than hashing them; the
// index map is built only once a graph is large enough to need it.
const valuePartitionInline = 16

func (diagram *Diagram[F, K, V]) takeValuePartition() *valuePartitionScratch[V] {
	held, ok := diagram.partitions.Get().(*valuePartitionScratch[V])
	if !ok || held == nil {
		return &valuePartitionScratch[V]{}
	}
	held.reset()
	return held
}

func (diagram *Diagram[F, K, V]) putValuePartition(walk *valuePartitionScratch[V]) {
	if walk == nil {
		return
	}
	walk.reset()
	diagram.partitions.Put(walk)
}

func (walk *valuePartitionScratch[V]) reset() {
	clear(walk.index)
	clear(walk.order)
	clear(walk.stack)
	clear(walk.cells)
	clear(walk.pending)
	walk.order = walk.order[:0]
	walk.edges = walk.edges[:0]
	walk.pending = walk.pending[:0]
	walk.regions = walk.regions[:0]
	walk.stack = walk.stack[:0]
	walk.queue = walk.queue[:0]
	walk.cells = walk.cells[:0]
}

// discover records every node reachable from root exactly once, in the
// low-first depth-first order the partition emits in, and counts the incoming
// edges of each one. The FDD is a hash-consed DAG: a shared subgraph is one
// node with several parents, so enumerating root-to-leaf paths would revisit
// it once per path and is exponential in the node count. One entry per node is
// the structural quantity this read is allowed to cost.
func (walk *valuePartitionScratch[V]) discover(root *node[V]) bool {
	walk.stack = append(walk.stack, root)
	for len(walk.stack) != 0 {
		last := len(walk.stack) - 1
		current := walk.stack[last]
		walk.stack[last] = nil
		walk.stack = walk.stack[:last]
		if current == nil {
			return false
		}
		if _, discovered := walk.lookup(current); discovered {
			continue
		}
		walk.install(current)
		if current.terminal {
			continue
		}
		if current.low == nil || current.high == nil {
			return false
		}
		walk.stack = append(walk.stack, current.high, current.low)
	}
	for len(walk.pending) < len(walk.order) {
		walk.pending = append(walk.pending, 0)
	}
	walk.pending = walk.pending[:len(walk.order)]
	for _, current := range walk.order {
		if current.terminal {
			walk.edges = append(walk.edges, [2]int32{-1, -1})
			continue
		}
		low, lowKnown := walk.lookup(current.low)
		high, highKnown := walk.lookup(current.high)
		if !lowKnown || !highKnown {
			return false
		}
		walk.edges = append(walk.edges, [2]int32{low, high})
		walk.pending[low]++
		walk.pending[high]++
	}
	return walk.pending[0] == 0
}

// lookup and install are the walk's own node directory. Discovery order is
// the row order, so a row number addresses every parallel table at once.
func (walk *valuePartitionScratch[V]) lookup(current *node[V]) (int32, bool) {
	if len(walk.order) > valuePartitionInline {
		row, known := walk.index[current]
		return row, known
	}
	for row, held := range walk.order {
		if held == current {
			return int32(row), true
		}
	}
	return 0, false
}

func (walk *valuePartitionScratch[V]) install(current *node[V]) {
	row := int32(len(walk.order))
	walk.order = append(walk.order, current)
	if len(walk.order) <= valuePartitionInline {
		return
	}
	if walk.index == nil {
		walk.index = make(map[*node[V]]int32, 2*valuePartitionInline)
	}
	if len(walk.index) != len(walk.order)-1 {
		clear(walk.index)
		for held := range walk.order {
			walk.index[walk.order[held]] = int32(held)
		}
		return
	}
	walk.index[current] = row
}

// accumulate carries region down the DAG, joining at every node the regions
// its parents deliver. A node is processed only once all of its incoming edges
// have arrived, so each decision costs one pair of conjunctions no matter how
// many paths reach it, and every terminal ends holding the exact union of the
// cells that select it.
func (walk *valuePartitionScratch[V]) accumulate(work *support.Work, region support.Mask) bool {
	empty := work.False()
	for range walk.order {
		walk.regions = append(walk.regions, empty)
	}
	walk.regions[0] = region
	walk.queue = append(walk.queue, 0)
	for cursor := 0; cursor < len(walk.queue); cursor++ {
		index := walk.queue[cursor]
		current := walk.order[index]
		if current.terminal {
			continue
		}
		low, high := empty, empty
		if arrived := walk.regions[index]; !support.Empty(arrived) {
			view, decomposed := work.Decompose(arrived)
			if !decomposed {
				return false
			}
			switch {
			case !view.Terminal && view.Atom == current.atom && support.Empty(view.Low):
				high = arrived
			case !view.Terminal && view.Atom == current.atom && support.Empty(view.High):
				low = arrived
			default:
				lowPart, lowOK := work.Conjoin(arrived, current.atom, false)
				highPart, highOK := work.Conjoin(arrived, current.atom, true)
				if !lowOK || !highOK {
					return false
				}
				low, high = lowPart, highPart
			}
		}
		if !walk.deliver(work, walk.edges[index][0], low) || !walk.deliver(work, walk.edges[index][1], high) {
			return false
		}
	}
	return true
}

func (walk *valuePartitionScratch[V]) deliver(work *support.Work, index int32, part support.Mask) bool {
	if index < 0 || int(index) >= len(walk.pending) || walk.pending[index] == 0 {
		return false
	}
	// An empty arrival adds nothing, and the first non-empty arrival is
	// already the join with the empty accumulator. Neither needs Boolean work.
	switch {
	case support.Empty(part):
	case support.Empty(walk.regions[index]):
		walk.regions[index] = part
	default:
		joined, ok := work.Or(walk.regions[index], part)
		if !ok {
			return false
		}
		walk.regions[index] = joined
	}
	walk.pending[index]--
	if walk.pending[index] == 0 {
		walk.queue = append(walk.queue, index)
	}
	return true
}

// collect reduces the settled terminals to one cell per distinct terminal, in
// the order the partition first reached them. Two terminal nodes carrying the
// same value denote one FDD piece, and a terminal no cell selects is not a
// piece at all.
func (walk *valuePartitionScratch[V]) collect(work *support.Work) bool {
	for index, current := range walk.order {
		if !current.terminal || support.Empty(walk.regions[index]) {
			continue
		}
		merged := false
		for slot := range walk.cells {
			if walk.cells[slot].terminal != current.value {
				continue
			}
			joined, ok := work.Or(walk.cells[slot].region, walk.regions[index])
			if !ok {
				return false
			}
			walk.cells[slot].region = joined
			merged = true
			break
		}
		if merged {
			continue
		}
		walk.cells = append(walk.cells, terminalCell[V]{terminal: current.value, region: walk.regions[index]})
	}
	return true
}

// beginRefinement opens the one support transaction a branched partition
// needs. A lent shell is reserved for that transaction and reopened in place,
// so consecutive reads share its Boolean scratch without sharing a candidate.
// Without a lent shell the read owns a private one for this traversal alone.
func (diagram *Diagram[F, K, V]) beginRefinement(scratch *support.Work) *support.Work {
	if scratch == nil {
		return support.New(diagram.guards)
	}
	if !scratch.OwnsManager(diagram.guards) || !scratch.BeginTransaction(nil) {
		return nil
	}
	return scratch
}

func (diagram *Diagram[F, K, V]) partitionValue(work *support.Work, value *node[V], cell support.Mask, output *[]support.Mask) bool {
	if value == nil || work == nil || output == nil {
		return false
	}
	frames := []valuePartitionFrame[V]{{value: value, region: cell}}
	for len(frames) != 0 {
		last := len(frames) - 1
		frame := frames[last]
		frames[last] = valuePartitionFrame[V]{}
		frames = frames[:last]
		if frame.value == nil {
			return false
		}
		if frame.value.terminal {
			*output = append(*output, frame.region)
			continue
		}
		low, lowOK := work.Conjoin(frame.region, frame.value.atom, false)
		high, highOK := work.Conjoin(frame.region, frame.value.atom, true)
		if !lowOK || !highOK {
			return false
		}
		frames = append(frames,
			valuePartitionFrame[V]{value: frame.value.high, region: high},
			valuePartitionFrame[V]{value: frame.value.low, region: low},
		)
	}
	return true
}

func (builder *Builder[F, K, V]) validValue(value Value[V]) bool {
	return builder != nil && builder.diagram != nil && value.owner == builder.diagram.owner && value.node != nil
}

func (builder *Builder[F, K, V]) validTerminal(id terminal.ID[V]) bool {
	return id == (terminal.ID[V]{}) || builder != nil && builder.terminalAuthority != nil && builder.terminalAuthority.Valid(id)
}

// ForEach streams sparse columns in schema order then ascending key order.
func (diagram *Diagram[F, K, V]) ForEach(root Root[F, K, V], visit func(Fact[F, K, V]) bool) (completed, valid bool) {
	if !diagram.Valid(root) || visit == nil {
		return false, false
	}
	for _, factor := range diagram.factors {
		column := findFactor(root.root, diagram.ranks[factor])
		if column == nil {
			continue
		}
		stack := make([]*keyNode[K, V], 0)
		current := column.keys
		for current != nil || len(stack) != 0 {
			for current != nil {
				stack = append(stack, current)
				current = current.left
			}
			last := len(stack) - 1
			current = stack[last]
			stack = stack[:last]
			if !visit(Fact[F, K, V]{Factor: factor, Key: current.key, Value: Value[V]{owner: diagram.owner, node: current.value}}) {
				return false, true
			}
			current = current.right
		}
	}
	return true, true
}

// releaseMemos drops the transaction's pointwise operation storage. A sealed
// or discarded Builder must not keep candidate nodes reachable through a memo.
// close ends the candidate scope. The unique tables and memos are cleared so
// no node of a finished transaction stays reachable, while their storage stays
// available to the next transaction this Builder opens.
func (builder *Builder[F, K, V]) close() {
	builder.open = false
	builder.token = leaseRef{}
	builder.terminalAuthority = nil
	clear(builder.terminals)
	clear(builder.decisions)
	clear(builder.updates)
	clear(builder.imports)
	builder.releaseMemos()
}

func (builder *Builder[F, K, V]) releaseMemos() {
	builder.zipWork = nil
	builder.transformWork = nil
	builder.transformNodes = nil
	builder.existsWork = nil
	builder.existsZip = nil
	builder.reindexNodes = nil
	builder.reindexZip = nil
	builder.reindexITEWork = nil
	builder.maskWork = nil
}

func (builder *Builder[F, K, V]) terminal(value terminal.ID[V]) *node[V] {
	if cached := builder.terminals[value]; cached != nil {
		return cached
	}
	created := &node[V]{terminal: true, value: value}
	builder.terminals[value] = created
	return created
}

// adoptTerminal returns this transaction's one node for value, adopting an
// immutable predecessor leaf that already carries it. A rewrite that keeps a
// predecessor leaf must keep it for every occurrence of that terminal:
// reduction and comparison both read one node per value as one meaning, so a
// second live node for the same value would leave a redundant decision above
// it and make an unmoved rewrite look structurally different.
func (builder *Builder[F, K, V]) adoptTerminal(value terminal.ID[V], first, second *node[V]) *node[V] {
	if cached := builder.terminals[value]; cached != nil {
		return cached
	}
	switch {
	case first != nil && first.terminal && first.value == value:
		builder.terminals[value] = first
		return first
	case second != nil && second.terminal && second.value == value:
		builder.terminals[value] = second
		return second
	}
	return builder.terminal(value)
}

// importNode registers an FDD from an earlier Builder in this Builder's local
// unique tables. Its nodes are immutable, so a predecessor node remains the
// canonical result when recursively imported children did not change. A fresh
// decision is needed only when canonicalization changes one of its children.
//
// Reusing predecessor terminals is also important for candidate terminal
// pages: this Builder retains its own terminal authority and never translates
// an ID through a different Diagram or Arena.
func (builder *Builder[F, K, V]) importNode(prior *node[V]) *node[V] {
	if prior == nil {
		return builder.terminal(terminal.ID[V]{})
	}
	if cached := builder.imports[prior]; cached != nil {
		return cached
	}
	if prior.terminal {
		// A terminal has no children to canonicalize. Register the immutable
		// predecessor leaf itself, rather than allocating a local duplicate.
		// A candidate leaf of the same ID may already be cached; it remains
		// valid in older candidate nodes, while this import must retain its
		// own predecessor identity for an exact no-op.
		builder.terminals[prior.value] = prior
		builder.imports[prior] = prior
		return prior
	}
	low := builder.importNode(prior.low)
	high := builder.importNode(prior.high)
	if low == prior.low && high == prior.high {
		key := decisionKey[V]{atom: prior.atom, low: low, high: high}
		builder.decisions[key] = prior
		builder.imports[prior] = prior
		return prior
	}
	result := builder.decision(prior.atom, low, high)
	builder.imports[prior] = result
	return result
}

func (builder *Builder[F, K, V]) decision(atom guard.Atom, low, high *node[V]) *node[V] {
	if low == high {
		return low
	}
	key := decisionKey[V]{atom: atom, low: low, high: high}
	if cached := builder.decisions[key]; cached != nil {
		return cached
	}
	created := &node[V]{atom: atom, low: low, high: high}
	builder.decisions[key] = created
	return created
}

// update computes the exact ITE overwrite iteratively through the guard BDD
// and FDD.  Its cache is builder-local; it cannot become a persistent second
// representation of facts.
func (builder *Builder[F, K, V]) update(prior *node[V], when support.Mask, value terminal.ID[V]) *node[V] {
	key := updateKey[V]{prior: prior, when: when, value: value}
	if cached := builder.updates[key]; cached != nil {
		return cached
	}
	view, valid := when.Decompose()
	if !valid {
		return prior
	}
	if view.Terminal {
		if view.Value {
			result := builder.terminal(value)
			builder.updates[key] = result
			return result
		}
		builder.updates[key] = prior
		return prior
	}
	// The BDD node and FDD node have a common sealed order.  If the FDD tests
	// an earlier atom, preserve it and continue the overwrite beneath both
	// branches.  Otherwise split the BDD support at its own next atom.
	whenRank, _ := builder.diagram.guards.Rank(view.Atom)
	if !prior.terminal {
		priorRank, _ := builder.diagram.guards.Rank(prior.atom)
		if priorRank < whenRank {
			low := builder.update(prior.low, when, value)
			high := builder.update(prior.high, when, value)
			result := builder.decision(prior.atom, low, high)
			builder.updates[key] = result
			return result
		}
		if priorRank == whenRank {
			low := builder.update(prior.low, view.Low, value)
			high := builder.update(prior.high, view.High, value)
			result := builder.decision(view.Atom, low, high)
			builder.updates[key] = result
			return result
		}
	}
	low := builder.update(prior, view.Low, value)
	high := builder.update(prior, view.High, value)
	result := builder.decision(view.Atom, low, high)
	builder.updates[key] = result
	return result
}

func columnValue[K scalar.Key, V any](node *keyNode[K, V], key K) *node[V] {
	for node != nil {
		if key < node.key {
			node = node.left
		} else if key > node.key {
			node = node.right
		} else {
			return node.value
		}
	}
	return nil
}

func findFactor[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V], rank uint32) *factorNode[F, K, V] {
	for node != nil {
		if rank < node.rank {
			node = node.left
		} else if rank > node.rank {
			node = node.right
		} else {
			return node
		}
	}
	return nil
}

func factorHeight[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V]) int8 {
	if node == nil {
		return 0
	}
	return node.height
}
func keyHeight[K scalar.Key, V any](node *keyNode[K, V]) int8 {
	if node == nil {
		return 0
	}
	return node.height
}

func makeFactor[F ~uint64, K scalar.Key, V any](factor F, rank uint32, keys *keyNode[K, V], left, right *factorNode[F, K, V]) *factorNode[F, K, V] {
	height := factorHeight(left)
	if factorHeight(right) > height {
		height = factorHeight(right)
	}
	return &factorNode[F, K, V]{factor: factor, rank: rank, keys: keys, left: left, right: right, height: height + 1}
}
func makeKey[K scalar.Key, V any](key K, value *node[V], left, right *keyNode[K, V]) *keyNode[K, V] {
	height := keyHeight(left)
	if keyHeight(right) > height {
		height = keyHeight(right)
	}
	return &keyNode[K, V]{key: key, value: value, left: left, right: right, height: height + 1}
}
func balanceFactor[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V]) *factorNode[F, K, V] {
	delta := int(factorHeight(node.left)) - int(factorHeight(node.right))
	if delta > 1 {
		left := node.left
		if factorHeight(left.left) < factorHeight(left.right) {
			left = rotateFactorLeft(left)
		}
		return rotateFactorRight(makeFactor(node.factor, node.rank, node.keys, left, node.right))
	}
	if delta < -1 {
		right := node.right
		if factorHeight(right.right) < factorHeight(right.left) {
			right = rotateFactorRight(right)
		}
		return rotateFactorLeft(makeFactor(node.factor, node.rank, node.keys, node.left, right))
	}
	// A balanced node is already the node its caller wants published: every
	// caller constructs it from the children it intends. Rebuilding it would
	// allocate a second node with identical fields and height.
	return node
}
func rotateFactorLeft[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V]) *factorNode[F, K, V] {
	right := node.right
	return makeFactor(right.factor, right.rank, right.keys, makeFactor(node.factor, node.rank, node.keys, node.left, right.left), right.right)
}
func rotateFactorRight[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V]) *factorNode[F, K, V] {
	left := node.left
	return makeFactor(left.factor, left.rank, left.keys, left.left, makeFactor(node.factor, node.rank, node.keys, left.right, node.right))
}
func balanceKey[K scalar.Key, V any](node *keyNode[K, V]) *keyNode[K, V] {
	delta := int(keyHeight(node.left)) - int(keyHeight(node.right))
	if delta > 1 {
		left := node.left
		if keyHeight(left.left) < keyHeight(left.right) {
			left = rotateKeyLeft(left)
		}
		return rotateKeyRight(makeKey(node.key, node.value, left, node.right))
	}
	if delta < -1 {
		right := node.right
		if keyHeight(right.right) < keyHeight(right.left) {
			right = rotateKeyRight(right)
		}
		return rotateKeyLeft(makeKey(node.key, node.value, node.left, right))
	}
	// A balanced node is already the node its caller wants published: every
	// caller constructs it from the children it intends. Rebuilding it would
	// allocate a second node with identical fields and height.
	return node
}
func rotateKeyLeft[K scalar.Key, V any](node *keyNode[K, V]) *keyNode[K, V] {
	right := node.right
	return makeKey(right.key, right.value, makeKey(node.key, node.value, node.left, right.left), right.right)
}
func rotateKeyRight[K scalar.Key, V any](node *keyNode[K, V]) *keyNode[K, V] {
	left := node.left
	return makeKey(left.key, left.value, left.left, makeKey(node.key, node.value, left.right, node.right))
}
func setFactor[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V], rank uint32, factor F, key K, value *node[V]) (*factorNode[F, K, V], bool) {
	if node == nil {
		keys, added := setKey[K, V](nil, key, value)
		return makeFactor(factor, rank, keys, nil, nil), added
	}
	if rank < node.rank {
		left, added := setFactor(node.left, rank, factor, key, value)
		return balanceFactor(makeFactor(node.factor, node.rank, node.keys, left, node.right)), added
	}
	if rank > node.rank {
		right, added := setFactor(node.right, rank, factor, key, value)
		return balanceFactor(makeFactor(node.factor, node.rank, node.keys, node.left, right)), added
	}
	keys, added := setKey(node.keys, key, value)
	return makeFactor(node.factor, node.rank, keys, node.left, node.right), added
}
func setKey[K scalar.Key, V any](node *keyNode[K, V], key K, value *node[V]) (*keyNode[K, V], bool) {
	if node == nil {
		return makeKey(key, value, nil, nil), true
	}
	if key < node.key {
		left, added := setKey(node.left, key, value)
		return balanceKey(makeKey(node.key, node.value, left, node.right)), added
	}
	if key > node.key {
		right, added := setKey(node.right, key, value)
		return balanceKey(makeKey(node.key, node.value, node.left, right)), added
	}
	return makeKey(key, value, node.left, node.right), false
}
func deleteFactor[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V], rank uint32, key K) (*factorNode[F, K, V], bool) {
	if node == nil {
		return nil, false
	}
	if rank < node.rank {
		left, removed := deleteFactor(node.left, rank, key)
		if !removed {
			return node, false
		}
		return balanceFactor(makeFactor(node.factor, node.rank, node.keys, left, node.right)), true
	}
	if rank > node.rank {
		right, removed := deleteFactor(node.right, rank, key)
		if !removed {
			return node, false
		}
		return balanceFactor(makeFactor(node.factor, node.rank, node.keys, node.left, right)), true
	}
	keys, removed := deleteKey(node.keys, key)
	if !removed {
		return node, false
	}
	if keys != nil {
		return makeFactor(node.factor, node.rank, keys, node.left, node.right), true
	}
	return joinFactors(node.left, node.right), true
}
func deleteKey[K scalar.Key, V any](node *keyNode[K, V], key K) (*keyNode[K, V], bool) {
	if node == nil {
		return nil, false
	}
	if key < node.key {
		left, removed := deleteKey(node.left, key)
		if !removed {
			return node, false
		}
		return balanceKey(makeKey(node.key, node.value, left, node.right)), true
	}
	if key > node.key {
		right, removed := deleteKey(node.right, key)
		if !removed {
			return node, false
		}
		return balanceKey(makeKey(node.key, node.value, node.left, right)), true
	}
	return joinKeys(node.left, node.right), true
}
func joinFactors[F ~uint64, K scalar.Key, V any](left, right *factorNode[F, K, V]) *factorNode[F, K, V] {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	min, rest := popFactorMin(right)
	return balanceFactor(makeFactor(min.factor, min.rank, min.keys, left, rest))
}
func popFactorMin[F ~uint64, K scalar.Key, V any](node *factorNode[F, K, V]) (*factorNode[F, K, V], *factorNode[F, K, V]) {
	if node.left == nil {
		return node, node.right
	}
	min, left := popFactorMin(node.left)
	return min, balanceFactor(makeFactor(node.factor, node.rank, node.keys, left, node.right))
}
func joinKeys[K scalar.Key, V any](left, right *keyNode[K, V]) *keyNode[K, V] {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	min, rest := popKeyMin(right)
	return balanceKey(makeKey(min.key, min.value, left, rest))
}
func popKeyMin[K scalar.Key, V any](node *keyNode[K, V]) (*keyNode[K, V], *keyNode[K, V]) {
	if node.left == nil {
		return node, node.right
	}
	min, left := popKeyMin(node.left)
	return min, balanceKey(makeKey(node.key, node.value, left, node.right))
}
