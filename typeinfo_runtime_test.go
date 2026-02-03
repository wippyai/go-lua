package lua

import (
	"testing"

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
	proto, err := CompileString(source, "typeinfo.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

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
	proto.SetTypeInfo(data)

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
