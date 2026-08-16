package outputowners

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/schema/relations"
)

// Build derives every Program-owned output from the canonical relation schema.
// The schema is the sole owner authority; this package adds no owner mapping.
func Build() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Program output owners: canonical schema: %w", err)
	}
	return derive(schema)
}

// Current validates the checked-in generated evidence against its canonical
// schema before a consumer may use it.
func Current() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Program output owners: canonical schema: %w", err)
	}
	if err := Generated.Validate(schema); err != nil {
		return Evidence{}, err
	}
	return Generated, nil
}

func derive(schema *relations.Schema) (Evidence, error) {
	if schema == nil {
		return Evidence{}, fmt.Errorf("Program output owners: nil canonical schema")
	}
	rows := make([]Row, 0, schema.Count())
	for _, relation := range schema.Rows() {
		if relation.Owner < relations.OwnerProgramSource || relation.Owner > relations.OwnerProgramModule {
			continue
		}
		output, ok := relations.CatalogName(relation.Definition.Token())
		if !ok {
			return Evidence{}, fmt.Errorf("Program output owners: unnamed canonical output")
		}
		rows = append(rows, Row{Output: output, Owner: relation.Owner})
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].Output < rows[right].Output })
	evidence := Evidence{SchemaDigest: fmt.Sprintf("%x", schema.Digest()), Rows: rows}
	evidence.Digest = digest(evidence)
	if err := evidence.Validate(schema); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Validate rejects every incomplete, duplicate, foreign, stale, or malformed
// output-owner row. It compares against the live canonical schema, not a
// second maintained list.
func (e Evidence) Validate(schema *relations.Schema) error {
	expected, err := expectedRows(schema)
	if err != nil {
		return err
	}
	if e.SchemaDigest != fmt.Sprintf("%x", schema.Digest()) || e.Digest != digest(e) {
		return fmt.Errorf("Program output owners: stale or invalid evidence digest")
	}
	seen := make(map[string]bool, len(e.Rows))
	for index, row := range e.Rows {
		owner, known := expected[row.Output]
		if !known {
			return fmt.Errorf("Program output owners: unknown output row %q", row.Output)
		}
		if seen[row.Output] {
			return fmt.Errorf("Program output owners: duplicate output row %q", row.Output)
		}
		if index != 0 && e.Rows[index-1].Output >= row.Output {
			return fmt.Errorf("Program output owners: output rows are not canonical")
		}
		if row.Owner != owner {
			return fmt.Errorf("Program output owners: output %q has owner %d, want %d", row.Output, row.Owner, owner)
		}
		seen[row.Output] = true
	}
	if len(seen) != len(expected) {
		for output := range expected {
			if !seen[output] {
				return fmt.Errorf("Program output owners: missing output row %q", output)
			}
		}
	}
	return nil
}

func expectedRows(schema *relations.Schema) (map[string]relations.Owner, error) {
	if schema == nil {
		return nil, fmt.Errorf("Program output owners: nil canonical schema")
	}
	expected := make(map[string]relations.Owner)
	for _, relation := range schema.Rows() {
		if relation.Owner < relations.OwnerProgramSource || relation.Owner > relations.OwnerProgramModule {
			continue
		}
		output, ok := relations.CatalogName(relation.Definition.Token())
		if !ok {
			return nil, fmt.Errorf("Program output owners: unnamed canonical output")
		}
		if _, duplicate := expected[output]; duplicate {
			return nil, fmt.Errorf("Program output owners: duplicate canonical output %q", output)
		}
		expected[output] = relation.Owner
	}
	if len(expected) == 0 {
		return nil, fmt.Errorf("Program output owners: empty canonical output denominator")
	}
	return expected, nil
}

// Canonical returns the exact detached byte stream used to derive Digest.
// Digest itself is omitted because it is derivable from these bytes.
func (e Evidence) Canonical() []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "%s\n", e.SchemaDigest)
	for _, row := range e.Rows {
		fmt.Fprintf(&out, "%s\x00%d\n", row.Output, row.Owner)
	}
	return append([]byte(nil), out.Bytes()...)
}

func digest(e Evidence) string {
	sum := sha256.Sum256(e.Canonical())
	return hex.EncodeToString(sum[:])
}
