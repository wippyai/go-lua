package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// UTF8Root is the authored mount selector of the utf8 library.
const UTF8Root = "utf8"

// utf8Exports is the authored export inventory of the utf8 library, in canonical
// order. Each name is one direct export of the contract root.
//
// The library publishes no metatable edge. A string reaches its members through
// the string library's metatable, and utf8 is a second, separate aggregate over
// the same values rather than a second index edge on them.
var utf8Exports = []string{"char", "codepoint", "codes", "len", "offset"}

// utf8Constants is the one exported value of the utf8 library that is not a
// callable: the pattern that matches one UTF-8 sequence. It is a constant, so it
// terminates the path it is reached by, and it is written as its exact bytes -
// the pattern contains byte values no source spelling of a character reproduces.
var utf8Constants = []valueExport{
	constantExport("charpattern",
		contract.Constant{Kind: contract.ConstantString, String: "[\x00-\x7F\xC2-\xF4][\x80-\xBF]*"},
		contract.MutabilityMutable),
}

// UTF8Exports returns a copy of the authored export inventory.
func UTF8Exports() []string { return copyNames(utf8Exports) }

// UTF8Contract authors the utf8 library contract instance against one declared
// library kind. Every export is a callable and carries its typed application
// envelope, the published pattern carries the value it is, and the root carries
// what the library is: a mutable aggregate. Nothing is deferred.
func UTF8Contract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       UTF8Root,
		Exports:    utf8Exports,
		Signatures: utf8Signatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
		Values:     utf8Constants,
	}.instance(kind)
}
