package lualib

import (
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// ErrorsRoot is the authored mount selector of the errors library.
const ErrorsRoot = "errors"

// ErrorFamilyKey is the key the errors library publishes its error family under.
// The value it names is the metatable every error object carries.
const ErrorFamilyKey = "Error"

// ErrorMethodKey is the key the error family indexes its methods through. An
// error object reaches `err:kind()` by way of it, which is what makes the method
// a member of the family and not a second export of the library root.
const ErrorMethodKey = "__index"

// The error family is authored as nested exported values, and the reason is what
// the ledger holds rather than a preference. A primitive metatable is a value no
// export path reaches - the string primitive's metatable is attached to a base
// type and is stated by the attachment form for exactly that reason - and the
// error family is the opposite shape: the metatable IS an exported value of the
// errors aggregate, its metamethods are its own entries, and the method table is
// the entry it publishes under __index. Every one of them is reached by walking
// exported values from the library root, so each has an address, and a member
// with an address is published at it.
//
// The ledger attaches one metatable to a base type, and it is the string
// primitive's. No error attachment exists there and none is authored here: an
// error object is produced by a call rather than written as a literal, so no base
// type carries the family and the attachment form has nothing to say about it.

// errorsExports is the authored export inventory of the errors library, in
// canonical order. Each name is one direct callable export of the contract root.
var errorsExports = []string{"is", "new", "wrap"}

// errorsValues are the exported values of the errors library that are not
// callables, in authored order: the two aggregates the export graph continues
// through, then the kind constants that terminate the paths they are reached by.
//
// Every one is published mutable, which is the library's own statement about its
// export and deliberately not the host's. A Wippy host boots this whole aggregate
// frozen, and that seal is a fact about the initial environment carried by the
// boot-root form, which only the environment class may declare.
var errorsValues = []valueExport{
	aggregateExport(exportPath(ErrorFamilyKey), contract.MutabilityMutable),
	aggregateExport(exportPath(ErrorFamilyKey, ErrorMethodKey), contract.MutabilityMutable),

	errorKind("ALREADY_EXISTS", "AlreadyExists"),
	errorKind("CANCELED", "Canceled"),
	errorKind("CONFLICT", "Conflict"),
	errorKind("INTERNAL", "Internal"),
	errorKind("INVALID", "Invalid"),
	errorKind("NOT_FOUND", "NotFound"),
	errorKind("PERMISSION_DENIED", "PermissionDenied"),
	errorKind("RATE_LIMITED", "RateLimited"),
	errorKind("TIMEOUT", "Timeout"),
	errorKind("UNAVAILABLE", "Unavailable"),
	// The unknown kind is the empty string, and it is published as the value it
	// is rather than as an absent member: a program that reads errors.UNKNOWN
	// gets a string back, and a contract that omitted it would say it gets
	// nothing.
	errorKind("UNKNOWN", ""),
}

// errorKind publishes one error-kind constant.
func errorKind(key, value string) valueExport {
	return constantExport(key,
		contract.Constant{Kind: contract.ConstantString, String: value},
		contract.MutabilityMutable)
}

// ErrorsExports returns a copy of the authored export inventory.
func ErrorsExports() []string { return copyNames(errorsExports) }

// ErrorsContract authors the errors library contract instance against one
// declared library kind. The root callables carry their typed application
// envelope, the family metamethods and error methods carry theirs at the address
// they are reached by, the kind constants carry the values they are, and the root
// carries what the library is: a mutable aggregate. Nothing is deferred.
func ErrorsContract(kind *library.Entry) (*contract.Instance, bool) {
	return librarySpec{
		Root:       ErrorsRoot,
		Exports:    errorsExports,
		Signatures: errorsSignatures,
		Aggregate:  contract.Aggregate(contract.MutabilityMutable),
		Values:     errorsValues,
		Methods:    errorsMethods,
	}.instance(kind)
}
