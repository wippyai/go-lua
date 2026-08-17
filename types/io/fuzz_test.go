package io

import (
	"bytes"
	"testing"

	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// FuzzManifestDecode protects the manifest wire boundary used for imported
// checker metadata. Arbitrary bytes must return an error rather than panic;
// successfully decoded values must also survive a canonical encode/decode
// round trip.
func FuzzManifestDecode(f *testing.F) {
	seeds := []*Manifest{
		New("empty"),
		func() *Manifest {
			m := New("types")
			m.DefineType("User", typetable.NewRecord().Field("id", typ.String).OptField("age", typ.Number).Build())
			m.SetExport(typ.NewArray(typeexpr.Optional(typ.String)))
			return m
		}(),
		func() *Manifest {
			m := New("signatures")
			m.DefineGlobalType("enabled", typ.Boolean)
			m.SetExport(typ.Func().Param("value", typ.Any).Returns(typ.String).Build())
			return m
		}(),
	}
	for _, seed := range seeds {
		data, err := Encode(seed)
		if err != nil {
			f.Fatalf("encode fuzz seed: %v", err)
		}
		f.Add(data)
	}
	f.Add([]byte{})
	f.Add([]byte("{"))

	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := Decode(data)
		if err != nil {
			return
		}
		encoded, err := Encode(decoded)
		if err != nil {
			t.Fatalf("encode decoded manifest: %v", err)
		}
		roundTripped, err := Decode(encoded)
		if err != nil {
			t.Fatalf("decode canonical manifest: %v", err)
		}
		reencoded, err := Encode(roundTripped)
		if err != nil {
			t.Fatalf("re-encode canonical manifest: %v", err)
		}
		if !bytes.Equal(encoded, reencoded) {
			t.Fatalf("manifest canonical round trip changed bytes\nfirst: %s\nsecond: %s", encoded, reencoded)
		}
	})
}
