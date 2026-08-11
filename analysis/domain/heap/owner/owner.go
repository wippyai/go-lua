// Package owner declares Heap's Link-scoped semantic family to the cold
// engine composition. Heap keeps all root, slot, payload, and value algebra;
// this package only connects that proved algebra to one homogeneous Factor.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/engine"
)

// coordinate is private carrier layout. A Heap key remains the exact Link
// Heap root (AllocationRoot or BootRoot) issued by heap.Schema; this ordinal
// is never exposed to a Rule or retained in a persisted semantic result.
type coordinate uint32

// Owner retains Heap's semantic authority and the only cold capabilities that
// a later Rule declaration may need. It does not cache Units, Targets, or a
// key-to-carrier map: those are exact Program-template products of Wave E.
type Owner struct {
	schema  heap.Schema
	rank    heap.WidenRank
	factor  *engine.Factor[coordinate, heap.Value]
	locator map[heap.Key]coordinate

	output engine.Output[heap.Value]
	read   engine.ReadForm[heap.Value, engine.OrderedCells[heap.Value]]
	write  engine.WriteForm[heap.Value]
	carry  engine.CarryForm
}

// Declare binds Heap's already-sealed key family to exactly one cold Factor.
// The declaration does not allocate a carrier, choose a Program point, or
// issue a concrete fact handle. An empty root universe is lawful: the
// universal Default-admission law is then vacuous.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema heap.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() {
		return nil, false
	}
	rank, ok := heap.NewWidenRank(schema)
	if !ok {
		return nil, false
	}
	domain := schema.Domain()
	defaultValue := schema.Default()
	if !domain.Equal(defaultValue, defaultValue) {
		return nil, false
	}
	locator, ok := locateKeys(schema)
	if !ok {
		return nil, false
	}
	owner := &Owner{schema: schema, rank: rank, locator: locator}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, heap.Value]{
		Semantic: semantic,
		KeyEnd:   uint64(schema.KeyCount()),
		Lattice:  domain,
		Default:  defaultValue,
		AdmitAt:  owner.admits,
		Fingerprint: func(value heap.Value) uint64 {
			fingerprint, valid := schema.Fingerprint(value)
			if !valid {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, heap.Value]{
			Width: rank.Width(),
			At:    owner.widenRank,
		},
	}, func(factor *engine.Factor[coordinate, heap.Value]) bool {
		read, readOK := engine.ExactReadForm(factor)
		write, writeOK := engine.ExactWriteForm(factor)
		carry, carryOK := engine.Carry(factor)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output = factor.Output()
		owner.read = read
		owner.write = write
		owner.carry = carry
		return true
	})
	if !ok || factor == nil {
		return nil, false
	}
	// Keep the typed issuer only after its declaration callback has closed.
	// Locate still cannot issue a capability until the Composition seals.
	owner.factor = factor
	return owner, true
}

// Schema exposes Heap's immutable semantic algebra. It is the authority for
// root/slot/payload incidence and must not be reconstructed by engine code.
func (owner *Owner) Schema() heap.Schema {
	if owner == nil {
		return heap.Schema{}
	}
	return owner.schema
}

// Output is the sole key-erased output capability for Heap Rules.
func (owner *Owner) Output() engine.Output[heap.Value] {
	if owner == nil {
		return engine.Output[heap.Value]{}
	}
	return owner.output
}

// ExactRead is the owner-issued exact observation shape. Wave E binds its
// actual schema Key at a Program point; callers cannot manufacture an ordinal.
func (owner *Owner) ExactRead() engine.ReadForm[heap.Value, engine.OrderedCells[heap.Value]] {
	if owner == nil {
		return engine.ReadForm[heap.Value, engine.OrderedCells[heap.Value]]{}
	}
	return owner.read
}

// ExactWrite is the owner-issued exact output shape. Wave E alone supplies
// the schema Key and concrete target.
func (owner *Owner) ExactWrite() engine.WriteForm[heap.Value] {
	if owner == nil {
		return engine.WriteForm[heap.Value]{}
	}
	return owner.write
}

// Carry is the explicit whole-Heap predecessor relation. It is distinct from
// exact reads so a Rule cannot turn a root-specific observation into a
// whole-factor fallback.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Heap's exact composition-local coordinate capability for one
// canonical Schema key. The precomputed Schema-order table is the only owner
// reverse lookup, so no ordinal or alternate semantic plane is exposed.
func (owner *Owner) Locate(key heap.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.locator[key]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func locateKeys(schema heap.Schema) (map[heap.Key]coordinate, bool) {
	keys := make(map[heap.Key]coordinate, schema.KeyCount())
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if !ok {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := keys[key]; duplicate {
			return nil, false
		}
		keys[key] = coordinate
	}
	return keys, true
}

func (owner *Owner) keyAt(index coordinate) (heap.Key, bool) {
	if owner == nil || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return heap.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

// admits is Heap's coordinate-specific value fence, transferred unchanged to
// factbinding at runtime. The engine has no second source of root incidence.
func (owner *Owner) admits(index coordinate, value heap.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.schema.Admits(key, value)
}

func (owner *Owner) widenRank(index coordinate, value heap.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	return owner.rank.At(key, value, component)
}
