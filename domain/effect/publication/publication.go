// Package publication declares Effect's publication directory: the coordinate
// space of admitted publication rows, the column the detached row is
// published in, and the semantic role the space is identified by.
//
// An admitted publication is a receipt Effect already sealed at one mounted
// call: what the Target authored the occurrence to do, which call it belongs
// to, and which subject and context it names. Every consumer of that fact
// used to reach it the same way - hold Effect's live algebra, ask it for the
// batch of one call, and walk the batch's rows - which made the batch the
// interface and the row something you could only obtain by reconstructing the
// batch that carried it.
//
// The column states the rows instead. It is a DIRECTORY: sealed once from the
// receipts Effect admitted, addressed by each receipt's own identity, and read
// back by that identity. Nothing folds it per point and no rule writes it,
// because a publication is not a per-point conclusion - it is what the program
// authored, and it is the same fact wherever it is read.
//
// The coordinate is the receipt's own content identity and nothing else. That
// identity is already the seal Effect issues the receipt under, so naming a
// row a second way here would make one fact answer to two vocabularies.
//
// The admitted identities are sealed with the column as its key universe, and
// because a row is addressed by its own seal that universe is exactly the set
// of rows: a read answers with a row if and only if this Link admitted it, and
// every other identity is not this Link's publication rather than a row it
// might be hiding. What the universe adds beyond the rows is enumeration and
// identity - the directory can be walked in its sealed order, and it has one
// name that moves whenever its membership does. A Link that authored no
// publication seals an empty directory, which is that statement about every
// identity.
package publication

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schemavocabulary "github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/effect/factor"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The identities this package declares. Each is authored here and named from
// here, so the rows and the references that resolve them are one statement.
const (
	// AxisKey is this coordinate space's authored identity, and therefore the
	// identity of the principal admitted to write the column below.
	AxisKey schema.Key = "effect-publication"
	// OutputKey is the column of admitted publications: each receipt identity
	// against the receipt Effect sealed under it.
	OutputKey schema.Key = "effect-publication/rows"
	// MembersOutputKey is the column of subject members: every semantic
	// member a publication's subject pack names, in the pack's own order. A
	// Value coordinate is a handle its Schema issues and cannot leave it, so
	// what is published is the identity the coordinate is keyed by; a
	// consumer holding the Link's Value schema joins (Module, member) to its
	// own coordinate. Every member is proven to resolve before it is
	// admitted, so that join cannot fail on a published member.
	MembersOutputKey schema.Key = "effect-publication/subject-members"
	// CallsOutputKey is the column of mounted calls: every call Effect
	// admitted publications on this Link for, against the span of the row
	// column that call's receipts occupy. A call whose span is empty is a
	// published fact - Effect admitted the call and it authored nothing -
	// which is the distinction the row column alone cannot state.
	CallsOutputKey schema.Key = "effect-publication/calls"
	// AxisRole is the semantic role this coordinate space is identified by.
	AxisRole = "axis/effect-publication"
	// directoryDomain separates this directory's key universe from every
	// other identity derived over the same Link.
	directoryDomain = "analysis/effect-publication-directory/v1"
	// membersDomain separates the subject-member column's key universe, and
	// derives each member's own coordinate from the row it belongs to and its
	// position in that row's pack.
	membersDomain = "analysis/effect-publication-subject-member/v1"
	// callsDomain separates the mounted-call column's key universe from the
	// row column's. The two columns are total over different populations, so
	// one universe identity could not prove absence for both.
	callsDomain = "analysis/effect-publication-calls/v1"
)

// Row is one admitted publication, detached from the algebra that sealed it.
//
// It carries no owner pointer and no mounted Pack input: a directory row
// crosses to readers that hold neither, and a row that carried a live input
// would publish a capability as a fact. The subject and context are therefore
// the identities those inputs seal to, which is what a consumer correlates
// with, and the descriptor is the authored disposition rather than a
// re-derivation of it.
type Row struct {
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
func (row Row) Available() bool {
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
func (row Row) MountedAt(module, call identity.ContentID) bool {
	return row.Available() && row.Module == module && row.Call == call
}

// Directory is one Link's whole publication statement: the admitted receipts
// and the mounted calls they were admitted on. The two are built in one walk
// so a call's row span is the position its receipts actually occupy rather
// than an offset a second pass recomputed.
type Directory struct {
	Rows    []Row
	Calls   []CallRow
	Members []MemberRow
}

// MemberRow is one semantic member of one publication's subject pack.
type MemberRow struct {
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

// Available reports whether this row states a complete subject member.
func (row MemberRow) Available() bool {
	return row.ID.Available() && row.RowID.Available() && row.Semantic.Available()
}

// MemberID derives one subject member's coordinate. It is the row it belongs
// to and its position in that row's pack, so two publications naming the same
// semantic member are two members here and neither hides the other.
func MemberID(rowID identity.ContentID, member int) (identity.ContentID, bool) {
	if !rowID.Available() || member < 0 {
		return identity.ContentID{}, false
	}
	var position [8]byte
	binary.BigEndian.PutUint64(position[:], uint64(member))
	return identity.DeriveContentID(membersDomain, rowID[:], position[:])
}

// CallRow is one mounted call Effect admitted publications on, and the span
// of the row column its receipts occupy.
//
// The span may be empty. That is the point of this column: a call Effect
// mounted and the program authored no publication on is a fact the row column
// cannot state, because a row column states only rows. A consumer asking
// "what did this call publish" reads an answer either way, and never has to
// treat "no rows found" as ambiguous between an empty call and a call the
// directory does not know.
type CallRow struct {
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

// Available reports whether this row states a complete mounted call. A zero
// length is a complete statement; a lost identity is not.
func (row CallRow) Available() bool {
	return row.ID.Available() && row.Module.Available() && row.Call.Available() && row.Application.Available()
}

// Detach reads this Link's whole publication directory out of Effect's sealed
// batches, in Effect's own mounted-call order and each call's canonical
// receipt order. The enumeration is Effect's sealed batch directory, so the
// rows are exactly the receipts it admitted and the directory cannot state
// one Effect never issued.
//
// The Value schema is required because a row proves its subject members
// resolve before it is admitted. Publishing a member a consumer could not
// join would move the failure from this seal to every read of it.
//
// A Link that admitted no publication yields an empty directory, which is
// that statement rather than a failure.
func Detach(owner *factor.Algebra, values *valuedomain.Schema) (Directory, bool) {
	if values == nil || !values.Valid() {
		return Directory{}, false
	}
	if owner == nil || !owner.Valid() {
		return Directory{}, false
	}
	count := owner.MountedCallCount()
	directory := Directory{Rows: make([]Row, 0), Calls: make([]CallRow, 0, count), Members: make([]MemberRow, 0)}
	for ordinal := 0; ordinal < count; ordinal++ {
		mounted, mountedOK := owner.MountedCallAt(ordinal)
		if !mountedOK {
			return Directory{}, false
		}
		batch, batchOK := owner.PublicationBatchForMountedCall(mounted)
		if !batchOK {
			return Directory{}, false
		}
		batchID, batchIDOK := batch.SealedContentID()
		module, call, provenanceOK := batch.CallProvenance()
		application, applicationOK := batch.ApplicationID()
		if !batchIDOK || !provenanceOK || !applicationOK {
			return Directory{}, false
		}
		offset := uint32(len(directory.Rows))
		for position := 0; position < batch.RowCount(); position++ {
			receipt, receiptOK := batch.RowAt(position)
			if !receiptOK {
				return Directory{}, false
			}
			row, members, rowOK := detach(receipt, values, uint32(len(directory.Members)))
			if !rowOK || !row.MountedAt(module, call) {
				return Directory{}, false
			}
			directory.Rows = append(directory.Rows, row)
			directory.Members = append(directory.Members, members...)
		}
		callRow := CallRow{
			ID: batchID, Module: module, Call: call, Application: application,
			RowOffset: offset, RowLength: uint32(len(directory.Rows)) - offset,
		}
		if !callRow.Available() {
			return Directory{}, false
		}
		directory.Calls = append(directory.Calls, callRow)
	}
	return directory, true
}

// detach reads one sealed receipt into the published row. Every field is
// taken from the receipt's own accessors, so a receipt that stopped
// authenticating cannot reach the directory as a partially read row.
func detach(receipt factor.MountedPublication, values *valuedomain.Schema, memberOffset uint32) (Row, []MemberRow, bool) {
	id, idOK := receipt.ContentID()
	module, call, provenanceOK := receipt.CallProvenance()
	application, applicationOK := receipt.ApplicationID()
	descriptorID, descriptorIDOK := receipt.DescriptorID()
	occurrenceID, occurrenceIDOK := receipt.OccurrenceID()
	subject, subjectOK := receipt.SubjectInput()
	if !idOK || !provenanceOK || !applicationOK || !descriptorIDOK || !occurrenceIDOK || !subjectOK {
		return Row{}, nil, false
	}
	subjectID, subjectIDOK := subject.ContentID()
	if !subjectIDOK || !subject.Valid() {
		return Row{}, nil, false
	}
	members := make([]MemberRow, 0, subject.MemberCount())
	for member := 0; member < subject.MemberCount(); member++ {
		semantic, semanticOK := subject.MemberAt(member)
		if !semanticOK {
			return Row{}, nil, false
		}
		if _, resolved := packtransfer.CoordinateForInputMember(values, subject, member); !resolved {
			return Row{}, nil, false
		}
		memberID, memberIDOK := MemberID(id, member)
		if !memberIDOK {
			return Row{}, nil, false
		}
		memberRow := MemberRow{ID: memberID, RowID: id, Semantic: semantic, Member: uint32(member)}
		if !memberRow.Available() {
			return Row{}, nil, false
		}
		members = append(members, memberRow)
	}
	contextID := identity.ContentID{}
	context, hasContext := receipt.ContextInput()
	if hasContext {
		var contextIDOK bool
		contextID, contextIDOK = context.ContentID()
		if !contextIDOK || !context.Valid() {
			return Row{}, nil, false
		}
		// The context is an authenticated mounted input the descriptor
		// authored a destination for. Its members are not published - no
		// consumer reads them - but a context whose members do not resolve is
		// a publication whose destination this Link cannot authenticate, and
		// that is refused here rather than at each consumer that trusted it.
		for member := 0; member < context.MemberCount(); member++ {
			if _, resolved := packtransfer.CoordinateForInputMember(values, context, member); !resolved {
				return Row{}, nil, false
			}
		}
	}
	effect := receipt.EffectIndex()
	if effect < 0 {
		return Row{}, nil, false
	}
	row := Row{
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

// AxisEntry is this package's axis declaration. A is the composition's own
// Link input record: this axis names nothing in it, because it mounts no
// authority of its own and binds no factor against one.
func AxisEntry[A any]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:     AxisKey,
		Storage: axis.StorageEngine,
		// The admitted identities are the column's key universe, so the
		// directory carries one name that moves with its membership and can
		// be walked in its sealed order.
		Cardinality: axis.CardinalityDense,
		// The receipts are Effect's admission on one Link and die with the
		// binding that admitted them, and the composition publishes the
		// column once: no rule writes it afterwards.
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		// Three columns, one axis: the receipts, the subject members they name,
		// and the calls they were admitted on are one statement about one Link,
		// sealed together and written by the same principal. Splitting them
		// across axes would let a reader hold a span that addressed another
		// publication's rows.
		Frame: axis.Frame{Outputs: []axis.Output{
			{Key: OutputKey, Writer: AxisKey},
			{Key: MembersOutputKey, Writer: AxisKey},
			{Key: CallsOutputKey, Writer: AxisKey},
		}},
		Semantic: schemavocabulary.RoleKey(AxisRole),
	}
}

// StructureSpecs contributes this axis's one semantic role to the structural
// vocabulary. The role is declared beside the axis that carries it, so the
// composition keeps no second role inventory.
func StructureSpecs() []structure.Spec { return schemavocabulary.RoleSpecs(AxisRole) }

// Content seals one Link's publication directory into the column's payload.
// The rows and the universe's membership are the same identities, because the
// column is total over exactly the directory it publishes: one statement, so
// the two can never drift.
//
// The sealed structural vocabulary is the gate every published disposition
// passes: a row states its kind, escape, mutability and lifetime as the
// authored enum's own value, and this is where that value is required to be a
// declared member of the catalog it belongs to. Downstream reads then carry a
// rank the seal ranked rather than a byte one producer happened to write.
//
// A duplicate identity, an unavailable row, and a disposition the vocabulary
// does not declare are all rejected. Absence of rows is not: an empty
// directory is the published fact that this Link admitted no publication.
func Content(rows []Row, denominator identity.ContentID, declared structure.Table) (snapshot.Content[identity.ContentID, Row], bool) {
	if !denominator.Available() {
		return snapshot.Content[identity.ContentID, Row]{}, false
	}
	admitted := make(map[identity.ContentID]Row, len(rows))
	members := make([]identity.ContentID, 0, len(rows))
	for _, row := range rows {
		if !row.Available() || !dispositionsDeclared(row, declared) {
			return snapshot.Content[identity.ContentID, Row]{}, false
		}
		if _, duplicate := admitted[row.ID]; duplicate {
			return snapshot.Content[identity.ContentID, Row]{}, false
		}
		admitted[row.ID] = row
		members = append(members, row.ID)
	}
	return snapshot.Content[identity.ContentID, Row]{
		Rows: admitted, Denominator: denominator, Members: members,
	}, true
}

// dispositionsDeclared resolves each of the row's four authored dispositions
// against the catalog that declares it. Each vocabulary's ordinals are its
// enum's own numbering, so the value is the rank and nothing translates
// between them: a disposition the table does not rank is one this analyzer's
// consumers cannot read, and it never becomes a published row.
func dispositionsDeclared(row Row, declared structure.Table) bool {
	for _, member := range [...]struct {
		category structure.Category
		ordinal  uint16
	}{
		{structure.CategoryPublicationEffectKind, uint16(row.Kind)},
		{structure.CategoryPublicationEscape, uint16(row.Escape)},
		{structure.CategoryPublicationMutability, uint16(row.Mutability)},
		{structure.CategoryPublicationLifetime, uint16(row.Lifetime)},
	} {
		if _, ranked := declared.At(member.category, member.ordinal); !ranked {
			return false
		}
	}
	return true
}

// MembersContent seals one Link's subject members into the member column.
// The members are required to tile the rows' spans exactly, in row order, so
// a published span always addresses the members of the row that declared it.
func MembersContent(members []MemberRow, rows []Row, denominator identity.ContentID) (snapshot.Content[identity.ContentID, MemberRow], bool) {
	if !denominator.Available() {
		return snapshot.Content[identity.ContentID, MemberRow]{}, false
	}
	admitted := make(map[identity.ContentID]MemberRow, len(members))
	ordered := make([]identity.ContentID, 0, len(members))
	covered := 0
	for _, row := range rows {
		if int(row.SubjectOffset) != covered {
			return snapshot.Content[identity.ContentID, MemberRow]{}, false
		}
		for position := 0; position < int(row.SubjectLength); position++ {
			index := covered + position
			if index >= len(members) {
				return snapshot.Content[identity.ContentID, MemberRow]{}, false
			}
			member := members[index]
			if !member.Available() || member.RowID != row.ID || member.Member != uint32(position) {
				return snapshot.Content[identity.ContentID, MemberRow]{}, false
			}
			if _, duplicate := admitted[member.ID]; duplicate {
				return snapshot.Content[identity.ContentID, MemberRow]{}, false
			}
			admitted[member.ID] = member
			ordered = append(ordered, member.ID)
		}
		covered += int(row.SubjectLength)
	}
	if covered != len(members) {
		return snapshot.Content[identity.ContentID, MemberRow]{}, false
	}
	return snapshot.Content[identity.ContentID, MemberRow]{
		Rows: admitted, Denominator: denominator, Members: ordered,
	}, true
}

// MembersDenominatorID is the identity of one Link's subject-member universe.
func MembersDenominatorID(linkID identity.ContentID, members []MemberRow) (identity.ContentID, bool) {
	if !linkID.Available() {
		return identity.ContentID{}, false
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(members)))
	parts := make([][]byte, 0, len(members)+2)
	parts = append(parts, linkID[:], count[:])
	for index := range members {
		if !members[index].Available() {
			return identity.ContentID{}, false
		}
		id := members[index].ID
		parts = append(parts, id[:])
	}
	return identity.DeriveContentID(membersDomain, parts...)
}

// MembersAxis is the address of one Link publication's subject-member column.
func MembersAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, MemberRow] {
	return snapshot.Axis[identity.ContentID, MemberRow]{SchemaID: runtimeSchema, Slot: slot}
}

// SubjectMember resolves one member of one publication's subject pack by the
// row's own span. The span is the row's statement, so a consumer reads its
// members without holding the pack the row was detached from.
func SubjectMember(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, MemberRow], row Row, member int) (MemberRow, bool) {
	if member < 0 || uint32(member) >= row.SubjectLength {
		return MemberRow{}, false
	}
	id, walked := snapshot.MemberAtAxis(published, address, int(row.SubjectOffset)+member)
	if !walked {
		return MemberRow{}, false
	}
	value, status := snapshot.Read(published, address, id)
	if status != snapshot.ReadHit || !value.Available() {
		return MemberRow{}, false
	}
	return value, true
}

// CallsContent seals one Link's mounted calls into the calls column. The
// spans are checked against the row count they address, so a published span
// can never reach past the directory it indexes, and the calls are required
// to tile the rows exactly: every row belongs to one call's span and no row
// belongs to two. A directory whose spans left a row unclaimed would be
// publishing a receipt no call admits.
func CallsContent(calls []CallRow, rowCount int, denominator identity.ContentID) (snapshot.Content[identity.ContentID, CallRow], bool) {
	if !denominator.Available() || rowCount < 0 {
		return snapshot.Content[identity.ContentID, CallRow]{}, false
	}
	admitted := make(map[identity.ContentID]CallRow, len(calls))
	mounted := make(map[[2]identity.ContentID]struct{}, len(calls))
	members := make([]identity.ContentID, 0, len(calls))
	covered := 0
	for _, call := range calls {
		if !call.Available() {
			return snapshot.Content[identity.ContentID, CallRow]{}, false
		}
		if int(call.RowOffset) != covered || int(call.RowOffset)+int(call.RowLength) > rowCount {
			return snapshot.Content[identity.ContentID, CallRow]{}, false
		}
		covered += int(call.RowLength)
		if _, duplicate := admitted[call.ID]; duplicate {
			return snapshot.Content[identity.ContentID, CallRow]{}, false
		}
		// One mounted coordinate names one call. Two rows sharing a module and
		// occurrence would make "the publications of this call" answer twice,
		// and a consumer selecting by provenance would read one of them
		// without learning the other exists.
		provenance := [2]identity.ContentID{call.Module, call.Call}
		if _, duplicate := mounted[provenance]; duplicate {
			return snapshot.Content[identity.ContentID, CallRow]{}, false
		}
		mounted[provenance] = struct{}{}
		admitted[call.ID] = call
		members = append(members, call.ID)
	}
	if covered != rowCount {
		return snapshot.Content[identity.ContentID, CallRow]{}, false
	}
	return snapshot.Content[identity.ContentID, CallRow]{
		Rows: admitted, Denominator: denominator, Members: members,
	}, true
}

// CallsDenominatorID is the identity of one Link's mounted-call universe. It
// folds the call identities and their count for the same reason the row
// universe folds its own: a universe that did not move with its membership
// would let a reader prove absence against a directory that has since changed.
func CallsDenominatorID(linkID identity.ContentID, calls []CallRow) (identity.ContentID, bool) {
	if !linkID.Available() {
		return identity.ContentID{}, false
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(calls)))
	parts := make([][]byte, 0, len(calls)+2)
	parts = append(parts, linkID[:], count[:])
	for index := range calls {
		if !calls[index].Available() {
			return identity.ContentID{}, false
		}
		id := calls[index].ID
		parts = append(parts, id[:])
	}
	return identity.DeriveContentID(callsDomain, parts...)
}

// CallsAxis is the address of one Link publication's mounted-call column.
func CallsAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, CallRow] {
	return snapshot.Axis[identity.ContentID, CallRow]{SchemaID: runtimeSchema, Slot: slot}
}

// MountedCall resolves one mounted call against a Link publication's calls
// column. A miss is a call this Link admitted no publications on and never
// mounted, which is distinct from a call whose span is empty.
func MountedCall(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, CallRow], id identity.ContentID) (CallRow, snapshot.ReadStatus) {
	row, status := snapshot.Read(published, address, id)
	if status == snapshot.ReadHit && !row.Available() {
		return CallRow{}, snapshot.ReadInvalid
	}
	return row, status
}

// MountedCallCount is the number of mounted calls this Link admitted.
func MountedCallCount(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, CallRow]) (int, bool) {
	return snapshot.MemberCountAtAxis(published, address)
}

// MountedCallAt is one mounted call's identity in Effect's own sealed order.
func MountedCallAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, CallRow], index int) (identity.ContentID, bool) {
	return snapshot.MemberAtAxis(published, address, index)
}

// Axis is the address of one Link publication's directory column.
func Axis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, Row] {
	return snapshot.Axis[identity.ContentID, Row]{SchemaID: runtimeSchema, Slot: slot}
}

// DenominatorID is the identity of one Link's publication directory: the key
// universe the column proves absence against.
//
// It folds the admitted identities and their count, not the Link alone. The
// directory IS its rows, so no two admissions share a universe: one whose
// identity did not move when its membership did would let a reader open one
// admission's rows under another's universe and read an absence that was
// never published.
func DenominatorID(linkID identity.ContentID, rows []Row) (identity.ContentID, bool) {
	if !linkID.Available() {
		return identity.ContentID{}, false
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(rows)))
	parts := make([][]byte, 0, len(rows)+2)
	parts = append(parts, linkID[:], count[:])
	for index := range rows {
		if !rows[index].Available() {
			return identity.ContentID{}, false
		}
		id := rows[index].ID
		parts = append(parts, id[:])
	}
	return identity.DeriveContentID(directoryDomain, parts...)
}

// Published resolves one publication identity against a Link publication's
// directory. A hit is a row this Link admitted; any other status is an
// identity this Link did not admit, and the row is never partially read.
func Published(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, Row], id identity.ContentID) (Row, snapshot.ReadStatus) {
	row, status := snapshot.Read(published, address, id)
	if status == snapshot.ReadHit && !row.Available() {
		return Row{}, snapshot.ReadInvalid
	}
	return row, status
}

// Count is the number of publication rows this Link admitted.
func Count(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, Row]) (int, bool) {
	return snapshot.MemberCountAtAxis(published, address)
}

// At is one admitted publication's identity in the directory's own sealed
// order. It is how a consumer walks the directory to select the publications
// of one mounted call: the provenance it selects by is carried by the row the
// identity resolves to, so no caller rebuilds a batch to find them.
func At(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, Row], index int) (identity.ContentID, bool) {
	return snapshot.MemberAtAxis(published, address, index)
}
