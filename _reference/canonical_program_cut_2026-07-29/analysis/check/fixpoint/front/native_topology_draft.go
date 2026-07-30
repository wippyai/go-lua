package front

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NativeTopologyKind is the closed set of lowering descriptions consumed by
// the native publication kernel. A kind identifies topology, never a semantic
// result: the draft records below deliberately have no Value, Stable, Fresh,
// Exhaustive, Ownership, Completion, or equivalent conclusion field.
type NativeTopologyKind uint8

const (
	NativeTopologyCallGraph NativeTopologyKind = iota + 1
	NativeTopologyConstantCandidate
	NativeTopologyPublicationSite
	NativeTopologyRecordConstruction
	NativeTopologyShape
	NativeTopologyShapeEpoch
	NativeTopologyShapeTransition
	NativeTopologyDiscriminant
	NativeTopologyRecursiveType
	NativeTopologySummary
	NativeTopologyFunctionEntry
	NativeTopologyCallee
	NativeTopologyEffect
	NativeTopologyKernelOccurrence
)

// NativeTopologyDraft is a tagged union. Exactly one payload matching Kind is
// present. The union is intentionally explicit: a generic family/value map
// would let lowering smuggle a semantic conclusion through a string field.
type NativeTopologyDraft struct {
	Kind NativeTopologyKind `json:"kind"`

	CallGraph        *NativeCallGraphDraft          `json:"call_graph,omitempty"`
	Constant         *NativeConstantCandidateDraft  `json:"constant,omitempty"`
	Publication      *NativePublicationSiteDraft    `json:"publication,omitempty"`
	Record           *NativeRecordTopologyDraft     `json:"record,omitempty"`
	Shape            *NativeShapeTopologyDraft      `json:"shape,omitempty"`
	ShapeEpoch       *NativeShapeEpochTopologyDraft `json:"shape_epoch,omitempty"`
	ShapeTransition  *NativeShapeTransitionDraft    `json:"shape_transition,omitempty"`
	Discriminant     *NativeDiscriminantDraft       `json:"discriminant,omitempty"`
	Recursive        *NativeRecursiveTopologyDraft  `json:"recursive,omitempty"`
	Summary          *NativeSummaryTopologyDraft    `json:"summary,omitempty"`
	FunctionEntry    *NativeFunctionEntryDraft      `json:"function_entry,omitempty"`
	Callee           *NativeCalleeTopologyDraft     `json:"callee,omitempty"`
	Effect           *NativeEffectTopologyDraft     `json:"effect,omitempty"`
	KernelOccurrence *NativeKernelOccurrenceDraft   `json:"kernel_occurrence,omitempty"`
}

type NativeBodyReference struct {
	Body      [32]byte `json:"body"`
	Prototype uint64   `json:"prototype,omitempty"`
	Display   string   `json:"display,omitempty"`
}

type NativeInstructionReference struct {
	Body     [32]byte `json:"body"`
	Position uint32   `json:"position"`
}

type NativeSymbolReference struct {
	Term    string `json:"term,omitempty"`
	Display string `json:"display,omitempty"`
}

type NativeSpanDraft struct {
	StartLine uint32 `json:"start_line,omitempty"`
	StartCol  uint32 `json:"start_col,omitempty"`
	EndLine   uint32 `json:"end_line,omitempty"`
	EndCol    uint32 `json:"end_col,omitempty"`
}

type NativeCallEdgeDraft struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// NativeCallGraphDraft is the closed direct lexical graph. Components,
// completion states, and result exactness are intentionally absent; the
// publication kernel derives those conclusions from Edges and body schemas.
type NativeCallGraphDraft struct {
	Bodies []NativeBodyReference `json:"bodies"`
	Edges  []NativeCallEdgeDraft `json:"edges"`
	Types  []NativeBodyTypeDraft `json:"types,omitempty"`
}

type NativeBodyTypeDraft struct {
	Display        string `json:"display"`
	FirstParameter []byte `json:"first_parameter,omitempty"`
	ResultSlots    uint32 `json:"result_slots"`
}

type NativeConstantOperator uint8

const (
	NativeConstantAssign NativeConstantOperator = iota + 1
	NativeConstantNegate
	NativeConstantAdd
	NativeConstantSubtract
	NativeConstantMultiply
	NativeConstantFloorDivide
	NativeConstantModulo
)

type NativeOperandShape uint8

const (
	NativeOperandAbsent NativeOperandShape = iota
	NativeOperandSymbol
	NativeOperandTemporary
	NativeOperandLiteral
)

type NativeOperandDraft struct {
	Shape   NativeOperandShape `json:"shape"`
	Term    string             `json:"term,omitempty"`
	Literal []byte             `json:"literal,omitempty"`
}

// NativeConstantCandidateDraft describes an assignment-shaped instruction and
// the complete structural uses of its destination. Whether the candidate has
// constant provenance is derived after solve; WriteSites and CaptureSites are
// coordinates, not uniqueness/capture verdicts.
type NativeConstantCandidateDraft struct {
	Site         NativeInstructionReference   `json:"site"`
	Destination  NativeOperandDraft           `json:"destination"`
	Operator     NativeConstantOperator       `json:"operator"`
	Inputs       []NativeOperandDraft         `json:"inputs"`
	WriteSites   []NativeInstructionReference `json:"write_sites"`
	CaptureSites []NativeInstructionReference `json:"capture_sites,omitempty"`
}

type NativePublicationSiteDraft struct {
	Site NativeInstructionReference `json:"site"`
	Span NativeSpanDraft            `json:"span"`
}

type NativeRecordEntryDraft struct {
	Field        string                  `json:"field,omitempty"`
	Value        NativeOperandDraft      `json:"value"`
	ValueSpan    NativeSpanDraft         `json:"value_span"`
	ProducerSite uint32                  `json:"producer_site,omitempty"`
	ProducerOp   NativeProducerOperation `json:"producer_op,omitempty"`
}

type NativeProducerOperation uint8

const (
	NativeProducerTable NativeProducerOperation = iota + 1
	NativeProducerCall
	NativeProducerMultiply
	NativeProducerOther
)

// NativeRecordTopologyDraft carries constructor slots plus the structural
// alias/use coordinates that follow the constructor. It cannot state freshness,
// ownership, a storage carrier, overflow behavior, or evaluation order.
type NativeRecordTopologyDraft struct {
	Site         NativeInstructionReference   `json:"site"`
	Destination  NativeSymbolReference        `json:"destination"`
	Entries      []NativeRecordEntryDraft     `json:"entries"`
	AliasSites   []NativeInstructionReference `json:"alias_sites,omitempty"`
	MemberWrites []NativeInstructionReference `json:"member_writes,omitempty"`
	CallUses     []NativeInstructionReference `json:"call_uses,omitempty"`
	KeySlots     uint32                       `json:"key_slots"`
	EntrySlots   uint32                       `json:"entry_slots"`
}

type NativeShapeFieldDraft struct {
	Name     string `json:"name"`
	Readonly uint8  `json:"readonly,omitempty"`
	Optional uint8  `json:"optional,omitempty"`
}

type NativeShapeOrigin uint8

const (
	NativeShapeDeclaredRoot NativeShapeOrigin = iota + 1
	NativeShapeDeclaredReturn
	NativeShapeParameter
	NativeShapeClaim
	NativeShapeConstructor
	NativeShapeTransitionBefore
	NativeShapeTransitionAfter
)

// NativeShapeTopologyDraft is a physical shape candidate. The kernel decides
// whether the field set denotes one reusable identity and computes that ID.
type NativeShapeTopologyDraft struct {
	Body          [32]byte                `json:"body"`
	Origin        NativeShapeOrigin       `json:"origin"`
	Fields        []NativeShapeFieldDraft `json:"fields"`
	OpenParts     uint32                  `json:"open_parts,omitempty"`
	MapParts      uint32                  `json:"map_parts,omitempty"`
	MetatableRefs uint32                  `json:"metatable_refs,omitempty"`
	StaticMembers uint32                  `json:"static_members,omitempty"`
}

type NativeShapeEpochTopologyDraft struct {
	Body       [32]byte                `json:"body"`
	Receiver   NativeSymbolReference   `json:"receiver"`
	Fields     []NativeShapeFieldDraft `json:"fields"`
	ReadSites  []uint32                `json:"read_sites"`
	WriteSites []uint32                `json:"write_sites"`
}

type NativeShapeTransitionDraft struct {
	Body       [32]byte                `json:"body"`
	Site       uint32                  `json:"site"`
	Before     []NativeShapeFieldDraft `json:"before"`
	AddedField string                  `json:"added_field"`
}

type NativeDiscriminantCaseDraft struct {
	Ordinal uint32 `json:"ordinal"`
	Literal []byte `json:"literal"`
}

type NativeDiscriminantDraft struct {
	Body         [32]byte                      `json:"body"`
	Field        string                        `json:"field"`
	Cases        []NativeDiscriminantCaseDraft `json:"cases"`
	MatchedCases []uint32                      `json:"matched_cases,omitempty"`
	TruthySites  []uint32                      `json:"truthy_sites,omitempty"`
}

// NativeRecursiveTopologyDraft records the recursive type graph's structural
// record-node counts. Fixpoint/equality/stability/cache conclusions are absent.
type NativeRecursiveTopologyDraft struct {
	Body             [32]byte `json:"body"`
	RecordNodes      uint32   `json:"record_nodes"`
	CycleRecordNodes uint32   `json:"cycle_record_nodes"`
}

type NativeSummaryTopologyDraft struct {
	Body            NativeBodyReference     `json:"body"`
	Parameters      [][]byte                `json:"parameters,omitempty"`
	Results         [][]byte                `json:"results"`
	MutableCaptures []NativeSymbolReference `json:"mutable_captures,omitempty"`
}

type NativeReturnShapeDraft struct {
	Slots    uint32 `json:"slots"`
	OpenTail uint32 `json:"open_tail,omitempty"`
}

type NativeFunctionEntryDraft struct {
	Body       NativeBodyReference      `json:"body"`
	Parameters uint32                   `json:"parameters"`
	Varargs    uint32                   `json:"varargs,omitempty"`
	Returns    []NativeReturnShapeDraft `json:"returns,omitempty"`
	ErrorCalls []uint32                 `json:"error_calls,omitempty"`
}

type NativeCalleeTopology uint8

const (
	NativeCalleeDirectLexical NativeCalleeTopology = iota + 1
	NativeCalleeLocalAlternatives
	NativeCalleeParameter
	NativeCalleeLiteralMember
	NativeCalleeOpen
)

type NativeCalleeTopologyDraft struct {
	Body            NativeBodyReference  `json:"body"`
	Site            uint32               `json:"site"`
	Topology        NativeCalleeTopology `json:"topology"`
	TargetSymbols   []string             `json:"target_symbols,omitempty"`
	ModuleLoadSites []uint32             `json:"module_load_sites,omitempty"`
}

type NativeEffectOperation uint8

const (
	NativeEffectFunction NativeEffectOperation = iota + 1
	NativeEffectChannelSelect
	NativeEffectCoroutineYield
	NativeEffectCoroutineResume
	NativeEffectRegisteredSuspend
	NativeEffectStringGsub
	NativeEffectTableSort
	NativeEffectModuleLoad
	NativeEffectProtectedCall
	NativeEffectDirectLexicalCall
	NativeEffectOpenCall
)

type NativeEffectTopologyDraft struct {
	Body            NativeBodyReference   `json:"body"`
	Site            uint32                `json:"site,omitempty"`
	Operation       NativeEffectOperation `json:"operation"`
	ArgumentShapes  []NativeOperandShape  `json:"argument_shapes,omitempty"`
	OpenCallSites   []uint32              `json:"open_call_sites,omitempty"`
	ErrorCallSites  []uint32              `json:"error_call_sites,omitempty"`
	AllocationSites []uint32              `json:"allocation_sites,omitempty"`
}

type NativeKernelOccurrence uint8

const (
	NativeKernelEvalClosure NativeKernelOccurrence = iota + 1
	NativeKernelEvalLength
	NativeKernelClaimAssert
)

type NativeKernelOccurrenceDraft struct {
	Site      NativeInstructionReference `json:"site"`
	Operation NativeKernelOccurrence     `json:"operation"`
}

type nativeTopologyBundle struct {
	Version uint8                 `json:"version"`
	Drafts  []NativeTopologyDraft `json:"drafts"`
}

func EncodeNativeTopologyDrafts(drafts []NativeTopologyDraft) ([]byte, error) {
	bundle := nativeTopologyBundle{Version: 1, Drafts: drafts}
	for index := range bundle.Drafts {
		if err := validateNativeTopologyDraft(bundle.Drafts[index]); err != nil {
			return nil, fmt.Errorf("front: native topology draft %d: %w", index, err)
		}
	}
	return json.Marshal(bundle)
}

func DecodeNativeTopologyDrafts(encoded []byte) ([]NativeTopologyDraft, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var bundle nativeTopologyBundle
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("front: decode native topology drafts: %w", err)
	}
	if decoder.More() || bundle.Version != 1 {
		return nil, fmt.Errorf("front: malformed native topology draft bundle")
	}
	for index := range bundle.Drafts {
		if err := validateNativeTopologyDraft(bundle.Drafts[index]); err != nil {
			return nil, fmt.Errorf("front: native topology draft %d: %w", index, err)
		}
	}
	return bundle.Drafts, nil
}

func validateNativeTopologyDraft(draft NativeTopologyDraft) error {
	payloads := []bool{
		draft.CallGraph != nil, draft.Constant != nil, draft.Publication != nil,
		draft.Record != nil, draft.Shape != nil, draft.ShapeEpoch != nil,
		draft.ShapeTransition != nil, draft.Discriminant != nil,
		draft.Recursive != nil, draft.Summary != nil, draft.FunctionEntry != nil,
		draft.Callee != nil, draft.Effect != nil,
		draft.KernelOccurrence != nil,
	}
	count := 0
	for _, present := range payloads {
		if present {
			count++
		}
	}
	if count != 1 || draft.Kind == 0 || int(draft.Kind) > len(payloads) || !payloads[int(draft.Kind)-1] {
		return fmt.Errorf("kind/payload mismatch")
	}
	return nil
}
