// Package wire is the shared artifact-decoding discipline for the canonical
// Program family owners. It carries the family-neutral scalar reads and the
// adversarial-input gates every section decoder needs, so a family package
// states its own field order and its own term constraints and restates
// neither the allocation gate nor the scalar bounds.
//
// It deliberately owns no schema: record framing, field order, and which
// families a term may name all stay with the family that authored the rows.
package wire

import (
	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/internal/framing"
)

// Count is the allocation gate for every decoded collection. The canonical
// ordinal ceiling is deliberately stricter than the machine's int ceiling,
// and the remaining-byte floor ensures an adversarial arity cannot reserve
// more rows than the unread stream can possibly contain. rowMinimum is the
// smallest number of bytes one row of the collection can occupy.
func Count(reader *framing.Reader, rowMinimum uint64) (int, error) {
	if reader == nil || rowMinimum == 0 {
		return 0, framing.ErrMalformed
	}
	value, err := reader.Count()
	if err != nil {
		return 0, err
	}
	if value > uint64(keyspace.MaxTermOrdinal) || value > uint64(^uint(0)>>1) ||
		value > uint64(reader.Remaining())/rowMinimum {
		return 0, framing.ErrMalformed
	}
	return int(value), nil
}

// Uint32 reads one scalar that must fit the canonical 32-bit domain.
func Uint32(reader *framing.Reader) (uint32, error) {
	if reader == nil {
		return 0, framing.ErrMalformed
	}
	value, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, framing.ErrMalformed
	}
	return uint32(value), nil
}

// Uint reads one unconstrained scalar, for a field whose whole 64-bit range
// is authored payload.
func Uint(reader *framing.Reader) (uint64, error) {
	if reader == nil {
		return 0, framing.ErrMalformed
	}
	return reader.Uint()
}

// Enum reads one closed vocabulary ordinal. Zero is never a member: every
// Program enum starts at one so an absent field cannot decode as a value.
func Enum(reader *framing.Reader, max uint64) (uint8, error) {
	value, err := Uint32(reader)
	if err != nil {
		return 0, err
	}
	if value == 0 || uint64(value) > max {
		return 0, framing.ErrMalformed
	}
	return uint8(value), nil
}

// Term reads one canonical term. A nonzero term must name a real family; the
// caller adds whichever family constraint its relation requires.
func Term(reader *framing.Reader) (keyspace.Term, error) {
	value, err := Uint32(reader)
	if err != nil {
		return 0, err
	}
	term := keyspace.Term(value)
	if term != 0 && keyspace.TermFamily(term) == keyspace.FamilyInvalid {
		return 0, framing.ErrMalformed
	}
	return term, nil
}

// ConstrainedTerm reads one canonical term that must satisfy admit. A zero
// term is refused: this is the read for a field that must name something.
func ConstrainedTerm(reader *framing.Reader, admit func(keyspace.Family) bool) (keyspace.Term, error) {
	term, err := Term(reader)
	if err != nil {
		return 0, err
	}
	family := keyspace.TermFamily(term)
	if family == keyspace.FamilyInvalid || admit == nil || !admit(family) {
		return 0, framing.ErrMalformed
	}
	return term, nil
}

// Key reads one nonzero Source key handle.
func Key(reader *framing.Reader) (keyspace.Key, error) {
	value, err := Uint32(reader)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, framing.ErrMalformed
	}
	return keyspace.Key(value), nil
}

// Bool reads one framed boolean.
func Bool(reader *framing.Reader) (bool, error) {
	if reader == nil {
		return false, framing.ErrMalformed
	}
	return reader.Bool()
}

// Coordinate reads the four exact authored source coordinate parts and
// refuses any quadruple the Source owner would not have minted.
func Coordinate(reader *framing.Reader) (source.Coordinate, error) {
	var parts [4]uint32
	for index := range parts {
		value, err := Uint32(reader)
		if err != nil {
			return source.Coordinate{}, err
		}
		parts[index] = value
	}
	coordinate, ok := source.CoordinateFromParts(parts[0], parts[1], parts[2], parts[3])
	if !ok {
		return source.Coordinate{}, framing.ErrMalformed
	}
	return coordinate, nil
}

// TermSequence reads one counted term sequence of at least minimum members,
// each satisfying admit. retain false validates the sequence and allocates
// nothing, which is the preflight half of a section decoder; the returned
// count is authoritative in both modes, so a relation whose law depends on
// arity does not have to retain rows to learn it.
func TermSequence(reader *framing.Reader, minimum int, retain bool, admit func(keyspace.Family) bool) ([]keyspace.Term, int, error) {
	count, err := Count(reader, UintWireMin)
	if err != nil {
		return nil, 0, err
	}
	if count < minimum {
		return nil, 0, framing.ErrMalformed
	}
	var terms []keyspace.Term
	if retain {
		terms = make([]keyspace.Term, count)
	}
	for index := 0; index < count; index++ {
		term, err := ConstrainedTerm(reader, admit)
		if err != nil {
			return nil, 0, err
		}
		if retain {
			terms[index] = term
		}
	}
	return terms, count, nil
}

// KeySequence reads one counted key sequence of at least minimum members. The
// returned count is authoritative in both modes.
func KeySequence(reader *framing.Reader, minimum int, retain bool) ([]keyspace.Key, int, error) {
	count, err := Count(reader, UintWireMin)
	if err != nil {
		return nil, 0, err
	}
	if count < minimum {
		return nil, 0, framing.ErrMalformed
	}
	var keys []keyspace.Key
	if retain {
		keys = make([]keyspace.Key, count)
	}
	for index := 0; index < count; index++ {
		key, err := Key(reader)
		if err != nil {
			return nil, 0, err
		}
		if retain {
			keys[index] = key
		}
	}
	return keys, count, nil
}

// UintWireMin is the smallest number of stream bytes one framed scalar can
// occupy. Family owners compose their per-row minimum from it.
const UintWireMin = uint64(3)

// BoolWireMin is the smallest number of stream bytes one framed boolean can
// occupy.
const BoolWireMin = uint64(3)

// WriteCoordinate emits the four exact authored source coordinate parts. It
// is the writer half of Coordinate, kept beside it so one definition fixes
// the field order both directions.
func WriteCoordinate(writer *framing.Writer, coordinate source.Coordinate) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	startLine, startColumn, endLine, endColumn := coordinate.Parts()
	for _, part := range [...]uint32{startLine, startColumn, endLine, endColumn} {
		if err := writer.Uint(uint64(part)); err != nil {
			return err
		}
	}
	return nil
}

// WriteTermSpan emits one counted term sequence from a sealed column window.
// Column layout is storage, so the wire carries the semantic sequence.
func WriteTermSpan(writer *framing.Writer, pool rows.Pool[keyspace.Term], span rows.Span) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(pool.Count(span))); err != nil {
		return err
	}
	for _, term := range pool.All(span) {
		if err := writer.Uint(uint64(term)); err != nil {
			return err
		}
	}
	return nil
}

// WriteKeySpan emits one counted key sequence from a sealed column window.
func WriteKeySpan(writer *framing.Writer, pool rows.Pool[keyspace.Key], span rows.Span) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(pool.Count(span))); err != nil {
		return err
	}
	for _, key := range pool.All(span) {
		if err := writer.Uint(uint64(key)); err != nil {
			return err
		}
	}
	return nil
}
