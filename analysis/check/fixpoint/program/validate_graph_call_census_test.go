package program

import (
	"fmt"
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type validateGraphCallRoute uint8

const (
	validateGraphSignatureCall validateGraphCallRoute = iota + 1
	validateGraphLexicalCall
	validateGraphDynamicCall
)

type validateGraphCallCensusKey struct {
	line  int
	path  string
	route validateGraphCallRoute
}

func TestValidateGraphCallCompositionCensus(t *testing.T) {
	src, err := os.ReadFile("../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts := parseChunk(t, string(src))
	reg := standard.Registry()
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"uuid"}, Schedule: transfer.ScheduleWTO,
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	keys := collectKeys(bindings, rootKey(Config{}.RootKey), reg, check.ModuleTypes, check.ModuleExports, stmts)
	lexicalMethods := make(map[string]int)
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind == bind.FunctionOriginMethod && origin.Method != "" {
			lexicalMethods[origin.Method] = origin.Func.Line()
		}
	}

	want := validateGraphExpectedCallCensus()
	var got map[validateGraphCallCensusKey]int
	leafAudited := make(map[int]bool)
	config := Config{Check: check}
	config.semanticProgramAudit = func(_ *body.Static, _ body.Config, solved *body.Result) error {
		fn := solved.Function()
		if fn == nil {
			return nil
		}
		switch fn.Line() {
		case 289, 690:
			if leafAudited[fn.Line()] {
				return nil
			}
			leafAudited[fn.Line()] = true
			for point := cfg.Point(0); int(point) < solved.Graph().Size(); point++ {
				site, ok := solved.CallSiteView(point)
				if !ok {
					continue
				}
				if route := validateGraphRoute(keys, lexicalMethods, solved, site); route == validateGraphLexicalCall {
					return fmt.Errorf("lexical target at line %d has lexical callee %s", fn.Line(), site.CalleePathRef().String())
				}
			}
		case 743:
			if got != nil {
				return nil
			}
			got = make(map[validateGraphCallCensusKey]int)
			for point := cfg.Point(0); int(point) < solved.Graph().Size(); point++ {
				site, ok := solved.CallSiteView(point)
				if !ok {
					continue
				}
				got[validateGraphCallCensusKey{
					line:  site.CallSpan().StartLine,
					path:  site.CalleePathRef().String(),
					route: validateGraphRoute(keys, lexicalMethods, solved, site),
				}]++
			}
		}
		return nil
	}
	if _, err := RunBoundChunk(stmts, bindings, config); err != nil {
		t.Fatal(err)
	}
	if !validateGraphCallCensusEqual(got, want) {
		t.Fatalf("compiler.validate_graph call census differs\n got: %#v\nwant: %#v", got, want)
	}
	if signature, lexical, dynamic := validateGraphCallRouteCounts(got); signature != 40 || lexical != 6 || dynamic != 3 {
		t.Fatalf("compiler.validate_graph call routes = signature:%d lexical:%d dynamic:%d, want 40/6/3", signature, lexical, dynamic)
	}
	if !leafAudited[289] || !leafAudited[690] {
		t.Fatalf("lexical leaf audits = %v, want lines 289 and 690", leafAudited)
	}
	for line, wantParams := range map[int][]string{289: {"self", "name"}, 690: {"graph"}} {
		var fnFound bool
		for _, origin := range bindings.FunctionOrigins() {
			if origin.Func.Line() != line {
				continue
			}
			fnFound = true
			if captures := bindings.DirectCaptures(origin.Func); len(captures) != 0 {
				t.Fatalf("lexical target line %d captures = %#v, want none", line, captures)
			}
			slots := bindings.ParamSlots(origin.Func)
			gotParams := make([]string, len(slots))
			for i := range slots {
				gotParams[i] = slots[i].Name
			}
			if !validateGraphStringsEqual(gotParams, wantParams) {
				t.Fatalf("lexical target line %d params = %v, want %v", line, gotParams, wantParams)
			}
		}
		if !fnFound {
			t.Fatalf("lexical target line %d missing", line)
		}
	}
}

func validateGraphCallCensusEqual(left, right map[validateGraphCallCensusKey]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, count := range left {
		if right[key] != count {
			return false
		}
	}
	return true
}

func validateGraphCallRouteCounts(census map[validateGraphCallCensusKey]int) (signature, lexical, dynamic int) {
	for key, count := range census {
		switch key.route {
		case validateGraphSignatureCall:
			signature += count
		case validateGraphLexicalCall:
			lexical += count
		case validateGraphDynamicCall:
			dynamic += count
		}
	}
	return signature, lexical, dynamic
}

func validateGraphStringsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateGraphRoute(keys programKeys, lexicalMethods map[string]int, solved *body.Result, site factflow.CallSiteView) validateGraphCallRoute {
	if _, ok := solved.CallSiteViewSignatureType(site); ok {
		return validateGraphSignatureCall
	}
	if key, ok := keys.pathKeys[site.CalleePathKey()]; ok {
		if fn := keys.functionByKey[key]; fn != nil && (fn.Line() == 289 || fn.Line() == 690) {
			return validateGraphLexicalCall
		}
	}
	if line := lexicalMethods[site.MethodName()]; line == 289 || line == 690 {
		return validateGraphLexicalCall
	}
	return validateGraphDynamicCall
}

func validateGraphExpectedCallCensus() map[validateGraphCallCensusKey]int {
	out := make(map[validateGraphCallCensusKey]int, 49)
	add := func(route validateGraphCallRoute, path string, lines ...int) {
		for _, line := range lines {
			out[validateGraphCallCensusKey{line: line, path: path, route: route}]++
		}
	}
	add(validateGraphSignatureCall, "table.create", 744, 761, 782, 794, 816, 866, 896, 934)
	add(validateGraphSignatureCall, "pairs", 746, 795, 818, 867, 898)
	add(validateGraphSignatureCall, "ipairs", 748, 753, 765, 775, 783, 785, 820, 832, 845, 852, 854, 904, 925, 935)
	add(validateGraphSignatureCall, "table.insert", 804, 881, 915, 939)
	add(validateGraphSignatureCall, "string.format", 804, 810, 881, 887, 939, 944)
	add(validateGraphSignatureCall, "table.concat", 812, 890, 947)
	add(validateGraphLexicalCall, "graph.resolve_reference", 767, 787, 834, 856)
	add(validateGraphLexicalCall, "compiler.find_root_nodes", 773, 843)
	add(validateGraphDynamicCall, "node_id.sub", 804, 881)
	add(validateGraphDynamicCall, "li.node_id.sub", 939)
	return out
}
