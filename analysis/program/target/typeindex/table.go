// Package typeindex owns the immutable Target-wide qualified type directory.
//
// The directory is deliberately separate from operation-local Values rows:
// names are a Target vocabulary and are never reconstructed from a provider,
// module, or downstream lookup path. Each authored name receives its own
// owner-issued Type handle, even when two declarations have equal structure.
package typeindex

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	sealedrows "github.com/wippyai/go-lua/internal/rows"
)

type row struct {
	name        string
	typ         vocabulary.Type
	declaration schematype.Type
}

// Table is the immutable qualified-name directory composed into a Target
// Contract. Its rows and declarations are private; callers receive only
// scalar handles, exact names, or ownership-isolated declaration values.
type Table struct {
	rows sealedrows.Rows[row]
}

// Compile validates, canonicalizes, and seals one complete qualified type
// directory. Names are sorted by exact spelling so sealing is independent of
// declaration order. Exact duplicates and case-fold collisions are refused:
// either would make a name lookup ambiguous or silently choose one owner.
func Compile(input []vocabulary.QualifiedTypeSpec) (Table, error) {
	if _, err := vocabulary.CheckedStoredLength("qualified type table", len(input)); err != nil {
		return Table{}, err
	}
	rows := make([]row, len(input))
	for index, item := range input {
		if err := validateName(item.Name); err != nil {
			return Table{}, fmt.Errorf("target/typeindex: type %d: %w", index, err)
		}
		if !item.Declaration.Available() {
			return Table{}, fmt.Errorf("target/typeindex: type %q has no declaration", item.Name)
		}
		rows[index] = row{name: item.Name, declaration: item.Declaration}
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].name < rows[right].name })
	caseNames := make(map[string]string, len(rows))
	for index := range rows {
		if index != 0 && rows[index-1].name == rows[index].name {
			return Table{}, fmt.Errorf("target/typeindex: duplicate qualified type %q", rows[index].name)
		}
		folded := strings.ToLower(rows[index].name)
		if previous, exists := caseNames[folded]; exists {
			return Table{}, fmt.Errorf("target/typeindex: ambiguous case-variant qualified types %q and %q", previous, rows[index].name)
		}
		caseNames[folded] = rows[index].name
		rows[index].typ = vocabulary.Type(index + 1)
	}
	return Table{rows: sealedrows.NewRows(rows)}, nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("qualified type name is empty")
	}
	if !utf8.ValidString(name) {
		return errors.New("qualified type name is not valid UTF-8")
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fmt.Errorf("qualified type name %q has no owner segment", name)
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("qualified type name %q has an empty segment", name)
		}
		if strings.TrimSpace(part) != part {
			return fmt.Errorf("qualified type name %q has whitespace in a segment", name)
		}
	}
	return nil
}

// Count returns the number of named type rows in the sealed directory.
func (table Table) Count() int { return table.rows.Count() }

// At enumerates one row in canonical exact-name order. The name is the
// authored qualified spelling; callers never need to rebuild it.
func (table Table) At(index int) (name string, typ vocabulary.Type, ok bool) {
	item, ok := table.rows.At(index)
	if !ok {
		return "", 0, false
	}
	return item.name, item.typ, true
}

// Lookup returns the owner-issued Type handle for one exact qualified name.
// No case folding or other normalization is applied at lookup time.
func (table Table) Lookup(name string) (vocabulary.Type, bool) {
	index := sort.Search(table.rows.Count(), func(index int) bool {
		item, ok := table.rows.At(index)
		return ok && item.name >= name
	})
	item, ok := table.rows.At(index)
	if !ok || item.name != name {
		return 0, false
	}
	return item.typ, true
}

// Name returns the exact authored spelling for one owner-issued Type handle.
func (table Table) Name(typ vocabulary.Type) (string, bool) {
	if typ == 0 || uint64(typ) > uint64(table.rows.Count()) {
		return "", false
	}
	item, ok := table.rows.At(int(typ) - 1)
	if !ok || item.typ != typ {
		return "", false
	}
	return item.name, true
}

// Declaration returns an ownership-isolated neutral declaration for one
// owner-issued Type handle.
func (table Table) Declaration(typ vocabulary.Type) (schematype.Type, bool) {
	if typ == 0 || uint64(typ) > uint64(table.rows.Count()) {
		return schematype.Type{}, false
	}
	item, ok := table.rows.At(int(typ) - 1)
	if !ok || item.typ != typ {
		return schematype.Type{}, false
	}
	return item.declaration, true
}
