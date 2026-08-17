package lua

import "github.com/wippyai/go-lua/stdlib"

const (
	// BaseLibName is here for consistency; the base functions have no namespace/library.
	BaseLibName = stdlib.BaseName
	// LoadLibName is here for consistency; the loading system has no namespace/library.
	LoadLibName = stdlib.PackageName
	// TabLibName is the name of the table Library.
	TabLibName = stdlib.TableName
	// StringLibName is the name of the string Library.
	StringLibName = stdlib.StringName
	// MathLibName is the name of the math Library.
	MathLibName = stdlib.MathName
	// DebugLibName is the name of the debug Library.
	DebugLibName = stdlib.DebugName
	// CoroutineLibName is the name of the coroutine Library.
	CoroutineLibName = stdlib.CoroutineName
	// Utf8LibName is the name of the utf8 Library.
	Utf8LibName = stdlib.UTF8Name
	// ErrorsLibName is the name of the errors Library.
	ErrorsLibName = stdlib.ErrorsName
)

type luaLib struct {
	libName string
	libFunc LGoFunc
}

var luaLibs = bindStandardLibraryOpeners(map[stdlib.ID]LGoFunc{
	stdlib.Package:   OpenPackage,
	stdlib.Base:      OpenBase,
	stdlib.Table:     OpenTable,
	stdlib.String:    OpenString,
	stdlib.Math:      OpenMath,
	stdlib.Debug:     OpenDebug,
	stdlib.Coroutine: OpenCoroutine,
	stdlib.UTF8:      OpenUtf8,
	stdlib.Errors:    OpenErrors,
})

func bindStandardLibraryOpeners(openers map[stdlib.ID]LGoFunc) []luaLib {
	bound, err := stdlib.Bind(openers)
	if err != nil {
		panic(err)
	}
	libraries := make([]luaLib, 0, len(bound))
	for _, item := range bound {
		if item.Value == nil {
			panic("lua: nil standard-library opener for " + string(item.Library.ID()))
		}
		libraries = append(libraries, luaLib{libName: item.Library.Name(), libFunc: item.Value})
	}
	return libraries
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
