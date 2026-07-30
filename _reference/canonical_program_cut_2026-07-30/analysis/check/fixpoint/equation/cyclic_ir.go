package equation

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// CellID is the opaque, stable identity of a frozen cyclic equation cell.
// It is intentionally not a State coordinate or a solver generation.
type CellID string

func (id CellID) valid() bool { return id != "" }

// EdgeReason identifies why a semantic dependency is retained.  It is part of
// the freeze certificate: a consumer may not turn a missing reason into an
// omitted read.
type EdgeReason string

const (
	EdgeContractRead        EdgeReason = "contract-read"
	EdgeContractWrite       EdgeReason = "contract-write"
	EdgeContractKill        EdgeReason = "contract-kill"
	EdgeContractAdvance     EdgeReason = "contract-advance"
	EdgeContractGuard       EdgeReason = "contract-guard"
	EdgeContractOutcome     EdgeReason = "contract-outcome"
	EdgePairedApply         EdgeReason = "paired-apply"
	EdgePublishedRead       EdgeReason = "published-read"
	EdgePathEquality        EdgeReason = "path-equality"
	EdgeAllocationPlacement EdgeReason = "allocation-placement"
	EdgeCrossAxisReduction  EdgeReason = "cross-axis-reduction"
	EdgeEntryParameter      EdgeReason = "entry-parameter"
)

func (reason EdgeReason) valid() bool {
	switch reason {
	case EdgeContractRead, EdgeContractWrite, EdgeContractKill, EdgeContractAdvance,
		EdgeContractGuard, EdgeContractOutcome, EdgePairedApply, EdgePublishedRead,
		EdgePathEquality, EdgeAllocationPlacement, EdgeCrossAxisReduction, EdgeEntryParameter:
		return true
	default:
		return false
	}
}

// SemanticDependency is directed producer-to-consumer.  Multiple reasons for
// the same pair are retained; contracts are evidence, not a lossy edge set.
type SemanticDependency struct {
	From   CellID
	To     CellID
	Reason EdgeReason
	// Evidence is the source-owned contract/influence spelling retained for
	// audits. It is descriptive only; Reason remains the stable wire class.
	Evidence string
}

// OutputSelector is a consumer-visible result surface.  Selectors are mapped
// to cells before slicing; they are not themselves an execution shortcut.
type OutputSelector struct {
	ID    string
	Cells []CellID
}

// CyclicArtifact is the entry-independent frozen certificate used by the
// Stage-4 VM.  Plan is copied by solve and is the production-owned schedule;
// callers must never derive a replacement plan from Dependencies.
type CyclicArtifact struct {
	Artifact       Artifact
	CellForTarget  map[Coordinate]CellID
	Plan           *solve.WTOPlan[CellID]
	Dependencies   []SemanticDependency
	Selectors      []OutputSelector
	ParameterCells []CellID
}

// NewCyclicArtifact validates a complete, closed graph certificate.  The
// graph is deliberately fail-closed: every planned cell must map to exactly
// one lowered equation and every semantic edge must be scheduled by the frozen
// WTO.
func NewCyclicArtifact(
	artifact Artifact,
	cellForTarget map[Coordinate]CellID,
	plan *solve.WTOPlan[CellID],
	dependencies []SemanticDependency,
	selectors []OutputSelector,
	parameterCells []CellID,
) (CyclicArtifact, error) {
	if artifact.CanonicalBytes() == nil || len(artifact.Equations) == 0 || plan == nil {
		return CyclicArtifact{}, fmt.Errorf("equation: malformed cyclic artifact")
	}
	if len(cellForTarget) != len(artifact.Equations) {
		return CyclicArtifact{}, fmt.Errorf("equation: cyclic artifact has incomplete cell map")
	}
	cells := make([]CellID, 0, len(artifact.Equations))
	seen := make(map[CellID]struct{}, len(artifact.Equations))
	for _, equation := range artifact.Equations {
		cell, ok := cellForTarget[equation.Target]
		if !ok || !cell.valid() {
			return CyclicArtifact{}, fmt.Errorf("equation: cyclic artifact has no cell for %s", equation.Target.Name)
		}
		if _, duplicate := seen[cell]; duplicate {
			return CyclicArtifact{}, fmt.Errorf("equation: cyclic artifact has duplicate cell %q", cell)
		}
		seen[cell] = struct{}{}
		cells = append(cells, cell)
	}
	if !plan.Matches(cells) {
		return CyclicArtifact{}, fmt.Errorf("equation: cyclic artifact WTO does not match lowered cells")
	}
	for _, edge := range dependencies {
		if !edge.From.valid() || !edge.To.valid() || !edge.Reason.valid() {
			return CyclicArtifact{}, fmt.Errorf("equation: malformed semantic dependency")
		}
		if _, ok := seen[edge.From]; !ok {
			return CyclicArtifact{}, fmt.Errorf("equation: semantic dependency has foreign source %q", edge.From)
		}
		if _, ok := seen[edge.To]; !ok || !plan.CoversInfluence(edge.From, edge.To) {
			return CyclicArtifact{}, fmt.Errorf("equation: semantic dependency is not covered by frozen WTO")
		}
	}
	selectorIDs := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		if selector.ID == "" {
			return CyclicArtifact{}, fmt.Errorf("equation: selector has no identity")
		}
		if _, duplicate := selectorIDs[selector.ID]; duplicate {
			return CyclicArtifact{}, fmt.Errorf("equation: duplicate selector %q", selector.ID)
		}
		selectorIDs[selector.ID] = struct{}{}
		for _, cell := range selector.Cells {
			if _, ok := seen[cell]; !ok {
				return CyclicArtifact{}, fmt.Errorf("equation: selector %q has foreign cell", selector.ID)
			}
		}
	}
	for _, cell := range parameterCells {
		if _, ok := seen[cell]; !ok {
			return CyclicArtifact{}, fmt.Errorf("equation: parameter footprint has foreign cell")
		}
	}
	out := CyclicArtifact{Artifact: artifact, CellForTarget: make(map[Coordinate]CellID, len(cellForTarget)), Plan: plan,
		Dependencies: append([]SemanticDependency(nil), dependencies...), Selectors: cloneSelectors(selectors), ParameterCells: append([]CellID(nil), parameterCells...)}
	for target, cell := range cellForTarget {
		out.CellForTarget[target] = cell
	}
	sort.Slice(out.Dependencies, func(i, j int) bool {
		if out.Dependencies[i].From != out.Dependencies[j].From {
			return out.Dependencies[i].From < out.Dependencies[j].From
		}
		if out.Dependencies[i].To != out.Dependencies[j].To {
			return out.Dependencies[i].To < out.Dependencies[j].To
		}
		if out.Dependencies[i].Reason != out.Dependencies[j].Reason {
			return out.Dependencies[i].Reason < out.Dependencies[j].Reason
		}
		return out.Dependencies[i].Evidence < out.Dependencies[j].Evidence
	})
	sort.Slice(out.ParameterCells, func(i, j int) bool { return out.ParameterCells[i] < out.ParameterCells[j] })
	return out, nil
}

func cloneSelectors(in []OutputSelector) []OutputSelector {
	out := make([]OutputSelector, len(in))
	for i, selector := range in {
		out[i] = OutputSelector{ID: selector.ID, Cells: append([]CellID(nil), selector.Cells...)}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Demand returns the semantic reverse-reachable seed cells for selectors.
// SCC closure and exact schedule restriction are intentionally delegated to
// solve.RestrictWTOPlan so this function cannot re-plan a body.
func (a CyclicArtifact) Demand(selectorIDs []string) ([]CellID, error) {
	if a.Plan == nil || len(selectorIDs) == 0 {
		return nil, fmt.Errorf("equation: empty cyclic demand")
	}
	wanted := make(map[string]struct{}, len(selectorIDs))
	for _, id := range selectorIDs {
		wanted[id] = struct{}{}
	}
	selected := make(map[CellID]struct{})
	found := 0
	for _, selector := range a.Selectors {
		if _, ok := wanted[selector.ID]; !ok {
			continue
		}
		found++
		for _, cell := range selector.Cells {
			selected[cell] = struct{}{}
		}
	}
	if found != len(wanted) {
		return nil, fmt.Errorf("equation: unknown output selector")
	}
	pred := make(map[CellID][]CellID, len(a.CellForTarget))
	for _, edge := range a.Dependencies {
		pred[edge.To] = append(pred[edge.To], edge.From)
	}
	queue := make([]CellID, 0, len(selected))
	for cell := range selected {
		queue = append(queue, cell)
	}
	for len(queue) != 0 {
		cell := queue[0]
		queue = queue[1:]
		for _, source := range pred[cell] {
			if _, seen := selected[source]; seen {
				continue
			}
			selected[source] = struct{}{}
			queue = append(queue, source)
		}
	}
	out := make([]CellID, 0, len(selected))
	for cell := range selected {
		out = append(out, cell)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// RestrictPlan turns selector demand into an SCC-closed frozen schedule. It
// delegates to solve.RestrictWTOPlan, which copies the selected structure but
// never performs another graph decomposition.
func (a CyclicArtifact) RestrictPlan(selectorIDs []string) (*solve.WTOPlan[CellID], error) {
	demand, err := a.Demand(selectorIDs)
	if err != nil {
		return nil, err
	}
	return solve.RestrictWTOPlan(a.Plan, demand)
}

// CanonicalBytes returns the complete frozen cyclic certificate.  In
// particular it retains the cell directory and WTO nesting: Artifact alone is
// insufficient authority for a caller to select a different fixed-point
// schedule.  The result contains no solve generation or process-local map
// iteration order.
func (a CyclicArtifact) CanonicalBytes() []byte {
	// Re-run the constructor's closed-world checks before publishing an
	// identity.  CyclicArtifact has exported fields for the benefit of the VM,
	// so an invalid manually-built value must never acquire a content ID.
	canonical, err := NewCyclicArtifact(a.Artifact, a.CellForTarget, a.Plan, a.Dependencies, a.Selectors, a.ParameterCells)
	if err != nil {
		return nil
	}
	a = canonical
	artifact := a.Artifact.CanonicalBytes()
	if artifact == nil {
		return nil
	}
	out := appendText(nil, "cyclic-equation-artifact/content-v2")
	out = appendBytes(out, artifact)

	type cellTarget struct {
		target Coordinate
		cell   CellID
	}
	targets := make([]cellTarget, 0, len(a.CellForTarget))
	for target, cell := range a.CellForTarget {
		targets = append(targets, cellTarget{target: target, cell: cell})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].target.less(targets[j].target) })
	out = appendU64(out, uint64(len(targets)))
	for _, item := range targets {
		out = append(out, item.target.Body[:]...)
		out = appendText(out, item.target.Name)
		out = appendText(out, string(item.cell))
	}

	var appendElements func([]solve.WTOElement[CellID])
	appendElements = func(elements []solve.WTOElement[CellID]) {
		out = appendU64(out, uint64(len(elements)))
		for _, element := range elements {
			out = appendText(out, string(element.Vertex))
			if element.Body == nil {
				out = append(out, 0)
				continue
			}
			out = append(out, 1)
			appendElements(element.Body)
		}
	}
	appendElements(a.Plan.Elements())
	influences := a.Plan.Influences()
	out = appendU64(out, uint64(len(influences)))
	for _, influence := range influences {
		out = appendText(out, string(influence.From))
		out = appendText(out, string(influence.To))
	}

	out = appendU64(out, uint64(len(a.Dependencies)))
	for _, dependency := range a.Dependencies {
		out = appendText(out, string(dependency.From))
		out = appendText(out, string(dependency.To))
		out = appendText(out, string(dependency.Reason))
		out = appendText(out, dependency.Evidence)
	}
	out = appendU64(out, uint64(len(a.Selectors)))
	for _, selector := range a.Selectors {
		out = appendText(out, selector.ID)
		out = appendU64(out, uint64(len(selector.Cells)))
		for _, cell := range selector.Cells {
			out = appendText(out, string(cell))
		}
	}
	return appendCellIDs(out, a.ParameterCells)
}

// ContentID identifies the complete frozen graph and schedule, not merely its
// equation syntax.
func (a CyclicArtifact) ContentID() ContentID {
	encoded := a.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

func appendCellIDs(out []byte, cells []CellID) []byte {
	out = appendU64(out, uint64(len(cells)))
	for _, cell := range cells {
		out = appendText(out, string(cell))
	}
	return out
}
