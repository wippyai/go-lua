package targetingress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/semanticsource"
)

var (
	ErrMissingRow   = errors.New("Target ingress requirements: missing canonical Target relation")
	ErrDuplicateRow = errors.New("Target ingress requirements: duplicate canonical Target relation")
	ErrUnknownRow   = errors.New("Target ingress requirements: unknown Target ingress relation")
	ErrInvalidRow   = errors.New("Target ingress requirements: invalid Target ingress row")
)

// Build derives the complete Target relation vocabulary and exact parent
// ingress contexts from the canonical relation schema. It does not inspect a
// Target implementation or create a secondary relation vocabulary.
func Build() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Target ingress requirements: canonical schema: %w", err)
	}
	return derive(schema.Rows(), schema.Digest(), semanticsource.CatalogSchema())
}

// Current validates the checked-in generated evidence against the live
// canonical Target relation semantics before a matrix can consume it.
func Current() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("Target ingress requirements: canonical schema: %w", err)
	}
	if err := Generated.Validate(schema); err != nil {
		return Evidence{}, err
	}
	return Generated, nil
}

func derive(rows []relations.Row, schemaDigest [sha256.Size]byte, vocabulary semanticsource.Schema) (Evidence, error) {
	expected := targetVocabulary(vocabulary)
	if len(expected) == 0 {
		return Evidence{}, ErrMissingRow
	}
	seen := make(map[semanticsource.Token]bool, len(expected))
	result := make([]Row, 0, len(expected))
	for _, source := range rows {
		token := source.Definition.Token()
		if source.Owner != relations.OwnerTarget {
			continue
		}
		if !expected[token] {
			return Evidence{}, fmt.Errorf("%w: %v", ErrUnknownRow, token)
		}
		if seen[token] {
			return Evidence{}, fmt.Errorf("%w: %v", ErrDuplicateRow, token)
		}
		row, err := deriveRow(source)
		if err != nil {
			return Evidence{}, err
		}
		seen[token] = true
		result = append(result, row)
	}
	for token := range expected {
		if !seen[token] {
			return Evidence{}, fmt.Errorf("%w: %v", ErrMissingRow, token)
		}
	}
	sort.Slice(result, func(left, right int) bool { return less(result[left].Relation, result[right].Relation) })
	evidence := Evidence{SchemaDigest: hex.EncodeToString(schemaDigest[:]), Rows: result}
	evidence.Digest = digest(evidence)
	return evidence, evidence.ValidateRows(expected)
}

func deriveRow(source relations.Row) (Row, error) {
	if source.Owner != relations.OwnerTarget || !targetOrigin(source.Definition.Token().Origin()) || source.Form == relations.FormUnset {
		return Row{}, ErrInvalidRow
	}
	ingress := make([]Reference, len(source.Parents))
	for index, parent := range source.Parents {
		if parent == (semanticsource.Token{}) {
			return Row{}, ErrInvalidRow
		}
		ingress[index] = reference(parent)
	}
	return Row{Relation: reference(source.Definition.Token()), Owner: source.Owner, Form: source.Form, Ingress: ingress}, nil
}

// Validate verifies generated evidence against the supplied live canonical
// schema, detecting staleness and every missing, duplicate, or unknown row.
func (e Evidence) Validate(schema *relations.Schema) error {
	if schema == nil || e.SchemaDigest != fmt.Sprintf("%x", schema.Digest()) || e.Digest != digest(e) {
		return ErrInvalidRow
	}
	if err := e.ValidateRows(targetVocabulary(semanticsource.CatalogSchema())); err != nil {
		return err
	}
	canonical := make(map[semanticsource.Token]relations.Row, schema.Count())
	for _, source := range schema.Rows() {
		token := source.Definition.Token()
		if source.Owner != relations.OwnerTarget {
			continue
		}
		if !targetOrigin(token.Origin()) {
			return fmt.Errorf("%w: %v", ErrUnknownRow, token)
		}
		canonical[token] = source
	}
	for _, row := range e.Rows {
		source, ok := canonical[tokenFor(row.Relation)]
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
// construction. It is kept small for mutation laws; production callers use
// Validate so the schema digest and relation semantics are both checked.
func (e Evidence) ValidateRows(expected map[semanticsource.Token]bool) error {
	if len(e.Rows) != len(expected) || len(expected) == 0 {
		return ErrMissingRow
	}
	seen := make(map[semanticsource.Token]bool, len(e.Rows))
	for index, row := range e.Rows {
		token := tokenFor(row.Relation)
		if token == (semanticsource.Token{}) || !expected[token] {
			return ErrUnknownRow
		}
		if seen[token] || index != 0 && !less(e.Rows[index-1].Relation, row.Relation) {
			return ErrDuplicateRow
		}
		if row.Owner != relations.OwnerTarget || !targetOrigin(row.Relation.Origin) || row.Form == relations.FormUnset || row.Form > relations.FormVirtualPredicate {
			return ErrInvalidRow
		}
		parents := make(map[semanticsource.Token]bool, len(row.Ingress))
		for parentIndex, parent := range row.Ingress {
			parentToken := tokenFor(parent)
			if parentToken == (semanticsource.Token{}) || parents[parentToken] || parentIndex != 0 && !less(row.Ingress[parentIndex-1], parent) {
				return ErrInvalidRow
			}
			parents[parentToken] = true
		}
		seen[token] = true
	}
	for token := range expected {
		if !seen[token] {
			return ErrMissingRow
		}
	}
	return nil
}

func targetVocabulary(vocabulary semanticsource.Schema) map[semanticsource.Token]bool {
	rows := make(map[semanticsource.Token]bool)
	for _, definition := range vocabulary.Definitions() {
		if targetOrigin(definition.Token().Origin()) {
			rows[definition.Token()] = true
		}
	}
	return rows
}

func targetOrigin(origin semanticsource.Origin) bool {
	switch origin {
	case semanticsource.OriginTargetContract, semanticsource.OriginTargetOperation, semanticsource.OriginTargetProtocol, semanticsource.OriginTargetBoot, semanticsource.OriginTargetGsub:
		return true
	default:
		return false
	}
}

func reference(token semanticsource.Token) Reference {
	return Reference{Origin: token.Origin(), Facet: token.Facet(), Revision: token.Revision()}
}

func tokenFor(reference Reference) semanticsource.Token {
	for _, definition := range semanticsource.CatalogSchema().Definitions() {
		token := definition.Token()
		if token.Origin() == reference.Origin && token.Facet() == reference.Facet && token.Revision() == reference.Revision {
			return token
		}
	}
	return semanticsource.Token{}
}

func less(left, right Reference) bool {
	if left.Origin != right.Origin {
		return left.Origin < right.Origin
	}
	if left.Facet != right.Facet {
		return left.Facet < right.Facet
	}
	return left.Revision < right.Revision
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
// Digest itself is omitted because it is derivable from these bytes.
func (e Evidence) Canonical() []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "%s\n", e.SchemaDigest)
	for _, row := range e.Rows {
		fmt.Fprintf(&out, "%d\x00%d\x00%d\x00%d\x00%d", row.Relation.Origin, row.Relation.Facet, row.Relation.Revision, row.Owner, row.Form)
		for _, ingress := range row.Ingress {
			fmt.Fprintf(&out, "\x00%d\x00%d\x00%d", ingress.Origin, ingress.Facet, ingress.Revision)
		}
		fmt.Fprintln(&out)
	}
	return append([]byte(nil), out.Bytes()...)
}

func digest(e Evidence) string {
	sum := sha256.Sum256(e.Canonical())
	return hex.EncodeToString(sum[:])
}
