package rows

import "github.com/wippyai/go-lua/analysis/identity"

// ArtifactScalarSpec is a single-use neutral template builder. Its state is
// private and shared by copied handles, so consuming any handle closes all of
// them. Nested rows are appended through methods below and cannot remain
// caller-mutable after the receipt takes ownership.
type ArtifactScalarSpec struct {
	state *artifactScalarSpecState
}

type artifactScalarSpecState struct {
	ArtifactID identity.ContentID
	ProgramID  identity.ContentID
	SchemaID   identity.ContentID
	Roles      []ArtifactScalarRole
	Points     []ArtifactScalarPoint
	Edges      []ArtifactScalarEdge
	Transfers  []ArtifactScalarTransfer
	Regions    []ArtifactScalarRegion
	Events     []ArtifactScalarEvent
	Rules      []ArtifactScalarRule
	Bodies     []ArtifactScalarBody
	Functions  []ArtifactScalarFunction
	consumed   bool
}

func NewArtifactScalarSpec(artifactID, programID, schemaID identity.ContentID, capacity ArtifactScalarCapacity) (*ArtifactScalarSpec, bool) {
	if !artifactID.Available() || !programID.Available() || !schemaID.Available() || capacity.Roles < 0 || capacity.Points < 0 || capacity.Edges < 0 || capacity.Transfers < 0 || capacity.Regions < 0 || capacity.Events < 0 || capacity.Rules < 0 || capacity.Bodies < 0 || capacity.Functions < 0 {
		return nil, false
	}
	return &ArtifactScalarSpec{state: &artifactScalarSpecState{
		ArtifactID: artifactID,
		ProgramID:  programID,
		SchemaID:   schemaID,
		Roles:      make([]ArtifactScalarRole, 0, capacity.Roles),
		Points:     make([]ArtifactScalarPoint, 0, capacity.Points),
		Edges:      make([]ArtifactScalarEdge, 0, capacity.Edges),
		Transfers:  make([]ArtifactScalarTransfer, 0, capacity.Transfers),
		Regions:    make([]ArtifactScalarRegion, 0, capacity.Regions),
		Events:     make([]ArtifactScalarEvent, 0, capacity.Events),
		Rules:      make([]ArtifactScalarRule, 0, capacity.Rules),
		Bodies:     make([]ArtifactScalarBody, 0, capacity.Bodies),
		Functions:  make([]ArtifactScalarFunction, 0, capacity.Functions),
	}}, true
}

// DeclareRole admits one stable Program-issued role identity. Role order is
// retained exactly and forms the canonical Link binding order.
func (spec *ArtifactScalarSpec) DeclareRole(semantic identity.ContentID) (ArtifactScalarRole, bool) {
	state, ok := spec.writable()
	if !ok || !semantic.Available() {
		return ArtifactScalarRole{}, false
	}
	for _, prior := range state.Roles {
		if prior.semantic == semantic {
			return ArtifactScalarRole{}, false
		}
	}
	role := ArtifactScalarRole{semantic: semantic}
	state.Roles = append(state.Roles, role)
	return role, true
}

func scalarSpecOwnsRole(state *artifactScalarSpecState, role ArtifactScalarRole) bool {
	if state == nil || !role.Available() {
		return false
	}
	for _, candidate := range state.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func (spec *ArtifactScalarSpec) writable() (*artifactScalarSpecState, bool) {
	if spec == nil || spec.state == nil || spec.state.consumed {
		return nil, false
	}
	return spec.state, true
}

func (spec *ArtifactScalarSpec) AddPoint(row ArtifactScalarPoint) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Decisions) != 0 {
		return -1, false
	}
	state.Points = append(state.Points, row)
	return len(state.Points) - 1, true
}

func (spec *ArtifactScalarSpec) AddPointDecision(point int, decision identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || point < 0 || point >= len(state.Points) || !decision.Available() {
		return false
	}
	state.Points[point].Decisions = append(state.Points[point].Decisions, decision)
	return true
}

func (spec *ArtifactScalarSpec) AddEdge(row ArtifactScalarEdge) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Resets) != 0 {
		return -1, false
	}
	state.Edges = append(state.Edges, row)
	return len(state.Edges) - 1, true
}

func (spec *ArtifactScalarSpec) AddEdgeReset(edge int, reset identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || edge < 0 || edge >= len(state.Edges) || !reset.Available() {
		return false
	}
	state.Edges[edge].Resets = append(state.Edges[edge].Resets, reset)
	return true
}

func (spec *ArtifactScalarSpec) AddTransfer(row ArtifactScalarTransfer) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Factors) != 0 {
		return -1, false
	}
	state.Transfers = append(state.Transfers, row)
	return len(state.Transfers) - 1, true
}

func (spec *ArtifactScalarSpec) AddTransferFactor(transfer int, factor ArtifactScalarRole) bool {
	state, ok := spec.writable()
	if !ok || transfer < 0 || transfer >= len(state.Transfers) || !scalarSpecOwnsRole(state, factor) {
		return false
	}
	state.Transfers[transfer].Factors = append(state.Transfers[transfer].Factors, factor)
	return true
}

func (spec *ArtifactScalarSpec) AddRegion(row ArtifactScalarRegion) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Members) != 0 {
		return -1, false
	}
	state.Regions = append(state.Regions, row)
	return len(state.Regions) - 1, true
}

func (spec *ArtifactScalarSpec) AddRegionMember(region int, member identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || region < 0 || region >= len(state.Regions) || !member.Available() {
		return false
	}
	state.Regions[region].Members = append(state.Regions[region].Members, member)
	return true
}

func (spec *ArtifactScalarSpec) AddEvent(row ArtifactScalarEvent) bool {
	state, ok := spec.writable()
	if !ok {
		return false
	}
	state.Events = append(state.Events, row)
	return true
}

func (spec *ArtifactScalarSpec) AddRule(row ArtifactScalarRule) bool {
	state, ok := spec.writable()
	if !ok || !scalarSpecOwnsRole(state, row.Role) {
		return false
	}
	state.Rules = append(state.Rules, row)
	return true
}

func (spec *ArtifactScalarSpec) AddBody(row ArtifactScalarBody) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Entry) != 0 || len(row.Exits) != 0 {
		return -1, false
	}
	state.Bodies = append(state.Bodies, row)
	return len(state.Bodies) - 1, true
}

func (spec *ArtifactScalarSpec) AddBodyEntry(body int, point identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || body < 0 || body >= len(state.Bodies) || !point.Available() {
		return false
	}
	state.Bodies[body].Entry = append(state.Bodies[body].Entry, point)
	return true
}

func (spec *ArtifactScalarSpec) AddBodyExit(body int, point identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || body < 0 || body >= len(state.Bodies) || !point.Available() {
		return false
	}
	state.Bodies[body].Exits = append(state.Bodies[body].Exits, point)
	return true
}

func (spec *ArtifactScalarSpec) AddFunction(row ArtifactScalarFunction) (int, bool) {
	state, ok := spec.writable()
	if !ok || len(row.Formals) != 0 || row.Vararg != (ArtifactScalarVarargPort{}) || row.HasVararg || len(row.Captures) != 0 || len(row.Outcomes) != 0 {
		return -1, false
	}
	state.Functions = append(state.Functions, row)
	return len(state.Functions) - 1, true
}

func (spec *ArtifactScalarSpec) AddFunctionFormal(function int, port ArtifactScalarFormalPort) bool {
	state, ok := spec.writable()
	if !ok || function < 0 || function >= len(state.Functions) || !port.ID.Available() || !port.Cell.Available() || !port.Storage.Available() || uint64(port.Position) != uint64(len(state.Functions[function].Formals)) {
		return false
	}
	state.Functions[function].Formals = append(state.Functions[function].Formals, port)
	return true
}

func (spec *ArtifactScalarSpec) SetFunctionVararg(function int, port ArtifactScalarVarargPort) bool {
	state, ok := spec.writable()
	if !ok || function < 0 || function >= len(state.Functions) || state.Functions[function].HasVararg || !port.ID.Available() || !port.Cell.Available() {
		return false
	}
	state.Functions[function].Vararg = port
	state.Functions[function].HasVararg = true
	return true
}

func (spec *ArtifactScalarSpec) AddFunctionCapture(function int, port ArtifactScalarCapturePort) bool {
	state, ok := spec.writable()
	if !ok || function < 0 || function >= len(state.Functions) || !port.ID.Available() || !port.Inner.Available() || !port.Outer.Available() || !port.InnerBody.Available() || !port.OuterBody.Available() || port.Inner == port.Outer || port.InnerBody == port.OuterBody || uint64(port.Position) != uint64(len(state.Functions[function].Captures)) {
		return false
	}
	state.Functions[function].Captures = append(state.Functions[function].Captures, port)
	return true
}

func (spec *ArtifactScalarSpec) AddFunctionOutcome(function int, outcome identity.ContentID) bool {
	state, ok := spec.writable()
	if !ok || function < 0 || function >= len(state.Functions) || !outcome.Available() {
		return false
	}
	state.Functions[function].Outcomes = append(state.Functions[function].Outcomes, outcome)
	return true
}
