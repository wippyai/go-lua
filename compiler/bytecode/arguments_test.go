package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	lua "github.com/wippyai/go-lua"
)

func TestArgumentMetadataRoundTrip(t *testing.T) {
	p := &lua.FunctionProto{ArgumentInfo: []byte("root"), FunctionPrototypes: []*lua.FunctionProto{{ArgumentInfo: []byte("child")}}}
	data, err := Dump(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Undump(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.ArgumentInfo, p.ArgumentInfo) || !bytes.Equal(decoded.FunctionPrototypes[0].ArgumentInfo, p.FunctionPrototypes[0].ArgumentInfo) {
		t.Fatal("per-callable metadata changed")
	}
}

func TestArgumentMetadataCorruptLength(t *testing.T) {
	data, err := Dump(&lua.FunctionProto{})
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[len(data)-4:], ^uint32(0))
	if _, err := Undump(data); !errors.Is(err, ErrCorruptedBytecode) {
		t.Fatalf("corrupt length accepted: %v", err)
	}
}

func TestVersion2WithoutArgumentMetadata(t *testing.T) {
	data, err := Dump(&lua.FunctionProto{})
	if err != nil {
		t.Fatal(err)
	}
	// v2 has the same layout for this childless proto, without the final
	// argument-metadata length. It cannot reconstruct erased declarations.
	data = data[:len(data)-4]
	data[4] = 2
	p, err := Undump(data)
	if err != nil || len(p.ArgumentInfo) != 0 {
		t.Fatalf("v2 decode: %v", err)
	}
}
