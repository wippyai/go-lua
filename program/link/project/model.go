// Package project owns Link's canonical authored module mounts and the
// derived exact-key quotient.  It deliberately has no dependency on Link's
// runtime geometry, static resolver, or analysis domains.
package project

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

// Module is one authored source module. Build establishes its canonical
// Shard order from Program content identity followed by exact Name.
type Module struct {
	Name    string
	Program *program.Program
}

// Input is the complete Project build boundary.
type Input struct {
	Modules []Module
	Target  *target.Contract
}

type mountRow struct {
	name    string
	program *program.Program
	id      keyspace.ContentID
}

type keyRow struct{ value keyspace.LiteralValue }

// authority is shared by the construction-only Draft and final Component.
// It is immutable after Build; Finalize only fences construction access.
type authority struct {
	// target is retained as the exact immutable Target authority.  A digest
	// alone cannot fence same-ordinal Target coordinates from another
	// contract, while Project's ForTarget/ForInitial mappings need that fence.
	target          *target.Contract
	contentID       keyspace.ContentID
	semanticReceipt SemanticSourceReceipt
	// mountContentID and applicationContentID are relation-local semantic
	// digests.  They deliberately exclude the enclosing Link identity (and,
	// for mounts/applications, the unrelated Target/dependent relations), so
	// the identities issued by Project cannot widen their dependency closure.
	mountContentID       keyspace.ContentID
	applicationContentID keyspace.ContentID
	mounts               []mountRow
	keys                 []keyRow
	targetKeys           []uint32
	// targetKeyByProject is the owner-local inverse of targetKeys.  It is a
	// derived quotient index, not another key identity: a zero row means that
	// the Project key was authored only by Program/source data and is not a
	// Target exact key.
	targetKeyByProject       []target.ExactKey
	initialKeys              map[target.InitialValue]uint32
	programKeys              [][]uint32
	applications             []applicationRow
	baseApplications         []uint32
	callApplications         []uint32
	callApplicationsBySource map[callSource]uint32
	importApplications       []uint32
}

// draftState is deliberately shared by Draft copies. A construction view must
// not remain usable through a value copy after any copy finalizes it.
type draftState struct {
	authority *authority
	consumed  bool
	fence     *draftFence
}

// draftFence is the only construction state retained by a Cold snapshot. It
// carries no semantic authority or Program pointer; its sole purpose is to
// invalidate a Cold obtained from Draft after Finalize consumes that Draft.
type draftFence struct{ consumed bool }

// Draft exposes source-only read views while Link is being assembled.
type Draft struct{ state *draftState }

// Component is the immutable owner-fenced Project published by Link.
type Component struct {
	authority *authority
}

// Mounts owns canonical Program mount coordinates.
type Mounts struct {
	authority *authority
	draft     *draftState
}

// Keys owns Link's exact Lua-key quotient and its direct source mappings.
type Keys struct {
	authority *authority
	draft     *draftState
}

// Applications owns finite executable Program application occurrences. It
// carries no target operation or lazy-resume vocabulary: neither is a source
// relation and no producer exists for it.
type Applications struct {
	authority *authority
	draft     *draftState
}

// Application is an opaque owner-fenced static project occurrence.
type Application struct {
	authority *authority
	ordinal   uint32
}

// Shard is the owner-fenced Project mount coordinate.  Its dense ordinal is
// intentionally private: consumers must retain the exact Project authority
// and use Mounts.Index/At to cross the storage boundary.
type Shard struct {
	authority *authority
	ordinal   uint32
}

// Key is the owner-fenced Project exact-key coordinate.  It cannot be
// confused with a Program key or a coordinate from another Project.
type Key struct {
	authority *authority
	ordinal   uint32
}

// Calls and Imports are typed existing-application subsequences.
type Calls struct {
	authority *authority
	draft     *draftState
}
type Imports struct {
	authority *authority
	draft     *draftState
}

// Operators exposes the exact Program function-style source alternatives.
// Distinct methods preserve primary/fallback order applications without a
// public kind union.
type Operators struct {
	authority *authority
	draft     *draftState
}
type Bases struct {
	authority *authority
	draft     *draftState
}

// Cold is a detached identity snapshot. It is deliberately not a view over
// the hot Project authority: no Project pointer, Program pointer, or module
// accessor crosses this boundary. Consumers that need authored module rows
// use Component.Mounts, while Link identity/dependency assembly consumes only
// these scalar relation identities.
type Cold struct {
	targetID  keyspace.ContentID
	contentID keyspace.ContentID
	// semanticReceipt is the detached owner-bound source receipt. It contains
	// only typed row identities and the exact Project owner identity.
	semanticReceipt SemanticSourceReceipt
	// draft is a construction-only lifecycle fence. It is not a semantic
	// authority or a Program pointer and is nil on Component snapshots.
	fence *draftFence
}

type applicationKind uint8

const (
	applicationCall applicationKind = iota + 1
	applicationImport
	applicationMeta
	applicationGeneric
)

type applicationSlot uint8

const (
	applicationSlotNone applicationSlot = iota
	applicationSlotUnaryNumeric
	applicationSlotLength
	applicationSlotArithmetic
	applicationSlotBitwise
	applicationSlotConcat
	applicationSlotEquality
	applicationSlotOrderPrimary
	applicationSlotOrderFallback
	applicationSlotIndexGet
	applicationSlotIndexSet
)

type applicationKey struct {
	kind  applicationKind
	shard uint32
	term  keyspace.Term
	slot  applicationSlot
}

// callSource is the owner-local inverse key for one executable Program Call.
// Its dense shard ordinal is meaningful only with the exact authority that
// owns the map; the public query validates that authority before consulting
// it. Keeping the inverse keyed by the source occurrence avoids a Project-
// wide scan while retaining Application as the sole issued identity.
type callSource struct {
	shard uint32
	term  keyspace.Term
}

type applicationRow struct {
	kind  applicationKind
	shard uint32
	term  keyspace.Term
	slot  applicationSlot
	root  uint32
}
