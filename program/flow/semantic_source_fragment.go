package flow

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

var errUnavailableSemanticSourceFragment = errors.New("program/flow: semantic-source fragment is unavailable")

// SemanticSourceFragment reports Flow's retained cold semantic-source
// relations. The rows are deliberately written in generated catalog order;
// this function is an owner-local publisher, not a second catalog or a
// generic keyspace census.
//
// The fragment contains authored Flow relations and the small derived
// projections whose owner is Flow. It does not publish literals, Body,
// activation, Mu, continuation, binding-selection, or Module relations.
func buildSemanticSourceFragment(view View) ([]semanticsource.Publication, error) {
	counts, err := flowSemanticSourceCounts(view)
	if err != nil {
		return nil, err
	}
	definitions, err := flowSemanticSourceDefinitions()
	if err != nil {
		return nil, err
	}
	if len(definitions) != len(counts) {
		return nil, fmt.Errorf("program/flow: semantic-source definition count mismatch")
	}
	publications := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		publication, err := semanticsource.SealPublication(definition, counts[index])
		if err != nil {
			return nil, fmt.Errorf("program/flow: seal semantic-source row %d: %w", index, err)
		}
		publications = append(publications, publication)
	}
	return publications, nil
}

// flowSemanticSourceCounts validates the live typed Flow projections and
// returns only the fixed owner-local cardinality vector. The vector is an
// ephemeral scalar witness for the existing sealed range; it is not retained
// as a second publication denominator.
func flowSemanticSourceCounts(view View) ([semanticSourceFragmentPublicationCount]int, error) {
	var counts [semanticSourceFragmentPublicationCount]int
	if !view.semanticSourceAvailable() {
		return counts, errUnavailableSemanticSourceFragment
	}
	provenance := view.Provenance()
	if !provenance.Source.Available() || !provenance.Flow.Available() ||
		!provenance.Static.Available() || !provenance.Module.Available() ||
		provenance.Flow != view.ContentID() {
		return counts, errUnavailableSemanticSourceFragment
	}

	authored := view.Authored()
	valuesCount, occurrenceCount, err := semanticValues(authored.Values(), authored.Calls().Count(), authored.Storage().Varargs().Count())
	if err != nil {
		return counts, err
	}
	lensCount, err := semanticLenses(authored.Access())
	if err != nil {
		return counts, err
	}
	storageCount, cellCount, globalCount, readCount, assignCount, writeCount, varargCount, bindCount, err := semanticStorage(authored.Storage())
	if err != nil {
		return counts, err
	}
	constructorCount, fieldCount, err := semanticConstructors(authored.Tables(), authored.Fields())
	if err != nil {
		return counts, err
	}
	operatorCount, unaryNumericCount, lengthCount, arithmeticCount, bitwiseCount, concatCount, equalityCount, orderCount, getCount, setCount, err := semanticOperators(view)
	if err != nil {
		return counts, err
	}
	functionCount, captureCount, err := semanticFunctions(authored.Functions())
	if err != nil {
		return counts, err
	}
	callCount, directCallCount, err := semanticCalls(view)
	if err != nil {
		return counts, err
	}
	controlCount, genericForCount, err := semanticControl(authored.Control())
	if err != nil {
		return counts, err
	}
	claimCount, err := semanticClaims(authored.Claims())
	if err != nil {
		return counts, err
	}
	typeValueCount, err := semanticTypeValues(authored.TypeValues())
	if err != nil {
		return counts, err
	}
	outcomeCount, err := semanticOutcomes(view.Outcomes())
	if err != nil {
		return counts, err
	}
	transferCount, err := semanticTransfers(view.Causal().Edges())
	if err != nil {
		return counts, err
	}
	counts = [...]int{
		valuesCount, occurrenceCount, lensCount,
		storageCount, cellCount, globalCount, readCount, assignCount, writeCount, varargCount, bindCount,
		constructorCount, fieldCount,
		operatorCount, unaryNumericCount, lengthCount, arithmeticCount, bitwiseCount, concatCount, equalityCount, orderCount, getCount, setCount,
		functionCount, captureCount,
		callCount, directCallCount,
		controlCount, genericForCount,
		claimCount, typeValueCount, outcomeCount, transferCount,
	}
	return counts, nil
}

func semanticValues(view Values, callCount, varargCount int) (values, occurrences int, err error) {
	count, err := denseCount("Values", view.Count())
	if err != nil {
		return 0, 0, err
	}
	callCount, err = denseCount("Call tail", callCount)
	if err != nil {
		return 0, 0, err
	}
	varargCount, err = denseCount("Vararg tail", varargCount)
	if err != nil {
		return 0, 0, err
	}
	values = count
	occurrences = 0
	for index := 0; index < count; index++ {
		term, ok := view.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyValues, uint32(index+1)) {
			return 0, 0, queryError("Values", index)
		}
		owner, tail, ok := view.Get(term)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
			return 0, 0, queryError("Values owner", index)
		}
		fixed, ok := view.Len(term)
		if !ok || fixed < 0 {
			return 0, 0, queryError("Values fixed length", index)
		}
		if err := addCount(&occurrences, fixed); err != nil {
			return 0, 0, fmt.Errorf("program/flow: Values occurrence overflow at %d: %w", index, err)
		}
		if tail != 0 {
			family := keyspace.TermFamily(tail)
			if family != keyspace.FamilyCall && family != keyspace.FamilyVararg || keyspace.TermOrdinal(tail) == 0 {
				return 0, 0, queryError("Values tail", index)
			}
			ordinal := keyspace.TermOrdinal(tail)
			if family == keyspace.FamilyCall && uint64(ordinal) > uint64(callCount) {
				return 0, 0, queryError("Values Call tail", index)
			}
			if family == keyspace.FamilyVararg && uint64(ordinal) > uint64(varargCount) {
				return 0, 0, queryError("Values Vararg tail", index)
			}
			if err := addCount(&occurrences, 1); err != nil {
				return 0, 0, fmt.Errorf("program/flow: Values tail overflow at %d: %w", index, err)
			}
		}
	}
	return values, occurrences, nil
}

func semanticLenses(view Access) (int, error) {
	exact := view.Exact()
	dynamic := view.Dynamic()
	exactCount, err := denseCount("exact Lens", exact.Count())
	if err != nil {
		return 0, err
	}
	dynamicCount, err := denseCount("dynamic Lens", dynamic.Count())
	if err != nil {
		return 0, err
	}
	for index := 0; index < exactCount; index++ {
		term, ok := exact.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(index+1)) {
			return 0, queryError("exact Lens", index)
		}
		if _, _, _, _, ok := exact.Get(term); !ok {
			return 0, queryError("exact Lens row", index)
		}
	}
	for index := 0; index < dynamicCount; index++ {
		term, ok := dynamic.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLensKey, uint32(index+1)) {
			return 0, queryError("dynamic Lens", index)
		}
		if _, _, _, ok := dynamic.Get(term); !ok {
			return 0, queryError("dynamic Lens row", index)
		}
	}
	return addCounts(exactCount, dynamicCount)
}

func semanticStorage(view Storage) (primary, cells, globals, reads, assigns, writes, varargs, binds int, err error) {
	cellView := view.Cells()
	readView := view.Reads()
	assignView := view.Assigns()
	writeView := view.Writes()
	varargView := view.Varargs()
	bindView := view.Binds()
	cells, err = denseCount("Cell", cellView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	reads, err = denseCount("Read", readView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	assigns, err = denseCount("Assign", assignView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	writes, err = denseCount("Write", writeView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	varargs, err = denseCount("Vararg", varargView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	binds, err = denseCount("Bind", bindView.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}

	for index := 0; index < cells; index++ {
		term, ok := cellView.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1)) {
			return 0, 0, 0, 0, 0, 0, 0, 0, queryError("Cell", index)
		}
		cellKind, body, key, ok := cellView.Get(term)
		if !ok {
			return 0, 0, 0, 0, 0, 0, 0, 0, queryError("Cell row", index)
		}
		switch cellKind {
		case CellGlobal:
			if body != 0 || key == 0 {
				return 0, 0, 0, 0, 0, 0, 0, 0, queryError("Global Cell row", index)
			}
			globals++
		case CellLocal:
			if body == 0 || key != 0 {
				return 0, 0, 0, 0, 0, 0, 0, 0, queryError("local Cell row", index)
			}
		default:
			return 0, 0, 0, 0, 0, 0, 0, 0, queryError("Cell kind", index)
		}
	}
	if err := addCount(&primary, cells); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, globals); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, reads); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, assigns); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, writes); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, varargs); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := addCount(&primary, binds); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	// Every remaining Storage family is queried explicitly. The row payload
	// is not reinterpreted here; its typed View is the owner authority.
	if err := scanTerms(readView.At, func(term keyspace.Term) bool { _, _, _, ok := readView.Get(term); return ok }, keyspace.FamilyRead, reads, "Read"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(assignView.At, func(term keyspace.Term) bool { _, _, ok := assignView.Get(term); return ok }, keyspace.FamilyAssign, assigns, "Assign"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(writeView.At, func(term keyspace.Term) bool { _, _, ok := writeView.Get(term); return ok }, keyspace.FamilyWrite, writes, "Write"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(varargView.At, func(term keyspace.Term) bool { _, _, ok := varargView.Get(term); return ok }, keyspace.FamilyVararg, varargs, "Vararg"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(bindView.At, func(term keyspace.Term) bool { _, _, ok := bindView.Get(term); return ok }, keyspace.FamilyBind, binds, "Bind"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	return primary, cells, globals, reads, assigns, writes, varargs, binds, nil
}

func semanticConstructors(tables Tables, fields Fields) (int, int, error) {
	tableCount, err := denseCount("Table", tables.Count())
	if err != nil {
		return 0, 0, err
	}
	fieldCount, err := denseCount("TableField", fields.Count())
	if err != nil {
		return 0, 0, err
	}
	for index := 0; index < tableCount; index++ {
		term, ok := tables.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyTable, uint32(index+1)) {
			return 0, 0, queryError("Table", index)
		}
		if _, ok := tables.Get(term); !ok {
			return 0, 0, queryError("Table row", index)
		}
		fieldN, ok := tables.FieldCount(term)
		if !ok || fieldN < 0 {
			return 0, 0, queryError("Table field range", index)
		}
		for fieldIndex := 0; fieldIndex < fieldN; fieldIndex++ {
			field, ok := tables.FieldAt(term, fieldIndex)
			if !ok || keyspace.TermFamily(field) != keyspace.FamilyTableField || keyspace.TermOrdinal(field) == 0 {
				return 0, 0, queryError("Table field order", index)
			}
		}
	}
	for index := 0; index < fieldCount; index++ {
		term, ok := fields.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyTableField, uint32(index+1)) {
			return 0, 0, queryError("TableField", index)
		}
		if _, _, _, _, ok := fields.Get(term); !ok {
			return 0, 0, queryError("TableField row", index)
		}
		if _, _, ok := fields.Values(term); !ok {
			return 0, 0, queryError("TableField values", index)
		}
	}
	return tableCount, fieldCount, nil
}

func semanticOperators(view View) (primary, unaryNumeric, length, arithmetic, bitwise, concat, equality, order, get, set int, err error) {
	authored := view.Authored().Operators()
	unaries := authored.Unaries()
	binaries := authored.Binaries()
	selects := authored.Selects()
	unaryCount, err := denseCount("Unary", unaries.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	binaryCount, err := denseCount("Binary", binaries.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	selectCount, err := denseCount("Select", selects.Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(unaries.At, func(term keyspace.Term) bool { _, _, _, ok := unaries.Get(term); return ok }, keyspace.FamilyUnary, unaryCount, "Unary"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(binaries.At, func(term keyspace.Term) bool { _, _, _, _, ok := binaries.Get(term); return ok }, keyspace.FamilyBinary, binaryCount, "Binary"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	if err := scanTerms(selects.At, func(term keyspace.Term) bool { _, _, _, _, ok := selects.Get(term); return ok }, keyspace.FamilySelect, selectCount, "Select"); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	primary, err = addCounts(unaryCount, binaryCount, selectCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	candidates := view.Candidates()
	unaryNumeric, err = scanCandidate("Unary numeric", candidates.Unary().NumericCount(), candidates.Unary().NumericAt, keyspace.FamilyUnary, unaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	length, err = scanCandidate("Unary length", candidates.Unary().LengthCount(), candidates.Unary().LengthAt, keyspace.FamilyUnary, unaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	arithmetic, err = scanCandidate("Binary arithmetic", candidates.Binary().ArithmeticCount(), candidates.Binary().ArithmeticAt, keyspace.FamilyBinary, binaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	bitwise, err = scanCandidate("Binary bitwise", candidates.Binary().BitwiseCount(), candidates.Binary().BitwiseAt, keyspace.FamilyBinary, binaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	concat, err = scanCandidate("Binary concat", candidates.Binary().ConcatCount(), candidates.Binary().ConcatAt, keyspace.FamilyBinary, binaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	equality, err = scanCandidate("Binary equality", candidates.Binary().EqualityCount(), candidates.Binary().EqualityAt, keyspace.FamilyBinary, binaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	order, err = scanCandidate("Binary order", candidates.Binary().OrderCount(), candidates.Binary().OrderAt, keyspace.FamilyBinary, binaryCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	get, err = scanCandidate("Read access", candidates.Access().GetCount(), candidates.Access().GetAt, keyspace.FamilyRead, view.Authored().Storage().Reads().Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	set, err = scanCandidate("Write access", candidates.Access().SetCount(), candidates.Access().SetAt, keyspace.FamilyWrite, view.Authored().Storage().Writes().Count())
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, err
	}
	return primary, unaryNumeric, length, arithmetic, bitwise, concat, equality, order, get, set, nil
}

func semanticFunctions(view Functions) (functions, captures int, err error) {
	functions, err = denseCount("Function", view.Count())
	if err != nil {
		return 0, 0, err
	}
	for index := 0; index < functions; index++ {
		term, ok := view.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1)) {
			return 0, 0, queryError("Function", index)
		}
		if _, _, _, ok := view.Get(term); !ok {
			return 0, 0, queryError("Function row", index)
		}
		captureCount, ok := view.CaptureCount(term)
		if !ok || captureCount < 0 {
			return 0, 0, queryError("Function capture range", index)
		}
		if err := addCount(&captures, captureCount); err != nil {
			return 0, 0, fmt.Errorf("program/flow: Function capture overflow at %d: %w", index, err)
		}
		for captureIndex := 0; captureIndex < captureCount; captureIndex++ {
			inner, outer, ok := view.CaptureAt(term, captureIndex)
			if !ok || keyspace.TermFamily(inner) != keyspace.FamilyCell || keyspace.TermOrdinal(inner) == 0 || keyspace.TermFamily(outer) != keyspace.FamilyCell || keyspace.TermOrdinal(outer) == 0 {
				return 0, 0, queryError("Function capture", index)
			}
		}
	}
	return functions, captures, nil
}

func semanticCalls(view View) (calls, directCalls int, err error) {
	authored := view.Authored().Calls()
	calls, err = denseCount("Call", authored.Count())
	if err != nil {
		return 0, 0, err
	}
	reads := view.Authored().Storage().Reads().Count()
	direct := view.DirectBindings()
	for index := 0; index < calls; index++ {
		term, ok := authored.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1)) {
			return 0, 0, queryError("Call", index)
		}
		if _, _, _, _, ok := authored.Get(term); !ok {
			return 0, 0, queryError("Call row", index)
		}
		read, form, ok := direct.Call(term)
		if !ok {
			continue
		}
		if keyspace.TermFamily(read) != keyspace.FamilyRead || keyspace.TermOrdinal(read) == 0 || uint64(keyspace.TermOrdinal(read)) > uint64(reads) || (form != CallFormPlain && form != CallFormMethod) {
			return 0, 0, queryError("direct Call binding", index)
		}
		if err := addCount(&directCalls, 1); err != nil {
			return 0, 0, err
		}
	}
	return calls, directCalls, nil
}

func semanticControl(view Control) (control, genericFor int, err error) {
	returns, err := scanTermsCount("Return", view.Returns().Count, view.Returns().At, func(term keyspace.Term) bool { _, _, ok := view.Returns().Get(term); return ok }, keyspace.FamilyReturn)
	if err != nil {
		return 0, 0, err
	}
	breaks, err := scanTermsCount("Break", view.Breaks().Count, view.Breaks().At, func(term keyspace.Term) bool { _, ok := view.Breaks().Get(term); return ok }, keyspace.FamilyBreak)
	if err != nil {
		return 0, 0, err
	}
	labels, err := scanTermsCount("Label", view.Labels().Count, view.Labels().At, func(term keyspace.Term) bool { _, ok := view.Labels().Get(term); return ok }, keyspace.FamilyLabel)
	if err != nil {
		return 0, 0, err
	}
	gotos, err := scanTermsCount("Goto", view.Gotos().Count, view.Gotos().At, func(term keyspace.Term) bool { _, _, ok := view.Gotos().Get(term); return ok }, keyspace.FamilyGoto)
	if err != nil {
		return 0, 0, err
	}
	branches, err := scanTermsCount("Branch", view.Branches().Count, view.Branches().At, func(term keyspace.Term) bool { _, _, _, _, ok := view.Branches().Get(term); return ok }, keyspace.FamilyBranch)
	if err != nil {
		return 0, 0, err
	}
	loops := view.Loops()
	loopCount, err := denseCount("Loop", loops.Count())
	if err != nil {
		return 0, 0, err
	}
	for index := 0; index < loopCount; index++ {
		term, ok := loops.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1)) {
			return 0, 0, queryError("Loop", index)
		}
		_, _, loopKind, _, ok := loops.Get(term)
		if !ok || loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
			return 0, 0, queryError("Loop row", index)
		}
		if loopKind == kind.LoopGenericFor {
			if err := addCount(&genericFor, 1); err != nil {
				return 0, 0, err
			}
		}
	}
	control, err = addCounts(returns, breaks, labels, gotos, branches, loopCount)
	if err != nil {
		return 0, 0, err
	}
	return control, genericFor, nil
}

func semanticClaims(view Claims) (int, error) {
	return scanTermsCount("ValueClaim", view.Count, view.At, func(term keyspace.Term) bool {
		_, _, claimKind, ok := view.Get(term)
		return ok && claimKind >= kind.ValueClaimTypeAs && claimKind <= kind.ValueClaimNonNil
	}, keyspace.FamilyValueClaim)
}

func semanticTypeValues(view TypeValues) (int, error) {
	return scanTermsCount("TypeValue", view.Count, view.At, func(term keyspace.Term) bool { _, ok := view.Get(term); return ok }, keyspace.FamilyTypeValue)
}

func semanticOutcomes(view Outcomes) (int, error) {
	count, err := denseCount("Outcome", view.Count())
	if err != nil {
		return 0, err
	}
	for index := 0; index < count; index++ {
		term, ok := view.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(index+1)) {
			return 0, queryError("Outcome", index)
		}
		if _, ok := view.Get(term); !ok {
			return 0, queryError("Outcome row", index)
		}
	}
	return count, nil
}

func semanticTransfers(view Edges) (int, error) {
	count := view.Count()
	if count < 0 {
		return 0, errors.New("program/flow: negative causal edge count")
	}
	for index := 0; index < count; index++ {
		if _, ok := view.At(index); !ok {
			return 0, queryError("causal edge", index)
		}
	}
	return count, nil
}

func scanCandidate(name string, count int, at func(int) (keyspace.Term, bool), family keyspace.Family, sourceCount int) (int, error) {
	count, err := denseCount(name, count)
	if err != nil {
		return 0, err
	}
	if sourceCount < 0 || !keyspace.TermOrdinalFits(sourceCount) {
		return 0, fmt.Errorf("program/flow: %s source denominator is unavailable", name)
	}
	var previous keyspace.Term
	for index := 0; index < count; index++ {
		term, ok := at(index)
		if !ok || keyspace.TermFamily(term) != family || keyspace.TermOrdinal(term) == 0 || uint64(keyspace.TermOrdinal(term)) > uint64(sourceCount) || (index > 0 && term <= previous) {
			return 0, queryError(name, index)
		}
		previous = term
	}
	return count, nil
}

func scanTermsCount(name string, countFn func() int, at func(int) (keyspace.Term, bool), row func(keyspace.Term) bool, family keyspace.Family) (int, error) {
	count, err := denseCount(name, countFn())
	if err != nil {
		return 0, err
	}
	for index := 0; index < count; index++ {
		term, ok := at(index)
		if !ok || term != keyspace.MakeTerm(family, uint32(index+1)) || !row(term) {
			return 0, queryError(name, index)
		}
	}
	return count, nil
}

func scanTerms(at func(int) (keyspace.Term, bool), row func(keyspace.Term) bool, family keyspace.Family, count int, name string) error {
	if count < 0 {
		return fmt.Errorf("program/flow: negative %s count", name)
	}
	for index := 0; index < count; index++ {
		term, ok := at(index)
		if !ok || term != keyspace.MakeTerm(family, uint32(index+1)) || !row(term) {
			return queryError(name, index)
		}
	}
	return nil
}

func denseCount(name string, count int) (int, error) {
	if count < 0 || !keyspace.TermOrdinalFits(count) {
		return 0, fmt.Errorf("program/flow: %s count is unavailable: %d", name, count)
	}
	return count, nil
}

func addCounts(values ...int) (int, error) {
	total := 0
	for _, value := range values {
		if err := addCount(&total, value); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func addCount(total *int, value int) error {
	if total == nil || value < 0 || *total < 0 || value > int(^uint(0)>>1)-*total {
		return errors.New("program/flow: semantic-source count overflow")
	}
	*total += value
	return nil
}

func queryError(name string, index int) error {
	return fmt.Errorf("program/flow: %s query inconsistency at index %d", name, index)
}
