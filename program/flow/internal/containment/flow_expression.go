package containment

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// emitFlowExpressions emits only authored expression containment.  Lexical
// ownership, control-body ownership, cell residence, and semantic references
// are intentionally not represented here; those relations belong to their
// owning proof lanes.  Every row is nevertheless checked before an edge is
// published so a malformed authored view fails closed at this boundary.
func emitFlowExpressions(
	preimage source.Preimage,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
	result *emission,
) (emission, error) {
	if result == nil {
		return emission{}, errFlowExpression("nil expression emission")
	}
	if err := validateFlowExpressionCounts(view, counts); err != nil {
		return emission{}, err
	}
	edgeStart := len(result.edges)
	edges := result.edges

	values := view.Values()
	for index := uint32(0); index < counts[keyspace.FamilyValues]; index++ {
		term, ok := values.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyValues, index+1) {
			return emission{}, errFlowExpression("noncanonical Values ordinal")
		}
		owner, tail, ok := values.Get(term)
		if !ok || !bodyTerm(owner, counts) {
			return emission{}, errFlowExpression("invalid Values owner")
		}
		length, ok := values.Len(term)
		if !ok || length < 0 {
			return emission{}, errFlowExpression("invalid Values range")
		}
		for memberIndex := 0; memberIndex < length; memberIndex++ {
			member, ok := values.Member(term, memberIndex)
			if !ok || !flowrole.ValueOccurrence(counts, member) {
				return emission{}, errFlowExpression("invalid Values member")
			}
			edges = append(edges, kernelEdge{child: member, parent: term})
		}
		if tail != 0 {
			if !flowrole.OpenOccurrence(counts, tail) {
				return emission{}, errFlowExpression("invalid Values tail")
			}
			edges = append(edges, kernelEdge{child: tail, parent: term})
		}
	}

	exact := view.Access().Exact()
	for index := uint32(0); index < counts[keyspace.FamilyLensExact]; index++ {
		term, ok := exact.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLensExact, index+1) {
			return emission{}, errFlowExpression("noncanonical exact Lens ordinal")
		}
		owner, base, source, fieldKind, ok := exact.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, base) ||
			!exactKeyTerm(view, source, fieldKind, counts) {
			return emission{}, errFlowExpression("invalid exact Lens foreign key")
		}
		edges = append(edges, kernelEdge{child: base, parent: term})
		edges = append(edges, kernelEdge{child: source, parent: term})
	}

	dynamic := view.Access().Dynamic()
	for index := uint32(0); index < counts[keyspace.FamilyLensKey]; index++ {
		term, ok := dynamic.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLensKey, index+1) {
			return emission{}, errFlowExpression("noncanonical dynamic Lens ordinal")
		}
		owner, base, key, ok := dynamic.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, base) || !flowrole.ValueOccurrence(counts, key) {
			return emission{}, errFlowExpression("invalid dynamic Lens foreign key")
		}
		edges = append(edges, kernelEdge{child: base, parent: term})
		edges = append(edges, kernelEdge{child: key, parent: term})
	}

	storage := view.Storage()
	reads := storage.Reads()
	for index := uint32(0); index < counts[keyspace.FamilyRead]; index++ {
		term, ok := reads.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyRead, index+1) {
			return emission{}, errFlowExpression("noncanonical Read ordinal")
		}
		owner, source, _, ok := reads.Get(term)
		if !ok || !bodyTerm(owner, counts) {
			return emission{}, errFlowExpression("invalid Read owner")
		}
		switch keyspace.TermFamily(source) {
		case keyspace.FamilyCell:
			if !termInFamily(source, keyspace.FamilyCell, counts) {
				return emission{}, errFlowExpression("invalid Read Cell")
			}
		case keyspace.FamilyLensExact, keyspace.FamilyLensKey:
			if !validTerm(source, counts) {
				return emission{}, errFlowExpression("invalid Read Lens")
			}
			edges = append(edges, kernelEdge{child: source, parent: term})
		default:
			return emission{}, errFlowExpression("invalid Read source family")
		}
	}

	binds := storage.Binds()
	for index := uint32(0); index < counts[keyspace.FamilyBind]; index++ {
		term, ok := binds.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyBind, index+1) {
			return emission{}, errFlowExpression("noncanonical Bind ordinal")
		}
		owner, valuesTerm, ok := binds.Get(term)
		if !ok || !bodyTerm(owner, counts) || !termInFamily(valuesTerm, keyspace.FamilyValues, counts) {
			return emission{}, errFlowExpression("invalid Bind foreign key")
		}
		edges = append(edges, kernelEdge{child: valuesTerm, parent: term})
	}

	assigns := storage.Assigns()
	for index := uint32(0); index < counts[keyspace.FamilyAssign]; index++ {
		term, ok := assigns.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyAssign, index+1) {
			return emission{}, errFlowExpression("noncanonical Assign ordinal")
		}
		owner, valuesTerm, ok := assigns.Get(term)
		if !ok || !bodyTerm(owner, counts) || !termInFamily(valuesTerm, keyspace.FamilyValues, counts) {
			return emission{}, errFlowExpression("invalid Assign foreign key")
		}
		edges = append(edges, kernelEdge{child: valuesTerm, parent: term})
	}

	writes := storage.Writes()
	for index := uint32(0); index < counts[keyspace.FamilyWrite]; index++ {
		term, ok := writes.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyWrite, index+1) {
			return emission{}, errFlowExpression("noncanonical Write ordinal")
		}
		assign, target, ok := writes.Get(term)
		if !ok || !termInFamily(assign, keyspace.FamilyAssign, counts) {
			return emission{}, errFlowExpression("invalid Write Assign")
		}
		switch keyspace.TermFamily(target) {
		case keyspace.FamilyCell:
			if !termInFamily(target, keyspace.FamilyCell, counts) {
				return emission{}, errFlowExpression("invalid Write Cell")
			}
		case keyspace.FamilyLensExact, keyspace.FamilyLensKey:
			if !validTerm(target, counts) {
				return emission{}, errFlowExpression("invalid Write Lens")
			}
			edges = append(edges, kernelEdge{child: target, parent: term})
		default:
			return emission{}, errFlowExpression("invalid Write target family")
		}
	}

	operators := view.Operators()
	unaries := operators.Unaries()
	for index := uint32(0); index < counts[keyspace.FamilyUnary]; index++ {
		term, ok := unaries.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyUnary, index+1) {
			return emission{}, errFlowExpression("noncanonical Unary ordinal")
		}
		owner, _, operand, ok := unaries.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, operand) {
			return emission{}, errFlowExpression("invalid Unary foreign key")
		}
		edges = append(edges, kernelEdge{child: operand, parent: term})
	}

	binaries := operators.Binaries()
	for index := uint32(0); index < counts[keyspace.FamilyBinary]; index++ {
		term, ok := binaries.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyBinary, index+1) {
			return emission{}, errFlowExpression("noncanonical Binary ordinal")
		}
		owner, _, left, right, ok := binaries.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) {
			return emission{}, errFlowExpression("invalid Binary foreign key")
		}
		edges = append(edges, kernelEdge{child: left, parent: term})
		edges = append(edges, kernelEdge{child: right, parent: term})
	}

	selects := operators.Selects()
	for index := uint32(0); index < counts[keyspace.FamilySelect]; index++ {
		term, ok := selects.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilySelect, index+1) {
			return emission{}, errFlowExpression("noncanonical Select ordinal")
		}
		owner, _, left, right, ok := selects.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, left) || !flowrole.ValueOccurrence(counts, right) {
			return emission{}, errFlowExpression("invalid Select foreign key")
		}
		edges = append(edges, kernelEdge{child: left, parent: term})
		edges = append(edges, kernelEdge{child: right, parent: term})
	}

	claims := view.Claims()
	for index := uint32(0); index < counts[keyspace.FamilyValueClaim]; index++ {
		term, ok := claims.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyValueClaim, index+1) {
			return emission{}, errFlowExpression("noncanonical ValueClaim ordinal")
		}
		owner, operand, _, ok := claims.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, operand) {
			return emission{}, errFlowExpression("invalid ValueClaim foreign key")
		}
		edges = append(edges, kernelEdge{child: operand, parent: term})
	}
	tables := view.Tables()
	fields := view.Fields()
	fieldSeen := make([]bool, int(counts[keyspace.FamilyTableField]))
	for index := uint32(0); index < counts[keyspace.FamilyTable]; index++ {
		table, ok := tables.At(int(index))
		if !ok || table != keyspace.MakeTerm(keyspace.FamilyTable, index+1) {
			return emission{}, errFlowExpression("noncanonical Table ordinal")
		}
		owner, ok := tables.Get(table)
		if !ok || !bodyTerm(owner, counts) {
			return emission{}, errFlowExpression("invalid Table owner")
		}
		fieldCount, ok := tables.FieldCount(table)
		if !ok || fieldCount < 0 {
			return emission{}, errFlowExpression("invalid Table field range")
		}
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			field, ok := tables.FieldAt(table, fieldIndex)
			if !ok || !termInFamily(field, keyspace.FamilyTableField, counts) {
				return emission{}, errFlowExpression("invalid Table field reference")
			}
			fieldOrdinal := keyspace.TermOrdinal(field)
			seen := &fieldSeen[fieldOrdinal-1]
			if *seen {
				return emission{}, errFlowExpression("duplicate Table field reference")
			}
			*seen = true
			fieldTable, _, _, _, ok := fields.Get(field)
			if !ok || fieldTable != table {
				return emission{}, errFlowExpression("Table field owner mismatch")
			}
		}
	}
	for _, seen := range fieldSeen {
		if !seen {
			return emission{}, errFlowExpression("orphan Table field")
		}
	}
	for index := uint32(0); index < counts[keyspace.FamilyTableField]; index++ {
		field, ok := fields.At(int(index))
		if !ok || field != keyspace.MakeTerm(keyspace.FamilyTableField, index+1) {
			return emission{}, errFlowExpression("noncanonical TableField ordinal")
		}
		table, key, valuesTerm, fieldKind, ok := fields.Get(field)
		if !ok || !termInFamily(table, keyspace.FamilyTable, counts) ||
			!fieldKeyTerm(view, key, fieldKind, counts) ||
			!termInFamily(valuesTerm, keyspace.FamilyValues, counts) {
			return emission{}, errFlowExpression("invalid TableField foreign key")
		}
		edges = append(edges, kernelEdge{child: field, parent: table})
		edges = append(edges, kernelEdge{child: key, parent: field})
		edges = append(edges, kernelEdge{child: valuesTerm, parent: field})
	}

	control := view.Control()
	returns := control.Returns()
	for index := uint32(0); index < counts[keyspace.FamilyReturn]; index++ {
		term, ok := returns.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyReturn, index+1) {
			return emission{}, errFlowExpression("noncanonical Return ordinal")
		}
		owner, valuesTerm, ok := returns.Get(term)
		if !ok || !bodyTerm(owner, counts) || !termInFamily(valuesTerm, keyspace.FamilyValues, counts) {
			return emission{}, errFlowExpression("invalid Return foreign key")
		}
		edges = append(edges, kernelEdge{child: valuesTerm, parent: term})
	}
	branches := control.Branches()
	for index := uint32(0); index < counts[keyspace.FamilyBranch]; index++ {
		term, ok := branches.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyBranch, index+1) {
			return emission{}, errFlowExpression("noncanonical Branch ordinal")
		}
		owner, condition, _, _, ok := branches.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, condition) {
			return emission{}, errFlowExpression("invalid Branch condition")
		}
		edges = append(edges, kernelEdge{child: condition, parent: term})
	}
	loops := control.Loops()
	for index := uint32(0); index < counts[keyspace.FamilyLoop]; index++ {
		term, ok := loops.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLoop, index+1) {
			return emission{}, errFlowExpression("noncanonical Loop ordinal")
		}
		owner, _, loopKind, controlTerm, ok := loops.Get(term)
		validControl := false
		switch loopKind {
		case kind.LoopWhile, kind.LoopRepeat:
			validControl = flowrole.ValueOccurrence(counts, controlTerm)
		case kind.LoopNumericFor, kind.LoopGenericFor:
			validControl = termInFamily(controlTerm, keyspace.FamilyValues, counts)
		}
		if !ok || !bodyTerm(owner, counts) || !validControl {
			return emission{}, errFlowExpression("invalid Loop foreign key")
		}
		edges = append(edges, kernelEdge{child: controlTerm, parent: term})
	}

	calls := view.Calls()
	for index := uint32(0); index < counts[keyspace.FamilyCall]; index++ {
		term, ok := calls.At(int(index))
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyCall, index+1) {
			return emission{}, errFlowExpression("noncanonical Call ordinal")
		}
		owner, callee, _, actuals, ok := calls.Get(term)
		if !ok || !bodyTerm(owner, counts) || !flowrole.ValueOccurrence(counts, callee) ||
			!termInFamily(actuals, keyspace.FamilyValues, counts) {
			return emission{}, errFlowExpression("invalid Call foreign key")
		}
		edges = append(edges, kernelEdge{child: callee, parent: term})
		edges = append(edges, kernelEdge{child: actuals, parent: term})
	}

	if err := validateFlowEdgeOwners(preimage, view, counts, edges[edgeStart:]); err != nil {
		return emission{}, err
	}
	return emission{edges: edges}, nil
}

func validateFlowExpressionCounts(view authored.View, counts [keyspace.FamilyCount]uint32) error {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return errFlowExpression("invalid or unsupported family cardinality")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if counts[family] > keyspace.MaxTermOrdinal {
			return errFlowExpression("family cardinality exceeds Term ordinal")
		}
	}
	if view.Values().Count() != int(counts[keyspace.FamilyValues]) ||
		view.Access().Exact().Count() != int(counts[keyspace.FamilyLensExact]) ||
		view.Access().Dynamic().Count() != int(counts[keyspace.FamilyLensKey]) ||
		view.Storage().Reads().Count() != int(counts[keyspace.FamilyRead]) ||
		view.Storage().Binds().Count() != int(counts[keyspace.FamilyBind]) ||
		view.Storage().Assigns().Count() != int(counts[keyspace.FamilyAssign]) ||
		view.Storage().Writes().Count() != int(counts[keyspace.FamilyWrite]) ||
		view.Tables().Count() != int(counts[keyspace.FamilyTable]) ||
		view.Fields().Count() != int(counts[keyspace.FamilyTableField]) ||
		view.Calls().Count() != int(counts[keyspace.FamilyCall]) ||
		view.Operators().Unaries().Count() != int(counts[keyspace.FamilyUnary]) ||
		view.Operators().Binaries().Count() != int(counts[keyspace.FamilyBinary]) ||
		view.Operators().Selects().Count() != int(counts[keyspace.FamilySelect]) ||
		view.Control().Returns().Count() != int(counts[keyspace.FamilyReturn]) ||
		view.Control().Branches().Count() != int(counts[keyspace.FamilyBranch]) ||
		view.Control().Loops().Count() != int(counts[keyspace.FamilyLoop]) ||
		view.Claims().Count() != int(counts[keyspace.FamilyValueClaim]) {
		return errFlowExpression("authored cardinality mismatch")
	}
	return nil
}

func bodyTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	return termInFamily(term, keyspace.FamilyBody, counts)
}

func termInFamily(term keyspace.Term, family keyspace.Family, counts [keyspace.FamilyCount]uint32) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}

func exactKeyTerm(view authored.View, term keyspace.Term, fieldKind kind.FieldKind, counts [keyspace.FamilyCount]uint32) bool {
	if fieldKind == kind.FieldName {
		return termInFamily(term, keyspace.FamilyKey, counts)
	}
	if fieldKind != kind.FieldExact || !validTerm(term, counts) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString:
		return true
	case keyspace.FamilyUnary:
		_, op, operand, ok := view.Operators().Unaries().Get(term)
		return ok && op == kind.UnaryNeg &&
			(termInFamily(operand, keyspace.FamilyInteger, counts) ||
				termInFamily(operand, keyspace.FamilyFloat, counts))
	default:
		return false
	}
}

func fieldKeyTerm(view authored.View, term keyspace.Term, fieldKind kind.FieldKind, counts [keyspace.FamilyCount]uint32) bool {
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return termInFamily(term, keyspace.FamilyKey, counts)
	case kind.FieldExact:
		return exactKeyTerm(view, term, kind.FieldExact, counts)
	case kind.FieldKey:
		return flowrole.ValueOccurrence(counts, term)
	default:
		return false
	}
}

func errFlowExpression(detail string) error {
	return errors.New("program/flow/containment: " + detail)
}
