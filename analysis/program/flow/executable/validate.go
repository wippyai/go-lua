package executable

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

type validated struct {
	counts [keyspace.FamilyCount]uint32
	source identity.ContentID
	flow   identity.ContentID
	static identity.ContentID
	module identity.ContentID
	entry  keyspace.Term
}

func validateInputs(
	sourceView source.View,
	flow authored.View,
	forest *containment.Result,
	control *sourcecontrol.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (validated, error) {
	var out validated
	if forest == nil || control == nil {
		return out, errors.New("program/flow/executable: structural owner is unavailable")
	}
	identity := sourceView.Identity()
	out.source = identity.ContentID()
	if !out.source.Available() || identity.Name() == "" || identity.TermCount() == 0 {
		return out, errors.New("program/flow/executable: Source identity is unavailable")
	}
	if !flow.Cold().ContentID().Available() {
		return out, errors.New("program/flow/executable: authored Flow is unavailable")
	}
	flowID := flow.Cold().ContentID()
	out.flow = flowID
	if !staticID.Available() || !moduleID.Available() {
		return out, errors.New("program/flow/executable: Static or Module identity is unavailable")
	}
	out.static = staticID
	out.module = moduleID
	if !containment.Matches(forest, out.source, flowID, staticID, moduleID) {
		return out, errors.New("program/flow/executable: containment provenance disagrees with Source, Flow, Static, or Module")
	}
	if !sourcecontrol.Matches(control, out.source, flowID, staticID, moduleID) {
		return out, errors.New("program/flow/executable: source-control provenance disagrees with Source, Flow, Static, or Module")
	}

	var total uint64
	var outcome uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return out, errors.New("program/flow/executable: invalid Source family denominator")
		}
		if family == keyspace.FamilyOutcome {
			outcome = uint64(count)
			continue
		}
		out.counts[family] = uint32(count)
		total += uint64(count)
	}
	if total == 0 || total > uint64(^uint32(0)) || total+outcome != uint64(identity.TermCount()) {
		return out, errors.New("program/flow/executable: Source denominator mismatch")
	}
	if forest.Count() != int(total) {
		return out, errors.New("program/flow/executable: containment denominator mismatch")
	}
	if control.NodeCount() == 0 {
		return out, errors.New("program/flow/executable: source-control coordinate space is empty")
	}
	if err := validateAuthoredCounts(flow, out.counts); err != nil {
		return out, err
	}
	if err := validateContainmentDenominator(forest, out.counts); err != nil {
		return out, err
	}
	entry, err := validateBodyRoots(sourceView, forest, control, out.counts)
	if err != nil {
		return out, err
	}
	out.entry = entry
	return out, nil
}

func validateAuthoredCounts(view authored.View, counts [keyspace.FamilyCount]uint32) error {
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyValues, view.Values().Count()},
		{keyspace.FamilyLensExact, view.Access().Exact().Count()},
		{keyspace.FamilyLensKey, view.Access().Dynamic().Count()},
		{keyspace.FamilyCell, view.Storage().Cells().Count()},
		{keyspace.FamilyRead, view.Storage().Reads().Count()},
		{keyspace.FamilyVararg, view.Storage().Varargs().Count()},
		{keyspace.FamilyBind, view.Storage().Binds().Count()},
		{keyspace.FamilyAssign, view.Storage().Assigns().Count()},
		{keyspace.FamilyWrite, view.Storage().Writes().Count()},
		{keyspace.FamilyTable, view.Tables().Count()},
		{keyspace.FamilyTableField, view.Fields().Count()},
		{keyspace.FamilyUnary, view.Operators().Unaries().Count()},
		{keyspace.FamilyBinary, view.Operators().Binaries().Count()},
		{keyspace.FamilySelect, view.Operators().Selects().Count()},
		{keyspace.FamilyFunction, view.Functions().Count()},
		{keyspace.FamilyCall, view.Calls().Count()},
		{keyspace.FamilyReturn, view.Control().Returns().Count()},
		{keyspace.FamilyBreak, view.Control().Breaks().Count()},
		{keyspace.FamilyLabel, view.Control().Labels().Count()},
		{keyspace.FamilyGoto, view.Control().Gotos().Count()},
		{keyspace.FamilyBranch, view.Control().Branches().Count()},
		{keyspace.FamilyLoop, view.Control().Loops().Count()},
		{keyspace.FamilyValueClaim, view.Claims().Count()},
		{keyspace.FamilyTypeValue, view.TypeValues().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || !keyspace.TermOrdinalFits(check.count) ||
			uint64(check.count) != uint64(counts[check.family]) {
			return errors.New("program/flow/executable: authored family denominator mismatch")
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			if !authoredDense(view, family, ordinal-1, term) {
				return errors.New("program/flow/executable: authored family is not dense")
			}
		}
	}
	return nil
}

func authoredDense(view authored.View, family keyspace.Family, index uint32, want keyspace.Term) bool {
	var got keyspace.Term
	var ok bool
	switch family {
	case keyspace.FamilyValues:
		got, ok = view.Values().At(int(index))
	case keyspace.FamilyLensExact:
		got, ok = view.Access().Exact().At(int(index))
	case keyspace.FamilyLensKey:
		got, ok = view.Access().Dynamic().At(int(index))
	case keyspace.FamilyCell:
		got, ok = view.Storage().Cells().At(int(index))
	case keyspace.FamilyRead:
		got, ok = view.Storage().Reads().At(int(index))
	case keyspace.FamilyVararg:
		got, ok = view.Storage().Varargs().At(int(index))
	case keyspace.FamilyBind:
		got, ok = view.Storage().Binds().At(int(index))
	case keyspace.FamilyAssign:
		got, ok = view.Storage().Assigns().At(int(index))
	case keyspace.FamilyWrite:
		got, ok = view.Storage().Writes().At(int(index))
	case keyspace.FamilyTable:
		got, ok = view.Tables().At(int(index))
	case keyspace.FamilyTableField:
		got, ok = view.Fields().At(int(index))
	case keyspace.FamilyUnary:
		got, ok = view.Operators().Unaries().At(int(index))
	case keyspace.FamilyBinary:
		got, ok = view.Operators().Binaries().At(int(index))
	case keyspace.FamilySelect:
		got, ok = view.Operators().Selects().At(int(index))
	case keyspace.FamilyFunction:
		got, ok = view.Functions().At(int(index))
	case keyspace.FamilyCall:
		got, ok = view.Calls().At(int(index))
	case keyspace.FamilyReturn:
		got, ok = view.Control().Returns().At(int(index))
	case keyspace.FamilyBreak:
		got, ok = view.Control().Breaks().At(int(index))
	case keyspace.FamilyLabel:
		got, ok = view.Control().Labels().At(int(index))
	case keyspace.FamilyGoto:
		got, ok = view.Control().Gotos().At(int(index))
	case keyspace.FamilyBranch:
		got, ok = view.Control().Branches().At(int(index))
	case keyspace.FamilyLoop:
		got, ok = view.Control().Loops().At(int(index))
	case keyspace.FamilyValueClaim:
		got, ok = view.Claims().At(int(index))
	case keyspace.FamilyTypeValue:
		got, ok = view.TypeValues().At(int(index))
	default:
		return true
	}
	return ok && got == want
}

func validateContainmentDenominator(forest *containment.Result, counts [keyspace.FamilyCount]uint32) error {
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			if !forest.Contains(term, term) {
				return errors.New("program/flow/executable: containment denominator is not canonical")
			}
		}
	}
	return nil
}

func validateBodyRoots(
	sourceView source.View,
	forest *containment.Result,
	control *sourcecontrol.Result,
	counts [keyspace.FamilyCount]uint32,
) (keyspace.Term, error) {
	bodyCount := counts[keyspace.FamilyBody]
	if bodyCount == 0 {
		return 0, errors.New("program/flow/executable: Body denominator is empty")
	}
	index := sourceView.Index()
	var entry keyspace.Term
	var entryCount uint32
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		forestParent, forestHasParent := forest.Parent(body)
		sourceParent, sourceHasParent := index.BodyParent(body)
		if sourceHasParent != forestHasParent || (sourceHasParent && sourceParent != forestParent) {
			return 0, errors.New("program/flow/executable: Body parent disagrees with containment")
		}
		if sourceHasParent && (keyspace.TermFamily(sourceParent) != keyspace.FamilyBody ||
			sourceParent == body || keyspace.TermOrdinal(sourceParent) > bodyCount) {
			return 0, errors.New("program/flow/executable: malformed Source Body parent")
		}
		if forestHasParent {
			parent := forestParent
			if keyspace.TermFamily(parent) != keyspace.FamilyBody || parent == body ||
				keyspace.TermOrdinal(parent) > bodyCount {
				return 0, errors.New("program/flow/executable: malformed Body parent")
			}
		} else {
			if entry != 0 {
				return 0, errors.New("program/flow/executable: multiple Entry Bodies")
			}
			entry = body
			entryCount++
		}
		length, ok := index.BodyRootLen(body)
		if !ok || length < 0 {
			return 0, errors.New("program/flow/executable: canonical Body roots are unavailable")
		}
		for cursor := 0; cursor < length; cursor++ {
			root, rootOK := index.BodyRootAt(body, cursor)
			if !rootOK || !validTerm(root, counts) {
				return 0, errors.New("program/flow/executable: malformed canonical Body root")
			}
			parent, parentOK := forest.Parent(root)
			if !parentOK || parent != body {
				return 0, errors.New("program/flow/executable: direct Body root parent disagrees with containment")
			}
			_, nodeOK := control.Cursor(body, uint32(cursor))
			// An unreachable root is valid; Cursor must still be present.
			if !nodeOK {
				return 0, errors.New("program/flow/executable: Body root lacks source-control cursor")
			}
		}
	}
	if entryCount != 1 || entry == 0 {
		return 0, errors.New("program/flow/executable: Entry Body is unavailable")
	}
	return entry, nil
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		family != keyspace.FamilyOutcome && ordinal != 0 && ordinal <= counts[family]
}
