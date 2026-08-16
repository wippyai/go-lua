package targetingress

import (
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

// Reference is an issued canonical relation identity. It deliberately keeps
// the typed semantic-source coordinates rather than reconstructing a relation
// from source spelling or a Target implementation handle.
type Reference struct {
	Origin   semanticsource.Origin
	Facet    semanticsource.Facet
	Revision semanticsource.Revision
}

// Row is one complete Target relation ingress declaration. Ingress is the
// canonical ordered parent vector: each parent is an exact use-context for
// this Target relation, not an inferred edge or a default policy.
type Row struct {
	Relation Reference
	Owner    relations.Owner
	Form     relations.Form
	Ingress  []Reference
}

// Evidence is generated cold proof material for the complete Target relation
// vocabulary and every exact ingress use-context.
type Evidence struct {
	SchemaDigest string
	Digest       string
	Rows         []Row
}

// Generated is assigned by the checked-in generated evidence source.
var Generated Evidence
