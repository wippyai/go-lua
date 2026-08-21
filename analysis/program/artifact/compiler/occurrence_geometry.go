package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// occurrenceLookup and occurrenceSpanGeometry are compiler-only geometry. The
// lookup keys the authored span proof while it is live; neither type crosses
// the publication boundary or becomes a Program index.
type occurrenceLookup struct {
	kind programschema.OccurrenceKind
	id   identity.ContentID
}

type occurrenceSpanGeometry struct {
	entry  []identity.ContentID
	finish []identity.ContentID
	route  identity.ContentID
}
