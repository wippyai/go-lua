package readmodel

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/contract"
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// Reader projects solved body boundary values into typed diagnostic read data.
type Reader struct {
	result     *body.Result
	typeValues *typevalue.Cache
}

type SourceSpan = readapi.SourceSpan
type CallSite = readapi.CallSite
type CallArgument = readapi.CallArgument
type CallArgumentMismatch = readapi.CallArgumentMismatch
type CallArgumentCheck = readapi.CallArgumentCheck
type OptionalAssignmentTarget = readapi.OptionalAssignmentTarget

const (
	CallArgumentMismatchMayBeNil = readapi.CallArgumentMismatchMayBeNil
)

// callArgumentMismatchSubjectPlan projects object-literal facts into pure
// readmodel candidates. Public readmodel owns selecting the report subject.
func (r Reader) callArgumentMismatchSubjectPlan(point cfg.Point, arg CallArgument, want typ.Type) (readapi.CallArgumentMismatchSubjectPlan, bool) {
	if r.result == nil || want == nil {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok || arg.Index < 0 {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	source, ok := site.ArgumentSourceAt(arg.Index)
	if !ok || !source.HasExpr {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	lit, ok := r.result.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	plan := readapi.CallArgumentMismatchSubjectPlan{
		Argument: arg,
		Expected: want,
	}
	for _, entry := range lit.Entries() {
		suffix := entry.Suffix()
		expected, ok := luatypeprojection.ExpectedTypeAtSegments(want, suffix.Segments)
		if !ok || expected == nil {
			continue
		}
		value, ok := r.objectEntryValue(point, entry)
		if !ok {
			continue
		}
		got, _ := r.ValueTypeWithPresence(value)
		if arg.FunctionType != nil {
			got = arg.FunctionType
		}
		plan.Candidates = append(plan.Candidates, readapi.CallArgumentMismatchCandidate{
			Argument: CallArgument{
				Index:                arg.Index,
				Value:                value,
				ValueHash:            r.ValueHash(value),
				TypeWithPresence:     got,
				UntrustedTopOrigin:   r.ValueHasUntrustedTopOrigin(value),
				CallerOwnedParameter: arg.CallerOwnedParameter,
				FunctionType:         arg.FunctionType,
				Span:                 sourceSpanFromFactflow(entry.ValueSpan()),
				Label:                readapi.CallArgumentMemberLabel(arg.Index, suffix.Segments, entry.ValueLabel()),
			},
			Expected:    expected,
			LabelSuffix: readapi.CallArgumentExpectedLabelSuffix(suffix.Segments),
			Admissible:  r.ValueProofAdmissible(value, expected),
		})
	}
	if field, ok := luatypeprojection.MissingRequiredRecordField(want, func(name string) bool {
		for _, entry := range lit.Entries() {
			suffix := entry.Suffix()
			if len(suffix.Segments) != 1 {
				continue
			}
			seg := suffix.Segments[0]
			if seg.Kind == segment.SegmentField && seg.Name == name {
				return true
			}
		}
		return false
	}); ok {
		plan.MissingRequiredField = field
	}
	return plan, true
}

// checkCallArgument returns the complete solved proof result for one argument
// against one expected type. It is a concrete readmodel helper; public
// obligation producers receive the resulting check on CallArgumentReport.
func (r Reader) checkCallArgument(point cfg.Point, arg CallArgument, want typ.Type, expectedLabel string, expectedSpan SourceSpan) CallArgumentCheck {
	if candidate, ok := r.admissibleCallArgumentProofCandidate(arg, want); ok {
		arg = candidate
	}
	var subjectPlan *readapi.CallArgumentMismatchSubjectPlan
	if plan, ok := r.callArgumentMismatchSubjectPlan(point, arg, want); ok {
		subjectPlan = &plan
	}
	plan := readapi.CallArgumentCheckPlan{
		Argument:            arg,
		Expected:            want,
		ExpectedLabel:       expectedLabel,
		ExpectedSpan:        expectedSpan,
		ValueAdmissible:     r.ValueProofAdmissible(arg.Value, want),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(arg.Value, want),
		IsSubtype:           r.IsSubtype,
		SubjectPlan:         subjectPlan,
	}
	return readapi.PlanCallArgumentCheck(plan)
}

type CallGenericInferenceConflict = readapi.CallGenericInferenceConflict
type CallGenericInferenceContribution = readapi.CallGenericInferenceContribution
type CallArgumentReport = readapi.CallArgumentReport
type CallArgumentObligation = readapi.CallArgumentObligation
type CallArityReport = readapi.CallArityReport
type CallCalleeReport = readapi.CallCalleeReport
type CallContractSource = readapi.CallContractSource
type Assignment = readapi.Assignment
type AssignmentCheck = readapi.AssignmentCheck

const (
	CallContractSourceLocalFunction     = readapi.CallContractSourceLocalFunction
	CallContractSourceImportedSignature = readapi.CallContractSourceImportedSignature
	CallContractSourceFunctionValue     = readapi.CallContractSourceFunctionValue
	CallContractSourceMemberFunction    = readapi.CallContractSourceMemberFunction
)

type callContract struct {
	Contract                    contract.Contract
	Source                      CallContractSource
	GenericConstraintViolations []callcontract.ArgumentConstraintViolation
	GenericInferenceConflicts   []CallGenericInferenceConflict
}

type callParamObligation struct {
	Index  int
	Type   typ.Type
	Origin readapi.CallArgumentObligationOrigin
}

func New(result *body.Result) Reader {
	return Reader{result: result, typeValues: result.TypeValues()}
}

// callPoints returns call-site points in the solved graph's deterministic RPO
// order.
func (r Reader) callPoints() []cfg.Point {
	if r.result == nil || r.result.Graph() == nil {
		return nil
	}
	return append([]cfg.Point(nil), r.result.Graph().RPO()...)
}

// ForEachCall visits assembled solved call records in deterministic RPO order.
func (r Reader) ForEachCall(visit func(CallSite) bool) bool {
	if visit == nil {
		return false
	}
	visited := false
	for _, point := range r.callPoints() {
		if !r.result.PointReachable(point) {
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok {
			continue
		}
		var args []CallArgument
		r.forEachCallArgument(point, func(arg CallArgument) bool {
			args = append(args, arg)
			return true
		})
		contract, hasContract := r.callContractAt(point)
		paramObligations := r.callParamObligationsAt(point)
		call := CallSite{
			Point:      point,
			CallSpan:   sourceSpanFromFactflow(site.CallSpan()),
			CalleeSpan: sourceSpanFromFactflow(site.CalleeSpan()),
			Reports:    r.callArgumentReports(point, contract, hasContract, args, paramObligations),
			Arity:      r.callArityReport(site, contract, hasContract),
			Callee:     r.callCalleeReport(point, site),
		}
		visited = true
		if !visit(call) {
			return true
		}
	}
	return visited
}

// ForEachAssignment visits annotated local assignments in deterministic RPO
// order. It intentionally exposes only solved values and target contracts; the
// public readmodel owns report planning and obligation passes own judgment
// emission.
func (r Reader) ForEachAssignment(visit func(Assignment) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !r.forEachLocalAssignment(point, fact, visit, &visited) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !r.forEachOrdinaryAssignment(point, fact, visit, &visited) {
				return true
			}
		}
	}
	return visited
}

// ForEachOptionalAssignmentTarget visits writes whose container may be nil.
func (r Reader) ForEachOptionalAssignmentTarget(visit func(OptionalAssignmentTarget) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.result.OrdinaryAssignment(point)
		if !ok {
			continue
		}
		target, ok := r.optionalAssignmentTarget(point, fact)
		if !ok {
			continue
		}
		visited = true
		if !visit(target) {
			return true
		}
	}
	return visited
}

func (r Reader) forEachLocalAssignment(point cfg.Point, fact semantics.LocalAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Type == nil || fact.Expr == nil {
		return true
	}
	expected, ok := r.result.TypeResolver().Type(fact.Type)
	if !ok || !readapi.ObligationTypeReportable(expected) {
		return true
	}
	if entry, ok := r.assignmentObjectLiteralEntry(point, fact, expected); ok {
		*visited = true
		return visit(entry)
	}
	value, ok := r.localAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	t, _ := r.ValueTypeWithPresence(value)
	assignment := Assignment{
		Point:              point,
		TargetLabel:        fact.Name,
		SourceLabel:        assignmentSourceLabel(fact.Expr),
		TargetKey:          assignmentTargetKey(fact),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		ExpectedLabel:      assignmentExpectedLabel(fact.Type),
		SourceSpan:         sourceSpanFromAST(ast.SpanOf(fact.Expr)),
		DeclarationSpan:    sourceSpanFromAST(ast.SpanOf(fact.Type)),
		NilableAccesses:    r.assignmentNilableAccessEvidence(point, fact.Expr),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
	}
	missingField, missingFieldOK := assignmentMissingRequired(point, r, fact, expected)
	var missingFieldType typ.Type
	if missingFieldOK {
		missingFieldType, _ = luatypeprojection.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: missingField}})
	}
	assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
		Assignment:               assignment,
		ValueAdmissible:          r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch:      r.ValueWitnessProvenMismatch(value, expected),
		MissingRequiredField:     missingField,
		MissingRequiredFieldType: missingFieldType,
		IsSubtype:                r.IsSubtype,
	})
	*visited = true
	return visit(assignment)
}

func (r Reader) localAssignmentSourceValue(point cfg.Point, fact semantics.LocalAssignmentFact) (product.Value, bool) {
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Expr != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Expr); ok {
			return value, true
		}
	}
	value, ok := r.SourceValue(point, fact.Source)
	if !ok {
		if fact.Expr != nil {
			return r.result.ExpressionValueBeforeBoundary(point, fact.Expr)
		}
		return product.Value{}, false
	}
	return value, true
}

func assignmentExpectedLabel(expr ast.TypeExpr) string {
	switch e := expr.(type) {
	case *ast.TypeRefExpr:
		return strings.Join(e.Path, ".")
	case *ast.PrimitiveTypeExpr:
		return e.Name
	case *ast.OptionalTypeExpr:
		if inner := assignmentExpectedLabel(e.Inner); inner != "" {
			return inner + "?"
		}
	case *ast.GenericTypeExpr:
		base := assignmentExpectedLabel(e.Base)
		if base == "" || len(e.Args) == 0 {
			return base
		}
		args := make([]string, 0, len(e.Args))
		for _, arg := range e.Args {
			label := assignmentExpectedLabel(arg)
			if label == "" {
				return base
			}
			args = append(args, label)
		}
		return base + "<" + strings.Join(args, ", ") + ">"
	}
	return ""
}

func (r Reader) assignmentNilableAccessEvidence(point cfg.Point, expr ast.Expr) []readapi.NilableAccessEvidence {
	var out []readapi.NilableAccessEvidence
	var visit func(ast.Expr, int)
	visit = func(expr ast.Expr, depth int) {
		if depth > typ.DefaultRecursionDepth {
			return
		}
		attr, ok := expr.(*ast.AttrGetExpr)
		if !ok || attr.Object == nil || attr.Key == nil {
			return
		}
		visit(attr.Object, depth+1)
		label := assignmentSourceLabel(attr.Object)
		access := assignmentAttrKeyLabel(attr)
		if label == "" || access == "" {
			return
		}
		t, ok := r.expressionTypeBeforeBoundary(point, attr.Object)
		nilable := typ.TypeEquals(t, typ.Nil) || typevalue.TypeIncludesNil(t)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || !nilable {
			return
		}
		out = append(out, readapi.NilableAccessEvidence{
			Label:  label,
			Access: access,
			Span:   sourceSpanFromAST(ast.SpanOf(attr.Object)),
		})
	}
	visit(expr, 0)
	return out
}

func (r Reader) forEachOrdinaryAssignment(point cfg.Point, fact semantics.OrdinaryAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Target == nil || fact.Value == nil {
		return true
	}
	expected, ok := r.ordinaryAssignmentTargetType(point, fact)
	if !ok || !readapi.ObligationTypeReportable(expected) {
		return true
	}
	value, ok := r.SourceValue(point, fact.Source)
	if !ok {
		return true
	}
	t, _ := r.ValueTypeWithPresence(value)
	assignment := Assignment{
		Point:              point,
		TargetLabel:        assignmentSourceLabel(fact.Target),
		SourceLabel:        assignmentSourceLabel(fact.Value),
		TargetKey:          assignmentTargetKeyForOrdinary(point, fact),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		SourceSpan:         sourceSpanFromAST(ast.SpanOf(fact.Value)),
		DeclarationSpan:    sourceSpanFromAST(ast.SpanOf(fact.Target)),
		NilableAccesses:    r.assignmentNilableAccessEvidence(point, fact.Value),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
	}
	assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
		Assignment:          assignment,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		IsSubtype:           r.IsSubtype,
	})
	if assignment.Check.Admissible {
		return true
	}
	*visited = true
	return visit(assignment)
}

func assignmentMissingRequired(point cfg.Point, r Reader, fact semantics.LocalAssignmentFact, expected typ.Type) (string, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return "", false
	}
	return luatypeprojection.MissingRequiredRecordField(expected, func(name string) bool {
		for _, entry := range literal.Entries {
			if len(entry.Suffix.Segments) != 1 {
				continue
			}
			seg := entry.Suffix.Segments[0]
			if seg.Kind == segment.SegmentField && seg.Name == name {
				return true
			}
		}
		return false
	})
}

func (r Reader) assignmentObjectLiteral(point cfg.Point, fact semantics.LocalAssignmentFact) (semantics.ObjectLiteralFact, bool) {
	literal, ok := r.result.ObjectLiteral(fact.Source.Expr)
	if !ok {
		literal, ok = r.result.ObjectLiteral(fact.Expr)
	}
	return literal, ok
}

func (r Reader) assignmentObjectLiteralEntry(point cfg.Point, fact semantics.LocalAssignmentFact, expected typ.Type) (Assignment, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return Assignment{}, false
	}
	for _, entry := range literal.Entries {
		entryExpected, ok := luatypeprojection.ExpectedConstructorEntryType(expected, entry.Suffix.Segments)
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			continue
		}
		t, literalOK := assignmentLiteralSourceType(entry.Value)
		value, valueOK := r.SourceValue(point, entry.Source)
		if !literalOK && !valueOK {
			continue
		}
		if !literalOK {
			entryValue := value
			if loweredSource, ok := sourcebridge.ValueSourceFromASTSource(entry.Source); ok {
				if before, beforeOK := r.result.SourceValueBeforeBoundary(point, loweredSource); beforeOK {
					entryValue = before
				}
			}
			var ok bool
			t, ok = luasourcevalue.ObjectLiteralEntryType(r.result.Registry(), r.typeValues, entryValue)
			if !ok {
				continue
			}
		}
		if t == nil || subtype.IsSubtype(t, entryExpected) {
			continue
		}
		targetLabel := fact.Name + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = assignmentSourceLabel(entry.Value)
		}
		if sourceLabel == "" {
			sourceLabel = targetLabel
		}
		assignment := Assignment{
			Point:              point,
			TargetLabel:        targetLabel,
			SourceLabel:        sourceLabel,
			TargetKey:          assignmentTargetKey(fact) + ":" + segment.FormatSegments(entry.Suffix.Segments),
			Value:              value,
			ValueHash:          assignmentValueHash(r, value, valueOK),
			TypeWithPresence:   t,
			Expected:           entryExpected,
			SourceSpan:         sourceSpanFromSemantic(entry.ValueSpan),
			DeclarationSpan:    sourceSpanFromAST(ast.SpanOf(fact.Type)),
			UntrustedTopOrigin: valueOK && r.ValueHasUntrustedTopOrigin(value),
		}
		assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
			Assignment:          assignment,
			ValueAdmissible:     false,
			ValueProvenMismatch: true,
		})
		return assignment, true
	}
	return Assignment{}, false
}

func assignmentTargetKey(fact semantics.LocalAssignmentFact) string {
	if fact.HasSymbol && fact.Symbol != 0 {
		return "sym:" + strconv.FormatUint(uint64(fact.Symbol), 10)
	}
	return "local:" + fact.Name + ":" + strconv.Itoa(fact.Index)
}

func assignmentTargetKeyForOrdinary(point cfg.Point, fact semantics.OrdinaryAssignmentFact) string {
	if fact.HasPath && !fact.Path.IsEmpty() {
		return "path:" + fact.Path.String()
	}
	return "ordinary:" + strconv.Itoa(int(point)) + ":" + strconv.Itoa(fact.Index)
}

func (r Reader) ordinaryAssignmentTargetType(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	if fact.HasPath && !fact.Path.IsEmpty() {
		if declared, ok := r.declaredWritePathType(fact.Path); ok {
			return declared, true
		}
		if declared, ok := r.dominatingDeclarationSourceWritePathType(point, fact.Path); ok {
			return declared, true
		}
	}
	attr, ok := assignmentTargetAttrExpr(fact.Target)
	if !ok || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.expressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		t, ok := access.Field(container, name)
		return r.ordinaryWritableTargetType(point, fact.Target, t), ok
	}
	if name := ast.KeyName(attr.Key); name != "" {
		t, ok := access.Field(container, name)
		return r.ordinaryWritableTargetType(point, fact.Target, t), ok
	}
	keyType, ok := r.expressionTypeBeforeBoundary(point, attr.Key)
	if !ok {
		keyType, ok = assignmentLiteralSourceType(attr.Key)
	}
	if !ok || keyType == nil {
		return nil, false
	}
	t, ok := access.WritableIndex(container, keyType)
	return r.ordinaryWritableTargetType(point, fact.Target, t), ok
}

func (r Reader) optionalAssignmentTarget(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (OptionalAssignmentTarget, bool) {
	container, ok := optionalAssignmentContainerExpr(fact.Target)
	if !ok || container == nil {
		return OptionalAssignmentTarget{}, false
	}
	containerType, ok := r.expressionTypeBeforeBoundary(point, container)
	if !ok || containerType == nil ||
		typ.IsAny(containerType) ||
		typ.IsUnknown(containerType) ||
		typ.IsNever(containerType) ||
		!typevalue.ProjectionHasNil(containerType) {
		return OptionalAssignmentTarget{}, false
	}
	return OptionalAssignmentTarget{
		Point:          point,
		ContainerLabel: assignmentSourceLabel(container),
		TargetLabel:    assignmentSourceLabel(fact.Target),
		TargetKey:      assignmentTargetKeyForOrdinary(point, fact) + ":optional-target",
		ContainerType:  containerType,
		ContainerSpan:  sourceSpanFromAST(ast.SpanOf(container)),
		TargetSpan:     sourceSpanFromAST(ast.SpanOf(fact.Target)),
	}, true
}

func optionalAssignmentContainerExpr(target ast.Expr) (ast.Expr, bool) {
	attr, ok := assignmentTargetAttrExpr(target)
	if !ok || attr.Object == nil {
		return nil, false
	}
	object := attr.Object
	for {
		next, ok := object.(*ast.AttrGetExpr)
		if !ok || next.KeySyntax != ast.AttrKeyIndex || next.Object == nil {
			return object, true
		}
		object = next.Object
	}
}

func (r Reader) ordinaryWritableTargetType(point cfg.Point, target ast.Expr, current typ.Type) typ.Type {
	if current == nil {
		return nil
	}
	if r.result != nil && target != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, target); ok {
			if family, familyOK := r.FullVariantOriginType(value); familyOK && family != nil && r.IsSubtype(current, family) {
				return family
			}
		}
	}
	if base, ok := literal.FamilyBase(current); ok && base != nil {
		return base
	}
	return current
}

func (r Reader) declaredPathType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 {
		return nil, false
	}
	declared, ok := r.symbolDeclaredType(p.Symbol)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplySegments(declared, p.Segments)
}

func (r Reader) declaredWritePathType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 {
		return nil, false
	}
	declared, ok := r.symbolDeclaredType(p.Symbol)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplyWriteSegments(declared, p.Segments)
}

func (r Reader) dominatingDeclarationSourcePathType(point cfg.Point, p path.Path) (typ.Type, bool) {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return nil, false
	}
	sourcePath, ok := r.result.ExpressionRefPath(declaration.Source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil, false
	}
	return r.declaredPathType(sourcePath.AppendSegments(p.Segments))
}

func (r Reader) dominatingDeclarationSourceWritePathType(point cfg.Point, p path.Path) (typ.Type, bool) {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return nil, false
	}
	sourcePath, ok := r.result.ExpressionRefPath(declaration.Source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil, false
	}
	return r.declaredWritePathType(sourcePath.AppendSegments(p.Segments))
}

func (r Reader) expressionTypeBeforeBoundary(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	value, ok := r.result.ExpressionValueBeforeBoundary(point, expr)
	if ok {
		if t, typeOK := r.ValueTypeWithPresence(value); typeOK && t != nil {
			return t, true
		}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		if value, valueOK := r.result.CallExprResultValue(call, 0); valueOK {
			if t, typeOK := r.ValueTypeWithPresence(value); typeOK && t != nil {
				return t, true
			}
		}
	}
	if p, pathOK := r.result.ExpressionPath(expr); pathOK && !p.IsEmpty() && p.Symbol != 0 {
		if declared, declaredOK := r.symbolDeclaredType(p.Symbol); declaredOK {
			if len(p.Segments) == 0 {
				return declared, true
			}
			return luatypeprojection.ApplySegments(declared, p.Segments)
		}
	}
	return assignmentLiteralSourceType(expr)
}

func assignmentTargetAttrExpr(expr ast.Expr) (*ast.AttrGetExpr, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return e, true
	case *ast.CastExpr:
		return assignmentTargetAttrExpr(e.Expr)
	default:
		return nil, false
	}
}

func assignmentValueHash(r Reader, value product.Value, ok bool) uint64 {
	if !ok {
		return 0
	}
	return r.ValueHash(value)
}

func assignmentSourceLabel(expr ast.Expr) string {
	return assignmentSourceLabelDepth(expr, 0)
}

func assignmentSourceLabelDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := assignmentSourceLabelDepth(e.Object, depth+1)
		key := assignmentAttrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return assignmentCallLabelDepth(e, depth+1)
	case *ast.CastExpr:
		return assignmentSourceLabelDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return assignmentSourceLabelDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func assignmentLiteralSourceType(expr ast.Expr) (typ.Type, bool) {
	return valueexpr.LiteralType(expr)
}

func assignmentCallLabelDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := assignmentSourceLabelDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := assignmentSourceLabelDepth(expr.Func, depth+1)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

func assignmentAttrKeyLabel(expr *ast.AttrGetExpr) string {
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		if name := ast.KeyName(expr.Key); name != "" {
			return "." + name
		}
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			return "[" + strconv.Quote(key.Value) + "]"
		case *ast.NumberExpr:
			return "[" + key.Value + "]"
		case *ast.IdentExpr:
			return "[" + key.Value + "]"
		}
	}
	if name := ast.KeyName(expr.Key); name != "" {
		return "." + name
	}
	return ""
}

func (r Reader) callArgumentReports(point cfg.Point, contract callContract, hasContract bool, args []CallArgument, params []callParamObligation) []CallArgumentReport {
	plan := readapi.CallArgumentReportPlan{
		Args:             args,
		GenericConflicts: contract.GenericInferenceConflicts,
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			check := r.checkCallArgument(point, arg, obligation.Type, obligation.ExpectedLabel, obligation.ExpectedSpan)
			check.ExpectedOrigin = obligation.Origin
			return check
		},
	}
	if hasContract {
		for _, violation := range contract.GenericConstraintViolations {
			if violation.Constraint == nil {
				continue
			}
			plan.GenericConstraints = append(plan.GenericConstraints, readapi.IndexedCallArgumentObligation{
				Index: violation.Index,
				Obligation: CallArgumentObligation{
					Type:          violation.Constraint,
					ExpectedLabel: contract.Source.ParameterLabel(violation.Index),
					ExpectedSpan:  contract.Source.ParameterSpan(violation.Index),
				},
			})
		}
		for _, arg := range args {
			param, ok := contract.Contract.ParamAt(arg.Index)
			if !ok || !param.Explicit || param.Type == nil {
				continue
			}
			plan.ExplicitParams = append(plan.ExplicitParams, readapi.IndexedCallArgumentObligation{
				Index: arg.Index,
				Obligation: CallArgumentObligation{
					Type:          param.AcceptedType(),
					ExpectedLabel: contract.Source.ParameterLabel(arg.Index),
					ExpectedSpan:  contract.Source.ParameterSpan(arg.Index),
				},
			})
		}
	}
	for _, obligation := range params {
		if obligation.Type == nil {
			continue
		}
		plan.OutcomeParams = append(plan.OutcomeParams, readapi.IndexedCallArgumentObligation{
			Index: obligation.Index,
			Obligation: CallArgumentObligation{
				Type:          obligation.Type,
				ExpectedLabel: CallContractSource{}.ParameterLabel(obligation.Index),
				Origin:        obligation.Origin,
			},
		})
	}
	return readapi.PlanCallArgumentReports(plan)
}

func (r Reader) callArityReport(site factflow.CallSite, contract callContract, hasContract bool) CallArityReport {
	if !hasContract {
		return CallArityReport{}
	}
	actual := site.ArgumentSourceCount()
	required := contract.Contract.RequiredArity()
	fixed := contract.Contract.ParamCount()
	return readapi.PlanCallArityReport(readapi.CallArityReportPlan{
		HasContract:    true,
		CallableName:   contract.Source.Name,
		ActualCount:    actual,
		RequiredCount:  required,
		FixedCount:     fixed,
		HasVararg:      contract.Contract.HasVararg(),
		CallSpan:       sourceSpanFromFactflow(site.CallSpan()),
		ParameterSpans: contract.Source.ParameterSpans,
		ArgumentSpans:  callArgumentSpans(site),
	})
}

func (r Reader) callCalleeReport(point cfg.Point, site factflow.CallSite) CallCalleeReport {
	if r.result == nil || site.CalleeMemberAccess() {
		return CallCalleeReport{}
	}
	p := site.CalleePathRef()
	if p.IsEmpty() {
		return CallCalleeReport{}
	}
	value, ok := r.result.PathValueAtBoundary(point, p)
	if !ok {
		return CallCalleeReport{}
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok {
		return CallCalleeReport{}
	}
	if declared, ok := r.declaredCalleeType(p); ok {
		if readapi.CallCalleeDeclaredNilOwnedByDeclaration(t, declared) {
			return CallCalleeReport{}
		}
		if readapi.CallCalleeDeclaredTypeMoreInformative(t, declared) {
			t = declared
		}
	}
	callable := false
	if _, ok := callcontract.Callable(t); ok {
		callable = true
	}
	return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
		CallableName: r.callContractSourceName(site),
		Type:         t,
		Callable:     callable,
		Span:         sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:     sourceSpanFromFactflow(site.CallSpan()),
	})
}

func (r Reader) declaredCalleeType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	return r.symbolDeclaredType(p.Symbol)
}

// callContractAt resolves the canonical callable contract for the call at
// point. It covers imported/registered signatures and local function values.
func (r Reader) callContractAt(point cfg.Point) (callContract, bool) {
	if r.result == nil {
		return callContract{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return callContract{}, false
	}
	if callContract, ok := r.declaredLocalCallContract(point, site); ok {
		return callContract, true
	}
	if fn, ok := r.result.CallSignatureType(site); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		name, _ := r.result.CallSignatureName(site)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceImportedSignature, Name: name},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	if fn, ok := r.memberCallFunctionType(point, site, site.MethodName()); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceMemberFunction, Name: r.callContractSourceName(site)},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	if fn, ok := r.result.FunctionValueTypeForCallSiteAtBoundary(point, site); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceFunctionValue, Name: r.callContractSourceName(site)},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	return callContract{}, false
}

func (r Reader) declaredLocalCallContract(point cfg.Point, site factflow.CallSite) (callContract, bool) {
	if r.result == nil {
		return callContract{}, false
	}
	fn, ok := r.result.FunctionValueTypeForCalleePath(site.View().CalleePathKey())
	if !ok || fn == nil {
		return callContract{}, false
	}
	instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
	return callContract{
		Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
		Source:                      CallContractSource{Kind: CallContractSourceLocalFunction, Name: r.callContractSourceName(site), ParameterSpans: r.localFunctionParamTypeSpans(site)},
		GenericConstraintViolations: violations,
		GenericInferenceConflicts:   conflicts,
	}, true
}

func (r Reader) localFunctionParamTypeSpans(site factflow.CallSite) []SourceSpan {
	if r.result == nil {
		return nil
	}
	spans := r.result.FunctionParamTypeSpansForTargetPath(site.CalleePathRef())
	if len(spans) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(spans))
	for i, span := range spans {
		out[i] = sourceSpanFromFactflow(span)
	}
	return out
}

func (r Reader) callContractSourceName(site factflow.CallSite) string {
	if r.result == nil {
		return ""
	}
	if methodPath, ok := site.MethodPath(); ok && !methodPath.IsEmpty() {
		display := methodPath.Clone()
		display.Root = methodPath.DisplayRoot(r.result.SymbolName)
		return display.String()
	}
	if callPath := site.CalleePathRef(); !callPath.IsEmpty() {
		display := callPath.Clone()
		display.Root = callPath.DisplayRoot(r.result.SymbolName)
		return display.String()
	}
	if name := r.result.SymbolName(site.CalleeSymbol()); name != "" {
		return name
	}
	if method := site.MethodName(); method != "" {
		return method
	}
	return ""
}

func (r Reader) instantiateCallFunctionType(point cfg.Point, site factflow.CallSite, fn *typ.Function) (*typ.Function, []callcontract.ArgumentConstraintViolation, []CallGenericInferenceConflict) {
	if r.result == nil || fn == nil || len(fn.TypeParams) == 0 {
		return fn, nil, nil
	}
	args := make([]typ.Type, site.ArgumentSourceCount())
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
			args[index] = fn
			return true
		}
		if value, ok := r.callArgumentValue(point, source); ok {
			if t, ok := r.ValueTypeWithPresence(value); ok {
				args[index] = t
			}
		}
		return true
	})
	instantiated, violations, trace := callcontract.InstantiateGenericCallWithTrace(fn, args)
	conflicts := r.genericInferenceConflicts(point, trace)
	if instantiated == nil {
		return fn, violations, conflicts
	}
	return instantiated, violations, conflicts
}

func (r Reader) genericInferenceConflicts(point cfg.Point, trace callcontract.GenericCallTrace) []CallGenericInferenceConflict {
	conflicts := callcontract.PlanGenericInferenceConflicts(trace)
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]CallGenericInferenceConflict, 0, len(conflicts))
	for _, conflict := range conflicts {
		out = append(out, CallGenericInferenceConflict{
			Index:         conflict.Index,
			ParamName:     conflict.ParamName,
			Span:          r.callArgumentSpan(point, conflict.Index),
			Contributions: r.genericInferenceReportContributions(point, conflict.Index, conflict.Contributions),
		})
	}
	return out
}

func (r Reader) genericInferenceReportContributions(point cfg.Point, index int, contributions []callcontract.InferenceContribution) []CallGenericInferenceContribution {
	if len(contributions) == 0 {
		return nil
	}
	out := make([]CallGenericInferenceContribution, 0, len(contributions))
	for _, contribution := range contributions {
		out = append(out, CallGenericInferenceContribution{
			Type: contribution.Type,
			Span: r.inferenceContributionSpan(point, index, contribution),
		})
	}
	return out
}

func (r Reader) inferenceContributionSpan(point cfg.Point, index int, contribution callcontract.InferenceContribution) SourceSpan {
	if r.result == nil {
		return SourceSpan{}
	}
	site, ok := r.result.CallSite(point)
	if !ok || index < 0 {
		return SourceSpan{}
	}
	plan := readapi.GenericInferenceContributionSpanPlan{
		Fallback: callArgumentSpan(site, index),
	}
	source, ok := site.ArgumentSourceAt(index)
	if ok && source.HasExpr {
		fact, ok := r.result.ObjectLiteralExpr(source.ExprRef)
		if ok {
			for _, entry := range fact.Entries() {
				suffix := entry.Suffix()
				plan.Candidates = append(plan.Candidates, readapi.GenericInferenceContributionSpanCandidate{
					Span:         sourceSpanFromFactflow(entry.ValueSpan()),
					SegmentDepth: len(suffix.Segments),
					Matches:      callcontract.InferenceContributionHasSegmentPrefix(contribution, suffix.Segments),
				})
			}
		}
	}
	return readapi.PlanGenericInferenceContributionSpan(plan)
}

// callParamObligationsAt returns pre-call argument obligations projected from
// the solved call outcome at point.
func (r Reader) callParamObligationsAt(point cfg.Point) []callParamObligation {
	if r.result == nil {
		return nil
	}
	outcome, ok := r.result.CallOutcomeAt(point)
	if !ok || len(outcome.ParamObligations) == 0 {
		return nil
	}
	out := make([]callParamObligation, 0, len(outcome.ParamObligations))
	for _, obligation := range outcome.ParamObligations {
		t, ok := r.ValueType(obligation.Value)
		if !ok || !readapi.CallArgumentObligationTypeReportable(t) {
			continue
		}
		out = append(out, callParamObligation{
			Index:  obligation.ParamIndex,
			Type:   t,
			Origin: r.callParamObligationOrigin(point, obligation),
		})
	}
	return out
}

func (r Reader) callParamObligationOrigin(point cfg.Point, obligation callpayload.CallParamObligation) readapi.CallArgumentObligationOrigin {
	if !obligation.Origin.HasOrigin {
		return readapi.CallArgumentObligationOrigin{}
	}
	site, _ := r.result.CallSite(point)
	return readapi.CallArgumentObligationOrigin{
		HasOrigin:         true,
		FunctionName:      r.callContractSourceName(site),
		SubjectLabel:      callArgumentLabel(obligation.Origin.ArgParam),
		ProviderLabel:     callParamObligationProviderLabel(obligation.Origin),
		MemberParamNumber: obligation.Origin.MemberParamIndex + 1,
	}
}

func callParamObligationProviderLabel(origin callpayload.CallParamObligationOrigin) string {
	var segs []segment.Segment
	if origin.ReceiverPath != "" {
		var ok bool
		segs, ok = pathaddr.RelativeStaticMemberSuffixSegments(origin.ReceiverPath)
		if !ok {
			return readapi.CallArgumentMemberLabel(origin.ReceiverParam, []segment.Segment{origin.Member}, "")
		}
	}
	segs = append(segs, origin.Member)
	return readapi.CallArgumentMemberLabel(origin.ReceiverParam, segs, "")
}

func callArgumentLabel(index int) string {
	return "argument " + strconv.Itoa(index+1)
}

func (r Reader) callReceiverTypeOrNil(point cfg.Point, site factflow.CallSite) typ.Type {
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil
	}
	return receiver
}

func callReceiverSupplied(site factflow.CallSite) bool {
	if _, ok := site.ReceiverSource(); ok {
		return true
	}
	p, ok := site.ReceiverPath()
	return ok && !p.IsEmpty()
}

func (r Reader) memberCallFunctionType(point cfg.Point, site factflow.CallSite, method string) (*typ.Function, bool) {
	if method == "" {
		return nil, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil, false
	}
	fn, status, ok := callcontract.MemberCallable(receiver, method)
	return fn, ok && status == callcontract.MemberCallOK && fn != nil
}

func (r Reader) callReceiverType(point cfg.Point, site factflow.CallSite) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if source, ok := site.ReceiverSource(); ok {
		if receiver, ok := r.receiverSourceType(point, source); ok && callcontract.ReceiverTypeUsable(receiver) {
			return receiver, true
		}
	}
	if p, ok := site.ReceiverPath(); ok && !p.IsEmpty() {
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			if receiver, ok := r.ValueTypeWithPresence(value); ok && callcontract.ReceiverTypeUsable(receiver) {
				return receiver, true
			}
		}
		if p.Symbol != 0 {
			if receiver, ok := r.symbolDeclaredType(p.Symbol); ok {
				return receiver, true
			}
		}
	}
	return nil, false
}

func (r Reader) receiverSourceType(point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	value, ok := r.callArgumentValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueTypeWithPresence(value)
}

func (r Reader) symbolDeclaredType(id symbol.ID) (typ.Type, bool) {
	if r.result == nil || id == 0 {
		return nil, false
	}
	expr, ok := r.result.SymbolTypeAnnotation(id)
	if !ok || expr == nil || r.result.TypeResolver() == nil {
		return nil, false
	}
	return r.result.TypeResolver().Type(expr)
}

// forEachCallArgument visits solved argument values for the call at point.
// It preserves the factflow argument order and reads the argument value from the
// solved pre-call state when available, falling back to the call boundary for
// specialized bodies that only materialize the argument there.
func (r Reader) forEachCallArgument(point cfg.Point, visit func(CallArgument) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return false
	}
	visited := false
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		value, ok := r.callArgumentValue(point, source)
		if !ok {
			value, ok = r.unknownArgumentValue()
		}
		if !ok {
			return true
		}
		visited = true
		return visit(r.callArgument(point, site, index, source, value))
	})
	return visited
}

func (r Reader) callArgument(point cfg.Point, site factflow.CallSite, index int, source factflow.ValueSource, value product.Value) CallArgument {
	got, _ := r.ValueTypeWithPresence(value)
	arg := CallArgument{
		Index:                index,
		Value:                value,
		ValueHash:            r.ValueHash(value),
		TypeWithPresence:     got,
		UntrustedTopOrigin:   r.ValueHasUntrustedTopOrigin(value),
		CallerOwnedParameter: r.callerOwnedParameterArgument(point, source),
		Span:                 callArgumentSpan(site, index),
		Label:                r.callArgumentLabel(site, index, source),
	}
	if candidate, ok := r.callArgumentBoundaryCandidate(point, source, value); ok {
		arg.ProofCandidateValue = candidate
		arg.ProofCandidateHash = r.ValueHash(candidate)
		arg.ProofCandidateType, _ = r.ValueTypeWithPresence(candidate)
		arg.ProofCandidateTop = r.ValueHasUntrustedTopOrigin(candidate)
		arg.HasProofCandidate = true
	}
	if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	} else if fn, ok := r.result.FunctionValueTypeForValueAtBoundary(point, value); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	}
	return arg
}

func (r Reader) admissibleCallArgumentProofCandidate(arg CallArgument, want typ.Type) (CallArgument, bool) {
	if !arg.HasProofCandidate || want == nil || !r.ValueProofAdmissible(arg.ProofCandidateValue, want) {
		return CallArgument{}, false
	}
	arg.Value = arg.ProofCandidateValue
	arg.ValueHash = arg.ProofCandidateHash
	arg.TypeWithPresence = arg.ProofCandidateType
	arg.UntrustedTopOrigin = arg.ProofCandidateTop
	arg.Mismatch = CallArgumentMismatch{}
	return arg, true
}

func (r Reader) callerOwnedParameterArgument(point cfg.Point, source factflow.ValueSource) bool {
	if r.result == nil || !source.HasExpr {
		return false
	}
	return r.callerOwnedParameterSource(point, source, nil)
}

func (r Reader) callerOwnedParameterSource(point cfg.Point, source factflow.ValueSource, active map[factflow.ExprRef]struct{}) bool {
	if r.result == nil || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	p, ok := r.result.ExpressionRefPath(source.ExprRef)
	if ok && r.callerOwnedParameterPath(p) {
		return true
	}
	if ok && r.callerOwnedParameterDeclarationSource(point, p, active) {
		return true
	}
	if active == nil {
		active = make(map[factflow.ExprRef]struct{}, 1)
	}
	if _, seen := active[source.ExprRef]; seen {
		return false
	}
	active[source.ExprRef] = struct{}{}
	op, ok := r.result.ExpressionOperationRef(source.ExprRef)
	if ok {
		if r.callerOwnedParameterSource(point, op.Left(), active) {
			return true
		}
		if op.Kind() == factflow.ExpressionOperationBinary && r.callerOwnedParameterSource(point, op.Right(), active) {
			return true
		}
	}
	if dyn, ok := r.result.DynamicIndexExpressionRef(source.ExprRef); ok {
		if tableSource, ok := dyn.TableSource(); ok && r.callerOwnedParameterSource(point, tableSource, active) {
			return true
		}
		if tablePath := dyn.TablePathRef(); !tablePath.IsEmpty() {
			if r.callerOwnedParameterPath(tablePath) || r.callerOwnedParameterDeclarationSource(point, tablePath, active) {
				return true
			}
		}
	}
	return false
}

func (r Reader) callerOwnedParameterDeclarationSource(point cfg.Point, p path.Path, active map[factflow.ExprRef]struct{}) bool {
	if p.IsEmpty() || p.Symbol == 0 || point == 0 || r.result == nil || r.result.Graph() == nil {
		return false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return false
	}
	return r.callerOwnedParameterSource(declaration.Point, declaration.Source, active)
}

func (r Reader) callerOwnedParameterPath(p path.Path) bool {
	if p.Symbol == 0 {
		return false
	}
	fn := r.result.Function()
	if fn == nil {
		return false
	}
	for _, slot := range r.result.FunctionParamSlots(fn) {
		if slot.Symbol != p.Symbol {
			continue
		}
		expr, annotated := r.result.SymbolTypeAnnotation(slot.Symbol)
		if !annotated {
			return true
		}
		t, ok := r.result.TypeResolver().Type(expr)
		return ok && refinement.ContainsFreeTypeParam(t)
	}
	return false
}

func (r Reader) contextualFunctionArgumentType(point cfg.Point, source factflow.ValueSource) (*typ.Function, bool) {
	if r.result == nil || !source.HasExpr || source.ExprRef == 0 {
		return nil, false
	}
	if _, ok := r.result.ExpressionFunction(source.ExprRef); !ok {
		return nil, false
	}
	t, ok := r.result.SignatureArgumentTypeAtBoundary(point, source)
	if !ok {
		return nil, false
	}
	fn, ok := t.(*typ.Function)
	return fn, ok && fn != nil
}

func (r Reader) callArgumentSpan(point cfg.Point, index int) SourceSpan {
	if r.result == nil {
		return SourceSpan{}
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return SourceSpan{}
	}
	return callArgumentSpan(site, index)
}

func (r Reader) unknownArgumentValue() (product.Value, bool) {
	if r.result == nil || r.result.Registry() == nil || r.typeValues == nil {
		return product.Value{}, false
	}
	return r.typeValues.FromType(r.result.Registry(), typ.Unknown), true
}

func (r Reader) callArgumentValue(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		return r.result.SourceValueAtBoundary(point, source)
	}
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueAtBoundary(point, source)
}

func (r Reader) callArgumentBoundaryCandidate(point cfg.Point, source factflow.ValueSource, current product.Value) (product.Value, bool) {
	if r.result == nil || !source.HasExpr || r.ValueHasUntrustedTopOrigin(current) {
		return product.Value{}, false
	}
	p, ok := r.result.ExpressionPathRef(source.ExprRef)
	if !ok || p.IsEmpty() {
		return product.Value{}, false
	}
	value, ok := r.result.PathValueAtBoundary(point, p)
	if !ok {
		return product.Value{}, false
	}
	if r.valueHasReadableType(value) && r.ValueHash(value) != r.ValueHash(current) {
		return value, true
	}
	return product.Value{}, false
}

func (r Reader) objectEntryValue(point cfg.Point, entry factflow.ObjectEntry) (product.Value, bool) {
	if value, ok := r.callArgumentValue(point, entry.Source()); ok {
		return value, true
	}
	return r.unknownArgumentValue()
}

// ValueHash returns the product-domain hash used for stable value references.
func (r Reader) ValueHash(value product.Value) uint64 {
	if r.result == nil {
		return 0
	}
	return product.Hash(r.result.Registry(), value)
}

func callArgumentSpan(site factflow.CallSite, index int) SourceSpan {
	span, ok := site.ArgumentSpanAt(index)
	if !ok {
		return SourceSpan{}
	}
	return sourceSpanFromFactflow(span)
}

func callArgumentSpans(site factflow.CallSite) []SourceSpan {
	count := site.ArgumentSourceCount()
	if count <= 0 {
		return nil
	}
	out := make([]SourceSpan, count)
	for i := 0; i < count; i++ {
		out[i] = callArgumentSpan(site, i)
	}
	return out
}

func (r Reader) callArgumentLabel(site factflow.CallSite, index int, source factflow.ValueSource) string {
	if index < 0 {
		return ""
	}
	if label, ok := site.ArgumentLabelAt(index); ok {
		return label
	}
	if r.result != nil && source.HasExpr {
		if p, ok := r.result.ExpressionPathRef(source.ExprRef); ok && !p.IsEmpty() {
			display := p.Clone()
			display.Root = p.DisplayRoot(r.result.SymbolName)
			return display.String()
		}
	}
	return ""
}

func sourceSpanFromFactflow(span factflow.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func sourceSpanFromSemantic(span semantics.SourceSpan) SourceSpan {
	endCol := span.EndCol
	if span.EndLine == span.StartLine && endCol <= span.StartCol {
		endCol = span.StartCol + 1
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    endCol,
	}
}

func sourceSpanFromAST(span source.Span) SourceSpan {
	endCol := span.EndCol
	if span.EndLine == span.StartLine && endCol <= span.StartCol {
		endCol = span.StartCol + 1
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    endCol,
	}
}

func (r Reader) SourceValue(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	switch source.Kind {
	case sourceprovenance.SourceExpression:
		if source.Expr == nil {
			return product.Value{}, false
		}
		if value, ok := r.result.ExpressionValueAtBoundary(point, source.Expr); ok {
			if !r.valueHasReadableType(value) {
				if p, ok := r.result.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
					if pathValue, pathOK := r.result.PathValueAtBoundary(point, p); pathOK {
						return pathValue, true
					}
				}
			}
			return value, true
		}
		_, proofTransparent := sourceprovenance.ProofInner(source.Expr)
		if !sourceprovenance.ProofInnerIsFunction(source.Expr) && proofTransparent {
			return product.Value{}, false
		}
		if value, ok := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		if p, ok := r.result.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
			return r.result.PathValueAtBoundary(point, p)
		}
		return product.Value{}, false
	case sourceprovenance.SourceCall, sourceprovenance.SourceVararg, sourceprovenance.SourceNil, sourceprovenance.SourceUnknown:
		if value, ok := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.result.SourceValueForExplanationAtBoundary(point, valueSource)
	default:
		return product.Value{}, false
	}
}

func (r Reader) valueHasReadableType(value product.Value) bool {
	t, ok := r.ValueType(value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func (r Reader) SourceType(point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	value, ok := r.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueType(value)
}

func (r Reader) ValueType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueType(value)
}

// RuntimeKindReducedType narrows declared by value's runtime-kind axis: the
// alternatives whose runtime kind the axis excludes are dropped. This reports
// the type a value actually holds on a path that a type() guard has narrowed
// (e.g. the else edge of type(v) == "number" makes a number | string value a
// string), which the declared witness alone does not reflect.
func (r Reader) RuntimeKindReducedType(value product.Value, declared typ.Type) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).RuntimeKindReducedType(value, declared)
}

func (r Reader) ValueHasUntrustedTopOrigin(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueHasUntrustedTopOrigin(value)
}

func (r Reader) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueTypeWithPresence(value)
}

// ValueHasExactIdentity reports whether value carries an exact identity lane,
// which a freshly constructed table literal holds and an imported or opaque
// value does not. It distinguishes a locally-built table whose witness type is
// complete from a value whose modeled type may omit reachable members.
func (r Reader) ValueHasExactIdentity(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueHasExactIdentity(value)
}

// ValueHasStackLocalExactIdentity reports whether value is a concrete table
// identity that remains confined to the current activation at point. Only this
// placement makes an absent field witness complete enough to type the read as
// exactly nil; escaped or shared identities can gain members through aliases or
// callbacks and must fall back to broader evidence.
func (r Reader) ValueHasStackLocalExactIdentity(point cfg.Point, value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	st, ok := r.result.StateAtBoundary(point)
	return ok && st.ValueHasStackLocalExactIdentity(r.result.Registry(), value)
}

// ValueHasLocalExclusiveExactIdentity reports whether value is a concrete table
// identity whose placement proves no external writer can materialize a missing
// slot at point. Stack values and owned-heap values are local-exclusive; shared
// or unknown placements are not.
func (r Reader) ValueHasLocalExclusiveExactIdentity(point cfg.Point, value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	st, ok := r.result.StateAtBoundary(point)
	return ok && st.ValueHasLocalExclusiveExactIdentity(r.result.Registry(), value)
}

func (r Reader) VariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).VariantOriginType(value)
}

// FullVariantOriginType returns the complete structural union of the value's
// variant-origin family, independent of any narrowing recorded on this value.
// It yields the broad declared shape the discriminated value originated from.
func (r Reader) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).FullVariantOriginType(value)
}

func (r Reader) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).RefineDeclaredType(declared, value)
}

// NarrowDeclaredByOrigin narrows declared by the value's variant-origin facts
// and presence only, without substituting any structural type witness. The
// result stays a sound supertype of the runtime value: discriminant narrowing
// removes union arms but never drops declared fields the way a partial observed
// table-literal witness can.
func (r Reader) NarrowDeclaredByOrigin(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return proof.New(r.result.Registry(), r.typeValues).NarrowDeclaredByOrigin(declared, value)
}

func (r Reader) ValueAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueAdmissible(value, want)
}

// ValueWitnessProvenMismatch reports whether a value carries a concrete
// (non-top) type witness whose presence-adjusted form is provably not a subtype
// of want. It signals a proven contradiction, not a gradual one: a value with no
// concrete witness (genuinely unknown or gradual) never qualifies, so callers can
// report a mismatch without over-reporting on unknown results.
func (r Reader) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueWitnessProvenMismatch(value, want)
}

func (r Reader) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueProofAdmissible(value, want)
}

func (r Reader) IsSubtype(sub, super typ.Type) bool {
	return proof.New(nil, r.typeValues).IsSubtype(sub, super)
}
