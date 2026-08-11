// Package linkboundary derives the closed Link boundary denominator from the
// canonical relation schema. It deliberately records schema semantics only;
// it neither reads Link implementation rows nor interprets an application.
package linkboundary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const (
	canonicalDomain  = "program.grammarproof.requirements.linkboundary"
	canonicalVersion = 1
	boundaryRecord   = 1
)

var (
	// ErrMissingBoundary rejects a schema with no canonical Link boundary row.
	ErrMissingBoundary = errors.New("link boundary requirements: missing canonical boundary row")
	// ErrDuplicateBoundary rejects more than one canonical Link boundary row.
	ErrDuplicateBoundary = errors.New("link boundary requirements: duplicate canonical boundary row")
	// ErrUnknownBoundary rejects any relation assigned to the Link boundary
	// owner that is not the one issued LinkBoundary relation.
	ErrUnknownBoundary = errors.New("link boundary requirements: unknown boundary row")
	// ErrInvalidBoundary rejects a canonical boundary whose sealed ownership,
	// form, or parent vector no longer states the complete boundary semantics.
	ErrInvalidBoundary = errors.New("link boundary requirements: invalid canonical boundary row")
)

// Reference is the stable issued relation identity carried by one boundary
// row. It is intentionally numeric rather than a Link implementation handle.
type Reference struct {
	Origin   semanticsource.Origin
	Facet    semanticsource.Facet
	Revision semanticsource.Revision
}

// Row is one complete Link boundary semantics row. The source schema has one
// row today; keeping this a slice makes a future schema extension fail closed
// until its exact semantics and tests are added here.
type Row struct {
	Boundary Reference
	Owner    relations.Owner
	Form     relations.Form
	Parents  []Reference
}

// Evidence is generated cold proof material for every canonical Link boundary
// row. SchemaDigest binds the rows to the complete relation-schema authority.
type Evidence struct {
	SchemaDigest string
	Digest       string
	Rows         []Row
}

// Canonical returns the complete validated boundary evidence as one detached
// canonical byte stream. Digest is the hexadecimal SHA-256 of these bytes and
// is therefore derivable rather than redundantly encoded. Invalid evidence has
// no publishable representation.
func (e Evidence) Canonical() []byte {
	if e.Validate() != nil {
		return nil
	}
	encoded, err := encodeCanonical(e)
	if err != nil {
		return nil
	}
	return encoded
}

// Build derives Link boundary evidence from the only canonical relation
// schema. The production Link package is intentionally not an input.
func Build() (Evidence, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return Evidence{}, fmt.Errorf("link boundary requirements: canonical schema: %w", err)
	}
	return derive(schema.Rows(), schema.Digest())
}

func derive(rows []relations.Row, schemaDigest [sha256.Size]byte) (Evidence, error) {
	expected := expectedBoundary()
	if expected == (semanticsource.Token{}) {
		return Evidence{}, ErrMissingBoundary
	}

	var boundaryRows []relations.Row
	for _, row := range rows {
		if row.Owner != relations.OwnerLinkBoundary {
			continue
		}
		if row.Definition.Token() != expected {
			return Evidence{}, fmt.Errorf("%w: %v", ErrUnknownBoundary, reference(row.Definition.Token()))
		}
		boundaryRows = append(boundaryRows, row)
	}
	if len(boundaryRows) == 0 {
		return Evidence{}, ErrMissingBoundary
	}
	if len(boundaryRows) != 1 {
		return Evidence{}, ErrDuplicateBoundary
	}
	row, err := deriveRow(boundaryRows[0], expected)
	if err != nil {
		return Evidence{}, err
	}
	evidence := Evidence{
		SchemaDigest: hex.EncodeToString(schemaDigest[:]),
		Rows:         []Row{row},
	}
	evidence.Digest = digest(evidence)
	return evidence, evidence.Validate()
}

func expectedBoundary() semanticsource.Token {
	for _, definition := range semanticsource.CatalogSchema().Definitions() {
		token := definition.Token()
		if token.Origin() == semanticsource.OriginLinkBoundary && token.Primary() {
			return token
		}
	}
	return semanticsource.Token{}
}

func deriveRow(row relations.Row, expected semanticsource.Token) (Row, error) {
	if row.Definition.Token() != expected || row.Owner != relations.OwnerLinkBoundary || row.Form != relations.FormVirtualPredicate {
		return Row{}, ErrInvalidBoundary
	}
	parents := append([]semanticsource.Token(nil), row.Parents...)
	sort.Slice(parents, func(left, right int) bool { return less(parents[left], parents[right]) })
	want := exactBoundaryParents(parents)
	if len(parents) != len(want) {
		return Row{}, ErrInvalidBoundary
	}
	for index, parent := range parents {
		if reference(parent) != want[index] {
			return Row{}, ErrInvalidBoundary
		}
	}
	return Row{
		Boundary: reference(expected),
		Owner:    row.Owner,
		Form:     row.Form,
		Parents:  want,
	}, nil
}

// exactBoundaryParents is the complete, factorized LinkBoundary input
// contract. LinkBoundary has one virtual predicate over an accepted
// application and one Target-owned descriptor family; it does not manufacture
// a second relation per Target facet. Every listed parent is nevertheless an
// independently owned semantic source and must remain present.
func exactBoundaryParents(parents []semanticsource.Token) []Reference {
	specs := boundaryParentSpecs()
	result := make([]Reference, len(specs))
	for index, spec := range specs {
		result[index] = referenceFor(spec.origin, spec.facet, parents)
	}
	return result
}

type boundaryParentSpec struct {
	origin semanticsource.Origin
	facet  semanticsource.Facet
}

func boundaryParentSpecs() []boundaryParentSpec {
	return []boundaryParentSpec{
		{semanticsource.OriginTargetOperation, 0},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult},
		{semanticsource.OriginTargetProtocol, 0},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape},
		{semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder},
		{semanticsource.OriginTargetGsub, 0},
		{semanticsource.OriginLinkProjectBaseApplication, 0},
	}
}

// referenceFor finds an exact issued parent in the supplied canonical row.
// It refuses to synthesize a token, so a changed revision or absent parent
// cannot be silently accepted.
func referenceFor(origin semanticsource.Origin, facet semanticsource.Facet, parents []semanticsource.Token) Reference {
	for _, parent := range parents {
		if parent.Origin() == origin && parent.Facet() == facet {
			return reference(parent)
		}
	}
	return Reference{}
}

// Validate rechecks generated evidence independently of the relation schema.
// Zero references, duplicate parents, noncanonical order, or an incomplete
// parent vector fail closed.
func (e Evidence) Validate() error {
	if !canonicalDigest(e.SchemaDigest) || len(e.Rows) != 1 || !canonicalDigest(e.Digest) || e.Digest != digest(e) {
		return ErrInvalidBoundary
	}
	row := e.Rows[0]
	if row.Boundary.Origin != semanticsource.OriginLinkBoundary || row.Boundary.Facet != 0 || row.Boundary.Revision == 0 ||
		row.Owner != relations.OwnerLinkBoundary || row.Form != relations.FormVirtualPredicate || len(row.Parents) != 32 {
		return ErrInvalidBoundary
	}
	want := boundaryParentSpecs()
	for index, parent := range row.Parents {
		if parent.Origin == 0 || parent.Revision == 0 || parent.Origin != want[index].origin || parent.Facet != want[index].facet ||
			(index != 0 && !referenceLess(row.Parents[index-1], parent)) {
			return ErrInvalidBoundary
		}
	}
	return nil
}

func canonicalDigest(value string) bool {
	if len(value) != hex.EncodedLen(sha256.Size) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func digest(e Evidence) string {
	encoded, err := encodeCanonical(e)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func encodeCanonical(e Evidence) ([]byte, error) {
	var out bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&out, canonicalDomain, canonicalVersion); err != nil {
		return nil, err
	}
	if err := writer.String(e.SchemaDigest); err != nil {
		return nil, err
	}
	if err := writer.Count(uint64(len(e.Rows))); err != nil {
		return nil, err
	}
	for _, row := range e.Rows {
		if err := writer.Record(boundaryRecord); err != nil {
			return nil, err
		}
		if err := writeReference(&writer, row.Boundary); err != nil {
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
			if err := writeReference(&writer, parent); err != nil {
				return nil, err
			}
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), nil
}

func writeReference(writer *canonical.Writer, reference Reference) error {
	for _, value := range [...]uint64{
		uint64(reference.Origin), uint64(reference.Facet), uint64(reference.Revision),
	} {
		if err := writer.Uint(value); err != nil {
			return err
		}
	}
	return nil
}

func reference(token semanticsource.Token) Reference {
	return Reference{Origin: token.Origin(), Facet: token.Facet(), Revision: token.Revision()}
}

func less(left, right semanticsource.Token) bool {
	if left.Origin() != right.Origin() {
		return left.Origin() < right.Origin()
	}
	if left.Facet() != right.Facet() {
		return left.Facet() < right.Facet()
	}
	return left.Revision() < right.Revision()
}

func referenceLess(left, right Reference) bool {
	if left.Origin != right.Origin {
		return left.Origin < right.Origin
	}
	if left.Facet != right.Facet {
		return left.Facet < right.Facet
	}
	return left.Revision < right.Revision
}
