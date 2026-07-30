package factkey

import (
	"bytes"
	"strconv"
	"strings"
)

// Truth is the closed marker-payload domain. Unknown is returned for an
// absent or malformed payload; Proven and Refuted are the two wire states.
type Truth uint8

const (
	TruthUnknown Truth = iota
	TruthProven
	TruthRefuted
)

// Fact payloads are immutable after publication, so the two closed truth
// spellings can share codec-owned storage across every producer.
var (
	truthProvenWire  = []byte("proven")
	truthRefutedWire = []byte("refuted")
)

// EncodeTruth is the sole marker-payload wire constructor.
func EncodeTruth(truth Truth) []byte {
	switch truth {
	case TruthProven:
		return truthProvenWire
	case TruthRefuted:
		return truthRefutedWire
	default:
		return nil
	}
}

// DecodeTruth fails closed to TruthUnknown for every non-domain payload.
func DecodeTruth(payload []byte) Truth {
	switch {
	case bytes.Equal(payload, truthProvenWire):
		return TruthProven
	case bytes.Equal(payload, truthRefutedWire):
		return TruthRefuted
	default:
		return TruthUnknown
	}
}

// DecodeTruthString is the string-storage counterpart used by output adapters
// after a fact value has already been projected to text.
func DecodeTruthString(payload string) Truth {
	switch payload {
	case "proven":
		return TruthProven
	case "refuted":
		return TruthRefuted
	default:
		return TruthUnknown
	}
}

// FreezeKind is the closed effect.freeze payload domain.
type FreezeKind uint8

const (
	FreezeInvalid FreezeKind = iota
	FreezeUnconditional
	FreezeGuarded
)

// FreezePayload states whether a freeze is unconditional or belongs to one
// branch edge. A guarded payload is valid only with a non-empty guard name.
type FreezePayload struct {
	Kind  FreezeKind
	Guard string
	Edge  bool
}

func EncodeFreezePayload(payload FreezePayload) []byte {
	switch payload.Kind {
	case FreezeUnconditional:
		return []byte("unconditional")
	case FreezeGuarded:
		if payload.Guard == "" {
			return nil
		}
		return []byte("guard/" + payload.Guard + "/" + strconv.FormatBool(payload.Edge))
	default:
		return nil
	}
}

func DecodeFreezePayload(encoded []byte) (FreezePayload, bool) {
	if bytes.Equal(encoded, []byte("unconditional")) {
		return FreezePayload{Kind: FreezeUnconditional}, true
	}
	rest, ok := strings.CutPrefix(string(encoded), "guard/")
	if !ok {
		return FreezePayload{}, false
	}
	guard, edge, ok := strings.Cut(rest, "/")
	if !ok || guard == "" || edge != "true" && edge != "false" {
		return FreezePayload{}, false
	}
	return FreezePayload{Kind: FreezeGuarded, Guard: guard, Edge: edge == "true"}, true
}

// NativeCallSCCPayload is the typed call_scc publication schema.
type NativeCallSCCPayload struct {
	Arguments   string
	Edges       []string
	Members     []string
	ResultSlots uint32
}

func EncodeNativeCallSCCPayload(payload NativeCallSCCPayload) string {
	var encoded strings.Builder
	encoded.Grow(192 + len(payload.Arguments))
	encoded.WriteString("arguments=")
	encoded.WriteString(payload.Arguments)
	encoded.WriteString(" completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} edges_closed=[")
	writeJoined(&encoded, payload.Edges)
	encoded.WriteString("] members=[")
	writeJoined(&encoded, payload.Members)
	encoded.WriteString("] results={'exact': True, 'count': ")
	writeUint(&encoded, uint64(payload.ResultSlots), 10)
	encoded.WriteByte('}')
	return encoded.String()
}

type NativeShapeIdentityKind uint8

const (
	NativeShapeIdentityInvalid NativeShapeIdentityKind = iota
	NativeShapeIdentityStable
	NativeShapeIdentityFieldRead
	NativeShapeIdentityTransition
)

// NativeShapeIdentityPayload is the closed shape_identity payload schema.
type NativeShapeIdentityPayload struct {
	Kind    NativeShapeIdentityKind
	ShapeID uint64
}

func EncodeNativeShapeIdentityPayload(payload NativeShapeIdentityPayload) string {
	var encoded strings.Builder
	encoded.Grow(160)
	switch payload.Kind {
	case NativeShapeIdentityStable:
		encoded.WriteString("distinct_identities=1 field_offsets=identical field_order=canonical interned=true shape_id=")
	case NativeShapeIdentityFieldRead:
		encoded.WriteString("epoch=field_read field_offsets=identical interned=true shape_id=")
	case NativeShapeIdentityTransition:
		encoded.WriteString("field_offsets=identical field_order=canonical interned=true shape_id=")
	default:
		return ""
	}
	writeHex16(&encoded, payload.ShapeID)
	switch payload.Kind {
	case NativeShapeIdentityStable:
		encoded.WriteString(" stable_across_modules=true stable_across_sites=true")
	case NativeShapeIdentityFieldRead:
		encoded.WriteString(" stable=true")
	}
	return encoded.String()
}

type NativeRecordOwnership uint8

const (
	NativeRecordOwnershipAbsent NativeRecordOwnership = iota
	NativeRecordOwnershipRetain
	NativeRecordOwnershipMove
)

// NativeRecordConstructionPayload is the typed record_construction schema.
type NativeRecordConstructionPayload struct {
	Entries           int
	BooleanStorage    bool
	NumericUnion      bool
	DuplicateChildren int
	Edges             int
	EvaluationOrder   bool
	Ownership         NativeRecordOwnership
}

func EncodeNativeRecordConstructionPayload(payload NativeRecordConstructionPayload) string {
	var encoded strings.Builder
	encoded.Grow(192)
	encoded.WriteString("entries=")
	writeInt(&encoded, payload.Entries)
	encoded.WriteString(" entry_storage=committed")
	if payload.BooleanStorage {
		encoded.WriteString(" boolean_storage=canonical_tag")
	}
	if payload.NumericUnion {
		encoded.WriteString(" field_carrier=numeric_union overflow=promote_integer_to_number")
	}
	if payload.Edges != 0 {
		encoded.WriteString(" duplicate_children=")
		writeInt(&encoded, payload.DuplicateChildren)
		encoded.WriteString(" edges=")
		writeInt(&encoded, payload.Edges)
	}
	if payload.EvaluationOrder {
		encoded.WriteString(" evaluation_order=preserved")
	}
	encoded.WriteString(" fresh=true")
	if payload.Ownership == NativeRecordOwnershipMove {
		encoded.WriteString(" ownership=move")
	}
	return encoded.String()
}

type NativeRecordEntryOwnershipPayload struct {
	Field     string
	Ownership NativeRecordOwnership
}

func EncodeNativeRecordEntryOwnershipPayload(payload NativeRecordEntryOwnershipPayload) string {
	ownership := "retain"
	if payload.Ownership == NativeRecordOwnershipMove {
		ownership = "move"
	}
	var encoded strings.Builder
	encoded.Grow(64 + len(payload.Field))
	encoded.WriteString("field=")
	encoded.WriteString(payload.Field)
	encoded.WriteString(" ownership=")
	encoded.WriteString(ownership)
	encoded.WriteString(" producer_bound=true write_barrier=required")
	return encoded.String()
}

// NativeFunctionEntryPayload is the typed function_entry publication schema.
type NativeFunctionEntryPayload struct {
	Parameters  uint32
	Varargs     bool
	CanThrow    bool
	ResultSlots uint32
	ResultOpen  bool
}

func EncodeNativeFunctionEntryPayload(payload NativeFunctionEntryPayload) string {
	var encoded strings.Builder
	encoded.Grow(240)
	encoded.WriteString("params=")
	if payload.Varargs {
		encoded.WriteString("{'exact': False, 'prefix': ")
		writeUint(&encoded, uint64(payload.Parameters), 10)
		encoded.WriteString(", 'open_tail': True}")
	} else {
		encoded.WriteString("{'exact': True, 'count': ")
		writeUint(&encoded, uint64(payload.Parameters), 10)
		encoded.WriteByte('}')
	}
	encoded.WriteString(" completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ")
	if payload.CanThrow {
		encoded.WriteString("['normal', 'throw']")
	} else {
		encoded.WriteString("['normal']")
	}
	encoded.WriteString("} results=")
	if payload.ResultOpen {
		encoded.WriteString("{'exact': False, 'prefix': 0, 'open_tail': True}")
	} else {
		encoded.WriteString("{'exact': True, 'count': ")
		writeUint(&encoded, uint64(payload.ResultSlots), 10)
		encoded.WriteByte('}')
	}
	return encoded.String()
}

type NativeCalleeCompleteness uint8

const (
	NativeCalleeUnknown NativeCalleeCompleteness = iota
	NativeCalleeComplete
	NativeCalleeIncomplete
)

type NativeCalleeSetPayload struct {
	Cardinality  int
	Completeness NativeCalleeCompleteness
}

func EncodeNativeCalleeSetPayload(payload NativeCalleeSetPayload) string {
	var encoded strings.Builder
	encoded.Grow(48)
	switch payload.Completeness {
	case NativeCalleeComplete:
		encoded.WriteString("cardinality=")
		writeInt(&encoded, payload.Cardinality)
		encoded.WriteString(" completeness=complete")
	case NativeCalleeIncomplete:
		encoded.WriteString("cardinality=")
		writeInt(&encoded, payload.Cardinality)
		encoded.WriteString(" completeness=incomplete")
	default:
		return "completeness=unknown"
	}
	return encoded.String()
}

type NativeDiscriminantSelectPayload struct {
	Cases             int
	DefaultRequired   bool
	DiscriminantField string
	Exhaustive        bool
}

func EncodeNativeDiscriminantSelectPayload(payload NativeDiscriminantSelectPayload) string {
	var encoded strings.Builder
	encoded.Grow(96 + payload.Cases*2 + len(payload.DiscriminantField))
	encoded.WriteString("cases=")
	writeInt(&encoded, payload.Cases)
	encoded.WriteString(" default_required=")
	encoded.WriteString(strconv.FormatBool(payload.DefaultRequired))
	encoded.WriteString(" dense_mapping=[")
	for index := 0; index < payload.Cases; index++ {
		if index != 0 {
			encoded.WriteByte(',')
		}
		writeInt(&encoded, index)
	}
	encoded.WriteString("] discriminant_field=")
	encoded.WriteString(payload.DiscriminantField)
	encoded.WriteString(" exhaustive=")
	encoded.WriteString(strconv.FormatBool(payload.Exhaustive))
	return encoded.String()
}

func writeJoined(encoded *strings.Builder, values []string) {
	for index, value := range values {
		if index != 0 {
			encoded.WriteByte(',')
		}
		encoded.WriteString(value)
	}
}

func writeInt(encoded *strings.Builder, value int) {
	var scratch [32]byte
	encoded.Write(strconv.AppendInt(scratch[:0], int64(value), 10))
}

func writeUint(encoded *strings.Builder, value uint64, base int) {
	var scratch [32]byte
	encoded.Write(strconv.AppendUint(scratch[:0], value, base))
}

func writeHex16(encoded *strings.Builder, value uint64) {
	var scratch [16]byte
	digits := strconv.AppendUint(scratch[:0], value, 16)
	for padding := len(digits); padding < 16; padding++ {
		encoded.WriteByte('0')
	}
	encoded.Write(digits)
}
