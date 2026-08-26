package relation

// GapState records whether a Placement declaration already has a native,
// typed relation binding. The inventory is intentionally explicit: an absent
// adapter is not an invitation for the engine to decode a domain payload or
// to substitute a legacy form.
type GapState uint8

const (
	GapBound GapState = iota + 1
	GapNeedsForeignColumns
	GapNeedsRouteAdapter
	GapNeedsVectorAdapter
	GapSeparateAxis
)

// SignatureGap names one authored Placement signature and the sealed owner
// boundary still required before it can bind. Family is a declaration family,
// not a fallback dispatch key.
type SignatureGap struct {
	Family string
	State  GapState
	Reason string
}

// Gaps is the complete Placement declaration inventory at this bridge
// boundary. It returns a fresh slice so callers cannot mutate the canonical
// inventory. Allocation birth is bound directly through Value's issued
// allocation candidate and fresh fact plus Placement's Fact column.
func Gaps() []SignatureGap {
	return []SignatureGap{
		{Family: "placement/allocationbirth", State: GapBound, Reason: "Value's issued allocation candidate and fresh fact bind to Placement's initial Fact"},
		{Family: "placement/capture", State: GapBound, Reason: "Heap's issued route key and Placement's route tag/facts bind the authored closure-capture fold"},
		{Family: "placement/containment", State: GapBound, Reason: "ContainmentRoutes derives the complete Placement and Heap vectors once and retains the authenticated parent Fact beside the selected child Fact"},
		{Family: "placement/formal", State: GapBound, Reason: "Placement-owned route tag and selected Fact columns bind its scalar formal fold"},
		{Family: "placement/freshbirth", State: GapBound, Reason: "Value's issued fresh-result candidate and result fact bind to Placement's initial Fact"},
		{Family: "placement/publicationescape", State: GapBound, Reason: "Placement-owned Requirement and Fact columns bind its publication escape fold"},
		{Family: "placement/returnescape", State: GapBound, Reason: "Placement-owned route tag and Fact columns bind its return escape fold"},
		{Family: "placement/store", State: GapBound, Reason: "Value's opaque storage-transfer candidate and source codecs plus Placement's route tag and Fact columns bind the Store fold"},
		{Family: "placement/suspension", State: GapBound, Reason: "SuspensionRoutes is the sole complete-Value consumer; it retains SourceSummary beside its owner-issued route key/tag and selected Placement Fact for the scalar fold"},
		{Family: "placement/suspension-evidence", State: GapBound, Reason: "canonical SuspensionRoutes consumes the complete Value vector once; Evidence consumes its owner-issued SourceSummary scalar and binds only the independent Evidence Factor"},
		{Family: "placement/transfer", State: GapBound, Reason: "Placement-owned route tag and Fact columns bind its transfer fold"},
	}
}
