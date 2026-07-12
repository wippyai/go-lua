package transformer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// EffectTerm is a structured symbolic state transaction. It is deliberately
// disjoint from ValueTerm: effects can never be smuggled through product.Value
// or mistaken for scalar output evidence.
type EffectTerm uint32

type EffectKind uint8

const (
	EffectInvalid EffectKind = iota
	EffectInvalidatePath
	EffectIndexMutation
	EffectAllocationTemplate
	effectKindCount
)

type InvalidationScope uint8

const (
	InvalidationScopeInvalid InvalidationScope = iota
	InvalidationScopeSubtree
	InvalidationScopeDescendants
)

// EffectSite is stable lexical provenance. Caller CFG points and solve
// generations are intentionally absent; fresh allocation rebasing belongs to
// specialization, not effect identity.
type EffectSite struct {
	Owner   uint64
	Ordinal uint32
}

// EffectTargetTerm identifies the object an effect mutates. It is a closed,
// tagged union: a target is backed by exactly one boundary path or one shared
// allocation template. Keeping allocation identity symbolic lets allocation
// and mutation effects compose before either is materialized for a caller.
type EffectTargetTerm struct {
	kind       effectTargetKind
	path       PathTerm
	allocation AllocationTemplateTerm
}

type effectTargetKind uint8

const (
	effectTargetInvalid effectTargetKind = iota
	effectTargetPath
	effectTargetAllocation
)

func PathEffectTarget(path PathTerm) EffectTargetTerm {
	if path == 0 {
		return EffectTargetTerm{}
	}
	return EffectTargetTerm{kind: effectTargetPath, path: path}
}

func AllocationEffectTarget(allocation AllocationTemplateTerm) EffectTargetTerm {
	if allocation == 0 {
		return EffectTargetTerm{}
	}
	return EffectTargetTerm{kind: effectTargetAllocation, allocation: allocation}
}

type PreciseDynamicTarget struct {
	Table  PathTerm
	Key    ValueTerm
	Suffix []segment.Segment
}

type InvalidatePathConfig struct {
	Target                          EffectTargetTerm
	Scope                           InvalidationScope
	PreserveStructuralWitness       bool
	PreserveDynamicValueMemberships bool
	Precise                         *PreciseDynamicTarget
}

// IndexMutationConfig is the atomic N3+N4 transaction. Invalidation is
// mandatory: constructing a dynamic write without its preceding invalidation
// barrier is impossible through this API.
type IndexMutationConfig struct {
	Invalidation InvalidatePathConfig
	Table        EffectTargetTerm
	Key          ValueTerm
	Value        ValueTerm
	KeyPath      PathTerm
	ValuePath    PathTerm
	Admission    dynamicindex.Admission
	Readback     factflow.DynamicIndexReadbackIntent
	Append       bool
	Site         EffectSite
}

type effectNode struct {
	kind         EffectKind
	invalidation InvalidatePathConfig
	table        EffectTargetTerm
	key          ValueTerm
	value        ValueTerm
	keyPath      PathTerm
	valuePath    PathTerm
	admission    dynamicindex.Admission
	readback     factflow.DynamicIndexReadbackIntent
	appendMode   bool
	site         EffectSite
	allocation   AllocationTemplateTerm
}

// EffectArena owns effect nodes while sharing the existing scalar/path arena.
// This keeps one canonical term vocabulary without changing scalar Relation
// publication or activating effects in production.
type EffectArena struct {
	terms *Arena
	nodes []effectNode
	keys  map[uint64][]EffectTerm
}

func NewEffectArena(terms *Arena) *EffectArena {
	return &EffectArena{terms: terms, nodes: []effectNode{{}}, keys: make(map[uint64][]EffectTerm)}
}

func (a *EffectArena) Terms() *Arena { return a.terms }

func (a *EffectArena) InvalidatePath(config InvalidatePathConfig) (EffectTerm, error) {
	if a == nil || a.terms == nil {
		return 0, fmt.Errorf("transformer: effect arena requires scalar/path terms")
	}
	if err := validInvalidationConfig(config); err != nil {
		return 0, err
	}
	if !a.ownsTarget(config.Target) {
		return 0, fmt.Errorf("transformer: invalidation target does not belong to effect arena")
	}
	return a.intern(effectNode{kind: EffectInvalidatePath, invalidation: cloneInvalidationConfig(config)}), nil
}

func (a *EffectArena) IndexMutation(config IndexMutationConfig) (EffectTerm, error) {
	if a == nil || a.terms == nil {
		return 0, fmt.Errorf("transformer: effect arena requires scalar/path terms")
	}
	if err := validInvalidationConfig(config.Invalidation); err != nil {
		return 0, fmt.Errorf("transformer: index mutation invalidation: %w", err)
	}
	if !a.ownsTarget(config.Invalidation.Target) || !a.ownsTarget(config.Table) || config.Key == 0 || config.Value == 0 {
		return 0, fmt.Errorf("transformer: index mutation requires table, key, and value terms")
	}
	if config.Invalidation.Target != config.Table {
		return 0, fmt.Errorf("transformer: index mutation invalidation and table must target the same object")
	}
	if config.Site.Owner == 0 {
		return 0, fmt.Errorf("transformer: index mutation requires lexical owner provenance")
	}
	if config.Admission == dynamicindex.AdmissionBottom {
		return 0, fmt.Errorf("transformer: index mutation cannot publish bottom admission")
	}
	if config.Readback < factflow.DynamicIndexReadbackNone || config.Readback > factflow.DynamicIndexReadbackKeyAndValue {
		return 0, fmt.Errorf("transformer: index mutation requires explicit readback intent")
	}
	return a.intern(effectNode{
		kind: EffectIndexMutation, invalidation: cloneInvalidationConfig(config.Invalidation),
		table: config.Table, key: config.Key, value: config.Value,
		keyPath: config.KeyPath, valuePath: config.ValuePath,
		admission: config.Admission, readback: config.Readback, appendMode: config.Append, site: config.Site,
	}), nil
}

// AllocationTemplate retains the heap/fresh half of one correlated symbolic
// allocation. Result ValueTerms reference the same AllocationTemplateTerm.
func (a *EffectArena) AllocationTemplate(allocation AllocationTemplateTerm) (EffectTerm, error) {
	if a == nil || a.terms == nil || !a.terms.validAllocation(allocation) {
		return 0, fmt.Errorf("transformer: allocation effect requires a valid shared allocation term")
	}
	return a.intern(effectNode{kind: EffectAllocationTemplate, allocation: allocation}), nil
}

func validInvalidationConfig(config InvalidatePathConfig) error {
	if !wellFormedEffectTarget(config.Target) {
		return fmt.Errorf("transformer: invalidation requires target")
	}
	if config.Scope != InvalidationScopeSubtree && config.Scope != InvalidationScopeDescendants {
		return fmt.Errorf("transformer: invalidation requires subtree or descendants scope")
	}
	if config.Precise != nil && (config.Precise.Table == 0 || config.Precise.Key == 0) {
		return fmt.Errorf("transformer: precise dynamic invalidation requires table and key terms")
	}
	return nil
}

func wellFormedEffectTarget(target EffectTargetTerm) bool {
	return (target.kind == effectTargetPath && target.path != 0 && target.allocation == 0) ||
		(target.kind == effectTargetAllocation && target.path == 0 && target.allocation != 0)
}

func (a *EffectArena) ownsTarget(target EffectTargetTerm) bool {
	if a == nil || a.terms == nil || !wellFormedEffectTarget(target) {
		return false
	}
	if target.kind == effectTargetPath {
		return int(target.path) < len(a.terms.paths)
	}
	return a.terms.validAllocation(target.allocation)
}

func (a *EffectArena) validTarget(target EffectTargetTerm, shape Shape) bool {
	if a == nil || a.terms == nil {
		return false
	}
	switch target.kind {
	case effectTargetPath:
		return target.path != 0 && target.allocation == 0 && a.terms.validPath(target.path, shape)
	case effectTargetAllocation:
		return target.path == 0 && target.allocation != 0 && a.terms.validAllocation(target.allocation)
	default:
		return false
	}
}

func cloneInvalidationConfig(config InvalidatePathConfig) InvalidatePathConfig {
	if config.Precise == nil {
		return config
	}
	precise := *config.Precise
	precise.Suffix = append([]segment.Segment(nil), precise.Suffix...)
	config.Precise = &precise
	return config
}

func (a *EffectArena) Kind(term EffectTerm) EffectKind {
	if a == nil || term == 0 || int(term) >= len(a.nodes) {
		return EffectInvalid
	}
	return a.nodes[term].kind
}

// Valid reports whether every scalar/path reference belongs to shape.
func (a *EffectArena) Valid(term EffectTerm, shape Shape) bool {
	if a == nil || a.terms == nil || term == 0 || int(term) >= len(a.nodes) {
		return false
	}
	n := a.nodes[term]
	if n.kind == EffectAllocationTemplate {
		return a.terms.validAllocation(n.allocation)
	}
	if !a.validTarget(n.invalidation.Target, shape) {
		return false
	}
	if p := n.invalidation.Precise; p != nil && (!a.terms.validPath(p.Table, shape) || !a.terms.validValue(p.Key, shape, make(map[ValueTerm]bool))) {
		return false
	}
	if n.kind == EffectInvalidatePath {
		return true
	}
	return n.kind == EffectIndexMutation &&
		a.validTarget(n.table, shape) &&
		a.terms.validValue(n.key, shape, make(map[ValueTerm]bool)) &&
		a.terms.validValue(n.value, shape, make(map[ValueTerm]bool)) &&
		(n.keyPath == 0 || a.terms.validPath(n.keyPath, shape)) &&
		(n.valuePath == 0 || a.terms.validPath(n.valuePath, shape))
}

func (a *EffectArena) intern(node effectNode) EffectTerm {
	if a == nil || a.terms == nil {
		return 0
	}
	key := a.terms.maskFingerprint(effectFingerprint(node))
	for _, term := range a.keys[key] {
		if effectNodeEqual(a.nodes[term], node) {
			return term
		}
	}
	term := EffectTerm(len(a.nodes))
	a.nodes = append(a.nodes, node)
	a.keys[key] = append(a.keys[key], term)
	return term
}

func effectFingerprint(node effectNode) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, 0x656666656374)
	h = internalhash.MixHash(h, uint64(node.kind))
	h = hashEffectTarget(h, node.invalidation.Target)
	h = internalhash.MixHash(h, uint64(node.invalidation.Scope))
	if node.invalidation.PreserveStructuralWitness {
		h = internalhash.MixHash(h, 1)
	}
	if node.invalidation.PreserveDynamicValueMemberships {
		h = internalhash.MixHash(h, 2)
	}
	if precise := node.invalidation.Precise; precise != nil {
		h = internalhash.MixHash(h, 3)
		h = internalhash.MixHash(h, uint64(precise.Table))
		h = internalhash.MixHash(h, uint64(precise.Key))
		h = internalhash.MixHash(h, uint64(len(precise.Suffix)))
		for _, suffix := range precise.Suffix {
			h = hashSegment(h, suffix)
		}
	}
	h = hashEffectTarget(h, node.table)
	h = internalhash.MixHash(h, uint64(node.key))
	h = internalhash.MixHash(h, uint64(node.value))
	h = internalhash.MixHash(h, uint64(node.keyPath))
	h = internalhash.MixHash(h, uint64(node.valuePath))
	h = internalhash.MixHash(h, uint64(node.admission))
	h = internalhash.MixHash(h, uint64(node.readback))
	if node.appendMode {
		h = internalhash.MixHash(h, 4)
	}
	h = internalhash.MixHash(h, node.site.Owner)
	h = internalhash.MixHash(h, uint64(node.site.Ordinal))
	return internalhash.MixHash(h, uint64(node.allocation))
}

func hashEffectTarget(h uint64, target EffectTargetTerm) uint64 {
	h = internalhash.MixHash(h, uint64(target.kind))
	h = internalhash.MixHash(h, uint64(target.path))
	return internalhash.MixHash(h, uint64(target.allocation))
}

func (a *EffectArena) canonical(node effectNode) string {
	inv := node.invalidation
	parts := []string{
		strconv.Itoa(int(node.kind)), a.canonicalTarget(inv.Target), strconv.Itoa(int(inv.Scope)),
		strconv.FormatBool(inv.PreserveStructuralWitness), strconv.FormatBool(inv.PreserveDynamicValueMemberships),
	}
	if inv.Precise != nil {
		parts = append(parts, a.terms.canonicalPath(inv.Precise.Table), a.terms.canonicalValue(inv.Precise.Key), fmt.Sprint(inv.Precise.Suffix))
	}
	if node.kind == EffectIndexMutation {
		parts = append(parts,
			a.canonicalTarget(node.table), a.terms.canonicalValue(node.key), a.terms.canonicalValue(node.value),
			a.terms.canonicalPath(node.keyPath), a.terms.canonicalPath(node.valuePath),
			strconv.Itoa(int(node.admission)), strconv.Itoa(int(node.readback)), strconv.FormatBool(node.appendMode),
			strconv.FormatUint(node.site.Owner, 10), strconv.FormatUint(uint64(node.site.Ordinal), 10),
		)
	}
	if node.kind == EffectAllocationTemplate && a.terms.validAllocation(node.allocation) {
		op := a.terms.allocations[node.allocation].op
		parts = append(parts, fmt.Sprintf("%d:%s:%d", op.Site().Owner, op.Site().Template, op.Site().Ordinal))
	}
	return strings.Join(parts, "|")
}

func (a *EffectArena) canonicalTarget(target EffectTargetTerm) string {
	switch target.kind {
	case effectTargetPath:
		// Preserve the historical canonical spelling for boundary-path effects.
		return a.terms.canonicalPath(target.path)
	case effectTargetAllocation:
		if a.terms.validAllocation(target.allocation) {
			op := a.terms.allocations[target.allocation].op
			return fmt.Sprintf("alloc:%d:%s:%d", op.Site().Owner, op.Site().Template, op.Site().Ordinal)
		}
	}
	return "invalid"
}

func effectNodeEqual(left, right effectNode) bool {
	if left.kind != right.kind || left.allocation != right.allocation || left.table != right.table || left.key != right.key || left.value != right.value ||
		left.keyPath != right.keyPath || left.valuePath != right.valuePath || left.admission != right.admission ||
		left.readback != right.readback || left.appendMode != right.appendMode || left.site != right.site ||
		left.invalidation.Target != right.invalidation.Target || left.invalidation.Scope != right.invalidation.Scope ||
		left.invalidation.PreserveStructuralWitness != right.invalidation.PreserveStructuralWitness ||
		left.invalidation.PreserveDynamicValueMemberships != right.invalidation.PreserveDynamicValueMemberships {
		return false
	}
	lp, rp := left.invalidation.Precise, right.invalidation.Precise
	if lp == nil || rp == nil {
		return lp == nil && rp == nil
	}
	if lp.Table != rp.Table || lp.Key != rp.Key || len(lp.Suffix) != len(rp.Suffix) {
		return false
	}
	for i := range lp.Suffix {
		if lp.Suffix[i] != rp.Suffix[i] {
			return false
		}
	}
	return true
}
