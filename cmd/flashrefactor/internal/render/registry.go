package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// compiler is a capability-minimal visitor context. Handlers receive detached
// syntax plus reviewed semantic objects only; there is deliberately no root
// path, executor, session, lock, or process handle to accidentally use.
type compiler struct {
	state        *renderState
	requirements requirementIndex
	registry     handlerRegistry
	consumed     map[string]string
	emitted      map[string]struct{}
	snapshot     semantic.Snapshot
	witnesses    []capturedWitness
}

// consumedSite is a proof/report row: a handler states exactly which reviewed
// declaration or destination it owns. The compiler rejects a second handler
// claiming the same semantic site, even if its file footprint happened to be
// ordered. This keeps hidden overlap from turning into last-writer-wins.
type consumedSite struct {
	key    string
	detail string
}

type editHandler interface {
	name() string
	recognize(cutplan.Edit) bool
	preflight(*compiler, cutplan.Operation, cutplan.Edit) ([]consumedSite, error)
	rewrite(*compiler, cutplan.Operation, cutplan.Edit) error
	provenance(cutplan.Operation, cutplan.Edit) []Provenance
}

type relocationHandler interface {
	name() string
	recognize(cutplan.Relocate) bool
	preflight(*compiler, cutplan.Operation, cutplan.Relocate) ([]consumedSite, error)
	rewrite(*compiler, cutplan.Operation, cutplan.Relocate) error
}

// handlerRegistry is immutable after construction. It is intentionally not a
// plugin API: adding a language form requires a source handler and its laws,
// never runtime registration, reflection, or a compatibility adapter.
type handlerRegistry struct {
	edits       []editHandler
	relocations []relocationHandler
}

func builtinRegistry() handlerRegistry {
	return handlerRegistry{
		edits: []editHandler{
			relocateEdit{},
			retireEdit{},
			generateEdit{},
		},
		relocations: []relocationHandler{
			containmentRelocation{},
			declarationRelocation{},
		},
	}
}

func (registry handlerRegistry) edit(edit cutplan.Edit) (editHandler, error) {
	var found editHandler
	for _, handler := range registry.edits {
		if !handler.recognize(edit) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("two edit handlers recognize %s: %s and %s", edit.Kind, found.name(), handler.name())
		}
		found = handler
	}
	if found == nil {
		return nil, fmt.Errorf("no edit handler recognizes %s", edit.Kind)
	}
	return found, nil
}

func (registry handlerRegistry) relocation(edit cutplan.Relocate) (relocationHandler, error) {
	var found relocationHandler
	for _, handler := range registry.relocations {
		if !handler.recognize(edit) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("two relocation handlers recognize %s: %s and %s", edit.Source, found.name(), handler.name())
		}
		found = handler
	}
	if found == nil {
		return nil, fmt.Errorf("no relocation AST-form handler recognizes %s", edit.Source)
	}
	return found, nil
}

func (compiler *compiler) claim(operation string, sites []consumedSite) error {
	for _, site := range sites {
		if prior, exists := compiler.consumed[site.key]; exists {
			if prior == operation && strings.HasPrefix(site.key, "path:") {
				continue // One reviewed operation may legitimately edit its source and route it.
			}
			return fmt.Errorf("hidden overlapping handler ownership of %s: %s and %s", site.detail, prior, operation)
		}
	}
	for _, site := range sites {
		compiler.consumed[site.key] = operation
	}
	return nil
}

func canonicalSites(values []consumedSite) []consumedSite {
	result := append([]consumedSite(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].key < result[j].key })
	return result
}

type relocateEdit struct{}

func (relocateEdit) name() string { return "relocate-edit" }
func (relocateEdit) recognize(edit cutplan.Edit) bool {
	return edit.Kind == cutplan.EditRelocate && edit.Relocate != nil
}
func (relocateEdit) preflight(compiler *compiler, operation cutplan.Operation, edit cutplan.Edit) ([]consumedSite, error) {
	handler, err := compiler.registry.relocation(*edit.Relocate)
	if err != nil {
		return nil, err
	}
	return handler.preflight(compiler, operation, *edit.Relocate)
}
func (relocateEdit) rewrite(compiler *compiler, operation cutplan.Operation, edit cutplan.Edit) error {
	handler, err := compiler.registry.relocation(*edit.Relocate)
	if err != nil {
		return err
	}
	return handler.rewrite(compiler, operation, *edit.Relocate)
}
func (relocateEdit) provenance(operation cutplan.Operation, edit cutplan.Edit) []Provenance {
	result := make([]Provenance, 0, len(edit.Relocate.Subjects))
	for _, subject := range edit.Relocate.Subjects {
		result = append(result, Provenance{Operation: operation.ID, Kind: ProvenanceRelocate, From: subject.From, To: subject.To,
			Paths: []string{edit.Relocate.Source, edit.Relocate.Destination.Path}, Containment: cloneContainment(edit.Relocate.Containment)})
	}
	return result
}

type containmentRelocation struct{}

func (containmentRelocation) name() string                         { return "containment-field-relocation" }
func (containmentRelocation) recognize(edit cutplan.Relocate) bool { return edit.Containment != nil }
func (containmentRelocation) preflight(compiler *compiler, operation cutplan.Operation, edit cutplan.Relocate) ([]consumedSite, error) {
	if err := compiler.state.preflightContainment(compiler.requirements, operation, edit); err != nil {
		return nil, err
	}
	sites := []consumedSite{{key: "path:" + edit.Source, detail: edit.Source}, {key: "path:" + edit.Destination.Path, detail: edit.Destination.Path}}
	for _, subject := range edit.Subjects {
		sites = append(sites, consumedSite{key: "object:" + subject.From.Object, detail: subject.From.Object})
	}
	return canonicalSites(sites), nil
}
func (containmentRelocation) rewrite(compiler *compiler, operation cutplan.Operation, edit cutplan.Relocate) error {
	return compiler.state.relocateContainment(compiler.requirements, operation, edit)
}

type declarationRelocation struct{}

func (declarationRelocation) name() string                         { return "whole-declaration-relocation" }
func (declarationRelocation) recognize(edit cutplan.Relocate) bool { return edit.Containment == nil }
func (declarationRelocation) preflight(compiler *compiler, _ cutplan.Operation, edit cutplan.Relocate) ([]consumedSite, error) {
	if err := compiler.state.preflightDeclarations(compiler.requirements, edit); err != nil {
		return nil, err
	}
	sites := []consumedSite{{key: "path:" + edit.Source, detail: edit.Source}, {key: "path:" + edit.Destination.Path, detail: edit.Destination.Path}}
	for _, subject := range edit.Subjects {
		sites = append(sites, consumedSite{key: "object:" + subject.From.Object, detail: subject.From.Object})
	}
	return canonicalSites(sites), nil
}
func (declarationRelocation) rewrite(compiler *compiler, _ cutplan.Operation, edit cutplan.Relocate) error {
	return compiler.state.relocateDeclarations(compiler.requirements, edit)
}

type retireEdit struct{}

func (retireEdit) name() string { return "retire-edit" }
func (retireEdit) recognize(edit cutplan.Edit) bool {
	return edit.Kind == cutplan.EditRetire && edit.Retire != nil
}
func (retireEdit) preflight(compiler *compiler, _ cutplan.Operation, edit cutplan.Edit) ([]consumedSite, error) {
	if err := compiler.state.preflightRetire(compiler.requirements, *edit.Retire); err != nil {
		return nil, err
	}
	sites := []consumedSite{{key: "path:" + edit.Retire.Source, detail: edit.Retire.Source}}
	for _, ref := range edit.Retire.Symbols {
		sites = append(sites, consumedSite{key: "object:" + ref.Object, detail: ref.Object})
	}
	return canonicalSites(sites), nil
}
func (retireEdit) rewrite(compiler *compiler, _ cutplan.Operation, edit cutplan.Edit) error {
	return compiler.state.retire(compiler.requirements, *edit.Retire)
}
func (retireEdit) provenance(operation cutplan.Operation, edit cutplan.Edit) []Provenance {
	result := make([]Provenance, 0, len(edit.Retire.Symbols))
	for _, object := range edit.Retire.Symbols {
		result = append(result, Provenance{Operation: operation.ID, Kind: ProvenanceRetire, From: object, Paths: []string{edit.Retire.Source}})
	}
	return result
}

type generateEdit struct{}

func (generateEdit) name() string { return "registered-generator" }
func (generateEdit) recognize(edit cutplan.Edit) bool {
	return edit.Kind == cutplan.EditGenerate && edit.Generate != nil
}
func (generateEdit) preflight(compiler *compiler, _ cutplan.Operation, edit cutplan.Edit) ([]consumedSite, error) {
	if err := compiler.state.preflightGenerate(*edit.Generate); err != nil {
		return nil, err
	}
	return []consumedSite{{key: "path:" + edit.Generate.Destination, detail: edit.Generate.Destination}}, nil
}
func (generateEdit) rewrite(compiler *compiler, _ cutplan.Operation, edit cutplan.Edit) error {
	return compiler.state.generate(*edit.Generate)
}
func (generateEdit) provenance(operation cutplan.Operation, edit cutplan.Edit) []Provenance {
	paths := append([]string(nil), edit.Generate.Inputs...)
	paths = append(paths, edit.Generate.Destination)
	return []Provenance{{Operation: operation.ID, Kind: ProvenanceGenerate, Paths: paths, Provider: edit.Generate.Provider}}
}
