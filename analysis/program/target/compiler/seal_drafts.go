package compiler

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

type operationDraft struct {
	source      int
	semantics   schematype.Semantics
	bindings    []vocabulary.BindingSpec
	formals     []vocabulary.TypeFormalSpec
	valuesVars  uint32
	valuesTypes []string
	rowFormals  uint32
	input       valuesDraft
	outcomes    []outcomeDraft
	behavior    behaviorDraft
	callbacks   []callbackDraft
	// Subedges remain authored vocabulary until the operation query boundary.
	// Target only freezes their Values/type and boot-key inputs; operation.Core
	// owns construction, canonical ordering, and every relation fence.
	subedges          []vocabulary.SubedgeSpec
	subedgeReadRoots  []vocabulary.InitialRoot
	suspensions       []suspensionDraft
	spawns            []spawnDraft
	resumes           []resumeDraft
	transfers         []transferDraft
	subedgeRelation   *vocabulary.SubedgeRelationSpec
	effects           []effectDraft
	effectTail        vocabulary.RowTail
	effectVar         vocabulary.RowVar
	formalEffects     []vocabulary.FormalEffectSpec
	formalEffectTail  vocabulary.RowTail
	types             map[string][]byte
	declarations      map[string]schematype.Type
	formalConstraints []schematype.Type
	constraints       []string
}

type behaviorDraft struct {
	results    []behaviorResultDraft
	predicates []behaviorPredicateDraft
}

type behaviorResultDraft struct {
	outcome  uint32
	result   uint32
	source   vocabulary.InputSource
	relation schema.EntryID
}

type behaviorPredicateDraft struct {
	outcome  uint32
	result   uint32
	subject  vocabulary.InputSource
	relation schema.EntryID
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
