package program

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// observerCallTemplatePlan is transaction-local. Transformer terms remain
// owned by their Relation arena and are consumed only through relation-owned
// specialization; they are never retained in the program artifact.
type observerCallTemplatePlan struct {
	edge      lexicalObserverCallEdge
	templates []transformer.ObserverCallTemplate
}

type observerBodyTemplatePlan struct {
	ref   lexicalObserverTemplateRef
	calls []observerCallTemplatePlan
}

type observerProgramTemplatePlan struct {
	root      lexicalObserverTemplateRef
	bodies    []observerBodyTemplatePlan
	recursive bool
	templates int
}

type observerCallEdgeIdentity struct {
	point      cfg.Point
	occurrence observation.Occurrence
	target     lexicalObserverTemplateRef
}

func observerEdgeTargetRef(edge lexicalObserverCallEdge) (lexicalObserverTemplateRef, bool) {
	switch edge.Target.Kind {
	case lexicalObserverTemplateTarget:
		return edge.Target.Template, edge.Target.Template != (lexicalObserverTemplateRef{})
	case lexicalObserverMuTarget:
		return edge.Target.Mu.Target, edge.Target.Mu.Target != (lexicalObserverTemplateRef{}) && edge.Target.Mu.Anchor != (lexicalObserverTemplateRef{})
	default:
		return lexicalObserverTemplateRef{}, false
	}
}

// matchObserverCallTemplates binds the finite lexical forest to the exact
// owner-local call templates produced by the solved relation transaction.
// Matching is identity-only: it does not evaluate guards or terms and cannot
// create body solves, relation equations, or invocation paths.
func matchObserverCallTemplates(
	ctx context.Context,
	forest lexicalObserverForest,
	catalog relationRunCatalog,
	relations transformer.RelationSnapshot,
) (observerProgramTemplatePlan, error) {
	if ctx == nil {
		return observerProgramTemplatePlan{}, fmt.Errorf("observer program: context is required")
	}
	if err := ctx.Err(); err != nil {
		return observerProgramTemplatePlan{}, err
	}
	if err := forest.validatePublication(catalog); err != nil {
		return observerProgramTemplatePlan{}, err
	}

	entries := make(map[lexicalObserverTemplateRef]relationCatalogEntry, len(catalog.entries))
	entriesByCell := make(map[transformer.CellRef]relationCatalogEntry, len(catalog.entries))
	for _, entry := range catalog.entries {
		if entry.identity.Prepared == nil || entry.compiler == nil {
			return observerProgramTemplatePlan{}, fmt.Errorf("observer program: catalog has an incomplete template")
		}
		ref := lexicalObserverTemplateRef{Body: entry.identity.Prepared.StableLexicalBodyID(), Cell: entry.identity.Cell}
		if _, duplicate := entries[ref]; duplicate {
			return observerProgramTemplatePlan{}, fmt.Errorf("observer program: duplicate catalog template %v", ref.Cell)
		}
		entries[ref] = entry
		if _, duplicate := entriesByCell[ref.Cell]; duplicate {
			return observerProgramTemplatePlan{}, fmt.Errorf("observer program: duplicate catalog cell %v", ref.Cell)
		}
		entriesByCell[ref.Cell] = entry
	}

	plan := observerProgramTemplatePlan{root: forest.Root, bodies: make([]observerBodyTemplatePlan, 0, len(forest.Diagnostic))}
	for _, scc := range forest.SCCs {
		plan.recursive = plan.recursive || scc.Recursive
	}
	for _, bodyTemplate := range forest.Diagnostic {
		if err := ctx.Err(); err != nil {
			return observerProgramTemplatePlan{}, err
		}
		_, cataloged := entries[bodyTemplate.Ref]
		relation, solved := relations.Lookup(bodyTemplate.Ref.Cell)
		if !cataloged || !solved || relation.ContextualReason() != "" || relation.Widened() {
			return observerProgramTemplatePlan{}, fmt.Errorf("observer program: template %v has no exact solved relation", bodyTemplate.Ref.Cell)
		}
		bodyPlan := observerBodyTemplatePlan{ref: bodyTemplate.Ref, calls: make([]observerCallTemplatePlan, len(bodyTemplate.Calls))}
		edgeIndex := make(map[observerCallEdgeIdentity]int, len(bodyTemplate.Calls))
		for index, edge := range bodyTemplate.Calls {
			target, valid := observerEdgeTargetRef(edge)
			targetEntry, targetOwned := entries[target]
			if !valid || !targetOwned || targetEntry.compiler == nil {
				return observerProgramTemplatePlan{}, fmt.Errorf("observer program: template %v has an unowned lexical edge", bodyTemplate.Ref.Cell)
			}
			identity := observerCallEdgeIdentity{point: edge.Point, occurrence: edge.Occurrence, target: target}
			if _, duplicate := edgeIndex[identity]; duplicate {
				return observerProgramTemplatePlan{}, fmt.Errorf("observer program: template %v duplicates call edge %d", bodyTemplate.Ref.Cell, edge.Point)
			}
			edgeIndex[identity] = index
			bodyPlan.calls[index].edge = edge
		}

		for _, template := range relation.ObserverCallTemplates() {
			point, hasPoint := template.Point()
			target := template.Target()
			targetEntry, targetOwned := entriesByCell[target.Cell]
			if !hasPoint || template.Owner() != bodyTemplate.Ref.Body || !targetOwned || targetEntry.identity.Prepared == nil ||
				target.Shape != targetEntry.compiler.Shape() {
				return observerProgramTemplatePlan{}, fmt.Errorf("observer program: relation %v exported a foreign call template", bodyTemplate.Ref.Cell)
			}
			targetRef := lexicalObserverTemplateRef{Body: targetEntry.identity.Prepared.StableLexicalBodyID(), Cell: target.Cell}
			identity := observerCallEdgeIdentity{point: point, occurrence: template.Occurrence(), target: targetRef}
			index, matched := edgeIndex[identity]
			if !matched {
				return observerProgramTemplatePlan{}, fmt.Errorf("observer program: relation %v call %d has no exact forest edge", bodyTemplate.Ref.Cell, point)
			}
			bodyPlan.calls[index].templates = append(bodyPlan.calls[index].templates, template)
			plan.templates++
		}
		for _, call := range bodyPlan.calls {
			if len(call.templates) == 0 {
				return observerProgramTemplatePlan{}, fmt.Errorf("observer program: reachable edge %v point %d has no owner-local call template", bodyTemplate.Ref.Cell, call.edge.Point)
			}
		}
		plan.bodies = append(plan.bodies, bodyPlan)
	}
	return plan, nil
}
