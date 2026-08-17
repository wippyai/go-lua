package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type operationRow struct {
	bindings    indexRange
	input       Values
	outcomes    indexRange
	valuesTypes indexRange
	callbacks   indexRange
	subedges    indexRange
	suspensions indexRange
	spawns      indexRange
	resumes     indexRange
	transfers   indexRange
	gsubTable   uint32
	releases    indexRange
	effects     indexRange
	typeFormals indexRange
	valuesVars  uint32
	rowFormals  uint32
	effectTail  RowTail
	effectVar   RowVar
}

type bindingRange struct {
	namespace  BindingNamespace
	owner      indexRange
	member     indexRange
	ownerKeys  indexRange
	memberKeys indexRange
}

type initialRootRow struct {
	identity string
	shape    BootShape
}

type bootShapeRow struct {
	root      InitialRoot
	aggregate BootAggregate
	immutable bool
	value     InitialValue
}

type initialValueRow struct {
	kind      InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      InitialRoot
	operation Operation
	binding   uint32
}

type initialEntryRow struct {
	root       InitialRoot
	key        ExactKey
	value      InitialValue
	mutability InitialMutability
}

type initialBindingRow struct {
	name string
	root InitialRoot
	key  ExactKey
}

type initialMetatableAttachmentRow struct {
	base      InitialValueKind
	metatable InitialRoot
}

type valuesRow struct {
	owner  Operation
	types  indexRange
	tail   ValuesTail
	varID  ValuesVar
	suffix indexRange
}

type typeRow struct {
	owner       Operation
	declaration schematype.Type
	bytes       []byte
}

type outcomeRow struct {
	kind            flowkind.OutcomeKind
	values          Values
	produced        indexRange
	fresh           indexRange
	callbackResults indexRange
	resultAliases   indexRange
}

type freshResultRow struct {
	result  uint32
	ordinal uint32
	kind    schematype.FreshClass
}

type callbackResultRow struct {
	result   uint32
	callback CallbackID
}

type resultAliasRow struct {
	result uint32
	source InputSource
}

type suspensionRow struct {
	yield        uint32
	reentry      uint32
	source       ReentrySource
	multiplicity ReentryMultiplicity
}

type spawnRow struct {
	owner        Operation
	function     InputSource
	child        CallbackID
	yield        uint32
	parentResume uint32
	childEntry   Values
	resumeValues Values
	alternatives [2]SpawnSiblingAlternative
}

type resumeRow struct {
	owner     Operation
	source    ResumeSource
	carrier   ValueFormal
	arguments Values
	outcomes  [5]uint32
}

type transferRow struct {
	owner        Operation
	endpoint     TransferEndpoint
	payload      InputSource
	alias        InputSource
	identity     TransferIdentity
	capabilities TransferCapabilities
	outcomes     indexRange
}

type protocolRow struct {
	acquisitions    indexRange
	states          indexRange
	transitions     indexRange
	escapes         indexRange
	callbackHolders indexRange
}

type stateRow struct {
	name  string
	final bool
}

type acquisitionRow struct {
	operation Operation
	outcome   uint32
	result    uint32
	state     State
}

type transitionRow struct {
	operation Operation
	input     InputSource
	from      State
	outcomes  indexRange
}

type transitionOutcomeRow struct {
	outcome uint32
	to      State
}

type escapeRow struct {
	operation Operation
	input     InputSource
}

type protocolCallbackHolderRow struct {
	operation Operation
	input     InputSource
	callback  CallbackID
}

type callbackRow struct {
	owner      Operation
	function   InputSource
	admission  schematype.CallableAdmission
	arguments  Values
	outcomes   [5]Values
	lifecycle  CallbackLifecycle
	subedge    SubedgeID
	effects    indexRange
	effectTail RowTail
	effectVar  RowVar
	release    uint32
}

type subedgeRow struct {
	owner            Operation
	role             uint32
	family           SubedgeFamily
	callee           SubedgeCalleeKind
	callback         CallbackID
	readRoot         InitialRoot
	readKey          ExactKey
	metaKey          ExactKey
	admission        schematype.CallableAdmission
	arguments        Values
	ruleEntry        bool
	argumentOrigins  indexRange
	outcomes         [5]Values
	admissionFailure Values
	admissionRoute   subedgeRouteRow
	routes           [5]subedgeRouteRow
}

type subedgeArgumentOriginRow struct {
	segment ArgumentSegment
	index   uint32
	kind    ArgumentSource
	source  InputSource
}

type subedgeRouteRow struct {
	route       SubedgeRoute
	adjustment  Adjustment
	result      Values
	placement   Placement
	offset      uint32
	outcome     uint32
	subedge     SubedgeID
	destination Values
}

type callbackReleaseRow struct {
	callback     CallbackID
	operation    Operation
	input        ValueFormal
	outcome      uint32
	mode         CallbackReleaseMode
	zeroBehavior CallbackReleaseZeroBehavior
	zeroOutcome  uint32
}

type producedRow struct {
	result           uint32
	target           Operation
	captures         indexRange
	typeValueCapture uint32 // relative capture index; noTypeValueCapture when absent
}

type captureRow struct {
	kind    CaptureKind
	ordinal uint32
}

type effectRow struct {
	target         Operation
	values         indexRange
	types          indexRange
	valuesVar      indexRange
	rows           indexRange
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type indexRange struct{ start, end uint32 }

type bindingIndexRow struct {
	binding   uint32
	operation Operation
}

// Contract is immutable after Seal. Every slice is private and every public
// hot query returns only scalar handles or values.
type Contract struct {
	operations         []operationRow
	types              []typeRow
	values             []valuesRow
	valueTypes         []Type
	outcomes           []outcomeRow
	effects            []effectRow
	effectVals         []ValueFormal
	effectType         []TypeFormal
	effectVars         []ValuesVar
	valuesVarTypes     []Type
	effectRows         []RowVar
	formals            []Type
	bindings           []bindingRange
	callbacks          []callbackRow
	subedges           []subedgeRow
	subedgeOrigins     []subedgeArgumentOriginRow
	callbackResults    []callbackResultRow
	resultAliases      []resultAliasRow
	suspensions        []suspensionRow
	spawns             []spawnRow
	resumes            []resumeRow
	transfers          []transferRow
	gsubTables         []gsubTableReplacementRow
	gsubEffects        []uint32
	transferOutcomes   []TransferPossibility
	callbackReleases   []callbackReleaseRow
	protocols          []protocolRow
	states             []stateRow
	acquisitions       []acquisitionRow
	transitions        []transitionRow
	transitionOutcomes []transitionOutcomeRow
	escapes            []escapeRow
	callbackHolders    []protocolCallbackHolderRow
	produced           []producedRow
	fresh              []freshResultRow
	captures           []captureRow
	segments           []string
	bindingKeys        []ExactKey
	lookup             []bindingIndexRow
	initialRoots       []initialRootRow
	exactKeys          []keyspace.LiteralValue
	bootShapes         []bootShapeRow
	initialValues      []initialValueRow
	initialValueBinds  []bindingRange
	initialEntries     []initialEntryRow
	initialBindings    []initialBindingRow
	initialMetatables  []initialMetatableAttachmentRow
	globalEnvRoot      InitialRoot
	initialAbsent      InitialValue
	sourceViews        SourceViews
	// semantic identity columns are sealed with the contract.  They are not a
	// second graph authority: each row is a cached canonical descriptor owned
	// by Target and indexed only by the existing dense Target tables.
	operationAnchors []identity.ContentID
	// Effect identity columns are likewise only projections of the existing
	// operation/callback/effect tables.  Effect descriptors intentionally have
	// no inverse index: duplicate authored occurrences are distinct evidence,
	// while their descriptor identity is the shared semantic quotient.
	effectOperationIDs      []identity.ContentID
	effectDescriptorIDs     []identity.ContentID
	effectOccurrenceIDs     []identity.ContentID
	operationEffectFamilies []identity.ContentID
	callbackEffectFamilies  []identity.ContentID
	operationContentIDs     []identity.ContentID
	callbackSelectors       []identity.ContentID
	callbackContentIDs      []identity.ContentID
	callbackContentIndex    []callbackContentIDRow
	outcomeSelectors        []identity.ContentID
	outcomeContentIDs       []identity.ContentID
	transferContentIDs      []identity.ContentID
	transferOutcomeIDs      []identity.ContentID
	resumeContentIDs        []identity.ContentID
	resumeContentIndex      []resumeContentIDRow
	inputFormalRanges       []indexRange
	inputFormalIDs          []identity.ContentID
	inputFormalIndex        []inputFormalIDRow
	outcomeResultRanges     []indexRange
	outcomeResultIDs        []identity.ContentID
	outcomeResultIndex      []outcomeResultIDRow
	initialValueContentIDs  []identity.ContentID
	bootRelationID          identity.ContentID
	boundCount              uint32
	opaque                  Operation
	sealed                  bool
}

type inputFormalIDRow struct {
	id     identity.ContentID
	op     Operation
	formal ValueFormal
}

type outcomeResultIDRow struct {
	id      identity.ContentID
	op      Operation
	outcome uint32
	result  uint32
}

// callbackContentIDRow and resumeContentIDRow are the immutable sorted
// reverse columns for the Target-owned portable relation identities. The
// forward columns remain dense by their existing sealed handles; these rows
// retain no authoring ordinal or secondary lookup map.
type callbackContentIDRow struct {
	id       identity.ContentID
	op       Operation
	callback CallbackID
}

type resumeContentIDRow struct {
	id     identity.ContentID
	op     Operation
	resume ResumeID
}
