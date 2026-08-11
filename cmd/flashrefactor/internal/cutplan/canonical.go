package cutplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CanonicalIntent returns a deterministic topological representation.  The
// list order of independent operations and any unordered declarative list has
// no semantic meaning and therefore cannot alter its digest.
func CanonicalIntent(intent Intent) (Intent, error) {
	if err := ValidateIntent(intent); err != nil {
		return Intent{}, err
	}
	result := intent
	result.Operations = append([]Operation(nil), intent.Operations...)
	order, err := topologicalOrder(result.Operations)
	if err != nil {
		return Intent{}, err
	}
	ordered := make([]Operation, len(result.Operations))
	for index, source := range order {
		ordered[index] = result.Operations[source]
	}
	result.Operations = ordered
	for index := range result.Operations {
		canonicalOperation(&result.Operations[index])
	}
	return result, nil
}

func canonicalOperation(operation *Operation) {
	operation.After = sorted(operation.After)
	operation.Edits = append([]Edit(nil), operation.Edits...)
	for index := range operation.Edits {
		canonicalEdit(&operation.Edits[index])
	}
	sort.Slice(operation.Edits, func(left, right int) bool { return editKey(operation.Edits[left]) < editKey(operation.Edits[right]) })

	operation.Bindings = append([]Binding(nil), operation.Bindings...)
	for index := range operation.Bindings {
		operation.Bindings[index].Receiver = append([]ReceiverPathStep(nil), operation.Bindings[index].Receiver...)
	}
	sort.Slice(operation.Bindings, func(left, right int) bool {
		return bindingKey(operation.Bindings[left]) < bindingKey(operation.Bindings[right])
	})

	operation.Imports = append([]Import(nil), operation.Imports...)
	for index := range operation.Imports {
		operation.Imports[index].Symbols = canonicalSymbols(operation.Imports[index].Symbols)
	}
	sort.Slice(operation.Imports, func(left, right int) bool {
		return importKey(operation.Imports[left]) < importKey(operation.Imports[right])
	})
	operation.Footprint.Read = sorted(operation.Footprint.Read)
	operation.Footprint.Write = sorted(operation.Footprint.Write)
	operation.Verify.Laws = append([]Law(nil), operation.Verify.Laws...)
	sort.Slice(operation.Verify.Laws, func(left, right int) bool {
		return lawKey(operation.Verify.Laws[left]) < lawKey(operation.Verify.Laws[right])
	})
	sort.Slice(operation.Verify.Gates, func(left, right int) bool { return operation.Verify.Gates[left] < operation.Verify.Gates[right] })
}

func canonicalEdit(edit *Edit) {
	if edit.Relocate != nil {
		edit.Relocate.Subjects = append([]Relocation(nil), edit.Relocate.Subjects...)
		sort.Slice(edit.Relocate.Subjects, func(left, right int) bool {
			return relocationKey(edit.Relocate.Subjects[left]) < relocationKey(edit.Relocate.Subjects[right])
		})
	}
	if edit.Retire != nil {
		edit.Retire.Symbols = canonicalSymbols(edit.Retire.Symbols)
	}
	if edit.Generate != nil {
		edit.Generate.Inputs = sorted(edit.Generate.Inputs)
	}
}

func canonicalSymbols(values []SymbolRef) []SymbolRef {
	result := append([]SymbolRef(nil), values...)
	sort.Slice(result, func(left, right int) bool { return result[left].Object < result[right].Object })
	return result
}

// IntentBytes returns the exact bytes used for an intent digest.
func IntentBytes(intent Intent) ([]byte, error) {
	canonical, err := CanonicalIntent(intent)
	if err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

// IntentDigest computes the canonical SHA-256 commitment.
func IntentDigest(intent Intent) (string, error) {
	data, err := IntentBytes(intent)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func topologicalOrder(operations []Operation) ([]int, error) {
	byID := make(map[string]int, len(operations))
	for index, operation := range operations {
		if operation.ID == "" {
			return nil, fmt.Errorf("operation has empty id")
		}
		if _, exists := byID[operation.ID]; exists {
			return nil, fmt.Errorf("duplicate operation id %q", operation.ID)
		}
		byID[operation.ID] = index
	}
	indegree := make([]int, len(operations))
	next := make([][]int, len(operations))
	for index, operation := range operations {
		seen := map[string]bool{}
		for _, predecessor := range operation.After {
			dependency, exists := byID[predecessor]
			if !exists {
				return nil, fmt.Errorf("operation %s depends on unknown operation %s", operation.ID, predecessor)
			}
			if dependency == index {
				return nil, fmt.Errorf("operation %s depends on itself", operation.ID)
			}
			if seen[predecessor] {
				return nil, fmt.Errorf("operation %s repeats dependency %s", operation.ID, predecessor)
			}
			seen[predecessor] = true
			indegree[index]++
			next[dependency] = append(next[dependency], index)
		}
	}
	ready := make([]int, 0, len(operations))
	for index := range operations {
		if indegree[index] == 0 {
			ready = append(ready, index)
		}
	}
	sort.Slice(ready, func(left, right int) bool { return operations[ready[left]].ID < operations[ready[right]].ID })
	order := make([]int, 0, len(operations))
	for len(ready) != 0 {
		index := ready[0]
		ready = ready[1:]
		order = append(order, index)
		for _, dependent := range next[index] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sort.Slice(ready, func(left, right int) bool { return operations[ready[left]].ID < operations[ready[right]].ID })
	}
	if len(order) != len(operations) {
		return nil, fmt.Errorf("operation dependency graph has a cycle")
	}
	return order, nil
}

func canonicalEvidence(evidence LockEvidence) LockEvidence {
	result := evidence
	result.Inputs.Files = append([]HashPath(nil), evidence.Inputs.Files...)
	sort.Slice(result.Inputs.Files, func(left, right int) bool { return result.Inputs.Files[left].Path < result.Inputs.Files[right].Path })
	result.Inputs.Absent = sorted(evidence.Inputs.Absent)
	result.Resolution.Objects = append([]ObjectEvidence(nil), evidence.Resolution.Objects...)
	for index := range result.Resolution.Objects {
		result.Resolution.Objects[index].Definition = canonicalPosition(evidence.Resolution.Objects[index].Definition)
		result.Resolution.Objects[index].References = append([]Position(nil), result.Resolution.Objects[index].References...)
		for site := range result.Resolution.Objects[index].References {
			result.Resolution.Objects[index].References[site] = canonicalPosition(evidence.Resolution.Objects[index].References[site])
		}
		sort.Slice(result.Resolution.Objects[index].References, func(left, right int) bool {
			return positionKey(result.Resolution.Objects[index].References[left]) < positionKey(result.Resolution.Objects[index].References[right])
		})
	}
	sort.Slice(result.Resolution.Objects, func(left, right int) bool {
		return result.Resolution.Objects[left].Object.Object < result.Resolution.Objects[right].Object.Object
	})
	result.Resolution.Providers = append([]ProviderEvidence(nil), evidence.Resolution.Providers...)
	sort.Slice(result.Resolution.Providers, func(left, right int) bool {
		return result.Resolution.Providers[left].Name < result.Resolution.Providers[right].Name
	})
	result.Routes = append([]ReferenceRoute(nil), evidence.Routes...)
	for index := range result.Routes {
		result.Routes[index].Sites = append([]ReferenceSiteRoute(nil), evidence.Routes[index].Sites...)
		for site := range result.Routes[index].Sites {
			result.Routes[index].Sites[site].Source = canonicalPosition(evidence.Routes[index].Sites[site].Source)
			result.Routes[index].Sites[site].Target = canonicalPosition(evidence.Routes[index].Sites[site].Target)
		}
		sort.Slice(result.Routes[index].Sites, func(left, right int) bool {
			return referenceSiteRouteKey(result.Routes[index].Sites[left]) < referenceSiteRouteKey(result.Routes[index].Sites[right])
		})
	}
	sort.Slice(result.Routes, func(left, right int) bool {
		return routeKey(result.Routes[left].From, result.Routes[left].To) < routeKey(result.Routes[right].From, result.Routes[right].To)
	})
	result.Gates = append([]GateEvidence(nil), evidence.Gates...)
	sort.Slice(result.Gates, func(left, right int) bool { return result.Gates[left].Gate < result.Gates[right].Gate })
	result.Hazards = append([]Hazard(nil), evidence.Hazards...)
	for index := range result.Hazards {
		result.Hazards[index].Paths = sorted(result.Hazards[index].Paths)
	}
	sort.Slice(result.Hazards, func(left, right int) bool { return hazardKey(result.Hazards[left]) < hazardKey(result.Hazards[right]) })
	result.Execution.Touched = sorted(evidence.Execution.Touched)
	result.Execution.Deleted = sorted(evidence.Execution.Deleted)
	result.Execution.Outputs = append([]HashPath(nil), evidence.Execution.Outputs...)
	sort.Slice(result.Execution.Outputs, func(left, right int) bool {
		return result.Execution.Outputs[left].Path < result.Execution.Outputs[right].Path
	})
	return result
}

func canonicalPosition(value Position) Position {
	value.PackageIDs = append([]string(nil), value.PackageIDs...)
	return value
}

func editKey(edit Edit) string {
	switch edit.Kind {
	case EditRelocate:
		return relocateKey(*edit.Relocate)
	case EditRetire:
		return "retire\x00" + edit.Retire.Source + "\x00" + symbolListKey(edit.Retire.Symbols)
	case EditGenerate:
		return "generate\x00" + string(edit.Generate.Provider) + "\x00" + strings.Join(sorted(edit.Generate.Inputs), "\x00") + "\x00" + edit.Generate.Destination
	default:
		return string(edit.Kind)
	}
}

func relocateKey(value Relocate) string {
	subjects := make([]string, 0, len(value.Subjects))
	for _, subject := range value.Subjects {
		subjects = append(subjects, relocationKey(subject))
	}
	containment := ""
	if value.Containment != nil {
		containment = value.Containment.Parent.Object + "\x00" + value.Containment.Child.Object + "\x00" + value.Containment.Through.Object
	}
	return "relocate\x00" + value.Source + "\x00" + value.Destination.Path + "\x00" + value.Destination.Package + "\x00" + strings.Join(sorted(subjects), "\x00") + "\x00" + containment
}

func relocationKey(value Relocation) string { return value.From.Object + "\x00" + value.To.Object }
func routeKey(from, to SymbolRef) string    { return from.Object + "\x00" + to.Object }
func referenceSiteRouteKey(value ReferenceSiteRoute) string {
	return positionKey(value.Source) + "\x00" + positionKey(value.Target)
}

func bindingKey(value Binding) string {
	parts := []string{value.Consumer, value.From.Object, value.To.Object, string(value.Form)}
	for _, step := range value.Receiver {
		parts = append(parts, string(step.Kind)+":"+step.Object.Object)
	}
	return strings.Join(parts, "\x00")
}

func importKey(value Import) string {
	from, to := "", ""
	if value.From != nil {
		from = value.From.Path + "\x00" + value.From.Name + "\x00" + value.From.Alias
	}
	if value.To != nil {
		to = value.To.Path + "\x00" + value.To.Name + "\x00" + value.To.Alias
	}
	return value.Consumer + "\x00" + from + "\x00" + to + "\x00" + symbolListKey(value.Symbols)
}

func lawKey(value Law) string {
	return value.ID + "\x00" + value.Package + "\x00" + value.Test
}
func positionKey(value Position) string {
	return strings.Join(value.PackageIDs, "\x00") + "\x00" + value.Path + "\x00" + strconv.Itoa(value.Offset) + "\x00" + strconv.Itoa(value.Line) + "\x00" + strconv.Itoa(value.Column) + "\x00" + string(value.Role)
}
func hazardKey(value Hazard) string {
	return value.Code + "\x00" + value.Severity + "\x00" + value.Detail
}
