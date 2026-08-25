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
)

// The identities this package declares. Each is authored here and named from
// here, so the rows and the references that resolve them are one statement.
const (
	// AxisKey is this coordinate space's authored identity, and therefore the
	// identity of the principal admitted to write the column below.
	AxisKey schema.Key = "effect-publication"
	// OutputKey is the one column this axis publishes: each admitted
	// publication identity against the receipt Effect sealed under it.
	OutputKey schema.Key = "effect-publication/rows"
	// AxisRole is the semantic role this coordinate space is identified by.
	AxisRole = "axis/effect-publication"
	// directoryDomain separates this directory's key universe from every
	// other identity derived over the same Link.
	directoryDomain = "analysis/effect-publication-directory/v1"
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

// Rows detaches every publication receipt Effect admitted on this Link, in
// Effect's own sealed mounted-call order and each call's canonical receipt
// order. The enumeration is Effect's sealed batch directory, so the rows are
// exactly the receipts it admitted and the directory cannot state one Effect
// never issued.
//
// A Link that admitted no publication yields no rows, which is a directory
// with an empty universe rather than a failure.
func Rows(owner *factor.Algebra) ([]Row, bool) {
	index, indexed := factor.NewMountedPublicationBatchIndex(owner)
	if !indexed || index == nil {
		return nil, false
	}
	rows := make([]Row, 0)
	for ordinal := 0; ordinal < index.Count(); ordinal++ {
		batch, batchOK := index.BatchAt(ordinal)
		if !batchOK {
			return nil, false
		}
		for position := 0; position < batch.RowCount(); position++ {
			receipt, receiptOK := batch.RowAt(position)
			if !receiptOK {
				return nil, false
			}
			row, rowOK := detach(receipt)
			if !rowOK {
				return nil, false
			}
			rows = append(rows, row)
		}
	}
	return rows, true
}

// detach reads one sealed receipt into the published row. Every field is
// taken from the receipt's own accessors, so a receipt that stopped
// authenticating cannot reach the directory as a partially read row.
func detach(receipt factor.MountedPublication) (Row, bool) {
	id, idOK := receipt.ContentID()
	module, call, provenanceOK := receipt.CallProvenance()
	application, applicationOK := receipt.ApplicationID()
	descriptorID, descriptorIDOK := receipt.DescriptorID()
	occurrenceID, occurrenceIDOK := receipt.OccurrenceID()
	subject, subjectOK := receipt.SubjectInput()
	if !idOK || !provenanceOK || !applicationOK || !descriptorIDOK || !occurrenceIDOK || !subjectOK {
		return Row{}, false
	}
	subjectID, subjectIDOK := subject.ContentID()
	if !subjectIDOK {
		return Row{}, false
	}
	contextID := identity.ContentID{}
	context, hasContext := receipt.ContextInput()
	if hasContext {
		var contextIDOK bool
		contextID, contextIDOK = context.ContentID()
		if !contextIDOK {
			return Row{}, false
		}
	}
	effect := receipt.EffectIndex()
	if effect < 0 {
		return Row{}, false
	}
	row := Row{
		ID: id, Module: module, Call: call, Application: application,
		Kind: receipt.Kind(), Escape: receipt.Escape(),
		Mutability: receipt.Mutability(), Lifetime: receipt.Lifetime(),
		DescriptorID: descriptorID, OccurrenceID: occurrenceID,
		Operation: receipt.Operation(), Callback: receipt.Callback(), Effect: uint32(effect),
		Subject: subjectID, Context: contextID, HasContext: hasContext,
	}
	return row, row.Available()
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
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Semantic:    schemavocabulary.RoleKey(AxisRole),
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
