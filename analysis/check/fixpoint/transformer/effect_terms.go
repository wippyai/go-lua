package transformer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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

type PreciseDynamicTarget struct {
	Table  PathTerm
	Key    ValueTerm
	Suffix []segment.Segment
}

type InvalidatePathConfig struct {
	Target                          PathTerm
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
	Table        PathTerm
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
	table        PathTerm
	key          ValueTerm
	value        ValueTerm
	keyPath      PathTerm
	valuePath    PathTerm
	admission    dynamicindex.Admission
	readback     factflow.DynamicIndexReadbackIntent
	appendMode   bool
	site         EffectSite
}

// EffectArena owns effect nodes while sharing the existing scalar/path arena.
// This keeps one canonical term vocabulary without changing scalar Relation
// publication or activating effects in production.
type EffectArena struct {
	terms *Arena
	nodes []effectNode
	keys  map[string][]EffectTerm
}

func NewEffectArena(terms *Arena) *EffectArena {
	return &EffectArena{terms: terms, nodes: []effectNode{{}}, keys: make(map[string][]EffectTerm)}
}

func (a *EffectArena) Terms() *Arena { return a.terms }

func (a *EffectArena) InvalidatePath(config InvalidatePathConfig) (EffectTerm, error) {
	if a == nil || a.terms == nil {
		return 0, fmt.Errorf("transformer: effect arena requires scalar/path terms")
	}
	if err := validInvalidationConfig(config); err != nil {
		return 0, err
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
	if config.Table == 0 || config.Key == 0 || config.Value == 0 {
		return 0, fmt.Errorf("transformer: index mutation requires table, key, and value terms")
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

func validInvalidationConfig(config InvalidatePathConfig) error {
	if config.Target == 0 {
		return fmt.Errorf("transformer: invalidation requires target path")
	}
	if config.Scope != InvalidationScopeSubtree && config.Scope != InvalidationScopeDescendants {
		return fmt.Errorf("transformer: invalidation requires subtree or descendants scope")
	}
	if config.Precise != nil && (config.Precise.Table == 0 || config.Precise.Key == 0) {
		return fmt.Errorf("transformer: precise dynamic invalidation requires table and key terms")
	}
	return nil
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
	if !a.terms.validPath(n.invalidation.Target, shape) {
		return false
	}
	if p := n.invalidation.Precise; p != nil && (!a.terms.validPath(p.Table, shape) || !a.terms.validValue(p.Key, shape, make(map[ValueTerm]bool))) {
		return false
	}
	if n.kind == EffectInvalidatePath {
		return true
	}
	return n.kind == EffectIndexMutation &&
		a.terms.validPath(n.table, shape) &&
		a.terms.validValue(n.key, shape, make(map[ValueTerm]bool)) &&
		a.terms.validValue(n.value, shape, make(map[ValueTerm]bool)) &&
		(n.keyPath == 0 || a.terms.validPath(n.keyPath, shape)) &&
		(n.valuePath == 0 || a.terms.validPath(n.valuePath, shape))
}

func (a *EffectArena) intern(node effectNode) EffectTerm {
	if a == nil || a.terms == nil {
		return 0
	}
	key := a.canonical(node)
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

func (a *EffectArena) canonical(node effectNode) string {
	inv := node.invalidation
	parts := []string{
		strconv.Itoa(int(node.kind)), a.terms.canonicalPath(inv.Target), strconv.Itoa(int(inv.Scope)),
		strconv.FormatBool(inv.PreserveStructuralWitness), strconv.FormatBool(inv.PreserveDynamicValueMemberships),
	}
	if inv.Precise != nil {
		parts = append(parts, a.terms.canonicalPath(inv.Precise.Table), a.terms.canonicalValue(inv.Precise.Key), fmt.Sprint(inv.Precise.Suffix))
	}
	if node.kind == EffectIndexMutation {
		parts = append(parts,
			a.terms.canonicalPath(node.table), a.terms.canonicalValue(node.key), a.terms.canonicalValue(node.value),
			a.terms.canonicalPath(node.keyPath), a.terms.canonicalPath(node.valuePath),
			strconv.Itoa(int(node.admission)), strconv.Itoa(int(node.readback)), strconv.FormatBool(node.appendMode),
			strconv.FormatUint(node.site.Owner, 10), strconv.FormatUint(uint64(node.site.Ordinal), 10),
		)
	}
	return strings.Join(parts, "|")
}

func effectNodeEqual(left, right effectNode) bool {
	if left.kind != right.kind || left.table != right.table || left.key != right.key || left.value != right.value ||
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
