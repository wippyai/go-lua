package transformer

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// FormalLexicalBodyCoordinates is the detached scalar publication of one
// lexical body after the forest-wide formal WTO has stabilized. There is one
// value per lexical body, irrespective of how many Apply sites target it.
//
// The scalar State values are materialized only from the named publication
// cells retained by relationCode. CallOutcomes are read from their formal
// producers; a closed signature allocation is projected from its already-
// published transaction output. This value owns no route, equation, provider,
// or replay capability.
type FormalLexicalBodyCoordinates struct {
	Body                lexicalidentity.StableLexicalBodyID
	PointInputs         map[cfg.Point]state.State
	PlannedNodeOutputs  map[cfg.Point]state.State
	PointReachable      map[cfg.Point]bool
	NodeOutputReachable map[cfg.Point]bool
	EdgeNormal          map[cfg.Edge]bool
	CallOutcomes        map[cfg.Point]callpayload.CallOutcome
	DiagnosticOutput    callpayload.DiagnosticOutput
	Calls               []FormalLexicalCallDependency
}

// FormalLexicalCallDependency is one frozen lexical Apply edge. Occurrence is
// the stable source-order occurrence at Point; no scheduler route participates
// in result lineage.
type FormalLexicalCallDependency struct {
	Point      cfg.Point
	Occurrence uint32
	Target     lexicalidentity.StableLexicalBodyID
}

// projectFormalLexicalBodies projects one already-completed canonical formal
// execution into exactly one detached record per lexical body. It deliberately
// does not execute a RelationProgram: the sole Solve entry owns WTO, validated
// Apply-observation detachment, and this publication in one transaction.
func projectFormalLexicalBodies(
	ctx context.Context,
	execution *formalRelationExecution,
) ([]FormalLexicalBodyCoordinates, error) {
	if ctx == nil || execution == nil || execution.algebra == nil || execution.algebra.program == nil {
		return nil, fmt.Errorf("transformer: formal lexical publication is unowned")
	}
	views, err := formalLexicalPublicationViews(execution)
	if err != nil {
		return nil, err
	}
	out := make([]FormalLexicalBodyCoordinates, len(views))
	for index := range views {
		view := views[index]
		var publicationErr error
		out[index], publicationErr = view.ProjectLexicalBody(ctx)
		if publicationErr != nil {
			return nil, publicationErr
		}
	}
	return out, nil
}

func formalLexicalPublicationViews(execution *formalRelationExecution) ([]FormalRelationPublicationView, error) {
	if execution == nil || execution.algebra == nil || execution.algebra.program == nil {
		return nil, fmt.Errorf("transformer: formal lexical publication is unowned")
	}
	p := execution.algebra.program
	out := make([]FormalRelationPublicationView, len(p.bodies))
	for index := range p.bodies {
		view, err := execution.Publication(p.bodies[index].body)
		if err != nil {
			return nil, err
		}
		out[index] = view
	}
	return out, nil
}

// ProjectLexicalBody forgets correlation independently at each named Result
// publication coordinate. It never enumerates invocation routes and never
// evaluates an equation or transfer.
func (v *FormalRelationPublicationView) ProjectLexicalBody(ctx context.Context) (FormalLexicalBodyCoordinates, error) {
	if ctx == nil || v == nil || v.execution == nil || v.body == nil || v.body.graph == nil {
		return FormalLexicalBodyCoordinates{}, fmt.Errorf("transformer: formal lexical body publication is unowned")
	}
	if err := ctx.Err(); err != nil {
		return FormalLexicalBodyCoordinates{}, err
	}
	out := FormalLexicalBodyCoordinates{
		Body:                v.body.body,
		PointInputs:         make(map[cfg.Point]state.State, len(v.pointInput)),
		PlannedNodeOutputs:  make(map[cfg.Point]state.State, len(v.pointOutput)),
		PointReachable:      make(map[cfg.Point]bool, len(v.pointInput)),
		NodeOutputReachable: make(map[cfg.Point]bool, len(v.pointOutput)),
		EdgeNormal:          make(map[cfg.Edge]bool, len(v.edgeNormal)),
		CallOutcomes:        make(map[cfg.Point]callpayload.CallOutcome),
		Calls:               append([]FormalLexicalCallDependency(nil), v.calls...),
	}

	// The structural freezer publishes every CFG point in canonical RPO. A map
	// miss here is therefore malformed publication metadata, not unreachable
	// flow. Unreachable points retain the exact owning-domain Bottom.
	for _, point := range cfg.RPOReadOnly(v.body.graph) {
		coordinates, declared := v.pointInput[point]
		if !declared {
			return FormalLexicalBodyCoordinates{}, fmt.Errorf("transformer: formal lexical body %s has no input cell for point %d", v.body.body, point)
		}
		value, reachable := v.body.domain.Bottom(), false
		if len(coordinates) != 0 {
			var err error
			value, reachable, err = v.joinPublishedCoordinates(ctx, coordinates)
			if err != nil {
				return FormalLexicalBodyCoordinates{}, err
			}
		}
		out.PointInputs[point], out.PointReachable[point] = value, reachable

		outputs, declared := v.pointOutput[point]
		if !declared {
			return FormalLexicalBodyCoordinates{}, fmt.Errorf("transformer: formal lexical body %s has no output cell for point %d", v.body.body, point)
		}
		value, reachable = v.body.domain.Bottom(), false
		if len(outputs) != 0 {
			var err error
			value, reachable, err = v.joinPublishedCoordinates(ctx, outputs)
			if err != nil {
				return FormalLexicalBodyCoordinates{}, err
			}
		}
		out.PlannedNodeOutputs[point], out.NodeOutputReachable[point] = value, reachable
	}

	edges := make([]cfg.Edge, 0, len(v.edgeNormal))
	for edge := range v.edgeNormal {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	for _, edge := range edges {
		reachable := false
		if coordinates := v.edgeNormal[edge]; len(coordinates) != 0 {
			var err error
			_, reachable, err = v.joinPublishedCoordinates(ctx, coordinates)
			if err != nil {
				return FormalLexicalBodyCoordinates{}, err
			}
		}
		out.EdgeNormal[edge] = reachable
	}

	callPoints := make([]cfg.Point, 0, len(v.callSites))
	for point := range v.callSites {
		callPoints = append(callPoints, point)
	}
	sort.Slice(callPoints, func(i, j int) bool { return callPoints[i] < callPoints[j] })
	for _, point := range callPoints {
		output, outputDeclared := out.PlannedNodeOutputs[point]
		outputReachable, outputReachabilityDeclared := out.NodeOutputReachable[point]
		pointReachable, pointReachabilityDeclared := out.PointReachable[point]
		if !outputDeclared || !outputReachabilityDeclared || !pointReachabilityDeclared {
			return FormalLexicalBodyCoordinates{}, fmt.Errorf("transformer: formal lexical body %s has no call output for point %d", v.body.body, point)
		}
		outcome, exact, err := v.callOutcomeFromPublishedOutput(ctx, point, output, pointReachable, outputReachable, v.callSites[point])
		if err != nil {
			return FormalLexicalBodyCoordinates{}, err
		}
		if !exact {
			return FormalLexicalBodyCoordinates{}, fmt.Errorf("transformer: formal lexical body %s has no call-outcome fiber for point %d", v.body.body, point)
		}
		out.CallOutcomes[point] = outcome
	}

	terminal, err := v.execution.bodyTerminalRelation(ctx, v.body.body)
	if err != nil {
		return FormalLexicalBodyCoordinates{}, err
	}
	diagnostics, _, err := v.execution.algebra.formalDiagnosticOutput(ctx, terminal.joined)
	if err != nil {
		return FormalLexicalBodyCoordinates{}, err
	}
	out.DiagnosticOutput = diagnostics.Clone()
	return out, nil
}

func formalLexicalCallDependencyLess(left, right FormalLexicalCallDependency) bool {
	if left.Point != right.Point {
		return left.Point < right.Point
	}
	if left.Occurrence != right.Occurrence {
		return left.Occurrence < right.Occurrence
	}
	for index := range left.Target {
		if left.Target[index] != right.Target[index] {
			return left.Target[index] < right.Target[index]
		}
	}
	return false
}

func (v *FormalRelationPublicationView) joinPublishedCoordinates(
	ctx context.Context,
	coordinates []formalPublishedCoordinate,
) (state.State, bool, error) {
	if len(coordinates) == 0 {
		return state.State{}, false, fmt.Errorf("transformer: formal lexical publication has no named coordinate")
	}
	joined := v.body.domain.Bottom()
	reachable := false
	for _, coordinate := range coordinates {
		coordinate.view = v
		value, err := v.joinState(ctx, coordinate)
		if err != nil {
			return state.State{}, false, err
		}
		if v.body.domain.Equal(value, v.body.domain.Bottom()) {
			continue
		}
		if !reachable {
			joined, reachable = value, true
		} else {
			joined = v.body.domain.Join(joined, value)
		}
	}
	return state.NormalizeForDomain(v.body.domain, joined), reachable, nil
}
