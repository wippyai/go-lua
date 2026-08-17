package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// OSRoot is the authored mount selector of the os library.
const OSRoot = "os"

// osExports is the authored export inventory of the os library, in canonical
// order.
//
// These exports cross the host boundary, and the envelope of each carries what
// its own effect row says about that crossing. os.clock is the case that shows
// why the row is carried rather than inferred: its result type is exact and its
// effect row is deliberately open, because the absence of a label is not a proof
// that a host call is a closed operation.
var osExports = []string{
	"clock", "date", "difftime", "execute", "exit", "getenv", "remove",
	"rename", "time", "tmpname",
}

// OSExports returns a copy of the authored export inventory.
func OSExports() []string { return copyNames(osExports) }

// OSContract authors the os library contract instance against one declared
// library kind. The root carries what the library is: a mutable aggregate.
// Nothing is deferred.
//
// The aggregate is published mutable although a host may well seal it. Sealing
// the os table is a policy of the initial environment a host boots, carried by
// the boot-root and denied-entry forms that only the environment class may
// declare; a library that published its own aggregate sealed would be stating
// that environment fact from inside a library contract.
func OSContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       OSRoot,
		Exports:    osExports,
		Signatures: osSignatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
	}.instance(kind)
}
