// Package position seals the sparse source-position projection owned by Flow.
//
// Position is deliberately a terminal projection: it consumes the authored
// Source order, the sealed Body and containment proofs, and the sealed
// Outcome relation, then emits one typed Source batch.  It does not retain a
// graph, reconstruct containment, or publish a second source authority.
package position
