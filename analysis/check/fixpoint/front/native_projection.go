package front

import (
	"encoding/json"
	"fmt"
)

// NativeProjection is the typed conclusion form produced by semantic kernels.
// Lowering topology never uses this type; NativeTopologyDraft is its separate,
// verdict-incapable boundary.
type NativeProjection struct {
	Version     uint8                        `json:"version"`
	Key         string                       `json:"key"`
	Value       string                       `json:"value"`
	Term        string                       `json:"term,omitempty"`
	Subject     string                       `json:"subject,omitempty"`
	Occurrence  string                       `json:"occurrence,omitempty"`
	Revocations []NativeProjectionRevocation `json:"revocations,omitempty"`
}

type NativeHostGlobalRequirement struct {
	Root   string   `json:"root"`
	Fields []string `json:"fields,omitempty"`
}

type NativeProjectionRevocation struct {
	Established string `json:"established,omitempty"`
	Revoked     string `json:"revoked,omitempty"`
	Event       string `json:"event,omitempty"`
}

func EncodeNativeProjection(row NativeProjection) ([]byte, error) {
	row.Version = 1
	if row.Key == "" || row.Value == "" {
		return nil, fmt.Errorf("front: incomplete native projection")
	}
	return json.Marshal(row)
}

func DecodeNativeProjection(encoded []byte) (NativeProjection, bool) {
	var row NativeProjection
	if json.Unmarshal(encoded, &row) != nil || row.Version != 1 || row.Key == "" || row.Value == "" {
		return NativeProjection{}, false
	}
	return row, true
}
