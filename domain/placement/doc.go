// Package placement owns the memory-placement vocabulary of the analyzer: the
// escape dispositions a value may undergo, the placement chain those
// dispositions force, the manifest projections both are serialized under, and
// the tri-state allocation-site license record a placement decision is proved
// from.
//
// What the domain owns:
//
//   - Escape, its frozen manifest spelling, and the total Escape-to-Placement
//     projection.
//   - Placement, its chain bottom < stack < owned-heap < shared-heap < unknown,
//     and the lattice that chain is ordered by. Interpreter and Register are
//     JIT-only spellings outside the chain and rank conservatively.
//   - Consequence, Event, and Boundary: the frozen wire labels of the manifest
//     placement consequence, the placement/event fact stream, and the
//     placement/contract fact stream.
//   - LicenseState and AllocationSiteLicenses: the must-proof evidence algebra
//     an allocation site's licenses are joined under, and its total boolean
//     projection.
//
// # Declaration surfaces
//
// The domain declares nothing on the analyzer declaration table
// (analysis/schema), and has no registration of its own. Every surface is
// blocked on the same missing thing rather than on a declaration that was
// merely not written:
//
// An axis entry requires a writer principal, and a principal is the
// Spec.Writes key of the rule that owns the factor lane.
// There is no placement rule role and therefore no placement factor lane in
// the artifact, so an axis entry here would have to mint a serialized ABI
// ordinal for a factor nothing writes. The lattice this package publishes is
// an algebra without a coordinate space: no engine.HotFactorSpec is built over
// Placement anywhere, so axis.Adopt has nothing to adopt.
//
// A rule entry, a composite entry, a denominator, and a query registration all
// resolve against a declared axis. With no placement axis, each would name a
// coordinate space that does not exist.
//
// The structural vocabulary surface declares the arm, event, and outcome
// catalogs of control flow. This package's Event and Boundary labels are
// placement fact labels rather than bracket-stream events, so declaring them
// there would break the density of the catalog they are not members of.
//
// No diagnostic publishes a placement code, and no library or environment
// contract carries a placement form.
//
// # The unavailable placement plane
//
// The consequence is visible at the analyzer root: Result publishes no
// placement surface and its identity format writes the plane as absent
// (analysis/analyze.go), the heap
// domain names the reason as PlacementFactorsUnbound
// (analysis/domain/heap/runtime_allocation_context.go), and the corpus carries
// placement expectations the analyzer answers as Unsupported. The plane opens
// when the closed semantic transfer over Value, Heap, Residence, Footprint,
// and Effect converges into a placement factor; the axis entry lands with that
// factor, not before it.
package placement
