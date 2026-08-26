package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Publish commits the exact sealed writable row layout of one relational
// child to a declared logical destination.  Its child may be Apply, Merge,
// ColumnProject, or another checked relational expression; Apply proposal
// authority, when present, remains attached to that child rather than being
// inferred from this node's syntax.  The contract's positional writable
// layout is the publication authority, while destination row/key authority
// remains in the mounted relation geometry.
type Publish struct {
	child    Expression
	contract PublishContract
}

// NewPublish constructs a publication expression without applying authority
// or destination checks.
func NewPublish(child Expression, contract PublishContract) Publish {
	return Publish{child: child, contract: contract}
}

// Child returns the published child expression.
func (publish Publish) Child() Expression { return publish.child }

// Contract returns the immutable publication contract.
func (publish Publish) Contract() PublishContract { return publish.contract }

// Kind implements Expression.
func (publish Publish) Kind() Kind { return KindPublish }

// Digest returns the deterministic structural identity.
func (publish Publish) Digest() identity.ContentID {
	parts := appendExpr(nil, publish.child)
	return derive("analysis/relation/schema/algebra/publish/v3", append(parts, publish.contract.digestBytes()...))
}

func (publish Publish) expression() {}
