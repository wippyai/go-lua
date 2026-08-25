package algebra

import "github.com/wippyai/go-lua/analysis/identity"

// Project renames or computes schema-defined columns from one child. Any
// semantic computation is named by a separate sealed contract; no function or
// physical projection is stored here.
type Project struct {
	child    Expression
	contract ProjectContract
}

// NewProject constructs a Project expression without applying checker rules.
func NewProject(child Expression, contract ProjectContract) Project {
	return Project{child: child, contract: contract}
}

// Child returns the projected child expression.
func (project Project) Child() Expression { return project.child }

// Contract returns the immutable projection contract.
func (project Project) Contract() ProjectContract { return project.contract }

// Kind implements Expression.
func (project Project) Kind() Kind { return KindProject }

// Digest returns the deterministic structural identity.
func (project Project) Digest() identity.ContentID {
	parts := appendExpr(nil, project.child)
	return derive("analysis/relation/schema/algebra/project/v1", append(parts, project.contract.digestBytes()...))
}

func (project Project) expression() {}
