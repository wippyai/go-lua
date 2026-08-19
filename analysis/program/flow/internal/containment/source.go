package containment

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Source owns lexical provenance, not expression containment.  Only the
// closed direct-source families below are allowed to become Term -> Body
// edges.  Literals, values, lenses, and operators are merely checked against
// their Source owner; their structural parent is emitted by the owner that
// actually consumes them.
func isDirectSourceFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall,
		keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyReturn,
		keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyLabel,
		keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface,
		keyspace.FamilyControlFault:
		return true
	default:
		return false
	}
}

func validateSourceCounts(preimage source.Preimage, counts [keyspace.FamilyCount]uint32) error {
	identity := preimage.Identity()
	literals := preimage.Literals()
	if literals.Nils().Count() != int(counts[keyspace.FamilyNil]) ||
		literals.Bools().Count() != int(counts[keyspace.FamilyBool]) ||
		literals.Integers().Count() != int(counts[keyspace.FamilyInteger]) ||
		literals.Floats().Count() != int(counts[keyspace.FamilyFloat]) ||
		literals.Strings().Count() != int(counts[keyspace.FamilyString]) ||
		preimage.Keys().Count() != int(counts[keyspace.FamilyKey]) ||
		preimage.Faults().Count() != int(counts[keyspace.FamilyControlFault]) {
		return errors.New("program/flow/containment: Source facet cardinality mismatch")
	}
	if identity.FamilyCount(keyspace.FamilyOutcome) != 0 {
		return errors.New("program/flow/containment: Source Outcome cardinality is nonzero")
	}
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString,
	} {
		if err := validateSourceLiteralTerms(preimage, family, counts); err != nil {
			return err
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyKey]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyKey, ordinal)
		owner, _, _, ok := preimage.Keys().Name(term)
		if !ok {
			owner, _, _, ok = preimage.Keys().List(term)
		}
		if !ok || !validTerm(owner, counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/containment: malformed Source key owner")
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyControlFault]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyControlFault, ordinal)
		row, ok := preimage.Faults().At(term)
		if !ok || !validTerm(row.Owner, counts) || keyspace.TermFamily(row.Owner) != keyspace.FamilyBody {
			return errors.New("program/flow/containment: malformed Source fault owner")
		}
	}
	return nil
}

func validateSourceLiteralTerms(preimage source.Preimage, family keyspace.Family, counts [keyspace.FamilyCount]uint32) error {
	var count uint32
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString:
		count = counts[family]
	default:
		return errors.New("program/flow/containment: invalid Source literal family")
	}
	for ordinal := uint32(1); ordinal <= count; ordinal++ {
		term := keyspace.MakeTerm(family, ordinal)
		var got, owner keyspace.Term
		var ok bool
		switch family {
		case keyspace.FamilyNil:
			got, owner, ok = preimage.Literals().Nils().At(int(ordinal - 1))
		case keyspace.FamilyBool:
			got, owner, _, ok = preimage.Literals().Bools().At(int(ordinal - 1))
		case keyspace.FamilyInteger:
			got, owner, _, ok = preimage.Literals().Integers().At(int(ordinal - 1))
		case keyspace.FamilyFloat:
			got, owner, _, ok = preimage.Literals().Floats().At(int(ordinal - 1))
		case keyspace.FamilyString:
			got, owner, _, ok = preimage.Literals().Strings().At(int(ordinal - 1))
		}
		if !ok || got != term || !validTerm(owner, counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/containment: malformed Source literal ordinal or owner")
		}
	}
	return nil
}

func emitSource(
	preimage source.Preimage,
	staticView staticquery.View,
	view authored.View,
	bodies *body.Result,
	counts [keyspace.FamilyCount]uint32,
	result *emission,
) error {
	if result == nil {
		return errors.New("program/flow/containment: nil Source emission")
	}
	order := preimage.Order()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBody]; ordinal++ {
		ownerBody := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		length, ok := order.BodyLen(ownerBody)
		if !ok || length < 0 {
			return errors.New("program/flow/containment: Source Body order unavailable")
		}
		for index := 0; index < length; index++ {
			term, ok := order.BodyAt(ownerBody, index)
			if !ok || !validTerm(term, counts) {
				return errors.New("program/flow/containment: malformed direct Source term")
			}
			family := keyspace.TermFamily(term)
			if family == keyspace.FamilyBody {
				if term == ownerBody {
					return errors.New("program/flow/containment: Body contains itself")
				}
				parent, hasParent := bodies.Parent(term)
				if !hasParent || parent != ownerBody {
					return errors.New("program/flow/containment: Source Body order disagrees with lexical Body parent")
				}
				continue
			}
			if !isDirectSourceFamily(family) {
				return errors.New("program/flow/containment: non-direct term in Source Body order")
			}
			actualOwner, ok := directOwner(preimage, staticView, view, term, counts)
			if !ok || actualOwner != ownerBody {
				return errors.New("program/flow/containment: direct Source owner mismatch")
			}
			result.edges = append(result.edges, kernelEdge{child: term, parent: ownerBody})
		}
	}
	return nil
}

func directOwner(
	preimage source.Preimage,
	staticView staticquery.View,
	view authored.View,
	term keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
) (keyspace.Term, bool) {
	if !validTerm(term, counts) {
		return 0, false
	}
	if owner, ok := authoredTermOwner(view, term, counts); ok {
		return owner, true
	}
	if owner, ok := staticTermOwner(staticView, term, counts); ok {
		return owner, true
	}
	if keyspace.TermFamily(term) == keyspace.FamilyControlFault {
		row, ok := preimage.Faults().At(term)
		return row.Owner, ok
	}
	return 0, false
}

func authoredTermOwner(view authored.View, term keyspace.Term, counts [keyspace.FamilyCount]uint32) (keyspace.Term, bool) {
	if !validTerm(term, counts) {
		return 0, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyValues:
		owner, _, ok := view.Values().Get(term)
		return owner, ok
	case keyspace.FamilyLensExact:
		owner, _, _, _, ok := view.Access().Exact().Get(term)
		return owner, ok
	case keyspace.FamilyLensKey:
		owner, _, _, ok := view.Access().Dynamic().Get(term)
		return owner, ok
	case keyspace.FamilyBind:
		owner, _, ok := view.Storage().Binds().Get(term)
		return owner, ok
	case keyspace.FamilyAssign:
		owner, _, ok := view.Storage().Assigns().Get(term)
		return owner, ok
	case keyspace.FamilyWrite:
		assign, _, ok := view.Storage().Writes().Get(term)
		if !ok {
			return 0, false
		}
		owner, _, ok := view.Storage().Assigns().Get(assign)
		return owner, ok
	case keyspace.FamilyRead:
		owner, _, _, ok := view.Storage().Reads().Get(term)
		return owner, ok
	case keyspace.FamilyVararg:
		owner, _, ok := view.Storage().Varargs().Get(term)
		return owner, ok
	case keyspace.FamilyCall:
		owner, _, _, _, ok := view.Calls().Get(term)
		return owner, ok
	case keyspace.FamilyBranch:
		owner, _, _, _, ok := view.Control().Branches().Get(term)
		return owner, ok
	case keyspace.FamilyLoop:
		owner, _, _, _, ok := view.Control().Loops().Get(term)
		return owner, ok
	case keyspace.FamilyTable:
		owner, ok := view.Tables().Get(term)
		return owner, ok
	case keyspace.FamilyTableField:
		table, _, _, _, ok := view.Fields().Get(term)
		if !ok {
			return 0, false
		}
		owner, ok := view.Tables().Get(table)
		return owner, ok
	case keyspace.FamilyUnary:
		owner, _, _, ok := view.Operators().Unaries().Get(term)
		return owner, ok
	case keyspace.FamilyBinary:
		owner, _, _, _, ok := view.Operators().Binaries().Get(term)
		return owner, ok
	case keyspace.FamilySelect:
		owner, _, _, _, ok := view.Operators().Selects().Get(term)
		return owner, ok
	case keyspace.FamilyValueClaim:
		owner, _, _, ok := view.Claims().Get(term)
		return owner, ok
	case keyspace.FamilyTypeValue:
		owner, ok := view.TypeValues().Get(term)
		return owner, ok
	case keyspace.FamilyCell:
		_, owner, _, ok := view.Storage().Cells().Get(term)
		return owner, ok && owner != 0
	case keyspace.FamilyReturn:
		owner, _, ok := view.Control().Returns().Get(term)
		return owner, ok
	case keyspace.FamilyBreak:
		owner, _, ok := view.Control().Breaks().Get(term)
		return owner, ok
	case keyspace.FamilyGoto:
		owner, _, ok := view.Control().Gotos().Get(term)
		return owner, ok
	case keyspace.FamilyLabel:
		owner, ok := view.Control().Labels().Get(term)
		return owner, ok
	default:
		return 0, false
	}
}

func staticTermOwner(view staticquery.View, term keyspace.Term, counts [keyspace.FamilyCount]uint32) (keyspace.Term, bool) {
	if !validTerm(term, counts) {
		return 0, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyTypeAlias:
		owner, _, _, _, ok := view.Declarations().Aliases().Get(term)
		return owner, ok
	case keyspace.FamilyTypeInterface:
		owner, _, _, ok := view.Declarations().Interfaces().Get(term)
		return owner, ok
	default:
		return 0, false
	}
}
