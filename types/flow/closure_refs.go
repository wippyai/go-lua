package flow

import (
	"cmp"
	"fmt"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
)

// ClosureRef is a function identity together with the lexical environment that
// must enter the function when this closure value is called.
//
// FunctionRefs intentionally remains a coarse identity axis. ClosureRefs is the
// context-sensitive closure-value axis: a returned or stored nested function can
// carry the captured cell store and captured function identities from its
// creation point without asking the driver to rediscover them later.
type ClosureRef struct {
	Ref       FunctionRef
	Cells     CaptureCellsKey
	EntryRefs FunctionRefsKey
	Closures  ClosureRefsKey
}

const closureContextDepth = 2

// ClosureRefOf constructs a closure identity from concrete entry components.
func ClosureRefOf(ref FunctionRef, cells CaptureCells, entryRefs FunctionRefs, closures ...ClosureRefs) ClosureRef {
	var closureRefs ClosureRefs
	if len(closures) > 0 {
		closureRefs = closures[0]
	}
	return ClosureRef{
		Ref:       ref,
		Cells:     cells.Key(),
		EntryRefs: FunctionRefsKeyOf(entryRefs),
		Closures:  closureRefsKeyOfDepth(closureRefs, closureContextDepth),
	}
}

// EntryCells returns the captured-cell store carried by r.
func (r ClosureRef) EntryCells() CaptureCells { return r.Cells.Cells() }

// EntryFunctionRefs returns the captured function-ref store carried by r.
func (r ClosureRef) EntryFunctionRefs() FunctionRefs { return r.EntryRefs.Refs() }

// EntryClosureRefs returns the captured closure-ref store carried by r.
func (r ClosureRef) EntryClosureRefs() ClosureRefs { return r.Closures.Refs() }

// ClosureRefSet is the finite may-set of closure values a runtime path may
// denote. Bottom is the empty set. Top means "some closure, unknown which
// environment".
type ClosureRefSet struct {
	top  bool
	refs []ClosureRef
}

// ClosureRefSetOf constructs a canonical finite set.
func ClosureRefSetOf(refs ...ClosureRef) ClosureRefSet {
	return canonicalClosureRefSet(refs)
}

// ClosureRefSetTop returns the unknown closure set.
func ClosureRefSetTop() ClosureRefSet { return ClosureRefSet{top: true} }

// IsTop reports whether s is the unknown closure set.
func (s ClosureRefSet) IsTop() bool { return s.top }

// IsBottom reports whether s is the empty closure set.
func (s ClosureRefSet) IsBottom() bool { return !s.top && len(s.refs) == 0 }

// Refs returns a copy of the finite closure refs. Top has no finite
// representation.
func (s ClosureRefSet) Refs() []ClosureRef {
	if s.top || len(s.refs) == 0 {
		return nil
	}
	return append([]ClosureRef(nil), s.refs...)
}

// Singleton returns the sole possible closure identity.
func (s ClosureRefSet) Singleton() (ClosureRef, bool) {
	if s.top || len(s.refs) != 1 {
		return ClosureRef{}, false
	}
	return s.refs[0], true
}

// Format renders s deterministically for tests and diagnostics.
func (s ClosureRefSet) Format() string {
	if s.top {
		return "⊤"
	}
	if len(s.refs) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(s.refs))
	for _, r := range s.refs {
		parts = append(parts, fmt.Sprintf("%d/%d cells=%s refs=%s closures=%s",
			r.Ref.GraphID, r.Ref.ParentHash, r.Cells.Format(), formatFunctionRefs(r.EntryFunctionRefs()), formatClosureRefs(r.EntryClosureRefs())))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// ClosureRefSetDomain is the closure-value may-set lattice.
//
// The canonical carrier has at most one abstract environment per function
// identity. When the same function body reaches a path with different captured
// environments, canonicalization joins the environment product
// (CaptureCells × FunctionRefs × ClosureRefs) instead of storing two separate
// same-function alternatives. This is the k-CFA-style context bound that keeps
// closure values finite without making Join depend on grouping.
var ClosureRefSetDomain = lattice.Lattice[ClosureRefSet]{
	Bottom: func() ClosureRefSet { return ClosureRefSet{} },
	Top:    ClosureRefSetTop,
	Equal: func(a, b ClosureRefSet) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.refs) != len(b.refs) {
			return false
		}
		for i := range a.refs {
			if !closureRefEqual(a.refs[i], b.refs[i]) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b ClosureRefSet) bool {
		return closureRefSetLessOrEq(a, b)
	},
	Join: func(a, b ClosureRefSet) ClosureRefSet {
		return joinClosureRefSet(a, b)
	},
	Meet: nil,
	Widen: func(prev, next ClosureRefSet) ClosureRefSet {
		return joinClosureRefSet(prev, next)
	},
}

// ClosureRefs maps runtime value paths to possible closure identities.
type ClosureRefs = map[constraint.PathKey]ClosureRefSet

// ClosureRefsKey is an exact comparable key for a closure-ref path map.
type ClosureRefsKey struct {
	n *closureRefsKeyNode
}

type closureRefsKeyNode struct {
	refs ClosureRefs
	hash uint64
}

type closureRefsKeyInterner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*closureRefsKeyNode
}

var canonicalClosureRefsKeys = &closureRefsKeyInterner{buckets: make(map[uint64][]*closureRefsKeyNode)}

// ResetCanonicalClosureRefsKeys clears the comparable-key interner for one
// checker analysis scope.
func ResetCanonicalClosureRefsKeys() {
	canonicalClosureRefsKeys.mu.Lock()
	defer canonicalClosureRefsKeys.mu.Unlock()
	canonicalClosureRefsKeys.buckets = make(map[uint64][]*closureRefsKeyNode)
}

// ClosureRefsDomain is the pointwise finite-map lattice for closure values.
var ClosureRefsDomain = latticeproduct.MapLattice[constraint.PathKey](ClosureRefSetDomain)

// ClosureRefsKeyOf returns an exact comparable key for refs.
func ClosureRefsKeyOf(refs ClosureRefs) ClosureRefsKey {
	return closureRefsKeyOfDepth(refs, closureContextDepth)
}

// Refs returns the immutable closure-ref map represented by k. The zero key
// denotes ClosureRefs bottom.
func (k ClosureRefsKey) Refs() ClosureRefs {
	if k.n == nil {
		return nil
	}
	return cloneClosureRefsFinite(k.n.refs)
}

// Format renders k deterministically.
func (k ClosureRefsKey) Format() string {
	return formatClosureRefs(k.Refs())
}

// ClosureRefAt returns the closure set for path.
func ClosureRefAt(refs ClosureRefs, path constraint.PathKey) (ClosureRefSet, bool) {
	if path == "" {
		return ClosureRefSet{}, false
	}
	if isClosureRefsTop(refs) {
		return ClosureRefSetTop(), true
	}
	if len(refs) == 0 {
		return ClosureRefSet{}, false
	}
	set, ok := refs[path]
	return set, ok && !set.IsBottom()
}

// WithClosureRef returns refs with path strongly updated to set. Updating to
// Bottom removes the key.
func WithClosureRef(refs ClosureRefs, path constraint.PathKey, set ClosureRefSet) ClosureRefs {
	if path == "" {
		return refs
	}
	if isClosureRefsTop(refs) {
		return refs
	}
	out := make(ClosureRefs, len(refs)+1)
	for k, v := range refs {
		out[k] = v
	}
	if set.IsBottom() {
		delete(out, path)
		return ClosureRefsDomain.Join(out, nil)
	}
	out[path] = set
	return ClosureRefsDomain.Join(out, nil)
}

// ProjectClosureRefsBySymbols keeps only paths rooted at one of symbols.
func ProjectClosureRefsBySymbols(refs ClosureRefs, symbols []cfg.SymbolID) ClosureRefs {
	if isClosureRefsTop(refs) {
		return ClosureRefsDomain.Top()
	}
	if len(refs) == 0 || len(symbols) == 0 {
		return ClosureRefsDomain.Bottom()
	}
	out := make(ClosureRefs)
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
	return ClosureRefsDomain.Join(out, nil)
}

// ProjectClosureRefsByPath keeps only path and its descendants.
func ProjectClosureRefsByPath(refs ClosureRefs, root constraint.Path) ClosureRefs {
	rootKey := root.Key()
	if isClosureRefsTop(refs) {
		return ClosureRefsDomain.Top()
	}
	if len(refs) == 0 || rootKey == "" {
		return ClosureRefsDomain.Bottom()
	}
	out := make(ClosureRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() {
			continue
		}
		if functionRefPathInSubtree(path, rootKey) {
			out[path] = set
		}
	}
	return ClosureRefsDomain.Join(out, nil)
}

// RebaseClosureRefs moves all closure facts under from to the corresponding
// subtree under to.
func RebaseClosureRefs(refs ClosureRefs, from, to constraint.Path) ClosureRefs {
	fromKey := from.Key()
	toKey := to.Key()
	if isClosureRefsTop(refs) {
		return ClosureRefsDomain.Top()
	}
	if len(refs) == 0 || fromKey == "" || toKey == "" {
		return ClosureRefsDomain.Bottom()
	}
	out := make(ClosureRefs)
	fromPrefix := string(fromKey)
	toPrefix := string(toKey)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsBottom() || !functionRefPathInSubtree(path, fromKey) {
			continue
		}
		suffix := strings.TrimPrefix(string(path), fromPrefix)
		out[constraint.PathKey(toPrefix+suffix)] = set
	}
	return ClosureRefsDomain.Join(out, nil)
}

// WithoutClosureRefSubtree returns refs with path and every descendant path
// removed.
func WithoutClosureRefSubtree(refs ClosureRefs, path constraint.PathKey) ClosureRefs {
	if isClosureRefsTop(refs) {
		return refs
	}
	if len(refs) == 0 || path == "" {
		return refs
	}
	out := make(ClosureRefs, len(refs))
	for k, v := range refs {
		if functionRefPathInSubtree(k, path) {
			continue
		}
		out[k] = v
	}
	return ClosureRefsDomain.Join(out, nil)
}

// ApplyClosureRefCellEffects applies cell effects to the closure environment
// stored at path. This is the closure-call counterpart of CaptureEffects.Apply:
// the callee mutates its captured lexical store, so the closure value's carried
// environment changes instead of blindly writing the caller's current cell store.
func ApplyClosureRefCellEffects(refs ClosureRefs, path constraint.PathKey, effects CaptureEffects) ClosureRefs {
	if isClosureRefsTop(refs) {
		return refs
	}
	if path == "" || CaptureEffectsDomain.Equal(effects, CaptureEffectsDomain.Bottom()) {
		return refs
	}
	set, ok := ClosureRefAt(refs, path)
	if !ok {
		return refs
	}
	if set.IsTop() {
		return WithClosureRef(refs, path, set)
	}
	updated := make([]ClosureRef, 0, len(set.refs))
	for _, ref := range set.refs {
		ref.Cells = effects.Apply(ref.EntryCells()).Key()
		updated = append(updated, ref)
	}
	return WithClosureRef(refs, path, ClosureRefSetOf(updated...))
}

func canonicalClosureRefSet(refs []ClosureRef) ClosureRefSet {
	if len(refs) == 0 {
		return ClosureRefSet{}
	}
	out := append([]ClosureRef(nil), refs...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && compareClosureRef(out[j], out[j-1]) < 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, r := range out {
		if r.Ref.GraphID == 0 {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Ref == r.Ref {
			if closureRefEqual(dst[len(dst)-1], r) {
				continue
			}
			dst[len(dst)-1] = joinClosureRefEnvironment(dst[len(dst)-1], r)
			continue
		}
		dst = append(dst, r)
	}
	return ClosureRefSet{refs: append([]ClosureRef(nil), dst...)}
}

func joinClosureRefSet(a, b ClosureRefSet) ClosureRefSet {
	if a.top || b.top {
		return ClosureRefSetTop()
	}
	out := append(a.Refs(), b.refs...)
	return canonicalClosureRefSet(out)
}

func closureRefSetLessOrEq(a, b ClosureRefSet) bool {
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
		c := compareFunctionRef(a.refs[i].Ref, b.refs[j].Ref)
		switch {
		case c < 0:
			return false
		case c == 0:
			if !closureRefLessOrEq(a.refs[i], b.refs[j]) {
				return false
			}
			i++
			j++
		default:
			j++
		}
	}
	return true
}

func joinClosureRefEnvironment(a, b ClosureRef) ClosureRef {
	if a.Ref != b.Ref {
		return a
	}
	return ClosureRef{
		Ref:       a.Ref,
		Cells:     CaptureCellsDomain.Join(a.EntryCells(), b.EntryCells()).Key(),
		EntryRefs: FunctionRefsKeyOf(FunctionRefsDomain.Join(a.EntryFunctionRefs(), b.EntryFunctionRefs())),
		Closures:  closureRefsKeyOfDepth(joinClosureRefsFinite(a.EntryClosureRefs(), b.EntryClosureRefs()), closureContextDepth),
	}
}

func closureRefLessOrEq(a, b ClosureRef) bool {
	if a == b {
		return true
	}
	return a.Ref == b.Ref &&
		CaptureCellsDomain.LessOrEq(a.EntryCells(), b.EntryCells()) &&
		FunctionRefsDomain.LessOrEq(a.EntryFunctionRefs(), b.EntryFunctionRefs()) &&
		closureRefsLessOrEq(a.EntryClosureRefs(), b.EntryClosureRefs())
}

func closureRefEqual(a, b ClosureRef) bool {
	// The environment components are interned exact comparable keys. Equal
	// captured cells/function-ref maps/closure-ref maps canonicalize to the same
	// key handles within an analysis scope, so reopening the maps here only
	// redoes recursive equality work that the key layer already owns.
	return a == b
}

func compareClosureRef(a, b ClosureRef) int {
	if a == b {
		return 0
	}
	if c := compareFunctionRef(a.Ref, b.Ref); c != 0 {
		return c
	}
	if c := compareCaptureCellsKey(a.Cells, b.Cells); c != 0 {
		return c
	}
	if c := compareFunctionRefsKey(a.EntryRefs, b.EntryRefs); c != 0 {
		return c
	}
	return compareClosureRefsKey(a.Closures, b.Closures)
}

func compareCaptureCellsKey(a, b CaptureCellsKey) int {
	if a == b {
		return 0
	}
	ah, bh := uint64(0), uint64(0)
	if a.n != nil {
		ah = a.n.hash
	}
	if b.n != nil {
		bh = b.n.hash
	}
	if c := cmp.Compare(ah, bh); c != 0 {
		return c
	}
	return cmp.Compare(a.Format(), b.Format())
}

func compareFunctionRefsKey(a, b FunctionRefsKey) int {
	if a == b {
		return 0
	}
	ah, bh := uint64(0), uint64(0)
	if a.n != nil {
		ah = a.n.hash
	}
	if b.n != nil {
		bh = b.n.hash
	}
	if c := cmp.Compare(ah, bh); c != 0 {
		return c
	}
	return cmp.Compare(formatFunctionRefs(a.Refs()), formatFunctionRefs(b.Refs()))
}

func compareClosureRefsKey(a, b ClosureRefsKey) int {
	if a == b {
		return 0
	}
	ah, bh := uint64(0), uint64(0)
	if a.n != nil {
		ah = a.n.hash
	}
	if b.n != nil {
		bh = b.n.hash
	}
	if c := cmp.Compare(ah, bh); c != 0 {
		return c
	}
	return cmp.Compare(formatClosureRefs(a.Refs()), formatClosureRefs(b.Refs()))
}

func closureRefsKeyOfDepth(refs ClosureRefs, depth int) ClosureRefsKey {
	return internClosureRefsKey(limitClosureRefsDepth(refs, depth))
}

func internClosureRefsKey(refs ClosureRefs) ClosureRefsKey {
	canonical := canonicalClosureRefsFinite(refs)
	if len(canonical) == 0 {
		return ClosureRefsKey{}
	}
	h := closureRefsKeyHash(canonical)

	canonicalClosureRefsKeys.mu.RLock()
	if existing, ok := lookupClosureRefsKey(canonicalClosureRefsKeys.buckets[h], canonical); ok {
		canonicalClosureRefsKeys.mu.RUnlock()
		return ClosureRefsKey{n: existing}
	}
	canonicalClosureRefsKeys.mu.RUnlock()

	canonicalClosureRefsKeys.mu.Lock()
	defer canonicalClosureRefsKeys.mu.Unlock()
	if existing, ok := lookupClosureRefsKey(canonicalClosureRefsKeys.buckets[h], canonical); ok {
		return ClosureRefsKey{n: existing}
	}
	node := &closureRefsKeyNode{refs: canonical, hash: h}
	canonicalClosureRefsKeys.buckets[h] = append(canonicalClosureRefsKeys.buckets[h], node)
	return ClosureRefsKey{n: node}
}

func lookupClosureRefsKey(bucket []*closureRefsKeyNode, refs ClosureRefs) (*closureRefsKeyNode, bool) {
	for _, node := range bucket {
		if closureRefsEqual(node.refs, refs) {
			return node, true
		}
	}
	return nil, false
}

func closureRefsKeyHash(refs ClosureRefs) uint64 {
	h := internal.FnvString("flow.ClosureRefsKey")
	for _, path := range constraint.SortedPathKeys(refs) {
		h = internal.HashCombine(h, internal.FnvString(string(path)))
		h = internal.HashCombine(h, closureRefSetHash(refs[path]))
	}
	return h
}

func closureRefSetHash(set ClosureRefSet) uint64 {
	h := internal.FnvString("flow.ClosureRefSet")
	if set.top {
		return internal.HashCombine(h, 1)
	}
	h = internal.HashCombine(h, 0)
	for _, ref := range set.refs {
		h = internal.HashCombine(h, ref.Ref.GraphID)
		h = internal.HashCombine(h, ref.Ref.ParentHash)
		if ref.Cells.n != nil {
			h = internal.HashCombine(h, ref.Cells.n.hash)
		}
		if ref.EntryRefs.n != nil {
			h = internal.HashCombine(h, ref.EntryRefs.n.hash)
		}
		if ref.Closures.n != nil {
			h = internal.HashCombine(h, ref.Closures.n.hash)
		}
	}
	return h
}

func formatFunctionRefs(refs FunctionRefs) string {
	if len(refs) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(refs))
	for _, path := range constraint.SortedPathKeys(refs) {
		parts = append(parts, fmt.Sprintf("%s:%s", path, refs[path].Format()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatClosureRefs(refs ClosureRefs) string {
	if len(refs) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(refs))
	for _, path := range constraint.SortedPathKeys(refs) {
		parts = append(parts, fmt.Sprintf("%s:%s", path, refs[path].Format()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func cloneClosureRefs(refs ClosureRefs) ClosureRefs {
	if isClosureRefsTop(refs) {
		return ClosureRefsDomain.Top()
	}
	return cloneClosureRefsFinite(refs)
}

func cloneClosureRefsFinite(refs ClosureRefs) ClosureRefs {
	if len(refs) == 0 {
		return nil
	}
	var out ClosureRefs
	for k, v := range refs {
		if v.IsBottom() {
			continue
		}
		if out == nil {
			out = make(ClosureRefs, len(refs))
		}
		out[k] = v
	}
	return out
}

func closureRefsEqual(a, b ClosureRefs) bool {
	if len(a) != len(b) {
		return false
	}
	for path := range a {
		bs, ok := b[path]
		if !ok || !closureRefSetEqual(a[path], bs) {
			return false
		}
	}
	return true
}

func closureRefsLessOrEq(a, b ClosureRefs) bool {
	keys := make(map[constraint.PathKey]struct{}, len(a)+len(b))
	for path := range a {
		keys[path] = struct{}{}
	}
	for path := range b {
		keys[path] = struct{}{}
	}
	for _, path := range constraint.SortedPathKeys(keys) {
		if !closureRefSetLessOrEq(a[path], b[path]) {
			return false
		}
	}
	return true
}

func joinClosureRefsFinite(a, b ClosureRefs) ClosureRefs {
	keys := make(map[constraint.PathKey]struct{}, len(a)+len(b))
	for path := range a {
		keys[path] = struct{}{}
	}
	for path := range b {
		keys[path] = struct{}{}
	}
	var out ClosureRefs
	for _, path := range constraint.SortedPathKeys(keys) {
		set := joinClosureRefSet(a[path], b[path])
		if set.IsBottom() {
			continue
		}
		if out == nil {
			out = make(ClosureRefs, len(keys))
		}
		out[path] = set
	}
	return out
}

func canonicalClosureRefsFinite(refs ClosureRefs) ClosureRefs {
	if len(refs) == 0 {
		return nil
	}
	var out ClosureRefs
	for _, path := range constraint.SortedPathKeys(refs) {
		set := refs[path]
		if set.IsTop() {
			// Per-path Top is a finite map value and remains representable in a
			// closure environment key.
		} else {
			set = canonicalClosureRefSet(set.refs)
		}
		if set.IsBottom() {
			continue
		}
		if out == nil {
			out = make(ClosureRefs, len(refs))
		}
		out[path] = set
	}
	return out
}

func closureRefSetEqual(a, b ClosureRefSet) bool {
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
}

func isClosureRefsTop(refs ClosureRefs) bool {
	return refs != nil && ClosureRefsDomain.Equal(refs, ClosureRefsDomain.Top())
}

func limitClosureRefsDepth(refs ClosureRefs, depth int) ClosureRefs {
	if len(refs) == 0 {
		return refs
	}
	out := make(ClosureRefs)
	for _, path := range constraint.SortedPathKeys(refs) {
		set := limitClosureRefSetDepth(refs[path], depth)
		if set.IsBottom() {
			continue
		}
		out[path] = set
	}
	return canonicalClosureRefsFinite(out)
}

func limitClosureRefSetDepth(set ClosureRefSet, depth int) ClosureRefSet {
	if set.IsBottom() || set.IsTop() {
		return set
	}
	if depth <= 0 {
		return ClosureRefSetTop()
	}
	out := make([]ClosureRef, 0, len(set.refs))
	for _, ref := range set.refs {
		ref.Closures = closureRefsKeyOfDepth(ref.EntryClosureRefs(), depth-1)
		out = append(out, ref)
	}
	return ClosureRefSetOf(out...)
}
