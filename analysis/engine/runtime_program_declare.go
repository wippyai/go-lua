// runtime_program_declare.go states one finished source admission as the
// sealed declaration the pure program-geometry function folds. It is the
// transitional handoff of the constructor migration: source admission still
// runs through the Binding's row transaction, because a Batch capability -
// a Site, an Occurrence, an Operand, and the rule rows a domain owner seals
// against them - cannot be minted outside it yet, while the geometry those
// rows fold into is already derived by constructTopology alone.
//
// Nothing here derives geometry. It reads the sealed rows the admission
// produced and the sealed templates the parent issued, and states them as
// coordinates: a member is addressed by the role, mount, point and occurrence
// its template row declares, a query by the identity it publishes under. The
// declaration leaves with the row transaction when domain admission moves onto
// declared rows.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// declaredMemberCoordinates is the coordinate inverse of one published member
// identity. It is derived from the sealed templates and the Link bootstrap
// witness, never from the row transaction.
type declaredMemberCoordinates map[identity.ContentID]declaredMemberRow

// declaredMountedCoordinates indexes every member identity the sealed mounts
// and the bootstrap witness can publish.
func declaredMountedCoordinates(source constructedSourcePlane, mounts []sealedProgramMount, bootstrap LinkBootstrapWitness) (declaredMemberCoordinates, bool) {
	coordinates := make(declaredMemberCoordinates)
	for _, mount := range mounts {
		template := mount.template
		if template == nil || !template.Available() {
			return nil, false
		}
		for ruleIndex := 0; ruleIndex < template.RuleCount(); ruleIndex++ {
			rule, ruleOK := template.RuleAt(ruleIndex)
			if !ruleOK {
				return nil, false
			}
			capability, capabilityOK := constructedRoleCapability(source, mount, rule.Role)
			if !capabilityOK {
				return nil, false
			}
			id := mountedRuleMemberID(capability, mount.module, rule.Point, rule.ID)
			if !id.Available() {
				return nil, false
			}
			coordinates[id] = declaredMemberRow{
				Plane: declaredMemberMount, Role: capability, Mount: mount.module,
				Point: rule.Point, Occurrence: rule.ID,
			}
		}
	}
	point, pointOK := bootstrap.Point()
	owner := bootstrap.OwnerID()
	if !pointOK || !owner.Available() {
		return nil, false
	}
	for index := 0; index < bootstrap.OccurrenceCount(); index++ {
		occurrence, occurrenceOK := bootstrap.OccurrenceAt(index)
		capability, capabilityOK := bootstrap.capabilityFor(occurrence)
		if !occurrenceOK || !capabilityOK {
			return nil, false
		}
		id := linkRuleMemberID(capability, owner, point.PointID, occurrence)
		if !id.Available() {
			return nil, false
		}
		coordinates[id] = declaredMemberRow{Plane: declaredMemberLink, Role: capability, Occurrence: occurrence}
	}
	return coordinates, true
}

// declareSealedTopology states one finished source admission as a topology
// declaration. It runs after SealSources: the Batch is sealed, every rule and
// query row its owners issued is final, and no further row can be admitted.
func declareSealedTopology(builder *BindingTopologyBuilder, mounts []sealedProgramMount, bootstrap LinkBootstrapWitness) (topologyDeclaration, bool) {
	if builder == nil || builder.binding == nil {
		return topologyDeclaration{}, false
	}
	rows := builder.mountedRows
	inner, locked := builder.lockTopologyOpen()
	if !locked {
		return topologyDeclaration{}, false
	}
	batch, semantic := inner.batch, inner.semantic
	spec := inner.spec
	inner.mu.Unlock()
	if rows == nil || rows.bootstrap == nil || batch == nil || semantic == nil {
		return topologyDeclaration{}, false
	}
	declaration := topologyDeclaration{
		binding:          builder.binding,
		batch:            batch,
		mounts:           mounts,
		bootstrap:        bootstrap,
		sites:            constructedSitePlane{mounted: rows.mounted, bootstrap: rows.bootstrap.site},
		materializations: spec.Materializations,
		directCandidates: spec.DirectCandidates,
		summaries:        spec.Summaries,
	}
	source, refusal := constructSourcePlane(declaration)
	if refusal.Available() {
		return topologyDeclaration{}, false
	}
	coordinates, indexed := declaredMountedCoordinates(source, mounts, bootstrap)
	if !indexed || len(spec.Rules) != len(spec.Groups) {
		return topologyDeclaration{}, false
	}
	declaration.members = make([]declaredMemberRow, len(spec.Rules))
	for ordinal := range spec.Rules {
		ref := equation.RuleAt(ordinal)
		id, published := semantic.memberAt[ref]
		member, addressed := coordinates[id]
		if !published || !addressed {
			return topologyDeclaration{}, false
		}
		member.ID = id
		member.Row = spec.Rules[ordinal]
		member.EnvironmentInput = spec.Groups[ordinal].EnvironmentInput
		if activation, registered := semantic.activationAt[ref]; registered {
			member.Activation, member.ActivationID = true, activation
		}
		declaration.members[ordinal] = member
	}
	declaration.queries = make([]declaredQueryRow, len(spec.Queries))
	stated := 0
	for id, ordinal := range semantic.queries {
		if ordinal >= uint64(len(spec.Queries)) || declaration.queries[ordinal].ID.Available() {
			return topologyDeclaration{}, false
		}
		declaration.queries[ordinal] = declaredQueryRow{ID: id, Row: spec.Queries[ordinal]}
		stated++
	}
	if stated != len(spec.Queries) {
		return topologyDeclaration{}, false
	}
	return declaration, true
}
