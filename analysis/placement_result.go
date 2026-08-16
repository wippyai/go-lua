package analysis

// placementResultReceipt deliberately has no public rows yet. A solved
// placement projection requires the closed semantic transfer over Value,
// Heap, Residence, Footprint, and Effect receipts; structural allocation or
// runtime-context receipts are not evidence of stack, owned, or shared
// placement. Until that transfer is bound, Result treats this plane as
// unavailable and the corpus oracle must report Unsupported.
//
// Keeping the distinct marker type lets the shared Result receipt directory
// preserve the plane boundary without conflating it with native publication or
// creating zero-row/false-clean placement handles.
type placementResultReceipt struct{}

func (receipt *placementResultReceipt) valid() bool { return false }
