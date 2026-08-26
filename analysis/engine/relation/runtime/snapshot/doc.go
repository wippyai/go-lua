// Package snapshot publishes the replacement relation runtime through the
// canonical analysis/snapshot store.
//
// The package is deliberately a projection boundary: terminal.Result.Root is
// the only relation fact authority, and analysis/snapshot.Snapshot is the
// only immutable published store.  This package owns neither a relation
// database nor a second value representation.  It only declares the stable
// logical row/column axes used to address the canonical publication.
package snapshot
