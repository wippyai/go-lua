package factor

import (
	"encoding/binary"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// PublicationMemberDomain separates the subject-member population from every
// other identity derived over the same Link. It derives each member's own
// coordinate here and the member universe the published column proves absence
// against, and it is exported because those two folds live in two packages: a
// domain spelled twice is two domains the moment one of them is edited.
const PublicationMemberDomain = "analysis/effect-publication-subject-member/v1"

// PublicationRow is one admitted publication, detached from the algebra that
// sealed it.
//
// It carries no owner pointer and no mounted Pack input: a directory row
// crosses to readers that hold neither, and a row that carried a live input
// would publish a capability as a fact. The subject and context are therefore
// the identities those inputs seal to, which is what a consumer correlates
// with, and the descriptor is the authored disposition rather than a
// re-derivation of it.
type PublicationRow struct {
	// ID is the publication receipt's own seal, and the coordinate this row
	// is published at.
	ID identity.ContentID
	// The mounted provenance: which call, in which module, in which
	// application, this publication belongs to.
	Module      identity.ContentID
	Call        identity.ContentID
	Application identity.ContentID
	// The authored descriptor and the identities Target sealed it under.
	Kind         vocabulary.PublicationEffectKind
	Escape       vocabulary.PublicationEscapeDisposition
	Mutability   vocabulary.PublicationMutabilityDisposition
	Lifetime     vocabulary.PublicationLifetimeDisposition
	DescriptorID identity.ContentID
	OccurrenceID identity.ContentID
	// The operation coordinate this publication is one effect of. A zero
	// Callback is the ordinary role and a non-zero one the callback role:
	// Effect distinguishes them by exactly this coordinate, so the row states
	// it once rather than carrying a second name for the same distinction.
	Operation vocabulary.Operation
	Callback  vocabulary.CallbackID
	Effect    uint32
	// The canonical subject the publication acts on, and the context it acts
	// in when the descriptor authored a destination. HasContext is explicit
	// because an absent context is a declared shape, not a zero identity.
	Subject    identity.ContentID
	Context    identity.ContentID
	HasContext bool
	// SubjectOpen states that the subject pack is open: its membership is not
	// closed at this Link, so the members below are what the pack authenticates
	// here rather than everything it will ever carry.
	SubjectOpen bool
	// SubjectOffset and SubjectLength are the half-open span of the member
	// column this publication's subject members occupy. The members are a
	// column rather than a field because a row is a pointer-free fact: a
	// slice on the row would put a live header in a Link-lifetime published
	// value, which is the same thing the mounted input is kept out for.
	SubjectOffset uint32
	SubjectLength uint32
}

// Available reports whether this row states a complete admitted publication.
// Absence is never a partially filled row: a row that lost its identity, its
// provenance, or its authored disposition is not a weaker publication, it is
// not one.
func (row PublicationRow) Available() bool {
	if !row.ID.Available() || !row.Module.Available() || !row.Call.Available() || !row.Application.Available() {
		return false
	}
	if !row.DescriptorID.Available() || !row.OccurrenceID.Available() || !row.Subject.Available() {
		return false
	}
	if row.HasContext != row.Context.Available() || row.Operation == 0 {
		return false
	}
	return row.Kind != vocabulary.PublicationEffectInvalid &&
		row.Escape != vocabulary.PublicationEscapeInvalid &&
		row.Mutability != vocabulary.PublicationMutabilityInvalid &&
		row.Lifetime != vocabulary.PublicationLifetimeInvalid
}

// MountedAt reports whether this row belongs to one exact mounted call. It is
// the provenance read a consumer selects a call's publications by, and it is
// the row's own statement rather than a join the caller assembles.
func (row PublicationRow) MountedAt(module, call identity.ContentID) bool {
	return row.Available() && row.Module == module && row.Call == call
}

// PublicationDirectory is one Link's whole publication statement: the
// admitted receipts and the mounted calls they were admitted on. The two are
// built in one walk so a call's row span is the position its receipts
// actually occupy rather than an offset a second pass recomputed.
type PublicationDirectory struct {
	Rows    []PublicationRow
	Calls   []PublicationCall
	Members []PublicationSubject
}

// MemberRows is the published projection of this directory's subject members:
// what each member is, without the Value handle the algebra resolved for it.
// It is the form the member column seals, so the column states published rows
// and never holds a coordinate that cannot leave the schema that issued it.
func (directory PublicationDirectory) CallRows() []PublicationCallRow {
	rows := make([]PublicationCallRow, 0, len(directory.Calls))
	for index := range directory.Calls {
		rows = append(rows, directory.Calls[index].Row())
	}
	return rows
}

// MemberRows is the published projection of this directory's subject members.
func (directory PublicationDirectory) MemberRows() []PublicationMemberRow {
	rows := make([]PublicationMemberRow, 0, len(directory.Members))
	for index := range directory.Members {
		rows = append(rows, directory.Members[index].Row())
	}
	return rows
}

// PublicationMemberRow is one semantic member of one publication's subject
// pack.
type PublicationMemberRow struct {
	// ID is this member's coordinate in the member column, derived from the
	// row it belongs to and its position in that row's pack.
	ID identity.ContentID
	// RowID is the publication whose subject names this member.
	RowID identity.ContentID
	// Semantic is the member identity Value's mounted coordinate is keyed by.
	Semantic identity.ContentID
	// Member is the member's position in its pack's own order.
	Member uint32
}

// PublicationSubject is one member of a publication's subject pack as the
// algebra that sealed it holds one.
//
// It states the member and the tag a selection pairs its cell by, and nothing
// about where that member's value lives: a Value coordinate is a handle
// Value's Schema issues, and this algebra holds no Value schema. Which member
// a publication names is Effect's statement; where that member is, is Value's,
// answered by Value's own relation over these members.
type PublicationSubject struct {
	row PublicationMemberRow
}

// Row is the published member this subject stands for.
func (subject PublicationSubject) Row() PublicationMemberRow { return subject.row }

// Predicate is the tag a selection over these members pairs its cells by. It
// folds the member's own identity, so it names exactly the member the
// directory admitted and two publications naming one semantic member carry
// two tags.
func (subject PublicationSubject) Predicate() (tag uint64, ok bool) {
	if !subject.row.Available() {
		return 0, false
	}
	return PublicationMemberTag(subject.row.ID)
}

// PublicationMemberTag folds one subject member's identity to the selection
// tag its cell is paired by. It is the one derivation of that tag: a consumer
// that folded its own would be a second authority over which cell is which
// member.
func PublicationMemberTag(id identity.ContentID) (uint64, bool) {
	if !id.Available() {
		return 0, false
	}
	tag := binary.BigEndian.Uint64(id[:8])
	if tag == 0 {
		tag = 1
	}
	return tag, true
}

// Available reports whether this row states a complete subject member.
func (row PublicationMemberRow) Available() bool {
	return row.ID.Available() && row.RowID.Available() && row.Semantic.Available()
}

// PublicationMemberID derives one subject member's coordinate. It is the row
// it belongs to and its position in that row's pack, so two publications
// naming the same semantic member are two members here and neither hides the
// other.
func PublicationMemberID(rowID identity.ContentID, member int) (identity.ContentID, bool) {
	if !rowID.Available() || member < 0 {
		return identity.ContentID{}, false
	}
	var position [8]byte
	binary.BigEndian.PutUint64(position[:], uint64(member))
	return identity.DeriveContentID(PublicationMemberDomain, rowID[:], position[:])
}

// PublicationCallRow is one mounted call Effect admitted publications on, and
// the span of the row column its receipts occupy.
//
// The span may be empty. That is the point of this column: a call Effect
// mounted and the program authored no publication on is a fact the row column
// cannot state, because a row column states only rows. A consumer asking
// "what did this call publish" reads an answer either way, and never has to
// treat "no rows found" as ambiguous between an empty call and a call the
// directory does not know.
type PublicationCallRow struct {
	// ID is the batch's own seal, and the coordinate this row is published at.
	ID identity.ContentID
	// The mounted provenance this call is addressed by.
	Module      identity.ContentID
	Call        identity.ContentID
	Application identity.ContentID
	// RowOffset and RowLength are the half-open span [RowOffset, RowOffset+
	// RowLength) of the row column's sealed order that this call's receipts
	// occupy. Effect's enumeration is per mounted call, so a call's receipts
	// are contiguous and the span is exact rather than a filter predicate.
	RowOffset uint32
	RowLength uint32
}

// PublicationCall is one mounted call as the algebra that sealed it holds
// one: the published row and the subject members its receipts name.
//
// The published row states a span; this states the members that span covers,
// because a nested member set is enumerated through its parent and a published
// row carries no slice to enumerate.
type PublicationCall struct {
	row      PublicationCallRow
	subjects []PublicationSubject
}

// Row is the published call this stands for.
func (call PublicationCall) Row() PublicationCallRow { return call.row }

// MemberCount is the number of subject members this call's receipts name.
func (call PublicationCall) MemberCount() int { return len(call.subjects) }

// MemberAt is one subject member in the call's own sealed member order.
func (call PublicationCall) MemberAt(index int) (PublicationSubject, bool) {
	if index < 0 || index >= len(call.subjects) {
		return PublicationSubject{}, false
	}
	return call.subjects[index], true
}

// Available reports whether this row states a complete mounted call. A zero
// length is a complete statement; a lost identity is not.
func (row PublicationCallRow) Available() bool {
	return row.ID.Available() && row.Module.Available() && row.Call.Available() && row.Application.Available()
}

// DetachPublications reads one Link's whole publication directory out of
// Effect's sealed batches, in Effect's own mounted-call order and each call's
// canonical receipt order. The enumeration is Effect's sealed batch
// directory, so the rows are exactly the receipts it admitted and the
// directory cannot state one Effect never issued.
//
// The Value schema is required because a row proves its subject members
// resolve before it is admitted. Publishing a member a consumer could not
// join would move the failure from this seal to every read of it.
//
// A Link that admitted no publication yields an empty directory, which is
// that statement rather than a failure.
func DetachPublications(owner *Algebra) (PublicationDirectory, bool) {
	if owner == nil || !owner.Valid() {
		return PublicationDirectory{}, false
	}
	count := owner.MountedCallCount()
	directory := PublicationDirectory{
		Rows: make([]PublicationRow, 0), Calls: make([]PublicationCall, 0, count),
		Members: make([]PublicationSubject, 0),
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		mounted, mountedOK := owner.MountedCallAt(ordinal)
		if !mountedOK {
			return PublicationDirectory{}, false
		}
		batch, batchOK := owner.PublicationBatchForMountedCall(mounted)
		if !batchOK {
			return PublicationDirectory{}, false
		}
		batchID, batchIDOK := batch.SealedContentID()
		module, call, provenanceOK := batch.CallProvenance()
		application, applicationOK := batch.ApplicationID()
		if !batchIDOK || !provenanceOK || !applicationOK {
			return PublicationDirectory{}, false
		}
		offset := uint32(len(directory.Rows))
		memberOffset := len(directory.Members)
		for position := 0; position < batch.RowCount(); position++ {
			receipt, receiptOK := batch.RowAt(position)
			if !receiptOK {
				return PublicationDirectory{}, false
			}
			row, members, rowOK := detach(receipt, uint32(len(directory.Members)))
			if !rowOK || !row.MountedAt(module, call) {
				return PublicationDirectory{}, false
			}
			directory.Rows = append(directory.Rows, row)
			directory.Members = append(directory.Members, members...)
		}
		callRow := PublicationCallRow{
			ID: batchID, Module: module, Call: call, Application: application,
			RowOffset: offset, RowLength: uint32(len(directory.Rows)) - offset,
		}
		if !callRow.Available() {
			return PublicationDirectory{}, false
		}
		directory.Calls = append(directory.Calls, PublicationCall{
			row: callRow, subjects: directory.Members[memberOffset:],
		})
	}
	return directory, true
}

// detach reads one sealed receipt into the published row. Every field is
// taken from the receipt's own accessors, so a receipt that stopped
// authenticating cannot reach the directory as a partially read row.
func detach(receipt MountedPublication, memberOffset uint32) (PublicationRow, []PublicationSubject, bool) {
	id, idOK := receipt.ContentID()
	module, call, provenanceOK := receipt.CallProvenance()
	application, applicationOK := receipt.ApplicationID()
	descriptorID, descriptorIDOK := receipt.DescriptorID()
	occurrenceID, occurrenceIDOK := receipt.OccurrenceID()
	subject, subjectOK := receipt.SubjectInput()
	if !idOK || !provenanceOK || !applicationOK || !descriptorIDOK || !occurrenceIDOK || !subjectOK {
		return PublicationRow{}, nil, false
	}
	subjectID, subjectIDOK := subject.ContentID()
	if !subjectIDOK || !subject.Valid() {
		return PublicationRow{}, nil, false
	}
	members := make([]PublicationSubject, 0, subject.MemberCount())
	for member := 0; member < subject.MemberCount(); member++ {
		semantic, semanticOK := subject.MemberAt(member)
		if !semanticOK {
			return PublicationRow{}, nil, false
		}
		memberID, memberIDOK := PublicationMemberID(id, member)
		if !memberIDOK {
			return PublicationRow{}, nil, false
		}
		memberRow := PublicationMemberRow{ID: memberID, RowID: id, Semantic: semantic, Member: uint32(member)}
		if !memberRow.Available() {
			return PublicationRow{}, nil, false
		}
		members = append(members, PublicationSubject{row: memberRow})
	}
	contextID := identity.ContentID{}
	context, hasContext := receipt.ContextInput()
	if hasContext {
		var contextIDOK bool
		contextID, contextIDOK = context.ContentID()
		if !contextIDOK || !context.Valid() {
			return PublicationRow{}, nil, false
		}
		// The context is an authenticated mounted input the descriptor
		// authored a destination for. That its members resolve to Value
		// coordinates is proven where the Value schema is - Value's own
		// relation over this publication's members - because this algebra
		// holds no Value schema to ask.
	}
	effect := receipt.EffectIndex()
	if effect < 0 {
		return PublicationRow{}, nil, false
	}
	row := PublicationRow{
		ID: id, Module: module, Call: call, Application: application,
		Kind: receipt.Kind(), Escape: receipt.Escape(),
		Mutability: receipt.Mutability(), Lifetime: receipt.Lifetime(),
		DescriptorID: descriptorID, OccurrenceID: occurrenceID,
		Operation: receipt.Operation(), Callback: receipt.Callback(), Effect: uint32(effect),
		Subject: subjectID, Context: contextID, HasContext: hasContext,
		SubjectOpen:   subject.IsOpen(),
		SubjectOffset: memberOffset, SubjectLength: uint32(len(members)),
	}
	return row, members, row.Available()
}

// sealPublications derives this Link's publication directory once, at seal,
// and indexes its calls by the mounted coordinate a rule resolves them from.
func (a *Algebra) sealPublications() bool {
	directory, ok := DetachPublications(a)
	if !ok {
		return false
	}
	index := make(map[mountedPublicationCallRef]uint32, len(directory.Calls))
	for ordinal := range directory.Calls {
		row := directory.Calls[ordinal].Row()
		ref := mountedPublicationCallRef{module: row.Module, occurrence: row.Call}
		if _, duplicate := index[ref]; duplicate {
			return false
		}
		index[ref] = uint32(ordinal)
	}
	subjects := make(map[identity.ContentID]uint32, len(directory.Members))
	for ordinal := range directory.Members {
		member := directory.Members[ordinal].Row()
		if !member.Available() {
			return false
		}
		if _, duplicate := subjects[member.ID]; duplicate {
			return false
		}
		subjects[member.ID] = uint32(ordinal)
	}
	receipts := make(map[identity.ContentID]MountedPublication, len(directory.Rows))
	for ordinal := 0; ordinal < a.MountedCallCount(); ordinal++ {
		mounted, mountedOK := a.MountedCallAt(ordinal)
		if !mountedOK {
			return false
		}
		batch, batchOK := a.PublicationBatchForMountedCall(mounted)
		if !batchOK {
			return false
		}
		for position := 0; position < batch.RowCount(); position++ {
			receipt, receiptOK := batch.RowAt(position)
			if !receiptOK {
				return false
			}
			id, idOK := receipt.ContentID()
			if !idOK {
				return false
			}
			receipts[id] = receipt
		}
	}
	a.publications = directory
	a.publicationCallIndex = index
	a.publicationSubjects = subjects
	a.publicationReceipts = receipts
	return true
}

// PublicationContextInput is the mounted context input one admitted receipt
// authored a destination for.
//
// It is a live Pack input rather than a published row, which is why it is
// reached through this algebra and never through the directory: a context is
// the one part of a publication no consumer reads, and the only thing anyone
// asks of it is whether its members resolve. That question belongs to Value,
// which holds the schema; this hands it the input to ask about.
func (a *Algebra) PublicationSubjectInput(receipt identity.ContentID) (packtransfer.MountedInput, bool) {
	if !a.Valid() || !receipt.Available() {
		return packtransfer.MountedInput{}, false
	}
	row, found := a.publicationReceipts[receipt]
	if !found {
		return packtransfer.MountedInput{}, false
	}
	return row.SubjectInput()
}

// PublicationContextInput is the mounted context input one admitted receipt
// authored a destination for.
func (a *Algebra) PublicationContextInput(receipt identity.ContentID) (packtransfer.MountedInput, bool) {
	if !a.Valid() || !receipt.Available() {
		return packtransfer.MountedInput{}, false
	}
	row, found := a.publicationReceipts[receipt]
	if !found {
		return packtransfer.MountedInput{}, false
	}
	return row.ContextInput()
}

// PublicationSubjectCount is the number of subject members this Link's
// publications name, across every mounted call.
func (a *Algebra) PublicationSubjectCount() int {
	if !a.Valid() {
		return 0
	}
	return len(a.publications.Members)
}

// PublicationSubjectAt is one subject member in this directory's sealed order.
func (a *Algebra) PublicationSubjectAt(index int) (PublicationSubject, bool) {
	if !a.Valid() || index < 0 || index >= len(a.publications.Members) {
		return PublicationSubject{}, false
	}
	return a.publications.Members[index], true
}

// PublicationSubjectOrdinal is the position one subject member occupies in
// this directory's sealed order. It answers only for a member this algebra
// sealed, addressed by the identity it minted for it.
func (a *Algebra) PublicationSubjectOrdinal(subject PublicationSubject) (uint32, bool) {
	if !a.Valid() {
		return 0, false
	}
	member := subject.Row()
	if !member.Available() {
		return 0, false
	}
	ordinal, found := a.publicationSubjects[member.ID]
	if !found || int(ordinal) >= len(a.publications.Members) || a.publications.Members[ordinal].Row() != member {
		return 0, false
	}
	return ordinal, true
}

// PublicationSubjectForOccurrence resolves the first subject member of the
// publication call at one mounted coordinate. It is the inverse the member set
// is entered by; the rest of the set is enumerated from its parent.
func (a *Algebra) PublicationSubjectForOccurrence(module, occurrence identity.ContentID) (PublicationSubject, bool) {
	call, found := a.PublicationCallForOccurrence(module, occurrence)
	if !found {
		return PublicationSubject{}, false
	}
	return call.MemberAt(0)
}

// Publications is this Link's sealed publication directory.
func (a *Algebra) Publications() PublicationDirectory {
	if !a.Valid() {
		return PublicationDirectory{}
	}
	return a.publications
}

// PublicationCallCount is the number of mounted calls this Link admitted
// publications on. A call that authored none is still one of them.
func (a *Algebra) PublicationCallCount() int {
	if !a.Valid() {
		return 0
	}
	return len(a.publications.Calls)
}

// PublicationCallAt is one publication call in this directory's sealed order.
func (a *Algebra) PublicationCallAt(index int) (PublicationCall, bool) {
	if !a.Valid() || index < 0 || index >= len(a.publications.Calls) {
		return PublicationCall{}, false
	}
	return a.publications.Calls[index], true
}

// PublicationCallOrdinal is the position one publication call occupies in this
// directory's sealed order. It answers only for a call this algebra sealed, so
// a row assembled elsewhere resolves to nothing rather than to a neighbour.
func (a *Algebra) PublicationCallOrdinal(call PublicationCall) (uint32, bool) {
	if !a.Valid() {
		return 0, false
	}
	row := call.Row()
	if !row.Available() {
		return 0, false
	}
	ordinal, found := a.publicationCallIndex[mountedPublicationCallRef{module: row.Module, occurrence: row.Call}]
	if !found || int(ordinal) >= len(a.publications.Calls) || a.publications.Calls[ordinal].Row() != row {
		return 0, false
	}
	return ordinal, true
}

// PublicationCallForOccurrence resolves the publication call at one mounted
// coordinate. It is the inverse a rule addresses its candidate by, and the
// only one: a caller that walked the directory to find it would be a second
// authority over which call an occurrence names.
func (a *Algebra) PublicationCallForOccurrence(module, occurrence identity.ContentID) (PublicationCall, bool) {
	if !a.Valid() || !module.Available() || !occurrence.Available() {
		return PublicationCall{}, false
	}
	ordinal, found := a.publicationCallIndex[mountedPublicationCallRef{module: module, occurrence: occurrence}]
	if !found || int(ordinal) >= len(a.publications.Calls) {
		return PublicationCall{}, false
	}
	return a.publications.Calls[ordinal], true
}
