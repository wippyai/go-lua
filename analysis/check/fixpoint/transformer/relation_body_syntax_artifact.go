package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// relationBodySyntaxArtifact is the sealed tier-2 local syntax for one
// lexical body.  It is deliberately not a cache key or a prepared compiler:
// every forest transaction creates fresh artifacts after the one forest-wide
// term closure has finished, and only then exposes them to SCC/link work.
//
// In particular, it owns no forest pointer, solver state, entry value, or
// open builder.  The opaque execution authorities are carried as execution
// inputs, never promoted into a structural key.  Stage 4 can therefore split
// SCC/link artifacts from this sealed local boundary without reopening local
// CFG/operator syntax.
type relationBodySyntaxArtifact struct {
	body                 lexicalidentity.StableLexicalBodyID
	variable             relationVar
	keys                 *keyspace.KeySpace
	roots                relationRootCarrier
	ambient              []relationEnvironmentRoot
	relation             Relation
	plan                 *operationplan.Plan
	graph                cfg.Graph
	pathSemantics        *factapply.PathSemanticAuthority
	rootAssignments      *factapply.RootAssignmentAuthority
	returns              *factapply.ReturnAuthority
	externalCalls        callpayload.CallOutcomeProgram
	genericForMembership factapply.GenericForMembershipAuthority
	nodeReads            [][]cfg.Point
	domain               lattice.Lattice[state.State]
	productDomain        state.ProductDomain
	entrySeedPlan        state.EntrySeedPlan
	initialStatePlan     state.InitialStatePlan
	sealed               bool
}

// freezeRelationBodySyntaxArtifact is intentionally called only after
// closeRelationProgramTerms and sealRelationProgramWorld.  Those operations
// still mutate the open per-body compiler/arena as part of a forest closure;
// the artifact begins after that mutation boundary and has no route back to a
// PreparedPlanCompiler.
func freezeRelationBodySyntaxArtifact(
	unit RelationProgramUnit,
	variable relationVar,
	roots relationRootCarrier,
	ambient []relationEnvironmentRoot,
	compiler *PreparedPlanCompiler,
) (relationBodySyntaxArtifact, error) {
	if compiler == nil || !compiler.frozen || compiler.builder == nil || compiler.builder.arena == nil ||
		!compiler.builder.arena.Sealed() || compiler.builder.effects == nil || !compiler.builder.effects.Sealed() {
		return relationBodySyntaxArtifact{}, fmt.Errorf("transformer: lexical body %s has unsealed local syntax", unit.Body)
	}
	relation := compiler.frozenRelation()
	if !compiler.codeBase.sealed || relation.code == nil || !relation.code.sealed || relation.arena == nil || relation.effects == nil ||
		!relation.arena.Sealed() || !relation.effects.Sealed() || relation.code.terms != relation.arena || relation.code.effects != relation.effects {
		return relationBodySyntaxArtifact{}, fmt.Errorf("transformer: lexical body %s did not produce sealed local relation syntax", unit.Body)
	}
	if unit.Body == (lexicalidentity.StableLexicalBodyID{}) || variable == 0 || unit.Graph == nil || unit.Plan == nil ||
		unit.KeySpace == nil || !unit.KeySpace.Valid() || !unit.Domain.Valid() || !unit.EntrySeedPlan.Valid() ||
		!unit.InitialStatePlan.ValidFor(unit.Body, unit.Graph.ID(), unit.Graph.Size()) {
		return relationBodySyntaxArtifact{}, fmt.Errorf("transformer: lexical body %s has incomplete sealed syntax inputs", unit.Body)
	}
	reads := make([][]cfg.Point, len(unit.NodeReads))
	for point := range unit.NodeReads {
		reads[point] = append([]cfg.Point(nil), unit.NodeReads[point]...)
	}
	return relationBodySyntaxArtifact{
		body: unit.Body, variable: variable, keys: unit.KeySpace, roots: roots,
		ambient: append([]relationEnvironmentRoot(nil), ambient...), relation: relation,
		plan: unit.Plan, graph: unit.Graph, pathSemantics: unit.PathSemantics,
		rootAssignments: unit.RootAssignments, returns: unit.Returns,
		externalCalls: unit.ExternalCallOutcome, genericForMembership: unit.GenericForMembership,
		nodeReads: reads, domain: unit.Domain.Lattice(), productDomain: unit.Domain,
		entrySeedPlan: unit.EntrySeedPlan.Clone(), initialStatePlan: unit.InitialStatePlan.Clone(),
		sealed: true,
	}, nil
}

func (a relationBodySyntaxArtifact) valid() bool {
	return a.sealed && a.body != (lexicalidentity.StableLexicalBodyID{}) && a.variable != 0 && a.keys != nil && a.keys.Valid() &&
		a.graph != nil && a.plan != nil && a.relation.code != nil && a.relation.code.sealed && a.relation.arena != nil &&
		a.relation.effects != nil && a.relation.arena.Sealed() && a.relation.effects.Sealed() && a.relation.code.terms == a.relation.arena &&
		a.relation.code.effects == a.relation.effects && a.productDomain.Valid() && a.entrySeedPlan.Valid() &&
		a.initialStatePlan.ValidFor(a.body, a.graph.ID(), a.graph.Size())
}

// materializeRelationProgramBody preserves the old whole-forest transaction
// shape.  Only SCC/link fields are born after this point; local syntax remains
// the sealed artifact above and is never mutated through this compatibility
// body view.
func (a relationBodySyntaxArtifact) materializeRelationProgramBody() (relationProgramBody, error) {
	if !a.valid() {
		return relationProgramBody{}, fmt.Errorf("transformer: malformed sealed lexical syntax artifact")
	}
	return relationProgramBody{
		body: a.body, variable: a.variable, keys: a.keys, roots: a.roots, ambient: append([]relationEnvironmentRoot(nil), a.ambient...),
		relation: a.relation, plan: a.plan, graph: a.graph, pathSemantics: a.pathSemantics,
		rootAssignments: a.rootAssignments, returns: a.returns, externalCalls: a.externalCalls,
		genericForMembership: a.genericForMembership, nodeReads: cloneRelationNodeReads(a.nodeReads),
		domain: a.domain, productDomain: a.productDomain, entrySeedPlan: a.entrySeedPlan.Clone(),
		initialStatePlan: a.initialStatePlan.Clone(), definitionFrames: make(map[callFrameTerm]relationVar),
	}, nil
}

func cloneRelationNodeReads(reads [][]cfg.Point) [][]cfg.Point {
	out := make([][]cfg.Point, len(reads))
	for point := range reads {
		out[point] = append([]cfg.Point(nil), reads[point]...)
	}
	return out
}

func validateDistinctRelationBodySyntaxArtifacts(artifacts []relationBodySyntaxArtifact) error {
	for index := range artifacts {
		if !artifacts[index].valid() {
			return fmt.Errorf("transformer: body syntax artifact %d is not sealed", index+1)
		}
		for prior := 0; prior < index; prior++ {
			if artifacts[prior].body == artifacts[index].body || artifacts[prior].relation.code == artifacts[index].relation.code ||
				artifacts[prior].relation.arena == artifacts[index].relation.arena || artifacts[prior].relation.effects == artifacts[index].relation.effects {
				return fmt.Errorf("transformer: body syntax artifacts %d and %d share local syntax ownership", prior+1, index+1)
			}
		}
	}
	return nil
}
