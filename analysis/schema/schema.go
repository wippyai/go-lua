// Package schema is the analyzer's declaration root. It owns the one sealed
// table that every declaration surface plugs into: rules today, and further
// surfaces (axes, queries, observations) later. The root knows only
// how to hold entries, identify them, and seal them; every entry-kind law
// belongs to the surface's own package.
//
// The root names identity and the framed encoding primitive the table's
// content stream is written with, and nothing else.
package schema

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// contentDomain and contentVersion separate the table's content stream from
// every other stream hashed in the analyzer, and pin the framing this digest
// was computed under.
const (
	contentDomain         = "wippy.analysis/schema/table"
	contentVersion uint64 = 1
)

// Key is the authored identity of one entry inside its surface. It is a
// construction input only: it derives the entry's stable EntryID and never
// crosses a verdict boundary, so the authored vocabulary is not a channel.
type Key string

func (key Key) Available() bool { return key != "" }

// SurfaceKind is the closed catalog of declaration surfaces, and the
// contributor identity carried by a verdict. A new surface is added by
// inserting one ordinal here and registering its own package's Surface
// implementation; no existing surface changes.
//
// The catalog order is the bind phase order: a surface is registered, sealed,
// and bound before every surface above it, so a surface that binds against
// another's output is declared above its producer. Identities are declared
// before the entries that reference them, so the structural vocabulary seals
// first: it names no other surface, and every surface above it may name a
// member of it. Axes produce the coordinate spaces rules write, so axes precede
// rules; diagnostics reference the rules and axes their subjects come from, so
// diagnostics follow both. Composites relate coordinate spaces and queries read
// them, so both follow axes; a denominator quantifies over the entries of a
// surface sealed below it, so it follows every surface it may name as an owner.
// Observations consume query families, denominator relations, and structural
// population/role identities, so they are the final surface: every identity
// and producer an observation names has already been sealed when its rows are
// admitted.
type SurfaceKind uint8

const (
	SurfaceKindInvalid SurfaceKind = iota
	SurfaceKindStructure
	SurfaceKindAxis
	SurfaceKindRule
	SurfaceKindDiagnostic
	SurfaceKindComposite
	SurfaceKindDenominator
	SurfaceKindQuery
	SurfaceKindObservation
	surfaceKindLimit
)

func (kind SurfaceKind) Available() bool {
	return kind > SurfaceKindInvalid && kind < surfaceKindLimit
}

// EntryID is the stable content identity of one entry. It is derived from the
// contributor and the authored key, so it survives reordering and carries no
// authored text.
type EntryID identity.ContentID

func (id EntryID) Available() bool { return identity.ContentID(id).Available() }

// NewEntryID derives one entry identity. Surfaces use it to name an entry in
// a verdict without exposing the authored key.
func NewEntryID(kind SurfaceKind, key Key) EntryID {
	if !kind.Available() || !key.Available() {
		return EntryID{}
	}
	hash := sha256.New()
	var framing [8]byte
	binary.BigEndian.PutUint64(framing[:], uint64(kind))
	if _, err := hash.Write(framing[:]); err != nil {
		return EntryID{}
	}
	binary.BigEndian.PutUint64(framing[:], uint64(len(key)))
	if _, err := hash.Write(framing[:]); err != nil {
		return EntryID{}
	}
	if _, err := hash.Write([]byte(key)); err != nil {
		return EntryID{}
	}
	var id EntryID
	copy(id[:], hash.Sum(nil))
	return id
}

// LawID names one admission law. Ordinals below rootLawLimit belong to the
// root; a surface numbers its own laws above that floor.
type LawID uint16

const (
	LawNone LawID = iota
	LawSurfaceCatalog
	LawSurfaceUnique
	// The ordinal here is retired. The root states coverage of the catalog and
	// not population of a surface, so it has no such law to raise, and a surface
	// that does require members states that requirement under its own ordinal
	// above SurfaceLawFloor rather than claiming this one.
	_
	LawEntryPresent
	LawEntryIdentity
	LawEntryAdmissible
	LawEntryUnique
	LawSurfaceCoverage
	LawSurfacePhase
	LawEntryContent
	// SurfaceLawFloor is the first law ordinal a surface may claim.
	SurfaceLawFloor LawID = 1024
)

// Disposition is the universal, kind-agnostic outcome vocabulary. It is the
// only rendered part of a verdict; everything else is an identity.
type Disposition uint8

const (
	DispositionAccepted Disposition = iota
	DispositionMalformed
	DispositionDuplicate
	DispositionIncomplete
)

func (disposition Disposition) String() string {
	switch disposition {
	case DispositionMalformed:
		return "malformed"
	case DispositionDuplicate:
		return "duplicate"
	case DispositionIncomplete:
		return "incomplete"
	default:
		return "accepted"
	}
}

// SealFailure is the external verdict of one rejected table. It carries the
// contributor, the entry, the law, and the universal disposition, and nothing
// else: rendering is the caller's job, from these identities. Schema is the
// digest of the table a verdict was issued against, and is empty for a
// rejection raised while that table was still being sealed.
type SealFailure struct {
	Schema      identity.ContentID
	Contributor SurfaceKind
	Entry       EntryID
	Law         LawID
	Disposition Disposition
}

func (failure SealFailure) Available() bool { return failure.Law != LawNone }

// SurfaceLawFailure is the constructor a surface uses to report its own law.
// A surface that claims a root law ordinal is itself malformed, and the
// verdict says so rather than silently renaming the law.
func SurfaceLawFailure(kind SurfaceKind, entry EntryID, law LawID, disposition Disposition) SealFailure {
	if law < SurfaceLawFloor {
		return SealFailure{Contributor: kind, Entry: entry, Law: LawSurfaceCatalog, Disposition: DispositionMalformed}
	}
	return SealFailure{Contributor: kind, Entry: entry, Law: law, Disposition: disposition}
}

// Entry is one declared row of any surface. The root reads the identity, the
// surface-owned admissibility verdict, and the entry's declared content; the
// concrete record type stays in its surface package.
type Entry interface {
	Key() Key
	EntryAvailable() bool
	// EntryContent writes the canonical bytes of this entry's own declared data
	// into the table's content stream, which the table digest is folded from. An
	// identity names an entry; content is what the entry says. Both are folded,
	// so a catalog that differs from another only in a declared property differs
	// from it in the digest as well.
	//
	// Canonical means the bytes are a function of the declaration alone. A
	// surface writes its record's fields in one fixed order, writes every
	// collection in its declared order prefixed by its arity, and lets the
	// writer's own tags and lengths delimit each item. Nothing formats a struct,
	// nothing iterates a map, and nothing reflects: two entries that differ in
	// any declared field write different bytes, and one entry writes the same
	// bytes on every run.
	//
	// Content is declared DATA. The identities an entry references, its keys,
	// and its metadata scalars - a role, a cone form, a tier, an ordinal, an
	// accepted row - are content, because they are what the declaration says and
	// what every derived inventory is projected from. A hook is not: the typed
	// function values a rule or an axis declares carry no canonical bytes, so
	// they are never written, and neither is the shape of the hook set they
	// form. What such a hook is declared against is written - the axis
	// identities it names, the principal it writes, the lane it enters on - so
	// the declarative half of a hooked entry is covered, and the executable half
	// is left to the surface's own admission laws. A value derived from a
	// written field is not written a second time.
	EntryContent(content *framing.Writer) error
}

// Surface is one registered contributor of entries. Seal runs after the root
// has admitted and indexed the surface's rows, so a surface law can consult
// the finished view when it states coverage or totality. Sealed carries the
// surfaces already sealed below this one, so a surface that references another
// resolves that reference against the same table it is being sealed into.
type Surface interface {
	Kind() SurfaceKind
	Entries() []Entry
	Seal(view View, sealed Sealed) SealFailure
}

// Sealed is the immutable projection of the surfaces sealed before the surface
// currently stating its laws. The catalog order is the dependency order, so a
// surface reaches every surface below it and none at or above it: a reference
// upward would name a table that does not exist yet.
type Sealed struct {
	views [surfaceKindLimit]View
	phase SurfaceKind
}

// Registered reports whether one surface was sealed before this one. A caller
// resolves a reference against a registered surface and may only form-check a
// reference to an unregistered one.
func (sealed Sealed) Registered(kind SurfaceKind) bool {
	return kind.Available() && kind < sealed.phase && sealed.views[kind].Available()
}

// Surface returns one already-sealed sibling view.
func (sealed Sealed) Surface(kind SurfaceKind) (View, bool) {
	if !sealed.Registered(kind) {
		return View{}, false
	}
	return sealed.views[kind], true
}

// Resolve looks one declared reference up in the table it is being sealed
// into: the surface the referenced entry is declared on, and its authored key
// there. It is the one resolution every referencing surface performs, so the
// dispositions a dangling reference carries are stated here rather than once
// per surface.
//
// A reference to a surface that is not sealed below the referrer names a table
// that does not exist yet, and the catalog order makes that malformed rather
// than merely unresolved. A reference that names a sealed surface but no entry
// on it is incomplete. The caller states the law the verdict is raised under,
// because what a reference means is the referring surface's own declaration.
func (sealed Sealed) Resolve(kind SurfaceKind, key Key) (Entry, Disposition) {
	if !kind.Available() || !key.Available() {
		return nil, DispositionMalformed
	}
	view, registered := sealed.Surface(kind)
	if !registered {
		return nil, DispositionMalformed
	}
	entry, resolved := view.ByID(NewEntryID(kind, key))
	if !resolved {
		return nil, DispositionIncomplete
	}
	return entry, DispositionAccepted
}

// View is the immutable sealed projection of one surface. Position is the
// declaration order; it is a traversal convenience and never an identity.
type View struct {
	kind    SurfaceKind
	entries []Entry
	index   map[EntryID]int
}

func (view View) Kind() SurfaceKind { return view.kind }

// Available reports whether this view names a registered surface. An empty
// inventory is available: whether a surface must hold entries, and which ones,
// is that surface's own law. The root states coverage of the catalog, not
// population of a surface, so a surface that is registered and declares
// nothing yet is a sealed surface with nothing in it rather than a hole.
func (view View) Available() bool { return view.kind.Available() }

func (view View) Count() int { return len(view.entries) }

func (view View) At(position int) (Entry, bool) {
	if position < 0 || position >= len(view.entries) {
		return nil, false
	}
	return view.entries[position], true
}

// ByID resolves one entry by its stable identity.
func (view View) ByID(id EntryID) (Entry, bool) {
	if view.index == nil {
		return nil, false
	}
	position, known := view.index[id]
	if !known {
		return nil, false
	}
	return view.entries[position], true
}

// Schema is the sealed declaration table. It is immutable once returned and is
// safe for concurrent readers.
type Schema struct {
	views  [surfaceKindLimit]View
	digest identity.ContentID
}

func (schema *Schema) Available() bool {
	if schema == nil || !schema.digest.Available() {
		return false
	}
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		if !schema.views[kind].Available() {
			return false
		}
	}
	return true
}

// Digest is the content identity of the sealed table. Every verdict issued
// against this table carries it.
func (schema *Schema) Digest() identity.ContentID {
	if schema == nil {
		return identity.ContentID{}
	}
	return schema.digest
}

// Surface returns the sealed view of one registered surface.
func (schema *Schema) Surface(kind SurfaceKind) (View, bool) {
	if schema == nil || !kind.Available() {
		return View{}, false
	}
	view := schema.views[kind]
	return view, view.Available()
}

// Builder collects surfaces before the one Seal transaction. It is not safe
// for concurrent use; a sealed Schema is.
type Builder struct {
	surfaces  []Surface
	installed [surfaceKindLimit]bool
	phase     SurfaceKind
	rejected  SealFailure
}

func NewBuilder() *Builder { return &Builder{} }

// Register admits one surface. The first rejection is retained and reported by
// Seal, so a caller may register the whole set before inspecting the verdict.
func (builder *Builder) Register(surface Surface) bool {
	if builder == nil {
		return false
	}
	if surface == nil {
		return builder.reject(SealFailure{Law: LawSurfaceCatalog, Disposition: DispositionMalformed})
	}
	kind := surface.Kind()
	if !kind.Available() {
		return builder.reject(SealFailure{Contributor: kind, Law: LawSurfaceCatalog, Disposition: DispositionMalformed})
	}
	if builder.installed[kind] {
		return builder.reject(SealFailure{Contributor: kind, Law: LawSurfaceUnique, Disposition: DispositionDuplicate})
	}
	// The catalog order is the bind phase order. Registering out of that order
	// would seal a consumer surface before its producer, so it is rejected here
	// rather than reordered silently.
	if kind <= builder.phase {
		return builder.reject(SealFailure{Contributor: kind, Law: LawSurfacePhase, Disposition: DispositionMalformed})
	}
	builder.phase = kind
	builder.installed[kind] = true
	builder.surfaces = append(builder.surfaces, surface)
	return true
}

func (builder *Builder) reject(failure SealFailure) bool {
	if !builder.rejected.Available() {
		builder.rejected = failure
	}
	return false
}

// Seal validates every registered surface and returns the one immutable table.
// Coverage is total: every declared SurfaceKind must have been registered, so a
// surface added to the catalog but never wired fails loudly here.
func (builder *Builder) Seal() (*Schema, SealFailure) {
	if builder == nil {
		return nil, SealFailure{Law: LawSurfaceCatalog, Disposition: DispositionIncomplete}
	}
	if builder.rejected.Available() {
		return nil, builder.rejected
	}
	if len(builder.surfaces) == 0 {
		return nil, SealFailure{Law: LawSurfaceCatalog, Disposition: DispositionIncomplete}
	}
	schema := &Schema{}
	hash := sha256.New()
	var content framing.Writer
	if content.Reset(hash, contentDomain, contentVersion) != nil {
		return nil, SealFailure{Law: LawSurfaceCatalog, Disposition: DispositionMalformed}
	}
	for _, surface := range builder.surfaces {
		kind := surface.Kind()
		view, failure := indexSurface(kind, surface.Entries(), &content)
		if failure.Available() {
			return nil, failure
		}
		// Registration order is the catalog order, so the views collected so far
		// are exactly the surfaces below this one.
		if failure = surface.Seal(view, Sealed{views: schema.views, phase: kind}); failure.Available() {
			failure.Contributor = kind
			return nil, failure
		}
		schema.views[kind] = view
	}
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		if !schema.views[kind].Available() {
			return nil, SealFailure{Contributor: kind, Law: LawSurfaceCoverage, Disposition: DispositionIncomplete}
		}
	}
	if content.Finish() != nil {
		return nil, SealFailure{Law: LawEntryContent, Disposition: DispositionMalformed}
	}
	copy(schema.digest[:], hash.Sum(nil))
	if !schema.digest.Available() {
		return nil, SealFailure{Law: LawSurfaceCatalog, Disposition: DispositionMalformed}
	}
	return schema, SealFailure{}
}

// indexSurface admits and indexes one surface's rows. It states the identity
// and uniqueness laws every entry of every surface is subject to, and nothing
// about how many rows a surface holds: an inventory of none is indexed like
// any other, and the surface that requires members states that requirement in
// its own Seal.
//
// Each admitted row is folded into the content stream as its identity followed
// by its declared content, in catalog order. The identity says which entry this
// is and the content says what it declares, so neither a renamed entry nor a
// rewritten declaration can reach the same digest.
func indexSurface(kind SurfaceKind, entries []Entry, content *framing.Writer) (View, SealFailure) {
	index := make(map[EntryID]int, len(entries))
	for position, entry := range entries {
		if entry == nil {
			return View{}, SealFailure{Contributor: kind, Law: LawEntryPresent, Disposition: DispositionMalformed}
		}
		id := NewEntryID(kind, entry.Key())
		if !id.Available() {
			return View{}, SealFailure{Contributor: kind, Law: LawEntryIdentity, Disposition: DispositionMalformed}
		}
		if !entry.EntryAvailable() {
			return View{}, SealFailure{Contributor: kind, Entry: id, Law: LawEntryAdmissible, Disposition: DispositionMalformed}
		}
		if _, duplicate := index[id]; duplicate {
			return View{}, SealFailure{Contributor: kind, Entry: id, Law: LawEntryUnique, Disposition: DispositionDuplicate}
		}
		index[id] = position
		if content.Record(uint64(kind)) != nil || content.Bytes(id[:]) != nil {
			return View{}, SealFailure{Contributor: kind, Entry: id, Law: LawEntryIdentity, Disposition: DispositionMalformed}
		}
		if entry.EntryContent(content) != nil {
			return View{}, SealFailure{Contributor: kind, Entry: id, Law: LawEntryContent, Disposition: DispositionMalformed}
		}
	}
	return View{kind: kind, entries: append([]Entry(nil), entries...), index: index}, SealFailure{}
}
