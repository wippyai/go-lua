package readmodel

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
)

func (r Reader) callArgumentBindings(site factflow.CallSite) []path.Path {
	var bindings []path.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := r.valueSourcePath(source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func (r Reader) callBindings(site factflow.CallSite) []path.Path {
	var bindings []path.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := r.valueSourcePath(source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func appendPathBinding(bindings []path.Path, index int, value path.Path) []path.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, path.Path{})
	}
	bindings[index] = value
	return bindings
}
