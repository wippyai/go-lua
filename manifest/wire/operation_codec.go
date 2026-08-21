package wire

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/domain/type/typ"
	typewire "github.com/wippyai/go-lua/types/signature/wire"
)

type valuesJSON struct {
	Fixed    []*typewire.TypeWire `json:"fixed,omitempty"`
	Tail     ValuesTail           `json:"tail"`
	Var      ValuesVar            `json:"var,omitempty"`
	TailType *typewire.TypeWire   `json:"tailType,omitempty"`
	Suffix   []*typewire.TypeWire `json:"suffix,omitempty"`
}

func (value Values) MarshalJSON() ([]byte, error) {
	fixed, err := encodeTypes(value.Fixed)
	if err != nil {
		return nil, err
	}
	suffix, err := encodeTypes(value.Suffix)
	if err != nil {
		return nil, err
	}
	var tail *typewire.TypeWire
	if value.TailType != nil {
		tail, err = typewire.EncodeType(value.TailType)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(valuesJSON{Fixed: fixed, Tail: value.Tail, Var: value.Var, TailType: tail, Suffix: suffix})
}

func (value *Values) UnmarshalJSON(data []byte) error {
	var encoded valuesJSON
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	fixed, err := decodeTypes(encoded.Fixed)
	if err != nil {
		return err
	}
	suffix, err := decodeTypes(encoded.Suffix)
	if err != nil {
		return err
	}
	var tail typ.Type
	if encoded.TailType != nil {
		tail, err = typewire.DecodeType(encoded.TailType)
		if err != nil {
			return err
		}
	}
	*value = Values{Fixed: fixed, Tail: encoded.Tail, Var: encoded.Var, TailType: tail, Suffix: suffix}
	return nil
}

// Revision 2 changes publication Subject from an implicit ValueFormal to an
// explicit InputSource kind/ordinal pair. All operation effect occurrences
// use this revision so an older reader cannot silently reinterpret a row.
const operationWireRevisionPublicationEffects uint8 = 2

type outcomeTailTypeJSON struct {
	Outcome uint32             `json:"outcome"`
	Type    *typewire.TypeWire `json:"type,omitempty"`
}

func (value OutcomeTailType) MarshalJSON() ([]byte, error) {
	encoded, err := encodeOptionalType(value.Type)
	if err != nil {
		return nil, err
	}
	return json.Marshal(outcomeTailTypeJSON{Outcome: value.Outcome, Type: encoded})
}

func (value *OutcomeTailType) UnmarshalJSON(data []byte) error {
	var encoded outcomeTailTypeJSON
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	decoded, err := decodeOptionalType(encoded.Type)
	if err != nil {
		return err
	}
	*value = OutcomeTailType{Outcome: encoded.Outcome, Type: decoded}
	return nil
}

// Operation needs a small custom envelope only for its direct amendment type;
// nested Values and OutcomeTailType values own their own codecs.
func (value Operation) MarshalJSON() ([]byte, error) {
	type plain Operation
	value = CloneOperation(value)
	encoded, err := encodeOptionalType(value.InputTailType)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		InputTailType *typewire.TypeWire `json:"inputTailType,omitempty"`
		WireRevision  uint8              `json:"wireRevision,omitempty"`
	}{plain: plain(value), InputTailType: encoded, WireRevision: operationWireRevision(value)})
}

func (value *Operation) UnmarshalJSON(data []byte) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	type plain Operation
	var encoded struct {
		plain
		InputTailType *typewire.TypeWire `json:"inputTailType,omitempty"`
		WireRevision  uint8              `json:"wireRevision,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return err
	}
	decoded, err := decodeOptionalType(encoded.InputTailType)
	if err != nil {
		return err
	}
	decodedOperation := CloneOperation(Operation(encoded.plain))
	if encoded.WireRevision != 0 && encoded.WireRevision != operationWireRevisionPublicationEffects {
		return fmt.Errorf("manifest: unsupported operation wire revision %d", encoded.WireRevision)
	}
	if encoded.WireRevision != 0 && !hasEffectOccurrences(decodedOperation) {
		return fmt.Errorf("manifest: operation wire revision %d is superfluous without effect occurrences", encoded.WireRevision)
	}
	if hasEffectOccurrences(decodedOperation) && encoded.WireRevision != operationWireRevisionPublicationEffects {
		return fmt.Errorf("manifest: operation effect occurrences require wire revision %d, got %d", operationWireRevisionPublicationEffects, encoded.WireRevision)
	}
	decodedOperation.InputTailType = decoded
	*value = decodedOperation
	return nil
}

func operationWireRevision(value Operation) uint8 {
	if hasEffectOccurrences(value) {
		return operationWireRevisionPublicationEffects
	}
	return 0
}

func hasEffectOccurrences(value Operation) bool {
	if len(value.Effects.Occurrences) != 0 {
		return true
	}
	for _, callback := range value.Callbacks {
		if len(callback.Effects.Occurrences) != 0 {
			return true
		}
	}
	return false
}

func encodeTypes(values []typ.Type) ([]*typewire.TypeWire, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]*typewire.TypeWire, len(values))
	for index, value := range values {
		encoded, err := typewire.EncodeType(value)
		if err != nil {
			return nil, fmt.Errorf("manifest: encode operation type %d: %w", index, err)
		}
		out[index] = encoded
	}
	return out, nil
}

func decodeTypes(values []*typewire.TypeWire) ([]typ.Type, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]typ.Type, len(values))
	for index, value := range values {
		decoded, err := typewire.DecodeType(value)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode operation type %d: %w", index, err)
		}
		out[index] = decoded
	}
	return out, nil
}

func encodeOptionalType(value typ.Type) (*typewire.TypeWire, error) {
	if value == nil {
		return nil, nil
	}
	return typewire.EncodeType(value)
}

func decodeOptionalType(value *typewire.TypeWire) (typ.Type, error) {
	if value == nil {
		return nil, nil
	}
	return typewire.DecodeType(value)
}
