package exportmanifest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		if sig, ok := functionExpressionSignature(prog, root, fact.Func, name, target); ok {
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
		fn, ok := expr.(*ast.FunctionExpr)
		if ok {
			sig, ok := functionExpressionSignature(prog, root, fn, name, fact.Path)
			if ok {
				m.DefineFunctionSignature(name, sig)
				continue
			}
		}
		if sig, ok := root.ExpressionSignatureAt(point, expr); ok {
			m.DefineFunctionSignature(name, sig.Clone())
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

func functionExpressionSignature(prog program.Result, result *body.Result, fn *ast.FunctionExpr, name string, target pathdom.Path) (signature.Function, bool) {
	fnType, typed := functionSignatureType(result, fn)
	sum, summarized := functionSummary(prog, result, fn, target)
	if !typed {
		if !summarized {
			return signature.Function{}, false
		}
		if inferred, ok := inferredFunctionTypeFromSummary(result.Registry(), result, fn, sum); ok {
			return signature.Function{
				Type:               inferred,
				Effect:             functionSummaryEffect(result.Registry(), sum, inferred),
				OperationalEffects: functionSummaryOperationalEffects(result.Registry(), sum, inferred, name),
			}, true
		}
		arity := untypedFunctionParamArity(result, fn)
		sig := signature.Function{
			Effect:             functionSummaryEffectForArity(result.Registry(), sum, arity, len(sum.Returns)),
			OperationalEffects: functionSummaryOperationalEffectsForArity(result.Registry(), sum, arity, len(sum.Returns), ""),
		}
		if sig.Effect.Pure() && (sig.OperationalEffects == nil || sig.OperationalEffects.IsEmpty()) {
			return signature.Function{}, false
		}
		return sig, true
	}
	if summarized {
		fnType = functionTypeWithInferredReturns(result.Registry(), result, fnType, sum)
	}
	sig := signature.Function{Type: fnType}
	if summarized {
		sig.Effect = functionSummaryEffect(result.Registry(), sum, fnType)
		sig.OperationalEffects = functionSummaryOperationalEffects(result.Registry(), sum, fnType, name)
	}
	return sig, true
}

func inferredFunctionTypeFromSummary(reg *axis.Registry, result *body.Result, fn *ast.FunctionExpr, sum summary.Summary) (*typ.Function, bool) {
	if reg == nil || result == nil || fn == nil {
		return nil, false
	}
	slots := result.FunctionParamSlots(fn)
	builder := typ.Func().ReserveParams(len(slots))
	inferredParam := false
	for i, slot := range slots {
		if slot.Vararg {
			continue
		}
		if slot.Type != nil {
			return nil, false
		}
		t := typ.Any
		if i < len(sum.ParamObligations) {
			obligation, ok := typevalue.TypeOf(reg, sum.ParamObligations[i])
			if ok && portableInferredSignatureType(obligation) {
				t = obligation
				inferredParam = true
			}
		}
		builder.Param(slot.Name, t)
	}
	returns, inferredReturn := inferredPortableReturnTypes(reg, result, sum)
	if len(returns) != 0 {
		builder.Returns(returns...)
	}
	if !inferredParam && !inferredReturn {
		return nil, false
	}
	return builder.Build(), true
}

func functionSignatureType(result *body.Result, fn *ast.FunctionExpr) (*typ.Function, bool) {
	if !functionSignatureParamsFullyTyped(result, fn) {
		return nil, false
	}
	t, ok := functionDefinitionMemberType(result, fn)
	if !ok {
		return nil, false
	}
	fnType, ok := t.(*typ.Function)
	return fnType, ok && fnType != nil
}

func functionSignatureParamsFullyTyped(result *body.Result, fn *ast.FunctionExpr) bool {
	if result == nil || fn == nil {
		return false
	}
	for _, slot := range result.FunctionParamSlots(fn) {
		if slot.Vararg {
			continue
		}
		if slot.Type == nil && !slot.ImplicitSelf {
			return false
		}
	}
	return true
}

func functionSummary(prog program.Result, result *body.Result, fn *ast.FunctionExpr, target pathdom.Path) (summary.Summary, bool) {
	id, ok := result.FunctionSymbol(fn)
	if ok && id != 0 {
		if key, ok := prog.FunctionKey(id); ok {
			return prog.Snapshot().Read(key)
		}
	}
	if !target.IsEmpty() {
		if key, ok := prog.PathKey(target.Key()); ok {
			return prog.Snapshot().Read(key)
		}
	}
	return summary.Summary{}, false
}

func functionTypeWithInferredReturns(reg *axis.Registry, result *body.Result, fn *typ.Function, sum summary.Summary) *typ.Function {
	if reg == nil || fn == nil || len(sum.Returns) == 0 {
		return fn
	}
	returns, ok := inferredPortableReturnTypes(reg, result, sum)
	if !ok {
		return fn
	}
	if len(returns) == 0 {
		return fn
	}
	if len(fn.Returns) != 0 {
		next := append([]typ.Type(nil), fn.Returns...)
		changed := false
		for i := range next {
			if i >= len(returns) {
				break
			}
			if declaredReturnCanUsePortableSummary(next[i]) {
				next[i] = returns[i]
				changed = true
			}
		}
		if !changed {
			return fn
		}
		return typ.RebuildFunction(typ.FunctionParts{
			TypeParams: fn.TypeParams,
			Params:     fn.Params,
			Variadic:   fn.Variadic,
			Returns:    next,
		})
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: fn.TypeParams,
		Params:     fn.Params,
		Variadic:   fn.Variadic,
		Returns:    returns,
	})
}

func declaredReturnCanUsePortableSummary(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t)
}

func inferredPortableReturnTypes(reg *axis.Registry, result *body.Result, sum summary.Summary) ([]typ.Type, bool) {
	if reg == nil || len(sum.Returns) == 0 {
		return nil, false
	}
	returns := make([]typ.Type, 0, len(sum.Returns))
	for _, value := range sum.Returns {
		value = enrichManifestReturnValue(reg, result, sum, value)
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || !portableInferredSignatureType(t) {
			return nil, false
		}
		returns = append(returns, t)
	}
	return returns, true
}

func enrichManifestReturnValue(reg *axis.Registry, result *body.Result, sum summary.Summary, value product.Value) product.Value {
	if reg == nil || result == nil || len(sum.HeapTableObjects) == 0 {
		return value
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return value
	}
	object, ok := sum.HeapTableObjects[id]
	if !ok {
		return value
	}
	ks := sum.HeapKeySpace
	if ks == nil {
		ks = result.KeySpace()
	}
	if ks == nil {
		return value
	}
	builder := staticmemberwitness.NewBuilder()
	for memberKey, memberValue := range object.StaticMembers() {
		if product.Equal(reg, memberValue, product.Bottom(reg)) {
			continue
		}
		segments, ok := ks.SuffixSegmentsView(memberKey)
		if !ok {
			continue
		}
		memberType, ok := manifestStaticMemberValueType(reg, result, memberValue)
		if !ok {
			continue
		}
		builder.Add(segments, memberType)
	}
	witness, ok := builder.Build()
	if !ok {
		return value
	}
	if existing, ok := typevalue.TypeOf(reg, value); ok && existing != nil {
		if merged, ok := typetable.OverlayRecordMembers(existing, witness); ok {
			witness = merged
		}
	}
	return typevalue.WithWitness(reg, value, witness)
}

func manifestStaticMemberValueType(reg *axis.Registry, result *body.Result, value product.Value) (typ.Type, bool) {
	if result != nil {
		if fn, ok := result.FunctionValueTypeForValue(value); ok && fn != nil {
			return fn, true
		}
	}
	t, ok := typevalue.TypeOf(reg, value)
	return t, ok && t != nil
}

func portableInferredSignatureType(t typ.Type) bool {
	return t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!declaredTypeContainsBoundaryTop(t) &&
		!typ.ContainsTypeParam(t)
}

func untypedFunctionParamArity(result *body.Result, fn *ast.FunctionExpr) int {
	if result == nil || fn == nil {
		return 0
	}
	slots := result.FunctionParamSlots(fn)
	if len(slots) == 0 {
		return 0
	}
	arity := 0
	for _, slot := range slots {
		if slot.Vararg {
			continue
		}
		arity++
	}
	return arity
}

func functionSummaryEffect(reg *axis.Registry, s summary.Summary, fn *typ.Function) effect.Row {
	if fn == nil {
		return effect.Empty
	}
	return functionSummaryEffectForArity(reg, s, len(fn.Params), functionSummaryReturnArity(s, fn))
}

func functionSummaryEffectForArity(reg *axis.Registry, s summary.Summary, paramArity, returnArity int) effect.Row {
	labels := errorReturnLabels(s.ReturnPresenceRelations, returnArity)
	labels = append(labels, normalReturnParamRefinementLabels(s.NormalReturnParams, paramArity)...)
	labels = append(labels, returnParamLiteralCaseLabels(reg, s.ReturnParamLiteralCases, paramArity, returnArity)...)
	storeRelations, exactStoreSources, exactStoreTargets := normalReturnStoreRelationLabels(s.NormalReturnFacts, paramArity)
	labels = append(labels, storeRelations...)
	labels = append(labels, normalReturnOwnershipLabels(s.NormalReturnFacts, paramArity, exactStoreSources)...)
	labels = append(labels, normalReturnMutationLabels(s.NormalReturnFacts, paramArity, exactStoreTargets)...)
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
	return functionSummaryOperationalEffectsForArity(reg, s, len(fn.Params), functionSummaryReturnArity(s, fn), signatureName, fn.Returns)
}

func functionSummaryReturnArity(s summary.Summary, fn *typ.Function) int {
	if fn != nil && len(fn.Returns) != 0 {
		return len(fn.Returns)
	}
	return len(s.Returns)
}

func functionSummaryOperationalEffectsForArity(reg *axis.Registry, s summary.Summary, paramArity, returnArity int, signatureName string, returnTypes ...[]typ.Type) *signature.OperationalEffects {
	var declaredReturns []typ.Type
	if len(returnTypes) != 0 {
		declaredReturns = returnTypes[0]
	}
	presenceRefinements := operationalNormalReturnPresenceRefinements(s.NormalReturnParams, paramArity)
	for _, refinement := range operationalNormalReturnFactPresenceRefinements(s.NormalReturnFacts, paramArity, returnArity) {
		presenceRefinements = appendOperationalPresenceRefinement(presenceRefinements, refinement)
	}
	sortOperationalPresenceRefinements(presenceRefinements)
	typeRefinements := operationalNormalReturnTypeRefinements(reg, s.NormalReturnParams, paramArity)
	for _, refinement := range operationalNormalReturnFactTypeRefinements(reg, s.NormalReturnFacts, paramArity, returnArity) {
		typeRefinements = appendOperationalTypeRefinement(typeRefinements, refinement)
	}
	sortOperationalTypeRefinements(typeRefinements)
	branchProofs := operationalBranchProofs(s.NormalReturnFacts, paramArity, returnArity)
	sortOperationalBranchProofs(branchProofs)
	var returnPresenceRelations []signature.ReturnPresenceRelation
	if len(declaredReturns) != 0 {
		returnPresenceRelations = operationalReturnPresenceRelations(s.ReturnPresenceRelations, returnArity)
	}
	var allocationTemplates []signature.ReturnAllocationTemplate
	if len(declaredReturns) != 0 {
		allocationTemplates = operationalReturnAllocationTemplates(reg, s, signatureName, returnArity, declaredReturns)
	}
	out := signature.OperationalEffects{
		ReturnPresenceRelations:         returnPresenceRelations,
		NormalReturnPresenceRefinements: presenceRefinements,
		NormalReturnTypeRefinements:     typeRefinements,
		PathPresenceImplications:        operationalPathPresenceImplications(reg, s.NormalReturnFacts, paramArity, returnArity),
		PathStaticMembers:               operationalPathStaticMembers(s.NormalReturnFacts, paramArity, reg),
		PathInvalidations:               operationalPathInvalidations(s.NormalReturnFacts, paramArity),
		BranchProofs:                    branchProofs,
		DynamicIndexFacts:               operationalDynamicIndexFacts(s.NormalReturnFacts, paramArity, returnArity, reg),
		KeyMemberships:                  operationalKeyMemberships(s.NormalReturnFacts, paramArity, returnArity),
		DynamicValueKeys:                operationalDynamicValueKeys(s.NormalReturnFacts, paramArity, returnArity),
		FrozenTables:                    operationalFrozenTables(s.NormalReturnFacts, paramArity),
		EscapeEvents:                    operationalEscapeEvents(s.NormalReturnFacts, paramArity),
		StoreRelations:                  operationalStoreRelations(s.NormalReturnFacts, paramArity),
		ReturnAllocationTemplates:       allocationTemplates,
	}
	if out.IsEmpty() {
		return nil
	}
	return &out
}

func operationalReturnAllocationTemplates(reg *axis.Registry, s summary.Summary, signatureName string, returnArity int, declaredReturns []typ.Type) []signature.ReturnAllocationTemplate {
	if reg == nil || signatureName == "" || returnArity <= 0 || len(s.Returns) == 0 || len(s.HeapTableObjects) == 0 || s.HeapKeySpace == nil {
		return nil
	}
	var out []signature.ReturnAllocationTemplate
	for i, value := range s.Returns {
		if i >= returnArity {
			break
		}
		declared := declaredReturnAt(declaredReturns, i)
		if typ.IsAny(declared) || typ.IsUnknown(declared) {
			continue
		}
		id, ok := product.Get(reg, value, identity.Key).ID()
		if !ok {
			continue
		}
		template, ok := allocationTemplateForReturn(reg, s.HeapKeySpace, s.HeapTableObjects, signatureName, i, id, declared)
		if ok {
			out = append(out, template)
		}
	}
	return out
}

func allocationTemplateForReturn(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	signatureName string,
	returnIndex int,
	rootID identity.ID,
	declared typ.Type,
) (signature.ReturnAllocationTemplate, bool) {
	if _, ok := objects[rootID]; !ok {
		return signature.ReturnAllocationTemplate{}, false
	}
	projector := allocationTemplateProjector{
		reg:           reg,
		ks:            ks,
		objects:       objects,
		signatureName: signatureName,
		returnIndex:   returnIndex,
		rawToTemplate: make(map[identity.ID]signature.AllocationTemplateID),
		visiting:      make(map[signature.AllocationTemplateID]struct{}),
		emitted:       make(map[signature.AllocationTemplateID]struct{}),
	}
	rootTemplate := projector.templateID(rootID, "root")
	projector.visit(rootID, rootTemplate, "root", declared)
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
	ks            *keyspace.KeySpace
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

func (p *allocationTemplateProjector) visit(raw identity.ID, templateID signature.AllocationTemplateID, path string, declared typ.Type) {
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
	if t, ok := typevalue.TypeOf(p.reg, object.Root()); ok {
		projected.Type = allocationTemplateExportType(t, declared)
	} else if declared != nil {
		projected.Type = declared
	}
	for _, member := range sortedHeapStaticMembers(p.ks, object.StaticMembers()) {
		memberDeclared, declaredOK := allocationTemplateMemberDeclaredType(declared, member.suffix)
		if declaredOK && declaredTypeContainsBoundaryTop(memberDeclared) {
			continue
		}
		memberID, ok := product.Get(p.reg, member.value, identity.Key).ID()
		if !ok {
			continue
		}
		childPath := path + segment.FormatSegments(member.suffix)
		childTemplate, ok := p.templateRef(memberID, childPath, memberDeclared)
		if !ok {
			continue
		}
		projected.StaticMembers = append(projected.StaticMembers, signature.AllocationStaticMemberTemplate{
			Suffix: member.suffix,
			Value:  childTemplate,
		})
	}
	for _, entry := range sortedHeapDynamicEntries(p.ks, object.DynamicIndexFacts()) {
		var projectedEntry signature.AllocationDynamicEntryTemplate
		if keyID, ok := product.Get(p.reg, entry.fact.KeyValue, identity.Key).ID(); ok {
			keyPath := fmt.Sprintf("%s:dynamic:%d:key", path, entry.index)
			if keyTemplate, ok := p.templateRef(keyID, keyPath, nil); ok {
				projectedEntry.Key = keyTemplate
			}
		}
		if keyType, ok := typevalue.TypeOf(p.reg, entry.fact.KeyValue); ok {
			projectedEntry.KeyType = keyType
		}
		if valueID, ok := product.Get(p.reg, entry.fact.Value, identity.Key).ID(); ok {
			valuePath := fmt.Sprintf("%s:dynamic:%d:value", path, entry.index)
			if valueTemplate, ok := p.templateRef(valueID, valuePath, nil); ok {
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

func (p *allocationTemplateProjector) templateRef(raw identity.ID, path string, declared typ.Type) (signature.AllocationTemplateID, bool) {
	if _, ok := p.exportableObject(raw); !ok {
		return "", false
	}
	id := p.templateID(raw, path)
	p.visit(raw, id, path, declared)
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

func declaredReturnAt(returns []typ.Type, index int) typ.Type {
	if index < 0 || index >= len(returns) {
		return nil
	}
	return returns[index]
}

func allocationTemplateExportType(impl, declared typ.Type) typ.Type {
	if declared == nil {
		return impl
	}
	if declared != nil && declaredTypeContainsBoundaryTop(declared) {
		return declared
	}
	if merged, ok := allocationTemplateDeclaredEnvelope(impl, declared); ok {
		return merged
	}
	return impl
}

func allocationTemplateDeclaredEnvelope(impl, declared typ.Type) (typ.Type, bool) {
	if impl == nil || declared == nil {
		return declared, declared != nil
	}
	declaredInner := unwrap.Optional(declared)
	declaredOptional := unwrap.IsOptionalLike(declared) && declaredInner != nil && !typ.TypeEquals(declaredInner, declared)
	implInner := unwrap.Optional(impl)
	merged, ok := mergeRecordMembers(implInner, declaredInner)
	if !ok {
		return nil, false
	}
	if declaredOptional {
		return typ.MaterializeOptional(merged), true
	}
	return merged, true
}

func allocationTemplateMemberDeclaredType(declared typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	if declared == nil || len(suffix) == 0 {
		return nil, false
	}
	t, ok := luatypeprojection.ApplySegments(declared, suffix)
	if !ok || t == nil {
		return nil, false
	}
	return t, true
}

func declaredTypeContainsBoundaryTop(t typ.Type) bool {
	return refinement.ContainsBoundaryTop(t)
}

type heapStaticMember struct {
	suffix []segment.Segment
	value  product.Value
}

func sortedHeapStaticMembers(ks *keyspace.KeySpace, in map[keyspace.Key]product.Value) []heapStaticMember {
	out := make([]heapStaticMember, 0, len(in))
	for key, value := range in {
		suffix, ok := ks.SuffixSegmentsView(key)
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

func sortedHeapDynamicEntries(ks *keyspace.KeySpace, in map[dynamicindex.Key]dynamicindex.Fact) []heapDynamicEntry {
	out := make([]heapDynamicEntry, 0, len(in))
	for key, fact := range in {
		if fact.Admission == dynamicindex.AdmissionRejected {
			continue
		}
		out = append(out, heapDynamicEntry{
			key:  string(ks.Format(key.Table)) + "|" + string(key.Site),
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

func operationalNormalReturnTypeRefinements(reg *axis.Registry, values []product.Value, arity int) []signature.PathTypeRefinement {
	if reg == nil || arity <= 0 || len(values) == 0 {
		return nil
	}
	limit := arity
	if len(values) < limit {
		limit = len(values)
	}
	var out []signature.PathTypeRefinement
	for i := range limit {
		if !presence.Equal(product.PresenceOf(values[i]), presence.Present()) {
			continue
		}
		t, ok := typevalue.TypeOf(reg, values[i])
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
			continue
		}
		out = append(out, signature.PathTypeRefinement{
			Path:      pathdom.NewPlaceholder(i),
			Type:      t,
			Assertion: product.Get(reg, values[i], assertion.Key),
		})
	}
	return out
}

func operationalNormalReturnFactPresenceRefinements(facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.PathPresenceRefinement {
	if len(facts.PathRefinements) == 0 {
		return nil
	}
	var out []signature.PathPresenceRefinement
	for _, fact := range facts.PathRefinements {
		if !boundaryPathInArity(fact.Path, paramArity, returnArity) {
			continue
		}
		p := product.PresenceOf(fact.Value)
		if !operationalPresence(p) || presence.Equal(p, presence.Maybe()) {
			continue
		}
		out = appendOperationalPresenceRefinement(out, signature.PathPresenceRefinement{
			Path:     fact.Path,
			Presence: p,
		})
	}
	return out
}

func operationalNormalReturnFactTypeRefinements(reg *axis.Registry, facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.PathTypeRefinement {
	if reg == nil || len(facts.PathRefinements) == 0 {
		return nil
	}
	var out []signature.PathTypeRefinement
	for _, fact := range facts.PathRefinements {
		if !boundaryPathInArity(fact.Path, paramArity, returnArity) {
			continue
		}
		t, ok := typevalue.TypeOf(reg, fact.Value)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
			continue
		}
		out = appendOperationalTypeRefinement(out, signature.PathTypeRefinement{
			Path:      fact.Path,
			Type:      t,
			Assertion: product.Get(reg, fact.Value, assertion.Key),
		})
	}
	return out
}

func appendOperationalPresenceRefinement(out []signature.PathPresenceRefinement, next signature.PathPresenceRefinement) []signature.PathPresenceRefinement {
	for _, existing := range out {
		if existing.Path.Equal(next.Path) && presence.Equal(existing.Presence, next.Presence) {
			return out
		}
	}
	return append(out, next)
}

func appendOperationalTypeRefinement(out []signature.PathTypeRefinement, next signature.PathTypeRefinement) []signature.PathTypeRefinement {
	for _, existing := range out {
		if existing.Path.Equal(next.Path) && typ.TypeEquals(existing.Type, next.Type) && assertion.Equal(existing.Assertion, next.Assertion) {
			return out
		}
	}
	return append(out, next)
}

func sortOperationalPresenceRefinements(refinements []signature.PathPresenceRefinement) {
	sort.SliceStable(refinements, func(i, j int) bool {
		left, right := refinements[i], refinements[j]
		if left.Path.String() != right.Path.String() {
			return left.Path.String() < right.Path.String()
		}
		return left.Presence < right.Presence
	})
}

func sortOperationalTypeRefinements(refinements []signature.PathTypeRefinement) {
	sort.SliceStable(refinements, func(i, j int) bool {
		left, right := refinements[i], refinements[j]
		if left.Path.String() != right.Path.String() {
			return left.Path.String() < right.Path.String()
		}
		if leftType, rightType := fmt.Sprint(left.Type), fmt.Sprint(right.Type); leftType != rightType {
			return leftType < rightType
		}
		return left.Assertion.String() < right.Assertion.String()
	})
}

func sortOperationalBranchProofs(proofs []signature.BranchProof) {
	sort.SliceStable(proofs, func(i, j int) bool {
		left, right := proofs[i], proofs[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Path.String() != right.Path.String() {
			return left.Path.String() < right.Path.String()
		}
		if left.Other.String() != right.Other.String() {
			return left.Other.String() < right.Other.String()
		}
		return left.Presence < right.Presence
	})
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

func operationalPathPresenceImplications(reg *axis.Registry, facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.PathPresenceImplication {
	if reg == nil || len(facts.PathPresenceImplications) == 0 {
		return nil
	}
	out := make([]signature.PathPresenceImplication, 0, len(facts.PathPresenceImplications))
	for _, fact := range facts.PathPresenceImplications {
		if !boundaryPathInArity(fact.Trigger, paramArity, returnArity) || !boundaryPathInArity(fact.Target, paramArity, returnArity) {
			continue
		}
		implication := signature.PathPresenceImplication{
			Trigger:         fact.Trigger,
			TriggerPresence: fact.TriggerPresence,
			HasTriggerType:  fact.HasTriggerValue,
			Target:          fact.Target,
			TargetPresence:  fact.TargetPresence,
		}
		if fact.HasTriggerValue {
			triggerType, ok := typevalue.TypeOf(reg, fact.TriggerValue)
			if !ok {
				continue
			}
			implication.TriggerType = triggerType
		}
		out = append(out, implication)
	}
	return out
}

func operationalPathInvalidations(facts callboundary.NormalReturnFacts, arity int) []signature.PathInvalidation {
	return operationalArityFacts(facts.PathInvalidations, arity,
		func(f callboundary.PathInvalidationFact) pathdom.Path { return f.Path },
		func(p pathdom.Path) signature.PathInvalidation { return signature.PathInvalidation{Path: p} })
}

func operationalBranchProofs(facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.BranchProof {
	if len(facts.BranchProofs) == 0 {
		return nil
	}
	out := make([]signature.BranchProof, 0, len(facts.BranchProofs))
	for _, proof := range facts.BranchProofs {
		if !boundaryPathInArity(proof.Path, paramArity, returnArity) {
			continue
		}
		switch proof.Kind {
		case pathevidence.BranchProofPathPresence:
			out = append(out, signature.BranchProof{
				Kind:     signature.BranchProofPathPresence,
				Path:     proof.Path,
				Presence: proof.Presence,
			})
		case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
			if !boundaryPathInArity(proof.Other, paramArity, returnArity) {
				continue
			}
			kind, ok := signatureBranchProofKind(proof.Kind)
			if !ok {
				continue
			}
			out = append(out, signature.BranchProof{
				Kind:  kind,
				Path:  proof.Path,
				Other: proof.Other,
			})
		}
	}
	return out
}

func signatureBranchProofKind(kind pathevidence.BranchProofKind) (signature.BranchProofKind, bool) {
	switch kind {
	case pathevidence.BranchProofPathPresence:
		return signature.BranchProofPathPresence, true
	case pathevidence.BranchProofPathEqual:
		return signature.BranchProofPathEqual, true
	case pathevidence.BranchProofPathNotEqual:
		return signature.BranchProofPathNotEqual, true
	case pathevidence.BranchProofIndexInRange:
		return signature.BranchProofIndexInRange, true
	default:
		return 0, false
	}
}

func operationalDynamicIndexFacts(facts callboundary.NormalReturnFacts, paramArity, returnArity int, reg *axis.Registry) []signature.DynamicIndexFact {
	if reg == nil || len(facts.DynamicIndexFacts) == 0 {
		return nil
	}
	domain := dynamicindex.Domain(reg)
	out := make([]signature.DynamicIndexFact, 0, len(facts.DynamicIndexFacts))
	for _, fact := range facts.DynamicIndexFacts {
		if fact.Site == "" || !boundaryPathInArity(fact.Table, paramArity, returnArity) {
			continue
		}
		if domain.Equal(fact.Value, dynamicindex.Bottom(reg)) ||
			domain.Equal(fact.Value, dynamicindex.Top()) ||
			fact.Value.Admission == dynamicindex.AdmissionRejected ||
			!operationalPresence(fact.Value.KeyPresence) {
			continue
		}
		key, ok := operationalDynamicIndexOperand(reg, fact.KeyPath, fact.Value.KeyValue, paramArity)
		if !ok {
			continue
		}
		value, ok := operationalDynamicIndexOperand(reg, fact.ValuePath, fact.Value.Value, paramArity)
		if !ok {
			continue
		}
		admission, ok := operationalDynamicIndexAdmission(fact.Value.Admission)
		if !ok {
			continue
		}
		out = append(out, signature.DynamicIndexFact{
			Table:       fact.Table,
			Site:        string(fact.Site),
			KeyPresence: fact.Value.KeyPresence,
			Key:         key,
			Value:       value,
			Admission:   admission,
		})
	}
	return out
}

func operationalDynamicIndexOperand(reg *axis.Registry, p pathdom.Path, value product.Value, arity int) (signature.DynamicIndexOperand, bool) {
	var out signature.DynamicIndexOperand
	if !p.IsEmpty() && placeholderPathInArity(p, arity) {
		out.Path = p
	}
	if t, ok := typevalue.TypeOf(reg, value); ok && portableDynamicIndexType(t) {
		out.Type = t
	}
	return out, !out.Path.IsEmpty() || out.Type != nil
}

func portableDynamicIndexType(t typ.Type) bool {
	return t != nil && t.Kind() != kind.TypeParam && t.Kind() != kind.Ref && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func operationalDynamicIndexAdmission(admission dynamicindex.Admission) (signature.DynamicIndexAdmission, bool) {
	switch admission {
	case dynamicindex.AdmissionAdmitted:
		return signature.DynamicIndexAdmissionAdmitted, true
	case dynamicindex.AdmissionUnknown:
		return signature.DynamicIndexAdmissionUnknown, true
	default:
		return "", false
	}
}

func operationalKeyMemberships(facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.KeyMembership {
	if len(facts.KeyMemberships) == 0 {
		return nil
	}
	out := make([]signature.KeyMembership, 0, len(facts.KeyMemberships))
	for _, fact := range facts.KeyMemberships {
		if !boundaryPathInArity(fact.Key, paramArity, returnArity) ||
			!boundaryPathInArity(fact.Table, paramArity, returnArity) {
			continue
		}
		out = append(out, signature.KeyMembership{
			Key:   fact.Key,
			Table: fact.Table,
		})
	}
	return out
}

func operationalDynamicValueKeys(facts callboundary.NormalReturnFacts, paramArity, returnArity int) []signature.DynamicValueKeyMembership {
	if len(facts.DynamicValueKeys) == 0 {
		return nil
	}
	out := make([]signature.DynamicValueKeyMembership, 0, len(facts.DynamicValueKeys))
	for _, fact := range facts.DynamicValueKeys {
		if fact.Site == "" ||
			!boundaryPathInArity(fact.Container, paramArity, returnArity) ||
			!boundaryPathInArity(fact.Table, paramArity, returnArity) {
			continue
		}
		out = append(out, signature.DynamicValueKeyMembership{
			Container: fact.Container,
			Site:      string(fact.Site),
			Table:     fact.Table,
		})
	}
	return out
}

func operationalFrozenTables(facts callboundary.NormalReturnFacts, arity int) []signature.FrozenTable {
	return operationalArityFacts(facts.FrozenTables, arity,
		func(f callboundary.FrozenTableFact) pathdom.Path { return f.Target },
		func(p pathdom.Path) signature.FrozenTable { return signature.FrozenTable{Target: p} })
}

// operationalArityFacts projects facts whose path lies within arity into output
// signature entries built by build.
func operationalArityFacts[F any, T any](facts []F, arity int, pathOf func(F) pathdom.Path, build func(pathdom.Path) T) []T {
	if arity <= 0 || len(facts) == 0 {
		return nil
	}
	out := make([]T, 0, len(facts))
	for _, fact := range facts {
		p := pathOf(fact)
		if !placeholderPathInArity(p, arity) {
			continue
		}
		out = append(out, build(p))
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

func boundaryPathInArity(p pathdom.Path, paramArity, returnArity int) bool {
	if p.IsPlaceholder() {
		idx := p.PlaceholderIndex()
		return idx >= 0 && idx < paramArity
	}
	if idx, ok := returnSlotPathIndex(p); ok {
		return idx >= 0 && idx < returnArity
	}
	return false
}

func returnSlotPathIndex(p pathdom.Path) (int, bool) {
	if p.Symbol != 0 || !strings.HasPrefix(p.Root, "ret[") || !strings.HasSuffix(p.Root, "]") {
		return 0, false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(p.Root, "ret["), "]")
	index, err := strconv.Atoi(body)
	if err != nil || index < 0 || p.Root != "ret["+strconv.Itoa(index)+"]" {
		return 0, false
	}
	return index, true
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

func returnParamLiteralCaseLabels(reg *axis.Registry, cases []summary.ReturnParamLiteralCase, paramArity, returnArity int) []effect.Label {
	if reg == nil || paramArity <= 0 || returnArity <= 0 || len(cases) == 0 {
		return nil
	}
	var out []effect.Label
	for _, c := range cases {
		if c.ParamIndex < 0 || c.ParamIndex >= paramArity ||
			c.ReturnIndex < 0 || c.ReturnIndex >= returnArity ||
			c.When == nil {
			continue
		}
		then, ok := typevalue.TypeOf(reg, c.Value)
		if !ok || !portableInferredSignatureType(then) {
			continue
		}
		proj, ok := projectionFromSegments(c.ParamSuffix)
		if !ok {
			continue
		}
		out = append(out, returns.Return{
			ReturnIndex: c.ReturnIndex,
			Transform: returns.ConditionalType{
				Source:     effect.ParamRef{Index: c.ParamIndex},
				Projection: proj,
				When:       c.When,
				Then:       then,
			},
		})
	}
	return out
}

func projectionFromSegments(segments []segment.Segment) (projection.Projection, bool) {
	if len(segments) == 0 {
		return projection.Projection{}, true
	}
	steps := make([]projection.Step, 0, len(segments))
	for _, seg := range segments {
		if seg.Kind != segment.SegmentField || seg.Name == "" {
			return projection.Projection{}, false
		}
		steps = append(steps, projection.Field(seg.Name))
	}
	return projection.Projection{Steps: steps}, true
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
