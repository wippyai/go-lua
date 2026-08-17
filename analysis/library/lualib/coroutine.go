package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// CoroutineRoot is the authored mount selector of the coroutine library.
const CoroutineRoot = "coroutine"

// coroutineExports is the authored export inventory of the coroutine library, in
// canonical order.
//
// What a coroutine export does to control flow - the point at which control
// leaves and may return - is the suspension form's business, and this library is
// the one that has such members. What the contract states besides is the typed
// application of each export and the ownership transfer its effect row carries:
// creating, resuming and wrapping all send a value across a control boundary,
// and the envelope says which argument position is sent.
var coroutineExports = []string{
	"close", "create", "isyieldable", "resume", "running", "spawn", "status",
	"wrap", "yield",
}

// CoroutineExports returns a copy of the authored export inventory.
func CoroutineExports() []string { return copyNames(coroutineExports) }

// The outcome cases a suspension relates. They are the authored keys of the
// sealed structural outcome vocabulary, resolved by a reader against that
// surface: a suspension names the case control leaves at and the case it
// re-enters at, and neither is an ordinal this library minted.
const (
	outcomeNormal = "outcome/normal"
	outcomeYield  = "outcome/yield"
)

// coroutineSuspensions are the coroutine exports that suspend.
//
// coroutine.yield leaves control at the yield outcome and re-enters at the
// normal one, restored by the call that resumed it, and one live suspension is
// discharged by that first restoration. The relation is the whole of what a
// suspension is; the values carried across the boundary are the callable
// envelope's business and are not restated here.
//
// coroutine.spawn leaves control at the same yield outcome and re-enters at the
// same normal one, and the two rows differ in the authority that restores it: a
// spawned activation is detached, so nothing the caller writes resumes it and
// the provider is what brings control back. One live suspension is discharged by
// that first restoration in both cases.
var coroutineSuspensions = map[string]contract.Suspension{
	"spawn": {
		Yield:        outcomeYield,
		Reentry:      outcomeNormal,
		Source:       contract.ReentryByProvider,
		Multiplicity: contract.ReentryOnce,
	},
	"yield": {
		Yield:        outcomeYield,
		Reentry:      outcomeNormal,
		Source:       contract.ReentryByCall,
		Multiplicity: contract.ReentryOnce,
	},
}

// CoroutineContract authors the coroutine library contract instance against one
// declared library kind. The root carries what the library is: a mutable
// aggregate. Nothing is deferred.
func CoroutineContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:        CoroutineRoot,
		Exports:     coroutineExports,
		Signatures:  coroutineSignatures,
		Aggregate:   contract.Aggregate(contract.MutabilityMutable),
		Suspensions: coroutineSuspensions,
	}.instance(kind)
}
