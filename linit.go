package lua

import (
	"fmt"
	"sort"
	"strings"

	declarations "github.com/wippyai/go-lua/manifest"
	"github.com/wippyai/go-lua/stdlib"
)

type luaLib struct {
	libName string
	libFunc LGoFunc
}

type nativeLibrary struct {
	opener    LGoFunc
	callables func() map[string]struct{}
}

var nativeDeclarations = mustNativeDeclarations()

var luaLibs = bindStandardLibraries(map[stdlib.ID]nativeLibrary{
	stdlib.Package:   {opener: OpenPackage, callables: func() map[string]struct{} { return callableNames(loFuncs) }},
	stdlib.Base:      {opener: OpenBase, callables: func() map[string]struct{} { return callableNames(baseFuncs) }},
	stdlib.Table:     {opener: OpenTable, callables: func() map[string]struct{} { return callableNames(tableFuncs) }},
	stdlib.String:    {opener: OpenString, callables: stringCallableNames},
	stdlib.Math:      {opener: OpenMath, callables: func() map[string]struct{} { return callableNames(mathFuncs) }},
	stdlib.Debug:     {opener: OpenDebug, callables: func() map[string]struct{} { return callableNames(debugFuncs) }},
	stdlib.Coroutine: {opener: OpenCoroutine, callables: func() map[string]struct{} { return callableNames(coFuncs) }},
	stdlib.UTF8:      {opener: OpenUtf8, callables: func() map[string]struct{} { return callableNames(utf8Funcs) }},
	stdlib.Errors:    {opener: OpenErrors, callables: func() map[string]struct{} { return callableNames(errorsFuncs) }},
})

func mustNativeDeclarations() *declarations.Catalogue {
	catalogue, err := declarations.Seal(stdlib.Providers()...)
	if err != nil {
		panic(err)
	}
	return catalogue
}

func bindStandardLibraries(implementations map[stdlib.ID]nativeLibrary) []luaLib {
	bound, err := stdlib.Bind(implementations)
	if err != nil {
		panic(err)
	}
	libraries := make([]luaLib, 0, len(bound))
	for _, item := range bound {
		if item.Value.opener == nil || item.Value.callables == nil {
			panic("lua: nil standard-library opener for " + string(item.Library.ID()))
		}
		if err := validateNativeCallables(item.Library, item.Value.callables()); err != nil {
			panic(err)
		}
		libraries = append(libraries, luaLib{libName: item.Library.Name(), libFunc: item.Value.opener})
	}
	return libraries
}

func callableNames[T any](functions map[string]T) map[string]struct{} {
	out := make(map[string]struct{}, len(functions))
	for name := range functions {
		out[name] = struct{}{}
	}
	return out
}

func stringCallableNames() map[string]struct{} {
	out := callableNames(strFuncs)
	for name := range strStatefulFuncs {
		out[name] = struct{}{}
	}
	return out
}

func validateNativeCallables(library stdlib.Library, actual map[string]struct{}) error {
	expected := make(map[string]struct{})
	prefix := library.Name()
	if prefix != "" {
		prefix += "."
	}
	for _, function := range nativeDeclarations.Functions() {
		if function.ProviderIdentity() != string(library.ID()) {
			continue
		}
		local := strings.TrimPrefix(function.CanonicalPath(), prefix)
		if local == function.CanonicalPath() && prefix != "" || strings.Contains(local, ".") {
			continue
		}
		expected[local] = struct{}{}
	}
	missing, foreign := setDifference(expected, actual), setDifference(actual, expected)
	if len(missing) != 0 || len(foreign) != 0 {
		return fmt.Errorf("lua: native callable coverage for %q: missing=%v foreign=%v", library.ID(), missing, foreign)
	}
	return nil
}

func setDifference(left, right map[string]struct{}) []string {
	var out []string
	for name := range left {
		if _, ok := right[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// mountManifestAliases installs every additional binding of an already
// mounted callable from the canonical manifest identity. Aliases therefore do
// not require another native implementation entry.
func mountManifestAliases(L *LState, library stdlib.ID) {
	entry, ok := stdlib.Lookup(library)
	if !ok || entry.Mount() != stdlib.MountModule {
		return
	}
	module := L.GetGlobal(entry.Name())
	table, ok := module.(*LTable)
	if !ok {
		return
	}
	for _, function := range nativeDeclarations.Functions() {
		bindings := function.Bindings()
		var value LValue
		for _, binding := range bindings {
			if candidate := manifestBindingValue(L, binding); candidate != LNil {
				value = candidate
				break
			}
		}
		if value == nil {
			continue
		}
		for _, binding := range bindings {
			member := binding.Member()
			if binding.Mount() == declarations.MountModule && binding.ModulePath() == entry.Name() && len(member) == 1 {
				table.RawSetString(member[0], value)
			}
		}
	}
}

func manifestBindingValue(L *LState, binding declarations.Binding) LValue {
	member := binding.Member()
	if len(member) != 1 {
		return LNil
	}
	switch binding.Mount() {
	case declarations.MountGlobals:
		return L.GetGlobal(member[0])
	case declarations.MountModule:
		module, ok := L.GetGlobal(binding.ModulePath()).(*LTable)
		if !ok {
			return LNil
		}
		return module.RawGetString(member[0])
	default:
		return LNil
	}
}

// OpenLibs loads the built-in libraries. It is equivalent to running OpenLoad,
// then OpenBase, then iterating over the other OpenXXX functions in any order.
func (ls *LState) OpenLibs() {
	// NB: Map iteration order in Go is deliberately randomised, so must open Load/Base
	// prior to iterating.
	for _, lib := range luaLibs {
		ls.Push(lib.libFunc)
		ls.Push(LString(lib.libName))
		ls.Call(1, 0)
	}
}
