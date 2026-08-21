// Package publication owns detached publication observations that are consumed
// by the public result projection. Runtime-allocation and direct-membership
// scaffolds were removed because clean production reachability had no consumer;
// a future implementation must enter through the canonical Snapshot schema,
// not recreate those private cross-domain transports here.
package publication
