package transformer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RelationProgramUnit is the complete immutable input for one lexical body in
// a forest transaction. Body, not a solve-local cell, owns call identity.
type RelationProgramUnit struct {
	Body     lexicalidentity.StableLexicalBodyID
	Registry *axis.Registry
	KeySpace *keyspace.KeySpace
	Graph    cfg.Graph
	Plan     *operationplan.Plan
	Shape    Shape
	// Domain is the exact application-owned State product selected by the
	// body's ExecutionFactory, including optional lanes and WIR widening
	// thresholds. The replacement engine never reconstructs it from Registry.
	Domain state.ProductDomain
	// PathSemantics is execution authority, not semantic syntax. The Plan
	// owns the immutable transaction payload; this authority only resolves its
	// frozen paths in the body's keyspace during tuple-coordinate execution.
	PathSemantics *factapply.PathSemanticAuthority
	// RootAssignments owns the one body-wide N4 concrete root/object kernel.
	RootAssignments *factapply.RootAssignmentAuthority
	// Returns owns the one body-wide N5 return heap/placement/projection kernel.
	Returns *factapply.ReturnAuthority
	// ExternalCallOutcome is the body's prepared signature/module/callable
	// producer. Lexical calls are intentionally excluded and are owned only by
	// RelationProgram frames.
	ExternalCallOutcome callpayload.CallOutcomeProgram
	// CustomExpressionValue is a transient preparation authority. Freeze invokes
	// it at most once per exact point/source and stores only immutable Constant
	// terms; RelationProgram and its executor never retain the callback.
	CustomExpressionValue sourcevalue.ExpressionValueProvider
	// GenericForMembership owns only non-scalar membership effects. The scalar
	// loop-variable value is the frozen generic-for Projection term.
	GenericForMembership factapply.GenericForMembershipAuthority
	// NodeReads is the exact finite intrabody read dependency set used by that
	// producer at each point. The application scheduler turns these into normal
	// equation edges; providers never read hidden solver state.
	NodeReads [][]cfg.Point
	// DirectLexicalDeclarations is the binder-sealed, plan-bound proof for
	// stable local function values declared directly by this lexical body.
	DirectLexicalDeclarations DirectLexicalDeclarationAuthority
	// Definitions are exact lexical-body declarations owned by this unit.
	// They seed body-analysis applications from the stabilized definition
	// coordinate; they are not calls and have no continuation into the owner.
	Definitions      []RelationProgramDefinition
	EntrySeedPlan    state.EntrySeedPlan
	InitialStatePlan state.InitialStatePlan
}

// RelationProgramDefinition binds one owner point to the lexical body whose
// closure environment is created there. Target identity is forest-stable;
// point belongs to the owning unit's sealed graph.
type RelationProgramDefinition struct {
	Target              lexicalidentity.StableLexicalBodyID
	Point               cfg.Point
	ExternallyReachable bool
}

type frozenLexicalCallTarget struct {
	variable relationVar
	shape    Shape
	results  uint32
	boundary DirectCallBoundary
}

func (t frozenLexicalCallTarget) valid() bool {
	return t.variable != 0
}

type frozenLexicalCallSurface interface {
	lookup(cfg.Point) (frozenLexicalCallSite, bool)
}

type frozenLexicalCallCandidate struct {
	identity identity.ID
	target   frozenLexicalCallTarget
}

type frozenLexicalCallSite struct {
	candidates []frozenLexicalCallCandidate
	residual   bool
}

func (s frozenLexicalCallSite) valid() bool {
	if len(s.candidates) == 0 {
		return false
	}
	for index, candidate := range s.candidates {
		if !candidate.target.valid() || index == 0 && candidate.identity == (identity.ID{}) && (len(s.candidates) != 1 || s.residual) ||
			index != 0 && candidate.identity == (identity.ID{}) {
			return false
		}
		if index > 0 && !relationCallIdentityLess(s.candidates[index-1].identity, candidate.identity) {
			return false
		}
	}
	return true
}

func relationCallIdentityLess(left, right identity.ID) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Site != right.Site {
		return left.Site < right.Site
	}
	return left.Index < right.Index
}

type programCallSurface struct {
	targets map[cfg.Point]frozenLexicalCallSite
}

func (s programCallSurface) lookup(point cfg.Point) (frozenLexicalCallSite, bool) {
	target, ok := s.targets[point]
	return target, ok
}

type relationProgramBody struct {
	body                    lexicalidentity.StableLexicalBodyID
	variable                relationVar
	keys                    *keyspace.KeySpace
	roots                   relationRootCarrier
	ambient                 []relationEnvironmentRoot
	relation                Relation
	plan                    *operationplan.Plan
	graph                   cfg.Graph
	pathSemantics           *factapply.PathSemanticAuthority
	rootAssignments         *factapply.RootAssignmentAuthority
	returns                 *factapply.ReturnAuthority
	externalCalls           callpayload.CallOutcomeProgram
	genericForMembership    factapply.GenericForMembershipAuthority
	nodeReads               [][]cfg.Point
	domain                  lattice.Lattice[state.State]
	productDomain           state.ProductDomain
	entrySeedPlan           state.EntrySeedPlan
	initialStatePlan        state.InitialStatePlan
	rootAllocations         *state.BoundaryAllocationAuthority
	frames                  []linkedRelationFrame
	definitionFrames        map[callFrameTerm]relationVar
	callReceivers           map[cfg.Point][]rootAssignmentTerm
	callReceiverAssignments map[cfg.Point]struct{}
}

// rootValueSlot resolves the sole concrete storage spelling used by the
// current registered leaf kernels. IN roots use the sealed boundary carrier;
// MID roots use their typed lexical register. The formal root identity itself
// remains body-owned and never becomes a caller/route coordinate.
func (b *relationProgramBody) rootValueSlot(root Root) (key.Value, bool) {
	if b == nil || b.relation.arena == nil {
		return 0, false
	}
	if root.Kind == RootMiddle {
		register, ok := b.relation.arena.middle.register(root)
		return register.slot, ok && register.slot != 0
	}
	if !b.roots.shape.validateInput(root) {
		return 0, false
	}
	offset := b.roots.shape.offset(root.Kind) + int(root.Index)
	if offset < 0 || offset >= len(b.roots.roots) || b.roots.roots[offset].root != root {
		return 0, false
	}
	return b.roots.roots[offset].slot, true
}

func (b *relationProgramBody) rootPathKey(root Root) (keyspace.Key, bool) {
	if b == nil || b.keys == nil || !b.keys.Valid() || b.relation.arena == nil {
		return keyspace.Key{}, false
	}
	if root.Kind == RootMiddle {
		register, ok := b.relation.arena.middle.register(root)
		if !ok {
			return keyspace.Key{}, false
		}
		switch register.kind {
		case relationMiddleRegisterSymbol:
			if register.symbol == 0 {
				return keyspace.Key{}, false
			}
			path := b.keys.FromPath(pathdom.NewPath(register.symbol, ""))
			return path, b.keys.FormatReadOnly(path) != ""
		case relationMiddleRegisterCallResult:
			_, path, err := frameCallResultCarrier(b.keys, b.body, register.point, register.ordinal)
			return path, err == nil && path.Kind != keyspace.KindInvalid && b.keys.FormatReadOnly(path) != ""
		default:
			// Expression registers are evaluator scratch. They deliberately have
			// no structural boundary image.
			return keyspace.Key{}, false
		}
	}
	if !b.roots.shape.validateInput(root) {
		return keyspace.Key{}, false
	}
	offset := b.roots.shape.offset(root.Kind) + int(root.Index)
	if offset < 0 || offset >= len(b.roots.roots) || b.roots.roots[offset].root != root {
		return keyspace.Key{}, false
	}
	return b.roots.roots[offset].path, true
}

type relationProgramDefinition struct {
	owner, target       relationVar
	point               cfg.Point
	frame               callFrameTerm
	externallyReachable bool
}

func relationAllocationTemplates(relation Relation) ([]identity.AllocationTemplate, error) {
	if relation.arena == nil || !relation.arena.Sealed() {
		return nil, fmt.Errorf("transformer: relation allocation inventory is unsealed")
	}
	templates := make([]identity.AllocationTemplate, 0, len(relation.arena.allocations)-1)
	for term := AllocationTemplateTerm(1); int(term) < len(relation.arena.allocations); term++ {
		inventory := relation.arena.allocations[term].templates
		if len(inventory) == 0 {
			return nil, fmt.Errorf("transformer: allocation %d has no frozen identity inventory", term)
		}
		templates = append(templates, inventory...)
	}
	objects, err := relationObjectMaterializationTemplates(relation)
	if err != nil {
		return nil, err
	}
	templates = append(templates, objects...)
	return templates, nil
}

type linkedRelationFrame struct {
	owner, target   relationVar
	term            callFrameTerm
	callerBody      lexicalidentity.StableLexicalBodyID
	targetBody      lexicalidentity.StableLexicalBodyID
	callerKeys      *keyspace.KeySpace
	targetKeys      *keyspace.KeySpace
	targetRoots     relationRootCarrier
	point           cfg.Point
	occurrence      uint32
	shape           Shape
	rootCircuit     []linkedFrameRoot
	closureProducer callFrameTerm
	ambientCircuit  []linkedFrameAmbientRoot
	outboundRoots   state.BoundaryRoots
	exitBridges     []linkedFrameExitBridge
	resultSelectors []linkedFrameResult
	resultSources   []linkedFrameResultSource
	boundary        linkedFrameBoundaryTopology
	route           linkedFrameRouteAuthority
	existentials    state.BoundaryExistentialNamespace
	allocations     *state.BoundaryAllocationAuthority
	control         linkedCallControl
}

type relationEnvironmentRoot struct {
	symbol  symbol.ID
	slot    key.Value
	path    keyspace.Key
	mutable bool
}

type linkedFrameAmbientRoot struct {
	value  ValueTerm
	path   PathTerm
	target relationEnvironmentRoot
}

// linkedFrameExitBridge is the exact target-side SSA address reaching the
// synthetic function exit for one structural input root. Boundary projection
// needs both spellings: the structural root carries boundary value facts while
// this certified version carries point-local descendants written by the
// callee. Both rebase to the same caller root ordinal.
type linkedFrameExitBridge struct {
	input int
	root  state.BoundaryRoot
}

// linkedFrameResultSource is the exact callee term whose value/object graph is
// returned through one result coordinate. Addressable sources retain their
// slot/path spelling; rvalues still carry their identity-reachable heap closure.
// It is a boundary lens, not a second result representation: outbound transport
// maps this root to the same caller destination as the canonical ret[n] root.
type linkedFrameResultSource struct {
	result uint32
	value  ValueTerm
	slot   key.Value
	path   keyspace.Key
}

type linkedCallControl uint8

const (
	linkedCallControlInvalid linkedCallControl = iota
	linkedCallControlOrdinary
	// Protected callbacks require a distinct pcall/xpcall callback edge. The
	// current lexical CallSurface cannot mint this authority, so execution
	// remains fail-closed until that typed producer is linked.
	linkedCallControlProtectedCallback
)

// linkedFrameRoot is one frozen cross-arena substitution wire. The callee
// root namespace is explicit; repeated caller terms are intentionally retained
// in separate wires so f(x,x) preserves alias correlation without interning
// new terms into either sealed arena.
type linkedFrameRoot struct {
	root        Root
	value       ValueTerm
	path        PathTerm
	destination linkedFrameInputDestinationLens
}

// linkedFrameInputDestinationLens is the single frozen caller-side structural
// vocabulary consumed by both coordinate-footprint closure and executable
// Apply. Paths remain in the caller's concrete keyspace until each consumer
// rekeys them into the shared formal fiber.
type linkedFrameInputDestinationLens struct {
	valueRoot         Root
	hasValueRoot      bool
	path              keyspace.Key
	hasPath           bool
	persistentPath    keyspace.Key
	hasPersistent     bool
	persistentRoot    Root
	hasPersistentRoot bool
}

// linkedFrameBoundaryTopology is the one frozen source-to-destination
// relation for an Apply boundary.  Static coordinate closure and executable
// materialization may attach different payloads to these edges, but neither is
// allowed to rediscover their structural endpoints.
type linkedFrameBoundaryTopology struct {
	destinations     []linkedFrameBoundaryDestination
	edges            []linkedFrameBoundaryEdge
	inputs           []int
	results          map[uint32][]int
	inputEdges       []int
	persistentEdges  []int
	ambientEdges     []int
	exitEdges        []int
	resultEdges      map[uint32][]int
	resultAliasEdges [][]int
}

type linkedFrameBoundaryDestinationKind uint8

const (
	linkedFrameBoundaryDestinationInvalid linkedFrameBoundaryDestinationKind = iota
	linkedFrameBoundaryDestinationInput
	linkedFrameBoundaryDestinationPersistentInput
	linkedFrameBoundaryDestinationAmbient
	linkedFrameBoundaryDestinationCanonicalResult
	linkedFrameBoundaryDestinationStateResult
)

type linkedFrameBoundaryDestination struct {
	kind      linkedFrameBoundaryDestinationKind
	input     int
	result    uint32
	valueRoot Root
	hasRoot   bool
	slot      key.Value
	path      keyspace.Key
	optional  bool
}

type linkedFrameBoundarySourceKind uint8

const (
	linkedFrameBoundarySourceInvalid linkedFrameBoundarySourceKind = iota
	linkedFrameBoundarySourceInput
	linkedFrameBoundarySourceAmbient
	linkedFrameBoundarySourceExitBridge
	linkedFrameBoundarySourceResult
	linkedFrameBoundarySourceResultAlias
)

type linkedFrameBoundaryEdge struct {
	kind        linkedFrameBoundarySourceKind
	input       int
	ambient     int
	exitBridge  int
	result      uint32
	resultAlias int
	destination int
	source      state.BoundaryFactorRoot
	root        Root
}

type linkedFrameResult struct {
	slot    uint32
	targets []linkedFrameResultTarget
}

type linkedFrameResultTarget struct {
	kind        factflow.CallResultTargetKind
	slot        key.Value
	path        keyspace.Key
	stateTarget bool
}

// linkedFrameRouteAuthority is the finite incoming ApplyRef coordinate. The
// executor crosses it with its sealed typed outcome kind; recursion at the
// same site therefore reuses one route instead of growing syntax.
type linkedFrameRouteAuthority struct {
	owner, target relationVar
	frame         callFrameTerm
}

func (r linkedFrameRouteAuthority) valid() bool { return r.owner != 0 && r.target != 0 && r.frame != 0 }

func linkedResultHasStateTarget(result linkedFrameResult) bool {
	for _, target := range result.targets {
		if target.stateTarget {
			return true
		}
	}
	return false
}

func linkedResultHasAddressTarget(result linkedFrameResult) bool {
	for _, target := range result.targets {
		if target.stateTarget && target.slot != 0 && target.path.Kind != keyspace.KindInvalid {
			return true
		}
	}
	return false
}

func (f linkedRelationFrame) inputRootCount() int { return len(f.rootCircuit) + len(f.ambientCircuit) }

func (f linkedRelationFrame) mutableAmbientCount() int {
	count := 0
	for _, root := range f.ambientCircuit {
		if root.target.mutable {
			count++
		}
	}
	return count
}

func (f linkedRelationFrame) valid() bool {
	valid := f.owner != 0 && f.target != 0 && f.term != 0 && f.callerBody != (lexicalidentity.StableLexicalBodyID{}) &&
		f.targetBody != (lexicalidentity.StableLexicalBodyID{}) && f.callerKeys != nil && f.callerKeys.Valid() &&
		f.targetKeys != nil && f.targetKeys.Valid() && f.targetRoots.valid(f.targetKeys) && f.point != 0 && len(f.rootCircuit) == f.shape.InputCount() &&
		f.route == (linkedFrameRouteAuthority{owner: f.owner, target: f.target, frame: f.term}) && f.route.valid() && f.existentials.Valid() && f.allocations != nil &&
		f.allocations.MatchesFrame(f.targetBody, f.callerBody, uint32(f.point), f.occurrence) && f.control == linkedCallControlOrdinary && len(f.outboundRoots) == f.inputRootCount() && f.boundary.valid(f)
	if !valid {
		return false
	}
	for _, ambient := range f.ambientCircuit {
		if ambient.value == 0 || ambient.path == 0 || ambient.target.symbol == 0 || ambient.target.slot == 0 ||
			ambient.target.path.Kind == keyspace.KindInvalid || f.targetKeys.FormatReadOnly(ambient.target.path) == "" {
			return false
		}
	}
	for _, bridge := range f.exitBridges {
		if bridge.input < 0 || bridge.input >= len(f.targetRoots.roots) || bridge.root.Slot == 0 ||
			bridge.root.Path.Kind != keyspace.KindResolverSym || f.targetKeys.FormatReadOnly(bridge.root.Path) == "" {
			return false
		}
	}
	return valid
}

func (t linkedFrameBoundaryTopology) valid(frame linkedRelationFrame) bool {
	if len(t.inputs) != len(frame.rootCircuit) || len(t.inputEdges) != len(frame.rootCircuit) ||
		len(t.persistentEdges) != len(frame.rootCircuit) || len(t.ambientEdges) != len(frame.ambientCircuit) ||
		len(t.exitEdges) != len(frame.exitBridges) || len(t.resultAliasEdges) != len(frame.resultSources) {
		return false
	}
	if len(t.destinations) == 0 || len(t.edges) == 0 {
		return len(t.destinations) == 0 && len(t.edges) == 0 && len(frame.rootCircuit) == 0 && frame.mutableAmbientCount() == 0 && len(frame.exitBridges) == 0 && len(frame.resultSelectors) == 0 && len(frame.resultSources) == 0
	}
	preimages := make([]int, len(t.destinations))
	optionalSuffix := false
	for _, destination := range t.destinations {
		if destination.optional && (destination.kind != linkedFrameBoundaryDestinationPersistentInput || destination.path.Kind == keyspace.KindInvalid) {
			return false
		}
		if destination.optional {
			optionalSuffix = true
		} else if optionalSuffix {
			return false
		}
	}
	for _, destination := range t.inputs {
		if destination < 0 || destination >= len(t.destinations) || t.destinations[destination].kind != linkedFrameBoundaryDestinationInput {
			return false
		}
	}
	for _, edge := range t.edges {
		if edge.kind == linkedFrameBoundarySourceInvalid || edge.destination < 0 || edge.destination >= len(t.destinations) {
			return false
		}
		preimages[edge.destination]++
		if edge.kind == linkedFrameBoundarySourceExitBridge && (edge.input < 0 || edge.input >= len(t.inputs) || edge.destination != t.inputs[edge.input]) {
			return false
		}
	}
	for input, edge := range t.inputEdges {
		if edge < 0 || edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceInput || t.edges[edge].input != input || t.edges[edge].destination != t.inputs[input] {
			return false
		}
	}
	for input, edge := range t.persistentEdges {
		if edge < 0 {
			continue
		}
		if edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceInput || t.edges[edge].input != input || !t.destinations[t.edges[edge].destination].optional {
			return false
		}
	}
	for ambient, edge := range t.ambientEdges {
		if edge < 0 {
			if frame.ambientCircuit[ambient].target.mutable {
				return false
			}
			continue
		}
		if edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceAmbient || t.edges[edge].ambient != ambient {
			return false
		}
	}
	for bridge, edge := range t.exitEdges {
		if edge < 0 || edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceExitBridge || t.edges[edge].exitBridge != bridge {
			return false
		}
	}
	for result, edges := range t.resultEdges {
		if len(edges) != len(t.results[result]) {
			return false
		}
		for index, edge := range edges {
			if edge < 0 || edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceResult || t.edges[edge].result != result || t.edges[edge].destination != t.results[result][index] {
				return false
			}
		}
	}
	for source, edges := range t.resultAliasEdges {
		for _, edge := range edges {
			if edge < 0 || edge >= len(t.edges) || t.edges[edge].kind != linkedFrameBoundarySourceResultAlias || t.edges[edge].resultAlias != source {
				return false
			}
		}
	}
	for _, count := range preimages {
		if count == 0 {
			return false
		}
	}
	return true
}

func freezeTargetExitBridges(reg *axis.Registry, target *relationProgramBody, frame *linkedRelationFrame) ([]linkedFrameExitBridge, error) {
	if reg == nil || target == nil || frame == nil || target.graph == nil || target.pathSemantics == nil || !target.pathSemantics.Valid() ||
		!target.roots.valid(target.keys) || len(target.roots.roots) != len(frame.rootCircuit) {
		return nil, fmt.Errorf("transformer: target exit bridge has no sealed path authority")
	}
	out := make([]linkedFrameExitBridge, 0, len(target.roots.roots))
	for index, carrier := range target.roots.roots {
		if index >= len(frame.outboundRoots) || frame.outboundRoots[index].Slot == 0 || frame.outboundRoots[index].Path.Kind == keyspace.KindInvalid {
			continue
		}
		sym := rootSymbol(carrier.slot)
		if sym == 0 {
			return nil, fmt.Errorf("transformer: target exit bridge root %d has no symbol", index)
		}
		visible, exact := target.pathSemantics.VisibleInputLocalPathKey(target.graph.Exit(), pathdom.NewPath(sym, ""))
		if !exact {
			continue
		}
		if visible.Kind != keyspace.KindResolverSym || target.keys.FormatReadOnly(visible) == "" {
			return nil, fmt.Errorf("transformer: target exit bridge root %d is outside target keyspace", index)
		}
		out = append(out, linkedFrameExitBridge{input: index, root: state.BoundaryRoot{
			Slot: carrier.slot, Path: visible, Value: product.Bottom(reg),
		}})
	}
	return out, nil
}

func frameExistentialNamespace(owner lexicalidentity.StableLexicalBodyID, point cfg.Point, occurrence uint32) state.BoundaryExistentialNamespace {
	return state.BoundaryExistentialNamespace{
		OwnerHi: binary.BigEndian.Uint64(owner[0:8]), OwnerLo: binary.BigEndian.Uint64(owner[8:16]),
		Point: uint32(point), Partition: occurrence,
	}
}

// frameCallResultCarrier seals the caller-owned structural address paired
// with one point-owned CallResult scalar.  ReturnSlot paths are reserved for
// the function's final N5 tuple; using a raw ret[n] path here would alias the
// descendants of every lexical call that returns the same ordinal.  The
// boundary existential reuses the frame's finite lexical namespace, so a
// recursive equation is stable while distinct call frames cannot collide.
func frameCallResultCarrier(keys *keyspace.KeySpace, owner lexicalidentity.StableLexicalBodyID, point cfg.Point, slot uint32) (key.Value, keyspace.Key, error) {
	if keys == nil || !keys.Valid() || point == 0 {
		return 0, keyspace.Key{}, fmt.Errorf("transformer: call result carrier has no lexical frame authority")
	}
	scalar := key.CallResult(uint32(point), slot)
	base := keys.FromPath(pathdom.Path{Root: fmt.Sprintf("ret[%d]", slot)})
	// A call result is point-owned: every finite lexical target alternative at
	// this caller point contributes to the same scalar and structural carrier.
	// Frame occurrence partitions allocation alpha-renaming, not result identity.
	path, exact := keys.ImportExistential(keys, base, frameExistentialNamespace(owner, point, 0))
	if scalar == 0 || !exact || path.Kind == keyspace.KindInvalid || keys.FormatReadOnly(path) == "" {
		return 0, keyspace.Key{}, fmt.Errorf("transformer: call result %d at point %d has no private structural carrier", slot, point)
	}
	return scalar, path, nil
}

func freezeFrameRootCircuit(frame callFrameNode, caller *relationProgramBody) ([]linkedFrameRoot, error) {
	if len(frame.values) != frame.shape.InputCount() || len(frame.paths) != len(frame.values) {
		return nil, fmt.Errorf("transformer: call frame root circuit width differs from target shape")
	}
	out := make([]linkedFrameRoot, 0, len(frame.values))
	index := 0
	for _, kind := range []RootKind{RootParam, RootCapture, RootGlobal, RootAmbient} {
		for ordinal := uint32(0); ordinal < frame.shape.count(kind); ordinal++ {
			if frame.values[index] == 0 {
				return nil, fmt.Errorf("transformer: call frame root circuit has a zero value selector")
			}
			wire := linkedFrameRoot{root: Root{Kind: kind, Index: ordinal}, value: frame.values[index], path: frame.paths[index]}
			if caller == nil || caller.relation.arena == nil || int(wire.value) >= len(caller.relation.arena.values) {
				return nil, fmt.Errorf("transformer: call frame root circuit has no caller lens")
			}
			node := caller.relation.arena.values[wire.value]
			if node.op == valueRoot {
				wire.destination.valueRoot, wire.destination.hasValueRoot = node.root, true
				if node.root.Kind == RootMiddle {
					for _, entry := range caller.relation.arena.middle.entries {
						if entry.middle != node.root {
							continue
						}
						wire.destination.persistentRoot, wire.destination.hasPersistentRoot = entry.input, true
						wire.destination.persistentPath, wire.destination.hasPersistent = caller.rootPathKey(entry.input)
						if !wire.destination.hasPersistent {
							return nil, fmt.Errorf("transformer: call frame persistent input has no caller path")
						}
						break
					}
				}
			}
			if wire.path != 0 {
				path, exact := guardedDynamicTermPath(caller, wire.path)
				if !exact {
					return nil, fmt.Errorf("transformer: call frame input path has no structural lens")
				}
				wire.destination.path = caller.keys.FromPath(path)
				wire.destination.hasPath = wire.destination.path.Kind != keyspace.KindInvalid
			}
			out = append(out, wire)
			index++
		}
	}
	return out, nil
}

func freezeLinkedFrameBoundaryTopology(caller *relationProgramBody, frame *linkedRelationFrame) (linkedFrameBoundaryTopology, error) {
	if caller == nil || frame == nil || caller.keys == nil || !caller.keys.Valid() {
		return linkedFrameBoundaryTopology{}, fmt.Errorf("transformer: Apply boundary topology has no caller authority")
	}
	t := linkedFrameBoundaryTopology{
		inputs: make([]int, len(frame.rootCircuit)), results: make(map[uint32][]int),
		inputEdges: make([]int, len(frame.rootCircuit)), persistentEdges: make([]int, len(frame.rootCircuit)),
		ambientEdges: make([]int, len(frame.ambientCircuit)), exitEdges: make([]int, len(frame.exitBridges)),
		resultEdges: make(map[uint32][]int), resultAliasEdges: make([][]int, len(frame.resultSources)),
	}
	for index := range t.persistentEdges {
		t.persistentEdges[index] = -1
	}
	for index := range t.ambientEdges {
		t.ambientEdges[index] = -1
	}
	appendDestination := func(destination linkedFrameBoundaryDestination) int {
		ordinal := len(t.destinations)
		t.destinations = append(t.destinations, destination)
		return ordinal
	}
	appendEdge := func(edge linkedFrameBoundaryEdge) int {
		ordinal := len(t.edges)
		t.edges = append(t.edges, edge)
		return ordinal
	}
	type pendingPersistentDestination struct {
		input       int
		source      state.BoundaryFactorRoot
		destination linkedFrameBoundaryDestination
	}
	var persistent []pendingPersistentDestination
	for index, wire := range frame.rootCircuit {
		destination := linkedFrameBoundaryDestination{kind: linkedFrameBoundaryDestinationInput, input: index, valueRoot: wire.destination.valueRoot, hasRoot: wire.destination.hasValueRoot}
		if index < len(frame.outboundRoots) && frame.outboundRoots[index].Path.Kind != keyspace.KindInvalid {
			destination.path = frame.outboundRoots[index].Path
		} else if wire.destination.hasPath {
			destination.path = wire.destination.path
		} else if wire.destination.hasValueRoot {
			destination.path, _ = caller.rootPathKey(wire.destination.valueRoot)
		}
		ordinal := appendDestination(destination)
		t.inputs[index] = ordinal
		source := frame.targetRoots.roots[index]
		t.inputEdges[index] = appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceInput, input: index, destination: ordinal, source: state.BoundaryFactorRoot{Slot: source.slot, Path: source.path}})
		if wire.root.Kind == RootParam && wire.destination.hasPersistent {
			persistent = append(persistent, pendingPersistentDestination{input: index, source: state.BoundaryFactorRoot{Slot: source.slot, Path: source.path}, destination: linkedFrameBoundaryDestination{kind: linkedFrameBoundaryDestinationPersistentInput, input: index, valueRoot: wire.destination.persistentRoot, hasRoot: wire.destination.hasPersistentRoot, path: wire.destination.persistentPath, optional: true}})
		}
	}
	for index, ambient := range frame.ambientCircuit {
		if !ambient.target.mutable {
			continue
		}
		outbound := len(frame.rootCircuit) + index
		if outbound >= len(frame.outboundRoots) {
			return linkedFrameBoundaryTopology{}, fmt.Errorf("transformer: Apply mutable ambient %d has no caller destination", index)
		}
		destination := appendDestination(linkedFrameBoundaryDestination{kind: linkedFrameBoundaryDestinationAmbient, input: index, slot: frame.outboundRoots[outbound].Slot, path: frame.outboundRoots[outbound].Path})
		t.ambientEdges[index] = appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceAmbient, ambient: index, destination: destination, source: state.BoundaryFactorRoot{Slot: ambient.target.slot, Path: ambient.target.path}})
	}
	for index, bridge := range frame.exitBridges {
		t.exitEdges[index] = appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceExitBridge, exitBridge: index, input: bridge.input, destination: t.inputs[bridge.input], source: state.BoundaryFactorRoot{Slot: bridge.root.Slot, Path: bridge.root.Path}})
	}
	for _, result := range frame.resultSelectors {
		for targetIndex, target := range result.targets {
			if !target.stateTarget {
				continue
			}
			kind := linkedFrameBoundaryDestinationStateResult
			if targetIndex == 0 {
				kind = linkedFrameBoundaryDestinationCanonicalResult
			}
			ordinal := appendDestination(linkedFrameBoundaryDestination{kind: kind, result: result.slot, slot: target.slot, path: target.path})
			t.results[result.slot] = append(t.results[result.slot], ordinal)
		}
		for _, destination := range t.results[result.slot] {
			t.resultEdges[result.slot] = append(t.resultEdges[result.slot], appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceResult, result: result.slot, destination: destination, root: Root{Kind: RootResult, Index: result.slot}, source: state.BoundaryFactorRoot{Slot: key.ReturnSlot(int(result.slot))}}))
		}
	}
	for index, source := range frame.resultSources {
		for _, destination := range t.results[source.result] {
			t.resultAliasEdges[index] = append(t.resultAliasEdges[index], appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceResultAlias, result: source.result, resultAlias: index, destination: destination, source: state.BoundaryFactorRoot{Slot: key.ReturnSlot(int(source.result)), Path: source.path}}))
		}
	}
	// Conditional destinations are a canonical suffix. Runtime may compact an
	// inactive subset without changing any mandatory destination ordinal, which
	// is part of boundary-existential identity and therefore semantic.
	for _, pending := range persistent {
		destination := appendDestination(pending.destination)
		t.persistentEdges[pending.input] = appendEdge(linkedFrameBoundaryEdge{kind: linkedFrameBoundarySourceInput, input: pending.input, destination: destination, source: pending.source})
	}
	return t, nil
}

// RelationProgram is one frozen forest transaction. Dense relation variables
// are assigned by the byte order of stable lexical body IDs, independent of
// input order, maps, CFG process IDs, or solve generations.
type RelationProgram struct {
	registry         *axis.Registry
	bodies           []relationProgramBody
	byBody           map[lexicalidentity.StableLexicalBodyID]relationVar
	recursiveSCCs    [][]relationVar
	definitions      []relationProgramDefinition
	formalSlots      *SlotSpace
	formalFibers     *formalFiberInventory
	formalComponents *formalComponentTerminalSchema
	formalRegion     *formalRelationRegionInventory
	formalGuards     *formalGuardVocabulary
	formalTemplate   *formalRelationTemplate
}

func planNeedsPathSemanticAuthority(plan *operationplan.Plan) bool {
	if plan == nil {
		return false
	}
	facts := plan.Facts()
	for raw := 0; raw < plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		if len(facts.BranchPresenceRelations(point)) != 0 || len(facts.BranchPathRelations(point)) != 0 ||
			len(facts.CallResultValues(point)) != 0 || len(facts.PostconditionRefinements(point)) != 0 || len(facts.PostconditionPathRelations(point)) != 0 {
			return true
		}
		if source, ok := facts.BranchConditionSource(point); ok && source.Kind == factflow.ValueSourceExpression && source.HasExpr {
			if _, dynamic := facts.DynamicIndexExpression(source.ExprRef); dynamic {
				return true
			}
		}
	}
	return false
}

// FreezeRelationProgram validates the complete forest and freezes every body
// exactly once. A lexical call may resolve only through its sealed CallSurface.
func FreezeRelationProgram(units []RelationProgramUnit, callTopology operationplan.CallTopology) (*RelationProgram, error) {
	if len(units) == 0 {
		return nil, fmt.Errorf("transformer: relation program has no lexical units")
	}
	if !callTopology.Complete() {
		return nil, fmt.Errorf("transformer: relation program has no complete call topology authority")
	}
	ordered := append([]RelationProgramUnit(nil), units...)
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i].Body[:], ordered[j].Body[:]) < 0 })
	topologyBodies := callTopology.Bodies()
	if len(topologyBodies) != len(ordered) {
		return nil, fmt.Errorf("transformer: call topology body inventory %d differs from lexical unit inventory %d", len(topologyBodies), len(ordered))
	}
	byBody := make(map[lexicalidentity.StableLexicalBodyID]relationVar, len(ordered))
	shapes := make(map[lexicalidentity.StableLexicalBodyID]Shape, len(ordered))
	var registry *axis.Registry
	productDomains := make(map[lexicalidentity.StableLexicalBodyID]state.ProductDomain, len(ordered))
	for index, unit := range ordered {
		unitDomain := unit.Domain.Lattice()
		if unit.Body == (lexicalidentity.StableLexicalBodyID{}) || index > 0 && unit.Body == ordered[index-1].Body {
			return nil, fmt.Errorf("transformer: relation program has zero or duplicate lexical body")
		}
		if topologyBodies[index] != unit.Body {
			return nil, fmt.Errorf("transformer: call topology body inventory differs at lexical body %s", unit.Body)
		}
		if unit.Registry == nil || unit.KeySpace == nil || !unit.KeySpace.Valid() || unit.Graph == nil || unit.Plan == nil || unit.Graph.Size() != unit.Plan.PointCount() {
			return nil, fmt.Errorf("transformer: lexical body %s has incomplete registry/keyspace/graph/plan ownership", unit.Body)
		}
		if !unit.Domain.Valid() || unit.Domain.Registry() != unit.Registry || unitDomain.Bottom == nil || unitDomain.Equal == nil || unitDomain.LessOrEq == nil || unitDomain.Join == nil || unitDomain.Widen == nil {
			return nil, fmt.Errorf("transformer: lexical body %s has no exact application State domain", unit.Body)
		}
		productDomains[unit.Body] = unit.Domain
		if !unit.EntrySeedPlan.Valid() {
			return nil, fmt.Errorf("transformer: lexical body %s has no prepared entry-seed authority", unit.Body)
		}
		if !unit.InitialStatePlan.ValidFor(unit.Body, unit.Graph.ID(), unit.Graph.Size()) {
			return nil, fmt.Errorf("transformer: lexical body %s has no exact initial-state authority", unit.Body)
		}
		if planNeedsPathSemanticAuthority(unit.Plan) && (unit.PathSemantics == nil || !unit.PathSemantics.Valid()) {
			return nil, fmt.Errorf("transformer: lexical body %s has State path semantics without frozen path authority", unit.Body)
		}
		if planHasFact(unit.Plan, operationplan.RootAssignment) && (unit.RootAssignments == nil || !unit.RootAssignments.Valid()) {
			return nil, fmt.Errorf("transformer: lexical body %s has root assignments without frozen N4 authority", unit.Body)
		}
		if planHasFact(unit.Plan, operationplan.Return) && (unit.Returns == nil || !unit.Returns.Valid()) {
			return nil, fmt.Errorf("transformer: lexical body %s has returns without frozen N5 authority", unit.Body)
		}
		if planHasExtension(unit.Plan, operationplan.BodyGenericFor) && unit.GenericForMembership == nil {
			return nil, fmt.Errorf("transformer: lexical body %s has generic-for operations without membership authority", unit.Body)
		}
		if registry == nil {
			registry = unit.Registry
		} else if registry != unit.Registry {
			return nil, fmt.Errorf("transformer: lexical body %s belongs to a foreign axis registry", unit.Body)
		}
		ownedSurface, exact := unit.Plan.CallSurface()
		if !exact || !ownedSurface.Complete() || ownedSurface.Owner() != unit.Body || ownedSurface.PointCount() != unit.Graph.Size() {
			return nil, fmt.Errorf("transformer: lexical body %s has no exact operation-plan-owned call surface", unit.Body)
		}
		hasExternal := false
		for _, site := range ownedSurface.Sites() {
			if allocation, exact := unit.Plan.SignatureAllocationOperation(site.Point); exact {
				signatureCall, signatureExact := unit.Plan.SignatureCallOperation(site.Point)
				if site.Target.Kind() != operationplan.CallSurfaceTargetExternal ||
					!signatureExact || !site.Target.MatchesExternalOperation(signatureCall) {
					return nil, fmt.Errorf("transformer: lexical body %s allocation call at %d has no exact external signature surface", unit.Body, site.Point)
				}
				template, templateExact := effectlowering.StaticSignatureAllocationTemplate(signatureCall.Signature())
				expected, expectedExact := operationplan.NewSignatureAllocationOperation(allocation.Site(), template)
				if !templateExact || !expectedExact || !allocationOperationEqual(allocation, expected) ||
					allocation.Site().Ordinal != uint32(site.Point) {
					return nil, fmt.Errorf("transformer: lexical body %s allocation call at %d has no exact external signature surface", unit.Body, site.Point)
				}
			}
			if site.Target.Kind() == operationplan.CallSurfaceTargetLexical {
				continue
			}
			// A static signature allocation is the closed Effect/value
			// transaction frozen below, not an ExternalCall provider. Its presence
			// in the complete call census must not manufacture provider ownership.
			if _, allocation := unit.Plan.SignatureAllocationOperation(site.Point); allocation {
				continue
			}
			hasExternal = true
			if unit.ExternalCallOutcome.Empty() {
				return nil, fmt.Errorf("transformer: lexical body %s external call at %d has no canonical producer", unit.Body, site.Point)
			}
		}
		if hasExternal && len(unit.NodeReads) != unit.Graph.Size() {
			return nil, fmt.Errorf("transformer: lexical body %s external read dependency width %d differs from graph %d", unit.Body, len(unit.NodeReads), unit.Graph.Size())
		}
		if !unit.Plan.BoundaryParamsValid() || !unit.Plan.BoundaryCapturesValid() || !unit.Plan.BoundaryGlobalsValid() ||
			len(unit.Plan.BoundaryParams()) != int(unit.Shape.Params) || len(unit.Plan.BoundaryCaptures()) != int(unit.Shape.Captures) || len(unit.Plan.BoundaryGlobals()) != int(unit.Shape.Globals) {
			return nil, fmt.Errorf("transformer: lexical body %s shape differs from its sealed boundary namespaces", unit.Body)
		}
		if unit.Shape.HeapTemplates != 0 {
			return nil, fmt.Errorf("transformer: lexical body %s has owner-local heap-template namespaces without a sealed allocation schema", unit.Body)
		}
		// The output-root namespace is semantic program structure, not caller
		// metadata. Return facts and declared boundary returns jointly define its
		// exact lower bound; freeze that bound before SlotSpace and every formal
		// carrier are built so N5 never has to invent or reject an Output root at
		// execution time.
		if results := uint32(planReturnArity(unit.Plan)); unit.Shape.Results < results {
			unit.Shape.Results = results
			ordered[index].Shape.Results = results
		}
		variable := relationVar(index + 1)
		byBody[unit.Body], shapes[unit.Body] = variable, unit.Shape
	}

	surfaces := make([]programCallSurface, len(ordered))
	identityTargets := make(map[identity.ID]lexicalidentity.StableLexicalBodyID)
	targetIdentities := make(map[lexicalidentity.StableLexicalBodyID]identity.ID)
	for index, unit := range ordered {
		surface := programCallSurface{targets: make(map[cfg.Point]frozenLexicalCallSite)}
		ownedSurface, _ := unit.Plan.CallSurface()
		for _, site := range ownedSurface.Sites() {
			targetBody, lexical := site.Target.LexicalBody()
			if !lexical {
				// External calls remain owned by their sealed signature operation;
				// rejected calls remain explicit residue in the plan and fail closed
				// in ordinary lowering. Neither receives a relation variable.
				continue
			}
			variable, exists := byBody[targetBody]
			if !exists {
				return nil, fmt.Errorf("transformer: lexical body %s calls missing body %s", unit.Body, targetBody)
			}
			targetIndex := int(variable - 1)
			targetPlan := ordered[targetIndex].Plan
			if !targetPlan.BoundaryCapturesValid() || !targetPlan.BoundaryGlobalsValid() {
				return nil, fmt.Errorf("transformer: lexical target %s has no exact capture/global order", targetBody)
			}
			target := frozenLexicalCallTarget{variable: variable, shape: shapes[targetBody], results: uint32(planReturnArity(targetPlan)), boundary: DirectCallBoundary{Captures: targetPlan.BoundaryCaptures(), Globals: targetPlan.BoundaryGlobals()}}
			surface.targets[site.Point] = frozenLexicalCallSite{candidates: []frozenLexicalCallCandidate{{target: target}}}
		}
		calls := callTopology.Sites(unit.Body)
		for callIndex, call := range calls {
			if callIndex > 0 && calls[callIndex-1].Point() == call.Point() {
				return nil, fmt.Errorf("transformer: lexical body %s has duplicate finite call point %d", unit.Body, call.Point())
			}
			classified, found := ownedSurface.Site(call.Point())
			candidates := call.Candidates()
			if !found || classified.Target.Kind() != operationplan.CallSurfaceTargetRejected || len(candidates) == 0 {
				return nil, fmt.Errorf("transformer: lexical body %s finite call point %d is not a rejected call with candidates", unit.Body, call.Point())
			}
			if _, duplicate := surface.targets[call.Point()]; duplicate {
				return nil, fmt.Errorf("transformer: lexical body %s finite call point %d overlaps an exact lexical call", unit.Body, call.Point())
			}
			frozen := frozenLexicalCallSite{candidates: make([]frozenLexicalCallCandidate, 0, len(candidates)), residual: call.Residual()}
			for candidateIndex, candidate := range candidates {
				if candidate.Identity.Kind != "lua.function" || candidate.Identity.Site != "symbol" || candidate.Identity.Index == 0 ||
					candidate.Target == (lexicalidentity.StableLexicalBodyID{}) || candidateIndex > 0 && candidate.Identity == candidates[candidateIndex-1].Identity {
					return nil, fmt.Errorf("transformer: lexical body %s finite call point %d has malformed or duplicate identity", unit.Body, call.Point())
				}
				variable, exists := byBody[candidate.Target]
				if !exists {
					return nil, fmt.Errorf("transformer: lexical body %s finite call point %d targets missing body %s", unit.Body, call.Point(), candidate.Target)
				}
				if prior, exists := identityTargets[candidate.Identity]; exists && prior != candidate.Target {
					return nil, fmt.Errorf("transformer: function identity at point %d aliases two lexical bodies", call.Point())
				}
				if prior, exists := targetIdentities[candidate.Target]; exists && prior != candidate.Identity {
					return nil, fmt.Errorf("transformer: lexical target %s aliases two function identities", candidate.Target)
				}
				identityTargets[candidate.Identity], targetIdentities[candidate.Target] = candidate.Target, candidate.Identity
				targetIndex := int(variable - 1)
				targetPlan := ordered[targetIndex].Plan
				if !targetPlan.BoundaryCapturesValid() || !targetPlan.BoundaryGlobalsValid() {
					return nil, fmt.Errorf("transformer: finite lexical target %s has no exact capture/global order", candidate.Target)
				}
				frozen.candidates = append(frozen.candidates, frozenLexicalCallCandidate{identity: candidate.Identity, target: frozenLexicalCallTarget{
					variable: variable, shape: shapes[candidate.Target], results: uint32(planReturnArity(targetPlan)),
					boundary: DirectCallBoundary{Captures: targetPlan.BoundaryCaptures(), Globals: targetPlan.BoundaryGlobals()},
				}})
			}
			if !frozen.valid() {
				return nil, fmt.Errorf("transformer: lexical body %s finite call point %d did not freeze", unit.Body, call.Point())
			}
			surface.targets[call.Point()] = frozen
		}
		surfaces[index] = surface
	}
	environments, err := closeRelationEnvironments(ordered, surfaces, byBody)
	if err != nil {
		return nil, err
	}
	ambientRoots := make([][]AmbientRoot, len(ordered))
	for index := range ordered {
		inventory := make([]AmbientRoot, len(environments.ambient[index]))
		for rootIndex, id := range environments.ambient[index] {
			_, mutable := environments.mutable[index][id]
			inventory[rootIndex] = AmbientRoot{Symbol: id, Mutable: mutable}
		}
		if !validAmbientRoots(inventory) {
			return nil, fmt.Errorf("transformer: lexical body %s has non-canonical ambient roots", ordered[index].Body)
		}
		ordered[index].Shape.Ambients = uint32(len(inventory))
		shapes[ordered[index].Body] = ordered[index].Shape
		ambientRoots[index] = inventory
	}
	// Surface discovery needs only relation variables. Once closure conversion
	// has derived the complete callable shapes, update every frozen target before
	// opening a term arena or interning a call frame.
	for owner := range surfaces {
		for point, site := range surfaces[owner].targets {
			for candidate := range site.candidates {
				target := site.candidates[candidate].target.variable
				if target == 0 || int(target) > len(ordered) {
					return nil, fmt.Errorf("transformer: ambient closure has foreign call target")
				}
				site.candidates[candidate].target.shape = ordered[target-1].Shape
				site.candidates[candidate].target.boundary.Ambients = append([]AmbientRoot(nil), ambientRoots[target-1]...)
			}
			surfaces[owner].targets[point] = site
		}
	}

	prepared := make([]*PreparedPlanCompiler, len(ordered))
	rootCarriers := make([]relationRootCarrier, len(ordered))
	for index, unit := range ordered {
		carrier, carrierErr := sealRelationRootCarrierWithAmbients(unit.Plan, unit.KeySpace, unit.Shape, ambientRoots[index])
		if carrierErr != nil {
			return nil, fmt.Errorf("transformer: seal lexical body %s root carrier: %w", unit.Body, carrierErr)
		}
		rootCarriers[index] = carrier
		declarations := unit.DirectLexicalDeclarations
		if !declarations.matches(unit.Plan) {
			// The empty declaration set has one exact structural proof and needs no
			// binder. A non-empty set still requires the authority handed off by
			// production preparation and fails closed here.
			declarations, err = sealEmptyDirectLexicalDeclarationAuthority(unit.Plan)
			if err != nil {
				return nil, fmt.Errorf("transformer: prepare lexical body %s: %w", unit.Body, err)
			}
		}
		planCompiler := NewPlanCompiler().withExpressionValueFreeze(unit.CustomExpressionValue, unit.Domain.Lattice().Bottom())
		compiler, prepareErr := planCompiler.PrepareWithDirectLexicalDeclarationsAndBoundaryRoots(
			unit.Registry,
			unit.Graph,
			unit.Plan,
			unit.Shape,
			declarations,
		)
		if prepareErr != nil {
			return nil, fmt.Errorf("transformer: prepare lexical body %s: %w", unit.Body, prepareErr)
		}
		if sealErr := compiler.sealAmbientEnvironment(ambientRoots[index]); sealErr != nil {
			return nil, fmt.Errorf("transformer: seal lexical body %s ambient environment: %w", ordered[index].Body, sealErr)
		}
		prepared[index] = compiler
	}
	program := &RelationProgram{registry: registry, bodies: make([]relationProgramBody, len(ordered)), byBody: byBody}
	program.recursiveSCCs = recursiveRelationSCCsFromTopology(callTopology, ordered, byBody)
	// Freeze every lexical WorldProgram before any term/effect arena seals.
	// Acyclic dependency closure owns this open-arena interval; only the final
	// reduction below may publish immutable relation code.
	for index, unit := range ordered {
		surface := surfaces[index]
		if !prepared[index].builder.arena.bindLexicalOwner(unit.Body) {
			return nil, fmt.Errorf("transformer: lexical body %s could not bind its sealed term owner", unit.Body)
		}
		if err := prepared[index].freezeRelationProgramWorld(surface); err != nil {
			return nil, fmt.Errorf("transformer: freeze lexical body %s: %w", unit.Body, err)
		}
	}
	// Definition-analysis frames are ordinary sealed input lenses, but they are
	// not inserted into a body program as call steps. The tuple scheduler later
	// connects each owner point output to one canonical analysis application.
	for index, unit := range ordered {
		if len(unit.Definitions) == 0 {
			continue
		}
		definitions := append([]RelationProgramDefinition(nil), unit.Definitions...)
		sort.Slice(definitions, func(i, j int) bool {
			if definitions[i].Point != definitions[j].Point {
				return definitions[i].Point < definitions[j].Point
			}
			return bytes.Compare(definitions[i].Target[:], definitions[j].Target[:]) < 0
		})
		occurrences := make(map[cfg.Point]uint32)
		for _, definition := range definitions {
			target, ok := byBody[definition.Target]
			if !ok || definition.Point == 0 || int(definition.Point) >= unit.Graph.Size() || target == relationVar(index+1) {
				return nil, fmt.Errorf("transformer: lexical body %s has malformed definition target %s at %d", unit.Body, definition.Target, definition.Point)
			}
			targetUnit := ordered[target-1]
			values := make([]ValueTerm, 0, targetUnit.Shape.InputCount())
			paths := make([]PathTerm, 0, targetUnit.Shape.InputCount())
			arena := prepared[index].builder.arena
			params := targetUnit.Plan.BoundaryParams()
			if len(params) != int(targetUnit.Shape.Params) || !targetUnit.EntrySeedPlan.Valid() {
				return nil, fmt.Errorf("transformer: definition target %s has incomplete parameter seed authority", definition.Target)
			}
			for _, param := range params {
				contract, present := targetUnit.EntrySeedPlan.ValueForSlot(key.SymbolValue(param))
				if !present {
					return nil, fmt.Errorf("transformer: definition target %s parameter %d has no exact entry seed", definition.Target, param)
				}
				values, paths = append(values, arena.Constant(contract)), append(paths, 0)
			}
			ambientSymbols := make([]symbol.ID, len(ambientRoots[target-1]))
			for ambientIndex, root := range ambientRoots[target-1] {
				ambientSymbols[ambientIndex] = root.Symbol
			}
			for _, symbols := range [][]symbol.ID{targetUnit.Plan.BoundaryCaptures(), targetUnit.Plan.BoundaryGlobals(), ambientSymbols} {
				for _, id := range symbols {
					value := arena.bindEnvironmentSymbol(id)
					path := arena.EnvironmentPath(id)
					if value == 0 || path == 0 {
						return nil, fmt.Errorf("transformer: definition target %s has unbound environment symbol %d", definition.Target, id)
					}
					values, paths = append(values, value), append(paths, path)
				}
			}
			occurrences[definition.Point]++
			frame := arena.relationFrame(target, definition.Point, occurrences[definition.Point], targetUnit.Shape, values, paths, 0)
			if frame == 0 {
				return nil, fmt.Errorf("transformer: definition target %s has no sealed input frame", definition.Target)
			}
			program.definitions = append(program.definitions, relationProgramDefinition{owner: relationVar(index + 1), target: target, point: definition.Point, frame: frame, externallyReachable: definition.ExternallyReachable})
		}
	}
	// Build the sole unsealed relationCode transducers while every owner arena
	// remains open. They are the dependency-closure substrate, not a second
	// runtime representation.
	for index, unit := range ordered {
		if err := prepared[index].reduceRelationProgramWorldUnsealed(); err != nil {
			return nil, fmt.Errorf("transformer: reduce lexical body %s: %w", unit.Body, err)
		}
	}
	if err := closeRelationProgramTerms(prepared, ordered, program.definitions); err != nil {
		return nil, err
	}
	for index := range rootCarriers {
		// Result closure cannot change an input namespace, but the carrier retains
		// the complete callable Shape as its ownership certificate.
		rootCarriers[index].shape = ordered[index].Shape
	}
	codes := make([]*relationCode, len(ordered))
	for index := range ordered {
		codes[index] = prepared[index].codeBase
	}
	if err := closeRelationGuardBoundarySyntax(codes); err != nil {
		return nil, err
	}
	// Term closure above is the last syntax-construction phase. Seal every
	// scalar/path/effect owner before deriving the immutable cross-body guard
	// vocabulary; plan attachment remains relation metadata and cannot mint
	// syntax in either arena.
	for index := range ordered {
		if err := prepared[index].sealRelationProgramSyntax(); err != nil {
			return nil, fmt.Errorf("transformer: seal lexical body %s syntax: %w", ordered[index].Body, err)
		}
	}
	if err := freezeRelationApplicationGuardPlans(codes); err != nil {
		return nil, err
	}
	if err := freezeRelationDefinitionGuardPlans(codes, program.definitions); err != nil {
		return nil, err
	}
	// Seal owner arenas and link frame/tuple authority. relationCode is the sole
	// retained executable representation; the invocation plan composes those
	// immutable per-body relations directly at solve time.
	for index, unit := range ordered {
		if err := prepared[index].sealRelationProgramWorld(); err != nil {
			return nil, fmt.Errorf("transformer: seal lexical body %s: %w", unit.Body, err)
		}
		ambientSymbols := make([]symbol.ID, len(prepared[index].ambientRoots))
		mutableAmbient := make(map[symbol.ID]struct{})
		for rootIndex, root := range prepared[index].ambientRoots {
			ambientSymbols[rootIndex] = root.Symbol
			if root.Mutable {
				mutableAmbient[root.Symbol] = struct{}{}
			}
		}
		ambient, ambientErr := sealRelationEnvironmentRoots(unit.KeySpace, ambientSymbols, mutableAmbient)
		if ambientErr != nil {
			return nil, fmt.Errorf("transformer: seal lexical body %s ambient roots: %w", unit.Body, ambientErr)
		}
		reads := make([][]cfg.Point, len(unit.NodeReads))
		for point := range unit.NodeReads {
			reads[point] = append([]cfg.Point(nil), unit.NodeReads[point]...)
		}
		program.bodies[index] = relationProgramBody{body: unit.Body, variable: relationVar(index + 1), keys: unit.KeySpace, roots: rootCarriers[index], ambient: ambient, relation: prepared[index].frozenRelation(), plan: unit.Plan, graph: unit.Graph, pathSemantics: unit.PathSemantics, rootAssignments: unit.RootAssignments, returns: unit.Returns, externalCalls: unit.ExternalCallOutcome, genericForMembership: unit.GenericForMembership, nodeReads: reads, domain: unit.Domain.Lattice(), productDomain: productDomains[unit.Body], entrySeedPlan: unit.EntrySeedPlan.Clone(), initialStatePlan: unit.InitialStatePlan.Clone(), definitionFrames: make(map[callFrameTerm]relationVar)}
	}
	for _, definition := range program.definitions {
		program.bodies[definition.owner-1].definitionFrames[definition.frame] = definition.target
	}
	// Receiver assignments are part of the outbound frame lens: link them
	// before frame transport is frozen so every call-result root owns its exact
	// caller slot/path. They are not a post-link execution annotation.
	for index := range program.bodies {
		program.bodies[index].callReceivers = indexRelationCallReceivers(program.bodies[index].relation.code)
	}
	if err := program.linkFrozenFrames(); err != nil {
		return nil, err
	}
	for index := range program.bodies {
		if len(program.bodies[index].callReceivers) != 0 {
			surface, _ := program.bodies[index].plan.CallSurface()
			for callPoint, receivers := range program.bodies[index].callReceivers {
				site, exact := surface.Site(callPoint)
				if !exact || site.Target.Kind() != operationplan.CallSurfaceTargetLexical {
					continue
				}
				if program.bodies[index].callReceiverAssignments == nil {
					program.bodies[index].callReceiverAssignments = make(map[cfg.Point]struct{})
				}
				for _, receiver := range receivers {
					program.bodies[index].callReceiverAssignments[receiver.transaction.Point()] = struct{}{}
				}
			}
		}
	}
	formalSlots, err := freezeSlotSpace(program)
	if err != nil {
		return nil, err
	}
	program.formalSlots = formalSlots
	formalRegion, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		return nil, err
	}
	program.formalRegion = formalRegion
	formalFibers, err := freezeFormalFiberInventoryWithSlots(program, formalSlots)
	if err != nil {
		return nil, err
	}
	program.formalFibers = formalFibers
	// Coordinate and identity footprint closure deliberately sees every lexical
	// Step. Once those semantic operators are frozen, collapse unobservable
	// acyclic Step chains before any WTO/template authority is published.
	if err := formalRegion.freezeObservableStepQuotient(program); err != nil {
		return nil, err
	}
	formalComponents, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		return nil, err
	}
	program.formalComponents = formalComponents
	formalGuards, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		return nil, err
	}
	if !formalGuards.valid() {
		return nil, fmt.Errorf("transformer: formal guard vocabulary failed ownership validation")
	}
	program.formalGuards = formalGuards
	formalTemplate, err := freezeFormalRelationTemplate(program)
	if err != nil {
		return nil, err
	}
	program.formalTemplate = formalTemplate
	return program, nil
}

// linkedFrameEnvironmentRoot resolves one descendant ambient dependency from
// the caller's sole lexical storage vocabulary. A symbol already owned by the
// caller Shape reuses that formal root; only truly indirect symbols use the
// explicit environment carrier.
func linkedFrameEnvironmentRoot(arena *Arena, roots relationRootCarrier, id symbol.ID) (ValueTerm, PathTerm, bool) {
	if arena == nil || id == 0 {
		return 0, 0, false
	}
	for _, carrier := range roots.roots {
		if rootSymbol(carrier.slot) != id {
			continue
		}
		value, path := arena.Root(carrier.root), arena.Path(carrier.root)
		return value, path, value != 0 && path != 0
	}
	value, exact := arena.environmentValue(id)
	path := arena.EnvironmentPath(id)
	return value, path, exact && value != 0 && path != 0
}

// linkFrozenFrames binds every structural ApplyRef to its target relation and
// complete target allocation-template inventory exactly once after all arenas
// have sealed. Hot application consumes only this dense table.
func (p *RelationProgram) linkFrozenFrames() error {
	if p == nil {
		return fmt.Errorf("transformer: nil relation program")
	}
	for callerIndex := range p.bodies {
		caller := &p.bodies[callerIndex]
		arena := caller.relation.arena
		if arena == nil || !arena.Sealed() {
			return fmt.Errorf("transformer: relation variable %d has no sealed term owner", caller.variable)
		}
		rootTemplates, err := relationAllocationTemplates(caller.relation)
		if err != nil {
			return fmt.Errorf("transformer: relation variable %d root allocation link: %w", caller.variable, err)
		}
		caller.rootAllocations, err = state.NewBoundaryAllocationAuthority(state.RootBoundaryAllocationRoute(caller.body), rootTemplates)
		if err != nil || !caller.rootAllocations.MatchesRoot(caller.body) {
			return fmt.Errorf("transformer: relation variable %d has no root allocation authority: %w", caller.variable, err)
		}
		caller.frames = make([]linkedRelationFrame, len(arena.callFrames))
		for frameTerm := callFrameTerm(1); int(frameTerm) < len(arena.callFrames); frameTerm++ {
			frame := arena.callFrames[frameTerm]
			_, definitionFrame := caller.definitionFrames[frameTerm]
			if frame.variable == 0 || int(frame.variable) > len(p.bodies) {
				return fmt.Errorf("transformer: relation variable %d retains a legacy or foreign call frame", caller.variable)
			}
			target := p.bodies[frame.variable-1].relation
			if target.arena == nil || !target.arena.Sealed() {
				return fmt.Errorf("transformer: call frame %d target is not sealed", frameTerm)
			}
			templates, err := relationAllocationTemplates(target)
			if err != nil {
				return fmt.Errorf("transformer: call frame %d target allocations: %w", frameTerm, err)
			}
			targetBody := p.bodies[frame.variable-1].body
			rootCircuit, err := freezeFrameRootCircuit(frame, caller)
			if err != nil {
				return fmt.Errorf("transformer: call frame %d substitution circuit: %w", frameTerm, err)
			}
			ambientCircuit := make([]linkedFrameAmbientRoot, 0, len(p.bodies[frame.variable-1].ambient))
			for _, ambient := range p.bodies[frame.variable-1].ambient {
				value, path, exact := linkedFrameEnvironmentRoot(arena, caller.roots, ambient.symbol)
				if !exact || value == 0 || path == 0 {
					return fmt.Errorf("transformer: call frame %d ambient symbol %d has no caller environment term", frameTerm, ambient.symbol)
				}
				ambientCircuit = append(ambientCircuit, linkedFrameAmbientRoot{value: value, path: path, target: ambient})
			}
			lens, err := state.NewBoundaryAllocationAuthority(state.ApplyBoundaryAllocationRoute(targetBody, caller.body, uint32(frame.point), frame.occurrence), templates)
			if err != nil {
				return fmt.Errorf("transformer: call frame %d allocation link: %w", frameTerm, err)
			}
			if !lens.MatchesFrame(targetBody, caller.body, uint32(frame.point), frame.occurrence) {
				return fmt.Errorf("transformer: call frame %d allocation authority drifted during link", frameTerm)
			}
			callSite, hasCallSite := caller.plan.Facts().CallSiteView(frame.point)
			if !definitionFrame && !hasCallSite {
				return fmt.Errorf("transformer: call frame %d has no frozen call-site result schema", frameTerm)
			}
			resultCount := int(frame.resultCount)
			// Width is frozen before Middle closure by
			// closeRelationCallResultMiddleSchemas. Linking consumes that authority;
			// it must never rediscover output vocabulary after arenas have sealed.
			results := make([]linkedFrameResult, resultCount)
			for slot := range results {
				// Every call result owns one canonical addressable SSA carrier,
				// independent of how the source syntax consumes it.  Lexical
				// assignments and direct returns are projections from this root;
				// expression-to-expression composition reuses it directly.  The
				// path half is essential: it carries descendant/object facts that
				// cannot be recovered from the scalar product value alone.
				scalar, path, carrierErr := frameCallResultCarrier(caller.keys, caller.body, frame.point, uint32(slot))
				if carrierErr != nil {
					return fmt.Errorf("transformer: call frame %d result carrier: %w", frameTerm, carrierErr)
				}
				results[slot] = linkedFrameResult{
					slot: uint32(slot),
					targets: []linkedFrameResultTarget{{
						slot: scalar, path: path, stateTarget: true,
					}},
				}
			}
			if definitionFrame && frame.resultCount != 0 {
				return fmt.Errorf("transformer: definition frame %d owns call results", frameTerm)
			}
			validTargets := true
			invalidTargetReason := ""
			if hasCallSite && !definitionFrame {
				callSite.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
					resultIndex := target.ResultIndex()
					if resultIndex < 0 || resultIndex >= len(results) {
						validTargets = false
						invalidTargetReason = fmt.Sprintf("result index %d is outside frame width %d", resultIndex, len(results))
						return false
					}
					selector := linkedFrameResultTarget{kind: target.Kind()}
					destinationPoint, hasDestinationPoint := relationCallReceiverPoint(caller.relation.code, frame.point, target.Index())
					switch target.Kind() {
					case factflow.CallResultTargetLocalAssignment:
						symbol := target.TargetSymbol()
						if symbol == 0 || target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 || target.TargetPathRef().Symbol != symbol {
							validTargets = false
							invalidTargetReason = "local assignment has no exact root"
							return false
						}
						selector.slot = key.SymbolValue(symbol)
						if caller.pathSemantics == nil {
							validTargets = false
							invalidTargetReason = "local assignment has no path authority"
							return false
						}
						if hasDestinationPoint {
							selector.path, _ = caller.pathSemantics.VisibleLocalPathKey(destinationPoint, target.TargetPathRef())
						}
					case factflow.CallResultTargetOrdinaryAssignment:
						if target.TargetPathEmpty() {
							// Dynamic-index assignment is represented by its subsequent
							// path-store transaction. The frame's canonical result carrier
							// already owns the scalar and descendants consumed there.
							if target.TargetSymbol() != 0 || target.Index() < 0 {
								validTargets = false
								invalidTargetReason = "dynamic ordinary assignment has malformed destination metadata"
								return false
							}
							return true
						}
						if target.TargetPathRef().Symbol == 0 || target.TargetSymbol() != target.TargetPathRef().Symbol {
							validTargets = false
							invalidTargetReason = "ordinary assignment has no exact path"
							return false
						}
						if caller.pathSemantics == nil {
							validTargets = false
							invalidTargetReason = "ordinary assignment has no path authority"
							return false
						}
						if hasDestinationPoint {
							selector.path, _ = caller.pathSemantics.VisibleLocalPathKey(destinationPoint, target.TargetPathRef())
						}
						if selector.path.Kind == keyspace.KindInvalid {
							selector.path = caller.keys.FromPath(target.TargetPathRef())
						}
					case factflow.CallResultTargetReturn:
						if target.Index() < 0 {
							validTargets = false
							invalidTargetReason = "return target has no exact destination index"
							return false
						}
						// A return consumer reads the point-owned CallResult during N5.
						// It must not prewrite the caller's final ReturnSlot tuple here.
						return true
					case factflow.CallResultTargetExpression:
						if target.Index() < 0 || target.TargetSymbol() != 0 || !target.TargetPathEmpty() {
							validTargets = false
							invalidTargetReason = "expression target has lexical address data"
							return false
						}
						// Expression consumption has no second destination: it reads
						// the canonical addressable result carrier installed above.
						// Keeping one root preserves both the scalar and its descendants.
						return true
					default:
						validTargets = false
						invalidTargetReason = fmt.Sprintf("unknown target kind %d", target.Kind())
						return false
					}
					if selector.path.Kind != keyspace.KindInvalid && caller.keys.FormatReadOnly(selector.path) == "" {
						validTargets = false
						invalidTargetReason = "target path is outside caller keyspace"
						return false
					}
					selector.stateTarget = selector.slot != 0 || selector.path.Kind != keyspace.KindInvalid
					results[resultIndex].targets = append(results[resultIndex].targets, selector)
					return true
				})
			}
			// A call used as a root-assignment source is a typed result
			// destination even when the CallSite syntax records only the
			// expression carrier. The canonical root-assignment transaction owns
			// the missing caller slot/path; add it to the same frame result lens so
			// outbound transport performs the write before the N4 completion tail.
			if !definitionFrame {
				for _, receiver := range caller.callReceivers[frame.point] {
					source, sourceOK := receiver.transaction.Source(0)
					if !sourceOK || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != frame.point ||
						source.TargetIndex < 0 || source.TargetIndex >= len(results) {
						validTargets = false
						invalidTargetReason = "call receiver has no exact result index"
						break
					}
					path, pathOK := receiver.transaction.TargetPath()
					if !pathOK || path.Symbol != receiver.transaction.TargetSymbol() || len(path.Segments) != 0 || caller.pathSemantics == nil {
						validTargets = false
						invalidTargetReason = "call receiver has no exact root path"
						break
					}
					target := linkedFrameResultTarget{
						kind: factflow.CallResultTargetLocalAssignment,
						slot: key.SymbolValue(receiver.transaction.TargetSymbol()), stateTarget: true,
					}
					target.path, pathOK = caller.pathSemantics.VisibleLocalPathKey(receiver.transaction.Point(), path)
					if !pathOK || target.path.Kind == keyspace.KindInvalid || caller.keys.FormatReadOnly(target.path) == "" {
						validTargets = false
						invalidTargetReason = "call receiver root is outside caller path authority"
						break
					}
					duplicate := false
					for _, prior := range results[source.TargetIndex].targets {
						duplicate = duplicate || prior.stateTarget && prior.slot == target.slot && prior.path == target.path
					}
					if !duplicate {
						results[source.TargetIndex].targets = append(results[source.TargetIndex].targets, target)
					}
				}
			}
			if !validTargets {
				return fmt.Errorf("transformer: call frame %d has an invalid result target: %s", frameTerm, invalidTargetReason)
			}
			resultSources, resultSourceErr := linkRelationFrameResultSources(&p.bodies[frame.variable-1], p.bodies[frame.variable-1].relation.code, results)
			if resultSourceErr != nil {
				return fmt.Errorf("transformer: call frame %d result-source lens: %w", frameTerm, resultSourceErr)
			}
			linked := linkedRelationFrame{
				owner: caller.variable, target: frame.variable, term: frameTerm,
				callerBody: caller.body, targetBody: targetBody, callerKeys: caller.keys, targetKeys: p.bodies[frame.variable-1].keys, targetRoots: p.bodies[frame.variable-1].roots,
				point: frame.point, occurrence: frame.occurrence, shape: frame.shape,
				rootCircuit: rootCircuit, closureProducer: frame.closureProducer, ambientCircuit: ambientCircuit, resultSelectors: results, resultSources: resultSources,
				control:      linkedCallControlOrdinary,
				route:        linkedFrameRouteAuthority{owner: caller.variable, target: frame.variable, frame: frameTerm},
				existentials: frameExistentialNamespace(caller.body, frame.point, frame.occurrence), allocations: lens,
			}
			linked.outboundRoots, err = freezeOutboundFrameRootSchema(p.registry, caller, &linked)
			if err != nil {
				return fmt.Errorf("transformer: call frame %d outbound root schema: %w", frameTerm, err)
			}
			if !definitionFrame {
				linked.exitBridges, err = freezeTargetExitBridges(p.registry, &p.bodies[frame.variable-1], &linked)
				if err != nil {
					return fmt.Errorf("transformer: call frame %d target exit bridge: %w", frameTerm, err)
				}
			}
			linked.boundary, err = freezeLinkedFrameBoundaryTopology(caller, &linked)
			if err != nil {
				return fmt.Errorf("transformer: call frame %d boundary topology: %w", frameTerm, err)
			}
			if !linked.valid() {
				return fmt.Errorf("transformer: call frame %d did not seal a complete boundary lens", frameTerm)
			}
			caller.frames[frameTerm] = linked
		}
	}
	return nil
}

func recursiveRelationSCCsFromTopology(topology operationplan.CallTopology, ordered []RelationProgramUnit, byBody map[lexicalidentity.StableLexicalBodyID]relationVar) [][]relationVar {
	components := make(map[uint32][]relationVar)
	for _, unit := range ordered {
		component := topology.Component(unit.Body)
		if component == 0 {
			continue
		}
		components[component] = append(components[component], byBody[unit.Body])
	}
	ids := make([]uint32, 0, len(components))
	for component := range components {
		ids = append(ids, component)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([][]relationVar, 0, len(ids))
	for _, component := range ids {
		members := components[component]
		sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
		out = append(out, members)
	}
	return out
}

func (p *PreparedPlanCompiler) freezeRelationProgramWorld(surface programCallSurface) error {
	p.freezeMu.Lock()
	defer p.freezeMu.Unlock()
	if p.frozen {
		return fmt.Errorf("compiler: lexical body was already frozen")
	}
	p.frozen, p.frozenDirect, p.freezeCount = true, len(surface.targets) != 0, 1
	world, err := p.freezeStructuralWorldProgramSurface(surface)
	if err != nil {
		p.freezeErr = fmt.Errorf("compiler: world program: %w", err)
		return p.freezeErr
	}
	p.worldBase = world
	return nil
}

func (p *PreparedPlanCompiler) reduceRelationProgramWorldUnsealed() error {
	p.freezeMu.Lock()
	defer p.freezeMu.Unlock()
	if p == nil || !p.frozen || p.freezeErr != nil || !p.worldBase.valid(true) || p.codeBase != nil || p.rootBase != 0 || p.reductionCount != 0 {
		return fmt.Errorf("compiler: lexical WorldProgram is not ready for sole reduction")
	}
	var err error
	p.codeBase, p.rootBase, err = reduceWorldProgramUnsealed(p.worldBase, p.builder.descriptors)
	if err != nil {
		p.freezeErr = fmt.Errorf("compiler: boundary reduction: %w", err)
		return p.freezeErr
	}
	return nil
}

func (p *PreparedPlanCompiler) sealRelationProgramWorld() error {
	p.freezeMu.Lock()
	defer p.freezeMu.Unlock()
	if p == nil || !p.frozen || p.freezeErr != nil || p.codeBase == nil || p.rootBase == 0 || p.reductionCount != 0 || !p.builder.arena.Sealed() || !p.builder.effects.Sealed() {
		return fmt.Errorf("compiler: syntax-sealed relation transducer is not ready for publication")
	}
	var err error
	p.codeBase, p.rootBase, err = sealRelationCode(p.codeBase, p.rootBase)
	if err != nil {
		p.freezeErr = fmt.Errorf("compiler: boundary seal: %w", err)
		return p.freezeErr
	}
	p.reductionCount = 1
	// relationCode is now the sole retained executable representation. The
	// WorldProgram arena was consumed by the one reduction above and has no
	// post-seal reader; release it with its owning prepared compiler.
	p.worldBase = WorldProgram{}
	return nil
}

// sealRelationProgramSyntax closes the complete term/effect circuit after
// forest-wide closure and before immutable cross-body metadata is derived.
// Relation metadata remains attachable until sealRelationProgramWorld performs
// the sole whole-code validation transaction.
func (p *PreparedPlanCompiler) sealRelationProgramSyntax() error {
	p.freezeMu.Lock()
	defer p.freezeMu.Unlock()
	if p == nil || !p.frozen || p.freezeErr != nil || p.codeBase == nil || p.rootBase == 0 || p.reductionCount != 0 || p.builder.arena.Sealed() || p.builder.effects.Sealed() {
		return fmt.Errorf("compiler: relation syntax is not ready to seal")
	}
	p.builder.arena.Seal()
	p.builder.effects.Seal()
	return nil
}

// Variable returns the forest-stable dense variable for body.
func (p *RelationProgram) Variable(body lexicalidentity.StableLexicalBodyID) (uint32, bool) {
	if p == nil {
		return 0, false
	}
	variable, ok := p.byBody[body]
	return uint32(variable), ok
}
