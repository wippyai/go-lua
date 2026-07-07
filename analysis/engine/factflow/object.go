package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ObjectEntry describes one static value written under an object constructor.
type ObjectEntry struct {
	suffix      path.Path
	source      ValueSource
	valueSpan   SourceSpan
	valueLabel  string
	expected    product.Value
	hasExpected bool
}

// ObjectEntryView provides read-only access to an object entry without
// exposing mutable internal path segment storage.
type ObjectEntryView struct {
	entry ObjectEntry
}

// NewObjectEntry creates a static object-entry descriptor.
func NewObjectEntry(suffix path.Path, source ValueSource) ObjectEntry {
	return ObjectEntry{
		suffix: suffix.Clone(),
		source: source,
	}
}

// NewObjectEntryWithMetadata creates a static object-entry descriptor with
// optional display metadata. The metadata remains syntax-free so downstream
// obligation readers do not need AST access for member-level diagnostics.
func NewObjectEntryWithMetadata(suffix path.Path, source ValueSource, valueSpan SourceSpan, valueLabel string) ObjectEntry {
	entry := NewObjectEntry(suffix, source)
	entry.valueSpan = valueSpan
	entry.valueLabel = valueLabel
	return entry
}

// Suffix returns the relative static suffix under the constructed object.
func (e ObjectEntry) Suffix() path.Path { return e.suffix.Clone() }

// Source returns the value assigned to the entry.
func (e ObjectEntry) Source() ValueSource { return e.source }

func (e ObjectEntry) ValueSpan() SourceSpan { return e.valueSpan }

func (e ObjectEntry) ValueLabel() string { return e.valueLabel }

// Expected returns the contextual value contract for this static entry when the
// object literal is checked against a declared table shape.
func (e ObjectEntry) Expected() (product.Value, bool) { return e.expected, e.hasExpected }

func (v ObjectEntryView) Source() ValueSource { return v.entry.source }

func (v ObjectEntryView) ValueSpan() SourceSpan { return v.entry.valueSpan }

func (v ObjectEntryView) ValueLabel() string { return v.entry.valueLabel }

func (v ObjectEntryView) Expected() (product.Value, bool) {
	return v.entry.expected, v.entry.hasExpected
}

// Suffix returns a defensive copy of the relative static suffix.
func (v ObjectEntryView) Suffix() path.Path { return v.entry.suffix.Clone() }

// SuffixSegmentCount returns the number of static suffix segments.
func (v ObjectEntryView) SuffixSegmentCount() int { return len(v.entry.suffix.Segments) }

// SuffixSegmentAt returns one static suffix segment by value.
func (v ObjectEntryView) SuffixSegmentAt(i int) (segment.Segment, bool) {
	if i < 0 || i >= len(v.entry.suffix.Segments) {
		return segment.Segment{}, false
	}
	return v.entry.suffix.Segments[i], true
}

// AppendSuffixTo appends the relative static suffix to root, copying the path
// once without exposing the stored suffix segments.
func (v ObjectEntryView) AppendSuffixTo(root path.Path) (path.Path, bool) {
	if root.IsEmpty() || len(v.entry.suffix.Segments) == 0 {
		return path.Path{}, false
	}
	return root.AppendSegments(v.entry.suffix.Segments), true
}

// SuffixSegments returns a defensive copy of the relative static suffix
// segments used to build the rootless static-member heap key.
func (v ObjectEntryView) SuffixSegments() []segment.Segment {
	if len(v.entry.suffix.Segments) == 0 {
		return nil
	}
	return append([]segment.Segment(nil), v.entry.suffix.Segments...)
}

// SuffixSegmentsView returns the relative static suffix as a read-only borrowed
// view. The returned slice is owned by the object literal sidecar and must not
// be mutated. Use SuffixSegments when the caller needs an owned slice.
func (v ObjectEntryView) SuffixSegmentsView() []segment.Segment {
	return v.entry.suffix.Segments
}

// WithExpected returns a copy carrying the contextual value contract for this
// entry.
func (e ObjectEntry) WithExpected(value product.Value) ObjectEntry {
	e = e.copy()
	e.expected = value
	e.hasExpected = true
	return e
}

func (e ObjectEntry) copy() ObjectEntry {
	e.suffix = e.suffix.Clone()
	return e
}

// ObjectLiteral describes static entries associated with an expression.
type ObjectLiteral struct {
	entries              []ObjectEntry
	listElementSource    ValueSource
	hasListElementSource bool
	expected             product.Value
	hasExpected          bool
	identity             identity.ID
}

// ObjectLiteralView provides read-only access to object literal entries without
// exposing mutable internal slices or path segment storage.
type ObjectLiteralView struct {
	literal ObjectLiteral
}

// NewObjectLiteral creates an object literal sidecar from static entries.
func NewObjectLiteral(entries []ObjectEntry) ObjectLiteral {
	return ObjectLiteral{entries: copyObjectEntries(entries)}
}

// View returns a read-only view of the owned object literal.
func (l ObjectLiteral) View() ObjectLiteralView { return ObjectLiteralView{literal: l} }

// Entries returns the static entries for this object literal.
func (l ObjectLiteral) Entries() []ObjectEntry { return copyObjectEntries(l.entries) }

// EntryCount returns the number of static entries.
func (v ObjectLiteralView) EntryCount() int { return len(v.literal.entries) }

// ForEachEntry visits static entries without allocating a defensive slice.
// Returning false stops iteration.
func (v ObjectLiteralView) ForEachEntry(fn func(ObjectEntryView) bool) {
	if fn == nil {
		return
	}
	for i := range v.literal.entries {
		if !fn(ObjectEntryView{entry: v.literal.entries[i]}) {
			return
		}
	}
}

// ListElementSource returns the value source for elements produced by an open
// list tail such as `{...}`. Unlike static entries, this proves the type of any
// yielded list element without proving that a particular numeric index exists.
func (v ObjectLiteralView) ListElementSource() (ValueSource, bool) {
	return v.literal.ListElementSource()
}

// Expected returns the declared contextual type value carried by this literal,
// if one was provided by lowering.
func (v ObjectLiteralView) Expected() (product.Value, bool) {
	return v.literal.Expected()
}

// Identity returns the stable literal identity attached during lowering, if
// this literal has one.
func (v ObjectLiteralView) Identity() (identity.ID, bool) {
	return v.literal.Identity()
}

// Expected returns the declared contextual type value the literal is assigned to,
// carried as a type-witness value so the factflow layer stays type-agnostic. The
// boolean reports whether a contextual record target is known for this literal.
func (l ObjectLiteral) Expected() (product.Value, bool) { return l.expected, l.hasExpected }

func (l ObjectLiteral) Identity() (identity.ID, bool) {
	if l.identity == (identity.ID{}) {
		return identity.ID{}, false
	}
	return l.identity, true
}

func (l ObjectLiteral) ListElementSource() (ValueSource, bool) {
	return l.listElementSource, l.hasListElementSource
}

// WithListElementSource returns a copy carrying the source of elements yielded
// by an open list tail in the table constructor.
func (l ObjectLiteral) WithListElementSource(source ValueSource) ObjectLiteral {
	out := l.copy()
	out.listElementSource = source
	out.hasListElementSource = true
	return out
}

// WithExpected returns a copy carrying the declared contextual type value.
func (l ObjectLiteral) WithExpected(value product.Value) ObjectLiteral {
	out := l.copy()
	out.expected = value
	out.hasExpected = true
	return out
}

// WithIdentity returns a copy carrying a stable literal identity.
func (l ObjectLiteral) WithIdentity(id identity.ID) ObjectLiteral {
	out := l.copy()
	out.identity = id
	return out
}

func (l ObjectLiteral) copy() ObjectLiteral {
	l.entries = copyObjectEntries(l.entries)
	return l
}

func copyObjectEntries(in []ObjectEntry) []ObjectEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]ObjectEntry, len(in))
	for i := range in {
		out[i] = in[i].copy()
	}
	return out
}

func copyObjectLiteralMap(in map[ExprRef]ObjectLiteral) map[ExprRef]ObjectLiteral {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ObjectLiteral, len(in))
	for expr, fact := range in {
		out[expr] = fact.copy()
	}
	return out
}
