// Package imports owns authored require/import occurrences.
//
// Module is intentionally independent of Source, Flow, Static, Link, and the
// root assembler.  It owns only canonical Program handles.  Key resolution
// and the chunk-entry surface are derived at the finalization boundary;
// authored Request participates in the Module identity.
package imports

import (
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Import is one dense module occurrence. Term, Call, Alias, and Request are
// authored. Key is filled by the Module finalizer and is deliberately excluded
// from the authored ContentID.
type Import struct {
	Term    keyspace.Term
	Call    keyspace.Term
	Alias   keyspace.Term
	Request keyspace.Term
	Key     keyspace.Key
}

// Input is the complete authored Module relation. Imports must be in their
// final dense Import-term order. A zero Alias means that the occurrence has no
// direct local binding. Request is the canonical authored String term and Key
// must be zero in authored input.
type Input struct {
	Imports []Import
}

// Resolution carries the authored Request witness and its derived Source Key
// in the same dense order as Input.Imports. Commit requires Request to match
// the authored Import row and never overwrites that row's Request.
type Resolution struct {
	Request keyspace.Term
	Key     keyspace.Key
}

// CommitInput is the complete derived Module projection. It is validated by
// this package and copied into immutable Component storage on success.
type CommitInput struct {
	Resolutions []Resolution
	Entry       Entry
}

// Draft is a one-shot construction capability. Copies share lifecycle state;
// only one copy can claim the owner Finalizer.
type Draft struct{ state *draftState }

// Finalizer is the owner-defined Module publication capability. It exposes
// the authored View while construction is active, then permits exactly one
// terminal Commit or Abort.
type Finalizer struct{ state *draftState }

// Component is immutable Module storage after a successful Commit.
type Component struct {
	imports []Import
	entry   entryData
	content identity.ContentID
}

// View is the compact direct Module capability. It can observe authored rows
// through an active Finalizer or committed rows through Component.View().
type View struct {
	state     *draftState
	component *Component
}

// Cold is an identity-only Module snapshot. It retains no Component pointer.
type Cold struct{ content identity.ContentID }

type authored struct {
	imports []Import
	content identity.ContentID
}

type draftState struct {
	mu       sync.Mutex
	authored *authored
	claimed  bool
	terminal bool
}

// Build validates and copies the complete authored Module relation. It does
// not create a final Component and accepts no derived resolution or Entry.
func Build(input Input) (*Draft, error) {
	imports := append([]Import(nil), input.Imports...)
	if !validAuthored(imports) {
		return nil, errors.New("program/imports: invalid authored imports")
	}
	content := authoredContent(imports)
	if !content.Available() {
		return nil, errors.New("program/imports: unavailable authored identity")
	}
	return &Draft{state: &draftState{
		authored: &authored{imports: imports, content: content},
	}}, nil
}

func validAuthored(imports []Import) bool {
	if !validArtifactImports(imports) {
		return false
	}
	for _, row := range imports {
		if row.Key != 0 {
			return false
		}
	}
	return true
}

func validFamilyTerm(term keyspace.Term, family keyspace.Family) bool {
	return term != 0 && keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0
}

func (d *Draft) active() bool {
	return d != nil && d.state != nil
}
