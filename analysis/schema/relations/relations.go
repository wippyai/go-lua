package relations

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"

	"github.com/wippyai/go-lua/analysis/schema/relations/internal/scc"
)

const (
	canonicalDomain  = "program.schema.relations"
	canonicalVersion = 2

	relationRecord = 1
)

var (
	// ErrInvalidDefinitions rejects an absent or malformed generated source
	// definition denominator.
	ErrInvalidDefinitions = errors.New("relation schema: invalid source definitions")
	// ErrInvalidDefinition rejects an empty or unissued row definition.
	ErrInvalidDefinition = errors.New("relation schema: invalid relation definition")
	// ErrUnknownDefinition rejects a definition outside the supplied generated
	// semantic-source denominator.
	ErrUnknownDefinition = errors.New("relation schema: unknown relation definition")
	// ErrDuplicateDefinition rejects more than one schema row for a token.
	ErrDuplicateDefinition = errors.New("relation schema: duplicate relation definition")
	// ErrMissingDefinition rejects a source definition without a schema row.
	ErrMissingDefinition = errors.New("relation schema: missing relation definition")
	// ErrInvalidOwner rejects an owner outside the existing Program/Target/Link
	// component set.
	ErrInvalidOwner = errors.New("relation schema: invalid owner component")
	// ErrInvalidForm rejects the zero/default or unknown relation form.
	ErrInvalidForm = errors.New("relation schema: invalid relation form")
	// ErrUnknownParent rejects a parent outside this exact schema.
	ErrUnknownParent = errors.New("relation schema: unknown parent relation")
	// ErrDuplicateParent rejects repeated parent tokens on one row.
	ErrDuplicateParent = errors.New("relation schema: duplicate parent relation")
	// ErrCyclicParents rejects a parent cycle that crosses existing component
	// ownership boundaries. Recursive relations inside one Owner are permitted.
	ErrCyclicParents = errors.New("relation schema: cyclic parent relations")
	// ErrCyclicOwners rejects a cyclic dependency between existing component
	// owners, even where individual relation rows do not themselves form a
	// cycle.
	ErrCyclicOwners = errors.New("relation schema: cyclic owner dependencies")
)

// Owner is one existing immutable Program, Target, or Link component. Root
// owners are intentionally absent: semantic relations belong to a vertical
// child, never to a root forwarding surface.
type Owner uint8

const (
	OwnerUnset Owner = iota
	OwnerProgramSource
	OwnerProgramFlow
	OwnerProgramStatic
	OwnerProgramModule
	// OwnerTarget is the one unchanged canonical Target authority. Target is
	// deliberately not decomposed here into a competing sub-owner vocabulary.
	OwnerTarget
	OwnerLinkProject
	OwnerLinkBoundary
	OwnerLinkModule
	OwnerLinkStatic
	OwnerLinkHost
)

func (owner Owner) valid() bool {
	return owner >= OwnerProgramSource && owner <= OwnerLinkHost
}

// Form states how a relation exists at seal. It is not an operation tag and
// cannot be consumed as runtime dispatch vocabulary.
type Form uint8

const (
	FormUnset Form = iota
	// FormAuthored is written directly by the owned lowering boundary.
	FormAuthored
	// FormSealDerived is computed once from owned relations during seal.
	FormSealDerived
	// FormVirtualPredicate is definitionally non-stored; membership is decided
	// by a generated pure predicate over canonical constituents.
	FormVirtualPredicate
)

func (form Form) valid() bool {
	return form >= FormAuthored && form <= FormVirtualPredicate
}

// Row is one compile-time declaration for exactly one semantic-source
// relation. Parents name exact semantic relation tokens, never strings or
// source positions. Storage and index dispositions belong exclusively to the
// separately owned claims ledger.
type Row struct {
	Definition semanticsource.RelationDef
	Owner      Owner
	Form       Form
	Parents    []semanticsource.Token
}

// Schema is the immutable relation-schema authority. Its rows are canonical
// token order and every externally returned slice is detached.
type Schema struct {
	rows      []Row
	canonical []byte
	digest    identity.ContentID
}

// Seal validates one complete declaration table and returns the sole
// canonical relation schema. The rows themselves are the denominator; there
// is no second generated definition catalog to compare against.
func Seal(rows []Row) (*Schema, error) {
	known, err := knownDefinitions(rows)
	if err != nil {
		return nil, err
	}
	sealed, err := validateRows(known, rows)
	if err != nil {
		return nil, err
	}
	if err := validateParentGraph(sealed); err != nil {
		return nil, err
	}
	encoded, err := encode(sealed)
	if err != nil {
		return nil, err
	}
	digest, ok := identity.DeriveContentID("program.schema.relations/digest/v2", encoded)
	if !ok {
		return nil, ErrInvalidDefinitions
	}
	return &Schema{
		rows:      sealed,
		canonical: encoded,
		digest:    digest,
	}, nil
}

func knownDefinitions(rows []Row) (map[semanticsource.Token]semanticsource.RelationDef, error) {
	if len(rows) == 0 {
		return nil, ErrInvalidDefinitions
	}
	known := make(map[semanticsource.Token]semanticsource.RelationDef, len(rows))
	for _, row := range rows {
		definition := row.Definition
		token := definition.Token()
		if definition == (semanticsource.RelationDef{}) || token == (semanticsource.Token{}) {
			continue
		}
		expected, ok := semanticsource.Declare(token.Origin(), token.Facet())
		if !ok || token == (semanticsource.Token{}) || expected != definition {
			return nil, ErrInvalidDefinitions
		}
		// Keep the first declaration so validateRows can report a duplicate
		// row precisely. The row table, not this map, is the admitted input
		// denominator.
		if _, exists := known[token]; !exists {
			known[token] = definition
		}
	}
	for token := range known {
		if token.Primary() {
			continue
		}
		parent, ok := semanticsource.Declare(token.Origin(), 0)
		if !ok {
			return nil, ErrInvalidDefinitions
		}
		if _, exists := known[parent.Token()]; !exists {
			return nil, ErrInvalidDefinitions
		}
	}
	return known, nil
}

func validateRows(known map[semanticsource.Token]semanticsource.RelationDef, rows []Row) ([]Row, error) {
	if len(rows) != len(known) {
		// Preserve the more specific duplicate/unknown errors when possible; a
		// complete denominator check occurs after individual rows are examined.
		if len(rows) == 0 {
			return nil, ErrMissingDefinition
		}
	}
	sealed := make([]Row, 0, len(rows))
	seen := make(map[semanticsource.Token]struct{}, len(rows))
	for _, row := range rows {
		token := row.Definition.Token()
		if row.Definition == (semanticsource.RelationDef{}) || token == (semanticsource.Token{}) {
			return nil, ErrInvalidDefinition
		}
		definition, exists := known[token]
		if !exists || definition != row.Definition {
			return nil, fmt.Errorf("%w: %v", ErrUnknownDefinition, token)
		}
		if _, duplicate := seen[token]; duplicate {
			return nil, fmt.Errorf("%w: %v", ErrDuplicateDefinition, token)
		}
		if !row.Owner.valid() {
			return nil, ErrInvalidOwner
		}
		if !row.Form.valid() {
			return nil, ErrInvalidForm
		}
		parents, err := canonicalParents(known, token, row.Parents)
		if err != nil {
			return nil, err
		}
		seen[token] = struct{}{}
		sealed = append(sealed, Row{
			Definition: definition,
			Owner:      row.Owner,
			Form:       row.Form,
			Parents:    parents,
		})
	}
	for token := range known {
		if _, exists := seen[token]; !exists {
			return nil, fmt.Errorf("%w: %v", ErrMissingDefinition, token)
		}
	}
	sort.Slice(sealed, func(left, right int) bool {
		return lessToken(sealed[left].Definition.Token(), sealed[right].Definition.Token())
	})
	return sealed, nil
}

func canonicalParents(known map[semanticsource.Token]semanticsource.RelationDef, self semanticsource.Token, parents []semanticsource.Token) ([]semanticsource.Token, error) {
	owned := append([]semanticsource.Token(nil), parents...)
	sort.Slice(owned, func(left, right int) bool { return lessToken(owned[left], owned[right]) })
	for index, parent := range owned {
		if _, exists := known[parent]; !exists || parent == (semanticsource.Token{}) {
			return nil, fmt.Errorf("%w: %v", ErrUnknownParent, parent)
		}
		if parent == self || (index > 0 && parent == owned[index-1]) {
			if parent == self {
				return nil, fmt.Errorf("%w: %v", ErrCyclicParents, parent)
			}
			return nil, fmt.Errorf("%w: %v", ErrDuplicateParent, parent)
		}
	}
	return owned, nil
}

func validateParentGraph(rows []Row) error {
	byToken := make(map[semanticsource.Token]Row, len(rows))
	for _, row := range rows {
		byToken[row.Definition.Token()] = row
	}
	for _, component := range parentSCCs(rows) {
		if len(component) < 2 {
			continue
		}
		owner := byToken[component[0]].Owner
		for _, token := range component[1:] {
			if byToken[token].Owner != owner {
				return fmt.Errorf("%w: %v", ErrCyclicParents, component)
			}
		}
	}
	return validateOwnerGraph(rows)
}

// parentSCCs reports the exact parent-graph strongly connected components in
// stable token order. It is a verification aid only: SCC membership is derived
// from rows at seal and is never retained in Schema or exposed as vocabulary.
func parentSCCs(rows []Row) [][]semanticsource.Token {
	byToken := make(map[semanticsource.Token]Row, len(rows))
	tokens := make([]semanticsource.Token, 0, len(rows))
	for _, row := range rows {
		token := row.Definition.Token()
		byToken[token] = row
		tokens = append(tokens, token)
	}
	return scc.Components(tokens, func(token semanticsource.Token) []semanticsource.Token {
		return byToken[token].Parents
	}, lessToken)
}

func validateOwnerGraph(rows []Row) error {
	ownersByToken := make(map[semanticsource.Token]Owner, len(rows))
	for _, row := range rows {
		ownersByToken[row.Definition.Token()] = row.Owner
	}
	edges := make(map[Owner]map[Owner]struct{}, len(rows))
	for _, row := range rows {
		for _, parent := range row.Parents {
			parentOwner := ownersByToken[parent]
			if parentOwner == row.Owner {
				continue
			}
			if edges[row.Owner] == nil {
				edges[row.Owner] = make(map[Owner]struct{})
			}
			edges[row.Owner][parentOwner] = struct{}{}
		}
	}
	owners := make([]Owner, 0, len(edges))
	for owner := range edges {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(left, right int) bool { return owners[left] < owners[right] })
	const (
		ownerUnseen = iota
		ownerVisiting
		ownerComplete
	)
	state := make(map[Owner]uint8, len(owners))
	var visit func(Owner) error
	visit = func(owner Owner) error {
		switch state[owner] {
		case ownerVisiting:
			return fmt.Errorf("%w: %d", ErrCyclicOwners, owner)
		case ownerComplete:
			return nil
		}
		state[owner] = ownerVisiting
		parents := make([]Owner, 0, len(edges[owner]))
		for parent := range edges[owner] {
			parents = append(parents, parent)
		}
		sort.Slice(parents, func(left, right int) bool { return parents[left] < parents[right] })
		for _, parent := range parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[owner] = ownerComplete
		return nil
	}
	for _, owner := range owners {
		if err := visit(owner); err != nil {
			return err
		}
	}
	return nil
}

func lessToken(left, right semanticsource.Token) bool {
	if left.Origin() != right.Origin() {
		return left.Origin() < right.Origin()
	}
	if left.Facet() != right.Facet() {
		return left.Facet() < right.Facet()
	}
	if left.Revision() != right.Revision() {
		return left.Revision() < right.Revision()
	}
	return left.Digest() < right.Digest()
}

func encode(rows []Row) ([]byte, error) {
	var out bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&out, canonicalDomain, canonicalVersion); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(len(rows))); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Record(relationRecord); err != nil {
			return nil, err
		}
		if err := writeToken(&writer, row.Definition.Token()); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(row.Owner)); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(row.Form)); err != nil {
			return nil, err
		}
		if err := writer.Count(uint64(len(row.Parents))); err != nil {
			return nil, err
		}
		for _, parent := range row.Parents {
			if err := writeToken(&writer, parent); err != nil {
				return nil, err
			}
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), nil
}

func writeToken(writer *framing.Writer, token semanticsource.Token) error {
	for _, value := range [...]uint64{
		uint64(token.Origin()), uint64(token.Facet()), uint64(token.Revision()), token.Digest(),
	} {
		if err := writer.Uint(value); err != nil {
			return err
		}
	}
	return nil
}

// Count reports the complete relation denominator size.
func (schema *Schema) Count() int {
	if schema == nil {
		return 0
	}
	return len(schema.rows)
}

// Rows returns a detached token-sorted snapshot.
func (schema *Schema) Rows() []Row {
	if schema == nil {
		return nil
	}
	rows := make([]Row, len(schema.rows))
	for index, row := range schema.rows {
		rows[index] = row
		rows[index].Parents = append([]semanticsource.Token(nil), row.Parents...)
	}
	return rows
}

// OwnerDefinitions returns the canonical-order relation definitions owned by
// one component. Expected is the owner's fixed publication interval width: a
// schema that no longer states exactly that many owned relations is rejected
// instead of silently narrowing the interval an owner seals.
func OwnerDefinitions(owner Owner, expected int) ([]semanticsource.RelationDef, bool) {
	if !owner.valid() || expected <= 0 {
		return nil, false
	}
	schema, err := CanonicalSchema()
	if err != nil || schema == nil {
		return nil, false
	}
	definitions := make([]semanticsource.RelationDef, 0, expected)
	for _, row := range schema.rows {
		if row.Owner == owner {
			definitions = append(definitions, row.Definition)
		}
	}
	if len(definitions) != expected {
		return nil, false
	}
	return definitions, true
}

// Canonical returns a detached deterministic encoding of this schema.
func (schema *Schema) Canonical() []byte {
	if schema == nil {
		return nil
	}
	return append([]byte(nil), schema.canonical...)
}

// Digest returns the SHA-256 identity of Canonical.
func (schema *Schema) Digest() identity.ContentID {
	if schema == nil {
		return identity.ContentID{}
	}
	return schema.digest
}

// SchemaDigest implements semanticsource.ProgramSchema.
func (schema *Schema) SchemaDigest() identity.ContentID { return schema.Digest() }

// DefinitionAt implements semanticsource.ProgramSchema in canonical order.
func (schema *Schema) DefinitionAt(index int) (semanticsource.RelationDef, bool) {
	if schema == nil || index < 0 || index >= len(schema.rows) {
		return semanticsource.RelationDef{}, false
	}
	return schema.rows[index].Definition, true
}

// Definition resolves one declaration only when the pair belongs to this
// sealed denominator.
func (schema *Schema) Definition(origin semanticsource.Origin, facet semanticsource.Facet) (semanticsource.RelationDef, bool) {
	if schema == nil {
		return semanticsource.RelationDef{}, false
	}
	want, ok := semanticsource.Declare(origin, facet)
	if !ok {
		return semanticsource.RelationDef{}, false
	}
	index := sort.Search(len(schema.rows), func(index int) bool {
		return !lessToken(schema.rows[index].Definition.Token(), want.Token())
	})
	if index >= len(schema.rows) || schema.rows[index].Definition != want {
		return semanticsource.RelationDef{}, false
	}
	return schema.rows[index].Definition, true
}
