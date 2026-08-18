// Package artifact is the portable persistence codec for a sealed Link. It is
// strictly downstream of the sealed Link: it reads Link only through the
// published child authorities and reopens one through the ordinary Seal, so
// Link never depends on how it is stored.
//
// The encoded stream carries only the identities an ordinary Seal needs to run
// again - target, claimed Link identity, canonical module rows, the Boundary
// endpoint requests, and the detached Host replay and module-cache contracts.
// Project relations, keys, sigma projections and caches stay Seal-derived and
// are absent from the wire, so the codec can never publish a second authority
// for a fact Link already owns.
//
// Opening is closed against hostile bytes. Decode admits every row against a
// fixed reconstruction ledger before it sizes any allocation, reseals through
// the same Link admission as authored construction, and accepts the result
// only when the replayed identity and the re-encoded bytes both agree with
// what was stored.
package artifact
