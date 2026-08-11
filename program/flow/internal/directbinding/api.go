package directbinding

import "github.com/wippyai/go-lua/program/keyspace"

// CallForm preserves authored call syntax. Plain and method calls may share
// an exact selector but have different argument-adjustment rules.
type CallForm uint8

const (
	CallFormPlain CallForm = iota + 1
	CallFormMethod
)

func (form CallForm) valid() bool {
	return form == CallFormPlain || form == CallFormMethod
}

// BindingSelections is the typed external exact-Read projection.
type BindingSelections struct{ result *Result }

// PublicationPaths is the typed exact Static publication projection.
type PublicationPaths struct{ result *Result }

// BindingPath is an immutable cursor over one external Read's exact suffix in
// leaf-to-root parent-chain order. It is deliberately typed to the
// BindingSelections projection rather than a generic path intermediate
// representation. Each Segment call advances one existing parent-chain row
// and returns a new value; it never mutates caller state or materializes the
// path.
type BindingPath struct {
	result    *Result
	current   uint32
	root      keyspace.Term
	remaining uint32
}

// PublicationPath is an immutable cursor over one Static publication's exact
// suffix in leaf-to-root parent-chain order. It shares Result's selector chain
// with BindingPath and carries no Static owner or path payload.
type PublicationPath struct {
	result    *Result
	current   uint32
	root      keyspace.Term
	remaining uint32
}

// DirectCalls is the typed exact direct-call projection.
type DirectCalls struct{ result *Result }
