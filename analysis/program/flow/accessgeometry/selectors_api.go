package accessgeometry

import "github.com/wippyai/go-lua/analysis/program/keyspace"

const (
	selectorCallPlain uint8 = iota + 1
	selectorCallMethod
)

// ExactReads is the typed external exact-Read selector column.
type ExactReads struct{ result *Result }

// TypePublications is the typed exact Static publication path column.
type TypePublications struct{ result *Result }

// ExactReadPath is an immutable cursor over one external Read's exact suffix
// in leaf-to-root parent-chain order. It is deliberately typed to the exact
// Read projection rather than a generic path intermediate representation.
// Each Segment call advances one existing parent-chain row and returns a new
// value; it never mutates caller state or materializes the path.
type ExactReadPath struct {
	result    *Result
	current   uint32
	root      keyspace.Term
	remaining uint32
}

// PublicationPath is an immutable cursor over one Static publication's exact
// suffix in leaf-to-root parent-chain order. It shares Result's selector chain
// with ExactReadPath and carries no Static owner or path payload.
type PublicationPath struct {
	result    *Result
	current   uint32
	root      keyspace.Term
	remaining uint32
}

// DirectCalls is the typed exact direct-call column.
type DirectCalls struct{ result *Result }
