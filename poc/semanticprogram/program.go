// Package semanticprogram is an isolated acceptance contract for compiling a
// prepared body into one syntax-free semantic program. It indexes existing
// operationplan payloads; it does not duplicate or symbolically interpret
// them.
package semanticprogram

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Family identifies semantic work outside operationplan's factflow catalog.
type Family string

const (
	GenericForCheck       Family = "body.generic-for-check"
	GenericForVariable    Family = "body.generic-for-variable"
	BoundaryObservation   Family = "observation.boundary-output"
	NodeObservation       Family = "observation.node-output"
	EdgeObservation       Family = "observation.edge-reachability"
	ExpressionFunctionDep Family = "source.expression-function"
)

// Role distinguishes executable transitions, sidecars owned by another
// transaction, source dependencies, and post-fixpoint observation consumers.
type Role uint8

const (
	Executable Role = iota + 1
	Sidecar
	Dependency
	Observation
)

// PayloadRef is a stable reference into an owner-retained immutable payload
// store. The program never retains AST nodes or copies semantic payloads.
type PayloadRef struct {
	Store string
	Key   uint64
}

// LayerDecl describes semantic work owned above factflow.
type LayerDecl struct {
	Point   cfg.Point
	To      cfg.Point // edge-observation destination; zero otherwise
	Family  Family
	Role    Role
	Owner   Family
	Payload PayloadRef
}

// Cell is one canonical factflow transaction or body-layer declaration.
type Cell struct {
	Point   cfg.Point
	Kind    operationplan.Kind
	Layer   Family
	Role    Role
	Owner   Family
	Phase   operationplan.Phase
	Barrier operationplan.Barrier
	Payload PayloadRef
}

// Program retains only references into immutable owner stores.
type Program struct {
	Rows         [][]Cell
	Dependencies []LayerDecl
	Observations []LayerDecl
	Missing      []Family
}

// MissingError reports every executable family without a concrete transaction
// handler. Compilation still returns the exhaustive metadata program so the
// caller can inspect the exact fail-closed boundary.
type MissingError struct{ Families []Family }

func (e MissingError) Error() string {
	return fmt.Sprintf("semanticprogram: missing concrete families: %v", e.Families)
}

// Compile builds a deterministic, exhaustive metadata program. supported is
// the set of body-layer executable families with proven concrete handlers;
// factflow cells are already handled by NodePoint/BranchEdge transactions.
func Compile(graph cfg.Graph, facts *operationplan.Plan, layers []LayerDecl, supported map[Family]bool) (Program, error) {
	if graph == nil || facts == nil || facts.PointCount() != graph.Size() {
		return Program{}, fmt.Errorf("semanticprogram: graph and matching operation plan are required")
	}
	p := Program{Rows: make([][]Cell, graph.Size())}
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		cursor := facts.Cursor(point)
		for fact, ok := cursor.Next(); ok; fact, ok = cursor.Next() {
			meta, described := operationplan.Describe(fact.Kind())
			if !described {
				return Program{}, fmt.Errorf("semanticprogram: unclassified fact kind %d at %d", fact.Kind(), point)
			}
			role := Executable
			if meta.Class == operationplan.CompositeSidecar {
				role = Sidecar
			}
			p.Rows[point] = append(p.Rows[point], Cell{Point: point, Kind: fact.Kind(), Role: role, Phase: meta.Phase, Barrier: meta.Barrier,
				Payload: PayloadRef{Store: "factflow", Key: uint64(point)}})
		}
	}
	seen := make(map[struct {
		point cfg.Point
		to    cfg.Point
		kind  Family
		key   uint64
	}]struct{}, len(layers))
	missing := make(map[Family]struct{})
	for _, decl := range layers {
		if decl.Family == "" || decl.Role == 0 || uint64(decl.Point) >= uint64(graph.Size()) {
			return Program{}, fmt.Errorf("semanticprogram: invalid layer declaration %+v", decl)
		}
		key := struct {
			point cfg.Point
			to    cfg.Point
			kind  Family
			key   uint64
		}{decl.Point, decl.To, decl.Family, decl.Payload.Key}
		if _, duplicate := seen[key]; duplicate {
			return Program{}, fmt.Errorf("semanticprogram: duplicate declaration %s at %d", decl.Family, decl.Point)
		}
		seen[key] = struct{}{}
		switch decl.Role {
		case Dependency, Sidecar:
			p.Dependencies = append(p.Dependencies, decl)
		case Observation:
			p.Observations = append(p.Observations, decl)
		case Executable:
			p.Rows[decl.Point] = append(p.Rows[decl.Point], Cell{Point: decl.Point, Layer: decl.Family, Role: decl.Role, Owner: decl.Owner, Payload: decl.Payload})
			if !supported[decl.Family] {
				missing[decl.Family] = struct{}{}
			}
		default:
			return Program{}, fmt.Errorf("semanticprogram: invalid role for %s", decl.Family)
		}
	}
	for family := range missing {
		p.Missing = append(p.Missing, family)
	}
	sort.Slice(p.Missing, func(i, j int) bool { return p.Missing[i] < p.Missing[j] })
	if len(p.Missing) != 0 {
		return p, MissingError{Families: append([]Family(nil), p.Missing...)}
	}
	return p, nil
}
