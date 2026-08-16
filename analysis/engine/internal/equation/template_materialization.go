package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// TemplateMaterialization is the sealed receipt of one target-owned lowering
// transaction. The ordinary Batch and Inputs below are the only materialized
// semantic rows; this receipt only authenticates their exact cold Composition,
// TemplateBinding, and formal-row correspondence.
type TemplateMaterialization struct{ data *templateMaterializationData }

// MaterializationOrigin authenticates the trigger row and locator that caused
// a receipt to be admitted.  Selection consumes this receipt-origin metadata;
// it never reopens a template or lowers a selected member.
type MaterializationOrigin struct {
	Family         composition.Key
	Application    composition.Key
	Target         composition.Key
	Endpoint       composition.Key
	TriggerOrdinal int
}

type templateMaterializationAuthority struct{ marker byte }

type templateMaterializationData struct {
	source    *composition.Composition
	binding   *templateBindingData
	batch     *Batch
	key       composition.Key
	authority *templateMaterializationAuthority
	sites     []Site // formal Batch Site row -> exact target-owned Site
	inputs    []Input
	origin    MaterializationOrigin
	hasOrigin bool
}

// MaterializeTemplateBoundary consumes one exact TemplateBinding and the
// complete Site denominator of its formal Batch. It reissues those Sites and
// the supplied ordinary Inputs into one new sealed target Batch. No cross-Batch
// Input or unresolved formal Site survives the transaction.
//
// The current formal Scope is empty. All concrete ports must therefore share
// one exact ambient Scope, matching the existing activation attachment law.
// Local formal decisions are alpha-renamed beneath that ambient Scope; the
// existing boundScope/boundExpr/boundReindex functions remain the sole lowering
// implementation.
func MaterializeTemplateBoundary(source *composition.Composition, binding TemplateBinding, sites []Site, inputs []Input) (TemplateMaterialization, bool) {
	if source == nil || !source.ID().Available() || !binding.Available() {
		return TemplateMaterialization{}, false
	}
	data := binding.data
	if data == nil || len(sites) != len(data.formals.sites) || len(sites) == 0 {
		return TemplateMaterialization{}, false
	}
	if !validateTemplateBindingReads(source, binding) {
		return TemplateMaterialization{}, false
	}
	formalSites, sitesOK := exactFormalSiteDenominator(data.formals, sites)
	if !sitesOK {
		return TemplateMaterialization{}, false
	}
	ambient, ambientOK := templateBindingAmbient(binding)
	if !ambientOK {
		return TemplateMaterialization{}, false
	}
	alpha, alphaOK := templateMaterializationAlpha(binding)
	if !alphaOK {
		return TemplateMaterialization{}, false
	}

	target := NewBatch()
	targetSites := make([]Site, len(formalSites))
	targetScopes := make([]Scope, len(formalSites))
	actualSites := make(map[composition.Key]Site, len(data.rows))
	for index, formalSite := range formalSites {
		formalRow, rowOK := data.formals.sealedSite(formalSite.row)
		if !rowOK {
			return TemplateMaterialization{}, false
		}
		if formalRow.formal {
			bindingRow, bound := bindingRowForPort(binding, FormalPort{batch: data.formals, row: formalSite.row})
			if !bound || !sameScope(bindingRow.actual.Scope(), ambient) {
				return TemplateMaterialization{}, false
			}
			if existing, found := actualSites[bindingRow.actual.Key()]; found {
				targetSites[index] = existing
				targetScopes[index] = ambient
				continue
			}
			init, disposition, initialized := bindingRow.actual.Init()
			if !initialized {
				return TemplateMaterialization{}, false
			}
			materialized, admitted := target.AdmitSite(bindingRow.actual.Source(), bindingRow.actual.Scope(), init, disposition)
			if !admitted {
				return TemplateMaterialization{}, false
			}
			actualSites[bindingRow.actual.Key()] = materialized
			targetSites[index] = materialized
			targetScopes[index] = ambient
			continue
		}
		scope, scoped := boundScope(formalSite.Scope(), ambient, alpha)
		init, disposition, initialized := formalSite.Init()
		if !scoped || !initialized {
			return TemplateMaterialization{}, false
		}
		init, initialized = boundExpr(init, alpha)
		if !initialized {
			return TemplateMaterialization{}, false
		}
		sourceKey, keyed := identityKey("analysis/engine/equation/template-materialized-site-source", func(writer *canonical.DigestWriter) bool {
			return writeSite(writer, formalSite) && writeKey(writer, binding.Key())
		})
		if !keyed {
			return TemplateMaterialization{}, false
		}
		materialized, admitted := target.AdmitSite(sourceKey, scope, init, disposition)
		if !admitted {
			return TemplateMaterialization{}, false
		}
		targetSites[index] = materialized
		targetScopes[index] = scope
	}

	materializedInputs := make([]Input, len(inputs))
	type pendingInput struct {
		source, target Site
		provenance     composition.Key
		pre            Expr
		reindex        Reindex
		post           Expr
	}
	pending := make([]pendingInput, len(inputs))
	seenInputs := make(map[composition.Key]struct{}, len(inputs))
	for index, input := range inputs {
		if !input.Available() || input.Source().batch != data.formals || input.Target().batch != data.formals ||
			input.Source().dynamic != nil || input.Target().dynamic != nil {
			return TemplateMaterialization{}, false
		}
		if _, duplicate := seenInputs[input.Key()]; duplicate {
			return TemplateMaterialization{}, false
		}
		seenInputs[input.Key()] = struct{}{}
		sourcePoint, sourceOK := materializationResolvedPointOpen(binding, input.Source(), targetSites, targetScopes, PortImport)
		targetPoint, targetOK := materializationResolvedPointOpen(binding, input.Target(), targetSites, targetScopes, PortExport)
		if !sourceOK || !targetOK {
			return TemplateMaterialization{}, false
		}
		reindex, reindexed := boundReindex(input.Reindex(), sourcePoint, targetPoint, ambient, alpha)
		if !reindexed {
			return TemplateMaterialization{}, false
		}
		pre, post := input.Pre(), input.Post()
		var bound bool
		if sourcePoint.local {
			pre, bound = boundExpr(pre, alpha)
			if !bound {
				return TemplateMaterialization{}, false
			}
		}
		if targetPoint.local {
			post, bound = boundExpr(post, alpha)
			if !bound {
				return TemplateMaterialization{}, false
			}
		}
		provenance, keyed := identityKey("analysis/engine/equation/template-materialized-input-provenance", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, input.Provenance()) && writeKey(writer, binding.Key()) && writeKey(writer, input.Key())
		})
		if !keyed {
			return TemplateMaterialization{}, false
		}
		pending[index] = pendingInput{source: sourcePoint.site, target: targetPoint.site, provenance: provenance, pre: pre, reindex: reindex, post: post}
	}

	// A formal template may also carry ordinary target rows.  Admit those rows
	// into the same open target transaction as its Sites.  Keeping this here is
	// important: a template must not first seal a Site-only batch and then
	// replay Rules/Groups into a second directory.  The row capabilities below
	// are all reissued from the formal Batch and become readable only when the
	// target Batch seals.
	formalOccurrences := make(map[uint32]Occurrence, len(data.formals.occurrences))
	for index, row := range data.formals.occurrences {
		if row.site == 0 || uint64(row.site) > uint64(len(targetSites)) {
			return TemplateMaterialization{}, false
		}
		site := targetSites[row.site-1]
		var occurrence Occurrence
		var admitted bool
		switch row.kind {
		case OccurrenceAt:
			occurrence, admitted = target.At(site)
		case OccurrenceFrom:
			occurrence, admitted = target.From(site, row.entity)
		case OccurrenceRelation:
			occurrence, admitted = target.Relation(site, row.entity)
		default:
			admitted = false
		}
		if !admitted {
			return TemplateMaterialization{}, false
		}
		formalOccurrences[uint32(index+1)] = occurrence
	}
	formalOperands := make(map[uint32]Operand, len(data.formals.operands))
	for index, row := range data.formals.operands {
		occurrence, found := formalOccurrences[row.occurrence]
		if !found {
			return TemplateMaterialization{}, false
		}
		operand, admitted := target.admitOperandInRealm(occurrence, row.entity, binding.Key())
		if !admitted {
			return TemplateMaterialization{}, false
		}
		formalOperands[uint32(index+1)] = operand
	}

	resolveTargetPoint := func(ref PointRef) (PointRef, bool) {
		if ref == 0 || uint64(ref) > uint64(len(data.formals.targets.points)) {
			return 0, false
		}
		formal := data.formals.targets.points[uint64(ref)-1]
		if !formal.Site.Available() || formal.Site.batch != data.formals || formal.Site.row == 0 || uint64(formal.Site.row) > uint64(len(targetSites)) {
			return 0, false
		}
		return PointAt(int(ref) - 1), true
	}
	resolveTargetInput := func(value BatchInput) (BatchInput, bool) {
		if !value.Source.Available() || !value.Target.Available() || value.Source.batch != data.formals || value.Target.batch != data.formals {
			return BatchInput{}, false
		}
		sourcePoint, sourceOK := materializationResolvedPointOpen(binding, value.Source, targetSites, targetScopes, PortImport)
		targetPoint, targetOK := materializationResolvedPointOpen(binding, value.Target, targetSites, targetScopes, PortExport)
		if !sourceOK || !targetOK {
			return BatchInput{}, false
		}
		reindex, ok := boundReindex(value.Reindex, sourcePoint, targetPoint, ambient, alpha)
		if !ok {
			return BatchInput{}, false
		}
		pre, post := value.Pre, value.Post
		if sourcePoint.local {
			pre, ok = boundExpr(pre, alpha)
			if !ok {
				return BatchInput{}, false
			}
		}
		if targetPoint.local {
			post, ok = boundExpr(post, alpha)
			if !ok {
				return BatchInput{}, false
			}
		}
		provenance, ok := identityKey("analysis/engine/equation/template-materialized-target-input-provenance", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, value.Provenance) && writeKey(writer, binding.Key())
		})
		if !ok {
			return BatchInput{}, false
		}
		return TargetBoundaryInput(sourcePoint.site, targetPoint.site, provenance, pre, reindex, post), true
	}
	for _, point := range data.formals.targets.points {
		if !point.Site.Available() || point.Site.batch != data.formals || point.Site.row == 0 || uint64(point.Site.row) > uint64(len(targetSites)) {
			return TemplateMaterialization{}, false
		}
		if _, admitted := target.AdmitPoint(targetSites[point.Site.row-1]); !admitted {
			return TemplateMaterialization{}, false
		}
	}
	for _, rule := range data.formals.targets.rules {
		occurrence, occurrenceOK := formalOccurrences[rule.Occurrence.row]
		operand, operandOK := formalOperands[rule.Operand.row]
		if !occurrenceOK || !operandOK {
			return TemplateMaterialization{}, false
		}
		copy := copyInstance(rule)
		copy.Occurrence, copy.Operand = occurrence, operand
		if !target.AdmitRule(copy) {
			return TemplateMaterialization{}, false
		}
	}
	for _, group := range data.formals.targets.groups {
		output, outputOK := resolveTargetPoint(group.Output)
		if !outputOK {
			return TemplateMaterialization{}, false
		}
		bound := BatchGroup{Members: append([]RuleRef(nil), group.Members...), Output: output, Premise: group.Premise}
		if bound.Premise.Available() {
			bound.Premise, outputOK = boundExpr(bound.Premise, alpha)
			if !outputOK {
				return TemplateMaterialization{}, false
			}
		}
		for _, input := range group.Inputs {
			value, inputOK := resolveTargetInput(input)
			if !inputOK {
				return TemplateMaterialization{}, false
			}
			bound.Inputs = append(bound.Inputs, value)
		}
		if group.EnvironmentInput != nil {
			value, inputOK := resolveTargetInput(*group.EnvironmentInput)
			if !inputOK {
				return TemplateMaterialization{}, false
			}
			bound.EnvironmentInput = &value
		}
		if !target.AdmitGroup(bound) {
			return TemplateMaterialization{}, false
		}
	}
	for _, edge := range data.formals.targets.factorEdges {
		targetPoint, targetOK := resolveTargetPoint(edge.Target)
		input, inputOK := resolveTargetInput(edge.Input)
		if !targetOK || !inputOK || !target.AdmitFactorEdge(BatchFactorEdge{Target: targetPoint, Input: input, Factor: edge.Factor}) {
			return TemplateMaterialization{}, false
		}
	}
	for _, edge := range data.formals.targets.environment {
		targetPoint, targetOK := resolveTargetPoint(edge.Target)
		input, inputOK := resolveTargetInput(edge.Input)
		if !targetOK || !inputOK || !target.AdmitEnvironmentEdge(BatchEnvironmentEdge{Target: targetPoint, Input: input}) {
			return TemplateMaterialization{}, false
		}
	}
	for _, input := range data.formals.targets.inputs {
		bound, inputOK := resolveTargetInput(input)
		if !inputOK || !target.AdmitInput(bound) {
			return TemplateMaterialization{}, false
		}
	}
	for _, value := range pending {
		if !target.AdmitInput(TargetBoundaryInput(value.source, value.target, value.provenance, value.pre, value.reindex, value.post)) {
			return TemplateMaterialization{}, false
		}
	}
	// Metadata is part of the formal Batch receipt, not an engine-side
	// reconstruction. Re-admit the exact sealed rows into this target Batch
	// before its single Seal so summaries/weak-target coverage cannot vanish
	// between formal issuance and topology assembly.
	summaries, weakTargets, metadataOK := materializationMetadata(source, binding)
	if !metadataOK {
		return TemplateMaterialization{}, false
	}
	for _, summary := range summaries {
		if !target.AdmitSummary(summary) {
			return TemplateMaterialization{}, false
		}
	}
	for _, weak := range weakTargets {
		if !target.AdmitWeakTarget(weak) {
			return TemplateMaterialization{}, false
		}
	}

	// Sites and all target rows are admitted in one open transaction. Inputs
	// are immutable receipts, so their keys can only be derived after the sole
	// Batch seal; no provisional seal or second target directory is permitted.
	if !target.Seal() {
		return TemplateMaterialization{}, false
	}
	for _, site := range targetSites {
		if !target.ownsConcreteSite(site) {
			return TemplateMaterialization{}, false
		}
	}
	for index, value := range pending {
		materializedInputs[index] = BoundaryInput(value.source, value.target, value.provenance, value.pre, value.reindex, value.post)
		if !materializedInputs[index].Available() || materializedInputs[index].Source().batch != target || materializedInputs[index].Target().batch != target {
			return TemplateMaterialization{}, false
		}
	}

	key, keyed := templateMaterializationKey(source, binding, target, materializedInputs)
	if !keyed {
		return TemplateMaterialization{}, false
	}
	result := &templateMaterializationData{
		source: source, binding: binding.data, batch: target, key: key,
		authority: &templateMaterializationAuthority{marker: 1}, sites: targetSites, inputs: materializedInputs,
	}
	return TemplateMaterialization{data: result}, true
}

// materializationMetadata reissues the formal metadata receipt through the
// exact port-read substitutions carried by TemplateBinding. A weak candidate
// that names an imported formal surface must resolve to its caller surface;
// collapsing two formal candidates to one caller surface is rejected rather
// than silently changing coverage.
func materializationMetadata(source *composition.Composition, binding TemplateBinding) ([]SummaryMapping, []WeakTargetMapping, bool) {
	if source == nil || !binding.Available() {
		return nil, nil, false
	}
	summaries, weakTargets, ok := binding.data.formals.TargetMetadataRows()
	if !ok {
		return nil, nil, false
	}
	substitutions := make(map[Surface]Surface)
	for _, row := range binding.data.rows {
		formal, formalOK := row.formal.rowValue()
		if !formalOK || len(formal.formalReads) != len(row.reads) {
			return nil, nil, false
		}
		for index, read := range formal.formalReads {
			actual := row.reads[index]
			if existing, duplicate := substitutions[read.Surface]; duplicate && existing != actual.Surface {
				return nil, nil, false
			}
			substitutions[read.Surface] = actual.Surface
		}
	}
	for index, value := range summaries {
		if !validSummaryMapping(value) {
			return nil, nil, false
		}
		if _, present := source.FactorIndex(value.Surface.Factor); !present {
			return nil, nil, false
		}
		if replacement, mapped := substitutions[value.Surface]; mapped {
			if replacement.Form != value.Surface.Form || replacement.Factor != value.Surface.Factor {
				return nil, nil, false
			}
			value.Surface = replacement
		}
		summaries[index] = value
	}
	for index, value := range weakTargets {
		if !validWeakTargetMapping(value) {
			return nil, nil, false
		}
		if _, present := source.FactorIndex(value.Surface.Factor); !present {
			return nil, nil, false
		}
		if replacement, mapped := substitutions[value.Surface]; mapped {
			value.Surface = replacement
		}
		seen := make(map[Surface]struct{}, len(value.Candidates))
		for candidateIndex, candidate := range value.Candidates {
			if replacement, mapped := substitutions[candidate]; mapped {
				candidate = replacement
			}
			if !candidate.Available() || candidate.Factor != value.Surface.Factor {
				return nil, nil, false
			}
			if _, duplicate := seen[candidate]; duplicate {
				return nil, nil, false
			}
			seen[candidate] = struct{}{}
			value.Candidates[candidateIndex] = candidate
		}
		sort.Slice(value.Candidates, func(left, right int) bool { return lessSurface(value.Candidates[left], value.Candidates[right]) })
		if !validWeakTargetMapping(value) {
			return nil, nil, false
		}
		weakTargets[index] = value
	}
	return summaries, weakTargets, true
}

// materializationResolvedPointOpen resolves a formal endpoint while the
// target Batch is still open. It carries the already-authenticated scope
// computed by the materializer; it deliberately does not read Site identity
// before the transaction's single seal.
func materializationResolvedPointOpen(binding TemplateBinding, formal Site, targets []Site, scopes []Scope, required PortMode) (templateResolvedPoint, bool) {
	if !binding.Available() || !formal.Available() || formal.batch != binding.data.formals || formal.dynamic != nil || formal.row == 0 || uint64(formal.row) > uint64(len(targets)) || len(scopes) != len(targets) {
		return templateResolvedPoint{}, false
	}
	target := targets[formal.row-1]
	scope := scopes[formal.row-1]
	if target.batch == nil || target.dynamic != nil || !scope.Available() {
		return templateResolvedPoint{}, false
	}
	formalRow, ok := binding.data.formals.sealedSite(formal.row)
	if !ok {
		return templateResolvedPoint{}, false
	}
	if !formalRow.formal {
		return templateResolvedPoint{ref: PointRef(formal.row), site: target, scope: scope, rawScope: formal.Scope(), local: true, open: true}, true
	}
	port := FormalPort{batch: binding.data.formals, row: formal.row}
	var actual Site
	if required == PortImport {
		var ingress Reindex
		var resolved bool
		actual, _, ingress, resolved = binding.ResolveImport(port)
		if !resolved || !ingress.Available() {
			return templateResolvedPoint{}, false
		}
	} else if required == PortExport {
		var egress Reindex
		var resolved bool
		actual, egress, resolved = binding.ResolveExport(port)
		if !resolved || !egress.Available() {
			return templateResolvedPoint{}, false
		}
	} else {
		return templateResolvedPoint{}, false
	}
	if !sameScope(scope, actual.Scope()) {
		return templateResolvedPoint{}, false
	}
	return templateResolvedPoint{ref: PointRef(formal.row), site: target, scope: scope, rawScope: formal.Scope(), open: true}, true
}

func validateTemplateBindingReads(source *composition.Composition, binding TemplateBinding) bool {
	if source == nil || !binding.Available() {
		return false
	}
	for _, row := range binding.data.rows {
		formalRow, ok := row.formal.rowValue()
		if !ok || !compatiblePortReads(source, formalRow.formalReads, row.reads) {
			return false
		}
	}
	return true
}

func exactFormalSiteDenominator(batch *Batch, values []Site) ([]Site, bool) {
	if batch == nil || !batch.Sealed() || len(values) != len(batch.sites) {
		return nil, false
	}
	result := make([]Site, len(batch.sites))
	for _, site := range values {
		if !site.Available() || site.batch != batch || site.dynamic != nil || site.row == 0 || uint64(site.row) > uint64(len(result)) || result[site.row-1].Available() {
			return nil, false
		}
		result[site.row-1] = site
	}
	for index, site := range result {
		row, ok := batch.sealedSite(uint32(index + 1))
		if !ok || !site.Available() || site.Key() != row.key {
			return nil, false
		}
	}
	return result, true
}

func bindingRowForPort(binding TemplateBinding, port FormalPort) (templateBindingRow, bool) {
	if !binding.Available() || !port.Available() || port.batch != binding.data.formals || port.row == 0 || uint64(port.row) > uint64(len(binding.data.bySite)) {
		return templateBindingRow{}, false
	}
	rowIndex := binding.data.bySite[port.row-1]
	if rowIndex == 0 || uint64(rowIndex) > uint64(len(binding.data.rows)) {
		return templateBindingRow{}, false
	}
	row := binding.data.rows[rowIndex-1]
	if !row.formal.Same(port) || !row.actual.Available() || row.actual.batch != binding.data.actuals || row.actual.dynamic != nil {
		return templateBindingRow{}, false
	}
	return row, true
}

func templateBindingAmbient(binding TemplateBinding) (Scope, bool) {
	if !binding.Available() {
		return Scope{}, false
	}
	var ambient Scope
	for _, row := range binding.data.rows {
		if !row.actual.Available() || !row.actual.Scope().Available() {
			return Scope{}, false
		}
		if !ambient.Available() {
			ambient = row.actual.Scope()
		} else if !sameScope(ambient, row.actual.Scope()) {
			return Scope{}, false
		}
	}
	return ambient, ambient.Available()
}

func templateMaterializationAlpha(binding TemplateBinding) (decisionAlpha, bool) {
	if !binding.Available() {
		return nil, false
	}
	result := make(decisionAlpha)
	for index, row := range binding.data.formals.sites {
		if row.formal {
			continue
		}
		site := Site{batch: binding.data.formals, row: uint32(index + 1)}
		if !site.Available() || !site.Scope().Available() {
			return nil, false
		}
		for _, decision := range site.Scope().row.decisions {
			if _, found := result[decision.key]; found {
				continue
			}
			key, ok := identityKey("analysis/engine/equation/template-materialized-decision", func(writer *canonical.DigestWriter) bool {
				return writeKey(writer, decision.key) && writeKey(writer, binding.data.formals.Key()) && writeKey(writer, binding.Key())
			})
			if !ok {
				return nil, false
			}
			bound, ok := NewDecision(key)
			if !ok {
				return nil, false
			}
			result[decision.key] = bound
		}
	}
	return result, true
}

func materializationResolvedPoint(binding TemplateBinding, formal Site, targets []Site, required PortMode) (templateResolvedPoint, bool) {
	if !binding.Available() || !formal.Available() || formal.batch != binding.data.formals || formal.dynamic != nil || formal.row == 0 || uint64(formal.row) > uint64(len(targets)) {
		return templateResolvedPoint{}, false
	}
	target := targets[formal.row-1]
	if !target.Available() || target.dynamic != nil {
		return templateResolvedPoint{}, false
	}
	formalRow, ok := binding.data.formals.sealedSite(formal.row)
	if !ok {
		return templateResolvedPoint{}, false
	}
	if !formalRow.formal {
		return templateResolvedPoint{ref: PointRef(formal.row), site: target, scope: target.Scope(), rawScope: formal.Scope(), local: true}, true
	}
	port := FormalPort{batch: binding.data.formals, row: formal.row}
	var actual Site
	if required == PortImport {
		var ingress Reindex
		var resolved bool
		actual, _, ingress, resolved = binding.ResolveImport(port)
		if !resolved || !ingress.Available() {
			return templateResolvedPoint{}, false
		}
	} else if required == PortExport {
		var egress Reindex
		var resolved bool
		actual, egress, resolved = binding.ResolveExport(port)
		if !resolved || !egress.Available() {
			return templateResolvedPoint{}, false
		}
	} else {
		return templateResolvedPoint{}, false
	}
	if target.Key() != actual.Key() || !sameScope(target.Scope(), actual.Scope()) {
		return templateResolvedPoint{}, false
	}
	return templateResolvedPoint{ref: PointRef(formal.row), site: target, scope: target.Scope(), rawScope: formal.Scope()}, true
}

func templateMaterializationKey(source *composition.Composition, binding TemplateBinding, batch *Batch, inputs []Input) (composition.Key, bool) {
	if source == nil || !source.ID().Available() || !binding.Available() || batch == nil || !batch.Sealed() {
		return composition.Key{}, false
	}
	keys := make([]composition.Key, len(inputs))
	for index, input := range inputs {
		if !input.Available() || input.Source().batch != batch || input.Target().batch != batch {
			return composition.Key{}, false
		}
		keys[index] = input.Key()
	}
	sort.Slice(keys, func(left, right int) bool { return lessKey(keys[left], keys[right]) })
	for index := 1; index < len(keys); index++ {
		if keys[index-1] == keys[index] {
			return composition.Key{}, false
		}
	}
	compositionID := source.ID()
	return identityKey("analysis/engine/equation/template-materialization", func(writer *canonical.DigestWriter) bool {
		if writer.Bytes(compositionID[:]) != nil || !writeKey(writer, binding.Key()) || !writeKey(writer, batch.Key()) || writer.Count(uint64(len(keys))) != nil {
			return false
		}
		for _, key := range keys {
			if !writeKey(writer, key) {
				return false
			}
		}
		return true
	})
}

func (value TemplateMaterialization) Available() bool {
	data := value.data
	return data != nil && data.source != nil && data.source.ID().Available() && data.binding != nil && data.batch != nil && data.batch.Sealed() &&
		(TemplateBinding{data: data.binding}).Available() && data.key.Available() && data.authority != nil && data.authority.marker == 1 &&
		len(data.sites) == len(data.binding.formals.sites)
}

// OwnedBy proves the materialization was issued from this exact cold
// composition and TemplateBinding actuals batch. It is an admission witness,
// not a projection of the underlying binding internals.
func (value TemplateMaterialization) OwnedBy(source *composition.Composition, base *Batch) bool {
	return value.Available() && source != nil && base != nil && value.data.source == source && value.data.binding != nil && value.data.binding.authority != nil && value.data.binding.actuals == base && value.data.batch != nil && value.data.batch != base
}

func (value TemplateMaterialization) Same(other TemplateMaterialization) bool {
	return value.Available() && other.Available() && value.data == other.data
}

func (value TemplateMaterialization) Key() composition.Key {
	if !value.Available() {
		return composition.Key{}
	}
	return value.data.key
}

func (value TemplateMaterialization) Batch() *Batch {
	if !value.Available() {
		return nil
	}
	return value.data.batch
}

// WithOrigin returns the same sealed materialization authenticated for one
// concrete activation locator. It is the only production route for attaching
// trigger metadata to a receipt.
func (value TemplateMaterialization) WithOrigin(origin MaterializationOrigin) (TemplateMaterialization, bool) {
	if !value.Available() || !origin.Family.Available() || !origin.Application.Available() || !origin.Target.Available() || !origin.Endpoint.Available() || origin.TriggerOrdinal < 0 {
		return TemplateMaterialization{}, false
	}
	key, ok := identityKey("analysis/engine/equation/template-materialization-origin", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, value.data.key) && writeKey(writer, origin.Family) && writeKey(writer, origin.Application) && writeKey(writer, origin.Target) && writeKey(writer, origin.Endpoint) && writer.Uint(uint64(origin.TriggerOrdinal)) == nil
	})
	if !ok {
		return TemplateMaterialization{}, false
	}
	copy := *value.data
	copy.key = key
	copy.origin = origin
	copy.hasOrigin = true
	return TemplateMaterialization{data: &copy}, true
}

func (value TemplateMaterialization) Origin() (MaterializationOrigin, bool) {
	if !value.Available() || !value.data.hasOrigin {
		return MaterializationOrigin{}, false
	}
	return value.data.origin, true
}

// Site returns the ordinary target-owned Site corresponding to one exact
// formal-Batch Site capability.
func (value TemplateMaterialization) Site(formal Site) (Site, bool) {
	if !value.Available() || !formal.Available() || formal.batch != value.data.binding.formals || formal.dynamic != nil || formal.row == 0 || uint64(formal.row) > uint64(len(value.data.sites)) {
		return Site{}, false
	}
	result := value.data.sites[formal.row-1]
	return result, result.Available() && result.batch == value.data.batch
}

func (value TemplateMaterialization) InputCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.data.inputs)
}

func (value TemplateMaterialization) InputAt(index int) (Input, bool) {
	if !value.Available() || index < 0 || index >= len(value.data.inputs) {
		return Input{}, false
	}
	return value.data.inputs[index], true
}

// ResolveImport returns the materialized target Site and the exact caller read
// slots only after this transaction authenticated their Factor/form against
// its cold Composition.
func (value TemplateMaterialization) ResolveImport(port FormalPort) (Site, []PortRead, bool) {
	if !value.Available() || !port.Available() || port.batch != value.data.binding.formals {
		return Site{}, nil, false
	}
	binding := TemplateBinding{data: value.data.binding}
	_, reads, _, ok := binding.ResolveImport(port)
	if !ok {
		return Site{}, nil, false
	}
	site, ok := value.Site(port.Site())
	return site, reads, ok
}

// ResolveExport returns the materialized target Site only for an export
// capability authenticated by this transaction's exact TemplateBinding.
func (value TemplateMaterialization) ResolveExport(port FormalPort) (Site, bool) {
	if !value.Available() || !port.Available() || port.batch != value.data.binding.formals {
		return Site{}, false
	}
	binding := TemplateBinding{data: value.data.binding}
	if _, _, ok := binding.ResolveExport(port); !ok {
		return Site{}, false
	}
	return value.Site(port.Site())
}
