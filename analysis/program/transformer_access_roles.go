package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

// IndexReads and IndexWrites are zero-copy views over the two candidate planes
// already sealed by Flow.AccessGeometry. They retain no candidate rows or
// lookup relation; At re-queries the one immutable Flow projection.
type IndexReads struct{ input TransformerInput }
type IndexWrites struct{ input TransformerInput }

func (input TransformerInput) IndexReads() IndexReads   { return IndexReads{input: input} }
func (input TransformerInput) IndexWrites() IndexWrites { return IndexWrites{input: input} }

func (view IndexReads) Count() int {
	geometry, ok := view.input.accessGeometry()
	if !ok {
		return 0
	}
	return geometry.IndexAccesses().Reads().Count()
}

func (view IndexWrites) Count() int {
	geometry, ok := view.input.accessGeometry()
	if !ok {
		return 0
	}
	return geometry.IndexAccesses().Writes().Count()
}

// At returns the existing candidate occurrence in Flow's canonical read
// order. The returned role contains only opaque subordinate proofs.
func (view IndexReads) At(index int) (IndexRead, bool) {
	geometry, ok := view.input.accessGeometry()
	if !ok || index < 0 || index >= geometry.IndexAccesses().Reads().Count() {
		return IndexRead{}, false
	}
	read, readOK := geometry.IndexAccesses().Reads().At(index)
	base, keyTerm, lens, geometryOK := geometry.IndexAccesses().Reads().Get(read)
	span, spanOK := view.input.Span(read)
	if !readOK || !geometryOK || !spanOK {
		return IndexRead{}, false
	}
	role := IndexRead{
		input:  view.input,
		slot:   index,
		read:   read,
		span:   span,
		base:   newIndexBase(view.input, read, base),
		lens:   newIndexLens(view.input, read, lens, keyTerm),
		result: IndexResult{input: view.input, access: read, term: read, span: span},
	}
	return role, role.Available()
}

// At returns the existing candidate occurrence in Flow's canonical write
// order. Values and Position are preserved as private geometry, never as raw
// public terms.
func (view IndexWrites) At(index int) (IndexWrite, bool) {
	geometry, ok := view.input.accessGeometry()
	if !ok || index < 0 || index >= geometry.IndexAccesses().Writes().Count() {
		return IndexWrite{}, false
	}
	write, writeOK := geometry.IndexAccesses().Writes().At(index)
	base, keyTerm, values, position, lens, geometryOK := geometry.IndexAccesses().Writes().Get(write)
	span, _ := view.input.Span(write)
	finish, finishOK := view.input.occurrenceFinish(write)
	predecessor, predecessorOK := view.input.assignmentPredecessor(write, finish)
	if !writeOK || !geometryOK || !finishOK || !predecessorOK {
		return IndexWrite{}, false
	}
	role := IndexWrite{
		input:  view.input,
		slot:   index,
		write:  write,
		span:   span,
		base:   newIndexBase(view.input, write, base),
		lens:   newIndexLens(view.input, write, lens, keyTerm),
		values: newIndexValues(view.input, write, values, position),
		finish: finish, predecessor: predecessor,
	}
	return role, role.Available()
}

// IndexRead is an opaque Program-owned IndexGet occurrence. Its source
// occurrence, receiver geometry, lens, and result proof are all borrowed from
// the sealed Flow projection; no raw term or candidate ordinal is public.
type IndexRead struct {
	input  TransformerInput
	slot   int
	read   keyspace.Term
	span   Span
	base   IndexBase
	lens   IndexLens
	result IndexResult
}

func (read IndexRead) Available() bool {
	if !read.input.Available() || read.slot < 0 || !exactSpan(read.input, read.span, read.read) || !read.base.Available() || !read.lens.Available() || !read.result.Available() {
		return false
	}
	if !read.exactlyComposedBy(read.input) || read.result.access != read.read {
		return false
	}
	geometry, ok := read.input.accessGeometry()
	if !ok || read.slot >= geometry.IndexAccesses().Reads().Count() {
		return false
	}
	current, currentOK := geometry.IndexAccesses().Reads().At(read.slot)
	base, keyTerm, lens, geometryOK := geometry.IndexAccesses().Reads().Get(current)
	return currentOK && geometryOK && current == read.read && base == read.base.term && lens == read.lens.term && keyTerm == read.lens.keyTerm
}

func (read IndexRead) exactlyComposedBy(input TransformerInput) bool {
	return read.input == input &&
		read.base.input == input && read.base.access == read.read &&
		read.lens.input == input && read.lens.access == read.read &&
		read.result.input == input && read.result.access == read.read
}

// ContextID is stable across equivalent Program replay and fenced by the
// published Program identity. It is not a Link or Heap identity.
func (read IndexRead) ContextID() identity.ContentID {
	if !read.Available() {
		return identity.ContentID{}
	}
	return accessRoleID("program/transformer/index-read", read.input, read.read)
}

func (read IndexRead) Equal(other IndexRead) bool {
	left, right := read.ContextID(), other.ContextID()
	return left.Available() && left == right
}

func (read IndexRead) Span() (Span, bool) {
	if !read.Available() {
		return Span{}, false
	}
	return read.span, true
}
func (read IndexRead) Base() (IndexBase, bool) {
	if !read.Available() {
		return IndexBase{}, false
	}
	return read.base, true
}
func (read IndexRead) Lens() (IndexLens, bool) {
	if !read.Available() {
		return IndexLens{}, false
	}
	return read.lens, true
}
func (read IndexRead) Result() (IndexResult, bool) {
	if !read.Available() {
		return IndexResult{}, false
	}
	return read.result, true
}

// IndexWrite is an opaque Program-owned IndexSet occurrence. CommitSpan is
// the existing Flow span whose Finish site is the assignment commit anchor.
type IndexWrite struct {
	input       TransformerInput
	slot        int
	write       keyspace.Term
	span        Span
	base        IndexBase
	lens        IndexLens
	values      IndexValues
	finish      flow.Site
	predecessor AssignmentPredecessor
}

func (write IndexWrite) Available() bool {
	if !write.input.Available() || write.slot < 0 || !exactOptionalSpan(write.input, write.span, write.write) || !write.base.Available() || !write.lens.Available() || !write.values.Available() ||
		!write.input.OwnsSite(write.finish) || !write.input.OwnsAssignmentPredecessor(write.predecessor) {
		return false
	}
	if !write.exactlyComposedBy(write.input) {
		return false
	}
	geometry, ok := write.input.accessGeometry()
	if !ok || write.slot >= geometry.IndexAccesses().Writes().Count() {
		return false
	}
	current, currentOK := geometry.IndexAccesses().Writes().At(write.slot)
	base, keyTerm, values, position, lens, geometryOK := geometry.IndexAccesses().Writes().Get(current)
	return currentOK && geometryOK && current == write.write && base == write.base.term && lens == write.lens.term && keyTerm == write.lens.keyTerm && values == write.values.term && position == write.values.position &&
		write.predecessor.write == write.write && write.predecessor.finish.Equal(write.finish)
}

func (write IndexWrite) exactlyComposedBy(input TransformerInput) bool {
	return write.input == input &&
		write.base.input == input && write.base.access == write.write &&
		write.lens.input == input && write.lens.access == write.write &&
		write.values.input == input && write.values.access == write.write &&
		write.predecessor.input == input && write.predecessor.write == write.write
}

func (write IndexWrite) ContextID() identity.ContentID {
	if !write.Available() {
		return identity.ContentID{}
	}
	return accessRoleID("program/transformer/index-write", write.input, write.write)
}

func (write IndexWrite) Equal(other IndexWrite) bool {
	left, right := write.ContextID(), other.ContextID()
	return left.Available() && left == right
}

func (write IndexWrite) Span() (Span, bool) {
	if !write.Available() || !write.span.Available() {
		return Span{}, false
	}
	return write.span, true
}

func (write IndexWrite) CommitSpan() (Span, bool) { return write.Span() }

// Finish is the exact causal Write commit site. Index writes need not have a
// complete Entry/Finish Span: their semantic input is the owner-issued
// assignment predecessor, while this finish remains the commit attachment.
func (write IndexWrite) Finish() (flow.Site, bool) {
	if !write.Available() {
		return flow.Site{}, false
	}
	return write.finish, true
}

// Predecessor returns the sole parent-issued assignment commit route. It is
// the raw-set rule input; callers must not scan incoming structural routes.
func (write IndexWrite) Predecessor() (AssignmentPredecessor, bool) {
	if !write.Available() {
		return AssignmentPredecessor{}, false
	}
	return write.predecessor, true
}

func (write IndexWrite) Base() (IndexBase, bool) {
	if !write.Available() {
		return IndexBase{}, false
	}
	return write.base, true
}
func (write IndexWrite) Lens() (IndexLens, bool) {
	if !write.Available() {
		return IndexLens{}, false
	}
	return write.lens, true
}
func (write IndexWrite) Values() (IndexValues, bool) {
	if !write.Available() {
		return IndexValues{}, false
	}
	return write.values, true
}

// IndexBase is an opaque existing evaluated receiver-base proof.
type IndexBase struct {
	input  TransformerInput
	access keyspace.Term
	term   keyspace.Term
	span   Span
}

func (base IndexBase) Available() bool {
	if !base.input.Available() || !validAccessTerm(base.access) || !validAccessTerm(base.term) || !exactOptionalSpan(base.input, base.span, base.term) {
		return false
	}
	if readBase, _, _, ok := readGeometry(base.input, base.access); ok {
		return readBase == base.term
	}
	if writeBase, _, _, _, _, ok := writeGeometry(base.input, base.access); ok {
		return writeBase == base.term
	}
	return false
}

func (base IndexBase) ContextID() identity.ContentID {
	if !base.Available() {
		return identity.ContentID{}
	}
	return accessSubroleID("program/transformer/index-base", base.input, base.access, base.term)
}

func (base IndexBase) Span() (Span, bool) {
	if !base.Available() || !base.span.Available() {
		return Span{}, false
	}
	return base.span, true
}

// IndexLensKind is the closed exact/dynamic lens geometry sum.
type IndexLensKind uint8

const (
	IndexLensInvalid IndexLensKind = iota
	IndexLensExact
	IndexLensDynamic
)

// IndexLens is an opaque existing Lens proof. Exact keys are normalized by
// Flow's Source authority; dynamic lenses intentionally have no key identity.
type IndexLens struct {
	input    TransformerInput
	access   keyspace.Term
	term     keyspace.Term
	keyTerm  keyspace.Term
	kind     IndexLensKind
	exactKey keyspace.Key
	exact    bool
	source   Span
}

func (lens IndexLens) Available() bool {
	if !lens.input.Available() || !validAccessTerm(lens.access) || !validAccessTerm(lens.term) || !validAccessTerm(lens.keyTerm) || !exactOptionalSpan(lens.input, lens.source, lens.keyTerm) {
		return false
	}
	_, keyTerm, currentLens, ok := readGeometry(lens.input, lens.access)
	if !ok {
		_, keyTerm, _, _, currentLens, ok = writeGeometry(lens.input, lens.access)
	}
	if !ok || currentLens != lens.term || keyTerm != lens.keyTerm {
		return false
	}
	view := lens.input.owner.Flow().AccessGeometry()
	switch lens.kind {
	case IndexLensExact:
		key, ok := view.ExactLenses().Get(lens.term)
		return ok && lens.exact && key == lens.exactKey
	case IndexLensDynamic:
		_, ok := view.DynamicLenses().Get(lens.term)
		return ok && lens.exactKey == 0
	default:
		return false
	}
}

func (lens IndexLens) ContextID() identity.ContentID {
	if !lens.Available() {
		return identity.ContentID{}
	}
	domain := "program/transformer/index-lens-dynamic"
	if lens.kind == IndexLensExact {
		domain = "program/transformer/index-lens-exact"
	}
	return accessSubroleID(domain, lens.input, lens.access, lens.term)
}

func (lens IndexLens) Kind() IndexLensKind {
	if !lens.Available() {
		return IndexLensInvalid
	}
	return lens.kind
}

func (lens IndexLens) ExactKey() (keyspace.Key, bool) {
	if !lens.Available() || lens.kind != IndexLensExact {
		return 0, false
	}
	return lens.exactKey, true
}

// Source is optional because an exact literal lens can have no executable
// Flow span. Dynamic key sources normally do have one.
func (lens IndexLens) Source() (Span, bool) {
	if !lens.Available() || !lens.source.Available() {
		return Span{}, false
	}
	return lens.source, true
}

// IndexResult is the existing read result proof. It is deliberately separate
// from IndexRead so later bindings cannot mistake a write for a read result.
type IndexResult struct {
	input  TransformerInput
	access keyspace.Term
	term   keyspace.Term
	span   Span
}

func (result IndexResult) Available() bool {
	if !result.input.Available() || !validAccessTerm(result.access) || !validAccessTerm(result.term) || !exactSpan(result.input, result.span, result.access) {
		return false
	}
	_, _, _, ok := readGeometry(result.input, result.access)
	return ok && result.term == result.access
}

func (result IndexResult) ContextID() identity.ContentID {
	if !result.Available() {
		return identity.ContentID{}
	}
	return accessSubroleID("program/transformer/index-result", result.input, result.access, result.term)
}

func (result IndexResult) Span() (Span, bool) {
	if !result.Available() {
		return Span{}, false
	}
	return result.span, true
}

// IndexValues is the existing assignment-values proof plus its fixed
// authored position. Position is geometry, not a public Write ordinal.
type IndexValues struct {
	input    TransformerInput
	access   keyspace.Term
	term     keyspace.Term
	position int
	span     Span
}

func (values IndexValues) Available() bool {
	if !values.input.Available() || !validAccessTerm(values.access) || !validAccessTerm(values.term) || values.position < 0 || !exactOptionalSpan(values.input, values.span, values.term) {
		return false
	}
	_, _, currentValues, currentPosition, _, ok := writeGeometry(values.input, values.access)
	return ok && currentValues == values.term && currentPosition == values.position
}

func (values IndexValues) ContextID() identity.ContentID {
	if !values.Available() {
		return identity.ContentID{}
	}
	return accessSubroleID("program/transformer/index-values", values.input, values.access, values.term, uint64(values.position))
}

// Position is the fixed authored Values member position for this exact
// assignment payload; it is scalar geometry, not a source coordinate.
func (values IndexValues) Position() int {
	if !values.Available() {
		return -1
	}
	return values.position
}

func (values IndexValues) Span() (Span, bool) {
	if !values.Available() || !values.span.Available() {
		return Span{}, false
	}
	return values.span, true
}

// Occurrence returns the exact existing Values denominator member for this
// write payload. It lets compilers preserve its identity without reopening
// Flow from a Link-local domain.
func (values IndexValues) Occurrence() (ValuesOccurrence, bool) {
	if !values.Available() {
		return ValuesOccurrence{}, false
	}
	occurrence, ok := values.input.valuesForTerm(values.term)
	return occurrence, ok && values.input.OwnsValuesOccurrence(occurrence)
}

func readGeometry(input TransformerInput, access keyspace.Term) (base, keyTerm, lens keyspace.Term, ok bool) {
	geometry, available := input.accessGeometry()
	if !available || !validAccessTerm(access) {
		return 0, 0, 0, false
	}
	return geometry.IndexAccesses().Reads().Get(access)
}

func writeGeometry(input TransformerInput, access keyspace.Term) (base, keyTerm, values keyspace.Term, position int, lens keyspace.Term, ok bool) {
	geometry, available := input.accessGeometry()
	if !available || !validAccessTerm(access) {
		return 0, 0, 0, 0, 0, false
	}
	return geometry.IndexAccesses().Writes().Get(access)
}

func exactSpan(input TransformerInput, span Span, term keyspace.Term) bool {
	if !input.Available() || !span.Available() || !input.OwnsSpan(span) {
		return false
	}
	want, ok := input.Span(term)
	return ok && span.Equal(want)
}

func exactOptionalSpan(input TransformerInput, span Span, term keyspace.Term) bool {
	if !input.Available() {
		return false
	}
	want, wantOK := input.Span(term)
	gotOK := span.Available()
	if wantOK != gotOK {
		return false
	}
	return !gotOK || (input.OwnsSpan(span) && span.Equal(want))
}

func (input TransformerInput) OwnsIndexRead(read IndexRead) bool {
	return input.Available() && read.exactlyComposedBy(input) && read.Available()
}

func (input TransformerInput) OwnsIndexWrite(write IndexWrite) bool {
	return input.Available() && write.exactlyComposedBy(input) && write.Available()
}

func (input TransformerInput) accessGeometry() (flow.AccessGeometry, bool) {
	if !input.Available() {
		return flow.AccessGeometry{}, false
	}
	geometry := input.owner.Flow().AccessGeometry()
	if !geometry.Available() {
		return flow.AccessGeometry{}, false
	}
	return geometry, true
}

func newIndexBase(input TransformerInput, access, term keyspace.Term) IndexBase {
	span, _ := input.Span(term)
	return IndexBase{input: input, access: access, term: term, span: span}
}

func newIndexLens(input TransformerInput, access, lens, keyTerm keyspace.Term) IndexLens {
	view := input.owner.Flow().AccessGeometry()
	if key, ok := view.ExactLenses().Get(lens); ok {
		source, _ := input.Span(keyTerm)
		return IndexLens{input: input, access: access, term: lens, keyTerm: keyTerm, kind: IndexLensExact, exactKey: key, exact: true, source: source}
	}
	if _, ok := view.DynamicLenses().Get(lens); ok {
		source, _ := input.Span(keyTerm)
		return IndexLens{input: input, access: access, term: lens, keyTerm: keyTerm, kind: IndexLensDynamic, source: source}
	}
	return IndexLens{}
}

func newIndexValues(input TransformerInput, access, term keyspace.Term, position int) IndexValues {
	span, _ := input.Span(term)
	return IndexValues{input: input, access: access, term: term, position: position, span: span}
}

func accessRoleID(domain string, input TransformerInput, term keyspace.Term, values ...uint64) identity.ContentID {
	if !input.Available() || !validAccessTerm(term) {
		return identity.ContentID{}
	}
	return transformerRoleID(domain, input.programID, func(writer *framing.Writer) bool {
		if !writeAccessSemantic(writer, input, term) {
			return false
		}
		for _, value := range values {
			if writer.Uint(value) != nil {
				return false
			}
		}
		return true
	})
}

func accessSubroleID(domain string, input TransformerInput, access, term keyspace.Term, values ...uint64) identity.ContentID {
	if !input.Available() || !validAccessTerm(access) || !validAccessTerm(term) {
		return identity.ContentID{}
	}
	return transformerRoleID(domain, input.programID, func(writer *framing.Writer) bool {
		if !writeAccessSemantic(writer, input, access) || !writeAccessSemantic(writer, input, term) {
			return false
		}
		for _, value := range values {
			if writer.Uint(value) != nil {
				return false
			}
		}
		return true
	})
}

// writeAccessSemantic commits Flow's sole body-qualified semantic path.
// Spans are schedule geometry and may legitimately be absent for Write
// coordinates; neither a dense authored ordinal nor a family-only fallback
// may become a reusable occurrence identity.
func writeAccessSemantic(writer *framing.Writer, input TransformerInput, term keyspace.Term) bool {
	if writer == nil || !validAccessTerm(term) {
		return false
	}
	path, ok := input.owner.Flow().SemanticTermPath(term)
	return ok && path.Available() && writer.Bytes(path[:]) == nil
}

func validAccessTerm(term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0
}
