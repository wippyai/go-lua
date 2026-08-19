package static

import (
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func (component *Component) View() View { return View{component: component} }

// Operators returns the sealed operator owner view. Static contributes only
// the enclosing publication lifetime; row storage and query semantics belong
// to the operators package.
func (view View) Operators() staticoperators.View {
	component := view.componentOf()
	if component == nil {
		return staticoperators.View{}
	}
	return component.operators.View()
}

// Operands returns the sealed operand owner view. Static contributes the
// enclosing publication lifetime and the sealed census column the annotation
// target admission reads; row storage and query semantics belong to the
// operands package.
func (view View) Operands() staticoperands.View {
	component := view.componentOf()
	if component == nil {
		return staticoperands.View{}
	}
	return component.operands.NewView(component.census)
}

// Contracts returns the sealed contract owner view. Static contributes only
// the enclosing publication lifetime; row storage and query semantics belong
// to the contracts package.
func (view View) Contracts() staticcontracts.View {
	component := view.componentOf()
	if component == nil {
		return staticcontracts.View{}
	}
	return component.contracts.View()
}

// Signatures returns the sealed signature owner view. Static contributes only
// the enclosing publication lifetime; row storage and query semantics belong
// to the signatures package.
func (view View) Signatures() staticsig.View {
	component := view.componentOf()
	if component == nil {
		return staticsig.View{}
	}
	return component.signatures.View()
}

// References returns the sealed TypeRef owner view. Static contributes only
// the enclosing publication lifetime; row storage and query semantics belong
// to the references package.
func (view View) References() staticrefs.View {
	component := view.componentOf()
	if component == nil {
		return staticrefs.View{}
	}
	return component.references.View()
}

// Publications returns the sealed publication owner view.
func (view View) Publications() staticpubs.View {
	component := view.componentOf()
	if component == nil {
		return staticpubs.View{}
	}
	return component.publications.View()
}

// Types returns the sealed type-forest owner view. Static contributes only
// the enclosing publication lifetime; row storage and query semantics belong
// to the types package. Resolving the component here is the one fence check:
// a cursor derived from an expired construction View is empty.
func (view View) Types() statictypes.View {
	component := view.componentOf()
	if component == nil {
		return statictypes.View{}
	}
	return component.types.View()
}

// Declarations returns the sealed declaration owner view. Static contributes
// only the enclosing publication lifetime; row storage and query semantics
// belong to the declarations package.
func (view View) Declarations() staticdecl.View {
	component := view.componentOf()
	if component == nil {
		return staticdecl.View{}
	}
	return component.declarations.View()
}
