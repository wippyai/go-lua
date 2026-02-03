package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/parse"
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeInfoInjection_TypeIs(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local ok1 = (Point:is({x = 1, y = 2})) ~= nil
		local ok2 = (User:is({id = "abc"})) ~= nil
		return ok1 and ok2, nil
	`
	manifest := typeio.NewManifest("typeinfo")
	pointType := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)
	manifest.DefineType("ID", typ.String)
	userType := typ.NewRecord().
		Field("id", typ.NewRef("", "ID")).
		Build()
	manifest.DefineType("User", userType)

	data, err := typeio.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	chunk, err := parse.ParseString(source, "typeinfo.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "typeinfo.lua", CompileOptions{TypeInfo: data})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if result != LTrue {
		t.Errorf("expected true result, got %v", result)
	}
	if errVal != LNil {
		t.Errorf("expected nil error, got %v", errVal)
	}
}

func TestTypeInfoRuntime_StringCastAndLib(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	source := `
		local s1 = string.rep("x", 3)
		local s2 = string("y")
		return s1, s2
	`
	manifest := typeio.NewManifest("string_cast")
	manifest.DefineType("string", typ.String)
	data, err := typeio.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	chunk, err := parse.ParseString(source, "string_cast.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "string_cast.lua", CompileOptions{TypeInfo: data})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	s1 := L.Get(-2)
	s2 := L.Get(-1)
	L.Pop(2)
	if s1 != LString("xxx") {
		t.Errorf("expected s1 to be %q, got %v", "xxx", s1)
	}
	if s2 != LString("y") {
		t.Errorf("expected s2 to be %q, got %v", "y", s2)
	}
}

func TestTypeInfoRuntime_TypeIsDotSyntax(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local val, err = Point.is({x = 1, y = 2})
		return val ~= nil, err
	`
	manifest := typeio.NewManifest("typeinfo_dot")
	pointType := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)

	data, err := typeio.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	chunk, err := parse.ParseString(source, "typeinfo_dot.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	proto, err := CompileWithOptions(chunk, "typeinfo_dot.lua", CompileOptions{TypeInfo: data})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	result := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)
	if result != LTrue {
		t.Errorf("expected true result, got %v", result)
	}
	if errVal != LNil {
		t.Errorf("expected nil error, got %v", errVal)
	}
}
