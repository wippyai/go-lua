package equation

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

func validScopedExpr(expr Expr, scope Scope) bool {
	if !expr.Available() || !scope.Available() {
		return false
	}
	for _, decision := range expr.Decisions() {
		if !scope.contains(decision) {
			return false
		}
	}
	return true
}

// TemplateRole maps one canonical fragment role to one presealed base Point. Axes
// membership belongs exclusively to the sealed receipt; a binding with a
// different base mapping is a different fixed-shape binding, never a branch
// inside this port table.
type TemplateRole struct {
	Role composition.Key
	Mode PortMode
	// Reads are the finite named prototype exact-read slots supplied by this
	// import role.  They are canonicalized by PortRead.Role, never declaration
	// position, and remain optional for control-only ports.
	Reads []PortRead
}

// TargetPoint is one symbolic point reference in a Template. Exactly one
// coordinate is present: Local names a pair-owned PointSpec, while TemplateRole names
// a static TemplateRole role. The raw PointRef/Role are builder input only; seal
// resolves them to issued point identities before any graph is compiled.
type TargetPoint struct {
	Local PointRef
	Role  composition.Key
}

// TemplateInput is an ordinary complete boundary input before its symbolic
// endpoints are resolved.  Pre/post conditions and provenance remain part of
// the template so member expansion cannot silently drop them.
type TemplateInput struct {
	Point      TargetPoint
	Provenance composition.Key
	Pre        Expr
	Reindex    Reindex
	Post       Expr
}

// TargetGroup is a finite ordinary Group shape. Expansion resolves its
// Local/TemplateRole point references and emits the normal compiled input form used
// by the sole equation compiler.
type TargetGroup struct {
	Members []RuleRef
	Output  TargetPoint
	Inputs  []TemplateInput
}

// TargetFactorEdge is one structural Factor-local transport in an
// activation fragment.  It is deliberately the same boundary shape as an
// ordinary Input, with symbolic endpoints resolved only while an accepted
// Member is expanded.  The edge has no Rule/Group/Member authority of its
// own; after expansion it is appended to TopologySpec.FactorEdges and follows
// the ordinary graph, demand, WTO, and runtime paths.
type TargetFactorEdge struct {
	// Source is the member-local or named-port endpoint. ExternalSource is
	// an already-issued source Point supplied by the owning target Batch;
	// exactly one of them is present. ExternalSource is not alpha-renamed or
	// copied during Member expansion.
	Source         TargetPoint
	ExternalSource Site
	Target         TargetPoint
	// ExternalTarget is the symmetric already-issued target Point form. The
	// ordinary FactorEdge topology remains the only runtime edge plane; this
	// field merely lets a structural fragment terminate at an existing base
	// point rather than forcing a synthetic export port.
	ExternalTarget Site
	Factor         composition.Key
	Provenance     composition.Key
	Pre            Expr
	Reindex        Reindex
	Post           Expr
}

// Template is a finite, data-only ordinary topology fragment. Its Rules use
// fixed identity placeholders; Local points and rule anchors become pair-owned
// at expansion, while static Ports bind only to already sealed base Points.
// It has no callback, conditional, iterator, or executable authority.
type Template struct {
	Rules       []RuleInstance
	Points      []PointSpec
	Roles       []TemplateRole
	Groups      []TargetGroup
	FactorEdges []TargetFactorEdge
	Summaries   []SummaryMapping
	WeakTargets []WeakTargetMapping
}

// normalizeTemplateFormal reissues one authored template into a dedicated
// formal Batch. Roles become FormalPorts; ordinary local sites, occurrences,
// and operands are reissued in the same transaction. This is the cold
// boundary used later by TemplateBinding; no caller Batch capability is
// retained in the shared plan.
func normalizeTemplateFormal(value Template) (result Template, resultBatch *Batch, resultPorts map[composition.Key]FormalPort, ok bool) {
	stage := "start"
	_ = stage
	base := templateBatch(value)
	if base != nil && !base.Sealed() {
		return Template{}, nil, nil, false
	}
	if base == nil {
		if len(value.Points) != 0 || len(value.Rules) != 0 {
			return Template{}, nil, nil, false
		}
		for _, edge := range value.FactorEdges {
			if edge.ExternalSource.Available() || edge.ExternalTarget.Available() {
				return Template{}, nil, nil, false
			}
		}
	}
	formal := NewBatch()
	stage = "roles"
	ports := make(map[composition.Key]FormalPort, len(value.Roles))
	for _, role := range value.Roles {
		port, ok := formal.AdmitFormalPort(role.Role, role.Mode, role.Reads)
		if !ok {
			return Template{}, nil, nil, false
		}
		ports[role.Role] = port
	}
	baseSiteCount := 0
	if base != nil {
		baseSiteCount = len(base.sites)
	}
	sites := make([]Site, baseSiteCount)
	stage = "sites"
	if base != nil {
		for index, row := range base.sites {
			if row.formal {
				port, ok := ports[row.formalRole]
				if !ok {
					return Template{}, nil, nil, false
				}
				sites[index] = port.Site()
				continue
			}
			site, ok := formal.AdmitSite(row.source, row.scope, row.init, row.disposition)
			if !ok {
				return Template{}, nil, nil, false
			}
			sites[index] = site
		}
	}
	occurrenceCount := 0
	if base != nil {
		occurrenceCount = len(base.occurrences)
	}
	occurrences := make([]Occurrence, occurrenceCount)
	stage = "occurrences"
	if base != nil {
		for index, row := range base.occurrences {
			if row.site == 0 || uint64(row.site) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			var ok bool
			switch row.kind {
			case OccurrenceAt:
				occurrences[index], ok = formal.At(sites[row.site-1])
			case OccurrenceFrom:
				occurrences[index], ok = formal.From(sites[row.site-1], row.entity)
			case OccurrenceRelation:
				occurrences[index], ok = formal.Relation(sites[row.site-1], row.entity)
			}
			if !ok {
				return Template{}, nil, nil, false
			}
		}
	}
	operandCount := 0
	if base != nil {
		operandCount = len(base.operands)
	}
	operands := make([]Operand, operandCount)
	stage = "operands"
	if base != nil {
		for index, row := range base.operands {
			if row.occurrence == 0 || uint64(row.occurrence) > uint64(len(occurrences)) {
				return Template{}, nil, nil, false
			}
			operand, ok := formal.AdmitOperand(occurrences[row.occurrence-1], row.entity)
			if !ok {
				return Template{}, nil, nil, false
			}
			operands[index] = operand
		}
	}
	result = copyTemplate(value)
	for index, point := range result.Points {
		if point.Site.row == 0 || uint64(point.Site.row) > uint64(len(sites)) {
			return Template{}, nil, nil, false
		}
		result.Points[index].Site = sites[point.Site.row-1]
	}
	for index, rule := range result.Rules {
		if rule.Occurrence.row == 0 || uint64(rule.Occurrence.row) > uint64(len(occurrences)) || rule.Operand.row == 0 || uint64(rule.Operand.row) > uint64(len(operands)) {
			return Template{}, nil, nil, false
		}
		result.Rules[index].Occurrence = occurrences[rule.Occurrence.row-1]
		result.Rules[index].Operand = operands[rule.Operand.row-1]
	}
	for index, edge := range result.FactorEdges {
		if edge.ExternalSource.Available() {
			if edge.ExternalSource.row == 0 || uint64(edge.ExternalSource.row) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			result.FactorEdges[index].ExternalSource = sites[edge.ExternalSource.row-1]
		}
		if edge.ExternalTarget.Available() {
			if edge.ExternalTarget.row == 0 || uint64(edge.ExternalTarget.row) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			result.FactorEdges[index].ExternalTarget = sites[edge.ExternalTarget.row-1]
		}
	}
	// The formal owner admits the complete ordinary target grammar into this
	// same open Batch. No engine-side copy may construct a second row graph.
	pointRefs := make(map[Site]PointRef, len(result.Points)+len(ports))
	pointSites := make(map[PointRef]Site, len(result.Points)+len(ports))
	for _, point := range result.Points {
		ref, admitted := formal.AdmitPoint(point.Site)
		if !admitted {
			return Template{}, nil, nil, false
		}
		pointRefs[point.Site] = ref
		pointSites[ref] = point.Site
	}
	// Boundary point ordinals follow the declared role vector. The ports map is
	// a lookup index into that vector and carries no order authority.
	for _, role := range result.Roles {
		port, present := ports[role.Role]
		if !present {
			return Template{}, nil, nil, false
		}
		if _, issued := pointRefs[port.Site()]; issued {
			continue
		}
		ref, admitted := formal.AdmitPoint(port.Site())
		if !admitted {
			return Template{}, nil, nil, false
		}
		pointRefs[port.Site()] = ref
		pointSites[ref] = port.Site()
	}
	for _, row := range result.Rules {
		if !formal.AdmitRule(row) {
			return Template{}, nil, nil, false
		}
	}
	resolvePoint := func(point TargetPoint) (PointRef, bool) {
		if point.Role.Available() {
			port, present := ports[point.Role]
			if !present {
				return 0, false
			}
			ref, present := pointRefs[port.Site()]
			return ref, present && ref != 0
		}
		if point.Local == 0 || uint64(point.Local) > uint64(len(result.Points)) {
			return 0, false
		}
		ref, present := pointRefs[result.Points[point.Local-1].Site]
		return ref, present && ref != 0
	}
	for _, group := range result.Groups {
		output, outputOK := resolvePoint(group.Output)
		if !outputOK {
			return Template{}, nil, nil, false
		}
		bound := BatchGroup{Output: output, Members: append([]RuleRef(nil), group.Members...)}
		for _, input := range group.Inputs {
			source, sourceOK := resolvePoint(input.Point)
			if !sourceOK {
				return Template{}, nil, nil, false
			}
			sourceSite, sourcePresent := pointSites[source]
			targetSite, targetPresent := pointSites[output]
			if !sourcePresent || !targetPresent {
				return Template{}, nil, nil, false
			}
			bound.Inputs = append(bound.Inputs, TargetBoundaryInput(sourceSite, targetSite, input.Provenance, input.Pre, input.Reindex, input.Post))
		}
		if !formal.AdmitGroup(bound) {
			return Template{}, nil, nil, false
		}
	}
	for _, edge := range result.FactorEdges {
		target, targetOK := resolvePoint(edge.Target)
		var source Site
		var sourceOK bool
		if edge.ExternalSource.Available() {
			source, sourceOK = edge.ExternalSource, true
		} else {
			ref, ok := resolvePoint(edge.Source)
			if ok {
				source, sourceOK = pointSites[ref]
				if !sourceOK {
					return Template{}, nil, nil, false
				}
			}
		}
		if !targetOK || !sourceOK {
			return Template{}, nil, nil, false
		}
		targetSite, targetPresent := pointSites[target]
		if !targetPresent {
			return Template{}, nil, nil, false
		}
		input := TargetBoundaryInput(source, targetSite, edge.Provenance, edge.Pre, edge.Reindex, edge.Post)
		if !formal.AdmitFactorEdge(BatchFactorEdge{Target: target, Input: input, Factor: edge.Factor}) {
			return Template{}, nil, nil, false
		}
	}
	for _, summary := range result.Summaries {
		if !formal.AdmitSummary(summary) {
			return Template{}, nil, nil, false
		}
	}
	for _, weak := range result.WeakTargets {
		if !formal.AdmitWeakTarget(weak) {
			return Template{}, nil, nil, false
		}
	}
	if !formal.Seal() {
		return Template{}, nil, nil, false
	}
	return result, formal, ports, true
}

func templateBatch(value Template) *Batch {
	var batch *Batch
	accept := func(candidate *Batch) bool {
		if candidate == nil || !candidate.Sealed() {
			return false
		}
		if batch == nil {
			batch = candidate
		}
		return batch == candidate
	}
	for _, point := range value.Points {
		if !point.Site.Available() || !accept(point.Site.batch) {
			return nil
		}
	}
	for _, rule := range value.Rules {
		if !rule.Occurrence.Available() || !rule.Operand.Available() || !rule.Operand.Occurrence().Same(rule.Occurrence) || !accept(rule.Occurrence.batch) || !accept(rule.Operand.batch) {
			return nil
		}
	}
	for _, edge := range value.FactorEdges {
		if edge.ExternalSource.Available() && !accept(edge.ExternalSource.batch) {
			return nil
		}
		if edge.ExternalTarget.Available() && !accept(edge.ExternalTarget.batch) {
			return nil
		}
	}
	return batch
}

func compatiblePortReads(source *composition.Composition, prototype, caller []PortRead) bool {
	if source == nil || len(prototype) != len(caller) {
		return false
	}
	for index := range prototype {
		if prototype[index].Role != caller[index].Role || !compatiblePortRead(source, prototype[index].Surface, caller[index].Surface) {
			return false
		}
	}
	return true
}

func compatiblePortRead(source *composition.Composition, prototype, caller Surface) bool {
	if source == nil || !prototype.Available() || !caller.Available() || prototype.Form != SurfaceReadExact || caller.Form != SurfaceReadExact ||
		prototype.Mode != TargetModeNone || caller.Mode != TargetModeNone || prototype.Semantic.Available() || caller.Semantic.Available() ||
		prototype.Normalizer.Available() || caller.Normalizer.Available() || prototype.Factor != caller.Factor {
		return false
	}
	_, found := source.FactorIndex(prototype.Factor)
	return found
}

func copyTemplate(value Template) Template {
	result := Template{
		Rules:       make([]RuleInstance, len(value.Rules)),
		Points:      append([]PointSpec(nil), value.Points...),
		Roles:       make([]TemplateRole, len(value.Roles)),
		Groups:      make([]TargetGroup, len(value.Groups)),
		FactorEdges: make([]TargetFactorEdge, len(value.FactorEdges)),
		Summaries:   make([]SummaryMapping, len(value.Summaries)),
		WeakTargets: make([]WeakTargetMapping, len(value.WeakTargets)),
	}
	for index, row := range value.Rules {
		result.Rules[index] = copyInstance(row)
	}
	for index, port := range value.Roles {
		result.Roles[index] = TemplateRole{Role: port.Role, Mode: port.Mode, Reads: append([]PortRead(nil), port.Reads...)}
	}
	for index, group := range value.Groups {
		copied := TargetGroup{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]TemplateInput(nil), group.Inputs...)}
		result.Groups[index] = copied
	}
	for index, edge := range value.FactorEdges {
		result.FactorEdges[index] = TargetFactorEdge{Source: edge.Source, ExternalSource: edge.ExternalSource, Target: edge.Target, ExternalTarget: edge.ExternalTarget, Factor: edge.Factor, Provenance: edge.Provenance, Pre: edge.Pre, Reindex: edge.Reindex, Post: edge.Post}
	}
	for index, summary := range value.Summaries {
		result.Summaries[index] = SummaryMapping{Surface: summary.Surface, Keys: append([]uint64(nil), summary.Keys...)}
	}
	for index, weak := range value.WeakTargets {
		result.WeakTargets[index] = WeakTargetMapping{Surface: weak.Surface, Candidates: append([]Surface(nil), weak.Candidates...)}
	}
	return result
}

// decisionAlpha is the invocation-local renaming of raw template decisions.
// Its one key per (raw decision, binding, Member) is deliberately independent
// of a local Point row: branches in different local points of one invocation
// remain correlated, while branches from distinct Members cannot alias.
type decisionAlpha map[composition.Key]Decision

func (alpha decisionAlpha) bind(decision Decision, local bool) (Decision, bool) {
	if !decision.Available() {
		return Decision{}, false
	}
	if !local {
		return decision, true
	}
	bound, found := alpha[decision.key]
	return bound, found && bound.Available()
}

// boundScope is exactly the canonical decision universe S union alpha(L).
// Site and Point retain source-row/member identity; Scope must not duplicate
// that identity or equal formal local universes cease to compose exactly.
func boundScope(scope, ambient Scope, alpha decisionAlpha) (Scope, bool) {
	if !scope.Available() || !ambient.Available() || alpha == nil {
		return Scope{}, false
	}
	decisions := make([]Decision, 0, len(ambient.row.decisions)+len(scope.row.decisions))
	decisions = append(decisions, ambient.row.decisions...)
	for _, decision := range scope.row.decisions {
		bound, ok := alpha.bind(decision, true)
		if !ok {
			return Scope{}, false
		}
		decisions = append(decisions, bound)
	}
	// NewScope sorts canonically and rejects alpha/ambient collisions.
	return NewScope(decisions...)
}

// boundExpr rebuilds a target-side substitution expression through alpha.
// A decision hash can change its global ROBDD order, so copying nodes would
// retain an invalid ordered DAG; rebuilding ITEs is the canonical operation.
func boundExpr(value Expr, alpha decisionAlpha) (Expr, bool) {
	if !value.Available() {
		return Expr{}, false
	}
	builder := newExprBuilder()
	seen := map[uint32]uint32{0: 0, 1: 1}
	var rewrite func(uint32) (uint32, bool)
	rewrite = func(index uint32) (uint32, bool) {
		if result, found := seen[index]; found {
			return result, true
		}
		if index < 2 || index > uint32(len(value.nodes)+1) {
			return 0, false
		}
		node := value.nodes[index-2]
		decision, ok := alpha.bind(node.decision, true)
		if !ok {
			return 0, false
		}
		low, ok := rewrite(node.low)
		if !ok {
			return 0, false
		}
		high, ok := rewrite(node.high)
		if !ok {
			return 0, false
		}
		test, ok := builder.node(decision, 0, 1)
		if !ok {
			return 0, false
		}
		result, ok := builder.ite(test, high, low)
		if !ok {
			return 0, false
		}
		seen[index] = result
		return result, true
	}
	root, ok := rewrite(value.root)
	if !ok {
		return Expr{}, false
	}
	return builder.freeze(root)
}

// boundReindex rewrites one simultaneous relation at its two endpoints. A
// local endpoint receives the Member alpha; a static TemplateRole endpoint retains
// its base decisions. Substitution expressions belong to the target scope.
func boundReindex(value Reindex, source, target templateResolvedPoint, ambient Scope, alpha decisionAlpha) (Reindex, bool) {
	if !value.Available() || !source.available() || !target.available() || !ambient.Available() || !sameScope(value.source, source.rawScope) || !sameScope(value.target, target.rawScope) {
		return Reindex{}, false
	}
	bySource := make(map[composition.Key]DecisionMap, len(ambient.row.decisions)+len(value.maps))
	ambientSources := make(map[composition.Key]struct{}, len(ambient.row.decisions))
	for _, decision := range ambient.row.decisions {
		// A static external source may be context-free while the selected
		// caller target carries the attachment ambient scope. Such ambient
		// decisions are newly introduced at the target and therefore require
		// no source map. Retain the identity fast path when both endpoints
		// contain the decision. A source-only decision introduced by a TemplateRole's
		// ambient lift is forgotten exactly once; a source-only decision already
		// present in the raw source retains its authored formal mapping below.
		sourceHas, targetHas := source.scope.contains(decision), target.scope.contains(decision)
		if sourceHas && !targetHas {
			if !source.rawScope.contains(decision) {
				bySource[decision.Key()] = Forget(decision)
				ambientSources[decision.Key()] = struct{}{}
			}
			continue
		}
		if sourceHas && targetHas {
			bySource[decision.Key()] = Identity(decision)
			ambientSources[decision.Key()] = struct{}{}
		}
	}
	for _, mapping := range value.maps {
		boundSource, ok := alpha.bind(mapping.Source, source.local)
		if !ok {
			return Reindex{}, false
		}
		if _, ambientSource := ambientSources[boundSource.Key()]; ambientSource {
			continue
		}
		var bound DecisionMap
		switch mapping.Disposition {
		case DecisionIdentity, DecisionRename:
			boundTarget, ok := alpha.bind(mapping.Target, target.local)
			if !ok {
				return Reindex{}, false
			}
			if boundSource == boundTarget {
				bound = Identity(boundSource)
			} else {
				bound = Rename(boundSource, boundTarget)
			}
		case DecisionForget:
			bound = Forget(boundSource)
		case DecisionSubstitute:
			expr := mapping.Expr
			if target.local {
				expr, ok = boundExpr(expr, alpha)
				if !ok {
					return Reindex{}, false
				}
			}
			bound = Substitute(boundSource, expr)
		default:
			return Reindex{}, false
		}
		bySource[boundSource.Key()] = bound
	}
	maps := make([]DecisionMap, 0, source.scope.Count())
	for index := 0; index < source.scope.Count(); index++ {
		decision, ok := source.scope.At(index)
		if !ok {
			return Reindex{}, false
		}
		mapping, found := bySource[decision.Key()]
		if !found {
			return Reindex{}, false
		}
		maps = append(maps, mapping)
	}
	return NewReindex(source.scope, target.scope, maps)
}

type templateResolvedPoint struct {
	ref      PointRef
	site     Site
	scope    Scope
	rawScope Scope
	local    bool
	open     bool
}

func (point templateResolvedPoint) available() bool {
	return point.ref != 0 && (point.site.Available() || point.open && point.site.batch != nil && point.site.dynamic == nil) && point.scope.Available() && point.rawScope.Available()
}
