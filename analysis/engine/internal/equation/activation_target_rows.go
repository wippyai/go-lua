package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

type activationTargetRows struct{ data *activationTargetRowsData }

type activationTargetRowsData struct {
	source  *composition.Composition
	binding *templateBindingData
	batch   *Batch
	key     composition.Key
	sites   []Site // formal Batch Site row -> exact target-owned Site
	inputs  []Input
}

// lowerActivationTargetRows consumes one exact TemplateBinding and the
// complete Site denominator of its formal Batch. It reissues those Sites and
// the supplied ordinary Inputs into one new sealed target Batch. No cross-Batch
// Input or unresolved formal Site survives the transaction.
//
// The current formal Scope is empty. All concrete ports must therefore share
// one exact ambient Scope, matching the existing activation attachment law.
// Local formal decisions are alpha-renamed beneath that ambient Scope; the
// existing boundScope/boundExpr/boundReindex functions remain the sole lowering
// implementation.
func lowerActivationTargetRows(source *composition.Composition, binding TemplateBinding, sites []Site, inputs []Input) (activationTargetRows, bool) {
	if source == nil || !source.ID().Available() || !binding.Available() {
		return activationTargetRows{}, false
	}
	data := binding.data
	if data == nil || len(sites) != len(data.formals.sites) || len(sites) == 0 {
		return activationTargetRows{}, false
	}
	if !validateTemplateBindingReads(source, binding) {
		return activationTargetRows{}, false
	}
	formalSites, sitesOK := exactFormalSiteDenominator(data.formals, sites)
	if !sitesOK {
		return activationTargetRows{}, false
	}
	ambient, ambientOK := templateBindingAmbient(binding)
	if !ambientOK {
		return activationTargetRows{}, false
	}
	alpha, alphaOK := activationTargetAlpha(binding)
	if !alphaOK {
		return activationTargetRows{}, false
	}

	target := NewBatch()
	targetSites := make([]Site, len(formalSites))
	targetScopes := make([]Scope, len(formalSites))
	actualSites := make(map[composition.Key]Site, len(data.rows))
	for index, formalSite := range formalSites {
		formalRow, rowOK := data.formals.sealedSite(formalSite.row)
		if !rowOK {
			return activationTargetRows{}, false
		}
		if formalRow.formal {
			bindingRow, bound := bindingRowForPort(binding, FormalPort{batch: data.formals, row: formalSite.row})
			if !bound || !sameScope(bindingRow.actual.Scope(), ambient) {
				return activationTargetRows{}, false
			}
			if existing, found := actualSites[bindingRow.actual.Key()]; found {
				targetSites[index] = existing
				targetScopes[index] = ambient
				continue
			}
			init, disposition, initialized := bindingRow.actual.Init()
			if !initialized {
				return activationTargetRows{}, false
			}
			lowered, admitted := target.AdmitSite(bindingRow.actual.Source(), bindingRow.actual.Scope(), init, disposition)
			if !admitted {
				return activationTargetRows{}, false
			}
			actualSites[bindingRow.actual.Key()] = lowered
			targetSites[index] = lowered
			targetScopes[index] = ambient
			continue
		}
		scope, scoped := boundScope(formalSite.Scope(), ambient, alpha)
		init, disposition, initialized := formalSite.Init()
		if !scoped || !initialized {
			return activationTargetRows{}, false
		}
		init, initialized = boundExpr(init, alpha)
		if !initialized {
			return activationTargetRows{}, false
		}
		sourceKey, keyed := identityKey("analysis/engine/equation/activation-target-site-source", func(writer *canonical.DigestWriter) bool {
			return writeSite(writer, formalSite) && writeKey(writer, binding.Key())
		})
		if !keyed {
			return activationTargetRows{}, false
		}
		lowered, admitted := target.AdmitSite(sourceKey, scope, init, disposition)
		if !admitted {
			return activationTargetRows{}, false
		}
		targetSites[index] = lowered
		targetScopes[index] = scope
	}

	loweredInputs := make([]Input, len(inputs))
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
			return activationTargetRows{}, false
		}
		if _, duplicate := seenInputs[input.Key()]; duplicate {
			return activationTargetRows{}, false
		}
		seenInputs[input.Key()] = struct{}{}
		sourcePoint, sourceOK := activationTargetResolvedPointOpen(binding, input.Source(), targetSites, targetScopes, PortImport)
		targetPoint, targetOK := activationTargetResolvedPointOpen(binding, input.Target(), targetSites, targetScopes, PortExport)
		if !sourceOK || !targetOK {
			return activationTargetRows{}, false
		}
		reindex, reindexed := boundReindex(input.Reindex(), sourcePoint, targetPoint, ambient, alpha)
		if !reindexed {
			return activationTargetRows{}, false
		}
		pre, post := input.Pre(), input.Post()
		var bound bool
		if sourcePoint.local {
			pre, bound = boundExpr(pre, alpha)
			if !bound {
				return activationTargetRows{}, false
			}
		}
		if targetPoint.local {
			post, bound = boundExpr(post, alpha)
			if !bound {
				return activationTargetRows{}, false
			}
		}
		provenance, keyed := identityKey("analysis/engine/equation/activation-target-input-provenance", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, input.Provenance()) && writeKey(writer, binding.Key()) && writeKey(writer, input.Key())
		})
		if !keyed {
			return activationTargetRows{}, false
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
			return activationTargetRows{}, false
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
			return activationTargetRows{}, false
		}
		formalOccurrences[uint32(index+1)] = occurrence
	}
	formalOperands := make(map[uint32]Operand, len(data.formals.operands))
	for index, row := range data.formals.operands {
		occurrence, found := formalOccurrences[row.occurrence]
		if !found {
			return activationTargetRows{}, false
		}
		operand, admitted := target.admitOperandInRealm(occurrence, row.entity, binding.Key())
		if !admitted {
			return activationTargetRows{}, false
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
		sourcePoint, sourceOK := activationTargetResolvedPointOpen(binding, value.Source, targetSites, targetScopes, PortImport)
		targetPoint, targetOK := activationTargetResolvedPointOpen(binding, value.Target, targetSites, targetScopes, PortExport)
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
		provenance, ok := identityKey("analysis/engine/equation/activation-target-target-input-provenance", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, value.Provenance) && writeKey(writer, binding.Key())
		})
		if !ok {
			return BatchInput{}, false
		}
		return TargetBoundaryInput(sourcePoint.site, targetPoint.site, provenance, pre, reindex, post), true
	}
	for _, point := range data.formals.targets.points {
		if !point.Site.Available() || point.Site.batch != data.formals || point.Site.row == 0 || uint64(point.Site.row) > uint64(len(targetSites)) {
			return activationTargetRows{}, false
		}
		if _, admitted := target.AdmitPoint(targetSites[point.Site.row-1]); !admitted {
			return activationTargetRows{}, false
		}
	}
	for _, rule := range data.formals.targets.rules {
		occurrence, occurrenceOK := formalOccurrences[rule.Occurrence.row]
		operand, operandOK := formalOperands[rule.Operand.row]
		if !occurrenceOK || !operandOK {
			return activationTargetRows{}, false
		}
		copy := copyInstance(rule)
		copy.Occurrence, copy.Operand = occurrence, operand
		if !target.AdmitRule(copy) {
			return activationTargetRows{}, false
		}
	}
	for _, group := range data.formals.targets.groups {
		output, outputOK := resolveTargetPoint(group.Output)
		if !outputOK {
			return activationTargetRows{}, false
		}
		bound := BatchGroup{Members: append([]RuleRef(nil), group.Members...), Output: output, Premise: group.Premise}
		if bound.Premise.Available() {
			bound.Premise, outputOK = boundExpr(bound.Premise, alpha)
			if !outputOK {
				return activationTargetRows{}, false
			}
		}
		for _, input := range group.Inputs {
			value, inputOK := resolveTargetInput(input)
			if !inputOK {
				return activationTargetRows{}, false
			}
			bound.Inputs = append(bound.Inputs, value)
		}
		if group.EnvironmentInput != nil {
			value, inputOK := resolveTargetInput(*group.EnvironmentInput)
			if !inputOK {
				return activationTargetRows{}, false
			}
			bound.EnvironmentInput = &value
		}
		if !target.AdmitGroup(bound) {
			return activationTargetRows{}, false
		}
	}
	for _, edge := range data.formals.targets.factorEdges {
		targetPoint, targetOK := resolveTargetPoint(edge.Target)
		input, inputOK := resolveTargetInput(edge.Input)
		if !targetOK || !inputOK || !target.AdmitFactorEdge(BatchFactorEdge{Target: targetPoint, Input: input, Factor: edge.Factor}) {
			return activationTargetRows{}, false
		}
	}
	for _, edge := range data.formals.targets.environment {
		targetPoint, targetOK := resolveTargetPoint(edge.Target)
		input, inputOK := resolveTargetInput(edge.Input)
		if !targetOK || !inputOK || !target.AdmitEnvironmentEdge(BatchEnvironmentEdge{Target: targetPoint, Input: input}) {
			return activationTargetRows{}, false
		}
	}
	for _, input := range data.formals.targets.inputs {
		bound, inputOK := resolveTargetInput(input)
		if !inputOK || !target.AdmitInput(bound) {
			return activationTargetRows{}, false
		}
	}
	for _, value := range pending {
		if !target.AdmitInput(TargetBoundaryInput(value.source, value.target, value.provenance, value.pre, value.reindex, value.post)) {
			return activationTargetRows{}, false
		}
	}
	// Metadata is part of the formal Batch rows, not an engine-side
	// reconstruction. Re-admit the exact sealed rows into this target Batch
	// before its single Seal so summaries/weak-target coverage cannot vanish
	// between formal issuance and topology sealing.
	summaries, weakTargets, metadataOK := activationTargetMetadata(source, binding)
	if !metadataOK {
		return activationTargetRows{}, false
	}
	for _, summary := range summaries {
		if !target.AdmitSummary(summary) {
			return activationTargetRows{}, false
		}
	}
	for _, weak := range weakTargets {
		if !target.AdmitWeakTarget(weak) {
			return activationTargetRows{}, false
		}
	}

	// Sites and all target rows are admitted in one open transaction. Inputs
	// are immutable rows, so their keys can only be derived after the sole
	// Batch seal; no provisional seal or second target directory is permitted.
	if !target.Seal() {
		return activationTargetRows{}, false
	}
	for _, site := range targetSites {
		if !target.ownsConcreteSite(site) {
			return activationTargetRows{}, false
		}
	}
	for index, value := range pending {
		loweredInputs[index] = BoundaryInput(value.source, value.target, value.provenance, value.pre, value.reindex, value.post)
		if !loweredInputs[index].Available() || loweredInputs[index].Source().batch != target || loweredInputs[index].Target().batch != target {
			return activationTargetRows{}, false
		}
	}

	key, keyed := activationTargetKey(source, binding, target, loweredInputs)
	if !keyed {
		return activationTargetRows{}, false
	}
	result := &activationTargetRowsData{
		source: source, binding: binding.data, batch: target, key: key, sites: targetSites, inputs: loweredInputs,
	}
	return activationTargetRows{data: result}, true
}

// activationTargetMetadata reissues the formal metadata rows through the
// exact port-read substitutions carried by TemplateBinding. A weak candidate
// that names an imported formal surface must resolve to its caller surface;
// collapsing two formal candidates to one caller surface is rejected rather
// than silently changing coverage.
func activationTargetMetadata(source *composition.Composition, binding TemplateBinding) ([]SummaryMapping, []WeakTargetMapping, bool) {
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

// activationTargetResolvedPointOpen resolves a formal endpoint while the
// target Batch is still open. It carries the already-authenticated scope
// computed by the materializer; it deliberately does not read Site identity
// before the transaction's single seal.
func activationTargetResolvedPointOpen(binding TemplateBinding, formal Site, targets []Site, scopes []Scope, required PortMode) (templateResolvedPoint, bool) {
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

func activationTargetAlpha(binding TemplateBinding) (decisionAlpha, bool) {
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
			key, ok := identityKey("analysis/engine/equation/activation-target-decision", func(writer *canonical.DigestWriter) bool {
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

func activationTargetKey(source *composition.Composition, binding TemplateBinding, batch *Batch, inputs []Input) (composition.Key, bool) {
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
	return identityKey("analysis/engine/equation/activation-target-rows", func(writer *canonical.DigestWriter) bool {
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

func (value activationTargetRows) Available() bool {
	data := value.data
	return data != nil && data.source != nil && data.source.ID().Available() && data.binding != nil && data.batch != nil && data.batch.Sealed() &&
		(TemplateBinding{data: data.binding}).Available() && data.key.Available() && len(data.sites) == len(data.binding.formals.sites)
}

func (value activationTargetRows) Key() composition.Key {
	if !value.Available() {
		return composition.Key{}
	}
	return value.data.key
}

func (value activationTargetRows) Batch() *Batch {
	if !value.Available() {
		return nil
	}
	return value.data.batch
}

// Site returns the ordinary target-owned Site corresponding to one exact
// formal-Batch Site capability.
func (value activationTargetRows) Site(formal Site) (Site, bool) {
	if !value.Available() || !formal.Available() || formal.batch != value.data.binding.formals || formal.dynamic != nil || formal.row == 0 || uint64(formal.row) > uint64(len(value.data.sites)) {
		return Site{}, false
	}
	result := value.data.sites[formal.row-1]
	return result, result.Available() && result.batch == value.data.batch
}

func (value activationTargetRows) InputCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.data.inputs)
}

func (value activationTargetRows) InputAt(index int) (Input, bool) {
	if !value.Available() || index < 0 || index >= len(value.data.inputs) {
		return Input{}, false
	}
	return value.data.inputs[index], true
}

// ResolveImport returns the lowered target Site and the exact caller read
// slots only after this transaction authenticated their Factor/form against
// its cold Composition.
func (value activationTargetRows) ResolveImport(port FormalPort) (Site, []PortRead, bool) {
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

// ResolveExport returns the lowered target Site only for an export
// capability authenticated by this transaction's exact TemplateBinding.
func (value activationTargetRows) ResolveExport(port FormalPort) (Site, bool) {
	if !value.Available() || !port.Available() || port.batch != value.data.binding.formals {
		return Site{}, false
	}
	binding := TemplateBinding{data: value.data.binding}
	if _, _, ok := binding.ResolveExport(port); !ok {
		return Site{}, false
	}
	return value.Site(port.Site())
}
