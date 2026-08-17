package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type operationDraft struct {
	source            int
	semantics         schematype.Semantics
	bindings          []BindingSpec
	canonical         uint32
	formals           []TypeFormalSpec
	valuesVars        uint32
	valuesTypes       []string
	rowFormals        uint32
	input             valuesDraft
	outcomes          []outcomeDraft
	callbacks         []callbackDraft
	subedges          []subedgeDraft
	suspensions       []suspensionDraft
	spawns            []spawnDraft
	resumes           []resumeDraft
	transfers         []transferDraft
	gsubTable         *gsubTableReplacementDraft
	effects           []effectDraft
	effectTail        RowTail
	effectVar         RowVar
	types             map[string][]byte
	declarations      map[string]schematype.Type
	formalConstraints []schematype.Type
	constraints       []string
}

type valuesDraft struct {
	types    []string
	tail     ValuesTail
	varID    ValuesVar
	tailType string
	suffix   []string
}

type outcomeDraft struct {
	source          int
	kind            flowkind.OutcomeKind
	values          valuesDraft
	anchor          string
	produced        []producedDraft
	fresh           []freshResultDraft
	callbackResults []callbackResultDraft
	resultAliases   []resultAliasDraft
}

type suspensionDraft struct {
	yield        uint32
	reentry      uint32
	source       ReentrySource
	multiplicity ReentryMultiplicity
}

type spawnDraft struct {
	function     InputSource
	child        CallbackRef
	yield        uint32
	parentResume uint32
	childEntry   uint32
	alternatives [2]SpawnSiblingAlternative
}

type resumeDraft struct {
	source    ResumeSource
	carrier   ValueFormal
	arguments valuesDraft
	outcomes  [5]uint32
}

type transferDraft struct {
	endpoint     TransferEndpoint
	payload      InputSource
	alias        InputSource
	identity     TransferIdentity
	capabilities TransferCapabilities
	outcomes     []TransferPossibility
}

type callbackResultDraft struct {
	result   uint32
	callback CallbackRef
}

type resultAliasDraft struct {
	result uint32
	source InputSource
}

type callbackDraft struct {
	source    int
	function  InputSource
	admission schematype.CallableAdmission
	arguments valuesDraft
	outcomes  [5]valuesDraft
	lifecycle CallbackLifecycle
	effects   rowDraft
	release   *callbackReleaseDraft
	sealed    CallbackID
}

// subedgeDraft keeps only normalized Target-local structure. Authoring refs
// are resolved after stable callback/subedge identity ordering; no Go call or
// recursive execution occurs while sealing a cyclic edge graph.
type subedgeDraft struct {
	source           int
	role             uint32
	family           SubedgeFamily
	callee           SubedgeCalleeKind
	callback         CallbackRef
	callbackRank     uint32
	readRoot         string
	readKey          keyspace.LiteralValue
	readRootID       InitialRoot
	metaKey          keyspace.LiteralValue
	admission        schematype.CallableAdmission
	arguments        valuesDraft
	ruleEntry        bool
	argumentOrigins  []subedgeArgumentOriginDraft
	outcomes         [5]valuesDraft
	admissionFailure valuesDraft
	admissionRoute   subedgeRouteDraft
	routes           [5]subedgeRouteDraft
	sealed           SubedgeID
}

type subedgeArgumentOriginDraft struct {
	segment ArgumentSegment
	index   uint32
	kind    ArgumentSource
	source  InputSource
}

type subedgeRouteDraft struct {
	route       SubedgeRoute
	adjustment  Adjustment
	result      valuesDraft
	placement   Placement
	offset      uint32
	outcome     uint32
	subedge     SubedgeRef
	subedgeRank uint32
	destination valuesDraft
}

type callbackReleaseDraft struct {
	operationSource SpecRef
	operation       Operation
	input           ValueFormal
	outcome         uint32
	mode            CallbackReleaseMode
	zeroBehavior    CallbackReleaseZeroBehavior
	zeroOutcome     uint32
}

type producedDraft struct {
	result       uint32
	targetSource SpecRef
	target       Operation
	captures     []CaptureSpec
}

const noTypeValueCapture = ^uint32(0)

type freshResultDraft struct {
	result  uint32
	ordinal uint32
	kind    schematype.FreshClass
}

type producedAnchorStep struct {
	outcome string
	result  uint32
}

type effectDraft struct {
	targetSource   SpecRef
	target         Operation
	values         []ValueFormal
	types          []TypeFormal
	valuesVar      []ValuesVar
	rows           []RowVar
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type rowDraft struct {
	effects  []effectDraft
	tail     RowTail
	variable RowVar
}
