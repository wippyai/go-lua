// Package targetingress generates the complete Target relation ingress
// denominator from the canonical Program/Target/Link relation schema.
//
//go:generate go run ./cmd/generate -out evidence_gen.go
package targetingress

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Row is one complete Target relation ingress declaration. Ingress is the
// canonical ordered parent vector: each parent is an exact use-context for
// this Target relation, not an inferred edge or a default policy.
type Row struct {
	Relation schema.EntryID
	Owner    denominator.RelationOwner
	Form     denominator.RelationForm
	Ingress  []schema.EntryID
}

// Evidence is generated cold proof material for the complete Target relation
// vocabulary and every exact ingress use-context.
type Evidence struct {
	Digest string
	Rows   []Row
}

// Generated is assigned by the checked-in generated evidence source.
var Generated Evidence
