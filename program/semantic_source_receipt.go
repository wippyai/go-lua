package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	programmodule "github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/semanticsource"
	programsource "github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// ErrSemanticSourceUnavailable reports that a Program does not have a valid
// sealed semantic-source receipt.  A zero-row child is valid; this error is
// reserved for an unavailable, foreign, stale, or malformed owner boundary.
var ErrSemanticSourceUnavailable = errSemanticSource

const programSemanticSourcePublicationCount = 57

// SemanticSourceView is one detached Program-child contribution. It carries
// both fences deliberately: OwnerID/ProgramID identify the exact quartet
// root, while ChildID identifies the exact immutable child authority that
// produced the claims. The claims are immutable semantic-source cardinalities,
// not rows or an erased relation stream.
type SemanticSourceView struct {
	program    keyspace.ContentID
	owner      keyspace.ContentID
	rangeValue semanticsource.PublicationRange
}

func (view SemanticSourceView) valid() bool {
	return view.program.Available() && view.owner.Available() && view.rangeValue.Valid()
}

// Valid reports whether the detached child contribution still has complete
// owner and canonical-definition provenance.
func (view SemanticSourceView) Valid() bool { return view.valid() }

// OwnerID is the exact Program ContentID which owns this contribution.
func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.program }

// ChildID is the exact immutable child ContentID which produced this
// contribution.
func (view SemanticSourceView) ChildID() keyspace.ContentID { return view.owner }

// ProgramID is the exact Program ContentID to which this child contribution
// was sealed. It is not inferred from a child ID or from relation tokens.
func (view SemanticSourceView) ProgramID() keyspace.ContentID { return view.program }

func (view SemanticSourceView) Range() semanticsource.PublicationRange {
	if !view.valid() {
		return semanticsource.PublicationRange{}
	}
	return view.rangeValue.Snapshot()
}

// Count reports all owner-local definitions, including required zero rows.
func (view SemanticSourceView) Count() int {
	if !view.valid() {
		return 0
	}
	return view.rangeValue.Count()
}

// At returns one canonical detached owner-local publication.
func (view SemanticSourceView) At(index int) (semanticsource.Publication, bool) {
	if !view.valid() {
		return semanticsource.Publication{}, false
	}
	return view.rangeValue.At(index)
}

// PublicationAt is the named form used by receipt consumers.
func (view SemanticSourceView) PublicationAt(index int) (semanticsource.Publication, bool) {
	return view.At(index)
}

// Publications returns a detached copy of the cardinality claims. It never
// exposes the receipt's backing storage to a caller.
func (view SemanticSourceView) Publications() []semanticsource.Publication {
	if !view.valid() {
		return nil
	}
	return view.rangeValue.Publications()
}

// SemanticSourceCursor walks one exact child view in canonical owner order.
// It intentionally knows no generic facet vocabulary beyond this named view.
type SemanticSourceCursor struct {
	view  SemanticSourceView
	count int
	index int
}

// Cursor creates a detached cursor over one child contribution.
func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	return SemanticSourceCursor{view: view, count: view.Count()}
}

// Next returns the next owner-local publication, including zero-count rows.
func (cursor *SemanticSourceCursor) Next() (semanticsource.Publication, bool) {
	if cursor == nil || !cursor.view.valid() || cursor.index < 0 || cursor.index >= cursor.count {
		return semanticsource.Publication{}, false
	}
	row, ok := cursor.view.At(cursor.index)
	if !ok {
		return semanticsource.Publication{}, false
	}
	cursor.index++
	return row, true
}

// These distinct names make the four owner boundaries visible in API and
// reflection without introducing a generic facet stream. They embed the
// common detached mechanics only; each field in SemanticSourceViews remains
// explicitly named and owner-specific.
type SourceSemanticSourceFragment struct{ SemanticSourceView }
type FlowSemanticSourceFragment struct{ SemanticSourceView }
type StaticSemanticSourceFragment struct{ SemanticSourceView }
type ModuleSemanticSourceFragment struct{ SemanticSourceView }

// SemanticSourceViews is the exact Program quartet of named child fragments.
// It has no map, registry, or erased stream: every owner is represented once.
type SemanticSourceViews struct {
	owner  keyspace.ContentID
	source SourceSemanticSourceFragment
	flow   FlowSemanticSourceFragment
	static StaticSemanticSourceFragment
	module ModuleSemanticSourceFragment
}

func (views SemanticSourceViews) valid() bool {
	if !views.owner.Available() || !views.source.Valid() || !views.flow.Valid() || !views.static.Valid() || !views.module.Valid() {
		return false
	}
	if views.source.Count() != 8 || views.flow.Count() != 33 || views.static.Count() != 10 || views.module.Count() != 6 {
		return false
	}
	return views.source.ProgramID() == views.owner &&
		views.flow.ProgramID() == views.owner &&
		views.static.ProgramID() == views.owner &&
		views.module.ProgramID() == views.owner
}

// Valid reports whether all four named child fragments are complete and
// fenced to this exact Program root.
func (views SemanticSourceViews) Valid() bool { return views.valid() }

// OwnerID is the exact Program ContentID owning this quartet.
func (views SemanticSourceViews) OwnerID() keyspace.ContentID { return views.owner }

func (views SemanticSourceViews) ProgramID() keyspace.ContentID        { return views.owner }
func (views SemanticSourceViews) Source() SourceSemanticSourceFragment { return views.source }
func (views SemanticSourceViews) Flow() FlowSemanticSourceFragment     { return views.flow }
func (views SemanticSourceViews) Static() StaticSemanticSourceFragment { return views.static }
func (views SemanticSourceViews) Module() ModuleSemanticSourceFragment { return views.module }

// ChildIDs returns the four exact child authorities in Program order.
func (views SemanticSourceViews) ChildIDs() (source, flow, static, module keyspace.ContentID) {
	return views.source.ChildID(), views.flow.ChildID(), views.static.ChildID(), views.module.ChildID()
}

func (views SemanticSourceViews) SourceID() keyspace.ContentID { return views.source.ChildID() }
func (views SemanticSourceViews) FlowID() keyspace.ContentID   { return views.flow.ChildID() }
func (views SemanticSourceViews) StaticID() keyspace.ContentID { return views.static.ChildID() }
func (views SemanticSourceViews) ModuleID() keyspace.ContentID { return views.module.ChildID() }

// Count reports exactly 57 Program definitions when the receipt is valid.
func (views SemanticSourceViews) Count() int {
	if !views.valid() {
		return 0
	}
	return views.source.Count() + views.flow.Count() + views.static.Count() + views.module.Count()
}

// At returns the exact aggregate partition without materializing a root
// publication slice. Source owns rows 0..5 and 6..7 around Flow's range;
// all other ranges are contiguous in owner order.
func (views SemanticSourceViews) At(index int) (semanticsource.Publication, bool) {
	if !views.valid() || index < 0 || index >= programSemanticSourcePublicationCount {
		return semanticsource.Publication{}, false
	}
	sourceCount := views.source.Count()
	flowCount := views.flow.Count()
	staticCount := views.static.Count()
	if index < sourceCount-2 {
		return views.source.At(index)
	}
	index -= sourceCount - 2
	if index < flowCount {
		return views.flow.At(index)
	}
	index -= flowCount
	if index < 2 {
		return views.source.At(sourceCount - 2 + index)
	}
	index -= 2
	if index < staticCount {
		return views.static.At(index)
	}
	index -= staticCount
	return views.module.At(index)
}

// Digest computes the aggregate receipt identity by traversing owner ranges
// in canonical Program order. No root claim slice or digest cache is retained.
func (views SemanticSourceViews) Digest() ([sha256.Size]byte, bool) {
	if !views.valid() || views.Count() != programSemanticSourcePublicationCount {
		return [sha256.Size]byte{}, false
	}
	hash := sha256.New()
	var frame [24]byte
	for index := 0; index < programSemanticSourcePublicationCount; index++ {
		row, ok := views.At(index)
		if !ok {
			return [sha256.Size]byte{}, false
		}
		token := row.Definition().Token()
		binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
		binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
		binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
		binary.BigEndian.PutUint64(frame[8:16], token.Digest())
		binary.BigEndian.PutUint64(frame[16:24], uint64(row.Count()))
		_, _ = hash.Write(frame[:])
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, true
}

// Cursor walks the explicit quartet fragments in generated catalog order.
// Source owns the FlowLiterals and FlowBody rows as well, so those two Source
// ranges are split around Flow's named range without flattening the owner
// boundaries. Duplicate Program mounts are intentionally outside this cursor
// and remain Link's responsibility.
type ReceiptCursor struct {
	views SemanticSourceViews
	count int
	index int
}

func (views SemanticSourceViews) Cursor() ReceiptCursor {
	return ReceiptCursor{views: views, count: views.Count()}
}

func (cursor *ReceiptCursor) Count() int {
	if cursor == nil || !cursor.views.valid() {
		return 0
	}
	return cursor.count
}

func (cursor *ReceiptCursor) At(index int) (semanticsource.Publication, bool) {
	if cursor == nil || !cursor.views.valid() || index < 0 || index >= cursor.count {
		return semanticsource.Publication{}, false
	}
	return cursor.views.At(index)
}

func (cursor *ReceiptCursor) Next() (semanticsource.Publication, bool) {
	if cursor == nil || !cursor.views.valid() || cursor.index >= cursor.count {
		return semanticsource.Publication{}, false
	}
	row, ok := cursor.At(cursor.index)
	if !ok {
		return semanticsource.Publication{}, false
	}
	cursor.index++
	return row, true
}

// SemanticSourceReceipt is the immutable, detached Program-owned semantic
// source boundary. It retains only the root ID, four child IDs, and the
// owner-local cardinality claims needed by a cold Link mount.
type SemanticSourceReceipt struct {
	owner keyspace.ContentID
	views SemanticSourceViews
}

// Valid reports receipt-local integrity. Program.SemanticSourceReceipt adds
// the live Program identity/provenance fence as well.
func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.views.valid() && receipt.views.owner == receipt.owner &&
		receipt.views.Count() == programSemanticSourcePublicationCount
}

func (receipt SemanticSourceReceipt) OwnerID() keyspace.ContentID   { return receipt.owner }
func (receipt SemanticSourceReceipt) ProgramID() keyspace.ContentID { return receipt.owner }
func (receipt SemanticSourceReceipt) Views() (SemanticSourceViews, bool) {
	if !receipt.Valid() {
		return SemanticSourceViews{}, false
	}
	return receipt.views, true
}

func (receipt SemanticSourceReceipt) Source() (SourceSemanticSourceFragment, bool) {
	views, ok := receipt.Views()
	if !ok {
		return SourceSemanticSourceFragment{}, false
	}
	return views.Source(), true
}

func (receipt SemanticSourceReceipt) Flow() (FlowSemanticSourceFragment, bool) {
	views, ok := receipt.Views()
	if !ok {
		return FlowSemanticSourceFragment{}, false
	}
	return views.Flow(), true
}

func (receipt SemanticSourceReceipt) Static() (StaticSemanticSourceFragment, bool) {
	views, ok := receipt.Views()
	if !ok {
		return StaticSemanticSourceFragment{}, false
	}
	return views.Static(), true
}

func (receipt SemanticSourceReceipt) Module() (ModuleSemanticSourceFragment, bool) {
	views, ok := receipt.Views()
	if !ok {
		return ModuleSemanticSourceFragment{}, false
	}
	return views.Module(), true
}

func (receipt SemanticSourceReceipt) ChildIDs() (source, flow, static, module keyspace.ContentID, ok bool) {
	views, valid := receipt.Views()
	if !valid {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	source, flow, static, module = views.ChildIDs()
	return source, flow, static, module, true
}

func (receipt SemanticSourceReceipt) Count() int {
	if !receipt.Valid() {
		return 0
	}
	return receipt.views.Count()
}

func (receipt SemanticSourceReceipt) Cursor() ReceiptCursor {
	return receipt.views.Cursor()
}

// Publications returns a fresh canonical quartet sequence. The returned
// claims are detached values; mutating this slice cannot alter the receipt.
func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	rows := make([]semanticsource.Publication, 0, programSemanticSourcePublicationCount)
	cursor := receipt.Cursor()
	for {
		row, ok := cursor.Next()
		if !ok {
			break
		}
		rows = append(rows, row)
	}
	if len(rows) != programSemanticSourcePublicationCount {
		return nil
	}
	return rows
}

// SemanticSourceReceipt returns the seal-time detached receipt. It never
// rebuilds child fragments on a hot mount; stale or foreign in-memory state
// fails closed against the exact Program quartet and Flow provenance.
func (program *Program) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if program == nil || !program.semanticReceipt.validFor(program) {
		return SemanticSourceReceipt{}, false
	}
	return program.semanticReceipt, true
}

func (program *Program) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := program.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}

func (program *Program) semanticOwnerIDs() (sourceID, flowID, staticID, moduleID keyspace.ContentID, ok bool) {
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	sourceID = program.source.Cold().ContentID()
	flowID = program.flow.View().ContentID()
	staticID = program.static.Cold().ContentID()
	moduleID = program.module.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	return sourceID, flowID, staticID, moduleID, true
}

func (receipt SemanticSourceReceipt) validFor(program *Program) bool {
	if program == nil || !receipt.Valid() || receipt.owner != program.id {
		return false
	}
	sourceID, flowID, staticID, moduleID, ok := program.semanticOwnerIDs()
	if !ok {
		return false
	}
	rootID, err := rootContentID(sourceID, flowID, staticID, moduleID)
	if err != nil || rootID != program.id {
		return false
	}
	provenance := program.flow.View().Provenance()
	if provenance.Source != sourceID || provenance.Flow != flowID ||
		provenance.Static != staticID || provenance.Module != moduleID {
		return false
	}
	if receipt.views.owner != program.id || receipt.views.SourceID() != sourceID ||
		receipt.views.FlowID() != flowID || receipt.views.StaticID() != staticID ||
		receipt.views.ModuleID() != moduleID {
		return false
	}
	// Keep the root federator scalar-only: each child validates its live typed
	// projections against the one sealed range, then the root binds the
	// receipt's federated value to that exact child range/digest.
	sourceFragment, err := programsource.SemanticSourceFragmentView(program.source.View())
	if err != nil {
		return false
	}
	flowFragment, err := flow.SemanticSourceFragmentView(program.flow.View())
	if err != nil {
		return false
	}
	staticFragment, err := programstatic.SemanticSourceFragmentView(program.static.View())
	if err != nil {
		return false
	}
	moduleFragment, err := programmodule.SemanticSourceFragmentView(program.module.View())
	if err != nil {
		return false
	}
	return samePublicationRange(receipt.views.source.Range(), sourceFragment.Range()) &&
		samePublicationRange(receipt.views.flow.Range(), flowFragment.Range()) &&
		samePublicationRange(receipt.views.static.Range(), staticFragment.Range()) &&
		samePublicationRange(receipt.views.module.Range(), moduleFragment.Range())
}

// samePublicationRange binds a root-federated owner interval to the exact
// sealed child scalar. PublicationRange.Valid recomputes the row digest, so a
// copied value with a changed row or cached digest cannot satisfy this fence.
func samePublicationRange(left, right semanticsource.PublicationRange) bool {
	if left.Count() != right.Count() {
		return false
	}
	leftDigest, leftOK := left.Digest()
	rightDigest, rightOK := right.Digest()
	return leftOK && rightOK && leftDigest == rightDigest
}

// buildProgramSemanticSourceReceipt is called exactly once by Publish. Each
// child remains responsible for its own typed validation; this root function
// only binds the four completed owner-local fragments to the exact quartet and
// checks their generated token slices.
func buildProgramSemanticSourceReceipt(program *Program) (SemanticSourceReceipt, bool) {
	if program == nil || !program.id.Available() {
		return SemanticSourceReceipt{}, false
	}
	sourceID, flowID, staticID, moduleID, ok := program.semanticOwnerIDs()
	if !ok {
		return SemanticSourceReceipt{}, false
	}
	provenance := program.flow.View().Provenance()
	if provenance.Source != sourceID || provenance.Flow != flowID ||
		provenance.Static != staticID || provenance.Module != moduleID {
		return SemanticSourceReceipt{}, false
	}

	sourceFragment, err := programsource.SemanticSourceFragmentView(program.source.View())
	if err != nil {
		return SemanticSourceReceipt{}, false
	}
	flowFragment, err := flow.SemanticSourceFragmentView(program.flow.View())
	if err != nil {
		return SemanticSourceReceipt{}, false
	}
	staticFragment, err := programstatic.SemanticSourceFragmentView(program.static.View())
	if err != nil {
		return SemanticSourceReceipt{}, false
	}
	moduleFragment, err := programmodule.SemanticSourceFragmentView(program.module.View())
	if err != nil {
		return SemanticSourceReceipt{}, false
	}
	if sourceFragment.Count() != 8 || flowFragment.Count() != 33 || staticFragment.Count() != 10 || moduleFragment.Count() != 6 {
		return SemanticSourceReceipt{}, false
	}

	views := SemanticSourceViews{
		owner: program.id,
		source: SourceSemanticSourceFragment{SemanticSourceView: SemanticSourceView{
			program: program.id, owner: sourceID, rangeValue: sourceFragment.Range(),
		}},
		flow: FlowSemanticSourceFragment{SemanticSourceView: SemanticSourceView{
			program: program.id, owner: flowID, rangeValue: flowFragment.Range(),
		}},
		static: StaticSemanticSourceFragment{SemanticSourceView: SemanticSourceView{
			program: program.id, owner: staticID, rangeValue: staticFragment.Range(),
		}},
		module: ModuleSemanticSourceFragment{SemanticSourceView: SemanticSourceView{
			program: program.id, owner: moduleID, rangeValue: moduleFragment.Range(),
		}},
	}
	receipt := SemanticSourceReceipt{owner: program.id, views: views}
	if _, ok := views.Digest(); !ok {
		return SemanticSourceReceipt{}, false
	}
	return receipt, receipt.validFor(program)
}
