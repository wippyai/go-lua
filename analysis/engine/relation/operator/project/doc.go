// Package project owns tuple projection into a mounted destination relation.
//
// Mount supplies the target and key layouts. Runtime resolves destination
// rows through the target Reader and tuple combinators carry the resulting
// owner-issued identity, scope, and lineage. Project never publishes state or
// hides a regrouping operation: a result slice is only ordered transport of
// exact output-scope partitions from one input range.
package project
