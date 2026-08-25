package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Publish proposes rows from one child to a declared logical destination.
// Commit mechanics and publication storage are outside the logical node.
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
	return derive("analysis/relation/schema/algebra/publish/v1", append(parts, publish.contract.digestBytes()...))
}

func (publish Publish) expression() {}
