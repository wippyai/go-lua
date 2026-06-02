package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/propagate"
)

// buildConditionDemand derives the SSA-version liveness demand that drives
// path-condition projection in the propagation worklist.
//
// Projection forgets condition facts whose referenced access-path versions are
// dead. Soundness requires that every fact still relevant to a downstream read
// stays live, so the demand enumerates every real read site recorded in the
// extracted inputs:
//
//   - branch/guard reads: paths referenced by edge conditions, demanded both at
//     the branch (From, where the guard is evaluated) and at the target (To,
//     where the narrowing applies);
//   - ordinary expression reads: assignment source paths, table/map/container
//     mutator target/value/key paths, demanded at the read point;
//   - phi operands: each operand version demanded at its predecessor edge;
//   - return-summary extraction: every visible param/upvalue/global root version
//     demanded at return/exit points, so interprocedural return summaries retain
//     their refinements.
//
// Def sites (whole-variable assignment targets and phi targets) kill downward
// demand for that defined version, so a reassigned variable's old version dies
// after its last use.
//
// It is built only for the checker pipeline (which sets Decomposer and records
// real reads); inputs without that read evidence propagate the path condition
// exactly, subject only to FVS widening.
func buildConditionDemand(inputs *Inputs) *propagate.Demand {
	if inputs == nil || inputs.Graph == nil || inputs.Decomposer == nil {
		return nil
	}
	g := inputs.Graph

	uses := make(map[cfg.Point][]constraint.Path)
	defs := make(map[cfg.Point][]cfg.Version)

	addUse := func(p cfg.Point, path constraint.Path) {
		if path.Symbol == 0 {
			return
		}
		uses[p] = append(uses[p], path)
	}

	// Branch/guard reads. A guard's narrowing applies on its target edge, so the
	// referenced paths are demanded both at the branch (From, where the guard is
	// evaluated) and at the target (To, where the narrowing takes effect). The
	// To-demand alone is not sufficient: it would make the guard live only at the
	// successor, but a guard whose narrowed path is read deeper in the then-region
	// (not at the immediate successor) must stay live across that region. Recording
	// the read sites inside the region (via ConditionExtraReads below) supplies
	// that; the From/To demand seeds the guard's own evaluation.
	for _, ec := range inputs.EdgeConditions {
		cond := ec.Condition
		for i := 0; i < cond.NumDisjuncts(); i++ {
			for _, lit := range cond.DisjunctConstraints(i) {
				for _, path := range constraint.SemanticAffectedPaths(lit) {
					addUse(ec.From, path)
					addUse(ec.To, path)
				}
			}
		}
	}

	// Ordinary expression reads via assignment sources.
	for _, a := range inputs.Assignments {
		addUse(a.Point, a.Source.Path)
		addUse(a.Point, a.Source.ContainerPath)
		addUse(a.Point, a.Source.MapPath)
		addUse(a.Point, a.Source.CalleePath)
		addUse(a.Point, a.Source.ReceiverPath)
		for _, op := range a.Source.Operands {
			addUse(a.Point, op.Path)
		}
		// Whole-variable assignment target defines a new SSA root version.
		if len(a.TargetPath.Segments) == 0 && a.TargetPath.Symbol != 0 {
			if ver := g.VisibleVersion(a.Point, a.TargetPath.Symbol); !ver.IsZero() {
				defs[a.Point] = append(defs[a.Point], ver)
			}
		}
	}

	// Table mutator reads (table.insert-like and map writes).
	for _, m := range inputs.TableMutatorAssignments {
		addUse(m.Point, m.Target)
		addUse(m.Point, m.ValuePath)
		if m.KeySymbol != 0 {
			addUse(m.Point, constraint.Path{Root: m.KeyVar, Symbol: m.KeySymbol})
		}
	}
	for _, m := range inputs.MapMutatorAssignments {
		addUse(m.Point, m.Target)
		addUse(m.Point, m.ValuePath)
		if m.KeySymbol != 0 {
			addUse(m.Point, constraint.Path{Root: m.KeyVar, Symbol: m.KeySymbol})
		}
	}
	// Phi operands and targets.
	for _, phi := range g.PhiNodes() {
		for _, op := range phi.Operands {
			if op.Version.Symbol == 0 {
				continue
			}
			addUse(op.From, versionPath(op.Version))
		}
		if phi.Target.Symbol != 0 && phi.Target.ID != 0 {
			defs[phi.Point] = append(defs[phi.Point], phi.Target)
		}
	}

	// Field-precise expression reads the flow inputs do not otherwise encode:
	// every variable/field/index access read anywhere in the body (operands,
	// index keys, nested call arguments, return and branch expressions, table
	// constructors), supplied by the checker via ExtractConditionReads.
	for p, paths := range inputs.ConditionExtraReads {
		for _, path := range paths {
			addUse(p, path)
		}
	}

	// Return-summary extraction: keep params/upvalues/globals live at returns.
	for _, p := range g.RPO() {
		node := g.Node(p)
		if node == nil {
			continue
		}
		if node.Kind != cfg.NodeReturn && node.Kind != cfg.NodeExit {
			continue
		}
		for sym, ver := range g.AllVisibleVersions(p) {
			if sym == 0 || ver.IsZero() || !returnSummaryKind(g, sym) {
				continue
			}
			addUse(p, versionPath(ver))
		}
	}

	return &propagate.Demand{Uses: uses, Defs: defs}
}

func returnSummaryKind(g cfg.VersionedGraph, sym cfg.SymbolID) bool {
	k, ok := g.SymbolKind(sym)
	if !ok {
		return true
	}
	switch k {
	case cfg.SymbolParam, cfg.SymbolUpvalue, cfg.SymbolGlobal:
		return true
	default:
		return false
	}
}

func versionPath(v cfg.Version) constraint.Path {
	return constraint.Path{Root: v.Root, Symbol: v.Symbol, Version: v.ID}
}
