package rows

import (
	"github.com/wippyai/go-lua/analysis/identity"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

// ArtifactScalarTemplate is the sealed, reusable Program structural input.
// It retains no binding, schema, or capability authority, so one exact template
// can be shared by independent Links and mounted repeatedly without rebuilding
// the Program interior.
//
// Every plane is read by index. The row planes are never handed out as slices,
// so a reader cannot append to or overwrite a sealed template.
type ArtifactScalarTemplate struct {
	artifact identity.ContentID
	program  identity.ContentID
	schema   identity.ContentID
	sealed   bool
	roles    []ArtifactScalarRole
	factors  []ArtifactScalarFactor
	points   []ArtifactScalarPoint
	edges    []ArtifactScalarEdge
	local    []ArtifactScalarTransfer
	regions  []ArtifactScalarRegion
	events   []ArtifactScalarEvent
	rules    []ArtifactScalarRule
	bodies   []ArtifactScalarBody
}

func NewArtifactScalarTemplate(spec *ArtifactScalarSpec) (*ArtifactScalarTemplate, bool) {
	if !validArtifactScalarSpec(spec) {
		return nil, false
	}
	state := spec.state
	template := &ArtifactScalarTemplate{artifact: state.ArtifactID, program: state.ProgramID, schema: state.SchemaID, sealed: true, roles: state.Roles, factors: state.Factors, points: state.Points, edges: state.Edges, local: state.Transfers, regions: state.Regions, events: state.Events, rules: state.Rules, bodies: state.Bodies}
	state.consumed = true
	state.Roles = nil
	state.Factors = nil
	state.Points = nil
	state.Edges = nil
	state.Transfers = nil
	state.Regions = nil
	state.Events = nil
	state.Rules = nil
	state.Bodies = nil
	state.stages = schemaissuance.Table{}
	state.stagesSet = false
	return template, true
}

func (template *ArtifactScalarTemplate) FactorCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.factors)
}

func (template *ArtifactScalarTemplate) FactorAt(index int) (ArtifactScalarFactor, bool) {
	if !template.Available() || index < 0 || index >= len(template.factors) {
		return ArtifactScalarFactor{}, false
	}
	return template.factors[index], true
}

func (template *ArtifactScalarTemplate) OwnsFactor(factor ArtifactScalarFactor) bool {
	if !template.Available() || !factor.Available() {
		return false
	}
	for _, candidate := range template.factors {
		if candidate == factor {
			return true
		}
	}
	return false
}

func (template *ArtifactScalarTemplate) Available() bool {
	return template != nil && template.sealed && template.artifact.Available() && template.program.Available() && template.schema.Available()
}
func (template *ArtifactScalarTemplate) ArtifactID() identity.ContentID {
	if !template.Available() {
		return identity.ContentID{}
	}
	return template.artifact
}
func (template *ArtifactScalarTemplate) ProgramID() identity.ContentID {
	if !template.Available() {
		return identity.ContentID{}
	}
	return template.program
}
func (template *ArtifactScalarTemplate) SchemaID() identity.ContentID {
	if !template.Available() {
		return identity.ContentID{}
	}
	return template.schema
}

func (template *ArtifactScalarTemplate) RoleCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.roles)
}

func (template *ArtifactScalarTemplate) RoleAt(index int) (ArtifactScalarRole, bool) {
	if !template.Available() || index < 0 || index >= len(template.roles) {
		return ArtifactScalarRole{}, false
	}
	return template.roles[index], true
}

// OwnsRole reports whether this exact role was declared by this template. It is
// the substitution fence a mounting owner proves before binding a role.
func (template *ArtifactScalarTemplate) OwnsRole(role ArtifactScalarRole) bool {
	if !template.Available() || !role.Available() {
		return false
	}
	for _, candidate := range template.roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func (template *ArtifactScalarTemplate) PointCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.points)
}

func (template *ArtifactScalarTemplate) PointAt(index int) (ArtifactScalarPoint, bool) {
	if !template.Available() || index < 0 || index >= len(template.points) {
		return ArtifactScalarPoint{}, false
	}
	return template.points[index], true
}

func (template *ArtifactScalarTemplate) EdgeCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.edges)
}

func (template *ArtifactScalarTemplate) EdgeAt(index int) (ArtifactScalarEdge, bool) {
	if !template.Available() || index < 0 || index >= len(template.edges) {
		return ArtifactScalarEdge{}, false
	}
	return template.edges[index], true
}

func (template *ArtifactScalarTemplate) TransferCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.local)
}

func (template *ArtifactScalarTemplate) TransferAt(index int) (ArtifactScalarTransfer, bool) {
	if !template.Available() || index < 0 || index >= len(template.local) {
		return ArtifactScalarTransfer{}, false
	}
	return template.local[index], true
}

func (template *ArtifactScalarTemplate) RegionCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.regions)
}

func (template *ArtifactScalarTemplate) RegionAt(index int) (ArtifactScalarRegion, bool) {
	if !template.Available() || index < 0 || index >= len(template.regions) {
		return ArtifactScalarRegion{}, false
	}
	return template.regions[index], true
}

func (template *ArtifactScalarTemplate) EventCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.events)
}

func (template *ArtifactScalarTemplate) EventAt(index int) (ArtifactScalarEvent, bool) {
	if !template.Available() || index < 0 || index >= len(template.events) {
		return ArtifactScalarEvent{}, false
	}
	return template.events[index], true
}

func (template *ArtifactScalarTemplate) RuleCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.rules)
}

func (template *ArtifactScalarTemplate) RuleAt(index int) (ArtifactScalarRule, bool) {
	if !template.Available() || index < 0 || index >= len(template.rules) {
		return ArtifactScalarRule{}, false
	}
	return template.rules[index], true
}

func (template *ArtifactScalarTemplate) BodyCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.bodies)
}

func (template *ArtifactScalarTemplate) BodyAt(index int) (ArtifactScalarBody, bool) {
	if !template.Available() || index < 0 || index >= len(template.bodies) {
		return ArtifactScalarBody{}, false
	}
	return template.bodies[index], true
}
