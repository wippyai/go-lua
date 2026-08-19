package operation

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type queryState struct {
	operations      []queryOperationRow
	callbacks       []queryCallbackRow
	types           []queryTypeRow
	values          []queryValuesRow
	effects         []queryEffectRow
	transfers       []queryTransferRow
	transferEnds    []vocabulary.TransferPossibility
	outcomeRows     []queryOutcomeRow
	produced        []queryProducedRow
	captures        []queryCaptureRow
	fresh           []queryFreshRow
	callbackResults []queryCallbackResultRow
	resultAliases   []queryResultAliasRow
	suspensions     []querySuspensionRow
	spawns          []querySpawnRow
	resumes         []queryResumeRow
	behaviorRows    []queryBehaviorResultRow
	predicateRows   []queryBehaviorPredicateRow
}

type queryOperationRow struct {
	input              vocabulary.Values
	outcomes           queryRange
	suspensions        queryRange
	spawns             queryRange
	resumes            queryRange
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

// queryCallbackRow is the callback-owned side of the operation effect plane.
// Callback IDs and their owner are issued by Geometry; this row retains only
// the sealed expected-effect range and row schema for that callback.
type queryCallbackRow struct {
	owner      vocabulary.Operation
	effects    queryRange
	effectTail vocabulary.RowTail
	effectVar  vocabulary.RowVar
	published  bool
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
	kind            flowkind.OutcomeKind
	values          vocabulary.Values
	produced        queryRange
	fresh           queryRange
	callbackResults queryRange
	resultAliases   queryRange
}

type queryProducedRow struct {
	result           uint32
	target           vocabulary.Operation
	captures         queryRange
	typeValueCapture uint32
}

type queryCaptureRow struct {
	kind    vocabulary.CaptureKind
	ordinal uint32
}

type queryFreshRow struct {
	result  uint32
	ordinal uint32
	kind    schematype.FreshClass
}

type queryCallbackResultRow struct {
	result   uint32
	callback vocabulary.CallbackID
}

type queryResultAliasRow struct {
	result uint32
	source vocabulary.InputSource
}

type querySuspensionRow struct {
	yield        uint32
	reentry      uint32
	source       vocabulary.ReentrySource
	multiplicity vocabulary.ReentryMultiplicity
}

type querySpawnRow struct {
	owner        vocabulary.Operation
	function     vocabulary.InputSource
	child        vocabulary.CallbackID
	yield        uint32
	parentResume uint32
	childEntry   vocabulary.Values
	resumeValues vocabulary.Values
	alternatives [2]vocabulary.SpawnSiblingAlternative
}

type queryResumeRow struct {
	owner     vocabulary.Operation
	source    vocabulary.ResumeSource
	carrier   vocabulary.ValueFormal
	arguments vocabulary.Values
	outcomes  [5]uint32
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
	publication    PublicationEffectDescriptor
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
