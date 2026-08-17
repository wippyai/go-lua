package io

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
	"github.com/wippyai/go-lua/types/signature"
)

// ProjectMethod returns the manifest exposed by a runtime entry that executes
// one named method from a shared Lua module body. The shared module remains the
// authority for types and effects; projection only selects and rekeys the
// requested public function.
func (m *Manifest) ProjectMethod(newPath, method string) (*Manifest, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest: project method from nil manifest")
	}
	if newPath == "" {
		return nil, fmt.Errorf("manifest: project method %q to empty path", method)
	}
	if method == "" {
		return m.Rebound(newPath), nil
	}
	record, ok := unwrap.Annotated(m.Export).(*typ.Record)
	if !ok {
		return nil, fmt.Errorf("manifest %q export is not a record; method %q is unavailable", m.Path, method)
	}
	var selected typ.Type
	if field := record.GetField(method); field != nil {
		selected = field.Type
	} else if member := record.GetStaticStringIndex(method); member != nil {
		selected = member.Type
	}
	if selected == nil {
		return nil, fmt.Errorf("manifest %q has no exported method %q", m.Path, method)
	}
	clone := *m
	clone.Path = newPath
	clone.Export = selected
	clone.FunctionSignatures = projectedMethodSignatures(m, newPath, method)
	clone.CallbackPhaseRegistrations = projectedCallbackRegistrations(m.CallbackPhaseRegistrations, m.Path, newPath, method)
	clone.CallbackPhaseInvocations = projectedCallbackInvocations(m.CallbackPhaseInvocations, m.Path, newPath, method)
	return &clone, nil
}

func projectedMethodSignatures(m *Manifest, newPath, method string) map[string]signature.Function {
	if m == nil || len(m.FunctionSignatures) == 0 {
		return nil
	}
	for _, key := range []string{m.Path + "." + method, method} {
		if sig, ok := m.FunctionSignatures[key]; ok {
			return map[string]signature.Function{newPath: sig.Clone()}
		}
	}
	return nil
}

func projectedCallbackRegistrations(items []CallbackPhaseRegistration, oldPath, newPath, method string) []CallbackPhaseRegistration {
	want := oldPath + "." + method
	var out []CallbackPhaseRegistration
	for _, item := range items {
		if item.Function != want && item.Function != method {
			continue
		}
		item.Function = newPath
		out = append(out, item)
	}
	return out
}

func projectedCallbackInvocations(items []CallbackPhaseInvocation, oldPath, newPath, method string) []CallbackPhaseInvocation {
	want := oldPath + "." + method
	var out []CallbackPhaseInvocation
	for _, item := range items {
		if item.Function != want && item.Function != method {
			continue
		}
		item.Function = newPath
		item.Before = append([]string(nil), item.Before...)
		item.After = append([]string(nil), item.After...)
		out = append(out, item)
	}
	return out
}
