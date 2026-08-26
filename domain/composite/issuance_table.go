package composite

import (
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// issuanceEntries is the authored analyzer issuance inventory: the declarative
// construction machine every rule subscription resolves against, extended with
// the occurrence code families this analyzer's own domains publish.
//
// It is one function because it is one inventory. A rule's subscription is
// sealed against the issuance surface, so any composition that seals this
// analyzer's rules composes this exact machine; a second list would be a
// second analyzer.
func issuanceEntries() ([]*issuanceschema.Entry, bool) {
	return programissuance.Entries(
		programissuance.CodeFamily{Key: "occurrence/allocation-empty", Kind: programschema.OccurrenceAllocation, Code: uint64(heapdomain.AllocationFormEmpty)},
		programissuance.CodeFamily{Key: "occurrence/allocation-closed", Kind: programschema.OccurrenceAllocation, Code: uint64(heapdomain.AllocationFormClosed)},
	)
}
