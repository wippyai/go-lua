package program

// This file is the only semantic-source composition point for Program.  The
// child packages own typed columns; they do not retain a semantic-source
// range, row, or generic publication stream. The public query below is
// the one root-owned composition surface and derives detached publications
// from those immutable typed columns.

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

const programSemanticSourcePublicationCount = 57

// SemanticSourcePublications returns the canonical Program-owned semantic
// source measure as detached claims. Program retains no duplicate claim
// vector; each claim is read from an immutable typed owner column and sealed
// into a caller-owned value for this query only.
func (program *Program) SemanticSourcePublications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	counts, definitions, ok := program.semanticSourceMeasures(schema)
	if !ok {
		return nil
	}
	rows := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		row, err := semanticsource.SealPublication(definition, counts[index])
		if err != nil {
			return nil
		}
		rows = append(rows, row)
	}
	return rows
}

func (program *Program) semanticSourceMeasures(schema semanticsource.ProgramSchema) ([]int, []semanticsource.RelationDef, bool) {
	if schema == nil || schema.Count() == 0 || !schema.SchemaDigest().Available() {
		return nil, nil, false
	}
	counts, err := deriveProgramSemanticSourceCounts(program, schema)
	if err != nil {
		return nil, nil, false
	}
	definitions, ok := programSemanticSourceDefinitions(schema)
	if !ok || len(counts) != len(definitions) {
		return nil, nil, false
	}
	return counts[:], definitions, true
}

// deriveProgramSemanticSourceCounts performs the one root composition cut.
// The returned array is ephemeral; no copy is installed on Program.
func deriveProgramSemanticSourceCounts(program *Program, schema semanticsource.ProgramSchema) ([programSemanticSourcePublicationCount]int, error) {
	var result [programSemanticSourcePublicationCount]int
	if schema == nil || schema.Count() == 0 || !schema.SchemaDigest().Available() {
		return result, errors.New("unavailable semantic-source schema")
	}
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil || !program.id.Available() {
		return result, errors.New("unavailable Program owner quartet")
	}

	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.ContentID()
	moduleID := program.module.View().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return result, errors.New("unavailable Program child identity")
	}
	provenance := program.flow.View().Provenance()
	if provenance.Source != sourceID || provenance.Flow != flowID || provenance.Static != staticID || provenance.Module != moduleID {
		return result, errors.New("Program Flow provenance mismatch")
	}
	rootID, err := rootContentID(sourceID, flowID, staticID, moduleID)
	if err != nil || rootID != program.id {
		return result, errors.New("Program owner identity mismatch")
	}

	sourceCounts, err := programSourceCounts(program.source.View())
	if err != nil {
		return result, err
	}
	flowCounts, err := programFlowCounts(program.flow.View())
	if err != nil {
		return result, err
	}
	staticCounts, err := programStaticCounts(program.static.View())
	if err != nil {
		return result, err
	}
	moduleCounts, err := programModuleCounts(program.module.View())
	if err != nil {
		return result, err
	}

	definitions, ok := programSemanticSourceDefinitions(schema)
	if !ok {
		return result, errors.New("invalid generated Program semantic-source schema")
	}
	for index, definition := range definitions {
		count, ok := programOwnerCount(definition.Token(), sourceCounts, flowCounts, staticCounts, moduleCounts)
		if !ok || count < 0 || !keyspace.TermOrdinalFits(count) {
			return result, fmt.Errorf("invalid semantic-source count at schema ordinal %d", index)
		}
		// SealPublication validates the generated definition/count contract at
		// the root cut without retaining a second row representation.
		if _, err := semanticsource.SealPublication(definition, count); err != nil {
			return result, err
		}
		result[index] = count
	}
	return result, nil
}

func programSemanticSourceDefinitions(schema semanticsource.ProgramSchema) ([]semanticsource.RelationDef, bool) {
	if schema == nil || schema.Count() == 0 {
		return nil, false
	}
	definitions := make([]semanticsource.RelationDef, 0, programSemanticSourcePublicationCount)
	for index := 0; index < schema.Count(); index++ {
		definition, ok := schema.DefinitionAt(index)
		if !ok {
			return nil, false
		}
		switch definition.Token().Origin() {
		case semanticsource.OriginProgramSourceProvenance,
			semanticsource.OriginProgramSourceOrder,
			semanticsource.OriginProgramSourceKey,
			semanticsource.OriginProgramSourceExactKey,
			semanticsource.OriginProgramSourceControlFault,
			semanticsource.OriginProgramFlowLiterals,
			semanticsource.OriginProgramFlowValues,
			semanticsource.OriginProgramFlowLens,
			semanticsource.OriginProgramFlowStorage,
			semanticsource.OriginProgramFlowConstructors,
			semanticsource.OriginProgramFlowOperators,
			semanticsource.OriginProgramFlowFunction,
			semanticsource.OriginProgramFlowCall,
			semanticsource.OriginProgramFlowControl,
			semanticsource.OriginProgramFlowClaim,
			semanticsource.OriginProgramFlowTypeValue,
			semanticsource.OriginProgramFlowOutcome,
			semanticsource.OriginProgramFlowTransfer,
			semanticsource.OriginProgramFlowBody,
			semanticsource.OriginProgramStatic,
			semanticsource.OriginProgramModuleImport,
			semanticsource.OriginProgramModuleEntry:
			definitions = append(definitions, definition)
		}
	}
	return definitions, len(definitions) == programSemanticSourcePublicationCount
}

func programOwnerCount(token semanticsource.Token, sourceCounts [8]int, flowCounts [33]int, staticCounts [10]int, moduleCounts [6]int) (int, bool) {
	switch token.Origin() {
	case semanticsource.OriginProgramSourceProvenance:
		return sourceCounts[0], token.Facet() == 0
	case semanticsource.OriginProgramSourceOrder:
		return sourceCounts[1], token.Facet() == 0
	case semanticsource.OriginProgramSourceKey:
		return sourceCounts[2], token.Facet() == 0
	case semanticsource.OriginProgramSourceExactKey:
		return sourceCounts[3], token.Facet() == 0
	case semanticsource.OriginProgramSourceControlFault:
		return sourceCounts[4], token.Facet() == 0
	case semanticsource.OriginProgramFlowLiterals:
		return sourceCounts[5], token.Facet() == 0
	case semanticsource.OriginProgramFlowBody:
		switch token.Facet() {
		case 0:
			return sourceCounts[6], true
		case semanticsource.FacetProgramFlowBodyRoots:
			return sourceCounts[7], true
		}
	case semanticsource.OriginProgramFlowValues:
		if token.Facet() == 0 {
			return flowCounts[0], true
		}
		if token.Facet() == semanticsource.FacetProgramFlowValueOccurrence {
			return flowCounts[1], true
		}
	case semanticsource.OriginProgramFlowLens:
		return flowCounts[2], token.Facet() == 0
	case semanticsource.OriginProgramFlowStorage:
		if token.Facet() <= semanticsource.FacetProgramFlowStorageBind {
			return flowCounts[3+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowConstructors:
		if token.Facet() <= semanticsource.FacetProgramFlowConstructorField {
			return flowCounts[11+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowOperators:
		if token.Facet() <= semanticsource.FacetProgramFlowIndexSet {
			return flowCounts[13+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowFunction:
		if token.Facet() <= semanticsource.FacetProgramFlowFunctionCapture {
			return flowCounts[23+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowCall:
		if token.Facet() <= semanticsource.FacetProgramFlowDirectCallBinding {
			return flowCounts[25+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowControl:
		if token.Facet() <= semanticsource.FacetProgramFlowGenericFor {
			return flowCounts[27+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowClaim:
		return flowCounts[29], token.Facet() == 0
	case semanticsource.OriginProgramFlowTypeValue:
		return flowCounts[30], token.Facet() == 0
	case semanticsource.OriginProgramFlowOutcome:
		return flowCounts[31], token.Facet() == 0
	case semanticsource.OriginProgramFlowTransfer:
		return flowCounts[32], token.Facet() == 0
	case semanticsource.OriginProgramStatic:
		switch token.Facet() {
		case 0:
			return staticCounts[0], true
		case semanticsource.FacetProgramStaticFunctionContract:
			return staticCounts[1], true
		case semanticsource.FacetProgramStaticCallTypeArguments:
			return staticCounts[2], true
		case semanticsource.FacetProgramStaticCellDeclaredType:
			return staticCounts[3], true
		case semanticsource.FacetProgramStaticClaimTarget:
			return staticCounts[4], true
		case semanticsource.FacetProgramStaticTypeValueTarget:
			return staticCounts[5], true
		case semanticsource.FacetProgramStaticTypeof:
			return staticCounts[6], true
		case semanticsource.FacetProgramStaticAnnotation:
			return staticCounts[7], true
		case semanticsource.FacetProgramStaticPublication:
			return staticCounts[8], true
		case semanticsource.FacetProgramStaticTypeRef:
			return staticCounts[9], true
		}
	case semanticsource.OriginProgramModuleImport:
		if token.Facet() <= semanticsource.FacetProgramModuleRequest {
			return moduleCounts[int(token.Facet())], true
		}
	case semanticsource.OriginProgramModuleEntry:
		if token.Facet() <= semanticsource.FacetProgramModuleEntryRootFunction {
			return moduleCounts[2+int(token.Facet())], true
		}
	}
	return 0, false
}

func programSourceCounts(view source.View) ([8]int, error) {
	var counts [8]int
	if !view.Identity().ContentID().Available() || view.Identity().Name() == "" {
		return counts, errors.New("unavailable Source view")
	}
	bodyCount := view.Identity().FamilyCount(keyspace.FamilyBody)
	if !countFits(bodyCount) {
		return counts, errors.New("invalid Source body cardinality")
	}
	direct, roots := 0, 0
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		bodyLen, ok := view.Order().BodyLen(body)
		if !ok || !addMeasure(&direct, bodyLen) {
			return counts, errors.New("invalid Source body order column")
		}
		rootLen, ok := view.Index().BodyRootLen(body)
		if !ok || !addMeasure(&roots, rootLen) {
			return counts, errors.New("invalid Source body-root column")
		}
	}
	literals := view.Literals()
	literalCount := literals.Nils().Count() + literals.Bools().Count() + literals.Integers().Count() + literals.Floats().Count() + literals.Strings().Count()
	keys, exactKeys := view.Keys().Count(), view.Keys().ExactCount()
	faults := view.Faults().Count()
	if !countFits(literalCount) || !countFits(keys) || !countFits(exactKeys) || !countFits(faults) {
		return counts, errors.New("invalid Source cardinality")
	}
	return [...]int{direct, direct, keys, exactKeys, faults, literalCount, bodyCount, roots}, nil
}

func programFlowCounts(view flow.View) ([33]int, error) {
	var counts [33]int
	if !view.ContentID().Available() {
		return counts, errors.New("unavailable Flow view")
	}
	authored := view.Authored()
	values := authored.Values()
	valueCount := values.Count()
	occurrences := 0
	for index := 0; index < valueCount; index++ {
		term, ok := values.At(index)
		if !ok {
			return counts, errors.New("invalid Flow values column")
		}
		fixed, ok := values.Len(term)
		if !ok || !addMeasure(&occurrences, fixed) {
			return counts, errors.New("invalid Flow value occurrence column")
		}
		_, tail, ok := values.Get(term)
		if !ok {
			return counts, errors.New("invalid Flow value row")
		}
		if tail != 0 && !addMeasure(&occurrences, 1) {
			return counts, errors.New("invalid Flow value tail column")
		}
	}
	access := authored.Access()
	storage := authored.Storage()
	cells, reads, assigns, writes := storage.Cells().Count(), storage.Reads().Count(), storage.Assigns().Count(), storage.Writes().Count()
	varargs, binds := storage.Varargs().Count(), storage.Binds().Count()
	globals := 0
	for index := 0; index < cells; index++ {
		term, ok := storage.Cells().At(index)
		if !ok {
			return counts, errors.New("invalid Flow cell column")
		}
		cellKind, _, _, ok := storage.Cells().Get(term)
		if !ok {
			return counts, errors.New("invalid Flow cell row")
		}
		if cellKind == flow.CellGlobal {
			globals++
		}
	}
	storagePrimary, ok := sumMeasures(cells, globals, reads, assigns, writes, varargs, binds)
	if !ok {
		return counts, errors.New("Flow storage cardinality overflow")
	}
	tables, fields := authored.Tables().Count(), authored.Fields().Count()
	operators := authored.Operators()
	unaries, binaries, selects := operators.Unaries().Count(), operators.Binaries().Count(), operators.Selects().Count()
	operatorPrimary, ok := sumMeasures(unaries, binaries, selects)
	if !ok {
		return counts, errors.New("Flow operator cardinality overflow")
	}
	candidates := view.Candidates()
	functions := authored.Functions()
	captures := 0
	for index := 0; index < functions.Count(); index++ {
		term, ok := functions.At(index)
		if !ok {
			return counts, errors.New("invalid Flow function column")
		}
		captureCount, ok := functions.CaptureCount(term)
		if !ok || !addMeasure(&captures, captureCount) {
			return counts, errors.New("invalid Flow capture column")
		}
	}
	calls := authored.Calls().Count()
	directCalls := 0
	for index := 0; index < calls; index++ {
		term, ok := authored.Calls().At(index)
		if !ok {
			return counts, errors.New("invalid Flow call column")
		}
		if _, _, ok := view.Selectors().DirectCall(term); ok {
			directCalls++
		}
	}
	control := authored.Control()
	returns, breaks := control.Returns().Count(), control.Breaks().Count()
	labels, gotos := control.Labels().Count(), control.Gotos().Count()
	branches, loops := control.Branches().Count(), control.Loops().Count()
	genericFor := 0
	for index := 0; index < loops; index++ {
		term, ok := control.Loops().At(index)
		if !ok {
			return counts, errors.New("invalid Flow loop column")
		}
		_, _, loopKind, _, ok := control.Loops().Get(term)
		if !ok {
			return counts, errors.New("invalid Flow loop row")
		}
		if loopKind == flowkind.LoopGenericFor {
			genericFor++
		}
	}
	controlPrimary, ok := sumMeasures(returns, breaks, labels, gotos, branches, loops)
	if !ok {
		return counts, errors.New("Flow control cardinality overflow")
	}
	transfers := view.Causal().Edges().Count()
	if !countFits(valueCount) || !countFits(occurrences) || !countFits(storagePrimary) || !countFits(cells) || !countFits(globals) || !countFits(reads) || !countFits(assigns) || !countFits(writes) || !countFits(varargs) || !countFits(binds) || !countFits(tables) || !countFits(fields) || !countFits(operatorPrimary) || !countFits(functions.Count()) || !countFits(captures) || !countFits(calls) || !countFits(directCalls) || !countFits(controlPrimary) || !countFits(genericFor) || !countFits(authored.Claims().Count()) || !countFits(authored.TypeValues().Count()) || !countFits(view.Outcomes().Count()) || !countFits(transfers) {
		return counts, errors.New("invalid Flow semantic cardinality")
	}
	return [...]int{
		valueCount, occurrences, access.Exact().Count() + access.Dynamic().Count(),
		storagePrimary, cells, globals, reads, assigns, writes, varargs, binds,
		tables, fields, operatorPrimary,
		candidates.Unary().NumericCount(), candidates.Unary().LengthCount(),
		candidates.Binary().ArithmeticCount(), candidates.Binary().BitwiseCount(), candidates.Binary().ConcatCount(), candidates.Binary().EqualityCount(), candidates.Binary().OrderCount(),
		candidates.Access().GetCount(), candidates.Access().SetCount(),
		functions.Count(), captures, calls, directCalls, controlPrimary, genericFor,
		authored.Claims().Count(), authored.TypeValues().Count(), view.Outcomes().Count(), transfers,
	}, nil
}

func programStaticCounts(view static.View) ([10]int, error) {
	var counts [10]int
	if !view.Available() {
		return counts, errors.New("unavailable Static view")
	}
	types, declarations, signatures, contracts, operators, operands := view.Types(), view.Declarations(), view.Signatures(), view.Contracts(), view.Operators(), view.Operands()
	primaryParts := []int{
		declarations.Aliases().Count(), declarations.Interfaces().Count(), declarations.TypeParams().Count(),
		types.Primitives().Count(), types.Literals().Count(), types.Optionals().Count(), types.Unions().Count(), types.Intersections().Count(), types.Generics().Count(), types.Arrays().Count(), types.Maps().Count(), types.Records().Count(),
		view.References().Count(), signatures.TypeFunctions().Count(), signatures.Assertions().Count(), operators.TypeOfs().Count(), operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count(),
	}
	primary, ok := sumMeasures(primaryParts...)
	if !ok {
		return counts, errors.New("Static primary cardinality overflow")
	}
	callArguments := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			return counts, errors.New("invalid Static call column")
		}
		argumentCount, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok || !addMeasure(&callArguments, argumentCount) {
			return counts, errors.New("invalid Static call-argument column")
		}
	}
	values := []int{primary, contracts.Functions().Count(), callArguments, declarations.DeclaredTypes().Count(), operands.Claims().Count(), operands.TypeValues().Count(), operators.TypeOfs().Count(), operands.Annotations().Count(), view.Publications().Count(), view.References().Count()}
	for _, value := range values {
		if !countFits(value) {
			return counts, errors.New("invalid Static semantic cardinality")
		}
	}
	return [...]int{values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8], values[9]}, nil
}

func programModuleCounts(view imports.View) ([6]int, error) {
	var counts [6]int
	if !view.ContentID().Available() {
		return counts, errors.New("unavailable Module view")
	}
	importsCount := view.Count()
	entry := view.Entry()
	returns := entry.ReturnCount()
	rootCells, rootFunctions, members := 0, 0, entry.MemberTotal()
	for index := 0; index < returns; index++ {
		returned, ok := entry.ReturnAt(index)
		if !ok {
			return counts, errors.New("invalid Module return column")
		}
		rootCount, ok := entry.RootCount(returned)
		if !ok {
			return counts, errors.New("invalid Module root column")
		}
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			if _, ok := entry.RootCell(returned, rootIndex); ok {
				rootCells++
			}
			if _, ok := entry.RootFunction(returned, rootIndex); ok {
				rootFunctions++
			}
		}
	}
	for _, value := range []int{importsCount, returns, rootCells, members, rootFunctions} {
		if !countFits(value) {
			return counts, errors.New("invalid Module semantic cardinality")
		}
	}
	return [...]int{importsCount, importsCount, returns, rootCells, members, rootFunctions}, nil
}

func countFits(value int) bool {
	return value >= 0 && uint64(value) <= uint64(keyspace.MaxTermOrdinal)
}

func addMeasure(total *int, value int) bool {
	if total == nil || value < 0 || *total < 0 || value > int(^uint(0)>>1)-*total {
		return false
	}
	*total += value
	return true
}

func sumMeasures(values ...int) (int, bool) {
	total := 0
	for _, value := range values {
		if !addMeasure(&total, value) {
			return 0, false
		}
	}
	return total, true
}
