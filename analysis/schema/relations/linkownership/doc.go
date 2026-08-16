// Package linkownership is the cold Wave-B ownership inventory generator for
// program/link.
//
// The authored four-file ledger and generated snapshot are intentionally absent
// while the Link owner DAG is being re-authored. Run fails closed on that
// absence; no partial or historical rows are accepted as evidence. The live
// Link package does not import this package, and no generated row is a runtime
// registry or semantic dispatch table.
package linkownership

//go:generate go run ./cmd -root ../../../.. -mode inventory
