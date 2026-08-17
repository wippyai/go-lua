package wire

import (
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
	encoded, err := encodeOptionalType(value.InputTailType)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		InputTailType *typewire.TypeWire `json:"inputTailType,omitempty"`
	}{plain: plain(value), InputTailType: encoded})
}

func (value *Operation) UnmarshalJSON(data []byte) error {
	type plain Operation
	var encoded struct {
		plain
		InputTailType *typewire.TypeWire `json:"inputTailType,omitempty"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	decoded, err := decodeOptionalType(encoded.InputTailType)
	if err != nil {
		return err
	}
	*value = Operation(encoded.plain)
	value.InputTailType = decoded
	return nil
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
