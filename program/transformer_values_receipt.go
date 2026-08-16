package program

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// valuesCatalog is sealed once at Program publication. Rows are aligned to
// the exact Authored.Values denominator; no hot query reconstructs a row from
// Flow or keeps a downstream Values index.
type valuesCatalog struct {
	owner  *Program
	input  TransformerInput
	rows   []valuesCatalogRow
	byID   []valuesCatalogIndex
	sealed bool
}

type valuesCatalogRow struct {
	term      keyspace.Term
	rootPath  keyspace.ContentID
	bodyPath  keyspace.ContentID
	id        keyspace.ContentID
	owner     keyspace.Term
	width     int
	members   []valuesCatalogMember
	byID      []valuesCatalogIndex
	tail      keyspace.Term
	tailPath  keyspace.ContentID
	tailKind  TailProducerKind
	tailProof TailProducer
	span      Span
	spanOK    bool
}

type valuesCatalogMember struct {
	term   keyspace.Term
	id     keyspace.ContentID
	span   Span
	spanOK bool
}

type valuesCatalogIndex struct {
	id    keyspace.ContentID
	index uint32
}

func buildValuesCatalog(owner *Program) (*valuesCatalog, bool) {
	if owner == nil {
		return nil, false
	}
	input := owner.TransformerInput()
	if !input.Available() {
		return nil, false
	}
	authored := owner.Flow().Authored().Values()
	rows := make([]valuesCatalogRow, authored.Count())
	byID := make([]valuesCatalogIndex, authored.Count())
	for index := 0; index < authored.Count(); index++ {
		term, termOK := authored.At(index)
		rowOwner, tail, rowOK := authored.Get(term)
		width, widthOK := authored.Len(term)
		rootPath, rootOK := owner.Flow().SemanticTermPath(term)
		span, _ := input.Span(term)
		if !termOK || !rowOK || !widthOK || width < 0 || !rootOK || keyspace.TermFamily(rowOwner) != keyspace.FamilyBody {
			return nil, false
		}
		body, bodyOK := input.Body(rowOwner)
		bodyPath := body.PathID()
		if !bodyOK || !bodyPath.Available() {
			return nil, false
		}
		members := make([]valuesCatalogMember, width)
		for memberIndex := 0; memberIndex < width; memberIndex++ {
			memberTerm, memberOK := authored.Member(term, memberIndex)
			memberPath, pathOK := owner.Flow().SemanticTermPath(memberTerm)
			memberSpan, _ := input.Span(memberTerm)
			if !memberOK || !pathOK {
				return nil, false
			}
			memberID := valuesMemberID(rootPath, uint32(memberIndex), memberPath)
			if !memberID.Available() {
				return nil, false
			}
			members[memberIndex] = valuesCatalogMember{term: memberTerm, id: memberID, span: memberSpan, spanOK: memberSpan.context.Available()}
		}
		tailPath, tailKind, tailProof, tailOK := sealValuesTail(input, rootPath, tail)
		if !tailOK {
			return nil, false
		}
		rowID := valuesOccurrenceID(rootPath, members, tailPath, tailKind)
		if !rowID.Available() {
			return nil, false
		}
		rows[index] = valuesCatalogRow{term: term, rootPath: rootPath, bodyPath: bodyPath, id: rowID, owner: rowOwner, width: width, members: members, tail: tail, tailPath: tailPath, tailKind: tailKind, tailProof: tailProof, span: span, spanOK: span.context.Available()}
		rows[index].byID = make([]valuesCatalogIndex, width)
		for memberIndex := range members {
			rows[index].byID[memberIndex] = valuesCatalogIndex{id: members[memberIndex].id, index: uint32(memberIndex)}
		}
		radixSortValuesIndexes(rows[index].byID)
		byID[index] = valuesCatalogIndex{id: rows[index].id, index: uint32(index)}
	}
	radixSortValuesIndexes(byID)
	catalog := &valuesCatalog{owner: owner, input: input, rows: rows, byID: byID}
	for index := range catalog.rows {
		catalog.rows[index].tailProof.catalog = catalog
		catalog.rows[index].tailProof.rowIndex = index
	}
	if !validateValuesCatalog(catalog) {
		return nil, false
	}
	catalog.sealed = true
	return catalog, true
}

func sealValuesTail(input TransformerInput, rootPath keyspace.ContentID, tail keyspace.Term) (keyspace.ContentID, TailProducerKind, TailProducer, bool) {
	if tail == 0 {
		return keyspace.ContentID{}, TailProducerInvalid, TailProducer{}, true
	}
	path, pathOK := input.owner.Flow().SemanticTermPath(tail)
	span, spanOK := input.Span(tail)
	producer, producerOK := issueTailProducer(input, span)
	if !pathOK || !spanOK || !producerOK {
		return keyspace.ContentID{}, TailProducerInvalid, TailProducer{}, false
	}
	return path, producer.kind, producer, true
}

func (catalog *valuesCatalog) valid() bool {
	return catalog != nil && catalog.sealed && catalog.owner != nil && catalog.owner.valuesCatalog == catalog &&
		catalog.input.owner == catalog.owner && catalog.input.Available() &&
		len(catalog.rows) == len(catalog.byID)
}

// validateValuesCatalog is the cold seal check. It proves that the catalog is
// the complete Authored.Values denominator and that both immutable lookup
// directories are exact permutations of their source rows/members. Hot
// receipts only need the scalar sealed/self/owner fence in valid.
func validateValuesCatalog(catalog *valuesCatalog) bool {
	if catalog == nil || catalog.owner == nil || !catalog.input.Available() {
		return false
	}
	authored := catalog.owner.Flow().Authored().Values()
	if len(catalog.rows) != authored.Count() || len(catalog.byID) != len(catalog.rows) {
		return false
	}
	seenRows := make([]bool, len(catalog.rows))
	seenMembers := make(map[keyspace.ContentID]struct{})
	for index := range catalog.rows {
		row := &catalog.rows[index]
		term, termOK := authored.At(index)
		rowOwner, tail, rowOK := authored.Get(row.term)
		width, widthOK := authored.Len(row.term)
		if !termOK || row.term != term || !rowOK || rowOwner != row.owner || !widthOK || width != row.width || width != len(row.members) || width != len(row.byID) ||
			row.term != keyspace.MakeTerm(keyspace.FamilyValues, uint32(index+1)) || keyspace.TermFamily(row.owner) != keyspace.FamilyBody || row.tail != tail || !row.rootPath.Available() || !row.bodyPath.Available() || !row.id.Available() {
			return false
		}
		body, bodyOK := catalog.input.Body(row.owner)
		if !bodyOK || body.PathID() != row.bodyPath {
			return false
		}
		rootPath, rootPathOK := catalog.owner.Flow().SemanticTermPath(row.term)
		if !rootPathOK || rootPath != row.rootPath {
			return false
		}
		wantID := valuesOccurrenceID(row.rootPath, row.members, row.tailPath, row.tailKind)
		if wantID != row.id {
			return false
		}
		seenMembersRow := make([]bool, len(row.members))
		for memberIndex, member := range row.members {
			wantMember, memberOK := authored.Member(row.term, memberIndex)
			memberPath, memberPathOK := catalog.owner.Flow().SemanticTermPath(member.term)
			if !memberOK || member.term != wantMember || !memberPathOK || !member.id.Available() || valuesMemberID(row.rootPath, uint32(memberIndex), memberPath) != member.id {
				return false
			}
			if _, duplicate := seenMembers[member.id]; duplicate {
				return false
			}
			seenMembers[member.id] = struct{}{}
		}
		for directoryIndex, entry := range row.byID {
			if !entry.id.Available() || (directoryIndex > 0 && bytes.Compare(row.byID[directoryIndex-1].id[:], entry.id[:]) >= 0) || entry.index >= uint32(len(row.members)) || entry.id != row.members[entry.index].id || seenMembersRow[entry.index] {
				return false
			}
			seenMembersRow[entry.index] = true
		}
		for _, present := range seenMembersRow {
			if !present {
				return false
			}
		}
		if row.tail == 0 {
			if row.tailKind != TailProducerInvalid || row.tailPath.Available() || row.tailProof.Available() {
				return false
			}
		} else {
			wantPath, wantKind, wantProof, tailProofOK := sealValuesTail(catalog.input, row.rootPath, row.tail)
			if !tailProofOK || row.tailKind == TailProducerInvalid || !row.tailPath.Available() || wantPath != row.tailPath || wantKind != row.tailKind || !wantProof.path.Available() || row.tailProof.path != wantProof.path || row.tailProof.kind != wantProof.kind || row.tailProof.input != catalog.input || row.tailProof.catalog != catalog || row.tailProof.rowIndex != index || row.tailProof.span != wantProof.span || row.tailProof.context != wantProof.context || !row.tailProof.span.Available() || !row.tailProof.context.Available() {
				return false
			}
		}
	}
	for directoryIndex, entry := range catalog.byID {
		if !entry.id.Available() || (directoryIndex > 0 && bytes.Compare(catalog.byID[directoryIndex-1].id[:], entry.id[:]) >= 0) || entry.index >= uint32(len(catalog.rows)) || entry.id != catalog.rows[entry.index].id || seenRows[entry.index] {
			return false
		}
		seenRows[entry.index] = true
	}
	for _, present := range seenRows {
		if !present {
			return false
		}
	}
	return true
}

// radixSortValuesIndexes sorts fixed-width ContentID keys in linear time.
// ContentID is a 32-byte value; sorting least-significant bytes first keeps
// the directory stable while avoiding comparison-sort work at publication.
func radixSortValuesIndexes(values []valuesCatalogIndex) {
	if len(values) < 2 {
		return
	}
	tmp := make([]valuesCatalogIndex, len(values))
	for pass := len(values[0].id) - 1; pass >= 0; pass-- {
		var counts [256]int
		for _, value := range values {
			counts[value.id[pass]]++
		}
		total := 0
		for index, count := range counts {
			counts[index], total = total, total+count
		}
		for _, value := range values {
			bucket := value.id[pass]
			tmp[counts[bucket]] = value
			counts[bucket]++
		}
		copy(values, tmp)
	}
}

// ValuesCatalog is the immutable Program Values visitor.
type ValuesCatalog struct{ input TransformerInput }

func (input TransformerInput) Values() ValuesCatalog { return ValuesCatalog{input: input} }

// Available is the narrow canonical-catalog fence. It distinguishes a
// published empty Values denominator from a missing, foreign, or unsealed
// catalog without reopening Flow or Authored.
func (view ValuesCatalog) Available() bool {
	if view.input.owner == nil || view.input.owner.valuesCatalog == nil {
		return false
	}
	catalog := view.input.owner.valuesCatalog
	return catalog.input == view.input && catalog.valid()
}

func (view ValuesCatalog) catalog() *valuesCatalog {
	if !view.Available() {
		return nil
	}
	return view.input.owner.valuesCatalog
}

func (view ValuesCatalog) Count() int {
	catalog := view.catalog()
	if catalog == nil {
		return 0
	}
	return len(catalog.rows)
}

func (view ValuesCatalog) At(index int) (ValuesOccurrence, bool) {
	catalog := view.catalog()
	if catalog == nil || index < 0 || index >= len(catalog.rows) {
		return ValuesOccurrence{}, false
	}
	row := &catalog.rows[index]
	return ValuesOccurrence{catalog: catalog, issuer: catalog, index: index, issuedID: row.id}, true
}

func (view ValuesCatalog) ForID(id keyspace.ContentID) (ValuesOccurrence, bool) {
	catalog := view.catalog()
	if catalog == nil || !id.Available() {
		return ValuesOccurrence{}, false
	}
	index := sort.Search(len(catalog.byID), func(index int) bool { return bytes.Compare(catalog.byID[index].id[:], id[:]) >= 0 })
	if index >= len(catalog.byID) || catalog.byID[index].id != id {
		return ValuesOccurrence{}, false
	}
	rowIndex := int(catalog.byID[index].index)
	row := &catalog.rows[rowIndex]
	return ValuesOccurrence{catalog: catalog, issuer: catalog, index: rowIndex, issuedID: row.id}, true
}

func (input TransformerInput) valuesForTerm(term keyspace.Term) (ValuesOccurrence, bool) {
	if !input.Available() || keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 || input.owner.valuesCatalog == nil {
		return ValuesOccurrence{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	values, ok := (ValuesCatalog{input: input}).At(index)
	row, rowOK := values.row()
	return values, ok && rowOK && row.term == term
}

// ValuesOccurrence is an opaque row handle into the sealed Values catalog.
type ValuesOccurrence struct {
	catalog  *valuesCatalog
	issuer   *valuesCatalog
	index    int
	issuedID keyspace.ContentID
}

func (values ValuesOccurrence) row() (*valuesCatalogRow, bool) {
	if values.catalog == nil || values.issuer == nil || values.catalog != values.issuer || !values.catalog.valid() ||
		values.index < 0 || values.index >= len(values.catalog.rows) || !values.issuedID.Available() ||
		values.catalog.rows[values.index].id != values.issuedID {
		return nil, false
	}
	return &values.catalog.rows[values.index], true
}

func (values ValuesOccurrence) Available() bool { _, ok := values.row(); return ok }

func (values ValuesOccurrence) ID() keyspace.ContentID {
	row, ok := values.row()
	if !ok {
		return keyspace.ContentID{}
	}
	return row.id
}

func (values ValuesOccurrence) matchesTerm(term keyspace.Term) bool {
	row, ok := values.row()
	return ok && row.term == term
}

func (values ValuesOccurrence) ContextID() keyspace.ContentID { return values.ID() }

// BodyPathID returns the already-sealed owner-neutral Body path for this
// Values row. It never reopens Flow or exposes the authored owner Term.
func (values ValuesOccurrence) BodyPathID() keyspace.ContentID {
	row, ok := values.row()
	if !ok {
		return keyspace.ContentID{}
	}
	return row.bodyPath
}

// Span returns the exact Values-root span retained by the sealed Program
// catalog. Non-executable Values rows intentionally have no span. The proof
// is never reconstructed from the private authored coordinate.
func (values ValuesOccurrence) Span() (Span, bool) {
	row, ok := values.row()
	if !ok || !row.spanOK || !row.span.Available() || !values.catalog.input.OwnsSpan(row.span) {
		return Span{}, false
	}
	return row.span, true
}

func (values ValuesOccurrence) Count() int {
	row, ok := values.row()
	if !ok {
		return 0
	}
	return row.width
}

func (values ValuesOccurrence) At(index int) (ValuesMember, bool) {
	row, ok := values.row()
	if !ok || index < 0 || index >= len(row.members) {
		return ValuesMember{}, false
	}
	return ValuesMember{
		values:      values,
		catalog:     values.catalog,
		parentIndex: values.index,
		parentID:    values.ID(),
		index:       index,
		issuedID:    row.members[index].id,
	}, true
}

func (values ValuesOccurrence) ForID(id keyspace.ContentID) (ValuesMember, bool) {
	row, ok := values.row()
	if !ok || !id.Available() {
		return ValuesMember{}, false
	}
	left := sort.Search(len(row.byID), func(index int) bool { return bytes.Compare(row.byID[index].id[:], id[:]) >= 0 })
	if left < len(row.byID) && row.byID[left].id == id {
		memberIndex := int(row.byID[left].index)
		return ValuesMember{
			values:      values,
			catalog:     values.catalog,
			parentIndex: values.index,
			parentID:    values.ID(),
			index:       memberIndex,
			issuedID:    row.members[memberIndex].id,
		}, true
	}
	return ValuesMember{}, false
}

func (values ValuesOccurrence) Tail() (TailProducer, bool) {
	row, ok := values.row()
	if !ok || row.tail == 0 || !row.tailProof.Available() {
		return TailProducer{}, false
	}
	return row.tailProof, true
}

func (input TransformerInput) OwnsValuesOccurrence(values ValuesOccurrence) bool {
	return input.Available() && values.catalog != nil && values.catalog.owner == input.owner && input.owner.valuesCatalog == values.catalog && values.Available()
}

// ValuesMember is an opaque ordered child of one sealed Values row.
type ValuesMember struct {
	values      ValuesOccurrence
	catalog     *valuesCatalog
	parentIndex int
	parentID    keyspace.ContentID
	index       int
	issuedID    keyspace.ContentID
}

func (member ValuesMember) row() (*valuesCatalogMember, bool) {
	if member.catalog == nil || member.values.catalog != member.catalog || member.values.index != member.parentIndex ||
		member.values.issuer != member.catalog || member.values.ID() != member.parentID || !member.issuedID.Available() {
		return nil, false
	}
	valuesRow, ok := member.values.row()
	if !ok || member.index < 0 || member.index >= len(valuesRow.members) || valuesRow.members[member.index].id != member.issuedID {
		return nil, false
	}
	return &valuesRow.members[member.index], true
}

func (member ValuesMember) Available() bool { _, ok := member.row(); return ok }

func (member ValuesMember) ID() keyspace.ContentID {
	row, ok := member.row()
	if !ok {
		return keyspace.ContentID{}
	}
	return row.id
}

func (member ValuesMember) ContextID() keyspace.ContentID { return member.ID() }

func (member ValuesMember) Position() (int, bool) {
	if !member.Available() {
		return 0, false
	}
	return member.index, true
}

func (member ValuesMember) Values() (ValuesOccurrence, bool) {
	if !member.Available() {
		return ValuesOccurrence{}, false
	}
	return member.values, true
}

func (member ValuesMember) Span() (Span, bool) {
	row, ok := member.row()
	if !ok {
		return Span{}, false
	}
	return row.span, row.spanOK
}

func (input TransformerInput) OwnsValuesMember(member ValuesMember) bool {
	return input.Available() && member.values.catalog != nil && member.values.catalog.owner == input.owner && input.owner.valuesCatalog == member.values.catalog && member.Available()
}

func valuesOccurrenceID(rootPath keyspace.ContentID, members []valuesCatalogMember, tailPath keyspace.ContentID, tailKind TailProducerKind) keyspace.ContentID {
	return transformerSemanticID("program/transformer/values-occurrence", func(writer *canonical.Writer) bool {
		if writer.Bytes(rootPath[:]) != nil || writer.Uint(uint64(len(members))) != nil {
			return false
		}
		for _, member := range members {
			if writer.Bytes(member.id[:]) != nil {
				return false
			}
		}
		return writer.Uint(uint64(tailKind)) == nil && writer.Bytes(tailPath[:]) == nil
	})
}

func valuesMemberID(rootPath keyspace.ContentID, index uint32, memberPath keyspace.ContentID) keyspace.ContentID {
	return transformerSemanticID("program/transformer/values-member", func(writer *canonical.Writer) bool {
		return writer.Bytes(rootPath[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(memberPath[:]) == nil
	})
}
