package outputowners

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Row binds one canonical Program output relation to its sole component
// owner. The relation identity is the schema EntryID; its key is recovered
// only by the generated renderer when a human-readable output is needed.
type Row struct {
	Relation schema.EntryID
	Owner    denominator.RelationOwner
}

// Evidence is the generated, canonical Program output-owner denominator.
type Evidence struct {
	Digest string
	Rows   []Row
}

// Generated is the checked-in immutable output-owner evidence value.
