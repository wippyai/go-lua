// Package placement owns the memory-placement vocabulary of the analyzer: the
// escape dispositions a value may undergo, the placement chain those
// dispositions force, and the manifest projections both are serialized under.
//
// What the domain owns:
//
//   - Escape, its frozen manifest spelling, and the total Escape-to-Placement
//     projection.
//   - Placement, its chain bottom < stack < owned-heap < shared-heap < unknown,
//     and the lattice that chain is ordered by. Interpreter and Register are
//     JIT-only spellings outside the chain and are rejected at the axis
//     boundary.
//   - Consequence, Event, and Boundary: the frozen wire labels of the manifest
//     placement consequence, the placement/event fact stream, and the
//     placement/contract fact stream.
//   - EvidenceState and AllocationEvidence: Placement-owned proof records for
//     facts derived from the solved factor and authenticated allocation roots.
//
// # Declarative analysis model
//
// Placement is a domain-owned factor, not a handwritten Result projection.
// Its coordinate is one Heap-issued allocation root. Heap's root universe
// already distinguishes Program allocation occurrences and Target fresh-result
// allocations, so Placement does not mint a second site identity. A field is
// not an allocation unit: a referenced payload has its own allocation root,
// while containment propagates an applicable escape through the graph. This is
// what permits a local holder and a shared payload to retain different results.
//
// Runtime process/actor/shared/thread contexts and mounted substitutions are
// evidence about an allocation, not extra Placement coordinates. Alternate
// paths, calls, mounts, and contexts contribute demands to the same root and
// the factor joins those demands in the placement chain. Thus a variation may
// displace a site to a more conservative class but can never overwrite a
// different variation or move the site downward.
//
// The domain owns the complete declaration: axis and factor algebra, seed and
// transfer rules, query fold, frozen result codec, and public result facade.
// The composite package only registers those declarations. The generic engine
// may expose typed allocation, relation, use, boundary, and context facts that
// any domain can consume; it must not contain stack/heap policy, Placement
// switches, fixture buckets, or a Placement-specific result lane.
//
// # Current integration state
//
// The Placement axis, allocation-root factor, Program-allocation seed,
// monotone displacement rules, heterogeneous Placement+Heap query, frozen
// summary codec, and typed publication facade are registered through the
// composite declaration. The mounted census includes containment, storage,
// returns, captures, suspension, declaration-level context transfer, and
// formal library effects. Runtime actor/thread delivery identity is a separate
// owner-authenticated input: until that owner publishes it, transfer remains
// Unknown rather than pretending a static declaration proves a live actor
// relationship. Placement is therefore a real domain-owned result surface,
// not a handwritten field on the engine's generic Result.
//
// Heap also declares Target fresh-result roots. The Heap denominator admits a
// fresh row only after the canonical mounted Program CallResult authenticates
// the Project application and Target (OutcomeResultID, ordinal) admission.
// Placement's separate Link-scoped fresh-root seed then gives each such
// retained root exactly one unconditional Stack seed, while the query labels
// its owner-authenticated origin as manifest.allocation. Missing or opaque
// CallResult evidence never enters that denominator and is not silently
// converted into Stack.
package placement
