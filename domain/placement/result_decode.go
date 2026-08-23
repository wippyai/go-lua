package placement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// SummaryResult is an immutable, allocation-free view over one encoded
// Placement summary. It retains only the caller's string and offsets into its
// fixed-width ID/state/evidence planes.
type SummaryResult struct {
	payload         string
	format          uint64
	allocationCount int
	idOffset        int
	stateOffset     int
	evidenceOffset  int
}

// SummaryResultIterator walks allocation rows in canonical ContentID order.
// Boot coordinates are not present in this public iterator.
type SummaryResultIterator struct {
	payload         string
	format          uint64
	allocationCount int
	idOffset        int
	stateOffset     int
	evidenceOffset  int
	index           int
}

// SummaryResultAllocation is one immutable allocation-row view. Its methods
// read directly from the retained wire string and do not allocate.
type SummaryResultAllocation struct {
	payload        string
	format         uint64
	idOffset       int
	stateOffset    int
	evidenceOffset int
}

// DecodeSummaryResult validates and opens one encoded Placement summary under
// the caller's exact expected Placement schema. A payload carrying another
// schema identity is rejected before its rows become available to a
// schema-bound consumer. There is deliberately no schema-less decoder: the
// Placement schema is the authority that authenticates the result identity.
func DecodeSummaryResult(expected Schema, present bool, rows uint64, payload string) (SummaryResult, bool) {
	if !expected.Valid() {
		return SummaryResult{}, false
	}
	return decodeSummaryResult(expected, present, rows, payload)
}

func decodeSummaryResult(expected Schema, present bool, rows uint64, payload string) (SummaryResult, bool) {
	if len(payload) < placementSummaryResultHeaderSize || rows > 1 {
		return SummaryResult{}, false
	}
	format := placementSummaryUint64(payload, 0)
	if format != placementSummaryResultFormat {
		return SummaryResult{}, false
	}
	if !placementSummaryIDAvailable(payload, 8) {
		return SummaryResult{}, false
	}
	expectedID := expected.ContentID()
	if !expectedID.Available() || !placementSummaryIDEquals(payload, 8, expectedID) {
		return SummaryResult{}, false
	}
	count64 := placementSummaryUint64(payload, 8+32)
	idOffset, stateOffset, evidenceOffset, count, ok := placementSummaryOffsets(len(payload), count64)
	if !ok {
		return SummaryResult{}, false
	}
	expectedAllocations := 0
	for dense := 0; dense < expected.DenseKeyCount(); dense++ {
		key, keyOK := expected.KeyAt(dense)
		if !keyOK {
			return SummaryResult{}, false
		}
		if key.Kind() == heapdomain.RootAllocation {
			expectedAllocations++
		}
	}
	// The result denominator is the exact owner-issued allocation universe.
	// A payload that omits a root cannot be made sound by its row metadata or
	// by treating the missing row as Unknown.
	if count != expectedAllocations {
		return SummaryResult{}, false
	}
	anyPresent := false
	for index := 0; index < count; index++ {
		idAt := idOffset + index*placementSummaryAllocationIDSize
		stateAt := stateOffset + index
		if !placementSummaryIDAvailable(payload, idAt) {
			return SummaryResult{}, false
		}
		rowID := identity.ContentID{}
		copy(rowID[:], payload[idAt:idAt+placementSummaryAllocationIDSize])
		key, keyOK := expected.Heap().KeyForID(rowID)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			return SummaryResult{}, false
		}
		_, rowPresent, stateOK := placementFromWireState(payload[stateAt])
		if !stateOK {
			return SummaryResult{}, false
		}
		// Every denominator row is complete. State zero is reserved for an
		// absent sparse cell and is not a legal public result row.
		if !rowPresent {
			return SummaryResult{}, false
		}
		anyPresent = true
		if index > 0 {
			previousAt := idAt - placementSummaryAllocationIDSize
			// The wire is canonicalized by the complete owner-issued ID. A
			// strict adjacent check rejects both duplicates and reordered rows
			// in one linear pass without a decoder-side set allocation.
			if payload[previousAt:idAt] >= payload[idAt:idAt+placementSummaryAllocationIDSize] {
				return SummaryResult{}, false
			}
		}
	}
	if present != anyPresent || rows != uint64(placementRowsForPresenceValue(anyPresent)) {
		return SummaryResult{}, false
	}

	// Revision 8 carries one fixed evidence record for every denominator row.
	// There is no compact class suffix to scan or infer; the exact fixed-plane
	// boundary is established by placementSummaryOffsets above.
	if !validateAllocationEvidencePlane(payload, evidenceOffset, idOffset, count) {
		return SummaryResult{}, false
	}
	return SummaryResult{
		payload:         payload,
		format:          format,
		allocationCount: count,
		idOffset:        idOffset,
		stateOffset:     stateOffset,
		evidenceOffset:  evidenceOffset,
	}, true
}

// Available reports whether this is a successfully decoded summary.
func (result SummaryResult) Available() bool {
	return result.payload != "" && result.allocationCount >= 0 &&
		result.format == placementSummaryResultFormat &&
		result.idOffset >= placementSummaryResultHeaderSize &&
		result.stateOffset >= result.idOffset &&
		result.stateOffset <= len(result.payload) &&
		result.evidenceOffset >= result.stateOffset && result.evidenceOffset <= len(result.payload) &&
		result.allocationCount <= (len(result.payload)-result.evidenceOffset)/placementSummaryEvidenceRecordSize
}

// Version reports the Placement summary wire revision.
func (result SummaryResult) Version() uint64 {
	if !result.Available() {
		return 0
	}
	return result.format
}

// SchemaID returns the exact Placement schema identity carried by the
// summary. DecodeSummaryResult compares this identity with the caller's
// expected Placement Schema before exposing the result; this accessor remains
// useful to diagnostics after a schema-bound result has been opened.
func (result SummaryResult) SchemaID() identity.ContentID {
	var id identity.ContentID
	if !result.Available() {
		return id
	}
	for index := range id {
		id[index] = result.payload[8+index]
	}
	return id
}

// AllocationCount reports the number of public allocation rows. Boot roots do
// not contribute to this denominator or iterator.
func (result SummaryResult) AllocationCount() int {
	if !result.Available() {
		return 0
	}
	return result.allocationCount
}

// Allocations opens a canonical ContentID-order iterator over the immutable
// allocation-root result.
func (result SummaryResult) Allocations() SummaryResultIterator {
	if !result.Available() {
		return SummaryResultIterator{}
	}
	return SummaryResultIterator{
		payload:         result.payload,
		format:          result.format,
		allocationCount: result.allocationCount,
		idOffset:        result.idOffset,
		stateOffset:     result.stateOffset,
		evidenceOffset:  result.evidenceOffset,
	}
}

// Allocation opens the canonical row for one owner-issued Heap allocation
// identity. The wire ID plane is strictly sorted, so identity lookup is
// logarithmic and requires neither a retained inverse directory nor a
// per-query allocation. The fixed state plane is direct-addressed after
// the binary search; fixed-width evidence remains direct-addressed as well.
func (result SummaryResult) Allocation(id identity.ContentID) (SummaryResultAllocation, bool) {
	index, found := result.allocationIndex(id)
	if !found {
		return SummaryResultAllocation{}, false
	}
	start := result.idOffset + index*placementSummaryAllocationIDSize

	return SummaryResultAllocation{
		payload:        result.payload,
		format:         result.format,
		idOffset:       start,
		stateOffset:    result.stateOffset + index,
		evidenceOffset: result.evidenceOffset + index*placementSummaryEvidenceRecordSize,
	}, true
}

// DeepFrozenFor reads the fixed-width evidence plane directly after one
// logarithmic ID lookup. Unlike Allocation it need not reconstruct the compact
// class-record rank, so repeated publication checks do not degrade to a
// quadratic prefix scan.
func (result SummaryResult) DeepFrozenFor(id identity.ContentID) (EvidenceState, bool) {
	index, found := result.allocationIndex(id)
	if !found {
		// An unreadable row has no proof column at all. Returning Unknown here
		// would hand a caller that drops the boolean an authenticated verdict
		// this result never carried.
		return invalidEvidenceState, false
	}
	offset := result.evidenceOffset + index*placementSummaryEvidenceRecordSize
	record := result.payload[offset : offset+placementSummaryEvidenceRecordSize]
	return EvidenceState(record[placementSummaryDeepFrozenOffset]), true
}

func (result SummaryResult) allocationIndex(id identity.ContentID) (int, bool) {
	if !result.Available() || !id.Available() {
		return 0, false
	}
	low, high := 0, result.allocationCount
	for low < high {
		middle := low + (high-low)/2
		start := result.idOffset + middle*placementSummaryAllocationIDSize
		if comparePlacementSummaryID(result.payload, start, id) < 0 {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low >= result.allocationCount {
		return 0, false
	}
	start := result.idOffset + low*placementSummaryAllocationIDSize
	return low, comparePlacementSummaryID(result.payload, start, id) == 0
}

func comparePlacementSummaryID(payload string, offset int, id identity.ContentID) int {
	for index := range id {
		left, right := payload[offset+index], id[index]
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
	}
	return 0
}

// Next returns the next allocation row. Every row consumes exactly one state
// byte and one fixed evidence record, regardless of presence.
func (iterator *SummaryResultIterator) Next() (SummaryResultAllocation, bool) {
	if iterator == nil || iterator.payload == "" || iterator.index >= iterator.allocationCount {
		return SummaryResultAllocation{}, false
	}
	index := iterator.index
	iterator.index++
	allocation := SummaryResultAllocation{
		payload:        iterator.payload,
		format:         iterator.format,
		idOffset:       iterator.idOffset + index*placementSummaryAllocationIDSize,
		stateOffset:    iterator.stateOffset + index,
		evidenceOffset: iterator.evidenceOffset + index*placementSummaryEvidenceRecordSize,
	}
	return allocation, true
}

// AllocationID returns the stable Heap allocation KeyID for this row.
func (allocation SummaryResultAllocation) AllocationID() identity.ContentID {
	var id identity.ContentID
	if !allocation.Available() {
		return id
	}
	for index := range id {
		id[index] = allocation.payload[allocation.idOffset+index]
	}
	return id
}

// Present reports whether this readable allocation has a Placement class.
// The second result distinguishes an absent class in a real row from an
// unavailable row; callers may not use false as both meanings.
func (allocation SummaryResultAllocation) Present() (bool, bool) {
	if !allocation.Available() {
		return false, false
	}
	_, present, ok := placementFromWireState(allocation.payload[allocation.stateOffset])
	return present, ok
}

// Placement returns the decoded Placement class for a present allocation.
func (allocation SummaryResultAllocation) Placement() (Placement, bool) {
	if !allocation.Available() {
		return invalidPlacementResult, false
	}
	value, present, ok := placementFromWireState(allocation.payload[allocation.stateOffset])
	if !ok || !present {
		return invalidPlacementResult, false
	}
	return value, true
}

// Evidence returns this allocation's immutable proof row. An unavailable or
// malformed row refuses; it is never represented as a valid all-absent row.
func (allocation SummaryResultAllocation) Evidence() (AllocationEvidence, bool) {
	if !allocation.Available() {
		return invalidAllocationEvidence(), false
	}
	evidence := AllocationEvidence{}
	if class, present := allocation.Placement(); present {
		evidence.Class = class
		evidence.HasClass = true
	}
	if allocation.evidenceOffset < 0 || allocation.evidenceOffset+placementSummaryEvidenceRecordSize > len(allocation.payload) {
		return invalidAllocationEvidence(), false
	}
	return decodeAllocationEvidence(allocation.payload[allocation.evidenceOffset:allocation.evidenceOffset+placementSummaryEvidenceRecordSize], evidence)
}

// Kind returns the owner-authenticated allocation kind when this row's
// producer established one. Program table/closure roots retain their concrete
// kind; Target fresh-result roots use the canonical manifest.allocation kind
// after Heap authenticates their FreshResult identity.
func (allocation SummaryResultAllocation) Kind() (AllocationKind, bool) {
	evidence, ok := allocation.Evidence()
	return evidence.Kind, ok && evidence.HasKind
}

// OwnerIdentity returns the owner-issued Heap-root identity, not a guessed
// containment-parent identity.
func (allocation SummaryResultAllocation) OwnerIdentity() (identity.ContentID, bool) {
	evidence, ok := allocation.Evidence()
	return evidence.OwnerIdentity, ok && evidence.HasOwnerIdentity
}

// Depth returns a static containment depth only when a producer supplied it.
func (allocation SummaryResultAllocation) Depth() (uint32, bool) {
	evidence, ok := allocation.Evidence()
	return evidence.Depth, ok && evidence.HasDepth
}

// FrameLocal returns this row's frame-local proof column. An unreadable row
// carries no column at all: absence is a statement about a row that exists, so
// an unavailable row yields the inadmissible state rather than absence.
func (allocation SummaryResultAllocation) FrameLocal() (EvidenceState, bool) {
	evidence, ok := allocation.Evidence()
	return proofColumn(evidence.FrameLocal, ok)
}

// DiesBeforeSuspension returns this row's suspension-liveness proof column.
func (allocation SummaryResultAllocation) DiesBeforeSuspension() (EvidenceState, bool) {
	evidence, ok := allocation.Evidence()
	return proofColumn(evidence.DiesBeforeSuspension, ok)
}

// DeepFrozen returns the transitive frozen-graph proof carried by this row.
func (allocation SummaryResultAllocation) DeepFrozen() (EvidenceState, bool) {
	evidence, ok := allocation.Evidence()
	return proofColumn(evidence.DeepFrozen, ok)
}

// RetainEscape returns the path-sensitive retain provenance serialized from
// the same canonical Placement factor as this row's class.
func (allocation SummaryResultAllocation) RetainEscape() (EvidenceState, bool) {
	evidence, ok := allocation.Evidence()
	return proofColumn(evidence.RetainEscape, ok)
}

// Fact reconstructs the one canonical Placement factor value carried by this
// row. It never infers retain provenance from the placement class.
func (allocation SummaryResultAllocation) Fact() (Fact, bool) {
	class, classOK := allocation.Placement()
	retained, retainedOK := allocation.RetainEscape()
	fact := Fact{Class: class, RetainEscape: retained}
	if !classOK || !retainedOK || !fact.Valid() || class == Bottom || retained == EvidenceAbsent {
		return invalidFact(), false
	}
	return fact, true
}

// proofColumn is the one projection every row-scoped proof accessor shares. It
// keeps unavailability and absence apart in both results, so a caller that
// drops the boolean still reads an inadmissible state instead of a column the
// result never carried.
func proofColumn(state EvidenceState, available bool) (EvidenceState, bool) {
	if !available || !state.Valid() {
		return invalidEvidenceState, false
	}
	return state, true
}

func (allocation SummaryResultAllocation) Available() bool {
	return allocation.payload != "" && allocation.idOffset >= placementSummaryResultHeaderSize &&
		allocation.idOffset+placementSummaryAllocationIDSize <= len(allocation.payload) &&
		allocation.stateOffset >= allocation.idOffset+placementSummaryAllocationIDSize &&
		allocation.stateOffset < len(allocation.payload) &&
		allocation.evidenceOffset >= 0 && allocation.evidenceOffset <= len(allocation.payload)-placementSummaryEvidenceRecordSize
}

func placementSummaryOffsets(payloadLength int, count64 uint64) (idOffset, stateOffset, evidenceOffset, count int, ok bool) {
	if payloadLength < placementSummaryResultHeaderSize {
		return 0, 0, 0, 0, false
	}
	remaining := payloadLength - placementSummaryResultHeaderSize
	rowBytes := placementSummaryAllocationIDSize + 1 + placementSummaryEvidenceRecordSize
	if count64 > uint64(remaining/rowBytes) || count64 > uint64(^uint(0)>>1) {
		return 0, 0, 0, 0, false
	}
	count = int(count64)
	idOffset = placementSummaryResultHeaderSize
	stateOffset = idOffset + count*placementSummaryAllocationIDSize
	evidenceOffset = stateOffset + count
	if evidenceOffset > payloadLength || evidenceOffset > payloadLength-count*placementSummaryEvidenceRecordSize {
		return 0, 0, 0, 0, false
	}
	if evidenceOffset+count*placementSummaryEvidenceRecordSize != payloadLength {
		return 0, 0, 0, 0, false
	}
	return idOffset, stateOffset, evidenceOffset, count, true
}

func validateAllocationEvidencePlane(payload string, offset, idOffset, count int) bool {
	if offset < 0 || idOffset < 0 || count < 0 || count > (len(payload)-offset)/placementSummaryEvidenceRecordSize || count > (len(payload)-idOffset)/placementSummaryAllocationIDSize {
		return false
	}
	for index := 0; index < count; index++ {
		start := offset + index*placementSummaryEvidenceRecordSize
		idStart := idOffset + index*placementSummaryAllocationIDSize
		record := payload[start : start+placementSummaryEvidenceRecordSize]
		if !validAllocationEvidenceBytes(record) || record[1] != 1 || record[2:34] != payload[idStart:idStart+placementSummaryAllocationIDSize] {
			return false
		}
	}
	return true
}

func validAllocationEvidenceBytes(payload string) bool {
	if len(payload) != placementSummaryEvidenceRecordSize || payload[0] > byte(AllocationKindManifest) || payload[1] > 1 || payload[34] > 1 ||
		payload[placementSummaryRetainEscapeOffset] > byte(EvidenceProven) || payload[placementSummaryFrameLocalOffset] > byte(EvidenceProven) ||
		payload[placementSummaryDiesBeforeOffset] > byte(EvidenceProven) || payload[placementSummaryDeepFrozenOffset] > byte(EvidenceProven) {
		return false
	}
	if payload[1] == 1 && !placementSummaryIDAvailable(payload, 2) {
		return false
	}
	if payload[1] == 0 {
		for index := 2; index < 34; index++ {
			if payload[index] != 0 {
				return false
			}
		}
	}
	if payload[34] == 0 && (payload[35] != 0 || payload[36] != 0 || payload[37] != 0 || payload[38] != 0) {
		return false
	}
	return true
}

func decodeAllocationEvidence(payload string, evidence AllocationEvidence) (AllocationEvidence, bool) {
	if !validAllocationEvidenceBytes(payload) {
		return invalidAllocationEvidence(), false
	}
	evidence.Kind = AllocationKind(payload[0])
	evidence.HasKind = evidence.Kind != AllocationKindUnknown
	if payload[1] == 1 {
		evidence.HasOwnerIdentity = true
		for index := range evidence.OwnerIdentity {
			evidence.OwnerIdentity[index] = payload[2+index]
		}
	}
	if payload[34] == 1 {
		evidence.HasDepth = true
		evidence.Depth = uint32(payload[35])<<24 | uint32(payload[36])<<16 | uint32(payload[37])<<8 | uint32(payload[38])
	}
	evidence.RetainEscape = EvidenceState(payload[placementSummaryRetainEscapeOffset])
	evidence.FrameLocal = EvidenceState(payload[placementSummaryFrameLocalOffset])
	evidence.DiesBeforeSuspension = EvidenceState(payload[placementSummaryDiesBeforeOffset])
	evidence.DeepFrozen = EvidenceState(payload[placementSummaryDeepFrozenOffset])
	return evidence, evidence.Valid()
}

func placementSummaryUint64(payload string, offset int) uint64 {
	return uint64(payload[offset])<<56 |
		uint64(payload[offset+1])<<48 |
		uint64(payload[offset+2])<<40 |
		uint64(payload[offset+3])<<32 |
		uint64(payload[offset+4])<<24 |
		uint64(payload[offset+5])<<16 |
		uint64(payload[offset+6])<<8 |
		uint64(payload[offset+7])
}

func placementSummaryIDAvailable(payload string, offset int) bool {
	if offset < 0 || offset+placementSummaryAllocationIDSize > len(payload) {
		return false
	}
	for index := 0; index < placementSummaryAllocationIDSize; index++ {
		if payload[offset+index] != 0 {
			return true
		}
	}
	return false
}

func placementSummaryIDEquals(payload string, offset int, expected identity.ContentID) bool {
	if !expected.Available() || offset < 0 || offset+placementSummaryAllocationIDSize > len(payload) {
		return false
	}
	for index := range expected {
		if payload[offset+index] != expected[index] {
			return false
		}
	}
	return true
}

func placementRowsForPresenceValue(present bool) uint32 {
	if present {
		return 1
	}
	return 0
}
