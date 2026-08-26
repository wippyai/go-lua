package typ

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sync"

	"github.com/wippyai/go-lua/domain/type/kind"
)

// CanonicalGraphReceipt is the owner-issued quotient of one authored
// type graph.  It is deliberately a source plane rather than another type
// representation: node identities, lexical scope facts, and source ordinal
// edges are retained, while canonical wire bytes are not.
//
// The receipt is immutable at its persistent public boundary. Node and Nodes
// return ownership-isolated edge slices. The mutable construction graph is a
// separate linear capability transferred exactly once by TakeSourcePlane,
// unless the receipt has been sealed: a sealed receipt holds a shared
// read-only plane that every consumer may read.
type CanonicalGraphReceipt struct {
	owner  *canonicalGraphReceiptOwner
	root   uint32
	digest CanonicalDigest
	nodes  []canonicalGraphNode
	source *canonicalGraphSourcePlane
	// shared is the read-only construction plane of a sealed receipt. It is
	// set exactly when the linear capability has been retired by Seal.
	shared []Type
}

// canonicalGraphSourcePlane is a linear construction capability. Receipt
// copies share this owner, so exactly one consumer can detach the mutable typ
// graph and no receipt retains it afterward.
type canonicalGraphSourcePlane struct {
	mu    sync.Mutex
	nodes []Type
}

type canonicalGraphReceiptOwner struct{}

var canonicalGraphReceiptAuthority = &canonicalGraphReceiptOwner{}

// CanonicalGraphScope is the lexical scope used to interpret a source node.
// Token is stable across equivalent roots; it is not a pointer or a dense
// ordinal from the discovery pass. Formals is the number of free formal
// positions represented by the token (binder-local positions are included for
// a binder node's own child scope).
type CanonicalGraphScope struct {
	Token   CanonicalDigest
	Formals uint32
}

// CanonicalGraphBinding identifies a local formal's owner and ordinal inside
// the receipt. Owner is a source-plane ordinal, not a portable identity; the
// owner node's Identity is the stable cross-receipt identity.
type CanonicalGraphBinding struct {
	Owner   uint32
	Ordinal uint32
}

// CanonicalGraphNode is the defensive public view of one canonical source
// node. Children are direct authored children in their semantic slot order.
// A TypeParam's binder ownership is carried by Binding rather than duplicated
// as a synthetic child edge.
type CanonicalGraphNode struct {
	Identity CanonicalDigest
	Kind     kind.Kind
	Closed   bool
	Scope    CanonicalGraphScope
	Binding  CanonicalGraphBinding
	Bound    bool
	Children []uint32
}

type canonicalGraphNode struct {
	identity CanonicalDigest
	kind     kind.Kind
	closed   bool
	scope    CanonicalGraphScope
	binding  CanonicalGraphBinding
	bound    bool
	children []uint32
}

// EncodeCanonicalGraph admits one root (closed or open) and issues its
// canonical source plane in the same discovery/refinement pass used by the
// canonical codec. Free external formals are discovered by the same ownership
// fixed point as EncodeCanonicalOpen and installed before discovery. It never
// emits no per-child canonical wire image and never retains the temporary
// byte image. Synthetic Runtime projections (record keys, Optional
// projections, tuple wrappers, and instantiation expansions) intentionally do
// not belong here; their owner adds them after this authored graph is sealed.
func EncodeCanonicalGraph(ctx context.Context, root Type) (CanonicalGraphReceipt, error) {
	admission, err := newCanonicalFormalsAdmission(ctx, 0)
	if err != nil {
		return CanonicalGraphReceipt{}, err
	}
	formals, err := canonicalOpenFormals(root)
	if err != nil {
		return CanonicalGraphReceipt{}, err
	}
	encoder := canonicalEncoderPool.Get().(*canonicalEncoder)
	defer canonicalEncoderPool.Put(encoder)
	encoder.reset(ctx, true, admission)
	defer encoder.release()
	if err := encoder.installFormals(formals); err != nil {
		return CanonicalGraphReceipt{}, err
	}
	rootIndex, err := encoder.discover(root)
	if err != nil {
		return CanonicalGraphReceipt{}, err
	}
	if err := encoder.finalizeScopedTypeParams(); err != nil {
		return CanonicalGraphReceipt{}, err
	}
	if err := encoder.admitScopedForm(root); err != nil {
		return CanonicalGraphReceipt{}, err
	}
	if err := encoder.refine(); err != nil {
		return CanonicalGraphReceipt{}, err
	}
	return encoder.issueCanonicalGraph(rootIndex)
}

// canonicalOpenFormals is the one owner-discovery seam shared by graph and
// byte receipts. The walk's fixed-point rounds are required for productive
// recursive declarations; taking only the first approximation would either
// reject a valid open graph or mint a scope that depends on traversal timing.
func canonicalOpenFormals(root Type) ([]*TypeParam, error) {
	walk := formalOwnershipWalk{memo: make(map[uintptr]formalOwnershipEntry)}
	var formals []*TypeParam
	for {
		free, err := walk.freeFormals(root)
		if err != nil {
			return nil, err
		}
		formals = free
		if !walk.cyclic || !walk.changed {
			return formals, nil
		}
	}
}

// Valid reports whether the receipt was minted by the canonical graph owner.
func (receipt CanonicalGraphReceipt) Valid() bool {
	return receipt.owner == canonicalGraphReceiptAuthority &&
		len(receipt.nodes) != 0 &&
		int(receipt.root) < len(receipt.nodes) &&
		receipt.digest != (CanonicalDigest{})
}

// Digest returns the stable canonical identity of the admitted root.
func (receipt CanonicalGraphReceipt) Digest() (CanonicalDigest, bool) {
	if !receipt.Valid() {
		return CanonicalDigest{}, false
	}
	return receipt.digest, true
}

// Root returns the defensive view of the admitted root node.
func (receipt CanonicalGraphReceipt) Root() (CanonicalGraphNode, bool) {
	if !receipt.Valid() {
		return CanonicalGraphNode{}, false
	}
	return receipt.Node(receipt.root)
}

// RootOrdinal returns the source-plane ordinal of the admitted root.
func (receipt CanonicalGraphReceipt) RootOrdinal() (uint32, bool) {
	if !receipt.Valid() {
		return 0, false
	}
	return receipt.root, true
}

// Node returns one defensive source-plane node view.
func (receipt CanonicalGraphReceipt) Node(ordinal uint32) (CanonicalGraphNode, bool) {
	if !receipt.Valid() || uint64(ordinal) >= uint64(len(receipt.nodes)) {
		return CanonicalGraphNode{}, false
	}
	node := receipt.nodes[ordinal]
	return CanonicalGraphNode{
		Identity: node.identity,
		Kind:     node.kind,
		Closed:   node.closed,
		Scope:    node.scope,
		Binding:  node.binding,
		Bound:    node.bound,
		Children: append([]uint32(nil), node.children...),
	}, true
}

// Nodes returns all source-plane nodes in source ordinal order. Every child
// slice is copied, so callers cannot mutate receipt topology.
func (receipt CanonicalGraphReceipt) Nodes() []CanonicalGraphNode {
	if !receipt.Valid() {
		return nil
	}
	result := make([]CanonicalGraphNode, len(receipt.nodes))
	for index, node := range receipt.nodes {
		result[index] = CanonicalGraphNode{
			Identity: node.identity,
			Kind:     node.kind,
			Closed:   node.closed,
			Scope:    node.scope,
			Binding:  node.binding,
			Bound:    node.bound,
			Children: append([]uint32(nil), node.children...),
		}
	}
	return result
}

// TakeSourcePlane transfers ownership of the complete mutable construction
// graph exactly once. Copies of the receipt share the same linear capability.
// After a successful take, the receipt remains a valid immutable identity and
// topology receipt but retains no typ.Type pointer.
func (receipt CanonicalGraphReceipt) TakeSourcePlane() ([]Type, bool) {
	if !receipt.Valid() || receipt.source == nil {
		return nil, false
	}
	receipt.source.mu.Lock()
	defer receipt.source.mu.Unlock()
	if len(receipt.source.nodes) != len(receipt.nodes) {
		return nil, false
	}
	nodes := receipt.source.nodes
	receipt.source.nodes = nil
	return nodes, true
}

// Seal retires this receipt's linear source capability in favour of a shared
// read-only construction plane. The linear rule exists so two consumers cannot
// each detach and then own one mutable graph; a sealed receipt is never
// detached and never mutated, so it may seed any number of consumers at once.
// This is the same sharing the fixed primitive seed graphs already rely on.
func (receipt CanonicalGraphReceipt) Seal() (CanonicalGraphReceipt, bool) {
	if !receipt.Valid() {
		return CanonicalGraphReceipt{}, false
	}
	if receipt.Sealed() {
		return receipt, true
	}
	plane, taken := receipt.TakeSourcePlane()
	if !taken {
		return CanonicalGraphReceipt{}, false
	}
	receipt.source = nil
	receipt.shared = plane
	return receipt, true
}

// Sealed reports whether this receipt carries a shared read-only plane.
func (receipt CanonicalGraphReceipt) Sealed() bool {
	return receipt.Valid() && len(receipt.shared) == len(receipt.nodes)
}

// SourcePlane yields the construction graph for one consumer. A linear
// receipt transfers ownership exactly once; a sealed receipt lends its shared
// plane to every consumer and retains it. The returned nodes are read-only in
// both cases for a sealed receipt: its owner published them as immutable.
func (receipt CanonicalGraphReceipt) SourcePlane() ([]Type, bool) {
	if receipt.Sealed() {
		return receipt.shared, true
	}
	return receipt.TakeSourcePlane()
}

func (e *canonicalEncoder) issueCanonicalGraph(rootIndex int) (CanonicalGraphReceipt, error) {
	if e == nil || rootIndex < 0 || rootIndex >= len(e.nodes) || len(e.classes) != len(e.nodes) {
		return CanonicalGraphReceipt{}, errors.New("typ: malformed canonical graph quotient")
	}
	classCount := len(e.representatives)
	if classCount == 0 || classCount > math.MaxUint32 {
		return CanonicalGraphReceipt{}, errors.New("typ: canonical graph node count overflow")
	}
	if err := e.reserve(classCount, canonicalFormalsNodeBytes); err != nil {
		return CanonicalGraphReceipt{}, err
	}
	identities := make([]CanonicalDigest, classCount)
	for class := range classCount {
		if err := e.checkpoint(); err != nil {
			return CanonicalGraphReceipt{}, err
		}
		identity, err := e.digestCanonicalClass(class)
		if err != nil {
			return CanonicalGraphReceipt{}, err
		}
		identities[class] = identity
	}

	occurrenceTokens, err := e.graphOccurrenceTokens(rootIndex, identities)
	if err != nil {
		return CanonicalGraphReceipt{}, err
	}
	sourceCount := len(e.nodes)
	if sourceCount > math.MaxUint32 {
		return CanonicalGraphReceipt{}, errors.New("typ: canonical graph source count overflow")
	}
	nodes := make([]canonicalGraphNode, sourceCount)
	sources := make([]Type, sourceCount)
	for sourceIndex := range sourceCount {
		if err := e.checkpoint(); err != nil {
			return CanonicalGraphReceipt{}, err
		}
		class := e.classes[sourceIndex]
		if class < 0 || class >= classCount {
			return CanonicalGraphReceipt{}, errors.New("typ: canonical graph source class")
		}
		canonical := e.nodes[sourceIndex]
		children, err := e.graphSourceChildren(sourceIndex)
		if err != nil {
			return CanonicalGraphReceipt{}, err
		}
		closed, scope, binding, bound, err := e.graphSourceScope(sourceIndex, identities, occurrenceTokens)
		if err != nil {
			return CanonicalGraphReceipt{}, err
		}
		valueKind := kind.Nil
		if canonical.source != nil {
			valueKind = canonical.source.Kind()
		}
		nodes[sourceIndex] = canonicalGraphNode{
			identity: identities[class],
			kind:     valueKind,
			closed:   closed,
			scope:    scope,
			binding:  binding,
			bound:    bound,
			children: children,
		}
		sources[sourceIndex] = canonical.source
	}
	// The source plane intentionally preserves distinct lexical occurrences,
	// even when their semantic classes are bisimilar. Reused source pointers
	// remain one source node because discovery interns them by authored identity.
	return CanonicalGraphReceipt{
		owner:  canonicalGraphReceiptAuthority,
		root:   uint32(rootIndex),
		digest: identities[e.classes[rootIndex]],
		nodes:  nodes,
		source: &canonicalGraphSourcePlane{nodes: sources},
	}, nil
}

// digestCanonicalClass emits one class as a canonical root into reusable
// scratch and hashes it immediately. The byte image never leaves this method
// and is cleared by the encoder release path.
func (e *canonicalEncoder) digestCanonicalClass(class int) (CanonicalDigest, error) {
	if class < 0 || class >= len(e.representatives) {
		return CanonicalDigest{}, errors.New("typ: canonical graph class")
	}
	e.out = e.out[:0]
	e.emissionStack = e.emissionStack[:0]
	if e.ordinals == nil {
		e.ordinals = make(map[int]uint64, len(e.representatives))
	} else {
		for key := range e.ordinals {
			delete(e.ordinals, key)
		}
	}
	if err := e.appendCanonicalOutputFrameString(canonicalScopedTypeDomain); err != nil {
		return CanonicalDigest{}, err
	}
	if err := e.appendCanonicalOutputUvarint(canonicalScopedTypeVersion); err != nil {
		return CanonicalDigest{}, err
	}
	if err := e.emitClass(class, e.ordinals); err != nil {
		return CanonicalDigest{}, err
	}
	digest := CanonicalDigest(sha256.Sum256(e.out))
	e.out = e.out[:0]
	e.emissionStack = e.emissionStack[:0]
	return digest, nil
}

func (e *canonicalEncoder) graphSourceChildren(sourceIndex int) ([]uint32, error) {
	if sourceIndex < 0 || sourceIndex >= len(e.nodes) {
		return nil, errors.New("typ: canonical graph source node")
	}
	node := e.nodes[sourceIndex]
	start := 0
	if node.typeParam != nil {
		// finalizeScopedTypeParams prepends the lexical owner edge. It is
		// represented by Binding, not by the authored child plane.
		if _, external := e.formals[node.typeParam]; !external {
			if len(node.edges) == 0 {
				return nil, errors.New("typ: canonical formal owner edge")
			}
			start = 1
		}
	}
	if len(node.edges)-start > math.MaxUint32 {
		return nil, errors.New("typ: canonical graph edge count overflow")
	}
	children := make([]uint32, 0, len(node.edges)-start)
	for _, edge := range node.edges[start:] {
		if edge < 0 || edge >= len(e.nodes) {
			return nil, errors.New("typ: canonical graph child source")
		}
		children = append(children, uint32(edge))
	}
	return children, nil
}

type canonicalGraphFormal struct {
	owner    Type
	ordinal  uint64
	source   int
	external bool
}

// graphOccurrenceTokens assigns deterministic lexical-path tokens to source
// occurrences. The path contains canonical child identities and authored slot
// positions, never source pointers or discovery ordinals. A source pointer
// reused in the graph therefore gets one token, while two same-shaped binder
// occurrences reached through different lexical slots remain distinct.
func (e *canonicalEncoder) graphOccurrenceTokens(root int, identities []CanonicalDigest) ([]CanonicalDigest, error) {
	if root < 0 || root >= len(e.nodes) {
		return nil, errors.New("typ: canonical graph occurrence root")
	}
	paths := make([][]byte, len(e.nodes))
	rootClass := e.classes[root]
	if rootClass < 0 || rootClass >= len(identities) {
		return nil, errors.New("typ: canonical graph occurrence class")
	}
	paths[root] = append(paths[root], identities[rootClass][:]...)
	queue := []int{root}
	for head := 0; head < len(queue); head++ {
		if err := e.checkpoint(); err != nil {
			return nil, err
		}
		current := queue[head]
		node := e.nodes[current]
		start := 0
		if node.typeParam != nil {
			if _, external := e.formals[node.typeParam]; !external {
				if len(node.edges) == 0 {
					return nil, errors.New("typ: canonical occurrence formal owner edge")
				}
				start = 1
			}
		}
		for slot, edge := range node.edges[start:] {
			if edge < 0 || edge >= len(e.nodes) {
				return nil, errors.New("typ: canonical occurrence child")
			}
			childClass := e.classes[edge]
			if childClass < 0 || childClass >= len(identities) {
				return nil, errors.New("typ: canonical occurrence child class")
			}
			candidate := append([]byte(nil), paths[current]...)
			candidate = append(candidate, identities[childClass][:]...)
			var word [8]byte
			binary.BigEndian.PutUint64(word[:], uint64(slot))
			candidate = append(candidate, word[:]...)
			if paths[edge] != nil && !canonicalGraphPathLess(candidate, paths[edge]) {
				continue
			}
			paths[edge] = candidate
			queue = append(queue, edge)
		}
	}
	tokens := make([]CanonicalDigest, len(paths))
	for index, path := range paths {
		if path == nil {
			return nil, errors.New("typ: canonical graph occurrence unreachable")
		}
		hash := sha256.New()
		_, _ = hash.Write([]byte("wippy.analysis.type.typ/canonical-graph-occurrence\x00\x01"))
		_, _ = hash.Write(path)
		copy(tokens[index][:], hash.Sum(nil))
	}
	return tokens, nil
}

func canonicalGraphPathLess(left, right []byte) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return bytes.Compare(left, right) < 0
}

// graphSourceScope derives lexical scope from source occurrence ownership. A
// binder's own formals are bound at its root; a descendant TypeParam remains
// open and carries the binder/ordinal binding. Nested binders do not leak their
// local formals into an outer scope.
func (e *canonicalEncoder) graphSourceScope(sourceIndex int, identities, occurrenceTokens []CanonicalDigest) (bool, CanonicalGraphScope, CanonicalGraphBinding, bool, error) {
	if sourceIndex < 0 || sourceIndex >= len(e.nodes) {
		return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical source scope node")
	}
	if len(e.scopeMark) != len(e.nodes) {
		e.scopeMark = make([]uint32, len(e.nodes))
		e.scopeEpoch = 0
	}
	e.scopeEpoch++
	if e.scopeEpoch == 0 {
		clear(e.scopeMark)
		e.scopeEpoch = 1
	}
	epoch := e.scopeEpoch
	stack := append(e.scopeStack[:0], sourceIndex)
	binders := make(map[Type]struct{})
	formals := make([]canonicalGraphFormal, 0, 4)
	seenFormals := make(map[*TypeParam]struct{})
	for len(stack) != 0 {
		if err := e.checkpoint(); err != nil {
			return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current < 0 || current >= len(e.nodes) || e.scopeMark[current] == epoch {
			continue
		}
		e.scopeMark[current] = epoch
		node := e.nodes[current]
		if node.source == nil {
			// nil is a valid bottom-level source and has no lexical children.
			continue
		}
		switch value := node.source.(type) {
		case *Function:
			binders[value] = struct{}{}
		case *Generic:
			binders[value] = struct{}{}
		}
		start := 0
		if node.typeParam != nil {
			if externalOrdinal, external := e.formals[node.typeParam]; external {
				if _, duplicate := seenFormals[node.typeParam]; !duplicate {
					seenFormals[node.typeParam] = struct{}{}
					formals = append(formals, canonicalGraphFormal{ordinal: externalOrdinal, external: true})
				}
			} else if prior, exists := e.binders[node.typeParam]; exists {
				ownerIndex, reachable := e.seen[prior.owner]
				if !reachable || ownerIndex < 0 || ownerIndex >= len(e.nodes) {
					return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical formal owner unavailable")
				}
				if _, duplicate := seenFormals[node.typeParam]; !duplicate {
					seenFormals[node.typeParam] = struct{}{}
					formals = append(formals, canonicalGraphFormal{owner: prior.owner, ordinal: prior.ordinal, source: ownerIndex})
				}
				start = 1
			} else {
				return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical formal binding")
			}
		}
		for _, edge := range node.edges[start:] {
			if edge < 0 || edge >= len(e.nodes) {
				return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical scope child")
			}
			stack = append(stack, edge)
		}
	}
	e.scopeStack = stack[:0]

	free := make([]canonicalGraphFormal, 0, len(formals))
	for _, formal := range formals {
		if _, bound := binders[formal.owner]; !bound {
			free = append(free, formal)
		}
	}
	// Preserve first lexical occurrence, but remove duplicate owners/ordinals
	// reached through a recursive backedge. The source graph itself remains
	// untouched; this is only scope token material.
	if len(free) > 1 {
		seen := make(map[[2]uint64]struct{}, len(free))
		unique := free[:0]
		for _, formal := range free {
			key := [2]uint64{uint64(formal.source), formal.ordinal}
			if formal.external {
				key[0] = math.MaxUint64
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			unique = append(unique, formal)
		}
		free = unique
	}

	localCount := uint64(0)
	source := e.nodes[sourceIndex].source
	switch value := source.(type) {
	case *Function:
		localCount = uint64(len(value.TypeParams))
	case *Generic:
		localCount = uint64(len(value.TypeParams))
	}
	closed := len(free) == 0
	var scopeToken CanonicalDigest
	if len(free) != 0 || localCount != 0 {
		scopeToken = canonicalGraphScopeDigest(free, occurrenceTokens, source, sourceIndex, localCount)
	}
	formalsCount := uint64(len(free)) + localCount
	if formalsCount > math.MaxUint32 {
		return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical scope formal overflow")
	}
	scope := CanonicalGraphScope{Token: scopeToken, Formals: uint32(formalsCount)}
	binding := CanonicalGraphBinding{}
	bound := false
	if parameter := e.nodes[sourceIndex].typeParam; parameter != nil {
		if _, external := e.formals[parameter]; external {
			return closed, scope, binding, bound, nil
		}
		formal, exists := e.binders[parameter]
		if !exists {
			return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical formal binding")
		}
		ownerIndex, reachable := e.seen[formal.owner]
		if !reachable || ownerIndex < 0 || ownerIndex >= len(e.nodes) {
			return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical formal binding owner")
		}
		if ownerIndex > math.MaxUint32 || formal.ordinal > math.MaxUint32 {
			return false, CanonicalGraphScope{}, CanonicalGraphBinding{}, false, errors.New("typ: canonical formal binding ordinal")
		}
		binding = CanonicalGraphBinding{Owner: uint32(ownerIndex), Ordinal: uint32(formal.ordinal)}
		bound = true
	}
	return closed, scope, binding, bound, nil
}

func canonicalGraphScopeDigest(formals []canonicalGraphFormal, occurrenceTokens []CanonicalDigest, binder Type, binderSource int, localCount uint64) CanonicalDigest {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.type.typ/canonical-graph-scope\x00\x01"))
	writeCanonicalGraphWord(hash, uint64(len(formals)))
	for _, formal := range formals {
		if formal.external {
			_, _ = hash.Write([]byte{1})
			writeCanonicalGraphWord(hash, formal.ordinal)
			continue
		}
		_, _ = hash.Write([]byte{0})
		if formal.source >= 0 && formal.source < len(occurrenceTokens) {
			_, _ = hash.Write(occurrenceTokens[formal.source][:])
		}
		writeCanonicalGraphWord(hash, formal.ordinal)
	}
	if binder != nil {
		if binderSource >= 0 && binderSource < len(occurrenceTokens) {
			_, _ = hash.Write(occurrenceTokens[binderSource][:])
		}
		writeCanonicalGraphWord(hash, localCount)
	}
	var digest CanonicalDigest
	copy(digest[:], hash.Sum(nil))
	return digest
}

func writeCanonicalGraphWord(hash interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = hash.Write(word[:])
}
