package flow

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
)

// FunctionRef is a stable abstract identity for a function body. It deliberately
// mirrors canonical/ref.FuncRef without importing the checker packages into the
// flow domain.
type FunctionRef struct {
	GraphID    uint64
	ParentHash uint64
}

// FunctionRefSet is the may-set of function bodies a value path may denote.
//
// Bottom is the empty set. Top means "some function identity, unknown which".
// A singleton is precise enough for callers to resolve a summary with the
// current capture cells.
type FunctionRefSet struct {
	top  bool
	refs []FunctionRef
}

// FunctionRefSetOf constructs a canonical finite set.
func FunctionRefSetOf(refs ...FunctionRef) FunctionRefSet {
	return canonicalFunctionRefSet(refs)
}

// FunctionRefSetTop returns the unknown function identity set.
func FunctionRefSetTop() FunctionRefSet {
	return FunctionRefSet{top: true}
}

// IsTop reports whether s is the unknown function identity set.
func (s FunctionRefSet) IsTop() bool { return s.top }

// IsBottom reports whether s is the empty identity set.
func (s FunctionRefSet) IsBottom() bool { return !s.top && len(s.refs) == 0 }

// Singleton returns the sole possible function identity.
func (s FunctionRefSet) Singleton() (FunctionRef, bool) {
	if s.top || len(s.refs) != 1 {
		return FunctionRef{}, false
	}
	return s.refs[0], true
}

// Refs returns a copy of the finite refs. Top has no finite representation.
func (s FunctionRefSet) Refs() []FunctionRef {
	if s.top || len(s.refs) == 0 {
		return nil
	}
	return append([]FunctionRef(nil), s.refs...)
}

// Format renders s deterministically for tests and diagnostics.
func (s FunctionRefSet) Format() string {
	if s.top {
		return "⊤"
	}
	if len(s.refs) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(s.refs))
	for _, r := range s.refs {
		parts = append(parts, fmt.Sprintf("%d/%d", r.GraphID, r.ParentHash))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// FunctionRefSetDomain is the finite may-set lattice. Join is set union; Widen
// is Join because the per-program function universe is finite.
var FunctionRefSetDomain = lattice.Lattice[FunctionRefSet]{
	Bottom: func() FunctionRefSet { return FunctionRefSet{} },
	Top:    FunctionRefSetTop,
	Equal: func(a, b FunctionRefSet) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.refs) != len(b.refs) {
			return false
		}
		for i := range a.refs {
			if a.refs[i] != b.refs[i] {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b FunctionRefSet) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		i, j := 0, 0
		for i < len(a.refs) {
			if j >= len(b.refs) {
				return false
			}
			switch compareFunctionRef(a.refs[i], b.refs[j]) {
			case -1:
				return false
			case 0:
				i++
				j++
			default:
				j++
			}
		}
		return true
	},
	Join: func(a, b FunctionRefSet) FunctionRefSet {
		return joinFunctionRefSet(a, b)
	},
	Meet: nil,
	Widen: func(prev, next FunctionRefSet) FunctionRefSet {
		return joinFunctionRefSet(prev, next)
	},
}

// FunctionRefs maps runtime value paths to possible function identities.
type FunctionRefs = map[constraint.PathKey]FunctionRefSet

// ReferencePathProjection is a finite vocabulary for reference-axis facts.
// Exact paths keep only the path itself. Subtrees keep the path and every
// descendant. Call-entry and closure-capture reducers use it to retain only the
// function/closure identities a callee can observe, without falling back to
// whole-symbol projection.
type ReferencePathProjection struct {
	Exact        []constraint.Path
	Subtrees     []constraint.Path
	addresses    referenceAddressProjection
	captureCells captureCellPathProjection
	normalized   bool
}

// referenceAddressProjection is the consumption form of ReferencePathProjection.
// It is normalized once from source paths so fact carriers can test exact keys
// and subtrees without repeatedly lowering paths back into stable addresses.
type referenceAddressProjection struct {
	ExactKeys []constraint.PathKey
	Exact     []StableAddress
	Subtrees  []StableAddress
}

// NewReferencePathProjection builds a projection from source paths and records
// its stable-address view. Source paths remain available for compatibility, but
// flow domains consume the normalized address view.
func NewReferencePathProjection(exact, subtrees []constraint.Path) ReferencePathProjection {
	p := ReferencePathProjection{
		Exact:    cloneReferenceProjectionPaths(exact),
		Subtrees: cloneReferenceProjectionPaths(subtrees),
	}
	p.addresses = referenceAddressProjectionOfPaths(p.Exact, p.Subtrees)
	p.captureCells = captureCellProjectionOfAddresses(p.addresses)
	p.normalized = true
	return p
}

// IsEmpty reports whether projection has no addressable reference vocabulary.
func (p ReferencePathProjection) IsEmpty() bool {
	return p.addressProjection().isEmpty()
}

// addressProjection returns the stable-address consumption view. Projections
// built through NewReferencePathProjection already carry this view; literal test
// values are normalized on demand.
func (p ReferencePathProjection) addressProjection() referenceAddressProjection {
	if p.normalized {
		return p.addresses
	}
	return referenceAddressProjectionOfPaths(p.Exact, p.Subtrees)
}

func (p ReferencePathProjection) captureCellProjection() captureCellPathProjection {
	if p.normalized {
		return p.captureCells
	}
	return captureCellProjectionOfAddresses(p.addressProjection())
}

func (p referenceAddressProjection) isEmpty() bool {
	return len(p.ExactKeys) == 0 && len(p.Subtrees) == 0
}

func (p referenceAddressProjection) contains(path constraint.PathKey) bool {
	if path == "" {
		return false
	}
	for _, exact := range p.ExactKeys {
		if exact == path {
			return true
		}
	}
	for _, root := range p.Subtrees {
		if StableAddressKeyHasPrefix(path, root) {
			return true
		}
	}
	return false
}

func referenceAddressProjectionOfPaths(exact, subtrees []constraint.Path) referenceAddressProjection {
	var out referenceAddressProjection
	for _, path := range exact {
		addr, ok := StableAddressOfPath(path)
		if !ok {
			continue
		}
		out.Exact = append(out.Exact, addr)
		out.ExactKeys = append(out.ExactKeys, addr.Key())
	}
	for _, path := range subtrees {
		addr, ok := StableAddressOfPath(path)
		if !ok {
			continue
		}
		out.Subtrees = append(out.Subtrees, addr)
	}
	return out
}

func cloneReferenceProjectionPaths(paths []constraint.Path) []constraint.Path {
	if len(paths) == 0 {
		return nil
	}
	out := make([]constraint.Path, len(paths))
	for i, path := range paths {
		out[i] = cloneAddressPath(path)
	}
	return out
}

// FunctionRefsKey is an exact comparable key for a function-identity path map.
// It is interned by FunctionRefsDomain equality, not by formatting identity, so
// summary/cache keys stay stable while retaining exact lattice semantics.
type FunctionRefsKey struct {
	n *functionRefsKeyNode
}

type functionRefsKeyNode struct {
	refs FunctionRefs
	hash uint64
}

type functionRefsKeyInterner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*functionRefsKeyNode
}

var canonicalFunctionRefsKeys = &functionRefsKeyInterner{buckets: make(map[uint64][]*functionRefsKeyNode)}

// ResetCanonicalFunctionRefsKeys clears the comparable-key interner for one
// checker analysis scope.
func ResetCanonicalFunctionRefsKeys() {
	canonicalFunctionRefsKeys.mu.Lock()
	defer canonicalFunctionRefsKeys.mu.Unlock()
	canonicalFunctionRefsKeys.buckets = make(map[uint64][]*functionRefsKeyNode)
}

// FunctionRefsDomain is the pointwise finite-map lattice for closure identities.
var FunctionRefsDomain = latticeproduct.MapLattice[constraint.PathKey](FunctionRefSetDomain)

// FunctionRefAtAddress returns the identity set for addr.
func FunctionRefAtAddress(refs FunctionRefs, addr StableAddress) (FunctionRefSet, bool) {
	if len(refs) == 0 || addr.Key() == "" {
		return FunctionRefSet{}, false
	}
	set, ok := refs[addr.Key()]
	if ok && !set.IsBottom() {
		return set, true
	}
	return FunctionRefSet{}, false
}

// FunctionRefAtPath returns the identity set for a structured path.
func FunctionRefAtPath(refs FunctionRefs, path constraint.Path) (FunctionRefSet, bool) {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return FunctionRefSet{}, false
	}
	return FunctionRefAtAddress(refs, addr)
}

// WithFunctionRefAddress returns refs with addr strongly updated to set.
// Updating to Bottom removes the key.
func WithFunctionRefAddress(refs FunctionRefs, addr StableAddress, set FunctionRefSet) FunctionRefs {
	path := addr.Key()
	if path == "" {
		return refs
	}
	out := make(FunctionRefs, len(refs)+1)
	for k, v := range refs {
		out[k] = v
	}
	if set.IsBottom() {
		delete(out, path)
		return FunctionRefsDomain.Join(out, nil)
	}
	out[path] = set
	return FunctionRefsDomain.Join(out, nil)
}

// WithFunctionRefPath returns refs with a structured path strongly updated to
// set. Updating to Bottom removes the key.
func WithFunctionRefPath(refs FunctionRefs, path constraint.Path, set FunctionRefSet) FunctionRefs {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return refs
	}
	return WithFunctionRefAddress(refs, addr, set)
}

// FunctionRefsKeyOf returns an exact comparable key for refs.
func FunctionRefsKeyOf(refs FunctionRefs) FunctionRefsKey {
	return internFunctionRefsKey(refs)
}

// Refs returns the immutable refs map represented by k. The zero key denotes
// FunctionRefs bottom.
func (k FunctionRefsKey) Refs() FunctionRefs {
	if k.n == nil {
		return FunctionRefsDomain.Bottom()
	}
	return FunctionRefsDomain.Join(k.n.refs, nil)
}

// ProjectFunctionRefsBySymbols keeps only paths rooted at one of symbols.
func ProjectFunctionRefsBySymbols(refs FunctionRefs, symbols []cfg.SymbolID) FunctionRefs {
	if len(refs) == 0 || len(symbols) == 0 {
		return FunctionRefsDomain.Bottom()
	}
	out := make(FunctionRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() {
			continue
		}
		for _, sym := range symbols {
			if functionRefPathBelongsToSymbol(path, sym) {
				out[path] = set
				break
			}
		}
	}
	return FunctionRefsDomain.Join(out, nil)
}

// FunctionRefRootSymbols returns the finite symbol roots that carry function
// identity facts.
func FunctionRefRootSymbols(refs FunctionRefs) []cfg.SymbolID {
	if len(refs) == 0 || FunctionRefsDomain.Equal(refs, FunctionRefsDomain.Top()) {
		return nil
	}
	var out []cfg.SymbolID
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() {
			continue
		}
		out = appendReferencePathRootSymbol(out, path)
	}
	return compactSortedSymbols(out)
}

// ProjectFunctionRefsByAddress keeps only addr and its descendants.
func ProjectFunctionRefsByAddress(refs FunctionRefs, addr StableAddress) FunctionRefs {
	if len(refs) == 0 || addr.Key() == "" {
		return FunctionRefsDomain.Bottom()
	}
	out := make(FunctionRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() {
			continue
		}
		if functionRefPathInAddressSubtree(path, addr) {
			out[path] = set
		}
	}
	return FunctionRefsDomain.Join(out, nil)
}

// ProjectFunctionRefsByPath keeps only path and its descendants.
func ProjectFunctionRefsByPath(refs FunctionRefs, path constraint.Path) FunctionRefs {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return FunctionRefsDomain.Bottom()
	}
	return ProjectFunctionRefsByAddress(refs, addr)
}

// ProjectFunctionRefsByReferencePaths keeps exactly the finite path vocabulary
// in projection. It is more precise than ProjectFunctionRefsBySymbols: captured
// table roots no longer pull every function-valued field into a callee context
// unless that table escapes as a whole subtree.
func ProjectFunctionRefsByReferencePaths(refs FunctionRefs, projection ReferencePathProjection) FunctionRefs {
	addresses := projection.addressProjection()
	if addresses.isEmpty() {
		return FunctionRefsDomain.Bottom()
	}
	if FunctionRefsDomain.Equal(refs, FunctionRefsDomain.Top()) {
		return FunctionRefsDomain.Top()
	}
	if len(refs) == 0 {
		return FunctionRefsDomain.Bottom()
	}
	out := make(FunctionRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() || !addresses.contains(path) {
			continue
		}
		out[path] = set
	}
	return FunctionRefsDomain.Join(out, nil)
}

// RebaseFunctionRefs moves all identity facts under from to the corresponding
// subtree under to. It is used for summary return slots: a callee summary records
// identities under placeholder paths ($0.f); the caller rebases that subtree onto
// the concrete assignment target (x.f).
func RebaseFunctionRefsAddress(refs FunctionRefs, from, to StableAddress) FunctionRefs {
	if FunctionRefsDomain.Equal(refs, FunctionRefsDomain.Top()) {
		return WithFunctionRefAddress(nil, to, FunctionRefSetTop())
	}
	if len(refs) == 0 || from.Key() == "" || to.Key() == "" {
		return FunctionRefsDomain.Bottom()
	}
	out := make(FunctionRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		pathAddr, ok := StableAddressFromCanonicalKey(path)
		if set.IsBottom() || !ok {
			continue
		}
		remainder, ok := pathAddr.RemainderAfterPrefix(from)
		if !ok {
			continue
		}
		target, ok := to.Append(remainder)
		if !ok {
			continue
		}
		out[target.Key()] = set
	}
	return FunctionRefsDomain.Join(out, nil)
}

// RebaseFunctionRefsPath is the path-shaped form used by producers that have
// structured paths but should not own stable-address normalization.
func RebaseFunctionRefsPath(refs FunctionRefs, from, to constraint.Path) FunctionRefs {
	fromAddr, fromOK := StableAddressOfPath(from)
	toAddr, toOK := StableAddressOfPath(to)
	if !fromOK || !toOK {
		return FunctionRefsDomain.Bottom()
	}
	return RebaseFunctionRefsAddress(refs, fromAddr, toAddr)
}

func functionRefPathBelongsToSymbol(path constraint.PathKey, sym cfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	root, ok := StableAddressOfSymbol(sym, nil)
	return ok && functionRefPathInAddressSubtree(path, root)
}

func appendReferencePathRootSymbol(symbols []cfg.SymbolID, path constraint.PathKey) []cfg.SymbolID {
	addr, ok := StableAddressFromCanonicalKey(path)
	if !ok {
		return symbols
	}
	sym, ok := addr.Symbol()
	if ok && sym != 0 {
		symbols = append(symbols, sym)
	}
	return symbols
}

func compactSortedSymbols(symbols []cfg.SymbolID) []cfg.SymbolID {
	if len(symbols) == 0 {
		return nil
	}
	slices.Sort(symbols)
	return slices.Compact(symbols)
}

// WithoutFunctionRefSubtreeAddress returns refs with addr and every descendant
// path removed. A strong write to a container path invalidates all function
// identities beneath that runtime value.
func WithoutFunctionRefSubtreeAddress(refs FunctionRefs, addr StableAddress) FunctionRefs {
	if len(refs) == 0 || addr.Key() == "" {
		return refs
	}
	found := false
	for k := range refs {
		if functionRefPathInAddressSubtree(k, addr) {
			found = true
			break
		}
	}
	if !found {
		return refs
	}
	out := make(FunctionRefs, len(refs))
	for k, v := range refs {
		if !functionRefPathInAddressSubtree(k, addr) {
			out[k] = v
		}
	}
	return FunctionRefsDomain.Join(out, nil)
}

func functionRefPathInAddressSubtree(path constraint.PathKey, root StableAddress) bool {
	return StableAddressKeyHasPrefix(path, root)
}

func referenceProjectionContainsPath(projection ReferencePathProjection, path constraint.PathKey) bool {
	return projection.addressProjection().contains(path)
}

func internFunctionRefsKey(refs FunctionRefs) FunctionRefsKey {
	canonical := FunctionRefsDomain.Join(refs, nil)
	if len(canonical) == 0 {
		return FunctionRefsKey{}
	}
	h := functionRefsKeyHash(canonical)

	canonicalFunctionRefsKeys.mu.RLock()
	if existing, ok := lookupFunctionRefsKey(canonicalFunctionRefsKeys.buckets[h], canonical); ok {
		canonicalFunctionRefsKeys.mu.RUnlock()
		return FunctionRefsKey{n: existing}
	}
	canonicalFunctionRefsKeys.mu.RUnlock()

	canonicalFunctionRefsKeys.mu.Lock()
	defer canonicalFunctionRefsKeys.mu.Unlock()
	if existing, ok := lookupFunctionRefsKey(canonicalFunctionRefsKeys.buckets[h], canonical); ok {
		return FunctionRefsKey{n: existing}
	}
	node := &functionRefsKeyNode{refs: canonical, hash: h}
	canonicalFunctionRefsKeys.buckets[h] = append(canonicalFunctionRefsKeys.buckets[h], node)
	return FunctionRefsKey{n: node}
}

func lookupFunctionRefsKey(bucket []*functionRefsKeyNode, refs FunctionRefs) (*functionRefsKeyNode, bool) {
	for _, node := range bucket {
		if FunctionRefsDomain.Equal(node.refs, refs) {
			return node, true
		}
	}
	return nil, false
}

func functionRefsKeyHash(refs FunctionRefs) uint64 {
	h := internal.FnvString("flow.FunctionRefsKey")
	for _, path := range constraint.SortedPathKeys(refs) {
		h = internal.HashCombine(h, internal.FnvString(string(path)))
		h = internal.HashCombine(h, functionRefSetHash(refs[path]))
	}
	return h
}

func functionRefSetHash(set FunctionRefSet) uint64 {
	h := internal.FnvString("flow.FunctionRefSet")
	if set.top {
		return internal.HashCombine(h, 1)
	}
	h = internal.HashCombine(h, 0)
	for _, ref := range set.refs {
		h = internal.HashCombine(h, ref.GraphID)
		h = internal.HashCombine(h, ref.ParentHash)
	}
	return h
}

func canonicalFunctionRefSet(refs []FunctionRef) FunctionRefSet {
	if len(refs) == 0 {
		return FunctionRefSet{}
	}
	out := append([]FunctionRef(nil), refs...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && compareFunctionRef(out[j], out[j-1]) < 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, r := range out {
		if r.GraphID == 0 {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1] == r {
			continue
		}
		dst = append(dst, r)
	}
	return FunctionRefSet{refs: append([]FunctionRef(nil), dst...)}
}

func joinFunctionRefSet(a, b FunctionRefSet) FunctionRefSet {
	if a.top || b.top {
		return FunctionRefSetTop()
	}
	if len(a.refs) == 0 {
		return b
	}
	if len(b.refs) == 0 {
		return a
	}

	out := make([]FunctionRef, 0, len(a.refs)+len(b.refs))
	i, j := 0, 0
	for i < len(a.refs) && j < len(b.refs) {
		switch compareFunctionRef(a.refs[i], b.refs[j]) {
		case -1:
			out = append(out, a.refs[i])
			i++
		case 0:
			out = append(out, a.refs[i])
			i++
			j++
		default:
			out = append(out, b.refs[j])
			j++
		}
	}
	out = append(out, a.refs[i:]...)
	out = append(out, b.refs[j:]...)
	if sameFunctionRefSlice(out, a.refs) {
		return a
	}
	if sameFunctionRefSlice(out, b.refs) {
		return b
	}
	return FunctionRefSet{refs: out}
}

func compareFunctionRef(a, b FunctionRef) int {
	if c := cmp.Compare(a.GraphID, b.GraphID); c != 0 {
		return c
	}
	return cmp.Compare(a.ParentHash, b.ParentHash)
}

func sameFunctionRefSlice(a, b []FunctionRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
