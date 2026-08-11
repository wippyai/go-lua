package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/program/link"
)

// Access is the sole typed Rule execution capability. It is valid for one
// synchronous symbolic Product row only. Rules see no carrier coordinate,
// guard graph, Fact schema, or mutable State.
type Access[K ~uint64, V any] struct {
	frame    *ruleExecution
	epoch    uint64
	identity *ruleIdentity
	output   *Factor[K, V]
}

// ReadRef is an opaque non-comparable declaration capability. It names one
// exact Factor at one exact ordered Rule input and cannot be reconstructed
// from a coordinate, key, or factor slot.
type ReadRef[K ~uint64, V any] struct {
	binding *readBinding[K, V]
	_       [0]func()
}

type readBinding[K ~uint64, V any] struct {
	owner    *ruleIdentity
	factor   *Factor[K, V]
	position int
	exact    bool
	key      uint64
}

// ReadAt evaluates the immutable input Facts root selected by ref at this
// Product row's representative valuation. Product has already refined every
// present typed FDD decision, so the representative cannot select a different
// value within the row. Reads never observe staged output writes.
func ReadAt[OK ~uint64, OV any, K ~uint64, V any](access Access[OK, OV], ref ReadRef[K, V], key K) (V, bool, bool) {
	var zero V
	binding := ref.binding
	if !access.valid() || binding == nil || binding.owner == nil || binding.owner != access.identity || binding.factor == nil || binding.factor.solver != access.output.solver || binding.position < 0 || binding.position >= access.frame.inputs.Len() || binding.exact && binding.key != uint64(key) || !binding.factor.admits(key) {
		return zero, false, false
	}
	input, ok := access.frame.inputs.Input(binding.position)
	if !ok {
		return zero, false, false
	}
	value, present, valid := binding.factor.binding.Read(input, key, access.frame.inputs.Valuation().At)
	if !valid {
		return zero, false, false
	}
	if binding.factor.slot < 0 || !access.frame.transaction.observeRead(access.frame.origins[binding.position], uint32(binding.factor.slot), uint64(key), access.frame.region) {
		return zero, false, false
	}
	return value, present, true
}

// Carry makes this callback's output Factor start from one declared input
// Factor plane. It is not a direct Facts Put: later Set or Join continues in
// the same staged patch, and a prior local staged write is discarded exactly
// as a strong transfer replaces that output plane.
func Carry[K ~uint64, V any](access Access[K, V], ref ReadRef[K, V]) bool {
	binding := ref.binding
	if !access.valid() || access.frame.carried || len(access.identity.writes) != 0 || binding == nil || binding.exact || binding.owner == nil || binding.owner != access.identity || binding.factor == nil || binding.factor != access.output || binding.position < 0 || binding.position >= access.frame.inputs.Len() {
		return false
	}
	input, ok := access.frame.inputs.Input(binding.position)
	if !ok || access.frame.patches == nil || !executionCarry(access.frame.patches, access.output, input) || access.output.slot < 0 || !access.frame.transaction.observePlane(access.frame.origins[binding.position], uint32(access.output.slot), access.frame.region) {
		return false
	}
	access.frame.carried = true
	return true
}

// Selection exposes the exact sealed Link selector only while a Relation
// Rule executes. Local At/From Rules return false and no endpoint taxonomy is
// exposed to domains.
func (access Access[K, V]) Selection() (link.Candidate, link.Application, bool) {
	if !access.valid() || access.frame.relation == nil {
		return link.Candidate{}, link.Application{}, false
	}
	relation := access.frame.relation
	if relation.target == nil || relation.target != access.identity || relation.target.anchor.form != ruleRelation || relation.candidate == (link.Candidate{}) {
		return link.Candidate{}, link.Application{}, false
	}
	return relation.candidate, relation.target.anchor.application, true
}

// Set strongly updates this Rule's declared output Factor under the exact
// current Product row. The patch is private to the row and cannot become a
// completed State value.
func (access Access[K, V]) Set(key K, value V) bool {
	if !access.writable() || !access.output.admits(key) || !access.identity.allowsWrite(uint64(key)) {
		return false
	}
	patch, ok := executionPatchFor(access.frame.patches, access.output)
	return ok && patch.Set(key, access.frame.region, value)
}

// Join weakly updates this Rule's declared output Factor under the exact
// current Product row. Undefined sparse leaves have the declared Factor
// Default semantics inside stage, never an invented engine default.
func (access Access[K, V]) Join(key K, value V) bool {
	if !access.writable() || !access.output.admits(key) || !access.identity.allowsWrite(uint64(key)) {
		return false
	}
	patch, ok := executionPatchFor(access.frame.patches, access.output)
	return ok && patch.WeakJoin(key, access.frame.region, value)
}

// Prune removes exactly this Product terminal tuple. It creates no default
// result and invalidates any staged row patches at row completion.
func (access Access[K, V]) Prune() bool {
	if !access.valid() || access.frame.pruned {
		return false
	}
	access.frame.pruned = true
	return true
}

func (access Access[K, V]) valid() bool {
	return access.frame != nil && access.epoch != 0 && access.identity != nil && access.output != nil && access.frame.epoch == access.epoch && access.frame.rule == access.identity && access.frame.transaction != nil && access.frame.transaction.executing
}

func (access Access[K, V]) writable() bool {
	return access.valid() && !access.frame.pruned && access.frame.patches != nil
}

func (identity *ruleIdentity) allowsWrite(key uint64) bool {
	if identity == nil || len(identity.writes) == 0 {
		return identity != nil
	}
	index := sort.Search(len(identity.writes), func(index int) bool { return identity.writes[index] >= key })
	return index < len(identity.writes) && identity.writes[index] == key
}

// executionPatches is the one row-local heterogeneous staged-output set. A
// typed stage.Patch remains owned by factbinding; this type owns only their
// shared callback lifetime and deterministic final attachment order. It is
// not a second Fact root, a vector, or a compatibility carrier.
type executionPatches struct {
	base     facts.Facts
	byFactor map[any]int
	entries  []executionPatchEntry
	closed   bool
}

type executionPatchEntry struct {
	slot    int
	key     any
	factor  any
	discard func()
	commit  func(facts.Facts) (facts.Facts, bool, bool)
}

func newExecutionPatches(base facts.Facts) *executionPatches {
	return &executionPatches{base: base, byFactor: make(map[any]int)}
}

func executionPatchFor[K ~uint64, V any](patches *executionPatches, output *Factor[K, V]) (*factbinding.Patch[K, V], bool) {
	if patches == nil || patches.closed || output == nil || output.slot < 0 {
		return nil, false
	}
	if index, present := patches.byFactor[output]; present {
		if index < 0 || index >= len(patches.entries) {
			return nil, false
		}
		entry, ok := patches.entries[index].factor.(*typedExecutionPatch[K, V])
		if !ok || entry.output != output || entry.patch == nil {
			return nil, false
		}
		return entry.patch, true
	}
	return installExecutionPatch(patches, output, patches.base, false)
}

func executionCarry[K ~uint64, V any](patches *executionPatches, output *Factor[K, V], input facts.Facts) bool {
	if patches == nil || patches.closed || output == nil || output.slot < 0 {
		return false
	}
	if index, present := patches.byFactor[output]; present {
		if index < 0 || index >= len(patches.entries) {
			return false
		}
		patches.entries[index].discard()
		patches.entries = append(patches.entries[:index], patches.entries[index+1:]...)
		delete(patches.byFactor, output)
		for position := index; position < len(patches.entries); position++ {
			patches.byFactor[patches.entries[position].key] = position
		}
	}
	_, ok := installExecutionPatch(patches, output, input, true)
	return ok
}

type typedExecutionPatch[K ~uint64, V any] struct {
	output *Factor[K, V]
	patch  *factbinding.Patch[K, V]
}

func installExecutionPatch[K ~uint64, V any](patches *executionPatches, output *Factor[K, V], input facts.Facts, carried bool) (*factbinding.Patch[K, V], bool) {
	if patches == nil || patches.closed || output == nil || output.slot < 0 || output.binding.Bound() == false {
		return nil, false
	}
	var patch *factbinding.Patch[K, V]
	if carried {
		patch = output.binding.BeginPatchFrom(patches.base, input)
	} else {
		patch = output.binding.BeginPatch(patches.base)
	}
	if patch == nil {
		return nil, false
	}
	typed := &typedExecutionPatch[K, V]{output: output, patch: patch}
	entry := executionPatchEntry{
		slot:   output.slot,
		key:    output,
		factor: typed,
		discard: func() {
			if typed.patch != nil {
				typed.patch.Discard()
				typed.patch = nil
			}
		},
		commit: func(base facts.Facts) (facts.Facts, bool, bool) {
			if typed.patch == nil {
				return facts.Facts{}, false, false
			}
			next, changed, ok := typed.patch.Accept()
			typed.patch = nil
			if !ok {
				return facts.Facts{}, false, false
			}
			plane, ok := output.binding.Plane(next)
			if !ok {
				return facts.Facts{}, false, false
			}
			next, ok = output.binding.Put(base, plane)
			return next, changed, ok
		},
	}
	patches.byFactor[output] = len(patches.entries)
	patches.entries = append(patches.entries, entry)
	return patch, true
}

// commit accepts every typed staged plane only after every Rule callback for
// the row succeeded. Reattachment is sorted by sealed Factor order, never map
// iteration. changed is the OR of stage's exact semantic deltas.
func (patches *executionPatches) commit() (facts.Facts, bool, bool) {
	if patches == nil || patches.closed {
		return facts.Facts{}, false, false
	}
	patches.closed = true
	defer clear(patches.byFactor)
	if len(patches.entries) == 0 {
		return patches.base, false, true
	}
	sort.Slice(patches.entries, func(left, right int) bool { return patches.entries[left].slot < patches.entries[right].slot })
	next := patches.base
	changed := false
	for index := range patches.entries {
		entry := &patches.entries[index]
		value, delta, ok := entry.commit(next)
		if !ok {
			for rest := index + 1; rest < len(patches.entries); rest++ {
				patches.entries[rest].discard()
			}
			clear(patches.entries)
			return facts.Facts{}, false, false
		}
		next, changed = value, changed || delta
	}
	clear(patches.entries)
	return next, changed, true
}

func (patches *executionPatches) discard() {
	if patches == nil || patches.closed {
		return
	}
	patches.closed = true
	for index := range patches.entries {
		patches.entries[index].discard()
	}
	clear(patches.entries)
	clear(patches.byFactor)
}
