package engine

import (
	"bytes"
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

const (
	artifactPointSourceVersion      uint64 = 1
	artifactEdgeSourceVersion       uint64 = 2
	artifactOccurrenceSourceVersion uint64 = 3
)

// artifactReceiptTopology is BindingTopology's immutable copy of the exact
// Program artifact structural receipt. It intentionally retains only scalar
// semantic identities and the parent-issued WTO bracket stream; it never
// retains a Program, Flow, callback builder, or alternate topology authority.
type artifactReceiptTopology struct {
	sealed     *BindingTopology
	mounts     []artifactMountReceipt
	points     []identity.ContentID
	pointMeta  map[identity.ContentID]artifactPointMetadata
	sites      map[identity.ContentID]equation.Site
	mounted    map[artifactMountedPoint]equation.Site
	mountedRef map[artifactMountedPoint]equation.PointRef
	bodies     map[artifactMountedBody]artifactBodyTransport
	functions  []artifactMountedFunction
	ruleSet    map[artifactMountedRule]artifactRuleInput
	callStages map[artifactMountedRuleOccurrence]artifactNativeCallStage
	pointRef   map[identity.ContentID]equation.PointRef
	edges      []artifactEnvironmentRow
	regions    []artifactWTORegionRow
	events     []artifactWTOEventRow
	bootstrap  *linkBootstrapReceipt
}

type linkBootstrapReceipt struct {
	owner       identity.ContentID
	point       LinkBootstrapPoint
	site        equation.Site
	occurrences map[identity.ContentID]struct{}
	roles       map[identity.ContentID]RuleSlotCapability
	claims      map[identity.ContentID]RuleSlotCapability
	semantic    identity.ContentID
	ref         equation.PointRef
	transports  []linkBootstrapTransport
}

type linkBootstrapTransport struct {
	capability RuleSlotCapability
	factor     composition.Key
}

type artifactMountedPoint struct {
	mount    identity.ContentID
	reusable identity.ContentID
}

// artifactMountedBody is the immutable parent-issued body transport anchor.
// Point IDs remain reusable artifact IDs here; every later use must resolve
// them through the same mount-qualified point receipt.
type artifactMountedBody struct {
	mount identity.ContentID
	body  identity.ContentID
}

type artifactBodyTransport struct {
	entry []identity.ContentID
	exits []identity.ContentID
}

type artifactMountedFunction struct {
	id, mount, artifact, reusable        identity.ContentID
	body, bodyContext, entry, callFormal identity.ContentID
	formals                              []ArtifactScalarFormalPort
	vararg                               ArtifactScalarVarargPort
	hasVararg                            bool
	captures                             []ArtifactScalarCapturePort
	outcomes                             []identity.ContentID
}

type artifactMountedRule struct {
	role       RuleSlotCapability
	mount      identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
}

type artifactRuleInput struct {
	point        identity.ContentID
	mountedPoint identity.ContentID
	mountedInput identity.ContentID
	stage        ArtifactRuleStage
	predecessor  artifactEnvironmentRow
	routed       bool
}

// artifactMountedRuleOccurrence deliberately omits the reusable point. Native
// Call stages are keyed by the exact mounted occurrence, so a second stage
// point for the same owner/call is an alias and is rejected while snapshotting.
type artifactMountedRuleOccurrence struct {
	role       RuleSlotCapability
	mount      identity.ContentID
	occurrence identity.ContentID
}

type artifactNativeCallStage struct {
	stage        ArtifactRuleStage
	point        identity.ContentID
	input        identity.ContentID
	mountedPoint identity.ContentID
	mountedInput identity.ContentID
}

// ArtifactStructuralArm is the engine-neutral structural-edge arm vocabulary.
type ArtifactStructuralArm uint8

const (
	ArtifactStructuralArmInvalid ArtifactStructuralArm = iota
	ArtifactStructuralArmLocal
	ArtifactStructuralArmResume
	ArtifactStructuralArmTrue
	ArtifactStructuralArmFalse
	ArtifactStructuralArmTail
	ArtifactStructuralArmThrow
	ArtifactStructuralArmYield
	ArtifactStructuralArmCancel
)

func (arm ArtifactStructuralArm) Valid() bool {
	return arm >= ArtifactStructuralArmLocal && arm <= ArtifactStructuralArmCancel
}

// ArtifactEventKind is the engine-neutral bracket-stream vocabulary.
type ArtifactEventKind uint8

const (
	ArtifactEventInvalid ArtifactEventKind = iota
	ArtifactEventEnter
	ArtifactEventPoint
	ArtifactEventExit
)

// ArtifactScalarRole is one opaque Program-owned role in a reusable artifact
// template. Engine compares the semantic identity but never interprets it as
// a domain producer tag.
type ArtifactScalarRole struct {
	semantic identity.ContentID
}

func (role ArtifactScalarRole) Available() bool { return role.semantic.Available() }
func (role ArtifactScalarRole) ID() identity.ContentID {
	if !role.Available() {
		return identity.ContentID{}
	}
	return role.semantic
}

// ArtifactScalarTemplate is the sealed, reusable Program structural input.
// It retains no SchemaBinding or RuleSlotCapability, so one exact template can
// be shared by independent Links and mounted repeatedly without rebuilding
// the Program interior.
type ArtifactScalarTemplate struct {
	artifact  identity.ContentID
	program   identity.ContentID
	schema    identity.ContentID
	sealed    bool
	roles     []ArtifactScalarRole
	points    []ArtifactScalarPoint
	edges     []ArtifactScalarEdge
	local     []ArtifactScalarTransfer
	regions   []ArtifactScalarRegion
	events    []ArtifactScalarEvent
	rules     []ArtifactScalarRule
	bodies    []ArtifactScalarBody
	functions []ArtifactScalarFunction
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
func (template *ArtifactScalarTemplate) FunctionCount() int {
	if !template.Available() {
		return 0
	}
	return len(template.functions)
}

// ArtifactScalarBinding is the short-lived Link substitution for one reusable
// template. Every declared role is bound exactly once to an exact capability;
// the structural template remains owner-neutral.
type ArtifactScalarBinding struct {
	template     *ArtifactScalarTemplate
	capabilities map[identity.ContentID]RuleSlotCapability
	sealed       bool
}

// ArtifactScalarReceipt is the immutable mounted-input pair. It retains the
// shared Program template plus only Link-local role substitutions.
type ArtifactScalarReceipt struct {
	template     *ArtifactScalarTemplate
	capabilities map[identity.ContentID]RuleSlotCapability
	sealed       bool
}

// ArtifactScalarCapacity reserves the exact immutable row planes one builder
// will fill. It is allocation shape only; final admission still validates
// every row and relation.
type ArtifactScalarCapacity struct {
	Roles, Points, Edges, Transfers, Regions, Events, Rules, Bodies, Functions int
}

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

type ArtifactScalarPoint struct {
	ID        identity.ContentID
	Decisions []identity.ContentID
	Initial   bool
}

type ArtifactScalarEdge struct {
	ID, From, To, Route, Guard, Decision identity.ContentID
	Component, Mu, Reset                 identity.ContentID
	Resets                               []identity.ContentID
	Arm                                  ArtifactStructuralArm
	Guarded, Truth, HasReset             bool
}

type ArtifactScalarTransfer struct {
	ID, From, To identity.ContentID
	Full         bool
	Factors      []ArtifactScalarRole
}

type ArtifactScalarRegion struct {
	ID, Head, Parent identity.ContentID
	Cyclic           bool
	Members          []identity.ContentID
}

type ArtifactScalarEvent struct {
	Kind   ArtifactEventKind
	Region identity.ContentID
	Point  identity.ContentID
}

// ArtifactRuleStage is the engine-neutral scalar encoding of the execution
// cut sealed by ProgramArtifact. It is retained as proof metadata; engine does
// not infer it from transports or interpret a domain rule name.
type ArtifactRuleStage uint8

const (
	ArtifactRuleStageInvalid ArtifactRuleStage = iota
	ArtifactRuleStageBase
	ArtifactRuleStageLocal
	ArtifactRuleStageCallDispatch
	ArtifactRuleStageCallSummary
	ArtifactRuleStageCallEffect
)

func (stage ArtifactRuleStage) valid() bool {
	return stage >= ArtifactRuleStageBase && stage <= ArtifactRuleStageCallEffect
}

func (stage ArtifactRuleStage) nativeCall() bool {
	return stage >= ArtifactRuleStageCallDispatch && stage <= ArtifactRuleStageCallEffect
}

type ArtifactScalarRule struct {
	Role             ArtifactScalarRole
	Stage            ArtifactRuleStage
	Point, Input, ID identity.ContentID
	Route            identity.ContentID
}

type ArtifactScalarBody struct {
	ID, Context, SemanticEntry identity.ContentID
	Function, CallFormal       identity.ContentID
	Callable                   bool
	Entry, Exits               []identity.ContentID
}

type ArtifactScalarFormalPort struct {
	ID, Cell, Storage identity.ContentID
	Position          uint32
}

type ArtifactScalarVarargPort struct{ ID, Cell identity.ContentID }

type ArtifactScalarCapturePort struct {
	ID, Inner, Outer     identity.ContentID
	InnerBody, OuterBody identity.ContentID
	Position             uint32
}

// ArtifactScalarFunction is the engine-neutral Program interface. Its nested
// rows are appended through the single-use builder; callers cannot retain a
// mutable slice that aliases the sealed cached template.
type ArtifactScalarFunction struct {
	ID, Body, BodyContext, Entry, CallFormal identity.ContentID
	Formals                                  []ArtifactScalarFormalPort
	Vararg                                   ArtifactScalarVarargPort
	HasVararg                                bool
	Captures                                 []ArtifactScalarCapturePort
	Outcomes                                 []identity.ContentID
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

func NewArtifactScalarTemplate(spec *ArtifactScalarSpec) (*ArtifactScalarTemplate, bool) {
	if !validArtifactScalarSpec(spec) {
		return nil, false
	}
	state := spec.state
	template := &ArtifactScalarTemplate{artifact: state.ArtifactID, program: state.ProgramID, schema: state.SchemaID, sealed: true, roles: state.Roles, points: state.Points, edges: state.Edges, local: state.Transfers, regions: state.Regions, events: state.Events, rules: state.Rules, bodies: state.Bodies, functions: state.Functions}
	state.consumed = true
	state.Roles = nil
	state.Points = nil
	state.Edges = nil
	state.Transfers = nil
	state.Regions = nil
	state.Events = nil
	state.Rules = nil
	state.Bodies = nil
	state.Functions = nil
	return template, true
}

func NewArtifactScalarBinding(template *ArtifactScalarTemplate) (*ArtifactScalarBinding, bool) {
	if !template.Available() {
		return nil, false
	}
	return &ArtifactScalarBinding{template: template, capabilities: make(map[identity.ContentID]RuleSlotCapability, len(template.roles))}, true
}

func (binding *ArtifactScalarBinding) BindRole(role ArtifactScalarRole, capability RuleSlotCapability) bool {
	if binding == nil || binding.sealed || !binding.template.Available() || !role.Available() || !capability.mounted() {
		return false
	}
	owned := false
	for _, candidate := range binding.template.roles {
		if candidate == role {
			owned = true
			break
		}
	}
	if !owned {
		return false
	}
	if _, duplicate := binding.capabilities[role.semantic]; duplicate {
		return false
	}
	binding.capabilities[role.semantic] = capability
	return true
}

func NewArtifactScalarReceipt(binding *ArtifactScalarBinding) (*ArtifactScalarReceipt, bool) {
	if binding == nil || binding.sealed || !binding.template.Available() || len(binding.capabilities) != len(binding.template.roles) {
		return nil, false
	}
	seenCapabilities := make(map[RuleSlotCapability]struct{}, len(binding.template.roles))
	for _, role := range binding.template.roles {
		capability, ok := binding.capabilities[role.semantic]
		if !ok || !capability.mounted() {
			return nil, false
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return nil, false
		}
		seenCapabilities[capability] = struct{}{}
	}
	binding.sealed = true
	return &ArtifactScalarReceipt{template: binding.template, capabilities: binding.capabilities, sealed: true}, true
}

func (receipt *ArtifactScalarReceipt) capability(role ArtifactScalarRole) (RuleSlotCapability, bool) {
	if receipt == nil || !receipt.sealed || !receipt.template.Available() || !role.Available() {
		return RuleSlotCapability{}, false
	}
	capability, ok := receipt.capabilities[role.semantic]
	return capability, ok && capability.mounted()
}

// validArtifactScalarSpec is the sole template relational admission fence. Builder
// methods protect storage ownership and basic scalar availability; this final
// pass still rejects forged cross-row structure before ownership transfers to
// the sealed template. Every later phase then proves only role/mount substitution.
func validArtifactScalarSpec(spec *ArtifactScalarSpec) bool {
	state, open := spec.writable()
	if !open || !state.ArtifactID.Available() || !state.ProgramID.Available() || !state.SchemaID.Available() || len(state.Points) == 0 || len(state.Events) == 0 || len(state.Bodies) == 0 {
		return false
	}
	points := make(map[identity.ContentID]struct{}, len(state.Points))
	for _, point := range state.Points {
		if !point.ID.Available() {
			return false
		}
		if _, duplicate := points[point.ID]; duplicate {
			return false
		}
		points[point.ID] = struct{}{}
		for _, decision := range point.Decisions {
			if !decision.Available() {
				return false
			}
		}
		for index := 1; index < len(point.Decisions); index++ {
			if bytes.Compare(point.Decisions[index-1][:], point.Decisions[index][:]) >= 0 {
				return false
			}
		}
	}

	edgeIDs := make(map[identity.ContentID]struct{}, len(state.Edges)+len(state.Transfers))
	routes := make(map[identity.ContentID]artifactEnvironmentRow, len(state.Edges))
	duplicateRoutes := make(map[identity.ContentID]struct{})
	pointDecisions := make(map[identity.ContentID]map[identity.ContentID]struct{}, len(state.Points))
	for _, point := range state.Points {
		decisions := make(map[identity.ContentID]struct{}, len(point.Decisions))
		for _, decision := range point.Decisions {
			decisions[decision] = struct{}{}
		}
		pointDecisions[point.ID] = decisions
	}
	for _, edge := range state.Edges {
		row := artifactEnvironmentRow{
			id: edge.ID, from: edge.From, to: edge.To, route: edge.Route, guard: edge.Guard, decision: edge.Decision,
			guarded: edge.Guarded, truth: edge.Truth, component: edge.Component, mu: edge.Mu, reset: edge.Reset,
			hasReset: edge.HasReset, resets: edge.Resets, arm: edge.Arm,
			transportOnly: edge.From == edge.To && !edge.Component.Available() && !edge.HasReset && !edge.Mu.Available(),
		}
		if !validArtifactRouteProof(row) {
			return false
		}
		if _, fromOK := points[edge.From]; !fromOK {
			return false
		}
		if _, toOK := points[edge.To]; !toOK {
			return false
		}
		if edge.Guarded {
			if _, decisionOK := pointDecisions[edge.From][edge.Decision]; !decisionOK {
				return false
			}
		}
		if _, duplicate := edgeIDs[edge.ID]; duplicate {
			return false
		}
		if _, duplicate := routes[edge.Route]; duplicate {
			duplicateRoutes[edge.Route] = struct{}{}
		} else {
			routes[edge.Route] = row
		}
		edgeIDs[edge.ID] = struct{}{}
	}
	for _, edge := range state.Transfers {
		if !edge.ID.Available() || !edge.From.Available() || !edge.To.Available() || edge.From == edge.To || edge.Full == (len(edge.Factors) != 0) {
			return false
		}
		if _, duplicate := edgeIDs[edge.ID]; duplicate {
			return false
		}
		edgeIDs[edge.ID] = struct{}{}
		for _, role := range edge.Factors {
			if !scalarSpecOwnsRole(state, role) {
				return false
			}
		}
	}

	regions := make(map[identity.ContentID]ArtifactScalarRegion, len(state.Regions))
	for _, region := range state.Regions {
		if !region.ID.Available() || !region.Head.Available() || len(region.Members) == 0 || region.Members[0] != region.Head {
			return false
		}
		if _, duplicate := regions[region.ID]; duplicate {
			return false
		}
		members := make(map[identity.ContentID]struct{}, len(region.Members))
		for _, member := range region.Members {
			if _, pointOK := points[member]; !pointOK {
				return false
			}
			if _, duplicate := members[member]; duplicate {
				return false
			}
			members[member] = struct{}{}
		}
		regions[region.ID] = region
	}
	for _, region := range state.Regions {
		if region.Parent.Available() {
			if _, parentOK := regions[region.Parent]; !parentOK || region.Parent == region.ID {
				return false
			}
		}
	}

	if !validArtifactScalarSchedule(points, regions, state.Events) {
		return false
	}

	pointRank := make(map[identity.ContentID]int, len(points))
	for _, event := range state.Events {
		if event.Kind == ArtifactEventPoint {
			pointRank[event.Point] = len(pointRank)
		}
	}
	type artifactStageGeometry struct {
		stage ArtifactRuleStage
		input identity.ContentID
	}
	stageGeometry := make(map[identity.ContentID]artifactStageGeometry)
	type artifactTemplateRuleOccurrence struct {
		role ArtifactScalarRole
		id   identity.ContentID
	}
	nativeOccurrences := make(map[artifactTemplateRuleOccurrence]struct{})
	for _, rule := range state.Rules {
		if !scalarSpecOwnsRole(state, rule.Role) || !rule.Stage.valid() || !rule.Point.Available() || !rule.ID.Available() {
			return false
		}
		if _, pointOK := points[rule.Point]; !pointOK {
			return false
		}
		if rule.Input.Available() {
			if _, inputOK := points[rule.Input]; !inputOK {
				return false
			}
		}
		switch rule.Stage {
		case ArtifactRuleStageBase:
			if rule.Input.Available() || rule.Route.Available() {
				return false
			}
		case ArtifactRuleStageLocal, ArtifactRuleStageCallDispatch, ArtifactRuleStageCallSummary, ArtifactRuleStageCallEffect:
			inputRank, inputOK := pointRank[rule.Input]
			outputRank, pointOK := pointRank[rule.Point]
			if !rule.Input.Available() || rule.Input == rule.Point || !inputOK || !pointOK || inputRank >= outputRank {
				return false
			}
			geometry := artifactStageGeometry{stage: rule.Stage, input: rule.Input}
			if prior, duplicate := stageGeometry[rule.Point]; duplicate && prior != geometry {
				return false
			}
			stageGeometry[rule.Point] = geometry
		}
		if rule.Stage.nativeCall() {
			key := artifactTemplateRuleOccurrence{role: rule.Role, id: rule.ID}
			if _, duplicate := nativeOccurrences[key]; duplicate {
				return false
			}
			nativeOccurrences[key] = struct{}{}
		}
		if rule.Route.Available() {
			if _, duplicate := duplicateRoutes[rule.Route]; duplicate {
				return false
			}
			predecessor, predecessorOK := routes[rule.Route]
			if !predecessorOK || predecessor.from != rule.Input {
				return false
			}
		}
	}
	for point, geometry := range stageGeometry {
		switch geometry.stage {
		case ArtifactRuleStageLocal:
		case ArtifactRuleStageCallDispatch:
			if owner, staged := stageGeometry[geometry.input]; staged && owner.stage.nativeCall() {
				return false
			}
		case ArtifactRuleStageCallSummary:
			if owner, staged := stageGeometry[geometry.input]; !staged || owner.stage != ArtifactRuleStageCallDispatch || point == geometry.input {
				return false
			}
		case ArtifactRuleStageCallEffect:
			if owner, staged := stageGeometry[geometry.input]; !staged || owner.stage != ArtifactRuleStageCallSummary || point == geometry.input {
				return false
			}
		default:
			return false
		}
	}

	bodies := make(map[identity.ContentID]struct{}, len(state.Bodies))
	for _, body := range state.Bodies {
		if !body.ID.Available() || !body.Context.Available() || !body.SemanticEntry.Available() || len(body.Entry) == 0 || len(body.Exits) == 0 ||
			body.Callable != body.Function.Available() || body.Callable != body.CallFormal.Available() {
			return false
		}
		if _, duplicate := bodies[body.ID]; duplicate {
			return false
		}
		bodies[body.ID] = struct{}{}
		seenEntry := make(map[identity.ContentID]struct{}, len(body.Entry))
		for _, point := range body.Entry {
			if _, pointOK := points[point]; !pointOK {
				return false
			}
			if _, duplicate := seenEntry[point]; duplicate {
				return false
			}
			seenEntry[point] = struct{}{}
		}
		seenExit := make(map[identity.ContentID]struct{}, len(body.Exits))
		for _, point := range body.Exits {
			if _, pointOK := points[point]; !pointOK {
				return false
			}
			if _, duplicate := seenExit[point]; duplicate {
				return false
			}
			seenExit[point] = struct{}{}
		}
	}
	seenFunctions := make(map[identity.ContentID]struct{}, len(state.Functions))
	seenFunctionBodies := make(map[identity.ContentID]struct{}, len(state.Functions))
	for _, function := range state.Functions {
		if !function.ID.Available() || !function.Body.Available() || !function.BodyContext.Available() || !function.Entry.Available() || !function.CallFormal.Available() ||
			function.HasVararg != function.Vararg.ID.Available() || function.HasVararg != function.Vararg.Cell.Available() || len(function.Outcomes) == 0 {
			return false
		}
		bodyFound := false
		for _, body := range state.Bodies {
			if body.ID == function.Body {
				bodyFound = body.Callable && body.Context == function.BodyContext && body.SemanticEntry == function.Entry && body.Function == function.ID && body.CallFormal == function.CallFormal
				break
			}
		}
		if !bodyFound {
			return false
		}
		if _, duplicate := seenFunctions[function.ID]; duplicate {
			return false
		}
		if _, duplicate := seenFunctionBodies[function.Body]; duplicate {
			return false
		}
		seenFunctions[function.ID], seenFunctionBodies[function.Body] = struct{}{}, struct{}{}
		seenFormals := make(map[identity.ContentID]struct{}, len(function.Formals))
		seenCells := make(map[identity.ContentID]struct{}, len(function.Formals))
		seenStorage := make(map[identity.ContentID]struct{}, len(function.Formals))
		for index, port := range function.Formals {
			if !port.ID.Available() || !port.Cell.Available() || !port.Storage.Available() || uint64(port.Position) != uint64(index) {
				return false
			}
			if _, duplicate := seenFormals[port.ID]; duplicate {
				return false
			}
			if _, duplicate := seenCells[port.Cell]; duplicate {
				return false
			}
			if _, duplicate := seenStorage[port.Storage]; duplicate {
				return false
			}
			seenFormals[port.ID], seenCells[port.Cell], seenStorage[port.Storage] = struct{}{}, struct{}{}, struct{}{}
		}
		seenCaptures := make(map[identity.ContentID]struct{}, len(function.Captures))
		for index, capture := range function.Captures {
			if !capture.ID.Available() || !capture.Inner.Available() || !capture.Outer.Available() || capture.Inner == capture.Outer ||
				capture.InnerBody != function.Body || capture.InnerBody == capture.OuterBody || uint64(capture.Position) != uint64(index) {
				return false
			}
			if _, outerOK := bodies[capture.OuterBody]; !outerOK {
				return false
			}
			if _, duplicate := seenCaptures[capture.ID]; duplicate {
				return false
			}
			seenCaptures[capture.ID] = struct{}{}
		}
		seenOutcomes := make(map[identity.ContentID]struct{}, len(function.Outcomes))
		for _, outcome := range function.Outcomes {
			if !outcome.Available() {
				return false
			}
			if _, duplicate := seenOutcomes[outcome]; duplicate {
				return false
			}
			seenOutcomes[outcome] = struct{}{}
		}
	}
	callableCount := 0
	for _, body := range state.Bodies {
		if body.Callable {
			callableCount++
		}
	}
	if callableCount != len(state.Functions) {
		return false
	}
	return true
}

func validArtifactScalarSchedule(points map[identity.ContentID]struct{}, regions map[identity.ContentID]ArtifactScalarRegion, events []ArtifactScalarEvent) bool {
	if len(events) == 0 {
		return false
	}
	entered := make(map[identity.ContentID]bool, len(regions))
	exited := make(map[identity.ContentID]bool, len(regions))
	seenPoint := make(map[identity.ContentID]struct{}, len(points))
	type frame struct {
		region identity.ContentID
		next   int
	}
	stack := make([]frame, 0, len(regions))
	for _, event := range events {
		switch event.Kind {
		case ArtifactEventEnter:
			region, regionOK := regions[event.Region]
			if !regionOK || event.Point.Available() || entered[event.Region] || exited[event.Region] {
				return false
			}
			if len(stack) == 0 {
				if region.Parent.Available() {
					return false
				}
			} else {
				parent := stack[len(stack)-1]
				if parent.next == 0 || region.Parent != parent.region {
					return false
				}
			}
			entered[event.Region] = true
			stack = append(stack, frame{region: event.Region})
		case ArtifactEventPoint:
			if event.Region.Available() {
				return false
			}
			if _, pointOK := points[event.Point]; !pointOK {
				return false
			}
			if _, duplicate := seenPoint[event.Point]; duplicate {
				return false
			}
			if len(stack) != 0 {
				current := &stack[len(stack)-1]
				members := regions[current.region].Members
				if current.next >= len(members) || members[current.next] != event.Point {
					return false
				}
				current.next++
			}
			seenPoint[event.Point] = struct{}{}
		case ArtifactEventExit:
			region, regionOK := regions[event.Region]
			if !regionOK || event.Point.Available() || len(stack) == 0 || stack[len(stack)-1].region != event.Region || exited[event.Region] {
				return false
			}
			if stack[len(stack)-1].next != len(region.Members) {
				return false
			}
			exited[event.Region] = true
			stack = stack[:len(stack)-1]
		default:
			return false
		}
	}
	if len(stack) != 0 || len(seenPoint) != len(points) || len(entered) != len(regions) || len(exited) != len(regions) {
		return false
	}
	for region := range regions {
		if !entered[region] || !exited[region] {
			return false
		}
	}
	return true
}

// MountedArtifactReceipt is the opaque Link-owned input row for the sole
// multi-mount lowerer.  MountID is the parent-issued module/shard
// substitution identity, not a Program ID or a caller-selected ordinal.
type MountedArtifactReceipt struct {
	receipt *ArtifactScalarReceipt
	mountID identity.ContentID
}

// NewMountedArtifactReceipt binds one reusable Program artifact to one exact
// Link mount identity.  The fields stay opaque so only the parent assembly
// can preserve mount ordering and feed the lowerer.
func NewMountedArtifactReceipt(receipt *ArtifactScalarReceipt, mountID identity.ContentID) (MountedArtifactReceipt, bool) {
	row := MountedArtifactReceipt{receipt: receipt, mountID: mountID}
	return row, receipt != nil && receipt.sealed && receipt.template.Available() && mountID.Available()
}

type artifactMountReceipt struct {
	mount    identity.ContentID
	artifact identity.ContentID
	program  identity.ContentID
	initial  identity.ContentID
}

type artifactPointMetadata struct {
	mount     identity.ContentID
	artifact  identity.ContentID
	reusable  identity.ContentID
	decisions []identity.ContentID
	initial   bool
}

type artifactEnvironmentRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	reusable identity.ContentID
	id       identity.ContentID
	from     identity.ContentID
	to       identity.ContentID
	route    identity.ContentID
	guard    identity.ContentID
	decision identity.ContentID
	guarded  bool
	truth    bool
	// component, mu, arm, and reset are parent proof metadata. Equation has
	// no corresponding runtime fields; lowerArtifactRows validates them before
	// admitting the boundary and retains them in the immutable receipt.
	component     identity.ContentID
	mu            identity.ContentID
	reset         identity.ContentID
	hasReset      bool
	resets        []identity.ContentID
	arm           ArtifactStructuralArm
	transportOnly bool
	local         bool
	full          bool
	factorRoles   []RuleSlotCapability
}

type artifactWTORegionRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	reusable identity.ContentID
	id       identity.ContentID
	head     identity.ContentID
	parent   identity.ContentID
	cyclic   bool
	members  []identity.ContentID
}

type artifactWTOEventRow struct {
	mount    identity.ContentID
	artifact identity.ContentID
	kind     ArtifactEventKind
	region   identity.ContentID
	point    identity.ContentID
}

// ArtifactPointReceipt is a BindingTopology-issued view of one exact Program
// artifact point.  It is not an equation coordinate and cannot be minted from
// a ContentID by callers.
type ArtifactPointReceipt struct {
	topology *BindingTopology
	index    uint32
}

// MountedNativeCallStageReceipt is a cold, graph-owned proof that one exact
// mounted Call occurrence was attached at its ProgramArtifact-issued native
// stage. It is issued by occurrence alone: callers never submit a reusable
// point to this lookup and therefore cannot splice another artifact point.
type MountedNativeCallStageReceipt struct {
	graph *ReceiptGraph
	key   artifactMountedRuleOccurrence
	stage artifactNativeCallStage
}

func (receipt MountedNativeCallStageReceipt) row() (artifactNativeCallStage, bool) {
	if receipt.graph == nil || !receipt.graph.valid() || receipt.graph.topology == nil || receipt.graph.topology.nativeCallStages == nil {
		return artifactNativeCallStage{}, false
	}
	row, ok := receipt.graph.topology.nativeCallStages[receipt.key]
	return row, ok && row == receipt.stage && row.stage.nativeCall() && row.point.Available() && row.input.Available() && row.mountedPoint.Available() && row.mountedInput.Available()
}

func (receipt MountedNativeCallStageReceipt) Available() bool { _, ok := receipt.row(); return ok }
func (receipt MountedNativeCallStageReceipt) Stage() ArtifactRuleStage {
	row, ok := receipt.row()
	if !ok {
		return ArtifactRuleStageInvalid
	}
	return row.stage
}
func (receipt MountedNativeCallStageReceipt) MountID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.mount
}
func (receipt MountedNativeCallStageReceipt) OccurrenceID() identity.ContentID {
	if _, ok := receipt.row(); !ok {
		return identity.ContentID{}
	}
	return receipt.key.occurrence
}
func (receipt MountedNativeCallStageReceipt) ReusablePointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.point
}
func (receipt MountedNativeCallStageReceipt) ReusableInputPointID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.input
}

// RuleMember resolves the already-attached member authenticated by this
// stage proof. The caller cannot substitute another point or occurrence.
func (receipt MountedNativeCallStageReceipt) RuleMember() (ReceiptRuleMember, bool) {
	row, ok := receipt.row()
	if !ok {
		return ReceiptRuleMember{}, false
	}
	return receipt.graph.lookupRuleMember(mountedRuleMemberID(receipt.key.role, receipt.key.mount, row.point, receipt.key.occurrence))
}

// MountedNativeCallStage resolves the exact native stage by owner capability,
// mount, and occurrence. Point identity is output-only proof material.
func (receipt *ReceiptGraph) MountedNativeCallStage(role RuleSlotCapability, mount, occurrence identity.ContentID) (MountedNativeCallStageReceipt, bool) {
	if receipt == nil || !receipt.valid() || !role.mounted() || role.state != receipt.state || role.authority != receipt.authority || !mount.Available() || !occurrence.Available() || receipt.topology.nativeCallStages == nil {
		return MountedNativeCallStageReceipt{}, false
	}
	key := artifactMountedRuleOccurrence{role: role, mount: mount, occurrence: occurrence}
	stage, ok := receipt.topology.nativeCallStages[key]
	result := MountedNativeCallStageReceipt{graph: receipt, key: key, stage: stage}
	return result, ok && result.Available()
}

func (point ArtifactPointReceipt) Available() bool {
	return point.topology != nil && point.topology.valid() && point.topology.artifact != nil && uint64(point.index) < uint64(len(point.topology.artifact.points))
}
func (point ArtifactPointReceipt) ID() identity.ContentID {
	if !point.Available() {
		return identity.ContentID{}
	}
	return point.topology.artifact.points[point.index]
}
func (point ArtifactPointReceipt) MountID() identity.ContentID {
	if !point.Available() {
		return identity.ContentID{}
	}
	return point.topology.artifact.pointMeta[point.ID()].mount
}
func (point ArtifactPointReceipt) ArtifactID() identity.ContentID {
	if !point.Available() {
		return identity.ContentID{}
	}
	return point.topology.artifact.pointMeta[point.ID()].artifact
}
func (point ArtifactPointReceipt) ReusableID() identity.ContentID {
	if !point.Available() {
		return identity.ContentID{}
	}
	return point.topology.artifact.pointMeta[point.ID()].reusable
}

// ArtifactEnvironmentReceipt is one exact parent structural edge retained by
// BindingTopology.  It exposes scalar artifact semantics, never the private
// equation Input or PointRef used to lower it.
type ArtifactEnvironmentReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (edge ArtifactEnvironmentReceipt) row() (artifactEnvironmentRow, bool) {
	if edge.topology == nil || !edge.topology.valid() || edge.topology.artifact == nil || uint64(edge.index) >= uint64(len(edge.topology.artifact.edges)) {
		return artifactEnvironmentRow{}, false
	}
	return edge.topology.artifact.edges[edge.index], true
}
func (edge ArtifactEnvironmentReceipt) Available() bool { _, ok := edge.row(); return ok }
func (edge ArtifactEnvironmentReceipt) ID() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.id
}
func (edge ArtifactEnvironmentReceipt) From() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.from
}
func (edge ArtifactEnvironmentReceipt) To() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.to
}
func (edge ArtifactEnvironmentReceipt) RouteID() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.route
}
func (edge ArtifactEnvironmentReceipt) Arm() ArtifactStructuralArm {
	row, ok := edge.row()
	if !ok {
		return ArtifactStructuralArmInvalid
	}
	return row.arm
}
func (edge ArtifactEnvironmentReceipt) GuardID() (identity.ContentID, bool) {
	row, ok := edge.row()
	return row.guard, ok && row.guarded
}
func (edge ArtifactEnvironmentReceipt) DecisionID() (identity.ContentID, bool) {
	row, ok := edge.row()
	return row.decision, ok && row.guarded && row.decision.Available()
}
func (edge ArtifactEnvironmentReceipt) Truth() (bool, bool) {
	row, ok := edge.row()
	return row.truth, ok && row.guarded
}
func (edge ArtifactEnvironmentReceipt) ComponentID() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.component
}
func (edge ArtifactEnvironmentReceipt) MuPathID() (identity.ContentID, bool) {
	row, ok := edge.row()
	return row.mu, ok && row.mu.Available()
}
func (edge ArtifactEnvironmentReceipt) HasResetWitness() bool {
	row, ok := edge.row()
	return ok && row.hasReset
}
func (edge ArtifactEnvironmentReceipt) ResetDigest() (identity.ContentID, bool) {
	row, ok := edge.row()
	return row.reset, ok && row.hasReset
}
func (edge ArtifactEnvironmentReceipt) ResetCount() int {
	row, ok := edge.row()
	if !ok {
		return 0
	}
	return len(row.resets)
}
func (edge ArtifactEnvironmentReceipt) ResetAt(index int) (identity.ContentID, bool) {
	row, ok := edge.row()
	if !ok || index < 0 || index >= len(row.resets) {
		return identity.ContentID{}, false
	}
	return row.resets[index], true
}
func (edge ArtifactEnvironmentReceipt) MountID() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.mount
}
func (edge ArtifactEnvironmentReceipt) ReusableID() identity.ContentID {
	row, ok := edge.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.reusable
}

// ArtifactWTOEventReceipt is one exact parent-issued WTO bracket/point row.
// Region and Point are a closed sum: callers cannot manufacture a mixed row.
type ArtifactWTOEventReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (event ArtifactWTOEventReceipt) row() (artifactWTOEventRow, bool) {
	if event.topology == nil || !event.topology.valid() || event.topology.artifact == nil || uint64(event.index) >= uint64(len(event.topology.artifact.events)) {
		return artifactWTOEventRow{}, false
	}
	return event.topology.artifact.events[event.index], true
}
func (event ArtifactWTOEventReceipt) Available() bool { _, ok := event.row(); return ok }
func (event ArtifactWTOEventReceipt) Kind() ArtifactEventKind {
	row, ok := event.row()
	if !ok {
		return ArtifactEventInvalid
	}
	return row.kind
}
func (event ArtifactWTOEventReceipt) RegionID() identity.ContentID {
	row, ok := event.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.region
}
func (event ArtifactWTOEventReceipt) PointID() identity.ContentID {
	row, ok := event.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.point
}

// ArtifactWTORegionReceipt is one exact parent-issued WTO region. Member
// order is preserved verbatim from the artifact and remains separate from
// Equation's private point coordinates.
type ArtifactWTORegionReceipt struct {
	topology *BindingTopology
	index    uint32
}

func (region ArtifactWTORegionReceipt) row() (artifactWTORegionRow, bool) {
	if region.topology == nil || !region.topology.valid() || region.topology.artifact == nil || uint64(region.index) >= uint64(len(region.topology.artifact.regions)) {
		return artifactWTORegionRow{}, false
	}
	return region.topology.artifact.regions[region.index], true
}
func (region ArtifactWTORegionReceipt) Available() bool { _, ok := region.row(); return ok }
func (region ArtifactWTORegionReceipt) ID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.id
}
func (region ArtifactWTORegionReceipt) Head() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.head
}
func (region ArtifactWTORegionReceipt) ParentID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.parent
}
func (region ArtifactWTORegionReceipt) Cyclic() bool {
	row, ok := region.row()
	return ok && row.cyclic
}
func (region ArtifactWTORegionReceipt) MemberCount() int {
	row, ok := region.row()
	if !ok {
		return 0
	}
	return len(row.members)
}
func (region ArtifactWTORegionReceipt) MemberAt(index int) (identity.ContentID, bool) {
	row, ok := region.row()
	if !ok || index < 0 || index >= len(row.members) {
		return identity.ContentID{}, false
	}
	return row.members[index], true
}
func (region ArtifactWTORegionReceipt) MountID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.mount
}
func (region ArtifactWTORegionReceipt) ReusableID() identity.ContentID {
	row, ok := region.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.reusable
}

// MountedFunctionBoundaryReceipt is Runtime's compact mount-qualified view of
// one reusable Program interface. It survives ReleaseArtifactReceipt and is
// issued only by the exact graph receipt that owns the mounted topology.
type MountedFunctionBoundaryReceipt struct {
	graph *ReceiptGraph
	index uint32
	id    identity.ContentID
}

func (receipt MountedFunctionBoundaryReceipt) row() (artifactMountedFunction, bool) {
	if receipt.graph == nil || !receipt.graph.valid() || receipt.graph.topology == nil || uint64(receipt.index) >= uint64(len(receipt.graph.topology.artifactFunctions)) {
		return artifactMountedFunction{}, false
	}
	row := receipt.graph.topology.artifactFunctions[receipt.index]
	return row, row.id == receipt.id && row.id.Available()
}
func (receipt MountedFunctionBoundaryReceipt) Available() bool { _, ok := receipt.row(); return ok }
func (receipt MountedFunctionBoundaryReceipt) ID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.id
}
func (receipt MountedFunctionBoundaryReceipt) MountID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.mount
}
func (receipt MountedFunctionBoundaryReceipt) ArtifactID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.artifact
}
func (receipt MountedFunctionBoundaryReceipt) ReusableID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.reusable
}
func (receipt MountedFunctionBoundaryReceipt) BodyID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.body
}
func (receipt MountedFunctionBoundaryReceipt) BodyContextID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.bodyContext
}
func (receipt MountedFunctionBoundaryReceipt) EntryID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.entry
}
func (receipt MountedFunctionBoundaryReceipt) CallFormalID() identity.ContentID {
	row, ok := receipt.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.callFormal
}
func (receipt MountedFunctionBoundaryReceipt) FormalCount() int {
	row, ok := receipt.row()
	if !ok {
		return 0
	}
	return len(row.formals)
}
func (receipt MountedFunctionBoundaryReceipt) FormalAt(index int) (MountedFunctionFormalPort, bool) {
	row, ok := receipt.row()
	if !ok || index < 0 || index >= len(row.formals) {
		return MountedFunctionFormalPort{}, false
	}
	port := MountedFunctionFormalPort{function: receipt, index: uint32(index), id: row.formals[index].ID}
	return port, port.Available()
}
func (receipt MountedFunctionBoundaryReceipt) Vararg() (MountedFunctionVarargPort, bool) {
	row, ok := receipt.row()
	if !ok || !row.hasVararg {
		return MountedFunctionVarargPort{}, false
	}
	port := MountedFunctionVarargPort{function: receipt, id: row.vararg.ID, cell: row.vararg.Cell}
	return port, port.Available()
}
func (receipt MountedFunctionBoundaryReceipt) CaptureCount() int {
	row, ok := receipt.row()
	if !ok {
		return 0
	}
	return len(row.captures)
}
func (receipt MountedFunctionBoundaryReceipt) CaptureAt(index int) (MountedFunctionCapturePort, bool) {
	row, ok := receipt.row()
	if !ok || index < 0 || index >= len(row.captures) {
		return MountedFunctionCapturePort{}, false
	}
	port := MountedFunctionCapturePort{function: receipt, index: uint32(index), id: row.captures[index].ID}
	return port, port.Available()
}
func (receipt MountedFunctionBoundaryReceipt) OutcomeCount() int {
	row, ok := receipt.row()
	if !ok {
		return 0
	}
	return len(row.outcomes)
}
func (receipt MountedFunctionBoundaryReceipt) OutcomeAt(index int) (identity.ContentID, bool) {
	row, ok := receipt.row()
	if !ok || index < 0 || index >= len(row.outcomes) {
		return identity.ContentID{}, false
	}
	return row.outcomes[index], true
}

type MountedFunctionFormalPort struct {
	function MountedFunctionBoundaryReceipt
	index    uint32
	id       identity.ContentID
}

func (port MountedFunctionFormalPort) row() (ArtifactScalarFormalPort, bool) {
	function, ok := port.function.row()
	if !ok || uint64(port.index) >= uint64(len(function.formals)) {
		return ArtifactScalarFormalPort{}, false
	}
	row := function.formals[port.index]
	return row, row.ID == port.id && uint64(row.Position) == uint64(port.index)
}
func (port MountedFunctionFormalPort) Available() bool { _, ok := port.row(); return ok }
func (port MountedFunctionFormalPort) ID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.ID
}
func (port MountedFunctionFormalPort) CellID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.Cell
}
func (port MountedFunctionFormalPort) StorageCellID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.Storage
}
func (port MountedFunctionFormalPort) Position() (int, bool) {
	row, ok := port.row()
	return int(row.Position), ok
}

type MountedFunctionVarargPort struct {
	function MountedFunctionBoundaryReceipt
	id, cell identity.ContentID
}

func (port MountedFunctionVarargPort) Available() bool {
	row, ok := port.function.row()
	return ok && row.hasVararg && row.vararg.ID == port.id && row.vararg.Cell == port.cell && port.id.Available() && port.cell.Available()
}
func (port MountedFunctionVarargPort) ID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.id
}
func (port MountedFunctionVarargPort) CellID() identity.ContentID {
	if !port.Available() {
		return identity.ContentID{}
	}
	return port.cell
}

type MountedFunctionCapturePort struct {
	function MountedFunctionBoundaryReceipt
	index    uint32
	id       identity.ContentID
}

func (port MountedFunctionCapturePort) row() (ArtifactScalarCapturePort, bool) {
	function, ok := port.function.row()
	if !ok || uint64(port.index) >= uint64(len(function.captures)) {
		return ArtifactScalarCapturePort{}, false
	}
	row := function.captures[port.index]
	return row, row.ID == port.id && uint64(row.Position) == uint64(port.index)
}
func (port MountedFunctionCapturePort) Available() bool { _, ok := port.row(); return ok }
func (port MountedFunctionCapturePort) ID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.ID
}
func (port MountedFunctionCapturePort) InnerCellID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.Inner
}
func (port MountedFunctionCapturePort) OuterCellID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.Outer
}
func (port MountedFunctionCapturePort) InnerBodyID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.InnerBody
}
func (port MountedFunctionCapturePort) OuterBodyID() identity.ContentID {
	row, ok := port.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.OuterBody
}
func (port MountedFunctionCapturePort) Position() (int, bool) {
	row, ok := port.row()
	return int(row.Position), ok
}

func (receipt *ReceiptGraph) MountedFunctionCount() int {
	if receipt == nil || !receipt.valid() {
		return 0
	}
	return len(receipt.topology.artifactFunctions)
}
func (receipt *ReceiptGraph) MountedFunctionAt(index int) (MountedFunctionBoundaryReceipt, bool) {
	if receipt == nil || !receipt.valid() || index < 0 || index >= len(receipt.topology.artifactFunctions) {
		return MountedFunctionBoundaryReceipt{}, false
	}
	row := receipt.topology.artifactFunctions[index]
	result := MountedFunctionBoundaryReceipt{graph: receipt, index: uint32(index), id: row.id}
	return result, result.Available()
}

// ArtifactPointCount, ArtifactPointAt, ArtifactEnvironmentCount,
// ArtifactEnvironmentAt, ArtifactWTORegionCount, ArtifactWTORegionAt,
// ArtifactWTOEventCount and ArtifactWTOEventAt are
// the typed receipt projections.  They intentionally expose no generic
// source-row builder, equation capability, or artifact pointer.
func (receipt *BindingTopology) ArtifactPointCount() int {
	if !receipt.valid() || receipt.artifact == nil {
		return 0
	}
	return len(receipt.artifact.points)
}
func (receipt *BindingTopology) ArtifactPointAt(index int) (ArtifactPointReceipt, bool) {
	if !receipt.valid() || receipt.artifact == nil || index < 0 || index >= len(receipt.artifact.points) {
		return ArtifactPointReceipt{}, false
	}
	return ArtifactPointReceipt{topology: receipt, index: uint32(index)}, true
}
func (receipt *BindingTopology) ArtifactEnvironmentCount() int {
	if !receipt.valid() || receipt.artifact == nil {
		return 0
	}
	return len(receipt.artifact.edges)
}
func (receipt *BindingTopology) ArtifactEnvironmentAt(index int) (ArtifactEnvironmentReceipt, bool) {
	if !receipt.valid() || receipt.artifact == nil || index < 0 || index >= len(receipt.artifact.edges) {
		return ArtifactEnvironmentReceipt{}, false
	}
	return ArtifactEnvironmentReceipt{topology: receipt, index: uint32(index)}, true
}
func (receipt *BindingTopology) ArtifactWTOEventCount() int {
	if !receipt.valid() || receipt.artifact == nil {
		return 0
	}
	return len(receipt.artifact.events)
}
func (receipt *BindingTopology) ArtifactWTORegionCount() int {
	if !receipt.valid() || receipt.artifact == nil {
		return 0
	}
	return len(receipt.artifact.regions)
}
func (receipt *BindingTopology) ArtifactWTORegionAt(index int) (ArtifactWTORegionReceipt, bool) {
	if !receipt.valid() || receipt.artifact == nil || index < 0 || index >= len(receipt.artifact.regions) {
		return ArtifactWTORegionReceipt{}, false
	}
	return ArtifactWTORegionReceipt{topology: receipt, index: uint32(index)}, true
}
func (receipt *BindingTopology) ArtifactWTOEventAt(index int) (ArtifactWTOEventReceipt, bool) {
	if !receipt.valid() || receipt.artifact == nil || index < 0 || index >= len(receipt.artifact.events) {
		return ArtifactWTOEventReceipt{}, false
	}
	return ArtifactWTOEventReceipt{topology: receipt, index: uint32(index)}, true
}

// BeginMountedArtifactReceiptAssembly is the sole Link-cardinality-aware
// ProgramArtifact structural lowerer. It consumes every ordered mount into
// one Binding-owned Batch and Topology; duplicate reusable Program artifacts
// remain distinct because every lowered identity is mount-qualified.
func BeginMountedArtifactReceiptAssembly(binding *SchemaBinding, mounts []MountedArtifactReceipt, bootstrap ...LinkBootstrapWitness) (*ReceiptAssembly, bool) {
	assembly, _, ok := BeginMountedArtifactReceiptAssemblyWithFailure(binding, mounts, bootstrap...)
	return assembly, ok
}

type ReceiptAssemblyFailure uint8

const (
	ReceiptAssemblyFailureNone ReceiptAssemblyFailure = iota
	ReceiptAssemblyFailureInput
	ReceiptAssemblyFailureSchema
	ReceiptAssemblyFailureSnapshot
	ReceiptAssemblyFailureTransaction
	ReceiptAssemblyFailureStructuralRows
	// Snapshot sub-stages preserve the owner boundary that rejected an
	// otherwise immutable artifact. They are diagnostics only: no partial
	// receipt escapes the assembly.
	ReceiptAssemblyFailureSnapshotBootstrap
	ReceiptAssemblyFailureSnapshotMount
	ReceiptAssemblyFailureSnapshotArtifact
	ReceiptAssemblyFailureSnapshotNamespace
	ReceiptAssemblyFailureSnapshotTopology
	ReceiptAssemblyFailureSnapshotTopologyMount
	ReceiptAssemblyFailureSnapshotTopologyPoint
	ReceiptAssemblyFailureSnapshotTopologyRegion
	ReceiptAssemblyFailureSnapshotTopologyEdge
	ReceiptAssemblyFailureSnapshotTopologyEvent
	ReceiptAssemblyFailureSnapshotTopologyBootstrap
	ReceiptAssemblyFailureSnapshotTopologySchedule
	ReceiptAssemblyFailureSnapshotTopologyRule
)

// BeginMountedArtifactReceiptAssemblyWithFailure exposes only the first
// closed assembly phase. It is the permanent diagnostic entrypoint used by
// production; it never returns a partial snapshot or mutable row.
func BeginMountedArtifactReceiptAssemblyWithFailure(binding *SchemaBinding, mounts []MountedArtifactReceipt, bootstrap ...LinkBootstrapWitness) (*ReceiptAssembly, ReceiptAssemblyFailure, bool) {
	if binding == nil || !binding.Sealed() || len(mounts) == 0 || len(bootstrap) != 1 || !bootstrap[0].Available() {
		return nil, ReceiptAssemblyFailureInput, false
	}
	schema := binding.Schema()
	if schema == nil || !schema.Available() {
		return nil, ReceiptAssemblyFailureSchema, false
	}
	rows, snapshotFailure := snapshotMountedArtifactReceipts(mounts, identity.ContentID(schema.ID().Digest()), bootstrap[0], binding.state)
	if snapshotFailure != ReceiptAssemblyFailureNone {
		return nil, snapshotFailure, false
	}
	assembly, ok := beginReceiptAssembly(binding)
	if !ok {
		return nil, ReceiptAssemblyFailureTransaction, false
	}
	if !lowerArtifactReceipt(assembly, rows) {
		assembly.Abort()
		return nil, ReceiptAssemblyFailureStructuralRows, false
	}
	return assembly, ReceiptAssemblyFailureNone, true
}

func artifactReceiptKey(id identity.ContentID, version uint64) (composition.Key, bool) {
	key := composition.Key{ID: composition.ID(id), Version: version}
	return key, id.Available() && key.Available()
}

func lowerArtifactReceipt(assembly *ReceiptAssembly, rows *artifactReceiptTopology) bool {
	if assembly == nil || assembly.builder == nil || rows == nil {
		return false
	}
	sites := make(map[identity.ContentID]equation.Site, len(rows.points))
	for _, id := range rows.points {
		source, sourceOK := artifactReceiptKey(id, artifactPointSourceVersion)
		metadata, metadataOK := rows.pointMeta[id]
		if !metadataOK || !metadata.reusable.Available() {
			return false
		}
		decisions := make([]equation.Decision, len(metadata.decisions))
		for index, semanticID := range metadata.decisions {
			decisionKey := mountedArtifactID("analysis/engine/artifact-decision/v1", metadata.mount, metadata.artifact, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(decisionKey, artifactPointSourceVersion))
			if !decisionOK {
				return false
			}
			decisions[index] = decision
		}
		scope, scopeOK := equation.NewScope(decisions...)
		if !scopeOK {
			return false
		}
		init := equation.FalseExpr()
		disposition := equation.InitAbsent
		if metadata.initial {
			init = equation.TrueExpr()
			disposition = equation.InitPresent
		}
		site, siteOK := assembly.builder.admitSite(source, scope, init, disposition)
		if !sourceOK || !siteOK {
			return false
		}
		sites[id] = site
	}
	if rows.bootstrap != nil {
		point := rows.bootstrap.point
		source, sourceOK := artifactReceiptKey(point.PointID, artifactPointSourceVersion)
		decisions := make([]equation.Decision, len(point.DecisionID))
		for index, semanticID := range point.DecisionID {
			decisionKey := mountedArtifactID("analysis/engine/link-bootstrap-decision/v1", rows.bootstrap.owner, rows.bootstrap.owner, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(decisionKey, artifactPointSourceVersion))
			if !decisionOK {
				return false
			}
			decisions[index] = decision
		}
		scope, scopeOK := equation.NewScope(decisions...)
		init := equation.FalseExpr()
		disposition := equation.InitAbsent
		if point.Initial {
			init = equation.TrueExpr()
			disposition = equation.InitPresent
		}
		if !sourceOK || !scopeOK {
			return false
		}
		site, siteOK := assembly.builder.admitSite(source, scope, init, disposition)
		if !siteOK {
			return false
		}
		rows.bootstrap.site = site
		sites[point.PointID] = site
	}
	rows.sites = sites
	rows.mounted = make(map[artifactMountedPoint]equation.Site, len(rows.pointMeta))
	for id, metadata := range rows.pointMeta {
		site, siteOK := sites[id]
		if !siteOK {
			return false
		}
		key := artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}
		if _, duplicate := rows.mounted[key]; duplicate {
			return false
		}
		rows.mounted[key] = site
	}
	inner, locked := assembly.builder.lockSourcesOpen()
	if !locked || inner.artifact != nil {
		if locked {
			inner.failLocked()
			inner.mu.Unlock()
		}
		return false
	}
	inner.artifact = rows
	inner.mu.Unlock()
	return true
}

func mustArtifactReceiptKey(id identity.ContentID, version uint64) composition.Key {
	key, _ := artifactReceiptKey(id, version)
	return key
}

// lowerArtifactRows completes the artifact's point/edge rows after all
// typed operand rows have been admitted to the open source Batch.  Keeping
// this commit after SealSources preserves one canonical Batch key while
// allowing the analysis dispatcher to issue every exact mounted operand.
type ReceiptArtifactRowFailure uint8

const (
	ReceiptArtifactRowFailureNone ReceiptArtifactRowFailure = iota
	ReceiptArtifactRowFailureOwner
	ReceiptArtifactRowFailurePoint
	ReceiptArtifactRowFailureBootstrap
	ReceiptArtifactRowFailureEdgeMetadata
	ReceiptArtifactRowFailureEdgeProof
	ReceiptArtifactRowFailureEdgeReset
	ReceiptArtifactRowFailureEdgeDecision
	ReceiptArtifactRowFailureEdgeReindex
	ReceiptArtifactRowFailureEdgeGuard
	ReceiptArtifactRowFailureEdgeReceipt
	ReceiptArtifactRowFailureEvent
	ReceiptArtifactRowFailureSchedule
)

func (builder *bindingTopologyBuilder) lowerArtifactRows() (ReceiptArtifactRowFailure, uint32, bool) {
	if builder == nil {
		return ReceiptArtifactRowFailureOwner, 0, false
	}
	inner, locked := builder.lockTopologyOpen()
	if !locked {
		return ReceiptArtifactRowFailureOwner, 0, false
	}
	rows := inner.artifact
	inner.mu.Unlock()
	if rows == nil {
		return ReceiptArtifactRowFailureNone, 0, true
	}
	sites := rows.sites
	refs := make(map[identity.ContentID]equation.PointRef, len(rows.points))
	pointDecisions := make(map[identity.ContentID]map[identity.ContentID]equation.Decision, len(rows.points))
	for pointIndex, id := range rows.points {
		site, siteOK := sites[id]
		row, issued := builder.issuePointRow(equation.PointSpec{Site: site})
		ref, added := builder.addSemanticPoint(id, row)
		if !siteOK || !issued || !added {
			return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
		}
		refs[id] = ref.ref
		metadata, metadataOK := rows.pointMeta[id]
		if !metadataOK {
			return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
		}
		decisions := make(map[identity.ContentID]equation.Decision, len(metadata.decisions))
		for _, semanticID := range metadata.decisions {
			decisionKey := mountedArtifactID("analysis/engine/artifact-decision/v1", metadata.mount, metadata.artifact, semanticID)
			decision, decisionOK := equation.NewDecision(mustArtifactReceiptKey(decisionKey, artifactPointSourceVersion))
			if !decisionOK {
				return ReceiptArtifactRowFailurePoint, uint32(pointIndex), false
			}
			decisions[semanticID] = decision
		}
		pointDecisions[id] = decisions
	}
	if rows.bootstrap != nil {
		point := rows.bootstrap.point.PointID
		row, issued := builder.issuePointRow(equation.PointSpec{Site: rows.sites[point]})
		semanticID := linkBootstrapPointSemanticID(rows.bootstrap.owner, point)
		ref, added := builder.addSemanticPoint(semanticID, row)
		if !issued || !added || !semanticID.Available() {
			return ReceiptArtifactRowFailureBootstrap, 0, false
		}
		rows.bootstrap.semantic = semanticID
		rows.bootstrap.ref = ref.ref
	}
	for edgeIndex, edge := range rows.edges {
		from, fromOK := sites[edge.from]
		to, toOK := sites[edge.to]
		target, targetOK := refs[edge.to]
		provenance, provenanceOK := artifactReceiptKey(edge.id, artifactEdgeSourceVersion)
		fromMetadata, fromMetadataOK := rows.pointMeta[edge.from]
		_, toMetadataOK := rows.pointMeta[edge.to]
		fromDecisions, fromDecisionsOK := pointDecisions[edge.from]
		toDecisions, toDecisionsOK := pointDecisions[edge.to]
		if !fromMetadataOK || !toMetadataOK || !fromDecisionsOK || !toDecisionsOK {
			return ReceiptArtifactRowFailureEdgeMetadata, uint32(edgeIndex), false
		}
		maps := make([]equation.DecisionMap, len(fromMetadata.decisions))
		resetSet := make(map[identity.ContentID]struct{}, len(edge.resets))
		for _, resetID := range edge.resets {
			// A recurrence reset is conditional on the decision being live at
			// this source Point. Parent reset receipts may contain decisions
			// outside the current lexical scope; clearing absent information is
			// an exact no-op, not malformed evidence.
			resetSet[resetID] = struct{}{}
		}
		for index, semanticID := range fromMetadata.decisions {
			decision, decisionOK := fromDecisions[semanticID]
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeDecision, uint32(edgeIndex), false
			}
			if _, forgotten := resetSet[semanticID]; forgotten {
				maps[index] = equation.Forget(decision)
			} else {
				targetDecision, retained := toDecisions[semanticID]
				if !retained {
					// Leaving a decision scope is an ordinary exact projection,
					// independent of recurrence reset. A reset is parent proof
					// that a still-scoped decision must be forgotten; absence
					// from the target Point is already the parent-owned proof that
					// this route leaves that decision's lexical scope.
					maps[index] = equation.Forget(decision)
					continue
				}
				maps[index] = equation.Identity(decision)
				if targetDecision != decision {
					maps[index] = equation.Rename(decision, targetDecision)
				}
			}
		}
		sourceScope := from.Scope()
		targetScope := to.Scope()
		omega, omegaOK := equation.NewReindex(sourceScope, targetScope, maps)
		if !omegaOK {
			return ReceiptArtifactRowFailureEdgeReindex, uint32(edgeIndex), false
		}
		pre := equation.TrueExpr()
		if edge.guarded {
			decision, decisionOK := fromDecisions[edge.decision]
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
			}
			pre, decisionOK = equation.DecisionExpr(decision)
			if !decisionOK {
				return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
			}
			if !edge.truth {
				pre, decisionOK = equation.NotExpr(pre)
				if !decisionOK {
					return ReceiptArtifactRowFailureEdgeGuard, uint32(edgeIndex), false
				}
			}
		}
		input := equation.BoundaryInput(from, to, provenance, pre, omega, equation.TrueExpr())
		if !fromOK || !toOK || !targetOK || !provenanceOK || !input.Available() {
			return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
		}
		if edge.local && !edge.full {
			seenFactors := make(map[composition.Key]struct{}, len(edge.factorRoles))
			for _, role := range edge.factorRoles {
				factor, semantic, factorOK := artifactTransportFactor(builder.inner, role)
				if _, duplicate := seenFactors[semantic]; duplicate {
					factorOK = false
				}
				if !factorOK {
					return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
				}
				receipt, issued := builder.issueFactorEdge(factor, equation.FactorEdge{Target: target, Input: input, Factor: semantic})
				if !issued || !builder.addFactorEdge(receipt) {
					return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
				}
				seenFactors[semantic] = struct{}{}
			}
			continue
		}
		receipt, issued := builder.issueEnvironmentEdge(equation.EnvironmentEdge{Target: target, Input: input, TransportOnly: edge.transportOnly})
		if !issued || !builder.addEnvironmentEdge(receipt) {
			return ReceiptArtifactRowFailureEdgeReceipt, uint32(edgeIndex), false
		}
	}
	if rows.bootstrap != nil {
		for transportIndex, transport := range rows.bootstrap.transports {
			factor, semantic, factorOK := bootstrapTransportFactor(builder.inner, transport.capability)
			if !factorOK || semantic != transport.factor {
				return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
			}
			for _, mount := range rows.mounts {
				targetID := mount.initial
				metadata, metadataOK := rows.pointMeta[targetID]
				if !metadataOK || !metadata.initial || metadata.mount != mount.mount || metadata.artifact != mount.artifact {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
				targetSite, siteOK := rows.sites[targetID]
				target, targetOK := refs[targetID]
				provenance, provenanceOK := linkBootstrapTransportKey(rows.bootstrap.owner, metadata, semantic)
				reindex, reindexOK := ruleInputReindex(rows.bootstrap.site.Scope(), targetSite.Scope())
				input := equation.BoundaryInput(rows.bootstrap.site, targetSite, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
				if !siteOK || !targetOK || !provenanceOK || !reindexOK || !input.Available() {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
				receipt, issued := builder.issueFactorEdge(factor, equation.FactorEdge{Target: target, Input: input, Factor: semantic})
				if !issued || !builder.addFactorEdge(receipt) {
					return ReceiptArtifactRowFailureBootstrap, uint32(transportIndex), false
				}
			}
		}
	}
	// The artifact WTO stream is the parent-issued semantic order.  Convert
	// its point events to dense ranks aligned with the equation PointSpec
	// rows; point IDs are mounted and therefore cannot be used as tie-breaks.
	eventRank := make(map[identity.ContentID]int, len(rows.points))
	for eventIndex, event := range rows.events {
		if event.kind != ArtifactEventPoint {
			continue
		}
		if !event.point.Available() {
			return ReceiptArtifactRowFailureEvent, uint32(eventIndex), false
		}
		if _, duplicate := eventRank[event.point]; duplicate {
			return ReceiptArtifactRowFailureEvent, uint32(eventIndex), false
		}
		eventRank[event.point] = len(eventRank)
	}
	if len(eventRank) != len(rows.points) {
		return ReceiptArtifactRowFailureEvent, uint32(len(rows.events)), false
	}
	rows.pointRef = refs
	rows.mountedRef = make(map[artifactMountedPoint]equation.PointRef, len(rows.pointMeta))
	for id, metadata := range rows.pointMeta {
		ref, refOK := refs[id]
		if !refOK {
			return ReceiptArtifactRowFailurePoint, 0, false
		}
		rows.mountedRef[artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}] = ref
	}
	inner, locked = builder.lockTopologyOpen()
	if !locked {
		return ReceiptArtifactRowFailureSchedule, 0, false
	}
	pointRanks := make([]int, len(inner.spec.Points))
	bootstrapOffset := 0
	if rows.bootstrap != nil {
		bootstrapOffset = 1
	}
	if len(pointRanks) != len(rows.points)+bootstrapOffset {
		inner.failLocked()
		inner.mu.Unlock()
		return ReceiptArtifactRowFailureSchedule, 0, false
	}
	for index, point := range rows.points {
		rank, rankOK := eventRank[point]
		if !rankOK {
			inner.failLocked()
			inner.mu.Unlock()
			return ReceiptArtifactRowFailureSchedule, uint32(index), false
		}
		pointRanks[index] = rank + bootstrapOffset
	}
	if rows.bootstrap != nil {
		// Link bootstrap is intentionally the deterministic rank-zero anchor;
		// all mounted artifact points retain their local WTO order after it.
		pointRanks[len(rows.points)] = 0
	}
	inner.spec.PointRanks = pointRanks
	inner.mu.Unlock()
	return ReceiptArtifactRowFailureNone, 0, true
}

func validArtifactRouteProof(edge artifactEnvironmentRow) bool {
	if edge.local {
		if !edge.id.Available() || !edge.from.Available() || !edge.to.Available() || edge.from == edge.to || edge.full == (len(edge.factorRoles) != 0) || edge.transportOnly != edge.full ||
			edge.route.Available() || edge.guard.Available() || edge.decision.Available() || edge.guarded || edge.truth ||
			edge.component.Available() || edge.mu.Available() || edge.reset.Available() || edge.hasReset || len(edge.resets) != 0 || edge.arm != ArtifactStructuralArmInvalid {
			return false
		}
		seen := make(map[RuleSlotCapability]struct{}, len(edge.factorRoles))
		for _, role := range edge.factorRoles {
			if !role.mounted() {
				return false
			}
			if _, duplicate := seen[role]; duplicate {
				return false
			}
			seen[role] = struct{}{}
		}
		return true
	}
	if !edge.id.Available() || !edge.route.Available() || !edge.arm.Valid() {
		return false
	}
	if edge.transportOnly != (edge.from == edge.to && !edge.component.Available() && !edge.mu.Available() && !edge.hasReset) {
		return false
	}
	if edge.guarded != edge.guard.Available() || edge.guarded != edge.decision.Available() || edge.hasReset != edge.reset.Available() || edge.mu.Available() != edge.hasReset {
		return false
	}
	for index, reset := range edge.resets {
		if !reset.Available() || index > 0 && bytes.Compare(edge.resets[index-1][:], reset[:]) >= 0 {
			return false
		}
	}
	return true
}

func artifactTransportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if !role.mounted() {
		return nil, composition.Key{}, false
	}
	return transportFactor(inner, role)
}

func bootstrapTransportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if !role.link() {
		return nil, composition.Key{}, false
	}
	return transportFactor(inner, role)
}

func transportFactor(inner *bindingTopologyBuilderState, role RuleSlotCapability) (bindingFactorReceipt, composition.Key, bool) {
	if inner == nil || inner.state == nil || inner.state.schema == nil || !role.available() || role.state != inner.state || role.authority != inner.authority {
		return nil, composition.Key{}, false
	}
	rule, roleOK := inner.state.roleSlots[role]
	ruleOrdinal, ruleOK := inner.state.schema.ruleOrdinalOf(rule)
	shape, shapeOK := inner.state.schema.ruleShapeAt(ruleOrdinal)
	factorOrdinal, factorOK := inner.state.schema.factorOrdinalOf(shape.Output)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !factorOK || factorOrdinal >= uint64(len(inner.factors)) {
		return nil, composition.Key{}, false
	}
	factor := inner.factors[factorOrdinal]
	if factor == nil {
		return nil, composition.Key{}, false
	}
	return factor, shape.Output, true
}

func linkBootstrapTransportKey(owner identity.ContentID, target artifactPointMetadata, factor composition.Key) (composition.Key, bool) {
	if !owner.Available() || !target.mount.Available() || !target.artifact.Available() || !target.reusable.Available() || !factor.Available() {
		return composition.Key{}, false
	}
	encoded := []byte("analysis/engine/link-bootstrap-factor-transfer/v1")
	encoded = append(encoded, owner[:]...)
	encoded = append(encoded, target.mount[:]...)
	encoded = append(encoded, target.artifact[:]...)
	encoded = append(encoded, target.reusable[:]...)
	encoded = append(encoded, factor.ID[:]...)
	for shift := uint(56); ; shift -= 8 {
		encoded = append(encoded, byte(factor.Version>>shift))
		if shift == 0 {
			break
		}
	}
	return artifactReceiptKey(identity.ContentID(sha256.Sum256(encoded)), artifactEdgeSourceVersion)
}

func mountedArtifactID(domain string, mount, artifact, id identity.ContentID) identity.ContentID {
	if !mount.Available() || !artifact.Available() || !id.Available() {
		return identity.ContentID{}
	}
	encoded := make([]byte, 0, len(domain)+1+len(mount)+len(artifact)+len(id))
	encoded = append(encoded, domain...)
	encoded = append(encoded, 0)
	encoded = append(encoded, mount[:]...)
	encoded = append(encoded, artifact[:]...)
	encoded = append(encoded, id[:]...)
	return identity.ContentID(sha256.Sum256(encoded))
}

func linkBootstrapPointSemanticID(owner, point identity.ContentID) identity.ContentID {
	return mountedArtifactID("analysis/engine/link-bootstrap-point/v1", owner, owner, point)
}

func snapshotMountedArtifactReceipts(mounts []MountedArtifactReceipt, schemaID identity.ContentID, bootstrap LinkBootstrapWitness, bindingState *schemaBindingState) (*artifactReceiptTopology, ReceiptAssemblyFailure) {
	if len(mounts) == 0 || !schemaID.Available() || !bootstrap.Available() || bindingState == nil {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	bootstrapPoint, pointOK := bootstrap.Point()
	if !pointOK {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	occurrences := make(map[identity.ContentID]struct{}, bootstrap.OccurrenceCount())
	for index := 0; index < bootstrap.OccurrenceCount(); index++ {
		id, idOK := bootstrap.OccurrenceAt(index)
		if !idOK {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		occurrences[id] = struct{}{}
	}
	roles := make(map[identity.ContentID]RuleSlotCapability, len(occurrences))
	for id := range occurrences {
		capability, capabilityOK := bootstrap.capabilityFor(id)
		if !capabilityOK || !capability.link() || capability.state != bindingState || capability.authority != bindingState.authority {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		roles[id] = capability
	}
	transports := make([]linkBootstrapTransport, bootstrap.transportCapabilityCount())
	seenTransportCapabilities := make(map[RuleSlotCapability]struct{}, len(transports))
	seenTransportFactors := make(map[composition.Key]struct{}, len(transports))
	if len(transports) != 0 && len(transports) != 2 {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransportPair(bindingState)
	if (len(transports) == 0 && transportsAuthorized) || (len(transports) != 0 && !transportsAuthorized) {
		return nil, ReceiptAssemblyFailureSnapshotBootstrap
	}
	for index := range transports {
		capability, capabilityOK := bootstrap.transportCapabilityAt(index)
		factor, factorOK := linkTransportFactorSemantic(bindingState, capability)
		if !capabilityOK || !factorOK || capability != authorizedTransports[index] {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportCapabilities[capability]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		if _, duplicate := seenTransportFactors[factor]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotBootstrap
		}
		seenTransportCapabilities[capability] = struct{}{}
		seenTransportFactors[factor] = struct{}{}
		transports[index] = linkBootstrapTransport{capability: capability, factor: factor}
	}
	result := &artifactReceiptTopology{pointMeta: make(map[identity.ContentID]artifactPointMetadata), sites: make(map[identity.ContentID]equation.Site), mounted: make(map[artifactMountedPoint]equation.Site), mountedRef: make(map[artifactMountedPoint]equation.PointRef), bodies: make(map[artifactMountedBody]artifactBodyTransport), ruleSet: make(map[artifactMountedRule]artifactRuleInput), callStages: make(map[artifactMountedRuleOccurrence]artifactNativeCallStage), pointRef: make(map[identity.ContentID]equation.PointRef), bootstrap: &linkBootstrapReceipt{owner: bootstrap.OwnerID(), point: bootstrapPoint, occurrences: occurrences, roles: roles, claims: make(map[identity.ContentID]RuleSlotCapability), transports: transports}}
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.receipt == nil || !mount.receipt.sealed || !mount.receipt.template.Available() || !mount.mountID.Available() || mount.receipt.template.schema != schemaID {
			return nil, ReceiptAssemblyFailureSnapshotMount
		}
		template := mount.receipt.template
		initialCount := 0
		for _, point := range template.points {
			if point.Initial {
				initialCount++
			}
		}
		if initialCount != 1 {
			return nil, ReceiptAssemblyFailureSnapshotTopologyPoint
		}
		if _, duplicate := seenMounts[mount.mountID]; duplicate {
			return nil, ReceiptAssemblyFailureSnapshotMount
		}
		seenMounts[mount.mountID] = struct{}{}
		for _, rule := range template.rules {
			capability, capabilityOK := mount.receipt.capability(rule.Role)
			if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
				return nil, ReceiptAssemblyFailureSnapshotNamespace
			}
		}
		for _, transfer := range template.local {
			for _, role := range transfer.Factors {
				capability, capabilityOK := mount.receipt.capability(role)
				if !capabilityOK || capability.state != bindingState || capability.authority != bindingState.authority {
					return nil, ReceiptAssemblyFailureSnapshotNamespace
				}
			}
		}
		if !appendMountedArtifactReceipt(result, mount.mountID, mount.receipt) {
			return nil, ReceiptAssemblyFailureSnapshotNamespace
		}
	}
	return result, ReceiptAssemblyFailureNone
}

func linkTransportFactorSemantic(state *schemaBindingState, capability RuleSlotCapability) (composition.Key, bool) {
	if state == nil || state.schema == nil || state.phase != schemaBindingSealed || !capability.link() || capability.state != state || capability.authority != state.authority {
		return composition.Key{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	rule, roleOK := state.roleSlots[capability]
	ruleOrdinal, ruleOK := state.schema.ruleOrdinalOf(rule)
	shape, shapeOK := state.schema.ruleShapeAt(ruleOrdinal)
	if !roleOK || !ruleOK || !shapeOK || shape.OutputKind != composition.FactorOutput || !shape.Output.Available() {
		return composition.Key{}, false
	}
	return shape.Output, true
}

// appendMountedArtifactReceipt admits one already-sealed scalar template into
// the shared mounted planes. Scalar relations were closed once by
// NewArtifactScalarTemplate; this pass only resolves Link roles, substitutes
// mount-qualified IDs, and checks that those substitutions stay in the mount.
func appendMountedArtifactReceipt(rows *artifactReceiptTopology, mount identity.ContentID, receipt *ArtifactScalarReceipt) bool {
	if rows == nil || rows.pointMeta == nil || rows.bodies == nil || rows.ruleSet == nil || rows.callStages == nil || receipt == nil || !receipt.sealed || !receipt.template.Available() || !mount.Available() {
		return false
	}
	template := receipt.template
	points := make(map[identity.ContentID]identity.ContentID, len(template.points))
	var initial identity.ContentID
	for _, point := range template.points {
		mounted := mountedArtifactID("analysis/engine/artifact-point/v1", mount, template.artifact, point.ID)
		if !mounted.Available() {
			return false
		}
		if _, duplicate := points[point.ID]; duplicate {
			return false
		}
		if _, duplicate := rows.pointMeta[mounted]; duplicate {
			return false
		}
		points[point.ID] = mounted
		rows.points = append(rows.points, mounted)
		rows.pointMeta[mounted] = artifactPointMetadata{mount: mount, artifact: template.artifact, reusable: point.ID, decisions: point.Decisions, initial: point.Initial}
		if point.Initial {
			if initial.Available() {
				return false
			}
			initial = mounted
		}
	}
	if !initial.Available() {
		return false
	}

	regions := make(map[identity.ContentID]identity.ContentID, len(template.regions))
	seenRegionIDs := make(map[identity.ContentID]struct{}, len(rows.regions)+len(template.regions))
	for _, prior := range rows.regions {
		seenRegionIDs[prior.id] = struct{}{}
	}
	regionOffset := len(rows.regions)
	for _, region := range template.regions {
		mounted := mountedArtifactID("analysis/engine/artifact-wto-region/v1", mount, template.artifact, region.ID)
		if !mounted.Available() {
			return false
		}
		if _, duplicate := regions[region.ID]; duplicate {
			return false
		}
		if _, duplicate := seenRegionIDs[mounted]; duplicate {
			return false
		}
		regions[region.ID] = mounted
		seenRegionIDs[mounted] = struct{}{}
		rows.regions = append(rows.regions, artifactWTORegionRow{mount: mount, artifact: template.artifact, reusable: region.ID, id: mounted, cyclic: region.Cyclic})
	}
	for index, region := range template.regions {
		row := &rows.regions[regionOffset+index]
		head, headOK := points[region.Head]
		if !headOK {
			return false
		}
		row.head = head
		if region.Parent.Available() {
			parent, parentOK := regions[region.Parent]
			if !parentOK {
				return false
			}
			row.parent = parent
		}
		row.members = make([]identity.ContentID, len(region.Members))
		for memberIndex, member := range region.Members {
			mounted, memberOK := points[member]
			if !memberOK {
				return false
			}
			row.members[memberIndex] = mounted
		}
	}

	routes := make(map[identity.ContentID]artifactEnvironmentRow, len(template.edges))
	seenEdgeIDs := make(map[identity.ContentID]struct{}, len(rows.edges)+len(template.edges)+len(template.local))
	for _, prior := range rows.edges {
		seenEdgeIDs[prior.id] = struct{}{}
	}
	for _, edge := range template.edges {
		mounted := mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, template.artifact, edge.ID)
		from, fromOK := points[edge.From]
		to, toOK := points[edge.To]
		if !mounted.Available() || !fromOK || !toOK {
			return false
		}
		if _, duplicate := seenEdgeIDs[mounted]; duplicate {
			return false
		}
		row := artifactEnvironmentRow{mount: mount, artifact: template.artifact, reusable: edge.ID, id: mounted, from: from, to: to, route: edge.Route, guard: edge.Guard, decision: edge.Decision, guarded: edge.Guarded, truth: edge.Truth, component: edge.Component, mu: edge.Mu, reset: edge.Reset, hasReset: edge.HasReset, resets: edge.Resets, arm: edge.Arm, transportOnly: edge.From == edge.To && !edge.Component.Available() && !edge.HasReset && !edge.Mu.Available()}
		rows.edges = append(rows.edges, row)
		seenEdgeIDs[mounted] = struct{}{}
		if edge.Route.Available() {
			if _, duplicate := routes[edge.Route]; !duplicate {
				routes[edge.Route] = row
			}
		}
	}
	for _, edge := range template.local {
		capabilities := make([]RuleSlotCapability, len(edge.Factors))
		for index, role := range edge.Factors {
			capability, capabilityOK := receipt.capability(role)
			if !capabilityOK {
				return false
			}
			capabilities[index] = capability
		}
		row := artifactEnvironmentRow{mount: mount, artifact: template.artifact, reusable: edge.ID, from: points[edge.From], to: points[edge.To], id: mountedArtifactID("analysis/engine/artifact-environment-edge/v1", mount, template.artifact, edge.ID), transportOnly: edge.Full, local: true, full: edge.Full, factorRoles: capabilities}
		if !row.id.Available() || !row.from.Available() || !row.to.Available() {
			return false
		}
		if _, duplicate := seenEdgeIDs[row.id]; duplicate {
			return false
		}
		rows.edges = append(rows.edges, row)
		seenEdgeIDs[row.id] = struct{}{}
	}

	for _, event := range template.events {
		row := artifactWTOEventRow{mount: mount, artifact: template.artifact, kind: event.Kind}
		if event.Region.Available() {
			region, regionOK := regions[event.Region]
			if !regionOK {
				return false
			}
			row.region = region
		}
		if event.Point.Available() {
			point, pointOK := points[event.Point]
			if !pointOK {
				return false
			}
			row.point = point
		}
		rows.events = append(rows.events, row)
	}

	for _, rule := range template.rules {
		capability, capabilityOK := receipt.capability(rule.Role)
		mountedPoint, pointOK := points[rule.Point]
		if !capabilityOK || !rule.Stage.valid() || !pointOK {
			return false
		}
		input := identity.ContentID{}
		if rule.Input.Available() {
			var inputOK bool
			input, inputOK = points[rule.Input]
			if !inputOK {
				return false
			}
		}
		key := artifactMountedRule{role: capability, mount: mount, point: rule.Point, occurrence: rule.ID}
		if _, duplicate := rows.ruleSet[key]; duplicate {
			return false
		}
		bound := artifactRuleInput{point: rule.Input, mountedPoint: mountedPoint, mountedInput: input, stage: rule.Stage}
		if rule.Route.Available() {
			predecessor, predecessorOK := routes[rule.Route]
			if !predecessorOK || predecessor.from != input {
				return false
			}
			bound.predecessor, bound.routed = predecessor, true
		}
		rows.ruleSet[key] = bound
		if rule.Stage.nativeCall() {
			callKey := artifactMountedRuleOccurrence{role: capability, mount: mount, occurrence: rule.ID}
			if _, duplicate := rows.callStages[callKey]; duplicate {
				return false
			}
			rows.callStages[callKey] = artifactNativeCallStage{stage: rule.Stage, point: rule.Point, input: rule.Input, mountedPoint: mountedPoint, mountedInput: input}
		}
	}
	for _, body := range template.bodies {
		key := artifactMountedBody{mount: mount, body: body.ID}
		if _, duplicate := rows.bodies[key]; duplicate {
			return false
		}
		// ArtifactScalarReceipt owns these slices and seals them before this
		// pass. Body transports intentionally retain reusable point IDs; the
		// mounted point inverse resolves them later.
		rows.bodies[key] = artifactBodyTransport{entry: body.Entry, exits: body.Exits}
	}
	seenFunctionIDs := make(map[identity.ContentID]struct{}, len(rows.functions)+len(template.functions))
	for _, prior := range rows.functions {
		seenFunctionIDs[prior.id] = struct{}{}
	}
	for _, function := range template.functions {
		mounted := mountedArtifactID("analysis/engine/artifact-function-interface/v1", mount, template.artifact, function.ID)
		if !mounted.Available() {
			return false
		}
		if _, duplicate := seenFunctionIDs[mounted]; duplicate {
			return false
		}
		if _, bodyOK := rows.bodies[artifactMountedBody{mount: mount, body: function.Body}]; !bodyOK {
			return false
		}
		row := artifactMountedFunction{
			id: mounted, mount: mount, artifact: template.artifact, reusable: function.ID,
			body: function.Body, bodyContext: function.BodyContext, entry: function.Entry, callFormal: function.CallFormal,
			formals: function.Formals, vararg: function.Vararg, hasVararg: function.HasVararg,
			captures: function.Captures, outcomes: function.Outcomes,
		}
		if !validArtifactMountedFunction(row, rows.bodies) {
			return false
		}
		rows.functions = append(rows.functions, row)
		seenFunctionIDs[mounted] = struct{}{}
	}
	rows.mounts = append(rows.mounts, artifactMountReceipt{mount: mount, artifact: template.artifact, program: template.program, initial: initial})
	return true
}

func validArtifactMountedFunction(row artifactMountedFunction, bodies map[artifactMountedBody]artifactBodyTransport) bool {
	if !row.id.Available() || !row.mount.Available() || !row.artifact.Available() || !row.reusable.Available() ||
		!row.body.Available() || !row.bodyContext.Available() || !row.entry.Available() || !row.callFormal.Available() ||
		row.id != mountedArtifactID("analysis/engine/artifact-function-interface/v1", row.mount, row.artifact, row.reusable) ||
		row.hasVararg != row.vararg.ID.Available() || row.hasVararg != row.vararg.Cell.Available() || len(row.outcomes) == 0 {
		return false
	}
	if _, bodyOK := bodies[artifactMountedBody{mount: row.mount, body: row.body}]; !bodyOK {
		return false
	}
	seenFormals := make(map[identity.ContentID]struct{}, len(row.formals))
	for index, port := range row.formals {
		if !port.ID.Available() || !port.Cell.Available() || !port.Storage.Available() || uint64(port.Position) != uint64(index) {
			return false
		}
		if _, duplicate := seenFormals[port.ID]; duplicate {
			return false
		}
		seenFormals[port.ID] = struct{}{}
	}
	seenCaptures := make(map[identity.ContentID]struct{}, len(row.captures))
	for index, capture := range row.captures {
		if !capture.ID.Available() || !capture.Inner.Available() || !capture.Outer.Available() || capture.Inner == capture.Outer ||
			capture.InnerBody != row.body || capture.InnerBody == capture.OuterBody || uint64(capture.Position) != uint64(index) {
			return false
		}
		if _, outerOK := bodies[artifactMountedBody{mount: row.mount, body: capture.OuterBody}]; !outerOK {
			return false
		}
		if _, duplicate := seenCaptures[capture.ID]; duplicate {
			return false
		}
		seenCaptures[capture.ID] = struct{}{}
	}
	seenOutcomes := make(map[identity.ContentID]struct{}, len(row.outcomes))
	for _, outcome := range row.outcomes {
		if !outcome.Available() {
			return false
		}
		if _, duplicate := seenOutcomes[outcome]; duplicate {
			return false
		}
		seenOutcomes[outcome] = struct{}{}
	}
	return true
}

// sealArtifactFunctionDirectory detaches only the mount-qualified formal
// interfaces required after expanded artifact rows are released. Nested
// slices are copied so the compact directory cannot alias a construction
// snapshot even inside this package.
func sealArtifactFunctionDirectory(rows *artifactReceiptTopology) ([]artifactMountedFunction, bool) {
	if rows == nil || rows.bodies == nil {
		return nil, rows == nil
	}
	result := make([]artifactMountedFunction, len(rows.functions))
	seen := make(map[identity.ContentID]struct{}, len(rows.functions))
	for index, row := range rows.functions {
		if !validArtifactMountedFunction(row, rows.bodies) {
			return nil, false
		}
		if _, duplicate := seen[row.id]; duplicate {
			return nil, false
		}
		seen[row.id] = struct{}{}
		row.formals = append([]ArtifactScalarFormalPort(nil), row.formals...)
		row.captures = append([]ArtifactScalarCapturePort(nil), row.captures...)
		row.outcomes = append([]identity.ContentID(nil), row.outcomes...)
		result[index] = row
	}
	return result, true
}

// sealNativeCallStageDirectory retains only the compact native-stage inverse
// needed after the expanded artifact snapshot is released. Every entry must
// already have an attached semantic member under the exact
// role+mount+point+occurrence identity.
func sealNativeCallStageDirectory(rows *artifactReceiptTopology, directory *semanticDirectory) (map[artifactMountedRuleOccurrence]artifactNativeCallStage, bool) {
	if rows == nil {
		return nil, true
	}
	if directory == nil || rows.callStages == nil {
		return nil, false
	}
	result := make(map[artifactMountedRuleOccurrence]artifactNativeCallStage, len(rows.callStages))
	for key, stage := range rows.callStages {
		rule, found := rows.ruleSet[artifactMountedRule{role: key.role, mount: key.mount, point: stage.point, occurrence: key.occurrence}]
		memberID := mountedRuleMemberID(key.role, key.mount, stage.point, key.occurrence)
		if !found || !stage.stage.nativeCall() || rule.stage != stage.stage || rule.point != stage.input || rule.mountedPoint != stage.mountedPoint || rule.mountedInput != stage.mountedInput || !memberID.Available() {
			return nil, false
		}
		if _, attached := directory.member(memberID); !attached {
			return nil, false
		}
		if _, duplicate := result[key]; duplicate {
			return nil, false
		}
		result[key] = stage
	}
	return result, true
}

// validPayload checks only mount substitution and publication ownership. The
// scalar artifact relations were admitted once by NewArtifactScalarTemplate;
// repeating their WTO/edge proofs here only re-proves immutable input.
func (rows *artifactReceiptTopology) validPayload(topology *BindingTopology) bool {
	if rows == nil || len(rows.mounts) == 0 || len(rows.points) == 0 || len(rows.events) == 0 || len(rows.pointMeta) != len(rows.points) || topology != nil && len(rows.pointRef) != len(rows.points) {
		return false
	}
	if rows.bootstrap == nil || !rows.bootstrap.owner.Available() || !rows.bootstrap.point.Known || !rows.bootstrap.point.PointID.Available() || len(rows.bootstrap.transports) != 0 && len(rows.bootstrap.transports) != 2 {
		return false
	}
	if topology != nil {
		authorizedTransports, transportsAuthorized := sealedLinkBootstrapTransportPair(topology.state)
		if (len(rows.bootstrap.transports) == 0 && transportsAuthorized) || (len(rows.bootstrap.transports) != 0 && !transportsAuthorized) {
			return false
		}
		seenCapabilities := make(map[RuleSlotCapability]struct{}, len(rows.bootstrap.transports))
		seenFactors := make(map[composition.Key]struct{}, len(rows.bootstrap.transports))
		for index, transport := range rows.bootstrap.transports {
			factor, factorOK := linkTransportFactorSemantic(topology.state, transport.capability)
			if !factorOK || factor != transport.factor || transport.capability.authority != topology.authority || transport.capability != authorizedTransports[index] {
				return false
			}
			if _, duplicate := seenCapabilities[transport.capability]; duplicate {
				return false
			}
			if _, duplicate := seenFactors[transport.factor]; duplicate {
				return false
			}
			seenCapabilities[transport.capability] = struct{}{}
			seenFactors[transport.factor] = struct{}{}
		}
	}
	mounts := make(map[identity.ContentID]artifactMountReceipt, len(rows.mounts))
	for _, mount := range rows.mounts {
		metadata, initialOK := rows.pointMeta[mount.initial]
		if !mount.mount.Available() || !mount.artifact.Available() || !mount.program.Available() || !mount.initial.Available() || !initialOK || !metadata.initial || metadata.mount != mount.mount || metadata.artifact != mount.artifact {
			return false
		}
		if _, duplicate := mounts[mount.mount]; duplicate {
			return false
		}
		mounts[mount.mount] = mount
	}
	points := make(map[identity.ContentID]struct{}, len(rows.points))
	pointMounts := make(map[identity.ContentID]identity.ContentID, len(rows.points))
	sourcePoints := make(map[artifactMountedPoint]struct{}, len(rows.points))
	initialCounts := make(map[identity.ContentID]int, len(rows.mounts))
	for _, id := range rows.points {
		if !id.Available() {
			return false
		}
		if _, duplicate := points[id]; duplicate {
			return false
		}
		points[id] = struct{}{}
		metadata, metadataOK := rows.pointMeta[id]
		mount, mountOK := mounts[metadata.mount]
		if !metadataOK || !metadata.reusable.Available() || !mountOK || mount.artifact != metadata.artifact {
			return false
		}
		if metadata.initial {
			if id != mount.initial {
				return false
			}
			initialCounts[metadata.mount]++
		}
		pointMounts[id] = metadata.mount
		sourcePoints[artifactMountedPoint{mount: metadata.mount, reusable: metadata.reusable}] = struct{}{}
	}
	for mount := range mounts {
		if initialCounts[mount] != 1 {
			return false
		}
	}
	regions := make(map[identity.ContentID]struct{}, len(rows.regions))
	regionMounts := make(map[identity.ContentID]identity.ContentID, len(rows.regions))
	for _, region := range rows.regions {
		mount, mountOK := mounts[region.mount]
		if !region.id.Available() || !region.head.Available() || !mountOK || mount.artifact != region.artifact || !region.reusable.Available() {
			return false
		}
		if _, duplicate := regions[region.id]; duplicate {
			return false
		}
		headMount, headMountOK := pointMounts[region.head]
		if !headMountOK || headMount != region.mount {
			return false
		}
		for _, member := range region.members {
			memberMount, pointOK := pointMounts[member]
			if !pointOK || memberMount != region.mount {
				return false
			}
		}
		regions[region.id] = struct{}{}
		regionMounts[region.id] = region.mount
	}
	for _, region := range rows.regions {
		if region.parent.Available() {
			parentMount, exists := regionMounts[region.parent]
			if !exists || parentMount != region.mount || region.parent == region.id {
				return false
			}
		}
	}
	edges := make(map[identity.ContentID]struct{}, len(rows.edges))
	for _, edge := range rows.edges {
		mount, mountOK := mounts[edge.mount]
		if !mountOK || mount.artifact != edge.artifact || !edge.reusable.Available() || !edge.id.Available() {
			return false
		}
		fromMount, fromOK := pointMounts[edge.from]
		if !fromOK || fromMount != edge.mount {
			return false
		}
		toMount, toOK := pointMounts[edge.to]
		if !toOK || toMount != edge.mount {
			return false
		}
		if _, duplicate := edges[edge.id]; duplicate {
			return false
		}
		edges[edge.id] = struct{}{}
	}
	for _, event := range rows.events {
		mount, mountOK := mounts[event.mount]
		if !mountOK || mount.artifact != event.artifact {
			return false
		}
		if event.region.Available() {
			regionMount, regionOK := regionMounts[event.region]
			if !regionOK || regionMount != event.mount {
				return false
			}
		}
		if event.point.Available() {
			pointMount, pointOK := pointMounts[event.point]
			if !pointOK || pointMount != event.mount {
				return false
			}
		}
	}
	for key, input := range rows.ruleSet {
		mount, mountOK := mounts[key.mount]
		if !mountOK || !key.role.mounted() || !input.stage.valid() || topology != nil && (key.role.state != topology.state || key.role.authority != topology.authority) || !key.point.Available() || !key.occurrence.Available() {
			return false
		}
		if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: key.point}]; !pointOK || !input.mountedPoint.Available() || pointMounts[input.mountedPoint] != key.mount {
			return false
		}
		if input.point.Available() {
			if _, inputOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: input.point}]; !inputOK || !input.mountedInput.Available() || pointMounts[input.mountedInput] != key.mount {
				return false
			}
		} else if input.mountedInput.Available() {
			return false
		}
		if input.routed {
			if input.predecessor.mount != key.mount || input.predecessor.artifact != mount.artifact {
				return false
			}
			fromMount, fromOK := pointMounts[input.predecessor.from]
			toMount, toOK := pointMounts[input.predecessor.to]
			if !fromOK || !toOK || fromMount != key.mount || toMount != key.mount {
				return false
			}
		}
	}
	nativeCount := 0
	for key, input := range rows.ruleSet {
		if !input.stage.nativeCall() {
			continue
		}
		nativeCount++
		callKey := artifactMountedRuleOccurrence{role: key.role, mount: key.mount, occurrence: key.occurrence}
		stage, found := rows.callStages[callKey]
		if !found || stage.stage != input.stage || stage.point != key.point || stage.input != input.point || stage.mountedPoint != input.mountedPoint || stage.mountedInput != input.mountedInput {
			return false
		}
	}
	if nativeCount != len(rows.callStages) {
		return false
	}
	for key, transport := range rows.bodies {
		if _, mountOK := mounts[key.mount]; !mountOK || !key.body.Available() || len(transport.entry) == 0 || len(transport.exits) == 0 {
			return false
		}
		for _, reusable := range transport.entry {
			if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: reusable}]; !pointOK {
				return false
			}
		}
		for _, reusable := range transport.exits {
			if _, pointOK := sourcePoints[artifactMountedPoint{mount: key.mount, reusable: reusable}]; !pointOK {
				return false
			}
		}
	}
	seenFunctionIDs := make(map[identity.ContentID]struct{}, len(rows.functions))
	for _, function := range rows.functions {
		if !validArtifactMountedFunction(function, rows.bodies) {
			return false
		}
		if _, mountOK := mounts[function.mount]; !mountOK {
			return false
		}
		if _, duplicate := seenFunctionIDs[function.id]; duplicate {
			return false
		}
		seenFunctionIDs[function.id] = struct{}{}
	}
	if topology != nil {
		expectedPoints := len(rows.points)
		if rows.bootstrap != nil {
			expectedPoints++
		}
		expectedEnvironmentEdges, expectedFactorEdges := 0, 0
		for _, edge := range rows.edges {
			if edge.local && !edge.full {
				expectedFactorEdges += len(edge.factorRoles)
			} else {
				expectedEnvironmentEdges++
			}
		}
		expectedFactorEdges += len(rows.mounts) * len(rows.bootstrap.transports)
		if topology.plan == nil || len(topology.plan.spec.Points) != expectedPoints || len(topology.plan.spec.EnvironmentEdges) != expectedEnvironmentEdges || len(topology.plan.spec.FactorEdges) != expectedFactorEdges {
			return false
		}
		for id, ref := range rows.pointRef {
			if !id.Available() || ref == 0 {
				return false
			}
			if _, found := topology.directory.point(id); !found {
				return false
			}
		}
		if rows.bootstrap != nil && (rows.bootstrap.ref == 0 || !rows.bootstrap.semantic.Available()) {
			return false
		}
		if rows.bootstrap != nil {
			if _, found := topology.directory.point(rows.bootstrap.semantic); !found {
				return false
			}
		}
	}
	return true
}

// valid keeps the open construction gates explicit. A non-nil topology is a
// published-owner query and must use the constant-time sealed owner fence.
func (rows *artifactReceiptTopology) valid(topology *BindingTopology) bool {
	if topology == nil {
		return rows != nil && rows.sealed == nil
	}
	return rows != nil && rows.sealed == topology && topology.artifact == rows
}

// seal performs the sole complete proof after the equation topology and its
// semantic directory exist. The private receipt planes are immutable after
// this point, so all later access is authenticated by exact owner identity.
func (rows *artifactReceiptTopology) seal(topology *BindingTopology) bool {
	if rows == nil || topology == nil || rows.sealed != nil || topology.artifact != rows || !rows.validPayload(topology) {
		return false
	}
	rows.sealed = topology
	return true
}

func (rows *artifactReceiptTopology) mountedSite(mount, reusable identity.ContentID) (equation.Site, bool) {
	if rows == nil || rows.mounted == nil || !mount.Available() || !reusable.Available() {
		return equation.Site{}, false
	}
	site, ok := rows.mounted[artifactMountedPoint{mount: mount, reusable: reusable}]
	// Mounted rule occurrence admission happens while the source Batch is
	// still open. Site.Available is intentionally a post-seal capability;
	// the caller holds the assembly's source lock and Batch.From authenticates
	// this exact mapped Site in the open phase.
	return site, ok
}

func (rows *artifactReceiptTopology) mountedPoint(mount, reusable identity.ContentID) (equation.Site, equation.PointRef, bool) {
	if rows == nil || rows.mounted == nil || rows.mountedRef == nil || !mount.Available() || !reusable.Available() {
		return equation.Site{}, 0, false
	}
	key := artifactMountedPoint{mount: mount, reusable: reusable}
	site, siteOK := rows.mounted[key]
	ref, refOK := rows.mountedRef[key]
	return site, ref, siteOK && refOK && ref != 0
}

func (rows *artifactReceiptTopology) mountedBody(mount, body identity.ContentID) (artifactBodyTransport, bool) {
	if rows == nil || rows.bodies == nil || !mount.Available() || !body.Available() {
		return artifactBodyTransport{}, false
	}
	value, ok := rows.bodies[artifactMountedBody{mount: mount, body: body}]
	return value, ok && len(value.entry) != 0 && len(value.exits) != 0
}

func (rows *artifactReceiptTopology) mountedRule(role RuleSlotCapability, mount, point, occurrence identity.ContentID) (artifactRuleInput, bool) {
	if rows == nil || rows.ruleSet == nil || !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return artifactRuleInput{}, false
	}
	input, ok := rows.ruleSet[artifactMountedRule{role: role, mount: mount, point: point, occurrence: occurrence}]
	return input, ok
}
