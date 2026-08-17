// Package lualib holds the authored Lua library contract instances the
// analyzer ships with.
//
// An instance is data, not a declaration. The declaration table owns the
// contract KINDS - the member-form algebra, the codec, the addressing form -
// and owns no world; which library is mounted, which values it exports and what
// the contract says about them is instance data, external to the schema,
// injectable and configurable per link. This package is one such external
// source, and it is external in the way that matters: the kind is handed in,
// nothing here resolves a process-global table, and a host that ships a
// different string library publishes its own instance without editing a
// declaration.
package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// StringRoot is the authored mount selector of the string library. It is the
// one name a contract carries, it selects a mount during project construction,
// and no member address derives from it: every member below is addressed by the
// path of exported values from the contract root that selector resolved to.
const StringRoot = "string"

// StringMetatableIndexKey is the metatable key through which the string
// library publishes its members to string values. `s:upper()` reaches
// string.upper through this edge, which is why the colon-method form needs no
// name path of its own: the edge resolves to the contract root, and the member
// is an export of that root.
const StringMetatableIndexKey = "__index"

// stringExports is the authored export inventory of the string library, in
// canonical order. Each name is one direct export of the contract root, and
// each is addressed as a one-step export path from that root.
//
// The list is authored rather than derived. A contract instance that computed
// its own members from the table it is meant to replace would be that table
// wearing a second face, and would agree with it by construction instead of by
// check. The drift law derives the string signature table's expected content
// from this inventory and fails when the table diverges from it, which is the
// check that authored data earns.
var stringExports = []string{
	"byte", "char", "dump", "find", "format", "gfind", "gmatch", "gsub",
	"len", "lower", "match", "pack", "packsize", "rep", "reverse", "sub",
	"unpack", "upper",
}

// stringPatternDelegations are the exports whose result selection is driven by
// a caller literal: the pattern argument decides how many capture slots the
// result row has and what each carries. That computation cannot be enumerated
// as contract data, so the contract delegates it to the rule that owns it
// rather than pretending to carry it.
var stringPatternDelegations = []string{"gfind", "gmatch", "match"}

// stringDenials are the exports the string library declares and refuses to
// publish. string.dump serializes a function into a binary chunk, which the
// analyzer's target cannot load back, so the member exists and is refused. The
// contract is where that statement belongs: it owns the member, so a consumer
// that excludes the binding and a model that gives it a return type it can
// never reach both derive from this one row instead of each carrying a list of
// their own.
var stringDenials = []string{"dump"}

// stringRefinements are the result refinements the string library publishes.
// string.byte reads one character of its subject at a position, so its result is
// absent exactly when that position lies past the subject's end: a caller that
// has proved the subject at least as long as the position has discharged that
// optionality, and nothing else about the slot changes. Omitting the position
// reads the first character.
var stringRefinements = map[string]wire.ResultRefinement{
	"byte": wire.SubjectLengthRefinement{Result: 0, Subject: 0, Position: 1, Default: 1},
}

// StringExports returns a copy of the authored export inventory.
func StringExports() []string { return copyNames(stringExports) }

// StringContract authors the string library contract instance against one
// declared library kind.
//
// The callable members carry their typed application envelope, and the members
// whose result the contract refines carry that refinement: the layer that owns
// the type wire publishes both formats, so what this contract says about an
// exported value is serialized rather than promised. The refused member carries
// the address it refuses, which is all a denial has to say.
//
// One payload format remains deferred, and the reason is stated rather than
// hidden. The pattern delegations name a rule that the declaration table does not
// declare yet: the rule-delegation format landed, and no sealed rule owns
// pattern-capture result selection for a delegation to name, so those members
// carry their address and say so.
func StringContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:           StringRoot,
		Exports:        stringExports,
		Signatures:     stringSignatures,
		Aggregate:      contract.Aggregate(contract.MutabilityMutable),
		Refinements:    stringRefinements,
		Delegations:    stringPatternDelegations,
		Denials:        stringDenials,
		MetatableIndex: StringMetatableIndexKey,
	}.instance(kind)
}
