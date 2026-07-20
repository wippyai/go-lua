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
	EffectObjectMaterialization
	EffectPathStore
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

type PathStoreConfig struct {
	// Target/Value/SourcePath/SuppressProof are the paired shorthand retained
	// for callers which genuinely own one identical assignment+static write.
	// New lowering should set HasAssignment/HasStatic and the independent
	// payloads below; mixing the two forms is rejected.
	Target              PathTerm
	Value               ValueTerm
	SourcePath          PathTerm
	SuppressProof       bool
	Assignment          PathStoreWriteConfig
	Static              PathStoreWriteConfig
	HasAssignment       bool
	HasStatic           bool
	StaticHasAnnotation bool
	Object              PathStoreObjectConfig
	Site                EffectSite
}

type PathStoreWriteConfig struct {
	Target        PathTerm
	Value         ValueTerm
	SourcePath    PathTerm
	HasSourcePath bool
	SuppressProof bool
}

type PathStoreObjectEntryConfig struct {
	Target        PathTerm
	Value         ValueTerm
	SourcePath    PathTerm
	HasSourcePath bool
	SuppressProof bool
	Expected      ValueTerm
	HasExpected   bool
}

type PathStoreHeapMemberConfig struct {
	Suffix      []segment.Segment
	Value       ValueTerm
	Expected    ValueTerm
	HasExpected bool
}

type PathStoreHeapObjectConfig struct {
	Root        ValueTerm
	Members     []PathStoreHeapMemberConfig
	StableShape bool
}

type PathStoreObjectConfig struct {
	Heaps     []PathStoreHeapObjectConfig
	Entries   []PathStoreObjectEntryConfig
	ListFloor int64
}

type effectNode struct {
	kind                         EffectKind
	invalidation                 InvalidatePathConfig
	table                        EffectTargetTerm
	key                          ValueTerm
	value                        ValueTerm
	keyPath                      PathTerm
	valuePath                    PathTerm
	admission                    dynamicindex.Admission
	readback                     factflow.DynamicIndexReadbackIntent
	appendMode                   bool
	site                         EffectSite
	allocation                   AllocationTemplateTerm
	pathStoreAssignment          PathStoreWriteConfig
	pathStoreStatic              PathStoreWriteConfig
	pathStoreHasAssignment       bool
	pathStoreHasStatic           bool
	pathStoreStaticHasAnnotation bool
	pathStoreObject              PathStoreObjectConfig
}

// EffectArena owns effect nodes while sharing the existing scalar/path arena.
// This keeps one canonical term vocabulary without changing scalar Relation
// publication or activating effects in production.
type EffectArena struct {
	terms  *Arena
	nodes  []effectNode
	keys   map[uint64][]EffectTerm
	sealed bool
}

func NewEffectArena(terms *Arena) *EffectArena {
	return &EffectArena{terms: terms, nodes: []effectNode{{}}, keys: make(map[uint64][]EffectTerm)}
}

func (a *EffectArena) Seal() {
	if a != nil {
		a.sealed = true
	}
}

func (a *EffectArena) Sealed() bool { return a != nil && a.sealed }

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

// ObjectMaterialization retains the heap half of one Lua table constructor.
// It is independent of storage: an rvalue constructor passed directly to a
// call materializes the same object graph as one assigned to a local path.
func (a *EffectArena) ObjectMaterialization(object PathStoreObjectConfig, site EffectSite) (EffectTerm, error) {
	if a == nil || a.terms == nil || site.Owner == 0 || len(object.Heaps) == 0 || len(object.Entries) != 0 || object.ListFloor != 0 {
		return 0, fmt.Errorf("transformer: object materialization requires a heap-only literal graph and provenance")
	}
	return a.intern(effectNode{kind: EffectObjectMaterialization, pathStoreObject: clonePathStoreObjectConfig(object), site: site}), nil
}

// PathStore freezes the exact PathAssignment/PathStaticMemberWrite transaction
// as one opaque ordered effect. Either write may exist independently.
func (a *EffectArena) PathStore(config PathStoreConfig) (EffectTerm, error) {
	if a == nil || a.terms == nil || config.Site.Owner == 0 {
		return 0, fmt.Errorf("transformer: path store requires scalar/path terms and provenance")
	}
	assignment, static, hasAssignment, hasStatic, err := canonicalPathStoreWrites(config)
	if err != nil {
		return 0, err
	}
	object := clonePathStoreObjectConfig(config.Object)
	if (len(object.Heaps) != 0 || len(object.Entries) != 0 || object.ListFloor != 0) && !hasAssignment {
		return 0, fmt.Errorf("transformer: path-store object sidecar requires assignment ownership")
	}
	return a.intern(effectNode{
		kind: EffectPathStore, pathStoreAssignment: assignment, pathStoreStatic: static,
		pathStoreHasAssignment: hasAssignment, pathStoreHasStatic: hasStatic,
		pathStoreStaticHasAnnotation: config.StaticHasAnnotation, pathStoreObject: object, site: config.Site,
	}), nil
}

func canonicalPathStoreWrites(config PathStoreConfig) (PathStoreWriteConfig, PathStoreWriteConfig, bool, bool, error) {
	paired := config.Target != 0 || config.Value != 0 || config.SourcePath != 0 || config.SuppressProof
	explicit := config.HasAssignment || config.HasStatic || config.Assignment != (PathStoreWriteConfig{}) || config.Static != (PathStoreWriteConfig{})
	if paired && explicit {
		return PathStoreWriteConfig{}, PathStoreWriteConfig{}, false, false, fmt.Errorf("transformer: path store mixes paired and independent write forms")
	}
	normalize := func(write PathStoreWriteConfig) PathStoreWriteConfig {
		if write.SourcePath != 0 {
			write.HasSourcePath = true
		}
		return write
	}
	valid := func(write PathStoreWriteConfig) bool {
		return write.Target != 0 && write.Value != 0 && write.HasSourcePath == (write.SourcePath != 0)
	}
	if paired {
		write := normalize(PathStoreWriteConfig{Target: config.Target, Value: config.Value, SourcePath: config.SourcePath, SuppressProof: config.SuppressProof})
		if !valid(write) {
			return PathStoreWriteConfig{}, PathStoreWriteConfig{}, false, false, fmt.Errorf("transformer: paired path store is incomplete")
		}
		return write, write, true, true, nil
	}
	config.Assignment, config.Static = normalize(config.Assignment), normalize(config.Static)
	if !config.HasAssignment && !config.HasStatic || config.HasAssignment != (config.Assignment != (PathStoreWriteConfig{})) || config.HasStatic != (config.Static != (PathStoreWriteConfig{})) ||
		config.HasAssignment && !valid(config.Assignment) || config.HasStatic && !valid(config.Static) {
		return PathStoreWriteConfig{}, PathStoreWriteConfig{}, false, false, fmt.Errorf("transformer: independent path store is incomplete")
	}
	return config.Assignment, config.Static, config.HasAssignment, config.HasStatic, nil
}

func clonePathStoreObjectConfig(in PathStoreObjectConfig) PathStoreObjectConfig {
	out := PathStoreObjectConfig{ListFloor: in.ListFloor, Entries: append([]PathStoreObjectEntryConfig(nil), in.Entries...), Heaps: make([]PathStoreHeapObjectConfig, len(in.Heaps))}
	for index, heap := range in.Heaps {
		out.Heaps[index].Root = heap.Root
		out.Heaps[index].StableShape = heap.StableShape
		out.Heaps[index].Members = make([]PathStoreHeapMemberConfig, len(heap.Members))
		for memberIndex, member := range heap.Members {
			member.Suffix = append([]segment.Segment(nil), member.Suffix...)
			out.Heaps[index].Members[memberIndex] = member
		}
	}
	return out
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
	if n.kind == EffectObjectMaterialization {
		return n.site.Owner != 0 && len(n.pathStoreObject.Heaps) != 0 && len(n.pathStoreObject.Entries) == 0 && n.pathStoreObject.ListFloor == 0 &&
			a.validPathStoreObject(n.pathStoreObject, shape)
	}
	if n.kind == EffectPathStore {
		validWrite := func(write PathStoreWriteConfig) bool {
			return a.terms.validPath(write.Target, shape) &&
				a.terms.validValue(write.Value, shape, make(map[ValueTerm]bool)) &&
				write.HasSourcePath == (write.SourcePath != 0) &&
				(!write.HasSourcePath || a.terms.validPath(write.SourcePath, shape))
		}
		return n.site.Owner != 0 && (n.pathStoreHasAssignment || n.pathStoreHasStatic) &&
			(!n.pathStoreHasAssignment || validWrite(n.pathStoreAssignment)) &&
			(!n.pathStoreHasStatic || validWrite(n.pathStoreStatic)) &&
			a.validPathStoreObject(n.pathStoreObject, shape)
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

func (a *EffectArena) validPathStoreObject(object PathStoreObjectConfig, shape Shape) bool {
	if object.ListFloor < 0 {
		return false
	}
	for _, heap := range object.Heaps {
		if !a.terms.validValue(heap.Root, shape, make(map[ValueTerm]bool)) {
			return false
		}
		for _, member := range heap.Members {
			if len(member.Suffix) == 0 ||
				!a.terms.validValue(member.Value, shape, make(map[ValueTerm]bool)) ||
				member.HasExpected != (member.Expected != 0) ||
				member.HasExpected && !a.terms.validValue(member.Expected, shape, make(map[ValueTerm]bool)) {
				return false
			}
		}
	}
	for _, entry := range object.Entries {
		if !a.terms.validPath(entry.Target, shape) ||
			!a.terms.validValue(entry.Value, shape, make(map[ValueTerm]bool)) ||
			entry.HasSourcePath != (entry.SourcePath != 0) ||
			entry.HasSourcePath && !a.terms.validPath(entry.SourcePath, shape) ||
			entry.HasExpected != (entry.Expected != 0) ||
			entry.HasExpected && !a.terms.validValue(entry.Expected, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
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
	if a.sealed {
		return 0
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
	h = hashPathStoreWrite(h, node.pathStoreAssignment, node.pathStoreHasAssignment, 11)
	h = hashPathStoreWrite(h, node.pathStoreStatic, node.pathStoreHasStatic, 13)
	if node.pathStoreStaticHasAnnotation {
		h = internalhash.MixHash(h, 1)
	}
	h = hashPathStoreObject(h, node.pathStoreObject)
	return internalhash.MixHash(h, uint64(node.allocation))
}

func hashPathStoreWrite(h uint64, write PathStoreWriteConfig, present bool, tag uint64) uint64 {
	if !present {
		return internalhash.MixHash(h, tag)
	}
	h = internalhash.MixHash(h, tag+1)
	h = internalhash.MixHash(h, uint64(write.Target))
	h = internalhash.MixHash(h, uint64(write.Value))
	h = internalhash.MixHash(h, uint64(write.SourcePath))
	if write.HasSourcePath {
		h = internalhash.MixHash(h, tag+3)
	}
	if write.SuppressProof {
		h = internalhash.MixHash(h, tag+2)
	}
	return h
}

func hashPathStoreObject(h uint64, object PathStoreObjectConfig) uint64 {
	h = internalhash.MixHash(h, uint64(object.ListFloor))
	h = internalhash.MixHash(h, uint64(len(object.Heaps)))
	for _, heap := range object.Heaps {
		h = internalhash.MixHash(h, uint64(heap.Root))
		if heap.StableShape {
			h = internalhash.MixHash(h, 1)
		}
		h = internalhash.MixHash(h, uint64(len(heap.Members)))
		for _, member := range heap.Members {
			h = internalhash.MixHash(h, uint64(member.Value))
			h = internalhash.MixHash(h, uint64(len(member.Suffix)))
			for _, suffix := range member.Suffix {
				h = hashSegment(h, suffix)
			}
			if member.HasExpected {
				h = internalhash.MixHash(h, 1)
				h = internalhash.MixHash(h, uint64(member.Expected))
			}
		}
	}
	h = internalhash.MixHash(h, uint64(len(object.Entries)))
	for _, entry := range object.Entries {
		h = internalhash.MixHash(h, uint64(entry.Target))
		h = internalhash.MixHash(h, uint64(entry.Value))
		if entry.HasSourcePath {
			h = internalhash.MixHash(h, 2)
			h = internalhash.MixHash(h, uint64(entry.SourcePath))
		}
		if entry.SuppressProof {
			h = internalhash.MixHash(h, 7)
		}
		if entry.HasExpected {
			h = internalhash.MixHash(h, 3)
			h = internalhash.MixHash(h, uint64(entry.Expected))
		}
	}
	return h
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
	if node.kind == EffectPathStore || node.kind == EffectObjectMaterialization {
		parts = append(parts,
			canonicalPathStoreWrite(a.terms, "assignment", node.pathStoreAssignment, node.pathStoreHasAssignment),
			canonicalPathStoreWrite(a.terms, "static", node.pathStoreStatic, node.pathStoreHasStatic),
			strconv.FormatBool(node.pathStoreStaticHasAnnotation), strconv.FormatUint(node.site.Owner, 10), strconv.FormatUint(uint64(node.site.Ordinal), 10),
		)
		parts = append(parts, canonicalPathStoreObject(a.terms, node.pathStoreObject))
	}
	return strings.Join(parts, "|")
}

func canonicalPathStoreWrite(terms *Arena, label string, write PathStoreWriteConfig, present bool) string {
	if !present {
		return label + ":absent"
	}
	return fmt.Sprintf("%s:%s:%s:%s:%t:%t", label, terms.canonicalPath(write.Target), terms.canonicalValue(write.Value), terms.canonicalPath(write.SourcePath), write.HasSourcePath, write.SuppressProof)
}

func canonicalPathStoreObject(terms *Arena, object PathStoreObjectConfig) string {
	parts := []string{strconv.FormatInt(object.ListFloor, 10)}
	for _, heap := range object.Heaps {
		parts = append(parts, fmt.Sprintf("h:%s:%t", terms.canonicalValue(heap.Root), heap.StableShape))
		for _, member := range heap.Members {
			expected := ""
			if member.HasExpected {
				expected = terms.canonicalValue(member.Expected)
			}
			parts = append(parts, fmt.Sprintf("m:%v:%s:%t:%s", member.Suffix, terms.canonicalValue(member.Value), member.HasExpected, expected))
		}
	}
	for _, entry := range object.Entries {
		expected := ""
		if entry.HasExpected {
			expected = terms.canonicalValue(entry.Expected)
		}
		parts = append(parts, fmt.Sprintf("e:%s:%s:%s:%t:%t:%t:%s", terms.canonicalPath(entry.Target), terms.canonicalValue(entry.Value), terms.canonicalPath(entry.SourcePath), entry.HasSourcePath, entry.SuppressProof, entry.HasExpected, expected))
	}
	return strings.Join(parts, ",")
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
		left.pathStoreAssignment != right.pathStoreAssignment || left.pathStoreStatic != right.pathStoreStatic || left.pathStoreHasAssignment != right.pathStoreHasAssignment || left.pathStoreHasStatic != right.pathStoreHasStatic || left.pathStoreStaticHasAnnotation != right.pathStoreStaticHasAnnotation ||
		left.keyPath != right.keyPath || left.valuePath != right.valuePath || left.admission != right.admission ||
		left.readback != right.readback || left.appendMode != right.appendMode || left.site != right.site ||
		left.invalidation.Target != right.invalidation.Target || left.invalidation.Scope != right.invalidation.Scope ||
		left.invalidation.PreserveStructuralWitness != right.invalidation.PreserveStructuralWitness ||
		left.invalidation.PreserveDynamicValueMemberships != right.invalidation.PreserveDynamicValueMemberships {
		return false
	}
	if !equalPathStoreObject(left.pathStoreObject, right.pathStoreObject) {
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

func equalPathStoreObject(left, right PathStoreObjectConfig) bool {
	if left.ListFloor != right.ListFloor || len(left.Heaps) != len(right.Heaps) || len(left.Entries) != len(right.Entries) {
		return false
	}
	for index := range left.Heaps {
		lh, rh := left.Heaps[index], right.Heaps[index]
		if lh.Root != rh.Root || len(lh.Members) != len(rh.Members) {
			return false
		}
		for memberIndex := range lh.Members {
			lm, rm := lh.Members[memberIndex], rh.Members[memberIndex]
			if lm.Value != rm.Value || lm.Expected != rm.Expected || lm.HasExpected != rm.HasExpected || len(lm.Suffix) != len(rm.Suffix) {
				return false
			}
			for suffixIndex := range lm.Suffix {
				if lm.Suffix[suffixIndex] != rm.Suffix[suffixIndex] {
					return false
				}
			}
		}
	}
	for index := range left.Entries {
		le, re := left.Entries[index], right.Entries[index]
		if le.Target != re.Target || le.Value != re.Value || le.SourcePath != re.SourcePath || le.HasSourcePath != re.HasSourcePath || le.SuppressProof != re.SuppressProof || le.Expected != re.Expected || le.HasExpected != re.HasExpected {
			return false
		}
	}
	return true
}
