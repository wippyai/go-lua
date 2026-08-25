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
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schemavocabulary "github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/effect/factor"
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
	// callsDomain separates the mounted-call column's key universe from the
	// row column's. The two columns are total over different populations, so
	// one universe identity could not prove absence for both.
	callsDomain = "analysis/effect-publication-calls/v1"
)

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
func Content(rows []factor.PublicationRow, denominator identity.ContentID, declared structure.Table) (snapshot.Content[identity.ContentID, factor.PublicationRow], bool) {
	if !denominator.Available() {
		return snapshot.Content[identity.ContentID, factor.PublicationRow]{}, false
	}
	admitted := make(map[identity.ContentID]factor.PublicationRow, len(rows))
	members := make([]identity.ContentID, 0, len(rows))
	for _, row := range rows {
		if !row.Available() || !dispositionsDeclared(row, declared) {
			return snapshot.Content[identity.ContentID, factor.PublicationRow]{}, false
		}
		if _, duplicate := admitted[row.ID]; duplicate {
			return snapshot.Content[identity.ContentID, factor.PublicationRow]{}, false
		}
		admitted[row.ID] = row
		members = append(members, row.ID)
	}
	return snapshot.Content[identity.ContentID, factor.PublicationRow]{
		Rows: admitted, Denominator: denominator, Members: members,
	}, true
}

// dispositionsDeclared resolves each of the row's four authored dispositions
// against the catalog that declares it. Each vocabulary's ordinals are its
// enum's own numbering, so the value is the rank and nothing translates
// between them: a disposition the table does not rank is one this analyzer's
// consumers cannot read, and it never becomes a published row.
func dispositionsDeclared(row factor.PublicationRow, declared structure.Table) bool {
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
func MembersContent(members []factor.PublicationMemberRow, rows []factor.PublicationRow, denominator identity.ContentID) (snapshot.Content[identity.ContentID, factor.PublicationMemberRow], bool) {
	if !denominator.Available() {
		return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
	}
	admitted := make(map[identity.ContentID]factor.PublicationMemberRow, len(members))
	ordered := make([]identity.ContentID, 0, len(members))
	covered := 0
	for _, row := range rows {
		if int(row.SubjectOffset) != covered {
			return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
		}
		for position := 0; position < int(row.SubjectLength); position++ {
			index := covered + position
			if index >= len(members) {
				return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
			}
			member := members[index]
			if !member.Available() || member.RowID != row.ID || member.Member != uint32(position) {
				return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
			}
			if _, duplicate := admitted[member.ID]; duplicate {
				return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
			}
			admitted[member.ID] = member
			ordered = append(ordered, member.ID)
		}
		covered += int(row.SubjectLength)
	}
	if covered != len(members) {
		return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{}, false
	}
	return snapshot.Content[identity.ContentID, factor.PublicationMemberRow]{
		Rows: admitted, Denominator: denominator, Members: ordered,
	}, true
}

// MembersDenominatorID is the identity of one Link's subject-member universe.
func MembersDenominatorID(linkID identity.ContentID, members []factor.PublicationMemberRow) (identity.ContentID, bool) {
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
	return identity.DeriveContentID(factor.PublicationMemberDomain, parts...)
}

// MembersAxis is the address of one Link publication's subject-member column.
func MembersAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, factor.PublicationMemberRow] {
	return snapshot.Axis[identity.ContentID, factor.PublicationMemberRow]{SchemaID: runtimeSchema, Slot: slot}
}

// SubjectMember resolves one member of one publication's subject pack by the
// row's own span. The span is the row's statement, so a consumer reads its
// members without holding the pack the row was detached from.
func SubjectMember(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationMemberRow], row factor.PublicationRow, member int) (factor.PublicationMemberRow, bool) {
	if member < 0 || uint32(member) >= row.SubjectLength {
		return factor.PublicationMemberRow{}, false
	}
	id, walked := snapshot.MemberAtAxis(published, address, int(row.SubjectOffset)+member)
	if !walked {
		return factor.PublicationMemberRow{}, false
	}
	value, status := snapshot.Read(published, address, id)
	if status != snapshot.ReadHit || !value.Available() {
		return factor.PublicationMemberRow{}, false
	}
	return value, true
}

// CallsContent seals one Link's mounted calls into the calls column. The
// spans are checked against the row count they address, so a published span
// can never reach past the directory it indexes, and the calls are required
// to tile the rows exactly: every row belongs to one call's span and no row
// belongs to two. A directory whose spans left a row unclaimed would be
// publishing a receipt no call admits.
func CallsContent(calls []factor.PublicationCallRow, rowCount int, denominator identity.ContentID) (snapshot.Content[identity.ContentID, factor.PublicationCallRow], bool) {
	if !denominator.Available() || rowCount < 0 {
		return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
	}
	admitted := make(map[identity.ContentID]factor.PublicationCallRow, len(calls))
	mounted := make(map[[2]identity.ContentID]struct{}, len(calls))
	members := make([]identity.ContentID, 0, len(calls))
	covered := 0
	for _, call := range calls {
		if !call.Available() {
			return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
		}
		if int(call.RowOffset) != covered || int(call.RowOffset)+int(call.RowLength) > rowCount {
			return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
		}
		covered += int(call.RowLength)
		if _, duplicate := admitted[call.ID]; duplicate {
			return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
		}
		// One mounted coordinate names one call. Two rows sharing a module and
		// occurrence would make "the publications of this call" answer twice,
		// and a consumer selecting by provenance would read one of them
		// without learning the other exists.
		provenance := [2]identity.ContentID{call.Module, call.Call}
		if _, duplicate := mounted[provenance]; duplicate {
			return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
		}
		mounted[provenance] = struct{}{}
		admitted[call.ID] = call
		members = append(members, call.ID)
	}
	if covered != rowCount {
		return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{}, false
	}
	return snapshot.Content[identity.ContentID, factor.PublicationCallRow]{
		Rows: admitted, Denominator: denominator, Members: members,
	}, true
}

// CallsDenominatorID is the identity of one Link's mounted-call universe. It
// folds the call identities and their count for the same reason the row
// universe folds its own: a universe that did not move with its membership
// would let a reader prove absence against a directory that has since changed.
func CallsDenominatorID(linkID identity.ContentID, calls []factor.PublicationCallRow) (identity.ContentID, bool) {
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
func CallsAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, factor.PublicationCallRow] {
	return snapshot.Axis[identity.ContentID, factor.PublicationCallRow]{SchemaID: runtimeSchema, Slot: slot}
}

// MountedCall resolves one mounted call against a Link publication's calls
// column. A miss is a call this Link admitted no publications on and never
// mounted, which is distinct from a call whose span is empty.
func MountedCall(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationCallRow], id identity.ContentID) (factor.PublicationCallRow, snapshot.ReadStatus) {
	row, status := snapshot.Read(published, address, id)
	if status == snapshot.ReadHit && !row.Available() {
		return factor.PublicationCallRow{}, snapshot.ReadInvalid
	}
	return row, status
}

// MountedCallCount is the number of mounted calls this Link admitted.
func MountedCallCount(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationCallRow]) (int, bool) {
	return snapshot.MemberCountAtAxis(published, address)
}

// MountedCallAt is one mounted call's identity in Effect's own sealed order.
func MountedCallAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationCallRow], index int) (identity.ContentID, bool) {
	return snapshot.MemberAtAxis(published, address, index)
}

// Axis is the address of one Link publication's directory column.
func Axis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, factor.PublicationRow] {
	return snapshot.Axis[identity.ContentID, factor.PublicationRow]{SchemaID: runtimeSchema, Slot: slot}
}

// DenominatorID is the identity of one Link's publication directory: the key
// universe the column proves absence against.
//
// It folds the admitted identities and their count, not the Link alone. The
// directory IS its rows, so no two admissions share a universe: one whose
// identity did not move when its membership did would let a reader open one
// admission's rows under another's universe and read an absence that was
// never published.
func DenominatorID(linkID identity.ContentID, rows []factor.PublicationRow) (identity.ContentID, bool) {
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
func Published(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationRow], id identity.ContentID) (factor.PublicationRow, snapshot.ReadStatus) {
	row, status := snapshot.Read(published, address, id)
	if status == snapshot.ReadHit && !row.Available() {
		return factor.PublicationRow{}, snapshot.ReadInvalid
	}
	return row, status
}

// Count is the number of publication rows this Link admitted.
func Count(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationRow]) (int, bool) {
	return snapshot.MemberCountAtAxis(published, address)
}

// At is one admitted publication's identity in the directory's own sealed
// order. It is how a consumer walks the directory to select the publications
// of one mounted call: the provenance it selects by is carried by the row the
// identity resolves to, so no caller rebuilds a batch to find them.
func At(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, factor.PublicationRow], index int) (identity.ContentID, bool) {
	return snapshot.MemberAtAxis(published, address, index)
}
