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
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/manifest"
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
				control.Throw{},
				postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Present{},
				},
			},
		},
		{
			name: Error,
			labels: []effect.Label{
				control.Throw{},
			},
		},
		{
			name: Require,
			labels: []effect.Label{
				dispatch.ModuleLoad{},
				control.Throw{},
			},
		},
		{
			name: Type,
			labels: []effect.Label{
				dispatch.TypePredicate{},
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
