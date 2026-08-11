package containment

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// flowLexicalOwner resolves only the closed set of terms which can occur at
// an ordinary expression edge.  It is not a general owner registry: cells,
// static sidecars, control targets, and construct bodies deliberately have no
// answer here.  Source-owned literals and keys are resolved from Preimage;
// authored rows are resolved from their owning typed view.
func flowLexicalOwner(
	preimage source.Preimage,
	view authored.View,
	term keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
) (keyspace.Term, bool) {
	if !validTerm(term, counts) {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		_, owner, ok := preimage.Literals().Nils().At(int(ordinal - 1))
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyBool:
		_, owner, _, ok := preimage.Literals().Bools().At(int(ordinal - 1))
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyInteger:
		_, owner, _, ok := preimage.Literals().Integers().At(int(ordinal - 1))
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyFloat:
		_, owner, _, ok := preimage.Literals().Floats().At(int(ordinal - 1))
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyString:
		_, owner, _, ok := preimage.Literals().Strings().At(int(ordinal - 1))
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyKey:
		if owner, _, _, ok := preimage.Keys().Name(term); ok {
			return owner, bodyTerm(owner, counts)
		}
		owner, _, _, ok := preimage.Keys().List(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyValues:
		owner, _, ok := view.Values().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyLensExact:
		owner, _, _, _, ok := view.Access().Exact().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyLensKey:
		owner, _, _, ok := view.Access().Dynamic().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyRead:
		owner, _, _, ok := view.Storage().Reads().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyVararg:
		owner, _, ok := view.Storage().Varargs().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyUnary:
		owner, _, _, ok := view.Operators().Unaries().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyBinary:
		owner, _, _, _, ok := view.Operators().Binaries().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilySelect:
		owner, _, _, _, ok := view.Operators().Selects().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyBind:
		owner, _, ok := view.Storage().Binds().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyAssign:
		owner, _, ok := view.Storage().Assigns().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyFunction:
		owner, _, _, ok := view.Functions().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyCall:
		owner, _, _, _, ok := view.Calls().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyReturn:
		owner, _, ok := view.Control().Returns().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyLoop:
		owner, body, loopKind, _, ok := view.Control().Loops().Get(term)
		if !ok || !bodyTerm(owner, counts) || !bodyTerm(body, counts) {
			return 0, false
		}
		// Repeat's condition is evaluated after its lexical Body, unlike
		// While and the two for forms whose controls run at the loop owner
		// frontier. The Loop edge therefore carries the repeat Body's
		// expression owner even though the Loop row itself is declared by
		// the enclosing owner.
		if loopKind == kind.LoopRepeat {
			return body, true
		}
		return owner, true
	case keyspace.FamilyBranch:
		owner, _, _, _, ok := view.Control().Branches().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyTable:
		owner, ok := view.Tables().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyTableField:
		table, _, _, _, ok := view.Fields().Get(term)
		if !ok {
			return 0, false
		}
		owner, tableOK := view.Tables().Get(table)
		return owner, tableOK && bodyTerm(owner, counts)
	case keyspace.FamilyWrite:
		assign, _, ok := view.Storage().Writes().Get(term)
		if !ok {
			return 0, false
		}
		owner, _, assignOK := view.Storage().Assigns().Get(assign)
		return owner, assignOK && bodyTerm(owner, counts)
	case keyspace.FamilyValueClaim:
		owner, _, _, ok := view.Claims().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	case keyspace.FamilyTypeValue:
		owner, ok := view.TypeValues().Get(term)
		return owner, ok && bodyTerm(owner, counts)
	default:
		return 0, false
	}
}

func validateFlowEdgeOwners(
	preimage source.Preimage,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	edges []kernelEdge,
) error {
	for _, edge := range edges {
		childOwner, childOK := flowLexicalOwner(preimage, view, edge.child, counts)
		parentOwner, parentOK := flowLexicalOwner(preimage, view, edge.parent, counts)
		if !childOK || !parentOK || childOwner != parentOwner {
			return errors.New("program/flow/containment: expression edge crosses lexical owner")
		}
	}
	return nil
}
