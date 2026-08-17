package targetingress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var (
	ErrMissingRow   = errors.New("target ingress requirements: missing canonical Target relation")
	ErrDuplicateRow = errors.New("target ingress requirements: duplicate canonical Target relation")
	ErrUnknownRow   = errors.New("target ingress requirements: unknown Target ingress relation")
	ErrInvalidRow   = errors.New("target ingress requirements: invalid Target ingress row")
)

// Build derives the complete Target relation vocabulary and exact parent
// ingress contexts from the generated denominator catalog. It does not
// inspect a Target implementation or create a secondary relation vocabulary.
func Build() (Evidence, error) {
	return derive(denominator.GeneratedRelationEntries())
}

// Current validates the checked-in generated evidence against the live
// denominator catalog before a matrix can consume it.
func Current() (Evidence, error) {
	entries := denominator.GeneratedRelationEntries()
	if err := Generated.Validate(entries); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

func derive(entries []*denominator.RelationEntry) (Evidence, error) {
	expected := targetVocabulary(entries)
	if len(expected) == 0 {
		return Evidence{}, ErrMissingRow
	}
	seen := make(map[schema.EntryID]bool, len(expected))
	result := make([]Row, 0, len(expected))
	for _, source := range entries {
		if source == nil || source.Owner() != denominator.RelationOwnerTarget {
			continue
		}
		id := source.ID()
		if !expected[id] {
			return Evidence{}, fmt.Errorf("%w: %x", ErrUnknownRow, id)
		}
		if seen[id] {
			return Evidence{}, fmt.Errorf("%w: %x", ErrDuplicateRow, id)
		}
		row, err := deriveRow(source)
		if err != nil {
			return Evidence{}, err
		}
		seen[id] = true
		result = append(result, row)
	}
	for id := range expected {
		if !seen[id] {
			return Evidence{}, fmt.Errorf("%w: %x", ErrMissingRow, id)
		}
	}
	sort.Slice(result, func(left, right int) bool { return less(result[left].Relation, result[right].Relation) })
	evidence := Evidence{Rows: result}
	evidence.Digest = digest(evidence)
	return evidence, evidence.Validate(entries)
}

func deriveRow(source *denominator.RelationEntry) (Row, error) {
	if source == nil || source.Owner() != denominator.RelationOwnerTarget || !source.Owner().Available() || !source.Form().Available() {
		return Row{}, ErrInvalidRow
	}
	parents := source.Parents()
	for _, parent := range parents {
		if !parent.Available() {
			return Row{}, ErrInvalidRow
		}
	}
	sort.Slice(parents, func(left, right int) bool { return less(parents[left], parents[right]) })
	return Row{Relation: source.ID(), Owner: source.Owner(), Form: source.Form(), Ingress: append([]schema.EntryID(nil), parents...)}, nil
}

// Validate verifies generated evidence against the supplied live canonical
// denominator, detecting staleness and every missing, duplicate, or unknown
// row.
func (e Evidence) Validate(entries []*denominator.RelationEntry) error {
	if e.Digest != digest(e) {
		return ErrInvalidRow
	}
	expected := targetVocabulary(entries)
	if err := e.ValidateRows(expected, entries); err != nil {
		return err
	}
	canonical := make(map[schema.EntryID]*denominator.RelationEntry, len(entries))
	for _, source := range entries {
		if source == nil {
			return ErrInvalidRow
		}
		if source.Owner() == denominator.RelationOwnerTarget {
			canonical[source.ID()] = source
		}
	}
	for _, row := range e.Rows {
		source, ok := canonical[row.Relation]
		if !ok {
			return ErrMissingRow
		}
		want, err := deriveRow(source)
		if err != nil || !equalRow(row, want) {
			return ErrInvalidRow
		}
	}
	return nil
}

// ValidateRows validates the evidence's typed vocabulary independently of
// construction. When entries are supplied, ingress parents are fenced to the
// same generated denominator catalog.
func (e Evidence) ValidateRows(expected map[schema.EntryID]bool, catalogs ...[]*denominator.RelationEntry) error {
	if len(e.Rows) != len(expected) || len(expected) == 0 {
		return ErrMissingRow
	}
	known := make(map[schema.EntryID]bool)
	if len(catalogs) != 0 {
		for _, entry := range catalogs[0] {
			if entry != nil {
				known[entry.ID()] = true
			}
		}
	}
	seen := make(map[schema.EntryID]bool, len(e.Rows))
	for index, row := range e.Rows {
		if !row.Relation.Available() || !expected[row.Relation] {
			return ErrUnknownRow
		}
		if seen[row.Relation] || index != 0 && !less(e.Rows[index-1].Relation, row.Relation) {
			return ErrDuplicateRow
		}
		if row.Owner != denominator.RelationOwnerTarget || !row.Owner.Available() || !row.Form.Available() {
			return ErrInvalidRow
		}
		parents := make(map[schema.EntryID]bool, len(row.Ingress))
		for parentIndex, parent := range row.Ingress {
			if !parent.Available() || len(known) != 0 && !known[parent] || parents[parent] || parentIndex != 0 && !less(row.Ingress[parentIndex-1], parent) {
				return ErrInvalidRow
			}
			parents[parent] = true
		}
		seen[row.Relation] = true
	}
	for id := range expected {
		if !seen[id] {
			return ErrMissingRow
		}
	}
	return nil
}

func targetVocabulary(entries []*denominator.RelationEntry) map[schema.EntryID]bool {
	rows := make(map[schema.EntryID]bool)
	for _, entry := range entries {
		if entry != nil && entry.Owner() == denominator.RelationOwnerTarget {
			rows[entry.ID()] = true
		}
	}
	return rows
}

func less(left, right schema.EntryID) bool {
	return bytes.Compare(left[:], right[:]) < 0
}

func equalRow(left, right Row) bool {
	if left.Relation != right.Relation || left.Owner != right.Owner || left.Form != right.Form || len(left.Ingress) != len(right.Ingress) {
		return false
	}
	for index := range left.Ingress {
		if left.Ingress[index] != right.Ingress[index] {
			return false
		}
	}
	return true
}

// Canonical returns the exact detached byte stream used to derive Digest.
func (e Evidence) Canonical() []byte {
	var out bytes.Buffer
	for _, row := range e.Rows {
		out.Write(row.Relation[:])
		out.WriteByte(0)
		out.WriteByte(byte(row.Owner))
		out.WriteByte(0)
		out.WriteByte(byte(row.Form))
		for _, ingress := range row.Ingress {
			out.WriteByte(0)
			out.Write(ingress[:])
		}
		out.WriteByte('\n')
	}
	return append([]byte(nil), out.Bytes()...)
}

func digest(e Evidence) string {
	sum := sha256.Sum256(e.Canonical())
	return hex.EncodeToString(sum[:])
}

func clone(e Evidence) Evidence {
	e.Rows = append([]Row(nil), e.Rows...)
	for index := range e.Rows {
		e.Rows[index].Ingress = append([]schema.EntryID(nil), e.Rows[index].Ingress...)
	}
	return e
}
