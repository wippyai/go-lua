package exportmanifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func publishFunctionSignatures(m *manifest.Manifest, modulePath string, result program.Result) {
	root := result.RootResult()
	if m == nil || modulePath == "" || root == nil || root.Graph() == nil {
		return
	}
	dom := dominance.ComputeImmediateDominatorInfo(root.Graph())
	for _, exportRoot := range returnedExportSourcePaths(root) {
		publishFunctionDefinitionSignatures(m, modulePath, result, root, dom, exportRoot)
		publishOrdinaryAssignmentFunctionSignatures(m, modulePath, result, root, dom, exportRoot)
	}
}

type returnedSourcePath struct {
	path   pathdom.Path
	points []cfg.Point
}

func returnedExportSourcePaths(result *body.Result) []returnedSourcePath {
	var out []returnedSourcePath
	seen := make(map[pathdom.PathKey]struct{})
	for _, point := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(point)
		if !ok || len(fact.Sources) == 0 {
			continue
		}
		source := fact.Sources[0]
		if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
			continue
		}
		p, ok := result.ExpressionPath(source.Expr)
		if !ok || p.IsEmpty() {
			continue
		}
		key := p.Key()
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			out = append(out, returnedSourcePath{path: p})
		}
		for i := range out {
			if out[i].path.Key() == key {
				out[i].points = append(out[i].points, point)
				break
			}
		}
	}
	return out
}

func dominatesAllReturnPoints(dom *dominance.ImmediateDominators, point cfg.Point, returns []cfg.Point) bool {
	if dom == nil || len(returns) == 0 {
		return false
	}
	for _, ret := range returns {
		if !dom.Dominates(point, ret) {
			return false
		}
	}
	return true
}

func publishFunctionDefinitionSignatures(
	m *manifest.Manifest,
	modulePath string,
	prog program.Result,
	root *body.Result,
	dom *dominance.ImmediateDominators,
	exportRoot returnedSourcePath,
) {
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, exportRoot.path, fact.Name)
		if !ok {
			continue
		}
		name, ok := functionSignatureName(modulePath, member)
		if !ok {
			continue
		}
		if sig, ok := functionExpressionSignature(prog, root, fact.Func, name); ok {
			m.DefineFunctionSignature(name, sig)
		}
	}
}

func publishOrdinaryAssignmentFunctionSignatures(
	m *manifest.Manifest,
	modulePath string,
	prog program.Result,
	root *body.Result,
	dom *dominance.ImmediateDominators,
	exportRoot returnedSourcePath,
) {
	if m == nil || root == nil || root.Graph() == nil || exportRoot.path.Symbol == 0 {
		return
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || !fact.HasPath || fact.Path.Symbol != exportRoot.path.Symbol {
			continue
		}
		member, ok := directMemberSegment(exportRoot.path.Segments, fact.Path.Segments)
		if !ok {
			continue
		}
		name, ok := functionSignatureName(modulePath, member)
		if !ok {
			continue
		}
		expr := ordinaryAssignmentRHSExpr(fact)
		if sig, ok := root.ExpressionSignatureAt(point, expr); ok {
			m.DefineFunctionSignature(name, sig.Clone())
			continue
		}
		fn, ok := expr.(*ast.FunctionExpr)
		if !ok {
			continue
		}
		sig, ok := functionExpressionSignature(prog, root, fn, name)
		if ok {
			m.DefineFunctionSignature(name, sig)
		}
	}
}

func functionDefinitionExportMember(result *body.Result, root pathdom.Path, name *ast.FuncName) (segment.Segment, bool) {
	if name == nil {
		return segment.Segment{}, false
	}
	if name.Method != "" {
		receiver, ok := result.ExpressionPath(name.Receiver)
		if !ok || !receiver.Equal(root) {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentField, Name: name.Method}, true
	}
	target, ok := result.ExpressionPath(name.Func)
	if !ok || target.Symbol != root.Symbol {
		return segment.Segment{}, false
	}
	return directMemberSegment(root.Segments, target.Segments)
}

func functionSignatureName(modulePath string, member segment.Segment) (string, bool) {
	if modulePath == "" {
		return "", false
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if member.Name == "" {
			return "", false
		}
		return modulePath + "." + member.Name, true
	case segment.SegmentIndexInt:
		return modulePath + segment.FormatSegments([]segment.Segment{member}), true
	default:
		return "", false
	}
}

func functionExpressionSignature(prog program.Result, result *body.Result, fn *ast.FunctionExpr, name string) (signature.Function, bool) {
	fnType, ok := functionSignatureType(result, fn)
	if !ok {
		return signature.Function{}, false
	}
	sig := signature.Function{Type: fnType}
	if summary, ok := functionSummary(prog, result, fn); ok {
		sig.Effect = functionSummaryEffect(summary, fnType)
		sig.OperationalEffects = functionSummaryOperationalEffects(result.Registry(), summary, fnType, name)
	}
	return sig, true
}

func functionSignatureType(result *body.Result, fn *ast.FunctionExpr) (*typ.Function, bool) {
	t, ok := functionDefinitionMemberType(result, fn)
	if !ok {
		return nil, false
	}
	fnType, ok := t.(*typ.Function)
	return fnType, ok && fnType != nil
}

func functionSummary(prog program.Result, result *body.Result, fn *ast.FunctionExpr) (summary.Summary, bool) {
	id, ok := result.FunctionSymbol(fn)
	if !ok || id == 0 {
		return summary.Summary{}, false
	}
	key, ok := prog.FunctionKey(id)
	if !ok {
		return summary.Summary{}, false
	}
	return prog.Snapshot().Read(key)
}

func functionSummaryEffect(s summary.Summary, fn *typ.Function) effect.Row {
	if fn == nil {
		return effect.Empty
	}
	labels := errorReturnLabels(s.ReturnPresenceRelations, len(fn.Returns))
	labels = append(labels, normalReturnParamRefinementLabels(s.NormalReturnParams, len(fn.Params))...)
	storeRelations, exactStoreSources, exactStoreTargets := normalReturnStoreRelationLabels(s.NormalReturnFacts, len(fn.Params))
	labels = append(labels, storeRelations...)
	labels = append(labels, normalReturnOwnershipLabels(s.NormalReturnFacts, len(fn.Params), exactStoreSources)...)
	labels = append(labels, normalReturnMutationLabels(s.NormalReturnFacts, len(fn.Params), exactStoreTargets)...)
	if len(labels) == 0 {
		return effect.Empty
	}
	row := effect.Empty
	for _, label := range labels {
		row = row.With(label)
	}
	return analyzedExportEffectRow(row)
}

func functionSummaryOperationalEffects(reg *axis.Registry, s summary.Summary, fn *typ.Function, signatureName string) *signature.OperationalEffects {
	if fn == nil {
		return nil
	}
	arity := len(fn.Params)
	out := signature.OperationalEffects{
		ReturnPresenceRelations:         operationalReturnPresenceRelations(s.ReturnPresenceRelations, len(fn.Returns)),
		NormalReturnPresenceRefinements: operationalNormalReturnPresenceRefinements(s.NormalReturnParams, arity),
		PathStaticMembers:               operationalPathStaticMembers(s.NormalReturnFacts, arity, reg),
		PathInvalidations:               operationalPathInvalidations(s.NormalReturnFacts, arity),
		FrozenTables:                    operationalFrozenTables(s.NormalReturnFacts, arity),
		EscapeEvents:                    operationalEscapeEvents(s.NormalReturnFacts, arity),
		StoreRelations:                  operationalStoreRelations(s.NormalReturnFacts, arity),
		ReturnAllocationTemplates:       operationalReturnAllocationTemplates(reg, s, signatureName),
	}
	if out.IsEmpty() {
		return nil
	}
	return &out
}

func operationalReturnAllocationTemplates(reg *axis.Registry, s summary.Summary, signatureName string) []signature.ReturnAllocationTemplate {
	if reg == nil || signatureName == "" || len(s.Returns) == 0 || len(s.HeapTableObjects) == 0 {
		return nil
	}
	var out []signature.ReturnAllocationTemplate
	for i, value := range s.Returns {
		id, ok := product.Get(reg, value, identity.Key).ID()
		if !ok {
			continue
		}
		template, ok := allocationTemplateForReturn(reg, s.HeapTableObjects, signatureName, i, id)
		if ok {
			out = append(out, template)
		}
	}
	return out
}

func allocationTemplateForReturn(
	reg *axis.Registry,
	objects map[identity.ID]heapidentity.TableObject,
	signatureName string,
	returnIndex int,
	rootID identity.ID,
) (signature.ReturnAllocationTemplate, bool) {
	if _, ok := objects[rootID]; !ok {
		return signature.ReturnAllocationTemplate{}, false
	}
	projector := allocationTemplateProjector{
		reg:           reg,
		objects:       objects,
		signatureName: signatureName,
		returnIndex:   returnIndex,
		rawToTemplate: make(map[identity.ID]signature.AllocationTemplateID),
		visiting:      make(map[signature.AllocationTemplateID]struct{}),
		emitted:       make(map[signature.AllocationTemplateID]struct{}),
	}
	rootTemplate := projector.templateID(rootID, "root")
	projector.visit(rootID, rootTemplate, "root")
	if len(projector.out) == 0 {
		return signature.ReturnAllocationTemplate{}, false
	}
	sort.Slice(projector.out, func(i, j int) bool {
		return projector.out[i].ID < projector.out[j].ID
	})
	return signature.ReturnAllocationTemplate{
		ReturnIndex: returnIndex,
		Root:        rootTemplate,
		Objects:     projector.out,
	}, true
}

type allocationTemplateProjector struct {
	reg           *axis.Registry
	objects       map[identity.ID]heapidentity.TableObject
	signatureName string
	returnIndex   int
	rawToTemplate map[identity.ID]signature.AllocationTemplateID
	visiting      map[signature.AllocationTemplateID]struct{}
	emitted       map[signature.AllocationTemplateID]struct{}
	out           []signature.AllocationObjectTemplate
}

func (p *allocationTemplateProjector) templateID(raw identity.ID, path string) signature.AllocationTemplateID {
	if raw == (identity.ID{}) {
		return ""
	}
	if id, ok := p.rawToTemplate[raw]; ok {
		return id
	}
	id := signature.AllocationTemplateID(fmt.Sprintf("%s:return:%d:%s", p.signatureName, p.returnIndex, path))
	p.rawToTemplate[raw] = id
	return id
}

func (p *allocationTemplateProjector) visit(raw identity.ID, templateID signature.AllocationTemplateID, path string) {
	if raw == (identity.ID{}) || templateID == "" {
		return
	}
	if _, ok := p.emitted[templateID]; ok {
		return
	}
	if _, ok := p.visiting[templateID]; ok {
		return
	}
	object, ok := p.exportableObject(raw)
	if !ok {
		return
	}
	p.visiting[templateID] = struct{}{}
	projected := signature.AllocationObjectTemplate{ID: templateID}
	if t, ok := valueType(p.reg, object.Root()); ok {
		projected.Type = t
	}
	for _, member := range sortedHeapStaticMembers(object.StaticMembers()) {
		memberID, ok := product.Get(p.reg, member.value, identity.Key).ID()
		if !ok {
			continue
		}
		childPath := path + segment.FormatSegments(member.suffix)
		childTemplate, ok := p.templateRef(memberID, childPath)
		if !ok {
			continue
		}
		projected.StaticMembers = append(projected.StaticMembers, signature.AllocationStaticMemberTemplate{
			Suffix: member.suffix,
			Value:  childTemplate,
		})
	}
	for _, entry := range sortedHeapDynamicEntries(object.DynamicIndexFacts()) {
		var projectedEntry signature.AllocationDynamicEntryTemplate
		if keyID, ok := product.Get(p.reg, entry.fact.KeyValue, identity.Key).ID(); ok {
			keyPath := fmt.Sprintf("%s:dynamic:%d:key", path, entry.index)
			if keyTemplate, ok := p.templateRef(keyID, keyPath); ok {
				projectedEntry.Key = keyTemplate
			}
		}
		if keyType, ok := typevalue.TypeOf(p.reg, entry.fact.KeyValue); ok {
			projectedEntry.KeyType = keyType
		}
		if valueID, ok := product.Get(p.reg, entry.fact.Value, identity.Key).ID(); ok {
			valuePath := fmt.Sprintf("%s:dynamic:%d:value", path, entry.index)
			if valueTemplate, ok := p.templateRef(valueID, valuePath); ok {
				projectedEntry.Value = valueTemplate
			}
		}
		if projectedEntry.Key == "" && projectedEntry.KeyType == nil && projectedEntry.Value == "" {
			continue
		}
		projected.DynamicEntries = append(projected.DynamicEntries, projectedEntry)
	}
	delete(p.visiting, templateID)
	p.emitted[templateID] = struct{}{}
	p.out = append(p.out, projected)
}

func (p *allocationTemplateProjector) templateRef(raw identity.ID, path string) (signature.AllocationTemplateID, bool) {
	if _, ok := p.exportableObject(raw); !ok {
		return "", false
	}
	id := p.templateID(raw, path)
	p.visit(raw, id, path)
	return id, true
}

func (p *allocationTemplateProjector) exportableObject(raw identity.ID) (heapidentity.TableObject, bool) {
	if raw == (identity.ID{}) {
		return heapidentity.TableObject{}, false
	}
	object, ok := p.objects[raw]
	if !ok || !valueHasIdentity(p.reg, object.Root(), raw) {
		return heapidentity.TableObject{}, false
	}
	return object, true
}

type heapStaticMember struct {
	suffix []segment.Segment
	value  product.Value
}

func sortedHeapStaticMembers(in map[pathdom.PathKey]product.Value) []heapStaticMember {
	out := make([]heapStaticMember, 0, len(in))
	for key, value := range in {
		suffix, ok := segment.ParseFormattedSegments(string(key))
		if !ok {
			continue
		}
		out = append(out, heapStaticMember{suffix: suffix, value: value})
	}
	sort.Slice(out, func(i, j int) bool {
		return segment.FormatSegments(out[i].suffix) < segment.FormatSegments(out[j].suffix)
	})
	return out
}

type heapDynamicEntry struct {
	index int
	key   string
	fact  dynamicindex.Fact
}

func sortedHeapDynamicEntries(in map[dynamicindex.Key]dynamicindex.Fact) []heapDynamicEntry {
	out := make([]heapDynamicEntry, 0, len(in))
	for key, fact := range in {
		if fact.Admission == dynamicindex.AdmissionRejected {
			continue
		}
		out = append(out, heapDynamicEntry{
			key:  string(key.Table) + "|" + string(key.Site),
			fact: fact,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].key < out[j].key
	})
	for i := range out {
		out[i].index = i
	}
	return out
}

func valueHasIdentity(reg *axis.Registry, value product.Value, want identity.ID) bool {
	got, ok := product.Get(reg, value, identity.Key).ID()
	return ok && got == want
}

func operationalReturnPresenceRelations(relations []summary.ReturnPresenceRelation, arity int) []signature.ReturnPresenceRelation {
	if arity <= 0 || len(relations) == 0 {
		return nil
	}
	out := make([]signature.ReturnPresenceRelation, 0, len(relations))
	for _, relation := range relations {
		if relation.TriggerIndex < 0 || relation.TriggerIndex >= arity || relation.TargetIndex < 0 || relation.TargetIndex >= arity {
			continue
		}
		if !operationalPresence(relation.TriggerPresence) || !operationalPresence(relation.TargetPresence) {
			continue
		}
		out = append(out, signature.ReturnPresenceRelation{
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: relation.TriggerPresence,
			TargetIndex:     relation.TargetIndex,
			TargetPresence:  relation.TargetPresence,
		})
	}
	return out
}

func operationalNormalReturnPresenceRefinements(values []product.Value, arity int) []signature.PathPresenceRefinement {
	if arity <= 0 || len(values) == 0 {
		return nil
	}
	limit := arity
	if len(values) < limit {
		limit = len(values)
	}
	var out []signature.PathPresenceRefinement
	for i := range limit {
		p := product.PresenceOf(values[i])
		if !operationalPresence(p) || presence.Equal(p, presence.Maybe()) {
			continue
		}
		out = append(out, signature.PathPresenceRefinement{
			Path:     pathdom.NewPlaceholder(i),
			Presence: p,
		})
	}
	return out
}

func operationalPathStaticMembers(facts callboundary.NormalReturnFacts, arity int, reg *axis.Registry) []signature.PathStaticMemberFact {
	if arity <= 0 || len(facts.PathStaticMembers) == 0 || reg == nil {
		return nil
	}
	out := make([]signature.PathStaticMemberFact, 0, len(facts.PathStaticMembers))
	for _, fact := range facts.PathStaticMembers {
		if !placeholderPathInArity(fact.Path, arity) {
			continue
		}
		t, ok := typevalue.TypeOf(reg, fact.Value)
		if !ok {
			continue
		}
		out = append(out, signature.PathStaticMemberFact{
			Path: fact.Path,
			Type: t,
		})
	}
	return out
}

func operationalPathInvalidations(facts callboundary.NormalReturnFacts, arity int) []signature.PathInvalidation {
	if arity <= 0 || len(facts.PathInvalidations) == 0 {
		return nil
	}
	out := make([]signature.PathInvalidation, 0, len(facts.PathInvalidations))
	for _, fact := range facts.PathInvalidations {
		if !placeholderPathInArity(fact.Path, arity) {
			continue
		}
		out = append(out, signature.PathInvalidation{Path: fact.Path})
	}
	return out
}

func operationalFrozenTables(facts callboundary.NormalReturnFacts, arity int) []signature.FrozenTable {
	if arity <= 0 || len(facts.FrozenTables) == 0 {
		return nil
	}
	out := make([]signature.FrozenTable, 0, len(facts.FrozenTables))
	for _, fact := range facts.FrozenTables {
		if !placeholderPathInArity(fact.Target, arity) {
			continue
		}
		out = append(out, signature.FrozenTable{Target: fact.Target})
	}
	return out
}

func operationalEscapeEvents(facts callboundary.NormalReturnFacts, arity int) []signature.EscapeEvent {
	if arity <= 0 || len(facts.EscapeEvents) == 0 {
		return nil
	}
	out := make([]signature.EscapeEvent, 0, len(facts.EscapeEvents))
	for _, fact := range facts.EscapeEvents {
		if !placeholderPathInArity(fact.Target, arity) {
			continue
		}
		kind, ok := operationalEscapeKind(fact.Kind)
		if !ok {
			continue
		}
		out = append(out, signature.EscapeEvent{
			Target:    fact.Target,
			Kind:      kind,
			Recursive: fact.Recursive,
		})
	}
	return out
}

func operationalStoreRelations(facts callboundary.NormalReturnFacts, arity int) []signature.StoreRelation {
	if arity <= 0 || len(facts.StoreRelations) == 0 {
		return nil
	}
	out := make([]signature.StoreRelation, 0, len(facts.StoreRelations))
	for _, relation := range facts.StoreRelations {
		if !placeholderPathInArity(relation.Source, arity) || !placeholderPathInArity(relation.Into, arity) {
			continue
		}
		out = append(out, signature.StoreRelation{Source: relation.Source, Into: relation.Into})
	}
	return out
}

func placeholderPathInArity(p pathdom.Path, arity int) bool {
	idx := p.PlaceholderIndex()
	return idx >= 0 && idx < arity
}

func operationalPresence(p presence.Value) bool {
	return presence.Equal(p, presence.Present()) || presence.Equal(p, presence.Absent()) || presence.Equal(p, presence.Maybe())
}

func operationalEscapeKind(kind callboundary.EscapeEventKind) (signature.EscapeKind, bool) {
	switch kind {
	case callboundary.EscapeEventBorrow:
		return signature.EscapeBorrow, true
	case callboundary.EscapeEventRetain:
		return signature.EscapeRetain, true
	case callboundary.EscapeEventStore:
		return signature.EscapeStore, true
	case callboundary.EscapeEventSend:
		return signature.EscapeSend, true
	case callboundary.EscapeEventExport:
		return signature.EscapeExport, true
	case callboundary.EscapeEventOpaque:
		return signature.EscapeOpaque, true
	default:
		return signature.EscapeNone, false
	}
}

func normalReturnOwnershipLabels(facts callboundary.NormalReturnFacts, arity int, exactStoreSources map[int]struct{}) []effect.Label {
	if arity <= 0 {
		return nil
	}
	var out []effect.Label
	for _, event := range facts.EscapeEvents {
		param, ok := rootPlaceholderParam(event.Target, arity)
		if !ok || !event.Recursive {
			continue
		}
		switch event.Kind {
		case callboundary.EscapeEventBorrow:
			out = append(out, ownership.Borrow{Param: effect.ParamRef{Index: param}})
		case callboundary.EscapeEventRetain:
			out = append(out, ownership.Retain{Param: effect.ParamRef{Index: param}})
		case callboundary.EscapeEventStore:
			if _, exact := exactStoreSources[param]; exact {
				continue
			}
			out = append(out, ownership.Store{
				Param: effect.ParamRef{Index: param},
				Into:  effect.ParamRef{Index: -1},
			})
		case callboundary.EscapeEventSend:
			out = append(out, ownership.SendParam{Param: effect.ParamRef{Index: param}})
		case callboundary.EscapeEventExport:
			out = append(out, ownership.Export{Param: effect.ParamRef{Index: param}})
		case callboundary.EscapeEventOpaque:
			out = append(out, ownership.Opaque{Param: effect.ParamRef{Index: param}})
		}
	}
	for _, fact := range facts.FrozenTables {
		param, ok := rootPlaceholderParam(fact.Target, arity)
		if !ok {
			continue
		}
		out = append(out, ownership.Freeze{Param: effect.ParamRef{Index: param}})
	}
	return out
}

func normalReturnStoreRelationLabels(facts callboundary.NormalReturnFacts, arity int) ([]effect.Label, map[int]struct{}, map[int]struct{}) {
	if arity <= 0 || len(facts.StoreRelations) == 0 {
		return nil, nil, nil
	}
	var out []effect.Label
	sources := make(map[int]struct{})
	targets := make(map[int]struct{})
	for _, relation := range facts.StoreRelations {
		source, ok := rootPlaceholderParam(relation.Source, arity)
		if !ok {
			continue
		}
		into, ok := rootPlaceholderParam(relation.Into, arity)
		if !ok {
			continue
		}
		out = append(out, ownership.Store{
			Param: effect.ParamRef{Index: source},
			Into:  effect.ParamRef{Index: into},
		})
		sources[source] = struct{}{}
		targets[into] = struct{}{}
	}
	if len(out) == 0 {
		return nil, nil, nil
	}
	return out, sources, targets
}

func rootPlaceholderParam(p pathdom.Path, arity int) (int, bool) {
	if len(p.Segments) != 0 {
		return 0, false
	}
	idx := p.PlaceholderIndex()
	if idx < 0 || idx >= arity {
		return 0, false
	}
	return idx, true
}

func normalReturnMutationLabels(facts callboundary.NormalReturnFacts, arity int, exactStoreTargets map[int]struct{}) []effect.Label {
	if arity <= 0 {
		return nil
	}
	var out []effect.Label
	for _, fact := range facts.PathInvalidations {
		param, ok := rootPlaceholderParam(fact.Path, arity)
		if !ok {
			continue
		}
		if _, exact := exactStoreTargets[param]; exact {
			continue
		}
		out = append(out, mutation.TableMutator{
			Target: effect.ParamRef{Index: param},
			Value:  effect.ParamRef{Index: -1},
		})
	}
	return out
}

func normalReturnParamRefinementLabels(values []product.Value, arity int) []effect.Label {
	if arity <= 0 || len(values) == 0 {
		return nil
	}
	limit := arity
	if len(values) < limit {
		limit = len(values)
	}
	var out []effect.Label
	for i := range limit {
		p := product.PresenceOf(values[i])
		switch {
		case presence.Equal(p, presence.Absent()):
			out = append(out, postcondition.NormalReturnRefinement{
				Target:     effect.ParamRef{Index: i},
				Refinement: postcondition.Absent{},
			})
		case presence.Equal(p, presence.Present()):
			out = append(out, postcondition.NormalReturnRefinement{
				Target:     effect.ParamRef{Index: i},
				Refinement: postcondition.Present{},
			})
		}
	}
	return out
}

func errorReturnLabels(relations []summary.ReturnPresenceRelation, arity int) []effect.Label {
	if arity < 2 {
		return nil
	}
	byKey := make(map[returnPresenceKey]struct{}, len(relations))
	for _, relation := range relations {
		byKey[returnPresenceKeyOf(relation)] = struct{}{}
	}
	var out []effect.Label
	for _, relation := range relations {
		if !presence.Equal(relation.TriggerPresence, presence.Present()) ||
			!presence.Equal(relation.TargetPresence, presence.Absent()) {
			continue
		}
		errorIndex := relation.TriggerIndex
		valueIndex := relation.TargetIndex
		if valueIndex < 0 || errorIndex < 0 || valueIndex >= arity || errorIndex >= arity || valueIndex >= errorIndex {
			continue
		}
		complement := returnPresenceKey{
			triggerIndex:    errorIndex,
			triggerPresence: presence.Absent(),
			targetIndex:     valueIndex,
			targetPresence:  presence.Present(),
		}
		if _, ok := byKey[complement]; !ok {
			continue
		}
		out = append(out, returns.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex})
	}
	return out
}

type returnPresenceKey struct {
	triggerIndex    int
	triggerPresence presence.Value
	targetIndex     int
	targetPresence  presence.Value
}

func returnPresenceKeyOf(relation summary.ReturnPresenceRelation) returnPresenceKey {
	return returnPresenceKey{
		triggerIndex:    relation.TriggerIndex,
		triggerPresence: relation.TriggerPresence,
		targetIndex:     relation.TargetIndex,
		targetPresence:  relation.TargetPresence,
	}
}
