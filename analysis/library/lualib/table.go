package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// TableRoot is the authored mount selector of the table library.
const TableRoot = "table"

// tableExports is the authored export inventory of the table library, in
// canonical order.
//
// Several of these exports mutate their subject, and what they mutate is carried
// by the effect row of the member's own application envelope: the insertion, the
// length change and the ownership transfer are stated about the argument
// positions the callable declares, so they ride the value the contract attaches
// to rather than a name a consumer rebuilt.
// getn and maxn are the Lua 5.0 length members. The host boots them, so the
// contract that owns the table aggregate publishes them: a member the host
// supplies and no contract describes is a value a program can reach and the
// analyzer can say nothing about.
var tableExports = []string{
	"concat", "create", "freeze", "getn", "insert", "isfrozen", "maxn", "move",
	"pack", "remove", "sort", "unpack",
}

// TableExports returns a copy of the authored export inventory.
func TableExports() []string { return copyNames(tableExports) }

// TableContract authors the table library contract instance against one declared
// library kind. The root carries what the library is: a mutable aggregate.
// Nothing is deferred.
func TableContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       TableRoot,
		Exports:    tableExports,
		Signatures: tableSignatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
	}.instance(kind)
}
