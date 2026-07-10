// Package service owns production checker sessions and immutable completed
// result snapshots.
//
// A WorkspaceSession is single-writer/multi-reader. Mutating calls and solves
// are serialized; query calls may run concurrently. CompletedResult values are
// immutable after publication and are safe to retain and share between
// goroutines. Accessors that return maps or slices return independent copies.
package service
