// Package boot owns the immutable initial environment ledger of a target.
//
// The owner is intentionally independent from target's operation builder. A
// boot Table is compiled only from authoring values, exact-key coordinates,
// and a small immutable operation binding map. It never receives an operation
// draft, Contract, or append callback.
package boot

import (
	"github.com/wippyai/go-lua/analysis/identity"
	sealedrows "github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Input is the complete immutable input to Compile. ExactKeys is the target
// key coordinate pool issued before boot compilation. Its index is the dense
// ExactKey handle minus one.
type Input struct {
	InitialRoots      []vocabulary.InitialRootSpec
	InitialEntries    []vocabulary.InitialEntrySpec
	InitialBindings   []vocabulary.InitialBindingSpec
	InitialMetatables []vocabulary.InitialMetatableAttachmentSpec
	Operations        operation.Core
	Keys              exactkey.Table
}

type rootRow struct {
	identity string
	shape    vocabulary.BootShape
}

type shapeRow struct {
	root      vocabulary.InitialRoot
	aggregate vocabulary.BootAggregate
	immutable bool
	value     vocabulary.InitialValue
}

type valueRow struct {
	kind      vocabulary.InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      vocabulary.InitialRoot
	operation vocabulary.Operation
	binding   uint32
}

type entryRow struct {
	root       vocabulary.InitialRoot
	key        vocabulary.ExactKey
	value      vocabulary.InitialValue
	mutability vocabulary.InitialMutability
}

type bindingRow struct {
	name string
	root vocabulary.InitialRoot
	key  vocabulary.ExactKey
}

type metatableRow struct {
	base      vocabulary.InitialValueKind
	metatable vocabulary.InitialRoot
}

// bindingRange is semantic denied-binding geometry. Its windows are minted
// by the shared PoolBuilders during resolution and can only be read through
// Pool.At/Count after the owner seals.
type bindingRange struct {
	namespace  vocabulary.BindingNamespace
	ownerKeys  sealedrows.Span
	memberKeys sealedrows.Span
}

// Table is the immutable boot/environment value composed into target.Contract.
// All row storage and canonical identity columns belong to this owner.
type Table struct {
	roots       sealedrows.Rows[rootRow]
	shapes      sealedrows.Rows[shapeRow]
	values      sealedrows.Rows[valueRow]
	valueBinds  sealedrows.Rows[bindingRange]
	keys        exactkey.Table
	bindingKeys sealedrows.Pool[vocabulary.ExactKey]
	entries     sealedrows.Rows[entryRow]
	bindings    sealedrows.Rows[bindingRow]
	metatables  sealedrows.Rows[metatableRow]
	globalRoot  vocabulary.InitialRoot
	absent      vocabulary.InitialValue
	valueIDs    []identity.ContentID
	bootID      identity.ContentID
}
