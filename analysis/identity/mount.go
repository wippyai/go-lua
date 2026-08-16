package identity

// MountID is the identity of one mounted Program instance within a Link. A
// Program mounted twice in one Link is two mounts with two MountIDs; the same
// Program mounted at the same coordinate in an identically assembled Link is
// one MountID.
//
// A MountID is not a Program identity. It answers which mount a fact came
// from, so facts derived under different mounts of the same Program never
// merge and never need a mount-aware comparison at the consumer.
//
// The Link owns what distinguishes one mount from another and derives the
// value through the domain-separated digest construction; this package
// carries and compares it. Its width matches the analysis-wide full digest
// width, so a derived digest becomes a MountID by conversion and never by
// truncation or re-hashing.
type MountID [32]byte

// Available reports whether id names a mount. The zero MountID names none:
// every failure path in the Link collapses to it, so a consumer never has to
// distinguish "not mounted yet" from "mount derivation failed".
//
// The loop is over a fixed width with no bounds beyond the array, so it costs
// a constant unrolled scan and no allocation on the carry path.
func (id MountID) Available() bool {
	var bits byte
	for _, value := range id {
		bits |= value
	}
	return bits != 0
}
