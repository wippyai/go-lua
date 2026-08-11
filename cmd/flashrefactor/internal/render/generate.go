package render

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
)

func (state *renderState) preflightGenerate(edit cutplan.Generate) error {
	if err := state.writeAllowed(edit.Destination); err != nil {
		return err
	}
	for _, path := range edit.Inputs {
		if _, err := state.inputBytes(path); err != nil {
			return fmt.Errorf("generator input %s: %w", path, err)
		}
	}
	return nil
}

func (state *renderState) generate(edit cutplan.Generate) error {
	if err := state.preflightGenerate(edit); err != nil {
		return err
	}
	inputs := make([]generate.Input, 0, len(edit.Inputs))
	for _, path := range edit.Inputs {
		bytes, err := state.inputBytes(path)
		if err != nil {
			return err
		}
		inputs = append(inputs, generate.Input{Path: path, Bytes: bytes})
	}
	result, err := state.registry.Render(edit, inputs)
	if err != nil {
		return err
	}
	if existing := state.files[edit.Destination]; existing != nil && existing.file != nil {
		return fmt.Errorf("generator destination %s already has a structural AST output", edit.Destination)
	}
	value := state.files[edit.Destination]
	if value == nil {
		value = &fileState{path: edit.Destination}
		state.files[edit.Destination] = value
	}
	if value.source == nil {
		if source, sourceErr := state.workspace.File(edit.Destination); sourceErr == nil {
			value.source = append([]byte(nil), source.Source...)
		}
	}
	value.deleted = false
	value.generated = append([]byte(nil), result.Bytes...)
	value.file, value.info, value.fset = nil, nil, nil
	key := string(result.Evidence.Name)
	if prior, exists := state.providers[key]; exists && prior.identity != result.Evidence.Identity {
		return fmt.Errorf("provider %s returned conflicting implementation identities", key)
	}
	state.providers[key] = providerEvidence{name: key, identity: result.Evidence.Identity}
	return nil
}

func (state *renderState) inputBytes(path string) ([]byte, error) {
	if current := state.files[path]; current != nil {
		if current.deleted {
			return nil, fmt.Errorf("input is retired")
		}
		return current.bytes()
	}
	file, err := state.workspace.File(path)
	if err != nil {
		return nil, fmt.Errorf("not present in immutable semantic workspace")
	}
	return append([]byte(nil), file.Source...), nil
}

func canonicalProviders(values map[string]providerEvidence) []cutplan.ProviderEvidence {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]cutplan.ProviderEvidence, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		result = append(result, cutplan.ProviderEvidence{Name: cutplan.Provider(value.name), Identity: value.identity})
	}
	return result
}
