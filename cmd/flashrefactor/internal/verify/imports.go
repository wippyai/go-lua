package verify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func verifyImports(beforeFiles, afterFiles map[string]SourceFile, routes []cutplan.Import) ([]ImportDelta, []ImportEdge, error) {
	before, err := importSpecs(beforeFiles, "pre-cut")
	if err != nil {
		return nil, nil, err
	}
	after, err := importSpecs(afterFiles, "post-cut")
	if err != nil {
		return nil, nil, err
	}
	beforeByConsumer := specsByConsumer(before)
	afterByConsumer := specsByConsumer(after)
	expectedByConsumer, err := routeDeltas(routes)
	if err != nil {
		return nil, nil, err
	}
	consumers := unionConsumerKeys(beforeByConsumer, afterByConsumer, expectedByConsumer)
	deltas := make([]ImportDelta, 0, len(consumers))
	for _, consumer := range consumers {
		beforeSet := beforeByConsumer[consumer]
		afterSet := afterByConsumer[consumer]
		removed := subtractSpecs(beforeSet, afterSet)
		added := subtractSpecs(afterSet, beforeSet)
		expected := expectedByConsumer[consumer]
		if !sameSpecs(expected.Removed, removed) || !sameSpecs(expected.Added, added) {
			return nil, nil, fmt.Errorf("declared import routes do not equal exact import-spec delta for %s: want -[%s] +[%s], got -[%s] +[%s]",
				consumer, importSpecList(expected.Removed), importSpecList(expected.Added), importSpecList(removed), importSpecList(added))
		}
		if len(removed) != 0 || len(added) != 0 {
			deltas = append(deltas, ImportDelta{Consumer: consumer, Removed: specList(removed), Added: specList(added)})
		}
	}
	graph := packageImportGraph(afterFiles, after)
	if cycle := importCycle(graph); len(cycle) != 0 {
		return nil, nil, fmt.Errorf("post-cut import graph contains cycle: %s", strings.Join(cycle, " -> "))
	}
	return deltas, edgeList(graph), nil
}

type importDeltaSet struct {
	Removed map[string]ImportSpec
	Added   map[string]ImportSpec
}

func routeDeltas(routes []cutplan.Import) (map[string]importDeltaSet, error) {
	result := map[string]importDeltaSet{}
	for _, route := range routes {
		if route.Consumer == "" || (route.From == nil && route.To == nil) {
			return nil, fmt.Errorf("declared import route needs a consumer and at least one endpoint")
		}
		delta, exists := result[route.Consumer]
		if !exists {
			delta = importDeltaSet{Removed: map[string]ImportSpec{}, Added: map[string]ImportSpec{}}
		}
		if route.From != nil {
			spec, err := declaredImportSpec(route.Consumer, *route.From)
			if err != nil {
				return nil, fmt.Errorf("declared source import for %s: %w", route.Consumer, err)
			}
			key := importSpecKey(spec)
			if _, duplicate := delta.Removed[key]; duplicate {
				return nil, fmt.Errorf("duplicate declared removed import %s", printableImportSpec(spec))
			}
			delta.Removed[key] = spec
		}
		if route.To != nil {
			spec, err := declaredImportSpec(route.Consumer, *route.To)
			if err != nil {
				return nil, fmt.Errorf("declared destination import for %s: %w", route.Consumer, err)
			}
			key := importSpecKey(spec)
			if _, duplicate := delta.Added[key]; duplicate {
				return nil, fmt.Errorf("duplicate declared added import %s", printableImportSpec(spec))
			}
			delta.Added[key] = spec
		}
		result[route.Consumer] = delta
	}
	return result, nil
}

func declaredImportSpec(consumer string, ref cutplan.ImportRef) (ImportSpec, error) {
	if ref.Path == "" {
		return ImportSpec{}, fmt.Errorf("empty import path")
	}
	// ImportSpec is intentionally the syntactic projection. The resolver owns
	// ref.Name; structural verification compares the raw alias spelling only.
	return ImportSpec{Consumer: consumer, Path: ref.Path, Alias: ref.Alias}, nil
}

func specsByConsumer(specs []ImportSpec) map[string]map[string]ImportSpec {
	result := map[string]map[string]ImportSpec{}
	for _, spec := range specs {
		set := result[spec.Consumer]
		if set == nil {
			set = map[string]ImportSpec{}
			result[spec.Consumer] = set
		}
		set[importSpecKey(spec)] = spec
	}
	return result
}

func unionConsumerKeys(before, after map[string]map[string]ImportSpec, expected map[string]importDeltaSet) []string {
	all := map[string]bool{}
	for _, set := range []map[string]map[string]ImportSpec{before, after} {
		for key := range set {
			all[key] = true
		}
	}
	for key := range expected {
		all[key] = true
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func subtractSpecs(left, right map[string]ImportSpec) map[string]ImportSpec {
	result := map[string]ImportSpec{}
	for key, spec := range left {
		if _, exists := right[key]; !exists {
			result[key] = spec
		}
	}
	return result
}

func sameSpecs(left, right map[string]ImportSpec) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, exists := right[key]; !exists {
			return false
		}
	}
	return true
}

func specList(specs map[string]ImportSpec) []ImportSpec {
	result := make([]ImportSpec, 0, len(specs))
	for _, spec := range specs {
		result = append(result, spec)
	}
	sort.Slice(result, func(i, j int) bool { return importSpecKey(result[i]) < importSpecKey(result[j]) })
	return result
}

func importSpecList(specs map[string]ImportSpec) string {
	values := specList(specs)
	result := make([]string, len(values))
	for index, spec := range values {
		result[index] = printableImportSpec(spec)
	}
	return strings.Join(result, ",")
}

func importSpecKey(spec ImportSpec) string {
	return spec.Consumer + "\x00" + spec.Path + "\x00" + spec.Alias
}

func printableImportSpec(spec ImportSpec) string {
	return spec.Consumer + ":" + spec.Alias + "\"" + spec.Path + "\""
}

func packageImportGraph(files map[string]SourceFile, specs []ImportSpec) map[string]ImportEdge {
	byPath := map[string]SourceFile{}
	for path, file := range files {
		byPath[path] = file
	}
	result := map[string]ImportEdge{}
	for _, spec := range specs {
		from := byPath[spec.Consumer].Package
		if from == "" || from == spec.Path {
			continue
		}
		edge := ImportEdge{From: from, To: spec.Path}
		result[edge.From+"\x00"+edge.To] = edge
	}
	return result
}

func edgeList(edges map[string]ImportEdge) []ImportEdge {
	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ImportEdge, len(keys))
	for index, key := range keys {
		result[index] = edges[key]
	}
	return result
}

func importCycle(edges map[string]ImportEdge) []string {
	children := map[string][]string{}
	for _, edge := range edges {
		children[edge.From] = append(children[edge.From], edge.To)
		if _, exists := children[edge.To]; !exists {
			children[edge.To] = nil
		}
	}
	for from := range children {
		sort.Strings(children[from])
	}
	state, stack, index := map[string]uint8{}, []string{}, map[string]int{}
	var visit func(string) []string
	visit = func(node string) []string {
		state[node], index[node] = 1, len(stack)
		stack = append(stack, node)
		for _, child := range children[node] {
			switch state[child] {
			case 0:
				if cycle := visit(child); len(cycle) != 0 {
					return cycle
				}
			case 1:
				cycle := append([]string(nil), stack[index[child]:]...)
				return append(cycle, child)
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
		return nil
	}
	nodes := make([]string, 0, len(children))
	for node := range children {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if state[node] == 0 {
			if cycle := visit(node); len(cycle) != 0 {
				return cycle
			}
		}
	}
	return nil
}
