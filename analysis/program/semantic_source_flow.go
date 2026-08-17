package program

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

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
		if !ok || !addProgramSemanticSourceMeasure(&occurrences, fixed) {
			return counts, errors.New("invalid Flow value occurrence column")
		}
		_, tail, ok := values.Get(term)
		if !ok {
			return counts, errors.New("invalid Flow value row")
		}
		if tail != 0 && !addProgramSemanticSourceMeasure(&occurrences, 1) {
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
	storagePrimary, ok := sumProgramSemanticSourceMeasures(cells, globals, reads, assigns, writes, varargs, binds)
	if !ok {
		return counts, errors.New("Flow storage cardinality overflow")
	}
	tables, fields := authored.Tables().Count(), authored.Fields().Count()
	operators := authored.Operators()
	unaries, binaries, selects := operators.Unaries().Count(), operators.Binaries().Count(), operators.Selects().Count()
	operatorPrimary, ok := sumProgramSemanticSourceMeasures(unaries, binaries, selects)
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
		if !ok || !addProgramSemanticSourceMeasure(&captures, captureCount) {
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
	controlPrimary, ok := sumProgramSemanticSourceMeasures(returns, breaks, labels, gotos, branches, loops)
	if !ok {
		return counts, errors.New("Flow control cardinality overflow")
	}
	transfers := view.Causal().Edges().Count()
	if !programSemanticSourceCountsFit(valueCount, occurrences, storagePrimary, cells, globals, reads, assigns, writes, varargs, binds, tables, fields, operatorPrimary, functions.Count(), captures, calls, directCalls, controlPrimary, genericFor, authored.Claims().Count(), authored.TypeValues().Count(), view.Outcomes().Count(), transfers) {
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
