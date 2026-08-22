// Package observation owns the declaration surface for observations that are
// published by an already-declared query family. An observation row is only
// data: it names the producer, the population it ranges over, the geometry
// and evidence anchor the producer uses, and the semantic role under which its
// result is frozen.
//
// The surface is deliberately below no domain or engine package. Runtime
// attachments and result values are not declarations; the sealed table carries
// the identities they agree on, and the runtime resolves those identities at
// its own boundary.
package observation

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawObservationIdentity
	LawProducerDeclared
	LawProducerPhase
	LawProducerResolves
	LawPopulationDeclared
	LawPopulationRelationPhase
	LawPopulationRelationResolves
	LawPopulationKindPhase
	LawPopulationKindResolves
	LawGeometryDeclared
	LawGeometryPhase
	LawGeometryResolves
	LawAnchorDeclared
	LawAnchorPhase
	LawAnchorResolves
	LawCodecDeclared
	LawCodecPhase
	LawCodecResolves
	LawProducerCompatibility
)

// Reference names one entry on a lower schema surface. A row carries keys,
// not pointers or projected runtime tokens, so the declaration is resolved
// against the one sealed catalog at observation-surface seal time.
type Reference struct {
	Surface schema.SurfaceKind
	Key     schema.Key
}

// Available reports whether a reference has a valid surface and key.
func (reference Reference) Available() bool {
	return reference.Surface.Available() && reference.Key.Available()
}

// Declared reports whether a row named a reference at all. This distinction is
// needed for the optional relation and kind arms of Population.
func (reference Reference) Declared() bool { return reference != (Reference{}) }

// Population states the declared population an observation ranges over. A
// live population may be named by an existing denominator relation, an
// existing structural diagnostic-observation kind, or both. At least one arm
// is required; no new bridge vocabulary is introduced here.
type Population struct {
	Relation Reference
	Kind     Reference
}

// Available reports whether every declared population arm is well formed and
// at least one arm is present. Surface/category checks are repeated by Seal so
// a malformed foreign row cannot pass by construction alone.
func (population Population) Available() bool {
	if !population.Relation.Declared() && !population.Kind.Declared() {
		return false
	}
	if population.Relation.Declared() && !population.Relation.Available() {
		return false
	}
	if population.Kind.Declared() && !population.Kind.Available() {
		return false
	}
	return true
}

// Spec is the authored declaration of one observation family. Producer,
// Geometry, Anchor, and Codec are references rather than identities derived
// locally: they resolve to existing query or structural rows, and the latter
// three are intentionally neutral role references so this package does not
// own producer-specific vocabulary.
type Spec struct {
	Key        schema.Key
	Producer   Reference
	Population Population
	Geometry   Reference
	Anchor     Reference
	Codec      Reference
}

// Entry is one admitted observation declaration. It is immutable once built.
type Entry struct {
	key        schema.Key
	id         schema.EntryID
	producer   Reference
	population Population
	geometry   Reference
	anchor     Reference
	codec      Reference
}

// New admits one authored observation declaration. A rejected spec returns
// false rather than a partially usable row.
func New(spec Spec) (*Entry, bool) {
	if !spec.Key.Available() || !spec.Producer.Available() || !spec.Population.Available() ||
		!spec.Geometry.Available() || !spec.Anchor.Available() || !spec.Codec.Available() {
		return nil, false
	}
	if spec.Producer.Surface != schema.SurfaceKindQuery || spec.Geometry.Surface != schema.SurfaceKindStructure ||
		spec.Anchor.Surface != schema.SurfaceKindStructure || spec.Codec.Surface != schema.SurfaceKindStructure {
		return nil, false
	}
	if spec.Population.Relation.Declared() && spec.Population.Relation.Surface != schema.SurfaceKindDenominator {
		return nil, false
	}
	if spec.Population.Kind.Declared() && spec.Population.Kind.Surface != schema.SurfaceKindStructure {
		return nil, false
	}
	entry := &Entry{
		key:        spec.Key,
		id:         schema.NewEntryID(schema.SurfaceKindObservation, spec.Key),
		producer:   spec.Producer,
		population: spec.Population,
		geometry:   spec.Geometry,
		anchor:     spec.Anchor,
		codec:      spec.Codec,
	}
	return entry, entry.EntryAvailable() && entry.declarationComplete()
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Producer() Reference { return entry.producer }

func (entry *Entry) Population() Population { return entry.population }

func (entry *Entry) Geometry() Reference { return entry.geometry }

func (entry *Entry) Anchor() Reference { return entry.anchor }

// Codec returns the semantic role reference under which the producer's result
// is frozen.
func (entry *Entry) Codec() Reference { return entry.codec }

// EntryAvailable is the root's admissibility question: does this row identify
// one observation entry. Reference resolution is the surface's own law,
// stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

// EntryContent writes the declaration's data in one fixed order. Optional
// references carry an explicit presence bit, so an absent population arm and
// a malformed empty reference cannot share canonical bytes.
func (entry *Entry) EntryContent(content *framing.Writer) error {
	if err := writeReference(content, entry.producer); err != nil {
		return err
	}
	if err := writeReference(content, entry.population.Relation); err != nil {
		return err
	}
	if err := writeReference(content, entry.population.Kind); err != nil {
		return err
	}
	if err := writeReference(content, entry.geometry); err != nil {
		return err
	}
	if err := writeReference(content, entry.anchor); err != nil {
		return err
	}
	return writeReference(content, entry.codec)
}

func writeReference(content *framing.Writer, reference Reference) error {
	if err := content.Bool(reference.Declared()); err != nil {
		return err
	}
	if err := content.Uint(uint64(reference.Surface)); err != nil {
		return err
	}
	return content.String(string(reference.Key))
}

func (entry *Entry) declarationComplete() bool {
	return entry.producer.Available() && entry.population.Available() &&
		entry.geometry.Available() && entry.anchor.Available() && entry.codec.Available()
}

// surface is the observation contribution to the analyzer declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of observation declarations to the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindObservation }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the observation surface's own laws over the indexed view. Every
// reference is resolved against a lower sealed surface, and a wrong category
// is malformed rather than silently treated as an absent declaration.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindObservation, entry.key) {
			return failure(entry.id, LawObservationIdentity, schema.DispositionMalformed)
		}
		if !entry.producer.Available() {
			return failure(entry.id, LawProducerDeclared, schema.DispositionIncomplete)
		}
		if entry.producer.Surface != schema.SurfaceKindQuery {
			return failure(entry.id, LawProducerPhase, schema.DispositionMalformed)
		}
		producer, producerDisposition := sealed.Resolve(entry.producer.Surface, entry.producer.Key)
		if producerDisposition != schema.DispositionAccepted {
			return failure(entry.id, LawProducerResolves, producerDisposition)
		}
		if !entry.population.Available() {
			return failure(entry.id, LawPopulationDeclared, schema.DispositionIncomplete)
		}
		if entry.population.Relation.Declared() {
			if entry.population.Relation.Surface != schema.SurfaceKindDenominator {
				return failure(entry.id, LawPopulationRelationPhase, schema.DispositionMalformed)
			}
			if _, disposition := sealed.Resolve(entry.population.Relation.Surface, entry.population.Relation.Key); disposition != schema.DispositionAccepted {
				return failure(entry.id, LawPopulationRelationResolves, disposition)
			}
		}
		if entry.population.Kind.Declared() {
			if entry.population.Kind.Surface != schema.SurfaceKindStructure {
				return failure(entry.id, LawPopulationKindPhase, schema.DispositionMalformed)
			}
			if _, disposition := structure.Resolve(sealed, entry.population.Kind.Key, structure.CategoryDiagnosticObservation); disposition != schema.DispositionAccepted {
				return failure(entry.id, LawPopulationKindResolves, disposition)
			}
		}
		if !entry.geometry.Available() {
			return failure(entry.id, LawGeometryDeclared, schema.DispositionIncomplete)
		}
		if entry.geometry.Surface != schema.SurfaceKindStructure {
			return failure(entry.id, LawGeometryPhase, schema.DispositionMalformed)
		}
		if _, disposition := structure.Resolve(sealed, entry.geometry.Key, structure.CategorySemanticRole); disposition != schema.DispositionAccepted {
			return failure(entry.id, LawGeometryResolves, disposition)
		}
		if !entry.anchor.Available() {
			return failure(entry.id, LawAnchorDeclared, schema.DispositionIncomplete)
		}
		if entry.anchor.Surface != schema.SurfaceKindStructure {
			return failure(entry.id, LawAnchorPhase, schema.DispositionMalformed)
		}
		if _, disposition := structure.Resolve(sealed, entry.anchor.Key, structure.CategorySemanticRole); disposition != schema.DispositionAccepted {
			return failure(entry.id, LawAnchorResolves, disposition)
		}
		if !entry.codec.Available() {
			return failure(entry.id, LawCodecDeclared, schema.DispositionIncomplete)
		}
		if entry.codec.Surface != schema.SurfaceKindStructure {
			return failure(entry.id, LawCodecPhase, schema.DispositionMalformed)
		}
		codec, disposition := structure.Resolve(sealed, entry.codec.Key, structure.CategorySemanticRole)
		if disposition != schema.DispositionAccepted {
			return failure(entry.id, LawCodecResolves, disposition)
		}
		producerEnvelope, producerOK := producer.(interface {
			ProducerEnvelope() (query.ProducerEnvelope, bool)
		})
		envelope, envelopeOK := query.ProducerEnvelope{}, false
		if producerOK {
			envelope, envelopeOK = producerEnvelope.ProducerEnvelope()
		}
		codecIdentity, codecOK := vocabulary.Key(codec.Spelling())
		if !producerOK || !envelopeOK || !envelope.Available() || !codecOK || envelope.Codec != codecIdentity {
			return failure(entry.id, LawProducerCompatibility, schema.DispositionMalformed)
		}
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindObservation, entry, law, disposition)
}

// Table is the immutable projection a consumer reads the sealed observation
// inventory through. It is intentionally only a row projection; runtime
// behavior and result payloads do not enter this table.
type Table struct {
	entries   []*Entry
	available bool
}

// NewTable projects one sealed observation view.
func NewTable(view schema.View) (Table, bool) {
	if view.Kind() != schema.SurfaceKindObservation || !view.Available() {
		return Table{}, false
	}
	entries := make([]*Entry, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return Table{}, false
		}
		entries[position] = entry
	}
	return Table{entries: entries, available: true}, true
}

func (table Table) Available() bool { return table.available }

func (table Table) Count() int {
	if !table.available {
		return 0
	}
	return len(table.entries)
}

func (table Table) At(position int) (*Entry, bool) {
	if !table.available || position < 0 || position >= len(table.entries) {
		return nil, false
	}
	return table.entries[position], true
}
