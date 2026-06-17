package stdlib

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLookupSeededFunctionTypes(t *testing.T) {
	tests := []struct {
		name string
		want *typ.Function
	}{
		{
			name: Assert,
			want: typ.Func().
				Param("v", typ.Any).
				OptParam("message", typ.Any).
				Returns(typ.Any).
				Build(),
		},
		{
			name: Error,
			want: typ.Func().
				Param("message", typ.Any).
				OptParam("level", typ.Integer).
				Returns(typ.Never).
				Build(),
		},
		{
			name: Require,
			want: typ.Func().
				Param("modname", typ.String).
				Returns(typ.Any).
				Build(),
		},
		{
			name: Type,
			want: typ.Func().
				Param("v", typ.Any).
				Returns(typ.String).
				Build(),
		},
		{
			name: Pairs,
			want: typ.Func().
				Param("t", typ.Any).
				Returns(typ.Any, typ.Any, typ.Nil).
				Build(),
		},
		{
			name: IPairs,
			want: typ.Func().
				Param("t", typ.Any).
				Returns(typ.Any, typ.Any, typ.Integer).
				Build(),
		},
		{
			name: PCall,
			want: typ.Func().
				Param("f", typ.Any).
				Variadic(typ.Any).
				Returns(typ.Boolean, typ.Any).
				Build(),
		},
		{
			name: XPCall,
			want: typ.Func().
				Param("f", typ.Any).
				Param("msgh", typ.Any).
				Variadic(typ.Any).
				Returns(typ.Boolean, typ.Any).
				Build(),
		},
		{
			name: TableInsert,
			want: typ.Func().
				Param("list", typ.Any).
				Param("pos_or_value", typ.Any).
				OptParam("value", typ.Any).
				Build(),
		},
		{
			name: "table.create",
			want: typ.Func().
				Param("narray", typ.Integer).
				OptParam("nhash", typ.Integer).
				Returns(typetable.NewRecord().Build()).
				Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.name)
			if !ok {
				t.Fatalf("Lookup(%q) missing", tt.name)
			}
			if !typ.TypeEquals(got.Type, tt.want) {
				t.Fatalf("type = %v, want %v", got.Type, tt.want)
			}
		})
	}
}

func TestLookupSeededEffects(t *testing.T) {
	tests := []struct {
		name   string
		labels []effect.Label
	}{
		{
			name: Assert,
			labels: []effect.Label{
				postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Present{},
				},
			},
		},
		{
			name:   Error,
			labels: nil,
		},
		{
			name: Require,
			labels: []effect.Label{
				dispatch.ModuleLoad{},
			},
		},
		{
			name: Type,
			labels: []effect.Label{
				ownership.BorrowAll{},
			},
		},
		{
			name: Pairs,
			labels: []effect.Label{
				iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed},
			},
		},
		{
			name: IPairs,
			labels: []effect.Label{
				iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed},
			},
		},
		{
			name: PCall,
			labels: []effect.Label{
				ownership.BorrowAll{},
				returns.Return{
					ReturnIndex: 1,
					Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
				},
			},
		},
		{
			name: XPCall,
			labels: []effect.Label{
				ownership.BorrowAll{},
				returns.Return{
					ReturnIndex: 1,
					Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
				},
			},
		},
		{
			name: TableInsert,
			labels: []effect.Label{
				mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
				ownership.Store{Param: effect.ParamRef{Index: -1}, Into: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "table.freeze",
			labels: []effect.Label{
				ownership.Freeze{Param: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "table.isfrozen",
			labels: []effect.Label{
				ownership.BorrowAll{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.name)
			if !ok {
				t.Fatalf("Lookup(%q) missing", tt.name)
			}
			for _, want := range tt.labels {
				if !hasLabel(got.Effect, want) {
					t.Fatalf("effect %v missing label %v", got.Effect, want)
				}
			}
			if len(got.Effect.Labels) != len(tt.labels) {
				t.Fatalf("effect label count = %d, want %d: %v", len(got.Effect.Labels), len(tt.labels), got.Effect)
			}
		})
	}
}

func TestStringUnpackDeclaresConservativeAnyWithoutReservedTransform(t *testing.T) {
	got, ok := Lookup("string.unpack")
	if !ok {
		t.Fatal("Lookup(\"string.unpack\") missing")
	}
	if len(got.Type.Returns) != 1 {
		t.Fatalf("string.unpack returns = %d, want 1", len(got.Type.Returns))
	}
	if !typ.TypeEquals(got.Type.Returns[0], typ.Any) {
		t.Fatalf("string.unpack return = %v, want conservative declared Any", got.Type.Returns[0])
	}

	for _, label := range got.Effect.Labels {
		ret, ok := label.(returns.Return)
		if !ok {
			continue
		}
		if _, ok := ret.Transform.(returns.StringUnpackValue); ok {
			t.Fatalf("string.unpack must not declare inactive StringUnpackValue transform: %v", ret)
		}
	}
}

func TestStringFindDeclaresConservativeReturnsWithoutReservedCorrelation(t *testing.T) {
	got, ok := Lookup("string.find")
	if !ok {
		t.Fatal("Lookup(\"string.find\") missing")
	}
	if len(got.Type.Returns) != 2 {
		t.Fatalf("string.find returns = %d, want 2", len(got.Type.Returns))
	}
	want := normalize.Optional(typ.Integer)
	for i, gotReturn := range got.Type.Returns {
		if !typ.TypeEquals(gotReturn, want) {
			t.Fatalf("string.find return %d = %v, want conservative declared optional integer", i, gotReturn)
		}
	}

	for _, label := range got.Effect.Labels {
		if correlated, ok := label.(returns.CorrelatedReturn); ok {
			t.Fatalf("string.find must not declare inactive CorrelatedReturn: %v", correlated)
		}
	}
}

func TestTypeDeclaresBorrowWithoutReservedPredicate(t *testing.T) {
	got, ok := Lookup(Type)
	if !ok {
		t.Fatalf("Lookup(%q) missing", Type)
	}
	if !hasLabel(got.Effect, ownership.BorrowAll{}) {
		t.Fatalf("type effect missing BorrowAll: %v", got.Effect)
	}
	for _, label := range got.Effect.Labels {
		if _, ok := label.(dispatch.TypePredicate); ok {
			t.Fatalf("type must not declare inactive TypePredicate: %v", label)
		}
	}
}

func TestSelectDeclaresConservativeAnyWithoutReservedVariadicTransform(t *testing.T) {
	got, ok := Lookup("select")
	if !ok {
		t.Fatal("Lookup(\"select\") missing")
	}
	if len(got.Type.Returns) != 1 {
		t.Fatalf("select returns = %d, want 1", len(got.Type.Returns))
	}
	if !typ.TypeEquals(got.Type.Returns[0], typ.Any) {
		t.Fatalf("select return = %v, want conservative declared Any", got.Type.Returns[0])
	}
	for _, label := range got.Effect.Labels {
		if _, ok := label.(dispatch.VariadicTransform); ok {
			t.Fatalf("select must not declare inactive VariadicTransform: %v", label)
		}
	}
}

func TestStdlibDoesNotDeclareReservedOrHighRiskEffectLabels(t *testing.T) {
	for name, sig := range Signatures() {
		for _, label := range sig.Effect.Labels {
			desc, ok := caplabel.DescriptorFor(label)
			if !ok {
				t.Fatalf("%s declares unaudited effect label %T: %v", name, label, label)
			}
			switch desc.Status {
			case capability.StatusReserved, capability.StatusReservedHighRisk:
				t.Fatalf("%s declares inactive effect label %s (%s): %v", name, desc.ID, desc.Status, label)
			}
		}
	}
}

func TestStdlibSignatureConstructionRejectsInactiveEffectLabels(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
	}{
		{"control io", control.IO{}},
		{"return deep element", returns.Return{
			ReturnIndex: 0,
			Transform:   returns.DeepElementOf{},
		}},
		{"return string unpack", returns.Return{
			ReturnIndex: 0,
			Transform:   returns.StringUnpackValue{},
		}},
		{"return select case", returns.Return{
			ReturnIndex: 0,
			Transform:   returns.SelectCaseOfParam{},
		}},
		{"return select result", returns.Return{
			ReturnIndex: 0,
			Transform:   returns.SelectResultOfCases{},
		}},
		{"return length", returns.ReturnLength{}},
		{"correlated return", returns.CorrelatedReturn{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustPanicContaining(t, "inactive effect label", func() {
				sig(typ.Func().Build(), tt.label)
			})
		})
	}
}

func TestLookupAndSignaturesCloneResults(t *testing.T) {
	first, ok := Lookup(Type)
	if !ok {
		t.Fatalf("Lookup(%q) missing", Type)
	}
	first.Type.Params[0].Name = "changed"
	first.Effect.Labels = nil

	second, ok := Lookup(Type)
	if !ok {
		t.Fatalf("Lookup(%q) missing after local mutation", Type)
	}
	if second.Type.Params[0].Name != "v" {
		t.Fatalf("Lookup returned aliased function params: %q", second.Type.Params[0].Name)
	}
	if len(second.Effect.Labels) == 0 {
		t.Fatal("Lookup returned aliased effect labels")
	}

	all := Signatures()
	delete(all, Require)
	if _, ok := Lookup(Require); !ok {
		t.Fatal("Signatures returned registry map itself")
	}
}

func TestSignaturesSeededNames(t *testing.T) {
	gotMap := Signatures()
	got := make([]string, 0, len(gotMap))
	for name := range gotMap {
		got = append(got, name)
	}
	sort.Strings(got)

	want := []string{
		Assert, Error, Require, ToString, Type, Pairs, IPairs, PCall, XPCall,
		"print", "tonumber", "next", "select", "rawget", "rawset", "rawequal",
		"rawlen", "setmetatable", "getmetatable", "collectgarbage", "unpack",
		TableInsert, "table.remove", "table.concat", "table.sort", "table.unpack",
		"table.pack", "table.move", "table.create", "table.freeze", "table.isfrozen",
		"string.byte", "string.char", "string.dump", "string.find", "string.format",
		"string.gmatch", "string.gsub", "string.len", "string.lower", "string.match",
		"string.pack", "string.packsize", "string.rep", "string.reverse", "string.sub",
		"string.unpack", "string.upper",
		"math.abs", "math.acos", "math.asin", "math.atan", "math.atan2", "math.ceil",
		"math.cos", "math.cosh", "math.deg", "math.exp", "math.floor", "math.fmod",
		"math.frexp", "math.ldexp", "math.log", "math.log10", "math.max", "math.min",
		"math.mod", "math.modf", "math.pow", "math.rad", "math.random", "math.randomseed",
		"math.sin", "math.sinh", "math.sqrt", "math.tan", "math.tanh", "math.tointeger",
		"math.type", "math.ult",
		"coroutine.close", "coroutine.create", "coroutine.isyieldable", "coroutine.resume",
		"coroutine.running", "coroutine.status", "coroutine.wrap", "coroutine.yield",
		"os.clock", "os.date", "os.difftime", "os.getenv", "os.time", "os.tmpname",
		"os.exit", "os.remove", "os.rename", "os.execute",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func TestAssertSignatureDeclaresNormalReturnPresentPostcondition(t *testing.T) {
	got, ok := Lookup(Assert)
	if !ok {
		t.Fatalf("Lookup(%q) missing", Assert)
	}
	want := postcondition.NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 0},
		Refinement: postcondition.Present{},
	}
	if !hasLabel(got.Effect, want) {
		t.Fatalf("assert effect %v missing %v", got.Effect, want)
	}
}

func TestRegistrySignaturesManifestRoundTrip(t *testing.T) {
	m := manifest.New("stdlib")
	want := Signatures()
	for name, sig := range want {
		m.DefineFunctionSignature(name, sig)
	}

	data, err := manifest.Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(got.FunctionSignatures) != len(want) {
		t.Fatalf("decoded signatures = %d, want %d", len(got.FunctionSignatures), len(want))
	}
	for name, wantSig := range want {
		gotSig, ok := got.FunctionSignatures[name]
		if !ok {
			t.Fatalf("decoded manifest missing %q", name)
		}
		if !wantSig.Equals(gotSig) {
			t.Fatalf("%s signature = %v, want %v", name, gotSig, wantSig)
		}
	}
}

func TestTypePackageDoesNotImportEffect(t *testing.T) {
	root := filepath.Clean("../../../../type/typ")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "github.com/wippyai/go-lua/analysis/domain/effect" ||
				strings.HasPrefix(path, "github.com/wippyai/go-lua/analysis/domain/effect/") {
				t.Fatalf("%s imports analysis/domain/effect", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk typ package: %v", err)
	}
}

func hasLabel(row effect.Row, want effect.Label) bool {
	return row.Has(func(got effect.Label) bool {
		return want.Equals(got)
	})
}

func mustPanicContaining(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if !strings.Contains(got.(string), want) {
			t.Fatalf("panic = %q, want to contain %q", got, want)
		}
	}()
	f()
}
