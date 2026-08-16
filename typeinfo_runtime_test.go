package lua

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
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

	manifest := typemanifest.New("typeinfo")
	pointType := typetable.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)
	manifest.DefineType("ID", typ.String)
	userType := typetable.NewRecord().
		Field("id", typ.NewRef("", "ID")).
		Build()
	manifest.DefineType("User", userType)

	data, err := typemanifest.Encode(manifest)
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

func TestTypeInfoRuntime_StringCastAndLib(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	source := `
		local s1 = string.rep("x", 3)
		local s2 = string("y")
		return s1, s2
	`
	proto, err := CompileString(source, "string_cast.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	manifest := typemanifest.New("string_cast")
	manifest.DefineType("string", typ.String)
	data, err := typemanifest.Encode(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	proto.SetTypeInfo(data)

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
	proto, err := CompileString(source, "typeinfo_dot.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	manifest := typemanifest.New("typeinfo_dot")
	pointType := typetable.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)

	data, err := typemanifest.Encode(manifest)
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

func TestTypeInfoRuntime_TypeIsNestedFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local function main()
			local val, err = Point:is({x = 1, y = 2})
			return val ~= nil, err
		end
		return main()
	`
	proto, err := CompileString(source, "typeinfo_nested.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	manifest := typemanifest.New("typeinfo_nested")
	pointType := typetable.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()
	manifest.DefineType("Point", pointType)

	data, err := typemanifest.Encode(manifest)
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

func TestTypeInfoRuntime_AnnotatedArrayField(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local function main()
			local ok, err = Holder:is({items = {}})
			return ok, err
		end
		return main()
	`
	proto, err := CompileString(source, "typeinfo_annotated_array.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	manifest := typemanifest.New("typeinfo_annotated_array")
	listType := typ.NewAnnotated(typ.NewArray(typ.Number), []annotation.Annotation{
		{Name: "min_len", Arg: annotation.Float64Arg(1)},
	})
	holderType := typetable.NewRecord().Field("items", listType).Build()
	manifest.DefineType("Holder", holderType)

	data, err := typemanifest.Encode(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	proto.SetTypeInfo(data)

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val != LNil {
		t.Errorf("expected nil value for empty items, got %v", val)
	}
	if errVal == LNil {
		t.Error("expected error for empty items, got nil")
	}
}

func TestTypeInfoRuntime_InstantiatedGeneric(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local function main()
			local ok, err = BoxNum:is({value = 1})
			return ok, err
		end
		return main()
	`
	proto, err := CompileString(source, "typeinfo_inst_generic.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	manifest := typemanifest.New("typeinfo_inst_generic")
	tp := typ.NewTypeParam("T", nil)
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	boxNum := typ.Instantiate(boxGeneric, typ.Number)
	manifest.DefineType("BoxNum", boxNum)

	data, err := typemanifest.Encode(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}
	proto.SetTypeInfo(data)

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Error("expected non-nil value for BoxNum, got nil")
	}
	if errVal != LNil {
		t.Errorf("expected nil error for BoxNum, got %v", errVal)
	}
}

func TestTypeInfoRuntime_ManifestRecordInjection(t *testing.T) {
	L := NewState()
	defer L.Close()
	OpenBase(L)

	source := `
		local function main()
			local value = {name = "Ada", age = 33}
			local ok, err = Sample:is(value)
			return ok, err
		end
		return main()
	`

	manifest := typemanifest.New("typeinfo_sample")
	sampleType := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	manifest.DefineType("Sample", sampleType)

	data, err := typemanifest.Encode(manifest)
	if err != nil {
		t.Fatalf("encode manifest failed: %v", err)
	}

	proto, err := CompileString(source, "typeinfo_sample.lua")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	proto.SetTypeInfo(data)

	fn := L.LoadProto(proto)
	L.Push(fn)
	if err := L.PCall(0, 2, nil); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	val := L.Get(-2)
	errVal := L.Get(-1)
	L.Pop(2)

	if val == LNil {
		t.Error("expected non-nil value for Sample, got nil")
	}
	if errVal != LNil {
		t.Errorf("expected nil error for Sample, got %v", errVal)
	}
}
