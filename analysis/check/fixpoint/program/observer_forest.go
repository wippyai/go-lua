package program

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// lexicalObserverTemplateRef is the stable identity of one body/cell
// template. It deliberately contains no invocation-path identity: callers of
// the same lexical body share this template.
type lexicalObserverTemplateRef struct {
	Body lexicalidentity.StableLexicalBodyID
	Cell transformer.CellRef
}

func compareLexicalObserverTemplateRef(left, right lexicalObserverTemplateRef) int {
	if order := bytes.Compare(left.Body[:], right.Body[:]); order != 0 {
		return order
	}
	if left.Cell.Function < right.Cell.Function {
		return -1
	}
	if left.Cell.Function > right.Cell.Function {
		return 1
	}
	if left.Cell.Slot < right.Cell.Slot {
		return -1
	}
	if left.Cell.Slot > right.Cell.Slot {
		return 1
	}
	return 0
}

// lexicalObserverMuRef is a canonical reference into one recursive equation
// family. Anchor is the least template in the SCC and Target is the equation
// selected by the call. This finite reference replaces invocation-path
// unfolding; it does not attach values, guards, or diagnostic meaning.
type lexicalObserverMuRef struct {
	Anchor lexicalObserverTemplateRef
	Target lexicalObserverTemplateRef
}

type lexicalObserverCallTargetKind uint8

const (
	lexicalObserverTemplateTarget lexicalObserverCallTargetKind = iota + 1
	lexicalObserverMuTarget
)

type lexicalObserverCallTarget struct {
	Kind     lexicalObserverCallTargetKind
	Template lexicalObserverTemplateRef
	Mu       lexicalObserverMuRef
}

// lexicalObserverCallEdge owns only preparation-sealed lexical identity.
// Point is the caller CFG location; Occurrence is its durable diagnostic
// anchor. Neither field is reconstructed from source text.
type lexicalObserverCallEdge struct {
	Point      cfg.Point
	Occurrence observation.Occurrence
	Target     lexicalObserverCallTarget
}

type lexicalObserverTemplate struct {
	Ref   lexicalObserverTemplateRef
	Calls []lexicalObserverCallEdge
}

type lexicalObserverSCC struct {
	Anchor    lexicalObserverTemplateRef
	Members   []lexicalObserverTemplateRef
	Recursive bool
}

// lexicalObserverWork describes retained graph size, not solver work. Its
// exact accounting makes accidental path expansion visible in tests and
// metrics: retained size is templates plus lexical calls.
type lexicalObserverWork struct {
	CatalogTemplates int
	CallSitesScanned int
	DiagnosticNodes  int
	DiagnosticEdges  int
}

func (w lexicalObserverWork) RetainedGraphUnits() int {
	return w.DiagnosticNodes + w.DiagnosticEdges
}

// lexicalObserverForest is a structural projection of a sealed relation
// catalog. Diagnostic contains only templates reachable from the unique chunk
// root. Uncalled is a separate policy input for declared-contract validation;
// it never creates a diagnostic invocation instance.
type lexicalObserverForest struct {
	Root       lexicalObserverTemplateRef
	Diagnostic []lexicalObserverTemplate
	SCCs       []lexicalObserverSCC
	Uncalled   []lexicalObserverTemplateRef
	Work       lexicalObserverWork
}

type lexicalObserverCatalogNode struct {
	ref      lexicalObserverTemplateRef
	entry    relationCatalogEntry
	callRefs []lexicalObserverCatalogEdge
}

type lexicalObserverCatalogEdge struct {
	point      cfg.Point
	occurrence observation.Occurrence
	target     int
}

// buildLexicalObserverForest projects a total, sealed relation catalog into a
// deterministic finite lexical observer graph. It performs no relation
// evaluation, body solve, summary read, or diagnostic publication.
func buildLexicalObserverForest(ctx context.Context, catalog relationRunCatalog) (lexicalObserverForest, error) {
	if ctx == nil {
		return lexicalObserverForest{}, fmt.Errorf("observer forest: context is required")
	}
	if err := ctx.Err(); err != nil {
		return lexicalObserverForest{}, err
	}
	if catalog.generation == nil || len(catalog.entries) == 0 {
		return lexicalObserverForest{}, fmt.Errorf("observer forest: total sealed catalog is required")
	}

	nodes := make([]lexicalObserverCatalogNode, len(catalog.entries))
	root := -1
	for index, entry := range catalog.entries {
		if err := ctx.Err(); err != nil {
			return lexicalObserverForest{}, err
		}
		if !entry.equationIdentityMatches(catalog.generation) || entry.identity.Prepared == nil || entry.compiler == nil || entry.identity.Cell == (transformer.CellRef{}) {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: catalog entry %d has incoherent sealed identity", index)
		}
		bodyID := entry.identity.Prepared.StableLexicalBodyID()
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: cell %v has no lexical body identity", entry.identity.Cell)
		}
		nodes[index] = lexicalObserverCatalogNode{ref: lexicalObserverTemplateRef{Body: bodyID, Cell: entry.identity.Cell}, entry: entry}
	}

	// Canonicalize before constructing any indexes. Catalog insertion order is
	// intentionally not an observable property of the forest.
	sort.Slice(nodes, func(i, j int) bool {
		return compareLexicalObserverTemplateRef(nodes[i].ref, nodes[j].ref) < 0
	})
	cellIndex := make(map[transformer.CellRef]int, len(nodes))
	for index := range nodes {
		if index != 0 && compareLexicalObserverTemplateRef(nodes[index-1].ref, nodes[index].ref) == 0 {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: duplicate body/cell template %v", nodes[index].ref.Cell)
		}
		if _, duplicate := cellIndex[nodes[index].ref.Cell]; duplicate {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: cell %v has multiple lexical bodies", nodes[index].ref.Cell)
		}
		cellIndex[nodes[index].ref.Cell] = index
		if nodes[index].entry.function == nil {
			if root >= 0 {
				return lexicalObserverForest{}, fmt.Errorf("observer forest: catalog has more than one chunk root")
			}
			root = index
		}
	}
	if root < 0 {
		return lexicalObserverForest{}, fmt.Errorf("observer forest: catalog has no unique chunk root")
	}

	work := lexicalObserverWork{CatalogTemplates: len(nodes)}
	for index := range nodes {
		if err := ctx.Err(); err != nil {
			return lexicalObserverForest{}, err
		}
		entry := nodes[index].entry
		plan := entry.identity.Prepared.OperationPlan()
		if plan == nil || plan.ObservationBody() != nodes[index].ref.Body || entry.direct.PointCount() != plan.PointCount() {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: cell %v plan identity is incomplete", entry.identity.Cell)
		}
		surface, ok := plan.CallSurface()
		if !ok || !surface.Complete() || surface.Owner() != nodes[index].ref.Body || surface.PointCount() != plan.PointCount() {
			return lexicalObserverForest{}, fmt.Errorf("observer forest: cell %v call surface is incomplete", entry.identity.Cell)
		}
		sites := surface.Sites()
		work.CallSitesScanned += len(sites)
		for _, site := range sites {
			if err := ctx.Err(); err != nil {
				return lexicalObserverForest{}, err
			}
			if site.Target.Kind() != operationplan.CallSurfaceTargetLexical {
				continue
			}
			targetBody, lexical := site.Target.LexicalBody()
			route, routed := entry.direct.Lookup(site.Point)
			targetByCell, ownedCell := cellIndex[route.Cell]
			occurrence, anchored := plan.CallInvocationObservationAnchor(site.Point)
			if !lexical || !routed || !ownedCell || nodes[targetByCell].ref.Body != targetBody ||
				route.Shape != nodes[targetByCell].entry.compiler.Shape() || !anchored || !occurrence.Valid() || occurrence.Kind != observation.CallInvocation {
				return lexicalObserverForest{}, fmt.Errorf("observer forest: cell %v point %d has no exact sealed lexical target", entry.identity.Cell, site.Point)
			}
			nodes[index].callRefs = append(nodes[index].callRefs, lexicalObserverCatalogEdge{
				point: site.Point, occurrence: occurrence, target: targetByCell,
			})
		}
		sort.Slice(nodes[index].callRefs, func(i, j int) bool {
			left, right := nodes[index].callRefs[i], nodes[index].callRefs[j]
			if left.point != right.point {
				return left.point < right.point
			}
			if left.occurrence != right.occurrence {
				return left.occurrence.Less(right.occurrence)
			}
			return compareLexicalObserverTemplateRef(nodes[left.target].ref, nodes[right.target].ref) < 0
		})
	}

	return assembleLexicalObserverForest(ctx, nodes, root, work)
}

func assembleLexicalObserverForest(ctx context.Context, nodes []lexicalObserverCatalogNode, root int, work lexicalObserverWork) (lexicalObserverForest, error) {
	if root < 0 || root >= len(nodes) {
		return lexicalObserverForest{}, fmt.Errorf("observer forest: invalid root index %d", root)
	}
	nodes, root, err := canonicalizeLexicalObserverCatalogNodes(nodes, root)
	if err != nil {
		return lexicalObserverForest{}, err
	}
	reachable := make([]bool, len(nodes))
	stack := []int{root}
	for len(stack) != 0 {
		if err := ctx.Err(); err != nil {
			return lexicalObserverForest{}, err
		}
		last := len(stack) - 1
		index := stack[last]
		stack = stack[:last]
		if reachable[index] {
			continue
		}
		reachable[index] = true
		for edge := len(nodes[index].callRefs) - 1; edge >= 0; edge-- {
			target := nodes[index].callRefs[edge].target
			if target < 0 || target >= len(nodes) {
				return lexicalObserverForest{}, fmt.Errorf("observer forest: template %v has invalid target %d", nodes[index].ref.Cell, target)
			}
			if !reachable[target] {
				stack = append(stack, target)
			}
		}
	}

	componentOf, components, err := lexicalObserverComponents(ctx, nodes, reachable)
	if err != nil {
		return lexicalObserverForest{}, err
	}
	sccs := make([]lexicalObserverSCC, 0, len(components))
	recursive := make([]bool, len(components))
	anchors := make([]lexicalObserverTemplateRef, len(components))
	for component, members := range components {
		sort.Ints(members)
		refs := make([]lexicalObserverTemplateRef, len(members))
		for index, member := range members {
			refs[index] = nodes[member].ref
		}
		isRecursive := len(members) > 1
		if len(members) == 1 {
			for _, edge := range nodes[members[0]].callRefs {
				if edge.target == members[0] {
					isRecursive = true
					break
				}
			}
		}
		anchors[component] = refs[0]
		recursive[component] = isRecursive
		sccs = append(sccs, lexicalObserverSCC{Anchor: refs[0], Members: refs, Recursive: isRecursive})
	}
	sort.Slice(sccs, func(i, j int) bool {
		return compareLexicalObserverTemplateRef(sccs[i].Anchor, sccs[j].Anchor) < 0
	})

	forest := lexicalObserverForest{Root: nodes[root].ref, SCCs: sccs, Work: work}
	for index := range nodes {
		if !reachable[index] {
			forest.Uncalled = append(forest.Uncalled, nodes[index].ref)
			continue
		}
		template := lexicalObserverTemplate{Ref: nodes[index].ref, Calls: make([]lexicalObserverCallEdge, 0, len(nodes[index].callRefs))}
		for _, edge := range nodes[index].callRefs {
			if !reachable[edge.target] {
				return lexicalObserverForest{}, fmt.Errorf("observer forest: reachable template %v points outside reachable closure", nodes[index].ref.Cell)
			}
			targetRef := nodes[edge.target].ref
			target := lexicalObserverCallTarget{Kind: lexicalObserverTemplateTarget, Template: targetRef}
			component := componentOf[index]
			if component == componentOf[edge.target] && recursive[component] {
				target = lexicalObserverCallTarget{Kind: lexicalObserverMuTarget, Mu: lexicalObserverMuRef{Anchor: anchors[component], Target: targetRef}}
			}
			template.Calls = append(template.Calls, lexicalObserverCallEdge{Point: edge.point, Occurrence: edge.occurrence, Target: target})
			forest.Work.DiagnosticEdges++
		}
		forest.Diagnostic = append(forest.Diagnostic, template)
		forest.Work.DiagnosticNodes++
	}
	return forest, nil
}

func canonicalizeLexicalObserverCatalogNodes(nodes []lexicalObserverCatalogNode, root int) ([]lexicalObserverCatalogNode, int, error) {
	order := make([]int, len(nodes))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(i, j int) bool {
		return compareLexicalObserverTemplateRef(nodes[order[i]].ref, nodes[order[j]].ref) < 0
	})
	remap := make([]int, len(nodes))
	canonical := make([]lexicalObserverCatalogNode, len(nodes))
	for target, source := range order {
		if target != 0 && compareLexicalObserverTemplateRef(nodes[order[target-1]].ref, nodes[source].ref) == 0 {
			return nil, 0, fmt.Errorf("observer forest: duplicate canonical template %v", nodes[source].ref.Cell)
		}
		remap[source] = target
		canonical[target] = nodes[source]
		canonical[target].callRefs = append([]lexicalObserverCatalogEdge(nil), nodes[source].callRefs...)
	}
	for index := range canonical {
		for edge := range canonical[index].callRefs {
			target := canonical[index].callRefs[edge].target
			if target < 0 || target >= len(nodes) {
				return nil, 0, fmt.Errorf("observer forest: template %v has invalid target %d", canonical[index].ref.Cell, target)
			}
			canonical[index].callRefs[edge].target = remap[target]
		}
		sort.Slice(canonical[index].callRefs, func(i, j int) bool {
			left, right := canonical[index].callRefs[i], canonical[index].callRefs[j]
			if left.point != right.point {
				return left.point < right.point
			}
			if left.occurrence != right.occurrence {
				return left.occurrence.Less(right.occurrence)
			}
			return compareLexicalObserverTemplateRef(canonical[left.target].ref, canonical[right.target].ref) < 0
		})
	}
	return canonical, remap[root], nil
}

// lexicalObserverComponents computes SCCs iteratively. Avoiding recursive DFS
// keeps graph construction total for deeply nested lexical programs without
// introducing a depth budget.
func lexicalObserverComponents(ctx context.Context, nodes []lexicalObserverCatalogNode, reachable []bool) ([]int, [][]int, error) {
	type frame struct{ node, next int }
	visited := make([]bool, len(nodes))
	finish := make([]int, 0, len(nodes))
	for start := range nodes {
		if !reachable[start] || visited[start] {
			continue
		}
		visited[start] = true
		frames := []frame{{node: start}}
		for len(frames) != 0 {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			top := &frames[len(frames)-1]
			if top.next < len(nodes[top.node].callRefs) {
				target := nodes[top.node].callRefs[top.next].target
				top.next++
				if reachable[target] && !visited[target] {
					visited[target] = true
					frames = append(frames, frame{node: target})
				}
				continue
			}
			finish = append(finish, top.node)
			frames = frames[:len(frames)-1]
		}
	}

	reverse := make([][]int, len(nodes))
	for source := range nodes {
		if !reachable[source] {
			continue
		}
		for _, edge := range nodes[source].callRefs {
			if reachable[edge.target] {
				reverse[edge.target] = append(reverse[edge.target], source)
			}
		}
	}
	for index := range reverse {
		sort.Ints(reverse[index])
	}

	componentOf := make([]int, len(nodes))
	for index := range componentOf {
		componentOf[index] = -1
	}
	components := make([][]int, 0)
	for order := len(finish) - 1; order >= 0; order-- {
		start := finish[order]
		if componentOf[start] >= 0 {
			continue
		}
		component := len(components)
		members := make([]int, 0, 1)
		pending := []int{start}
		componentOf[start] = component
		for len(pending) != 0 {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			last := len(pending) - 1
			node := pending[last]
			pending = pending[:last]
			members = append(members, node)
			for edge := len(reverse[node]) - 1; edge >= 0; edge-- {
				predecessor := reverse[node][edge]
				if componentOf[predecessor] < 0 {
					componentOf[predecessor] = component
					pending = append(pending, predecessor)
				}
			}
		}
		components = append(components, members)
	}
	return componentOf, components, nil
}
