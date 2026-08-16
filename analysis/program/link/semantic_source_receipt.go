package link

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SemanticSourceReceipt is Link's detached aggregate denominator. It retains
// only the exact Link owner identity and the already-sealed schema-indexed
// cardinality result; no child authority, Program, schema builder, or row
// relation crosses this boundary.
type SemanticSourceReceipt struct {
	owner        identity.ContentID
	publications semanticsource.Publications
}

func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.publications.SchemaDigest().Available() && receipt.publications.Count() > 0
}

func (receipt SemanticSourceReceipt) OwnerID() identity.ContentID { return receipt.owner }

func (receipt SemanticSourceReceipt) Publications() (semanticsource.Publications, bool) {
	if !receipt.Valid() {
		return semanticsource.Publications{}, false
	}
	return receipt.publications.Clone(), true
}

func buildSemanticSourceReceipt(link *Link) (SemanticSourceReceipt, error) {
	if link == nil || !link.sealedSemanticSource() {
		return SemanticSourceReceipt{}, errors.New("link: unavailable semantic-source owner")
	}
	publications, err := buildSemanticSourcePublications(link)
	if err != nil || publications.Count() == 0 {
		return SemanticSourceReceipt{}, errSemanticSourceAssemblySchema
	}
	receipt := SemanticSourceReceipt{owner: link.id, publications: publications}
	if !receipt.Valid() {
		return SemanticSourceReceipt{}, errSemanticSourceAssemblySchema
	}
	return receipt, nil
}

// SemanticSourceReceipt returns the one seal-time aggregate. It performs no
// schema assembly, child traversal, ContentID derivation, or typed-row query.
func (link *Link) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if link == nil || !link.semanticReceipt.Valid() || link.semanticReceipt.OwnerID() != link.id {
		return SemanticSourceReceipt{}, false
	}
	return link.semanticReceipt, true
}

// SourcePublications is the sole production source denominator entrypoint.
// It returns the detached seal-time aggregate; no child traversal or schema
// assembly occurs on this query path.
func (link *Link) SourcePublications() (semanticsource.Publications, error) {
	receipt, ok := link.SemanticSourceReceipt()
	if !ok {
		return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
	}
	publications, ok := receipt.Publications()
	if !ok {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	return publications, nil
}
