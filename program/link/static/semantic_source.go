package static

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is one detached LinkStatic-owned source-family receipt.
// The rows are emitted from the typed Component projections before the owner
// is detached; no ordinal-only fallback can manufacture a row.
type SemanticSourceView struct {
	owner   keyspace.ContentID
	digests []keyspace.ContentID
}

func (view SemanticSourceView) valid() bool {
	if !view.owner.Available() {
		return false
	}
	for _, digest := range view.digests {
		if !digest.Available() {
			return false
		}
	}
	return true
}

// OwnerID returns the exact sealed Static Component identity that owns this
// detached view.
func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }

// Count reports the detached typed row count, including zero.
func (view SemanticSourceView) Count() int {
	if !view.valid() {
		return 0
	}
	return len(view.digests)
}

// DigestAt is bounded by the same detached row storage used by Count.
func (view SemanticSourceView) DigestAt(index int) (keyspace.ContentID, bool) {
	if !view.valid() || index < 0 || index >= len(view.digests) {
		return keyspace.ContentID{}, false
	}
	digest := view.digests[index]
	return digest, true
}

// Digests returns a detached copy of the canonical typed-row identities.
func (view SemanticSourceView) Digests() []keyspace.ContentID {
	if !view.valid() {
		return nil
	}
	return append([]keyspace.ContentID(nil), view.digests...)
}

// SemanticSourceCursor walks one exact LinkStatic view. It is owner-local and
// detached; there is no generic AnalysisFacet stream.
type SemanticSourceCursor struct {
	view  SemanticSourceView
	index int
}

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	return SemanticSourceCursor{view: view}
}

func (cursor *SemanticSourceCursor) Next() (keyspace.ContentID, bool) {
	if cursor == nil || !cursor.view.valid() || cursor.index < 0 || cursor.index >= len(cursor.view.digests) {
		return keyspace.ContentID{}, false
	}
	digest := cursor.view.digests[cursor.index]
	cursor.index++
	return digest, digest.Available()
}

// SemanticSourceViews names the five LinkStatic families explicitly. The
// generated catalog remains the vocabulary authority; these fields are only
// typed owner projections.
type SemanticSourceViews struct {
	owner                                         keyspace.ContentID
	static, resolution, expression, export, input SemanticSourceView
}

func (views SemanticSourceViews) all() []SemanticSourceView {
	return []SemanticSourceView{views.static, views.resolution, views.expression, views.export, views.input}
}

func (views SemanticSourceViews) valid() bool {
	if !views.owner.Available() {
		return false
	}
	for _, view := range views.all() {
		if !view.valid() || view.owner != views.owner {
			return false
		}
	}
	return true
}

func (views SemanticSourceViews) OwnerID() keyspace.ContentID { return views.owner }
func (views SemanticSourceViews) Static() SemanticSourceView  { return views.static }
func (views SemanticSourceViews) Resolution() SemanticSourceView {
	return views.resolution
}
func (views SemanticSourceViews) Expression() SemanticSourceView {
	return views.expression
}
func (views SemanticSourceViews) Export() SemanticSourceView { return views.export }
func (views SemanticSourceViews) Input() SemanticSourceView  { return views.input }

// viewFor binds one generated LinkStatic definition to its typed owner view.
func (views SemanticSourceViews) viewFor(token semanticsource.Token) (SemanticSourceView, bool) {
	if token.Origin() != semanticsource.OriginLinkStatic {
		return SemanticSourceView{}, false
	}
	switch token.Facet() {
	case 0:
		return views.static, true
	case semanticsource.FacetLinkStaticResolution:
		return views.resolution, true
	case semanticsource.FacetLinkStaticExpression:
		return views.expression, true
	case semanticsource.FacetLinkStaticExport:
		return views.export, true
	case semanticsource.FacetLinkStaticInput:
		return views.input, true
	default:
		return SemanticSourceView{}, false
	}
}

// SemanticSourceReceipt is the detached owner-bound Static publication. It
// retains only the Static Component identity and copied row digests.
type SemanticSourceReceipt struct {
	owner keyspace.ContentID
	views SemanticSourceViews
}

func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	views, ok := receipt.Views()
	if !ok {
		return nil
	}
	schema := semanticsource.CatalogSchema()
	rows := make([]semanticsource.Publication, 0, 5)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined || definition.Token().Origin() != semanticsource.OriginLinkStatic {
			continue
		}
		view, found := views.viewFor(definition.Token())
		if !found {
			return nil
		}
		publication, err := semanticsource.SealPublication(definition, view.Count())
		if err != nil {
			return nil
		}
		rows = append(rows, publication)
	}
	return rows
}

func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.views.valid() && receipt.views.owner == receipt.owner
}

func (receipt SemanticSourceReceipt) OwnerID() keyspace.ContentID { return receipt.owner }

func (receipt SemanticSourceReceipt) Views() (SemanticSourceViews, bool) {
	if !receipt.Valid() {
		return SemanticSourceViews{}, false
	}
	return receipt.views, true
}

// SemanticSourceReceipt captures typed rows at the Component owner boundary.
func (c *Component) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if c == nil || !c.contentID.Available() || !c.semanticReceipt.Valid() || c.semanticReceipt.OwnerID() != c.contentID {
		return SemanticSourceReceipt{}, false
	}
	return c.semanticReceipt, true
}

func (c *Component) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := c.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}


// SemanticSourceReceipt returns the already detached receipt. A Draft fence
// still invalidates the snapshot after finalization consumes its authority.
func (v Cold) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if !v.live() || !v.semanticReceipt.Valid() || v.semanticReceipt.OwnerID() != v.contentID {
		return SemanticSourceReceipt{}, false
	}
	return v.semanticReceipt, true
}

func (v Cold) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := v.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}


// staticReceiptWriter encodes the exact typed scalar/identity projection of
// one row. It is a detached receipt digest, not a second row model.
type staticReceiptWriter struct {
	h     hash.Hash
	valid bool
	owner keyspace.ContentID
	token semanticsource.Token
}

func newStaticReceiptWriter(owner keyspace.ContentID, token semanticsource.Token) *staticReceiptWriter {
	w := &staticReceiptWriter{
		h:     sha256.New(),
		valid: owner.Available() && token.Origin() != 0 && token.Revision() != 0 && token.Digest() != 0,
		owner: owner,
		token: token,
	}
	if !w.valid {
		return w
	}
	_, _ = w.h.Write([]byte("wippy.link/static/semantic-source-row/v2"))
	_, _ = w.h.Write([]byte{0})
	_, _ = w.h.Write(owner[:])
	var frame [16]byte
	binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
	binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
	binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
	binary.BigEndian.PutUint64(frame[8:16], token.Digest())
	_, _ = w.h.Write(frame[:])
	return w
}

func (w *staticReceiptWriter) u64(value uint64) {
	if w == nil || !w.valid {
		return
	}
	var frame [8]byte
	binary.BigEndian.PutUint64(frame[:], value)
	_, _ = w.h.Write(frame[:])
}

func (w *staticReceiptWriter) term(value keyspace.Term) { w.u64(uint64(value)) }

func (w *staticReceiptWriter) id(value keyspace.ContentID) {
	if w == nil || !w.valid || !value.Available() {
		if w != nil {
			w.valid = false
		}
		return
	}
	_, _ = w.h.Write(value[:])
}

func (w *staticReceiptWriter) expression(ref ExpressionRef) {
	if w == nil || !w.valid {
		return
	}
	w.id(ref.StaticID())
	w.u64(uint64(ref.ShardOrdinal()))
	w.term(ref.Reference())
}

func (w *staticReceiptWriter) finish() (keyspace.ContentID, bool) {
	if w == nil || !w.valid {
		return keyspace.ContentID{}, false
	}
	var id keyspace.ContentID
	copy(id[:], w.h.Sum(nil))
	return id, id.Available()
}

type staticRowEmitter struct {
	owner  keyspace.ContentID
	token  semanticsource.Token
	rows   []keyspace.ContentID
	failed bool
}

func (emitter *staticRowEmitter) row(write func(*staticReceiptWriter)) {
	if emitter == nil || emitter.failed || write == nil {
		return
	}
	writer := newStaticReceiptWriter(emitter.owner, emitter.token)
	write(writer)
	digest, ok := writer.finish()
	if !ok {
		emitter.failed = true
		return
	}
	emitter.rows = append(emitter.rows, digest)
}

func staticTypedRows(c *Component, token semanticsource.Token, emit func(*staticRowEmitter)) (SemanticSourceView, bool) {
	if c == nil || !c.contentID.Available() || emit == nil {
		return SemanticSourceView{}, false
	}
	emitter := &staticRowEmitter{owner: c.contentID, token: token}
	emit(emitter)
	if emitter.failed {
		return SemanticSourceView{}, false
	}
	return SemanticSourceView{owner: emitter.owner, digests: emitter.rows}, true
}
