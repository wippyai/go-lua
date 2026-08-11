package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func (compiler *compiler) emit(values []Provenance) error {
	for _, value := range values {
		key := provenanceKey(value)
		if _, exists := compiler.emitted[key]; exists {
			return fmt.Errorf("authored element was consumed more than once: %s", key)
		}
		compiler.emitted[key] = struct{}{}
		compiler.state.provenance = append(compiler.state.provenance, cloneProvenance(value))
	}
	return nil
}

func (compiler *compiler) emitRoutes(operation cutplan.Operation) error {
	for _, binding := range operation.Bindings {
		if err := compiler.emit([]Provenance{{
			Operation: operation.ID, Kind: ProvenanceBinding, From: binding.From, To: binding.To,
			Paths: []string{binding.Consumer}, Receiver: append([]cutplan.ReceiverPathStep(nil), binding.Receiver...),
		}}); err != nil {
			return err
		}
	}
	for _, route := range operation.Imports {
		if err := compiler.emit([]Provenance{{
			Operation: operation.ID, Kind: ProvenanceImport, Objects: append([]cutplan.SymbolRef(nil), route.Symbols...),
			Paths: []string{route.Consumer}, ImportFrom: cloneImport(route.From), ImportTo: cloneImport(route.To),
		}}); err != nil {
			return err
		}
	}
	return nil
}

func canonicalProvenance(values []Provenance) []Provenance {
	result := make([]Provenance, 0, len(values))
	for _, value := range values {
		result = append(result, cloneProvenance(value))
	}
	sort.Slice(result, func(i, j int) bool { return provenanceKey(result[i]) < provenanceKey(result[j]) })
	return result
}

func cloneProvenance(value Provenance) Provenance {
	value.Objects = append([]cutplan.SymbolRef(nil), value.Objects...)
	value.Paths = append([]string(nil), value.Paths...)
	value.Receiver = append([]cutplan.ReceiverPathStep(nil), value.Receiver...)
	value.Containment = cloneContainment(value.Containment)
	value.ImportFrom = cloneImport(value.ImportFrom)
	value.ImportTo = cloneImport(value.ImportTo)
	sort.Slice(value.Objects, func(i, j int) bool { return value.Objects[i].Object < value.Objects[j].Object })
	sort.Strings(value.Paths)
	return value
}

func provenanceKey(value Provenance) string {
	objects := make([]string, len(value.Objects))
	for index, object := range value.Objects {
		objects[index] = object.Object
	}
	paths := append([]string(nil), value.Paths...)
	receiver := make([]string, len(value.Receiver))
	for index, step := range value.Receiver {
		receiver[index] = string(step.Kind) + ":" + step.Object.Object
	}
	containment := ""
	if value.Containment != nil {
		containment = value.Containment.Parent.Object + "\x01" + value.Containment.Child.Object + "\x01" + value.Containment.Through.Object
	}
	importFrom, importTo := importKey(value.ImportFrom), importKey(value.ImportTo)
	sort.Strings(objects)
	sort.Strings(paths)
	sort.Strings(receiver)
	return strings.Join([]string{
		value.Operation, string(value.Kind), value.From.Object, value.To.Object,
		strings.Join(objects, "\x01"), strings.Join(paths, "\x01"), strings.Join(receiver, "\x01"),
		containment, importFrom, importTo, string(value.Provider),
	}, "\x00")
}

func cloneContainment(value *cutplan.Containment) *cutplan.Containment {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneImport(value *cutplan.ImportRef) *cutplan.ImportRef {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func importKey(value *cutplan.ImportRef) string {
	if value == nil {
		return ""
	}
	return value.Path + "\x01" + value.Name + "\x01" + value.Alias
}
