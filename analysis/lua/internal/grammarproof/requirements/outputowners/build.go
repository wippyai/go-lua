package outputowners

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Build derives every Program-owned output from the generated denominator
// catalog. The catalog is the sole owner authority; this package adds no
// owner mapping.
func Build() (Evidence, error) {
	return derive(denominator.GeneratedRelationEntries())
}

// Current validates the checked-in generated evidence against the live
// denominator catalog before a consumer may use it.
func Current() (Evidence, error) {
	entries := denominator.GeneratedRelationEntries()
	if err := Generated.Validate(entries); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

func derive(entries []*denominator.RelationEntry) (Evidence, error) {
	rows := make([]Row, 0, len(entries))
	seen := make(map[schema.EntryID]bool)
	for _, relation := range entries {
		if relation == nil || !relation.Owner().Program() {
			continue
		}
		if !relation.ID().Available() || seen[relation.ID()] {
			return Evidence{}, fmt.Errorf("program output owners: duplicate or malformed output relation")
		}
		seen[relation.ID()] = true
		rows = append(rows, Row{Relation: relation.ID(), Owner: relation.Owner()})
	}
	if len(rows) == 0 {
		return Evidence{}, fmt.Errorf("program output owners: empty canonical output denominator")
	}
	sort.Slice(rows, func(left, right int) bool { return less(rows[left].Relation, rows[right].Relation) })
	evidence := Evidence{Rows: rows}
	evidence.Digest = digest(evidence)
	if err := evidence.Validate(entries); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Validate rejects every incomplete, duplicate, foreign, stale, or malformed
// output-owner row. It compares against the live generated denominator.
func (e Evidence) Validate(entries []*denominator.RelationEntry) error {
	if e.Digest != digest(e) {
		return fmt.Errorf("program output owners: invalid evidence digest")
	}
	expected, err := expectedRows(entries)
	if err != nil {
		return err
	}
	seen := make(map[schema.EntryID]bool, len(e.Rows))
	for index, row := range e.Rows {
		owner, known := expected[row.Relation]
		if !known {
			return fmt.Errorf("program output owners: unknown output relation")
		}
		if seen[row.Relation] {
			return fmt.Errorf("program output owners: duplicate output relation")
		}
		if index != 0 && !less(e.Rows[index-1].Relation, row.Relation) {
			return fmt.Errorf("program output owners: output rows are not canonical")
		}
		if row.Owner != owner {
			return fmt.Errorf("program output owners: output relation has owner %d, want %d", row.Owner, owner)
		}
		seen[row.Relation] = true
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("program output owners: output denominator is incomplete")
	}
	return nil
}

func expectedRows(entries []*denominator.RelationEntry) (map[schema.EntryID]denominator.RelationOwner, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("program output owners: empty generated denominator catalog")
	}
	expected := make(map[schema.EntryID]denominator.RelationOwner)
	for _, relation := range entries {
		if relation == nil || !relation.Owner().Program() {
			continue
		}
		if _, duplicate := expected[relation.ID()]; duplicate {
			return nil, fmt.Errorf("program output owners: duplicate canonical output")
		}
		expected[relation.ID()] = relation.Owner()
	}
	if len(expected) == 0 {
		return nil, fmt.Errorf("program output owners: empty canonical output denominator")
	}
	return expected, nil
}

// Canonical returns the exact detached byte stream used to derive Digest.
func (e Evidence) Canonical() []byte {
	var out bytes.Buffer
	for _, row := range e.Rows {
		out.Write(row.Relation[:])
		out.WriteByte(0)
		out.WriteByte(byte(row.Owner))
		out.WriteByte('\n')
	}
	return append([]byte(nil), out.Bytes()...)
}

func digest(e Evidence) string {
	sum := sha256.Sum256(e.Canonical())
	return hex.EncodeToString(sum[:])
}

func less(left, right schema.EntryID) bool {
	return bytes.Compare(left[:], right[:]) < 0
}

func clone(e Evidence) Evidence {
	e.Rows = append([]Row(nil), e.Rows...)
	return e
}
