package target

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Raw domain values are confined to this file's adapters.  Laws may request a
// richer fixture through the helpers below, but their Specs still contain only
// schematype.Type declarations.
type testRawType = typ.Type
type testRawTypeParam = typ.TypeParam
type testRawField = typ.Field
type testRawRecordParts = typ.RecordParts
type testRawStaticMember = typ.StaticMember

const testRawStaticMemberStringIndex = typ.StaticMemberStringIndex

var (
	testRawAny     = typ.Any
	testRawBoolean = typ.Boolean
	testRawInteger = typ.Integer
	testRawNumber  = typ.Number
	testRawString  = typ.String
	testRawNil     = typ.Nil
	testRawSelf    = typ.Self
	testRawUnknown = typ.Unknown
)

// The target package authors only neutral schema declarations.  These helpers
// keep the Lua domain encoder in this single test composition file; individual
// target laws do not import or construct domain types.
var (
	testNil     = testPrimitive(schematype.PrimitiveNil)
	testBoolean = testPrimitive(schematype.PrimitiveBoolean)
	testNumber  = testPrimitive(schematype.PrimitiveNumber)
	testInteger = testPrimitive(schematype.PrimitiveInteger)
	testString  = testPrimitive(schematype.PrimitiveString)
	testAny     = testPrimitive(schematype.PrimitiveAny)
	testNever   = testPrimitive(schematype.PrimitiveNever)
	testUnknown schematype.Type
)

func testLiteralString(value string) schematype.Type {
	return testEncode(typ.LiteralString(value))
}

func testLiteralBool(value bool) schematype.Type {
	return testEncode(typ.LiteralBool(value))
}

func testUnion(values ...schematype.Type) schematype.Type {
	raw := make([]typ.Type, len(values))
	for index, value := range values {
		decoded, err := domaincontract.Decode(context.Background(), value, nil)
		if err != nil {
			panic(err)
		}
		raw[index] = decoded
	}
	return testEncode(typ.MaterializeUnion(raw))
}

func testRawOf(value schematype.Type) typ.Type {
	decoded, err := domaincontract.Decode(context.Background(), value, nil)
	if err != nil {
		panic(err)
	}
	return decoded
}

func testNewTypeParam(name string, constraint interface{}) *typ.TypeParam {
	if constraint == nil {
		return typ.NewTypeParam(name, nil)
	}
	switch constraint := constraint.(type) {
	case schematype.Type:
		if !constraint.Available() {
			return typ.NewTypeParam(name, nil)
		}
		return typ.NewTypeParam(name, testRawOf(constraint))
	case typ.Type:
		return typ.NewTypeParam(name, constraint)
	default:
		panic("unsupported target test formal constraint")
	}
}

func testRawFunction() typ.Type { return typ.Func().Build() }

func testFunction() schematype.Type { return testEncode(testRawFunction()) }

func testBuiltinTableTop() schematype.Type { return testEncode(typ.BuiltinTableTopMarker()) }

func testRawRecord(parts typ.RecordParts) typ.Type { return typ.RebuildRecord(parts) }

func testMutableRecord(parts typ.RecordParts) *typ.Record { return typ.RebuildRecord(parts) }

func testRawAnnotated(value typ.Type, annotations []annotation.Annotation) typ.Type {
	return typ.NewAnnotated(value, annotations)
}

func testRawRef(module, name string) typ.Type { return typ.NewRef(module, name) }

func testRecord(parts typ.RecordParts) schematype.Type { return testEncode(testRawRecord(parts)) }

func testRawArray(value typ.Type) typ.Type { return typ.NewArray(value) }

func testRawGeneric(name string, params []*typ.TypeParam, body typ.Type) typ.Type {
	return typ.NewGeneric(name, params, body)
}

func testRawInstantiate(generic typ.Type, arguments ...typ.Type) typ.Type {
	concrete, ok := generic.(*typ.Generic)
	if !ok {
		panic("target test instantiate requires a generic")
	}
	return typ.Instantiate(concrete, arguments...)
}

func testRawRecursive(name string, body func(typ.Type) typ.Type) typ.Type {
	return typ.NewRecursive(name, body)
}

func testRawRecursivePlaceholder(name string) *typ.Recursive {
	return typ.NewRecursivePlaceholder(name)
}

func testMeta(value schematype.Type) schematype.Type {
	return testEncode(typ.NewMeta(testRawOf(value)))
}

func testOperationTypes(values ...interface{}) ([]schematype.Type, []TypeFormalSpec) {
	var formal *typ.TypeParam
	for _, value := range values {
		if candidate, ok := value.(*typ.TypeParam); ok {
			formal = candidate
			break
		}
	}
	var scope []*typ.TypeParam
	var declarations []TypeFormalSpec
	if formal != nil {
		scope = []*typ.TypeParam{formal}
		constraint := schematype.Type{}
		if formal.Constraint != nil {
			constraint = testEncode(formal.Constraint)
		}
		declarations = []TypeFormalSpec{{Constraint: constraint}}
	}
	out := make([]schematype.Type, len(values))
	for index, value := range values {
		switch value := value.(type) {
		case schematype.Type:
			out[index] = value
		case typ.Type:
			out[index] = testEncode(value, scope...)
		default:
			panic("unsupported target test declaration")
		}
	}
	return out, declarations
}

func testPrimitive(primitive schematype.Primitive) schematype.Type {
	value, ok := schematype.NewPrimitive(primitive)
	if !ok {
		panic("invalid test type-contract primitive")
	}
	return value
}

func testTypes(values ...schematype.Type) []schematype.Type {
	return append([]schematype.Type(nil), values...)
}

func testEncode(value typ.Type, formals ...*typ.TypeParam) schematype.Type {
	encoded, err := domaincontract.Encode(context.Background(), value, formals)
	if err != nil {
		panic(err)
	}
	return encoded
}

func testEncodeOrUnavailable(value typ.Type, formals ...*typ.TypeParam) schematype.Type {
	encoded, err := domaincontract.Encode(context.Background(), value, formals)
	if err != nil {
		return schematype.Type{}
	}
	return encoded
}

func testEncodeStorage(value typ.Type, formals ...*typ.TypeParam) schematype.Type {
	encoded, err := domaincontract.EncodeStorage(context.Background(), value, formals)
	if err != nil {
		panic(err)
	}
	return encoded
}

// testSeal supplies the same explicit Lua type authority that production
// profile authoring supplies. It is a test composition helper, not a target
// default: production Seal still rejects an omitted Semantics implementation.
func testSeal(spec *Spec) (*Contract, error) {
	if spec != nil {
		spec.Semantics = domaincontract.NewSemantics()
	}
	return Seal(spec)
}

func requireTestSeal(t testing.TB, spec *Spec) *Contract {
	t.Helper()
	contract, err := testSeal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
