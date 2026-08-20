package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// initSeen allocates the one dense occurrence plane for each authored
// composite family. The planes belong to the traversal session only; they are
// not part of the immutable evaluation-port result and do not encode a
// second containment relation.
func (walker *Session) initSeen() error {
	if walker == nil || !walker.view.ContentID().Available() {
		return errors.New("program/flow/evaluation: authored view is unavailable")
	}
	for _, family := range evaluationCompositeFamilies {
		count := walker.familyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return errors.New("program/flow/evaluation: invalid evaluation family cardinality")
		}
		if count != 0 {
			walker.seen[family] = make([]bool, count+1)
		}
	}
	return nil
}

var evaluationCompositeFamilies = [...]keyspace.Family{
	keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
	keyspace.FamilyRead, keyspace.FamilyUnary, keyspace.FamilyBinary,
	keyspace.FamilySelect, keyspace.FamilyBind, keyspace.FamilyAssign,
	keyspace.FamilyCall, keyspace.FamilyReturn,
	keyspace.FamilyTable, keyspace.FamilyTableField, keyspace.FamilyValueClaim,
	keyspace.FamilyWrite, keyspace.FamilyBranch, keyspace.FamilyLoop,
}

func isEvaluationComposite(family keyspace.Family) bool {
	for _, candidate := range evaluationCompositeFamilies {
		if candidate == family {
			return true
		}
	}
	return false
}

// familyCount returns only authored counts. Source-owned literal, key, and
// Body handles are validated by their typed family and nonzero ordinal because
// an authored View deliberately does not retain the Source denominator.
func (walker *Session) familyCount(family keyspace.Family) int {
	if walker == nil {
		return -1
	}
	switch family {
	case keyspace.FamilyValues:
		return walker.view.Values().Count()
	case keyspace.FamilyLensExact:
		return walker.view.Access().Exact().Count()
	case keyspace.FamilyLensKey:
		return walker.view.Access().Dynamic().Count()
	case keyspace.FamilyCell:
		return walker.view.Storage().Cells().Count()
	case keyspace.FamilyRead:
		return walker.view.Storage().Reads().Count()
	case keyspace.FamilyVararg:
		return walker.view.Storage().Varargs().Count()
	case keyspace.FamilyBind:
		return walker.view.Storage().Binds().Count()
	case keyspace.FamilyAssign:
		return walker.view.Storage().Assigns().Count()
	case keyspace.FamilyWrite:
		return walker.view.Storage().Writes().Count()
	case keyspace.FamilyTable:
		return walker.view.Tables().Count()
	case keyspace.FamilyTableField:
		return walker.view.Fields().Count()
	case keyspace.FamilyUnary:
		return walker.view.Operators().Unaries().Count()
	case keyspace.FamilyBinary:
		return walker.view.Operators().Binaries().Count()
	case keyspace.FamilySelect:
		return walker.view.Operators().Selects().Count()
	case keyspace.FamilyFunction:
		return walker.view.Functions().Count()
	case keyspace.FamilyCall:
		return walker.view.Calls().Count()
	case keyspace.FamilyReturn:
		return walker.view.Control().Returns().Count()
	case keyspace.FamilyBreak:
		return walker.view.Control().Breaks().Count()
	case keyspace.FamilyLabel:
		return walker.view.Control().Labels().Count()
	case keyspace.FamilyGoto:
		return walker.view.Control().Gotos().Count()
	case keyspace.FamilyBranch:
		return walker.view.Control().Branches().Count()
	case keyspace.FamilyLoop:
		return walker.view.Control().Loops().Count()
	case keyspace.FamilyValueClaim:
		return walker.view.Claims().Count()
	case keyspace.FamilyTypeValue:
		return walker.view.TypeValues().Count()
	default:
		return -1
	}
}

// validTerm validates the terms that can occur on the evaluation traversal.
// Source-owned scalar handles have no authored upper-bound plane by design;
// their Source admission is checked at the whole-Flow boundary. Every
// authored family, in contrast, is checked against its committed dense view.
func (walker *Session) validTerm(term keyspace.Term) bool {
	if walker == nil {
		return false
	}
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family == keyspace.FamilyInvalid || ordinal == 0 {
		return false
	}
	switch family {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyKey,
		keyspace.FamilyBody:
		return true
	case keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
		keyspace.FamilyCell, keyspace.FamilyRead, keyspace.FamilyVararg,
		keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyWrite,
		keyspace.FamilyTable, keyspace.FamilyTableField, keyspace.FamilyUnary,
		keyspace.FamilyBinary, keyspace.FamilySelect, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyReturn, keyspace.FamilyBreak,
		keyspace.FamilyLabel, keyspace.FamilyGoto, keyspace.FamilyBranch,
		keyspace.FamilyLoop, keyspace.FamilyValueClaim, keyspace.FamilyTypeValue:
		count := walker.familyCount(family)
		return count >= 0 && uint64(ordinal) <= uint64(count)
	default:
		return false
	}
}

// owner returns the authored lexical Body for one evaluation row. Scalar
// Source handles and global Cells intentionally have no authored owner in this
// view and return (0,false,nil). No owner table is built or retained.
func (walker *Session) owner(term keyspace.Term) (keyspace.Term, bool, error) {
	if !walker.validTerm(term) {
		return 0, false, errors.New("program/flow/evaluation: term is unavailable")
	}
	var owner keyspace.Term
	var ok bool
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyValues:
		owner, _, ok = walker.view.Values().Get(term)
	case keyspace.FamilyLensExact:
		owner, _, _, _, ok = walker.view.Access().Exact().Get(term)
	case keyspace.FamilyLensKey:
		owner, _, _, ok = walker.view.Access().Dynamic().Get(term)
	case keyspace.FamilyCell:
		cellKind, body, _, cellOK := walker.view.Storage().Cells().Get(term)
		if !cellOK {
			return 0, false, errors.New("program/flow/evaluation: Cell row is unavailable")
		}
		if cellKind == authored.CellGlobal {
			return 0, false, nil
		}
		owner, ok = body, true
	case keyspace.FamilyRead:
		owner, _, _, ok = walker.view.Storage().Reads().Get(term)
	case keyspace.FamilyVararg:
		owner, _, ok = walker.view.Storage().Varargs().Get(term)
	case keyspace.FamilyBind:
		owner, _, ok = walker.view.Storage().Binds().Get(term)
	case keyspace.FamilyAssign:
		owner, _, ok = walker.view.Storage().Assigns().Get(term)
	case keyspace.FamilyWrite:
		assign, _, writeOK := walker.view.Storage().Writes().Get(term)
		if !writeOK {
			return 0, false, errors.New("program/flow/evaluation: Write row is unavailable")
		}
		owner, _, ok = walker.view.Storage().Assigns().Get(assign)
	case keyspace.FamilyTable:
		owner, ok = walker.view.Tables().Get(term)
	case keyspace.FamilyTableField:
		table, _, _, _, fieldOK := walker.view.Fields().Get(term)
		if !fieldOK {
			return 0, false, errors.New("program/flow/evaluation: TableField row is unavailable")
		}
		owner, ok = walker.view.Tables().Get(table)
	case keyspace.FamilyUnary:
		owner, _, _, ok = walker.view.Operators().Unaries().Get(term)
	case keyspace.FamilyBinary:
		owner, _, _, _, ok = walker.view.Operators().Binaries().Get(term)
	case keyspace.FamilySelect:
		owner, _, _, _, ok = walker.view.Operators().Selects().Get(term)
	case keyspace.FamilyFunction:
		owner, _, _, ok = walker.view.Functions().Get(term)
	case keyspace.FamilyCall:
		owner, _, _, _, ok = walker.view.Calls().Get(term)
	case keyspace.FamilyReturn:
		owner, _, ok = walker.view.Control().Returns().Get(term)
	case keyspace.FamilyBreak:
		owner, _, ok = walker.view.Control().Breaks().Get(term)
	case keyspace.FamilyLabel:
		owner, ok = walker.view.Control().Labels().Get(term)
	case keyspace.FamilyGoto:
		owner, _, ok = walker.view.Control().Gotos().Get(term)
	case keyspace.FamilyBranch:
		owner, _, _, _, ok = walker.view.Control().Branches().Get(term)
	case keyspace.FamilyLoop:
		owner, _, _, _, ok = walker.view.Control().Loops().Get(term)
	case keyspace.FamilyValueClaim:
		owner, _, _, ok = walker.view.Claims().Get(term)
	case keyspace.FamilyTypeValue:
		owner, ok = walker.view.TypeValues().Get(term)
	default:
		return 0, false, nil
	}
	if !ok {
		return 0, false, errors.New("program/flow/evaluation: authored row is unavailable")
	}
	if keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return 0, false, errors.New("program/flow/evaluation: authored row has invalid Body owner")
	}
	return owner, true, nil
}

func (walker *Session) rootAllowed(term keyspace.Term) bool {
	if !walker.validTerm(term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyValues,
		keyspace.FamilyLensExact, keyspace.FamilyLensKey, keyspace.FamilyRead,
		keyspace.FamilyVararg, keyspace.FamilyUnary, keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyFunction, keyspace.FamilyCall, keyspace.FamilyReturn,
		keyspace.FamilyTable, keyspace.FamilyTypeValue, keyspace.FamilyValueClaim,
		keyspace.FamilyTableField:
		return true
	default:
		return false
	}
}

// pushWithPrefix is the one frame-ingress path used by both Select event
// enumeration and the pending-prefix projection. Prefix is seal scratch and
// never crosses the Session boundary.
func (walker *Session) pushWithPrefix(term, expectedOwner keyspace.Term, prefix uint32) error {
	if !walker.validTerm(term) {
		return errors.New("program/flow/evaluation: child term is unavailable")
	}
	actualOwner, hasOwner, err := walker.owner(term)
	if err != nil {
		return err
	}
	if hasOwner {
		if expectedOwner != 0 && actualOwner != expectedOwner {
			return errors.New("program/flow/evaluation: expression crosses Body owner")
		}
		expectedOwner = actualOwner
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if isEvaluationComposite(family) {
		plane := walker.seen[family]
		if uint64(ordinal) >= uint64(len(plane)) || plane[ordinal] {
			if walker.pending != nil && walker.pending.discover {
				// Discovery seeds every composite family once and records
				// direct edges while each seed is processed. A child may
				// therefore already have a seed frame; its edge is still
				// complete, but the child must not be traversed twice.
				return nil
			}
			return errors.New("program/flow/evaluation: duplicate or cyclic composite occurrence")
		}
		plane[ordinal] = true
	}
	walker.stack = append(walker.stack, frame{term: term, owner: expectedOwner, prefix: prefix})
	if walker.pending != nil && !walker.pending.discover {
		if err := walker.pending.subject(term, prefix); err != nil {
			walker.stack = walker.stack[:len(walker.stack)-1]
			return err
		}
	}
	return nil
}
