package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type queryState struct {
	operations    []queryOperationRow
	types         []queryTypeRow
	values        []queryValuesRow
	effects       []queryEffectRow
	transfers     []queryTransferRow
	transferEnds  []vocabulary.TransferPossibility
	outcomeRows   []queryOutcomeRow
	behaviorRows  []queryBehaviorResultRow
	predicateRows []queryBehaviorPredicateRow
}

type queryOperationRow struct {
	input              vocabulary.Values
	outcomes           queryRange
	behavior           queryRange
	behaviorPredicates queryRange
	valuesTypes        []vocabulary.Type
	transfers          queryRange
	effects            []int
	typeFormals        []vocabulary.Type
	rowFormals         uint32
	effectTail         vocabulary.RowTail
	effectVar          vocabulary.RowVar
}

type queryRange struct{ start, end int }

func (r queryRange) len() int {
	if r.end < r.start {
		return 0
	}
	return r.end - r.start
}

type queryTypeRow struct {
	declaration schematype.Type
}

type queryValuesRow struct {
	owner  vocabulary.Operation
	types  []vocabulary.Type
	tail   vocabulary.ValuesTail
	varID  vocabulary.ValuesVar
	suffix []vocabulary.Type
}

type queryOutcomeRow struct {
	kind   flowkind.OutcomeKind
	values vocabulary.Values
}

type queryBehaviorResultRow struct {
	outcome  uint32
	result   uint32
	source   vocabulary.InputSource
	relation schema.EntryID
}

type queryBehaviorPredicateRow struct {
	outcome  uint32
	result   uint32
	subject  vocabulary.InputSource
	relation schema.EntryID
}

type queryTransferRow struct {
	owner        vocabulary.Operation
	endpoint     vocabulary.TransferEndpoint
	payload      vocabulary.InputSource
	alias        vocabulary.InputSource
	identity     vocabulary.TransferIdentity
	capabilities vocabulary.TransferCapabilities
	outcomes     queryRange
}

type queryEffectRow struct {
	target         vocabulary.Operation
	values         []vocabulary.ValueFormal
	types          []vocabulary.TypeFormal
	valuesVar      []vocabulary.ValuesVar
	rows           []vocabulary.RowVar
	publication    vocabulary.PublicationEffectSpec
	hasPublication bool
}

func invalidQuery(message string) error { return queryError(message) }

type queryError string

func (err queryError) Error() string { return "target/operation: " + string(err) }

func validQueryRange(values []int, bound int) bool {
	for _, value := range values {
		if value < 0 || value >= bound {
			return false
		}
	}
	return true
}

func (state queryState) validValues(handle vocabulary.Values) bool {
	return handle != 0 && int(handle) <= len(state.values)
}

func validQueryType(handle vocabulary.Type, count int) bool {
	return handle != 0 && int(handle) <= count
}
