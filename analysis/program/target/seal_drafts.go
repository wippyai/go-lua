package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type operationDraft struct {
	source            int
	semantics         schematype.Semantics
	bindings          []vocabulary.BindingSpec
	canonical         uint32
	formals           []vocabulary.TypeFormalSpec
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
	subedgeRelation   *subedgeRelationDraft
	effects           []effectDraft
	effectTail        vocabulary.RowTail
	effectVar         vocabulary.RowVar
	types             map[string][]byte
	declarations      map[string]schematype.Type
	formalConstraints []schematype.Type
	constraints       []string
}

type valuesDraft struct {
	types    []string
	tail     vocabulary.ValuesTail
	varID    vocabulary.ValuesVar
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
	source       vocabulary.ReentrySource
	multiplicity vocabulary.ReentryMultiplicity
}

type spawnDraft struct {
	function     vocabulary.InputSource
	child        vocabulary.CallbackRef
	yield        uint32
	parentResume uint32
	childEntry   uint32
	alternatives [2]vocabulary.SpawnSiblingAlternative
}

type resumeDraft struct {
	source    vocabulary.ResumeSource
	carrier   vocabulary.ValueFormal
	arguments valuesDraft
	outcomes  [5]uint32
}

type transferDraft struct {
	endpoint     vocabulary.TransferEndpoint
	payload      vocabulary.InputSource
	alias        vocabulary.InputSource
	identity     vocabulary.TransferIdentity
	capabilities vocabulary.TransferCapabilities
	outcomes     []vocabulary.TransferPossibility
}

type subedgeRelationDraft struct {
	operand       vocabulary.ValueFormal
	selector      uint32
	subedgeSource vocabulary.SubedgeRef
	subedgeRank   uint32
	resultOutcome uint32
	result        uint32
	effects       []uint32
}

type callbackResultDraft struct {
	result   uint32
	callback vocabulary.CallbackRef
}

type resultAliasDraft struct {
	result uint32
	source vocabulary.InputSource
}

type callbackDraft struct {
	source    int
	function  vocabulary.InputSource
	admission schematype.CallableAdmission
	arguments valuesDraft
	outcomes  [5]valuesDraft
	lifecycle vocabulary.CallbackLifecycle
	effects   rowDraft
	release   *callbackReleaseDraft
	sealed    vocabulary.CallbackID
}

// subedgeDraft keeps only normalized Target-local structure. Authoring refs
// are resolved after stable callback/subedge identity ordering; no Go call or
// recursive execution occurs while sealing a cyclic edge graph.
type subedgeDraft struct {
	source           int
	role             uint32
	family           vocabulary.SubedgeFamily
	callee           vocabulary.SubedgeCalleeKind
	callback         vocabulary.CallbackRef
	callbackRank     uint32
	readRoot         string
	readKey          keyspace.LiteralValue
	readRootID       vocabulary.InitialRoot
	metaKey          keyspace.LiteralValue
	admission        schematype.CallableAdmission
	arguments        valuesDraft
	ruleEntry        bool
	argumentOrigins  []subedgeArgumentOriginDraft
	outcomes         [5]valuesDraft
	admissionFailure valuesDraft
	admissionRoute   subedgeRouteDraft
	routes           [5]subedgeRouteDraft
	sealed           vocabulary.SubedgeID
}

type subedgeArgumentOriginDraft struct {
	segment vocabulary.ArgumentSegment
	index   uint32
	kind    vocabulary.ArgumentSource
	source  vocabulary.InputSource
}

type subedgeRouteDraft struct {
	route       vocabulary.SubedgeRoute
	adjustment  vocabulary.Adjustment
	result      valuesDraft
	placement   vocabulary.Placement
	offset      uint32
	outcome     uint32
	subedge     vocabulary.SubedgeRef
	subedgeRank uint32
	destination valuesDraft
}

type callbackReleaseDraft struct {
	operationSource vocabulary.SpecRef
	operation       vocabulary.Operation
	input           vocabulary.ValueFormal
	outcome         uint32
	mode            vocabulary.CallbackReleaseMode
	zeroBehavior    vocabulary.CallbackReleaseZeroBehavior
	zeroOutcome     uint32
}

type producedDraft struct {
	result       uint32
	targetSource vocabulary.SpecRef
	target       vocabulary.Operation
	captures     []vocabulary.CaptureSpec
}

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
	targetSource   vocabulary.SpecRef
	target         vocabulary.Operation
	values         []vocabulary.ValueFormal
	types          []vocabulary.TypeFormal
	valuesVar      []vocabulary.ValuesVar
	rows           []vocabulary.RowVar
	publication    vocabulary.PublicationEffectSpec
	hasPublication bool
}

type rowDraft struct {
	effects  []effectDraft
	tail     vocabulary.RowTail
	variable vocabulary.RowVar
}

func (values valuesDraft) key() (string, error) {
	// Framed components prevent any concatenation ambiguity; this is cold seal
	// bookkeeping only and never a public binding or effect identity.
	parts := make([]int, 0)
	for _, typ := range values.types {
		if _, err := vocabulary.CheckedStoredLength("Values key type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	for _, typ := range values.suffix {
		if _, err := vocabulary.CheckedStoredLength("Values key suffix type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	if _, err := vocabulary.CheckedStoredLength("Values key tail type bytes", len(values.tailType)); err != nil {
		return "", err
	}
	parts = append(parts, 4, 1, 4, 4, len(values.tailType))
	total, err := vocabulary.CheckedStoredTotal("Values key", parts...)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, total)
	for _, typ := range values.types {
		length, lengthErr := vocabulary.CheckedStoredLength("Values key type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	out = appendUint32(out, ^uint32(0))
	out = append(out, byte(values.tail))
	out = appendUint32(out, uint32(values.varID))
	out = appendUint32(out, uint32(len(values.tailType)))
	out = append(out, values.tailType...)
	for _, typ := range values.suffix {
		length, lengthErr := vocabulary.CheckedStoredLength("Values key suffix type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	return string(out), nil
}

func appendUint32(out []byte, value uint32) []byte {
	return append(out, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}
