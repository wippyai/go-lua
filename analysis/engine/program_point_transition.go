package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
)

// ProgramPointTransitionAdmission is the one construction-time admission for
// an exact module-call geometry.  The rows are kept together deliberately:
// the generation is the authenticated target of the transition, rather than
// a caller-selected target mount or body.
type ProgramPointTransitionAdmission struct {
	Transition modulecomposition.ModuleCallTransition
	Generation modulecomposition.InitGeneration
}

func (admission ProgramPointTransitionAdmission) available() bool {
	return admission.Transition.Available() && admission.Generation.Available()
}

// ProgramPointTransition is one immutable graph-owned point pair derived from
// an admitted ModuleCallTransition and InitGeneration.  A generation body may
// have more than one canonical entry point, so one schema transition expands
// into one row per entry point.  The schema rows themselves are intentionally
// not exposed as mutable construction state; their authenticated identities
// and exact Context endpoints are retained as scalars.
type ProgramPointTransition struct {
	program                      *CommittedProgram
	transition                   modulecomposition.ModuleCallTransition
	generation                   modulecomposition.InitGeneration
	transitionID, generationID   identity.ContentID
	fromContextID, toContextID   identity.ContentID
	source, target               equation.Point
	sourceOrdinal, targetOrdinal contextfiber.PointOrdinal

	available bool
}

// completeGeometry checks the row without calling CommittedProgram.valid.  The
// latter checks the retained rows in turn, so using it here would recurse.  It
// runs once, in constructProgramPointTransitions, which is the sole issuer.
func (row ProgramPointTransition) completeGeometry() bool {
	program := row.program
	if program == nil || program.self != program || program.graph == nil || len(program.pointOwners) != program.graph.PointCount() || !program.contexts.Available() ||
		!row.transition.Available() || !row.generation.Available() || row.transition.ID() != row.transitionID || row.generation.ID() != row.generationID ||
		!row.transitionID.Available() || !row.generationID.Available() || !row.fromContextID.Available() || !row.toContextID.Available() ||
		!program.graph.OwnsPoint(row.source) || !program.graph.OwnsPoint(row.target) {
		return false
	}
	sourceOrdinal, sourceOK := program.graph.PointIndex(row.source)
	targetOrdinal, targetOK := program.graph.PointIndex(row.target)
	if !sourceOK || !targetOK || sourceOrdinal < 0 || targetOrdinal < 0 ||
		row.sourceOrdinal != contextfiber.PointOrdinal(sourceOrdinal) || row.targetOrdinal != contextfiber.PointOrdinal(targetOrdinal) {
		return false
	}
	from, fromOK := program.contexts.Context(row.fromContextID)
	to, toOK := program.contexts.Context(row.toContextID)
	if !fromOK || !toOK || !from.Available() || !to.Available() || from.ID() != row.fromContextID || to.ID() != row.toContextID {
		return false
	}
	if row.transition.GenerationID() != row.generation.ID() || row.transition.CacheIngressID() != row.generation.CacheIngressID() ||
		row.transition.LinkID() != program.contexts.LinkID() || row.generation.LinkID() != program.contexts.LinkID() ||
		from.LinkID() != program.contexts.LinkID() || to.LinkID() != program.contexts.LinkID() ||
		from.ModuleKey() != row.transition.SourceModuleKey() || to.ModuleKey() != row.generation.ModuleKey() {
		return false
	}
	canonicalTransition, transitionOK := program.contexts.Transition(row.fromContextID, row.toContextID)
	if !transitionOK || !canonicalTransition.Available() ||
		canonicalTransition.ID() != row.transition.TransitionID() ||
		canonicalTransition.LinkID() != row.transition.LinkID() ||
		canonicalTransition.FromContextID() != row.fromContextID ||
		canonicalTransition.ToContextID() != row.toContextID {
		return false
	}
	sourceOwner := program.pointOwners[sourceOrdinal]
	targetOwner := program.pointOwners[targetOrdinal]
	return sourceOwner.Mounted() && targetOwner.Mounted() &&
		sourceOwner.ModuleKey() == row.transition.SourceModuleKey() && targetOwner.ModuleKey() == row.generation.ModuleKey()
}

// Available reports whether this row names the exact graph and context
// directory that issued it.  The verdict is sealed by the constructor; a
// committed program never exchanges the graph or the directory it committed.
func (row ProgramPointTransition) Available() bool { return row.available }

// TransitionID returns the exact schema ModuleCallTransition identity.
func (row ProgramPointTransition) TransitionID() identity.ContentID {
	if !row.available {
		return identity.ContentID{}
	}
	return row.transitionID
}

// GenerationID returns the exact schema InitGeneration identity.
func (row ProgramPointTransition) GenerationID() identity.ContentID {
	if !row.available {
		return identity.ContentID{}
	}
	return row.generationID
}

// Transition returns the exact schema row that authenticated this geometry.
func (row ProgramPointTransition) Transition() modulecomposition.ModuleCallTransition {
	if !row.available {
		return modulecomposition.ModuleCallTransition{}
	}
	return row.transition
}

// Generation returns the exact schema row that authenticated the target body.
func (row ProgramPointTransition) Generation() modulecomposition.InitGeneration {
	if !row.available {
		return modulecomposition.InitGeneration{}
	}
	return row.generation
}

// FromContextID returns the exact source execution-context identity.
func (row ProgramPointTransition) FromContextID() identity.ContentID {
	if !row.available {
		return identity.ContentID{}
	}
	return row.fromContextID
}

// ToContextID returns the exact target execution-context identity.
func (row ProgramPointTransition) ToContextID() identity.ContentID {
	if !row.available {
		return identity.ContentID{}
	}
	return row.toContextID
}

// SourcePoint returns the exact graph-owned source Point handle.
func (row ProgramPointTransition) SourcePoint() equation.Point {
	if !row.available {
		return equation.Point{}
	}
	return row.source
}

// TargetPoint returns the exact graph-owned target Point handle.
func (row ProgramPointTransition) TargetPoint() equation.Point {
	if !row.available {
		return equation.Point{}
	}
	return row.target
}

// SourceContext resolves the exact source Context retained by the row.
func (row ProgramPointTransition) SourceContext() (executioncontext.Context, bool) {
	if !row.available {
		return executioncontext.Context{}, false
	}
	return row.program.contexts.Context(row.fromContextID)
}

// TargetContext resolves the exact target Context retained by the row.
func (row ProgramPointTransition) TargetContext() (executioncontext.Context, bool) {
	if !row.available {
		return executioncontext.Context{}, false
	}
	return row.program.contexts.Context(row.toContextID)
}

// PointTransitionCount reports the number of committed point-transition rows.
func (committed *CommittedProgram) PointTransitionCount() int {
	if committed == nil || !committed.valid() {
		return 0
	}
	return len(committed.pointTransitions)
}

// PointTransitionAt returns one immutable point-transition row in canonical
// admission/body-entry order.
func (committed *CommittedProgram) PointTransitionAt(index int) (ProgramPointTransition, bool) {
	if committed == nil || !committed.valid() || index < 0 || index >= len(committed.pointTransitions) {
		return ProgramPointTransition{}, false
	}
	row := committed.pointTransitions[index]
	return row, row.available
}

// PointTransitions returns a detached copy of the committed rows.  The rows
// retain their graph ownership fence; changing the returned slice cannot
// mutate the committed program.
func (committed *CommittedProgram) PointTransitions() []ProgramPointTransition {
	if committed == nil || !committed.valid() {
		return nil
	}
	result := append([]ProgramPointTransition(nil), committed.pointTransitions...)
	for _, row := range result {
		if !row.available {
			return nil
		}
	}
	return result
}

type pointTransitionGeometryKey struct {
	fromContextID, toContextID identity.ContentID
	source, target             composition.Key
}

// constructProgramPointTransitions is the bounded engine-owned join between
// schema module-composition rows and sealed equation geometry.  It runs only
// after the semantic directory and graph exist, and every point is resolved by
// its mount-qualified semantic identity; no caller-supplied ordinal or role is
// accepted.
func constructProgramPointTransitions(committed *CommittedProgram, declaration topologyDeclaration, mounts constructedMountPlane) ([]ProgramPointTransition, bool) {
	if committed == nil || !declaration.mounted() {
		return nil, len(declaration.pointTransitions) == 0
	}
	if committed.graph == nil || committed.directory == nil || !committed.contexts.Available() || len(committed.pointOwners) != committed.graph.PointCount() {
		return nil, false
	}
	if len(declaration.pointTransitions) == 0 {
		return nil, true
	}

	mountByModule := make(map[identity.ContentID]sealedProgramMount, len(mounts.mounts))
	for _, mount := range mounts.mounts {
		if mount.template == nil || !mount.template.Available() || !mount.module.Available() {
			return nil, false
		}
		if _, duplicate := mountByModule[mount.module]; duplicate {
			return nil, false
		}
		mountByModule[mount.module] = mount
	}

	seenTransitions := make(map[identity.ContentID]struct{}, len(declaration.pointTransitions))
	seenGeometry := make(map[pointTransitionGeometryKey]struct{})
	result := make([]ProgramPointTransition, 0, len(declaration.pointTransitions))
	for _, admission := range declaration.pointTransitions {
		transition, generation := admission.Transition, admission.Generation
		if !admission.available() || transition.GenerationID() != generation.ID() || transition.CacheIngressID() != generation.CacheIngressID() ||
			transition.LinkID() != committed.contexts.LinkID() || generation.LinkID() != committed.contexts.LinkID() {
			return nil, false
		}
		if _, duplicate := seenTransitions[transition.ID()]; duplicate {
			return nil, false
		}
		seenTransitions[transition.ID()] = struct{}{}

		sourceMount, sourceOK := mountByModule[transition.SourceModuleKey()]
		targetMount, targetOK := mountByModule[generation.ModuleKey()]
		if !sourceOK || !targetOK || sourceMount.template == nil || targetMount.template == nil ||
			sourceMount.template.ArtifactID() != transition.ArtifactID() || sourceMount.template.ProgramID() != transition.ProgramID() ||
			targetMount.template.ArtifactID() != generation.ArtifactID() || targetMount.template.ProgramID() != generation.ProgramID() {
			return nil, false
		}

		fromContext, fromOK := committed.contexts.Context(transition.FromContextID())
		toContext, toOK := committed.contexts.Context(transition.ToContextID())
		if !fromOK || !toOK || !fromContext.Available() || !toContext.Available() ||
			fromContext.LinkID() != committed.contexts.LinkID() || toContext.LinkID() != committed.contexts.LinkID() ||
			fromContext.ModuleKey() != transition.SourceModuleKey() || toContext.ModuleKey() != generation.ModuleKey() {
			return nil, false
		}
		canonicalTransition, transitionOK := committed.contexts.Transition(transition.FromContextID(), transition.ToContextID())
		if !transitionOK || !canonicalTransition.Available() ||
			canonicalTransition.ID() != transition.TransitionID() ||
			canonicalTransition.LinkID() != transition.LinkID() ||
			canonicalTransition.FromContextID() != transition.FromContextID() ||
			canonicalTransition.ToContextID() != transition.ToContextID() {
			return nil, false
		}

		// ModuleCallTransition has already authenticated the source reusable
		// point from the mounted Program's canonical StageCallDispatch row. The
		// engine only qualifies that exact point for this mount; it never scans
		// role stages or chooses one from the native-stage inverse.
		sourcePoint, sourceOrdinal, sourcePointOK := resolveProgramMountedPoint(committed, sourceMount.module, sourceMount.template.ArtifactID(), transition.SourcePointID())
		if !sourcePointOK {
			return nil, false
		}

		body, bodyOK := exactTemplateBody(targetMount.template, generation.BodyID())
		if !bodyOK || len(body.Entry) == 0 {
			return nil, false
		}
		seenEntries := make(map[identity.ContentID]struct{}, len(body.Entry))
		for _, entryPointID := range body.Entry {
			if !entryPointID.Available() {
				return nil, false
			}
			if _, duplicate := seenEntries[entryPointID]; duplicate {
				return nil, false
			}
			seenEntries[entryPointID] = struct{}{}
			targetPoint, targetOrdinal, targetPointOK := resolveProgramMountedPoint(committed, targetMount.module, targetMount.template.ArtifactID(), entryPointID)
			if !targetPointOK {
				return nil, false
			}
			geometry := pointTransitionGeometryKey{
				fromContextID: transition.FromContextID(), toContextID: transition.ToContextID(),
				source: sourcePoint.Key(), target: targetPoint.Key(),
			}
			if _, duplicate := seenGeometry[geometry]; duplicate {
				return nil, false
			}
			seenGeometry[geometry] = struct{}{}
			result = append(result, ProgramPointTransition{
				program: committed, transition: transition, generation: generation,
				transitionID: transition.ID(), generationID: generation.ID(),
				fromContextID: transition.FromContextID(), toContextID: transition.ToContextID(),
				source: sourcePoint, target: targetPoint,
				sourceOrdinal: sourceOrdinal, targetOrdinal: targetOrdinal,
			})
			issued := &result[len(result)-1]
			issued.available = issued.completeGeometry()
			if !issued.available {
				return nil, false
			}
		}
	}
	return result, true
}

func exactTemplateBody(template *rows.ArtifactScalarTemplate, bodyID identity.ContentID) (rows.ArtifactScalarBody, bool) {
	if template == nil || !template.Available() || !bodyID.Available() {
		return rows.ArtifactScalarBody{}, false
	}
	var found rows.ArtifactScalarBody
	for index := 0; index < template.BodyCount(); index++ {
		body, held := template.BodyAt(index)
		if !held || !body.ID.Available() {
			return rows.ArtifactScalarBody{}, false
		}
		if body.ID != bodyID {
			continue
		}
		if found.ID.Available() {
			return rows.ArtifactScalarBody{}, false
		}
		found = body
	}
	return found, found.ID.Available()
}

func resolveProgramMountedPoint(committed *CommittedProgram, mount, artifact, reusable identity.ContentID) (equation.Point, contextfiber.PointOrdinal, bool) {
	if committed == nil || committed.graph == nil || committed.directory == nil || !mount.Available() || !artifact.Available() || !reusable.Available() || len(committed.pointOwners) != committed.graph.PointCount() {
		return equation.Point{}, 0, false
	}
	semantic := mountedArtifactID("analysis/engine/artifact-point/v1", mount, artifact, reusable)
	locator, located := committed.directory.point(semantic)
	point, pointOK := locator.Resolve(committed.graph)
	pointIndex, indexed := committed.graph.PointIndex(point)
	if !located || !pointOK || !indexed || pointIndex < 0 || pointIndex >= len(committed.pointOwners) {
		return equation.Point{}, 0, false
	}
	owner := committed.pointOwners[pointIndex]
	if !owner.Mounted() || owner.ModuleKey() != mount {
		return equation.Point{}, 0, false
	}
	return point, contextfiber.PointOrdinal(pointIndex), true
}
