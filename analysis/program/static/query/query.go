package query

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// ContentID returns the authored Static identity when this view is available.
func (view View) ContentID() identity.ContentID {
	if !view.available() {
		return identity.ContentID{}
	}
	return view.snapshot.contentID
}

// Operators returns the sealed operator owner view. The query vertical
// contributes only composition and the enclosing publication fence; operator
// rows and their semantics remain owned by operators.
func (view View) Operators() staticoperators.View {
	if !view.available() {
		return staticoperators.View{}
	}
	return staticoperators.NewView(&view.snapshot.operators)
}

// Operands returns the sealed operand owner view under the canonical census
// column used by annotation-target admission.
func (view View) Operands() staticoperands.View {
	if !view.available() {
		return staticoperands.View{}
	}
	return view.snapshot.operands.NewView(view.snapshot.census)
}

// Contracts returns the sealed contract owner view.
func (view View) Contracts() staticcontracts.View {
	if !view.available() {
		return staticcontracts.View{}
	}
	return view.snapshot.contracts.View()
}

// Signatures returns the sealed signature owner view.
func (view View) Signatures() staticsig.View {
	if !view.available() {
		return staticsig.View{}
	}
	return view.snapshot.signatures.View()
}

// References returns the sealed TypeRef owner view.
func (view View) References() staticrefs.View {
	if !view.available() {
		return staticrefs.View{}
	}
	return view.snapshot.references.View()
}

// Publications returns the sealed publication owner view.
func (view View) Publications() staticpubs.View {
	if !view.available() {
		return staticpubs.View{}
	}
	return view.snapshot.publications.View()
}

// Types returns the sealed type-forest owner view.
func (view View) Types() statictypes.View {
	if !view.available() {
		return statictypes.View{}
	}
	return view.snapshot.types.View()
}

// Declarations returns the sealed declaration owner view.
func (view View) Declarations() staticdecl.View {
	if !view.available() {
		return staticdecl.View{}
	}
	return view.snapshot.declarations.View()
}
