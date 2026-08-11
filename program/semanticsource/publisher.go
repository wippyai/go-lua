package semanticsource

import (
	"errors"
	"sort"
)

var (
	// ErrInvalidPublication rejects a zero, forged, or malformed publication.
	ErrInvalidPublication = errors.New("semantic source: invalid publication")
	// ErrUnexpectedPublication rejects a relation outside the one Schema.
	ErrUnexpectedPublication = errors.New("semantic source: unexpected publication")
	// ErrDuplicatePublication rejects a second publication of one expected
	// relation or facet.
	ErrDuplicatePublication = errors.New("semantic source: duplicate publication")
	// ErrMissingPublication rejects sealing before every Schema definition has
	// been reported, including zero-row definitions.
	ErrMissingPublication = errors.New("semantic source: missing publication")
	// ErrPublicationOrder rejects a fixed owner interval whose otherwise valid
	// rows were supplied in a different order than its declared schema range.
	ErrPublicationOrder = errors.New("semantic source: publication order")
	// ErrPublisherSealed rejects additions after successful finalization.
	ErrPublisherSealed = errors.New("semantic source: publisher is sealed")
)

const publicationSeal uint64 = 0xA6F1_4B3C_91D8_52E7

// Publication is an owner-reported cardinality claim for one generated
// relation. It deliberately does not carry erased rows or claim that this
// package checked their semantics or derivation.
type Publication struct {
	definition RelationDef
	count      int
	seal       uint64
}

// Definition reports the generated relation definition.
func (p Publication) Definition() RelationDef { return p.definition }

// Count includes zero for a required relation with an admitted zero claim.
func (p Publication) Count() int { return p.count }

func (p Publication) valid() bool {
	return p.seal == publicationSeal && p.definition.valid() && p.count >= 0
}

// SealPublication emits an owner-reported cardinality claim. It checks only
// this cold package's identity and non-negative-count contract; it is not an
// erased row-validation API. Owner wrapper laws separately establish the reported
// count against frozen typed rows. Final assembly separately derives and
// verifies publications from sealed Program, Target, and Link owners.
func SealPublication(definition RelationDef, count int) (Publication, error) {
	if !definition.valid() || count < 0 {
		return Publication{}, ErrInvalidPublication
	}
	return Publication{definition: definition, count: count, seal: publicationSeal}, nil
}

// Publisher accumulates exactly one owner-reported measure for every relation
// in one immutable Schema. It checks only identity and completeness of the
// admitted cardinality claims; it is the sole publication state machine.
type Publisher struct {
	schema   Schema
	accepted map[Token]Publication
	sealed   bool
}

// NewPublisher starts a publication cut over one issued immutable Schema.
func NewPublisher(schema Schema) (*Publisher, error) {
	if err := schema.validationError(); err != nil {
		return nil, err
	}
	return &Publisher{
		schema:   schema,
		accepted: make(map[Token]Publication, schema.Count()),
	}, nil
}

// Schema reports the immutable expected denominator.
func (s *Publisher) Schema() Schema {
	if s == nil {
		return Schema{}
	}
	return s.schema
}

// Accept records exactly one owner-reported relation measure. Facets may arrive
// before their primary relation because Schema validates parenthood for the
// complete denominator rather than imposing an accidental insertion order.
func (s *Publisher) Accept(value Publication) error {
	if s == nil || !s.schema.valid() || s.accepted == nil {
		return ErrInvalidSchema
	}
	if s.sealed {
		return ErrPublisherSealed
	}
	if !value.valid() {
		return ErrInvalidPublication
	}
	if !s.schema.has(value.definition.token) {
		return ErrUnexpectedPublication
	}
	if _, exists := s.accepted[value.definition.token]; exists {
		return ErrDuplicatePublication
	}
	s.accepted[value.definition.token] = value
	return nil
}

// Seal publishes detached token-sorted measures after every expected primary
// relation and facet has been supplied exactly once.
func (s *Publisher) Seal() (Publications, error) {
	if s == nil || !s.schema.valid() || s.accepted == nil {
		return Publications{}, ErrInvalidSchema
	}
	if s.sealed {
		return Publications{}, ErrPublisherSealed
	}
	if len(s.accepted) != s.schema.Count() {
		return Publications{}, ErrMissingPublication
	}
	measures := make([]Measure, 0, s.schema.Count())
	for _, definition := range s.schema.definitions {
		publication, exists := s.accepted[definition.token]
		if !exists {
			return Publications{}, ErrMissingPublication
		}
		measures = append(measures, Measure{token: definition.token, count: publication.count})
	}
	sort.Slice(measures, func(left, right int) bool {
		return compareToken(measures[left].token, measures[right].token) < 0
	})
	s.sealed = true
	return Publications{schema: s.schema, measures: measures}, nil
}

// Measure is one detached admitted owner-reported relation cardinality claim.
// It carries only the generated Token and count; typed rows never become a
// second untyped semantic transport.
type Measure struct {
	token Token
	count int
}

// Token reports the canonical relation identity.
func (m Measure) Token() Token { return m.token }

// Count reports the admitted owner-reported cardinality claim, including zero.
func (m Measure) Count() int { return m.count }

// Publications is the immutable detached cardinality result of a completed
// Publisher. It is the cold source-catalog measure consumed by final
// assembly checks, never by the hot solver.
type Publications struct {
	schema   Schema
	measures []Measure
}

// Schema reports the exact immutable denominator used for publication.
func (p Publications) Schema() Schema { return p.schema }

// Count reports every expected definition, including zero-row publications.
func (p Publications) Count() int { return len(p.measures) }

// At returns one detached canonical token-sorted measure.
func (p Publications) At(index int) (Measure, bool) {
	if !p.schema.valid() || index < 0 || index >= len(p.measures) {
		return Measure{}, false
	}
	return p.measures[index], true
}

// Measures returns a detached canonical token-sorted copy.
func (p Publications) Measures() []Measure {
	if !p.schema.valid() {
		return nil
	}
	return append([]Measure(nil), p.measures...)
}

// Clone returns a detached immutable copy of the completed publication
// denominator without reopening or reassembling its schema.
func (p Publications) Clone() Publications {
	if !p.schema.valid() {
		return Publications{}
	}
	return Publications{schema: p.schema, measures: append([]Measure(nil), p.measures...)}
}
