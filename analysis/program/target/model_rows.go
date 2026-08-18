package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type operationRow struct {
	bindings        indexRange
	input           vocabulary.Values
	outcomes        indexRange
	valuesTypes     indexRange
	callbacks       indexRange
	subedges        indexRange
	suspensions     indexRange
	spawns          indexRange
	resumes         indexRange
	transfers       indexRange
	subedgeRelation uint32
	releases        indexRange
	effects         indexRange
	typeFormals     indexRange
	valuesVars      uint32
	rowFormals      uint32
	effectTail      vocabulary.RowTail
	effectVar       vocabulary.RowVar
}

type bindingRange struct {
	namespace  vocabulary.BindingNamespace
	owner      indexRange
	member     indexRange
	ownerKeys  indexRange
	memberKeys indexRange
}

type initialRootRow struct {
	identity string
	shape    vocabulary.BootShape
}

type bootShapeRow struct {
	root      vocabulary.InitialRoot
	aggregate vocabulary.BootAggregate
	immutable bool
	value     vocabulary.InitialValue
}

type initialValueRow struct {
	kind      vocabulary.InitialValueKind
	boolean   bool
	integer   int64
	floatBits uint64
	string    string
	root      vocabulary.InitialRoot
	operation vocabulary.Operation
	binding   uint32
}

type initialEntryRow struct {
	root       vocabulary.InitialRoot
	key        vocabulary.ExactKey
	value      vocabulary.InitialValue
	mutability vocabulary.InitialMutability
}

type initialBindingRow struct {
	name string
	root vocabulary.InitialRoot
	key  vocabulary.ExactKey
}

type initialMetatableAttachmentRow struct {
	base      vocabulary.InitialValueKind
	metatable vocabulary.InitialRoot
}

type valuesRow struct {
	owner  vocabulary.Operation
	types  indexRange
	tail   vocabulary.ValuesTail
	varID  vocabulary.ValuesVar
	suffix indexRange
}

type typeRow struct {
	owner       vocabulary.Operation
	declaration schematype.Type
	bytes       []byte
}

type outcomeRow struct {
	kind            flowkind.OutcomeKind
	values          vocabulary.Values
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
	callback vocabulary.CallbackID
}

type resultAliasRow struct {
	result uint32
	source vocabulary.InputSource
}

type suspensionRow struct {
	yield        uint32
	reentry      uint32
	source       vocabulary.ReentrySource
	multiplicity vocabulary.ReentryMultiplicity
}

type spawnRow struct {
	owner        vocabulary.Operation
	function     vocabulary.InputSource
	child        vocabulary.CallbackID
	yield        uint32
	parentResume uint32
	childEntry   vocabulary.Values
	resumeValues vocabulary.Values
	alternatives [2]vocabulary.SpawnSiblingAlternative
}

type resumeRow struct {
	owner     vocabulary.Operation
	source    vocabulary.ResumeSource
	carrier   vocabulary.ValueFormal
	arguments vocabulary.Values
	outcomes  [5]uint32
}

type transferRow struct {
	owner        vocabulary.Operation
	endpoint     vocabulary.TransferEndpoint
	payload      vocabulary.InputSource
	alias        vocabulary.InputSource
	identity     vocabulary.TransferIdentity
	capabilities vocabulary.TransferCapabilities
	outcomes     indexRange
}

type subedgeRelationRow struct {
	operand       vocabulary.ValueFormal
	selector      uint32
	subedge       vocabulary.SubedgeID
	resultOutcome uint32
	result        uint32
	effects       indexRange
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
	operation vocabulary.Operation
	outcome   uint32
	result    uint32
	state     vocabulary.State
}

type transitionRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
	from      vocabulary.State
	outcomes  indexRange
}

type transitionOutcomeRow struct {
	outcome uint32
	to      vocabulary.State
}

type escapeRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
}

type protocolCallbackHolderRow struct {
	operation vocabulary.Operation
	input     vocabulary.InputSource
	callback  vocabulary.CallbackID
}

type callbackRow struct {
	owner      vocabulary.Operation
	function   vocabulary.InputSource
	admission  schematype.CallableAdmission
	arguments  vocabulary.Values
	outcomes   [5]vocabulary.Values
	lifecycle  vocabulary.CallbackLifecycle
	subedge    vocabulary.SubedgeID
	effects    indexRange
	effectTail vocabulary.RowTail
	effectVar  vocabulary.RowVar
	release    uint32
}

type subedgeRow struct {
	owner            vocabulary.Operation
	role             uint32
	family           vocabulary.SubedgeFamily
	callee           vocabulary.SubedgeCalleeKind
	callback         vocabulary.CallbackID
	readRoot         vocabulary.InitialRoot
	readKey          vocabulary.ExactKey
	metaKey          vocabulary.ExactKey
	admission        schematype.CallableAdmission
	arguments        vocabulary.Values
	ruleEntry        bool
	argumentOrigins  indexRange
	outcomes         [5]vocabulary.Values
	admissionFailure vocabulary.Values
	admissionRoute   subedgeRouteRow
	routes           [5]subedgeRouteRow
}

type subedgeArgumentOriginRow struct {
	segment vocabulary.ArgumentSegment
	index   uint32
	kind    vocabulary.ArgumentSource
	source  vocabulary.InputSource
}

type subedgeRouteRow struct {
	route       vocabulary.SubedgeRoute
	adjustment  vocabulary.Adjustment
	result      vocabulary.Values
	placement   vocabulary.Placement
	offset      uint32
	outcome     uint32
	subedge     vocabulary.SubedgeID
	destination vocabulary.Values
}

type callbackReleaseRow struct {
	callback     vocabulary.CallbackID
	operation    vocabulary.Operation
	input        vocabulary.ValueFormal
	outcome      uint32
	mode         vocabulary.CallbackReleaseMode
	zeroBehavior vocabulary.CallbackReleaseZeroBehavior
	zeroOutcome  uint32
}

type producedRow struct {
	result           uint32
	target           vocabulary.Operation
	captures         indexRange
	typeValueCapture uint32 // relative capture index; noTypeValueCapture when absent
}

type captureRow struct {
	kind    vocabulary.CaptureKind
	ordinal uint32
}

// effectOwner discriminates the owner of one row in the flat effect table. The
// table is indexed by both operationRow.effects and callbackRow.effects, so the
// owner is what tells the two populations apart without walking either index.
type effectOwner uint8

const (
	effectOwnerOperation effectOwner = iota + 1
	effectOwnerCallback
)

type effectRow struct {
	owner          effectOwner
	target         vocabulary.Operation
	values         indexRange
	types          indexRange
	valuesVar      indexRange
	rows           indexRange
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type indexRange struct{ start, end uint32 }

func (r indexRange) len() int { return int(r.end - r.start) }

type bindingIndexRow struct {
	binding   uint32
	operation vocabulary.Operation
}

// Contract is immutable after Seal. Every slice is private and every public
// hot query returns only scalar handles or values.
type Contract struct {
	operations             []operationRow
	types                  []typeRow
	values                 []valuesRow
	valueTypes             []vocabulary.Type
	outcomes               []outcomeRow
	effects                []effectRow
	effectVals             []vocabulary.ValueFormal
	effectType             []vocabulary.TypeFormal
	effectVars             []vocabulary.ValuesVar
	valuesVarTypes         []vocabulary.Type
	effectRows             []vocabulary.RowVar
	formals                []vocabulary.Type
	bindings               []bindingRange
	callbacks              []callbackRow
	subedges               []subedgeRow
	subedgeOrigins         []subedgeArgumentOriginRow
	callbackResults        []callbackResultRow
	resultAliases          []resultAliasRow
	suspensions            []suspensionRow
	spawns                 []spawnRow
	resumes                []resumeRow
	transfers              []transferRow
	subedgeRelations       []subedgeRelationRow
	subedgeRelationEffects []uint32
	transferOutcomes       []vocabulary.TransferPossibility
	callbackReleases       []callbackReleaseRow
	protocols              []protocolRow
	states                 []stateRow
	acquisitions           []acquisitionRow
	transitions            []transitionRow
	transitionOutcomes     []transitionOutcomeRow
	escapes                []escapeRow
	callbackHolders        []protocolCallbackHolderRow
	produced               []producedRow
	fresh                  []freshResultRow
	captures               []captureRow
	segments               []string
	bindingKeys            []vocabulary.ExactKey
	lookup                 []bindingIndexRow
	initialRoots           []initialRootRow
	exactKeys              []keyspace.LiteralValue
	bootShapes             []bootShapeRow
	initialValues          []initialValueRow
	initialValueBinds      []bindingRange
	initialEntries         []initialEntryRow
	initialBindings        []initialBindingRow
	initialMetatables      []initialMetatableAttachmentRow
	globalEnvRoot          vocabulary.InitialRoot
	initialAbsent          vocabulary.InitialValue
	counts                 denominator.CountRows
	// identityColumns carries the identity plane's own columns. The layout is
	// declared with the rest of the model; the values are written and read only
	// by the identity altitude.
	identityColumns
	boundCount uint32
	opaque     vocabulary.Operation
	sealed     bool
}

// identityColumns are the content identities the identity altitude computes
// over the published read surface and seals with the contract. They are not a
// second graph authority: each row is a cached canonical descriptor indexed
// only by the existing dense Target tables.
type identityColumns struct {
	operationAnchors []identity.ContentID
	// Effect identity columns are projections of the existing
	// operation/callback/effect tables. Effect descriptors intentionally have
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
}

type inputFormalIDRow struct {
	id     identity.ContentID
	op     vocabulary.Operation
	formal vocabulary.ValueFormal
}

type outcomeResultIDRow struct {
	id      identity.ContentID
	op      vocabulary.Operation
	outcome uint32
	result  uint32
}

// callbackContentIDRow and resumeContentIDRow are the immutable sorted
// reverse columns for the Target-owned portable relation identities. The
// forward columns remain dense by their existing sealed handles; these rows
// retain no authoring ordinal or secondary lookup map.
type callbackContentIDRow struct {
	id       identity.ContentID
	op       vocabulary.Operation
	callback vocabulary.CallbackID
}

type resumeContentIDRow struct {
	id     identity.ContentID
	op     vocabulary.Operation
	resume vocabulary.ResumeID
}
