package io

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestManifestArgumentContractsRoundTrip(t *testing.T) {
	m := NewManifest("arguments")
	m.ArgumentContracts = map[string]*ArgumentContract{
		"1:2": {Signature: typ.Func().Param("id", typ.String).Param("untyped", typ.Any).Variadic(typ.Integer).Build(), Types: map[string]typ.Type{"ID": typ.String}},
	}
	encoded, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	c := decoded.ArgumentContracts["1:2"]
	if c == nil || !typ.TypeEquals(c.Signature, m.ArgumentContracts["1:2"].Signature) || c.Types["ID"] != typ.String {
		t.Fatal("argument contracts changed across encoding")
	}
	again, err := decoded.Encode()
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("encoding must be deterministic", err)
	}
	for _, version := range []byte{13, 14} {
		old, err := m.encodeVersion(version)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeManifest(old)
		if err != nil || len(decoded.ArgumentContracts) != 0 {
			t.Fatal("older manifests should decode without contracts", err)
		}
	}
}
