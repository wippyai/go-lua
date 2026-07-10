package bytecode

import (
	"bytes"
	"errors"
	"testing"

	lua "github.com/wippyai/go-lua"
)

func TestDumpUndumpRoundTripExecutes(t *testing.T) {
	state := lua.NewState()
	defer state.Close()

	fn, err := state.LoadString(`
		local function fib(n)
			if n < 2 then return n end
			return fib(n - 1) + fib(n - 2)
		end
		return fib(10)
	`)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}

	data, err := Dump(fn.Proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	decoded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	roundTrip := lua.NewState()
	defer roundTrip.Close()
	roundTrip.Push(roundTrip.NewFunctionFromProto(decoded))
	if err := roundTrip.PCall(0, 1, nil); err != nil {
		t.Fatalf("decoded proto failed to execute: %v", err)
	}
	if got := roundTrip.ToNumber(-1); got != 55 {
		t.Fatalf("decoded proto returned %v, want 55", got)
	}
}

func TestDumpUndumpPreservesProtoMetadata(t *testing.T) {
	state := lua.NewState()
	defer state.Close()

	fn, err := state.LoadString(`
		local label = "hello"
		local function inner(x)
			return label .. ":" .. x
		end
		return inner("world"), true, 42, 3.5
	`)
	if err != nil {
		t.Fatalf("LoadString failed: %v", err)
	}
	fn.Proto.TypeInfo = []byte("manifest-v1")

	data, err := Dump(fn.Proto)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	decoded, err := Undump(data)
	if err != nil {
		t.Fatalf("Undump failed: %v", err)
	}

	if len(decoded.Code) != len(fn.Proto.Code) {
		t.Fatalf("Code length = %d, want %d", len(decoded.Code), len(fn.Proto.Code))
	}
	for i, op := range decoded.Code {
		if op != fn.Proto.Code[i] {
			t.Fatalf("Code[%d] = %d, want %d", i, op, fn.Proto.Code[i])
		}
	}
	if len(decoded.FunctionPrototypes) != len(fn.Proto.FunctionPrototypes) {
		t.Fatalf("nested proto count = %d, want %d", len(decoded.FunctionPrototypes), len(fn.Proto.FunctionPrototypes))
	}
	if !bytes.Equal(decoded.TypeInfo, fn.Proto.TypeInfo) {
		t.Fatalf("TypeInfo = %q, want %q", decoded.TypeInfo, fn.Proto.TypeInfo)
	}
	for _, child := range decoded.FunctionPrototypes {
		if !bytes.Equal(child.TypeInfo, fn.Proto.TypeInfo) {
			t.Fatalf("nested TypeInfo = %q, want %q", child.TypeInfo, fn.Proto.TypeInfo)
		}
	}
	if len(decoded.Constants) != len(fn.Proto.Constants) {
		t.Fatalf("constant count = %d, want %d", len(decoded.Constants), len(fn.Proto.Constants))
	}
	for i, got := range decoded.Constants {
		want := fn.Proto.Constants[i]
		if got.Type() != want.Type() || got.String() != want.String() {
			t.Fatalf("constant[%d] = %s %q, want %s %q", i, got.Type(), got.String(), want.Type(), want.String())
		}
	}
}

func TestUndumpRejectsBadHeaderAndVersion(t *testing.T) {
	if _, err := Undump([]byte{0, 0, 0, 0}); !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("Undump bad header error = %v, want %v", err, ErrInvalidHeader)
	}

	if _, err := Undump([]byte{0x43, 0x41, 0x55, 0x4C, 99}); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("Undump bad version error = %v, want %v", err, ErrVersionMismatch)
	}
}

func TestDumpRejectsUnsupportedConstants(t *testing.T) {
	state := lua.NewState()
	defer state.Close()

	proto := &lua.FunctionProto{
		Constants: []lua.LValue{state.NewTable()},
	}
	if _, err := Dump(proto); !errors.Is(err, ErrUnsupportedConstant) {
		t.Fatalf("Dump unsupported constant error = %v, want %v", err, ErrUnsupportedConstant)
	}
}
