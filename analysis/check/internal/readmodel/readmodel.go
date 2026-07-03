package readmodel

import (
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/contract"
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/castsem"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

// Reader projects solved body boundary values into typed diagnostic read data.
type Reader struct {
	result     *body.Result
	parents    []*body.Result
	typeValues *typevalue.Cache
}

type SourceSpan = readapi.SourceSpan
type CallSite = readapi.CallSite
type CallArgument = readapi.CallArgument
type CallArgumentMismatch = readapi.CallArgumentMismatch
type CallArgumentCheck = readapi.CallArgumentCheck
type OptionalAssignmentTarget = readapi.OptionalAssignmentTarget
type UnresolvedTypeReference = readapi.UnresolvedTypeReference
type MissingMemberRead = readapi.MissingMemberRead
type ResultShapeExhaustiveness = readapi.ResultShapeExhaustiveness

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
type Return = readapi.Return
type ReturnCheck = readapi.ReturnCheck
type NonNilAssertion = readapi.NonNilAssertion
type NumericForOperand = readapi.NumericForOperand
type ConcatOperand = readapi.ConcatOperand
type FrozenTableMutation = readapi.FrozenTableMutation
type LifecycleObligation = readapi.LifecycleObligation
type UnusedLocal = readapi.UnusedLocal
type DeadAssignment = readapi.DeadAssignment
type DeadAssignmentOverwrite = readapi.DeadAssignmentOverwrite
type DeadAssignmentExit = readapi.DeadAssignmentExit
type ChannelSelectExhaustiveness = readapi.ChannelSelectExhaustiveness
type UnresolvedValueReference = readapi.UnresolvedValueReference
type RedundantConditionBranch = readapi.RedundantConditionBranch
type DominatingBranchProof = readapi.DominatingBranchProof

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

func NewWithParent(result, parent *body.Result) Reader {
	return NewWithParents(result, parent)
}

func NewWithParents(result *body.Result, parents ...*body.Result) Reader {
	r := New(result)
	for _, parent := range parents {
		if parent != nil {
			r.parents = append(r.parents, parent)
		}
	}
	return r
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

func (r Reader) CallCalleeReportAt(point cfg.Point) (CallCalleeReport, bool) {
	if r.result == nil {
		return CallCalleeReport{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return CallCalleeReport{}, false
	}
	report := r.callCalleeReport(point, site)
	return report, report.Kind != readapi.CallCalleeReportNone
}

// ForEachMissingMemberRead visits static member reads whose receiver is known
// to reject the member on the current solved path. It is the readmodel-owned
// source for the eventual missing-member-read obligation pass.
func (r Reader) ForEachMissingMemberRead(visit func(MissingMemberRead) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[*ast.AttrGetExpr]struct{})
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.walkMissingMemberReads(point, expr, seen, visit, &visited, 0, false)
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !emit(fact.Expr) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !emit(fact.Value) || !r.walkAssignmentTargetMissingMemberReads(point, fact.Target, seen, visit, &visited, 0) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
		if fact, ok := r.result.ExpressionEvaluation(point); ok {
			if !emit(fact.Expr) {
				return true
			}
		}
	}
	return visited
}

// ForEachResultShapeExhaustiveness visits case-specific field reads on
// discriminated unions where solved state has not proved the required case.
func (r Reader) ForEachResultShapeExhaustiveness(visit func(ResultShapeExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[*ast.AttrGetExpr]struct{})
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.walkResultShapeReads(point, expr, seen, visit, &visited, 0)
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !emit(fact.Expr) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !r.walkResultShapeAssignmentTargetReads(point, fact.Target, seen, visit, &visited, 0) || !emit(fact.Value) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
	}
	return visited
}

func (r Reader) walkResultShapeAssignmentTargetReads(
	point cfg.Point,
	target ast.Expr,
	seen map[*ast.AttrGetExpr]struct{},
	visit func(ResultShapeExhaustiveness) bool,
	visited *bool,
	depth int,
) bool {
	if target == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		if !r.walkResultShapeReads(point, t.Object, seen, visit, visited, depth+1) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkResultShapeReads(point, t.Key, seen, visit, visited, depth+1)
		}
	case *ast.CastExpr:
		return r.walkResultShapeAssignmentTargetReads(point, t.Expr, seen, visit, visited, depth+1)
	case *ast.NonNilAssertExpr:
		return r.walkResultShapeAssignmentTargetReads(point, t.Expr, seen, visit, visited, depth+1)
	}
	return true
}

func (r Reader) walkResultShapeReads(
	point cfg.Point,
	expr ast.Expr,
	seen map[*ast.AttrGetExpr]struct{},
	visit func(ResultShapeExhaustiveness) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	next := func(child ast.Expr) bool {
		return r.walkResultShapeReads(point, child, seen, visit, visited, depth+1)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !next(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex && !next(e.Key) {
			return false
		}
		if _, done := seen[e]; done {
			return true
		}
		seen[e] = struct{}{}
		item, ok := r.resultShapeRead(point, e)
		if !ok {
			return true
		}
		*visited = true
		return visit(item)
	case *ast.FuncCallExpr:
		if !next(e.Func) || !next(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !next(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !next(field.Key) {
				return false
			}
			if !next(field.Value) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.RelationalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.StringConcatOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return next(e.Expr)
	case *ast.UnaryNotOpExpr:
		return next(e.Expr)
	case *ast.UnaryLenOpExpr:
		return next(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return next(e.Expr)
	case *ast.CastExpr:
		return next(e.Expr)
	case *ast.NonNilAssertExpr:
		return next(e.Expr)
	}
	return true
}

// ForEachRedundantConditionBranch visits normally reachable branch conditions
// that can produce user-facing redundant-condition warnings.
func (r Reader) ForEachRedundantConditionBranch(visit func(RedundantConditionBranch) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.result.BranchCondition(point)
		if !ok || !redundantConditionUserVisibleBranchKind(fact.Kind) {
			continue
		}
		branch := RedundantConditionBranch{
			Point:         point,
			Check:         fact.Check,
			ConditionSpan: sourceSpanFromAST(ast.SpanOf(fact.Condition)),
			StatementSpan: sourceSpanFromAST(ast.SpanOf(fact.Stmt)),
		}
		visited = true
		if !visit(branch) {
			return true
		}
	}
	return visited
}

// DominatingTruthyBranchForPath returns a prior branch edge that proves the
// branch check path truthy before point, with invalidation already accounted for.
func (r Reader) DominatingTruthyBranchForPath(point cfg.Point, check branchcond.Check) (DominatingBranchProof, bool) {
	if r.result == nil || check.Path.IsEmpty() {
		return DominatingBranchProof{}, false
	}
	branch, ok := r.result.DominatingTruthyBranchForPath(point, check.Path)
	if !ok {
		return DominatingBranchProof{}, false
	}
	fact, ok := r.result.BranchCondition(branch)
	if !ok {
		return DominatingBranchProof{}, false
	}
	return DominatingBranchProof{
		Point: branch,
		Check: fact.Check,
		Span:  redundantConditionBranchSpan(fact),
	}, true
}

// DominatingBranchCheckForPath returns a prior direct branch check accepted by
// accepts whose selected edge proves something about check.Path before point.
func (r Reader) DominatingBranchCheckForPath(
	point cfg.Point,
	check branchcond.Check,
	accepts func(branchcond.Check, bool) bool,
) (DominatingBranchProof, bool) {
	if r.result == nil || check.Path.IsEmpty() || accepts == nil {
		return DominatingBranchProof{}, false
	}
	branch, edge, ok := r.result.DominatingBranchCheckForPath(point, check.Path, func(_ cfg.Point, prior branchcond.Check, cond bool) bool {
		return accepts(prior, cond)
	})
	if !ok {
		return DominatingBranchProof{}, false
	}
	fact, ok := r.result.BranchCondition(branch)
	if !ok {
		return DominatingBranchProof{}, false
	}
	return DominatingBranchProof{
		Point: branch,
		Check: fact.Check,
		Edge:  edge,
		Span:  redundantConditionBranchSpan(fact),
	}, true
}

func redundantConditionUserVisibleBranchKind(kind semantics.BranchKind) bool {
	return kind == semantics.BranchIf || kind == semantics.BranchWhile || kind == semantics.BranchRepeat
}

func redundantConditionBranchSpan(fact semantics.BranchConditionFact) SourceSpan {
	span := ast.SpanOf(fact.Condition)
	if !span.Valid() {
		span = ast.SpanOf(fact.Stmt)
	}
	return sourceSpanFromAST(span)
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

// ForEachReturn visits returned expressions with explicit declared return
// contracts in deterministic RPO order.
func (r Reader) ForEachReturn(visit func(Return) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	expectedValues := r.result.ReturnTypeValues()
	if len(expectedValues) == 0 {
		return false
	}
	expectedSpans := functionReturnTypeSpans(r.result.Function())
	visited := false
	for _, point := range r.result.ReturnPoints() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.result.ReturnFact(point)
		if !ok {
			continue
		}
		for index, expr := range fact.Exprs {
			if index >= len(expectedValues) {
				continue
			}
			source := returnSourceAt(fact, index)
			ret, ok := r.returnObjectLiteralEntry(point, index, source, expectedValues[index], expectedSpans)
			if !ok {
				ret, ok = r.returnValue(point, index, expr, source, expectedValues[index], expectedSpans)
			}
			if !ok {
				continue
			}
			visited = true
			if !visit(ret) {
				return true
			}
		}
	}
	return visited
}

// ForEachNonNilAssertion visits runtime non-nil assertions in deterministic RPO
// order, projecting each operand through solved boundary state. The obligation
// pass owns deciding whether the operand is provably nil-only.
func (r Reader) ForEachNonNilAssertion(visit func(NonNilAssertion) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	seen := make(map[nonNilAssertionKey]struct{})
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.forEachNonNilAssertionInExpr(point, expr, seen, visit, &visited, 0)
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !emit(fact.Value) || !emit(fact.Target) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
	}
	return visited
}

type nonNilAssertionKey struct {
	point cfg.Point
	expr  *ast.NonNilAssertExpr
}

func (r Reader) forEachNonNilAssertionInExpr(
	point cfg.Point,
	expr ast.Expr,
	seen map[nonNilAssertionKey]struct{},
	visit func(NonNilAssertion) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if assert, ok := expr.(*ast.NonNilAssertExpr); ok {
		r.forEachNonNilAssertionInExpr(point, assert.Expr, seen, visit, visited, depth+1)
		key := nonNilAssertionKey{point: point, expr: assert}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		item, ok := r.nonNilAssertion(point, assert)
		if !ok {
			return true
		}
		*visited = true
		return visit(item)
	}
	return r.walkNonNilAssertionExprChildren(point, expr, seen, visit, visited, depth)
}

func (r Reader) walkNonNilAssertionExprChildren(
	point cfg.Point,
	expr ast.Expr,
	seen map[nonNilAssertionKey]struct{},
	visit func(NonNilAssertion) bool,
	visited *bool,
	depth int,
) bool {
	next := func(child ast.Expr) bool {
		return r.forEachNonNilAssertionInExpr(point, child, seen, visit, visited, depth+1)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !next(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return next(e.Key)
		}
	case *ast.FuncCallExpr:
		if !next(e.Func) || !next(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !next(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !next(field.Key) {
				return false
			}
			if !next(field.Value) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.RelationalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.StringConcatOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return next(e.Expr)
	case *ast.UnaryNotOpExpr:
		return next(e.Expr)
	case *ast.UnaryLenOpExpr:
		return next(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return next(e.Expr)
	case *ast.CastExpr:
		return next(e.Expr)
	}
	return true
}

func (r Reader) nonNilAssertion(point cfg.Point, assert *ast.NonNilAssertExpr) (NonNilAssertion, bool) {
	if assert == nil || assert.Expr == nil {
		return NonNilAssertion{}, false
	}
	value, valueOK := r.result.ExpressionValueAtBoundary(point, assert.Expr)
	t, typeOK := r.nonNilAssertionOperandType(point, assert.Expr, value, valueOK)
	if !typeOK || t == nil {
		return NonNilAssertion{}, false
	}
	return NonNilAssertion{
		Point:            point,
		OperandLabel:     assignmentSourceLabel(assert.Expr),
		OperandKey:       nonNilAssertionOperandKey(point, assert.Expr),
		Value:            value,
		ValueHash:        assignmentValueHash(r, value, valueOK),
		TypeWithPresence: t,
		OperandNilOnly:   readapi.NonNilAssertionOperandNilOnly(t),
		OperandSpan:      sourceSpanFromAST(ast.SpanOf(assert.Expr)),
		AssertionSpan:    sourceSpanFromAST(ast.SpanOf(assert)),
	}, true
}

func (r Reader) nonNilAssertionOperandType(point cfg.Point, operand ast.Expr, value product.Value, valueOK bool) (typ.Type, bool) {
	if valueOK {
		if t, ok := r.ValueTypeWithPresence(value); ok && t != nil {
			return t, true
		}
	}
	return r.expressionTypeBeforeBoundary(point, operand)
}

func nonNilAssertionOperandKey(point cfg.Point, operand ast.Expr) string {
	span := ast.SpanOf(operand)
	return strconv.Itoa(int(point)) + ":" +
		strconv.Itoa(span.StartLine) + ":" +
		strconv.Itoa(span.StartCol) + ":" +
		strconv.Itoa(span.EndLine) + ":" +
		strconv.Itoa(span.EndCol)
}

// ForEachConcatOperand visits `..` operands whose solved projection still
// includes nil in deterministic RPO order.
func (r Reader) ForEachConcatOperand(visit func(ConcatOperand) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[readmodelConcatSeenKey]struct{})
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.walkConcatOperands(point, expr, concatOperandContext{}, seen, visit, &visited, 0)
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !emit(fact.Expr) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !emit(fact.Value) || !r.walkConcatAssignmentTargetReads(point, fact.Target, concatOperandContext{}, seen, visit, &visited, 0) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
	}
	return visited
}

type readmodelConcatSeenKey struct {
	expr  *ast.StringConcatOpExpr
	point cfg.Point
}

type concatOperandContext struct {
	present map[path.PathKey]struct{}
	absent  map[path.PathKey]struct{}
}

func (c concatOperandContext) withPresent(p path.Path) concatOperandContext {
	if p.IsEmpty() {
		return c
	}
	next := c.clone()
	if next.present == nil {
		next.present = make(map[path.PathKey]struct{}, 1)
	}
	delete(next.absent, p.Key())
	next.present[p.Key()] = struct{}{}
	return next
}

func (c concatOperandContext) withAbsent(p path.Path) concatOperandContext {
	if p.IsEmpty() {
		return c
	}
	next := c.clone()
	if next.absent == nil {
		next.absent = make(map[path.PathKey]struct{}, 1)
	}
	delete(next.present, p.Key())
	next.absent[p.Key()] = struct{}{}
	return next
}

func (c concatOperandContext) clone() concatOperandContext {
	if len(c.present) == 0 && len(c.absent) == 0 {
		return c
	}
	next := concatOperandContext{}
	if len(c.present) != 0 {
		next.present = make(map[path.PathKey]struct{}, len(c.present))
		for key := range c.present {
			next.present[key] = struct{}{}
		}
	}
	if len(c.absent) != 0 {
		next.absent = make(map[path.PathKey]struct{}, len(c.absent))
		for key := range c.absent {
			next.absent[key] = struct{}{}
		}
	}
	return next
}

func (c concatOperandContext) hasPresent(p path.Path) bool {
	_, ok := c.present[p.Key()]
	return ok
}

func (c concatOperandContext) hasAbsent(p path.Path) bool {
	_, ok := c.absent[p.Key()]
	return ok
}

func (r Reader) walkConcatOperands(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[readmodelConcatSeenKey]struct{},
	visit func(ConcatOperand) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if logical, ok := expr.(*ast.LogicalOpExpr); ok {
		if !r.walkConcatOperands(point, logical.Lhs, ctx, seen, visit, visited, depth+1) {
			return false
		}
		switch logical.Operator {
		case "and":
			next, reachable := r.concatExpressionEdgeContext(point, logical.Lhs, true, ctx)
			return !reachable || r.walkConcatOperands(point, logical.Rhs, next, seen, visit, visited, depth+1)
		case "or":
			next, reachable := r.concatExpressionEdgeContext(point, logical.Lhs, false, ctx)
			return !reachable || r.walkConcatOperands(point, logical.Rhs, next, seen, visit, visited, depth+1)
		default:
			return r.walkConcatOperands(point, logical.Rhs, ctx, seen, visit, visited, depth+1)
		}
	}
	if concat, ok := expr.(*ast.StringConcatOpExpr); ok {
		if !r.walkConcatExprChildren(point, expr, ctx, seen, visit, visited, depth+1) {
			return false
		}
		key := readmodelConcatSeenKey{expr: concat, point: point}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		if operand, ok := r.concatOperand(point, concat.Lhs, "left", ctx); ok {
			*visited = true
			return visit(operand)
		}
		if operand, ok := r.concatOperand(point, concat.Rhs, "right", ctx); ok {
			*visited = true
			return visit(operand)
		}
		return true
	}
	return r.walkConcatExprChildren(point, expr, ctx, seen, visit, visited, depth+1)
}

func (r Reader) walkConcatExprChildren(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[readmodelConcatSeenKey]struct{},
	visit func(ConcatOperand) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil {
		return true
	}
	walk := func(child ast.Expr) bool {
		return r.walkConcatOperands(point, child, ctx, seen, visit, visited, depth)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !walk(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return walk(e.Key)
		}
	case *ast.FuncCallExpr:
		if !walk(e.Func) || !walk(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !walk(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !walk(field.Key) {
				return false
			}
			if !walk(field.Value) {
				return false
			}
		}
	case *ast.RelationalOpExpr:
		return walk(e.Lhs) && walk(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return walk(e.Lhs) && walk(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return walk(e.Expr)
	case *ast.UnaryNotOpExpr:
		return walk(e.Expr)
	case *ast.UnaryLenOpExpr:
		return walk(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return walk(e.Expr)
	case *ast.CastExpr:
		return walk(e.Expr)
	case *ast.NonNilAssertExpr:
		return walk(e.Expr)
	}
	return true
}

func (r Reader) walkConcatAssignmentTargetReads(
	point cfg.Point,
	target ast.Expr,
	ctx concatOperandContext,
	seen map[readmodelConcatSeenKey]struct{},
	visit func(ConcatOperand) bool,
	visited *bool,
	depth int,
) bool {
	if target == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		if !r.walkConcatOperands(point, t.Object, ctx, seen, visit, visited, depth+1) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkConcatOperands(point, t.Key, ctx, seen, visit, visited, depth+1)
		}
	case *ast.CastExpr:
		return r.walkConcatAssignmentTargetReads(point, t.Expr, ctx, seen, visit, visited, depth+1)
	case *ast.NonNilAssertExpr:
		return r.walkConcatAssignmentTargetReads(point, t.Expr, ctx, seen, visit, visited, depth+1)
	}
	return true
}

func (r Reader) concatExpressionEdgeContext(point cfg.Point, expr ast.Expr, cond bool, ctx concatOperandContext) (concatOperandContext, bool) {
	if r.result == nil || expr == nil {
		return ctx, true
	}
	next := ctx
	for _, implied := range r.result.ExpressionImpliedChecksOnEdge(expr, cond) {
		check := implied.Check
		if check.Kind == branchcond.CheckNone {
			continue
		}
		next = concatContextWithBranchCheck(next, check, implied.Polarity)
	}
	return next, true
}

func concatContextWithBranchCheck(ctx concatOperandContext, check branchcond.Check, cond bool) concatOperandContext {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if cond {
			return ctx.withPresent(check.Path)
		}
		return ctx.withAbsent(check.Path)
	case branchcond.CheckFalsy:
		if cond {
			return ctx.withAbsent(check.Path)
		}
		return ctx.withPresent(check.Path)
	case branchcond.CheckNil:
		if cond {
			return ctx.withAbsent(check.Path)
		}
		return ctx.withPresent(check.Path)
	case branchcond.CheckNotNil:
		if cond {
			return ctx.withPresent(check.Path)
		}
		return ctx.withAbsent(check.Path)
	case branchcond.CheckTypeEqual:
		if cond && check.TypeName != "nil" && check.TypeName != "" {
			return ctx.withPresent(check.Path)
		}
		if !cond && check.TypeName == "nil" {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckTypeNot:
		if cond && check.TypeName == "nil" {
			return ctx.withPresent(check.Path)
		}
		if !cond && check.TypeName != "nil" && check.TypeName != "" {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckLiteralEqual:
		if cond && !typ.Nil.Equals(check.Literal) && !typ.False.Equals(check.Literal) {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckLiteralNot:
		if !cond && !typ.Nil.Equals(check.Literal) && !typ.False.Equals(check.Literal) {
			return ctx.withPresent(check.Path)
		}
	}
	return ctx
}

func (r Reader) concatOperand(point cfg.Point, operand ast.Expr, side string, ctx concatOperandContext) (ConcatOperand, bool) {
	if operand == nil || r.concatOperandProvenPresent(point, operand, ctx) {
		return ConcatOperand{}, false
	}
	t, ok := r.concatOperandType(point, operand)
	if !ok || !readapi.ConcatOperandNilRisk(t) {
		return ConcatOperand{}, false
	}
	if withoutNil := typetable.PresentReadonlyEntryValue(t); withoutNil != nil && !typ.IsNever(withoutNil) {
		if r.concatOperandProvenPresentBySolvedValue(point, operand) {
			return ConcatOperand{}, false
		}
	}
	return ConcatOperand{
		Point:            point,
		Side:             side,
		OperandLabel:     assignmentSourceLabel(operand),
		OperandKey:       concatOperandKey(point, operand, side),
		TypeWithPresence: t,
		OperandSpan:      sourceSpanFromAST(ast.SpanOf(operand)),
	}, true
}

func (r Reader) concatOperandType(point cfg.Point, operand ast.Expr) (typ.Type, bool) {
	current, currentOK := r.expressionTypeBeforeBoundary(point, operand)
	if declared, ok := r.concatDominatingLocalDeclaredOperandType(point, operand); ok {
		if !currentOK || typ.Nil.Equals(current) {
			return declared, true
		}
	}
	if attr, ok := operand.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex && !r.concatOperandProvenPresentBySolvedValue(point, operand) {
		if indexed, indexedOK := r.concatIndexedReadType(point, attr); indexedOK {
			return indexed, true
		}
		if currentOK && current != nil && !typevalue.ProjectionHasNil(current) {
			return normalize.Optional(current), true
		}
	}
	return current, currentOK
}

func (r Reader) concatIndexedReadType(point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.expressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	key, ok := r.expressionTypeBeforeBoundary(point, attr.Key)
	if !ok || key == nil {
		return nil, false
	}
	indexed, ok := access.RuntimeIndex(container, key)
	if !ok || indexed == nil {
		return nil, false
	}
	if typevalue.ProjectionHasNil(indexed) {
		return indexed, true
	}
	return normalize.Optional(indexed), true
}

func (r Reader) concatDominatingLocalDeclaredOperandType(point cfg.Point, operand ast.Expr) (typ.Type, bool) {
	p, ok := r.result.ExpressionPath(operand)
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	fact, _, ok := r.result.DominatingRootLocalAssignment(point, p.Symbol)
	if !ok || fact.Type == nil || r.result.TypeResolver() == nil {
		return nil, false
	}
	declared, ok := r.result.TypeResolver().Type(fact.Type)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplySegments(declared, p.Segments)
}

func (r Reader) concatOperandProvenPresent(point cfg.Point, operand ast.Expr, ctx concatOperandContext) bool {
	if r.concatOperandProvenPresentBySolvedValue(point, operand) {
		return true
	}
	p, ok := r.result.ExpressionPath(operand)
	if !ok || p.IsEmpty() {
		return false
	}
	if ctx.hasAbsent(p) {
		return false
	}
	if ctx.hasPresent(p) {
		return true
	}
	return r.result.PathProvenTruthyByDominatingBranch(point, p)
}

func (r Reader) concatOperandProvenPresentBySolvedValue(point cfg.Point, operand ast.Expr) bool {
	if attr, ok := operand.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex {
		return r.result.ExpressionReadProvenPresentBeforeBoundary(point, operand)
	}
	value, ok := r.result.ExpressionValueBeforeBoundary(point, operand)
	if !ok {
		return false
	}
	p := product.PresenceOf(value)
	if presence.Equal(p, presence.Present()) {
		return true
	}
	if presence.Equal(p, presence.Absent()) {
		return false
	}
	if t, ok := r.ValueTypeWithPresence(value); ok && typ.Nil.Equals(t) {
		return false
	}
	return false
}

func concatOperandKey(point cfg.Point, operand ast.Expr, side string) string {
	span := ast.SpanOf(operand)
	return side + ":" +
		strconv.Itoa(int(point)) + ":" +
		strconv.Itoa(span.StartLine) + ":" +
		strconv.Itoa(span.StartCol) + ":" +
		strconv.Itoa(span.EndLine) + ":" +
		strconv.Itoa(span.EndCol)
}

// ForEachNumericForOperand visits the init, limit, and explicit step operands of
// numeric-for loops in deterministic RPO order.
func (r Reader) ForEachNumericForOperand(visit func(NumericForOperand) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.result.NumericFor(point)
		if !ok || fact.Role != cfgfacts.NumericForRoleInit {
			continue
		}
		operands := []struct {
			expr ast.Expr
			role string
		}{
			{expr: fact.Init, role: "initial value"},
			{expr: fact.Limit, role: "limit"},
		}
		if fact.Step != nil {
			operands = append(operands, struct {
				expr ast.Expr
				role string
			}{expr: fact.Step, role: "step"})
		}
		for _, operand := range operands {
			item, ok := r.numericForOperand(point, operand.expr, operand.role)
			if !ok {
				continue
			}
			visited = true
			if !visit(item) {
				return true
			}
		}
	}
	return visited
}

func (r Reader) numericForOperand(point cfg.Point, expr ast.Expr, role string) (NumericForOperand, bool) {
	if expr == nil {
		return NumericForOperand{}, false
	}
	explicitTopLikeCast := numericForOperandExplicitTopLikeCast(expr)
	typeExpr := expr
	if explicitTopLikeCast {
		if cast, ok := expr.(*ast.CastExpr); ok && cast != nil && cast.Expr != nil {
			typeExpr = cast.Expr
		}
	}
	t, ok := r.expressionTypeBeforeBoundary(point, typeExpr)
	if !ok || t == nil {
		return NumericForOperand{}, false
	}
	return NumericForOperand{
		Point:               point,
		Role:                role,
		OperandLabel:        assignmentSourceLabel(expr),
		OperandKey:          numericForOperandKey(point, expr, role),
		TypeWithPresence:    t,
		OperandSpan:         sourceSpanFromAST(ast.SpanOf(expr)),
		ExplicitTopLikeCast: explicitTopLikeCast,
		DefinitelyNotNumber: readapi.NumericForDefinitelyNotNumber(t),
	}, true
}

func numericForOperandKey(point cfg.Point, expr ast.Expr, role string) string {
	span := ast.SpanOf(expr)
	return role + ":" +
		strconv.Itoa(int(point)) + ":" +
		strconv.Itoa(span.StartLine) + ":" +
		strconv.Itoa(span.StartCol) + ":" +
		strconv.Itoa(span.EndLine) + ":" +
		strconv.Itoa(span.EndCol)
}

func numericForOperandExplicitTopLikeCast(expr ast.Expr) bool {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || cast == nil || cast.Type == nil {
		return false
	}
	primitive, ok := cast.Type.(*ast.PrimitiveTypeExpr)
	if !ok || primitive == nil {
		return false
	}
	return castsem.IsAnyTarget(primitive.Name) || castsem.IsUnknownTarget(primitive.Name)
}

// ForEachFrozenTableMutation visits writes and mutating calls that target a table
// identity proved frozen at the mutation point.
func (r Reader) ForEachFrozenTableMutation(visit func(FrozenTableMutation) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if mutation, ok := r.frozenAssignmentMutation(point, fact); ok {
				visited = true
				if !visit(mutation) {
					return true
				}
			}
		}
		if mutation, ok := r.frozenCallMutation(point); ok {
			visited = true
			if !visit(mutation) {
				return true
			}
		}
	}
	return visited
}

func (r Reader) frozenAssignmentMutation(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (FrozenTableMutation, bool) {
	if fact.Target == nil || !fact.HasContainerPath || fact.ContainerPath.IsEmpty() {
		return FrozenTableMutation{}, false
	}
	tableID, ok := r.frozenMutationContainerIdentity(point, fact.ContainerPath)
	if !ok {
		return FrozenTableMutation{}, false
	}
	in, ok := r.result.StateAt(point)
	if !ok || !in.IsTableFrozen(tableID) {
		return FrozenTableMutation{}, false
	}
	frozenSpan, hasFrozenSpan := r.frozenProofSpan(point, fact.ContainerPath)
	return FrozenTableMutation{
		Point:              point,
		Kind:               readapi.FrozenTableMutationAssignment,
		ContainerLabel:     fact.ContainerPath.String(),
		ContainerKey:       string(fact.ContainerPath.Key()),
		MutationSpan:       sourceSpanFromAST(ast.SpanOf(fact.Target)),
		FreezeProofSpan:    frozenSpan,
		HasFreezeProofSpan: hasFrozenSpan,
	}, true
}

func (r Reader) frozenCallMutation(point cfg.Point) (FrozenTableMutation, bool) {
	outcome, ok := r.result.CallOutcomeAt(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	in, ok := r.result.StateAt(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	for _, target := range r.frozenCallInvalidationTargets(site, outcome) {
		tableID, ok := r.frozenMutationContainerIdentity(point, target)
		if !ok || !in.IsTableFrozen(tableID) {
			continue
		}
		frozenSpan, hasFrozenSpan := r.frozenProofSpan(point, target)
		return FrozenTableMutation{
			Point:              point,
			Kind:               readapi.FrozenTableMutationCall,
			ContainerLabel:     target.String(),
			ContainerKey:       string(target.Key()),
			MutationSpan:       sourceSpanFromFactflow(site.CallSpan()),
			FreezeProofSpan:    frozenSpan,
			HasFreezeProofSpan: hasFrozenSpan,
		}, true
	}
	return FrozenTableMutation{}, false
}

func (r Reader) frozenCallInvalidationTargets(site factflow.CallSite, outcome callpayload.CallOutcome) []path.Path {
	var out []path.Path
	appendSubstituted := func(bindings []path.Path, target path.Path) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		for _, existing := range out {
			if existing.Equal(substituted) {
				return
			}
		}
		out = append(out, substituted)
	}
	argBindings := r.callArgumentBindings(site)
	callBindings := r.callBindings(site)
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(argBindings, invalidation.Path)
	}
	for _, write := range outcome.ParamPathWrites {
		appendSubstituted(argBindings, write.Path)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path)
	}
	return out
}

func (r Reader) callArgumentBindings(site factflow.CallSite) []path.Path {
	var bindings []path.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := r.result.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func (r Reader) callBindings(site factflow.CallSite) []path.Path {
	var bindings []path.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := r.result.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func appendPathBinding(bindings []path.Path, index int, value path.Path) []path.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, path.Path{})
	}
	bindings[index] = value
	return bindings
}

func (r Reader) frozenMutationContainerIdentity(point cfg.Point, container path.Path) (identity.ID, bool) {
	reg := r.result.Registry()
	if reg == nil {
		return identity.ID{}, false
	}
	value, ok := r.result.PathValueBeforeBoundary(point, container)
	if !ok {
		return identity.ID{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	return id, ok && id != (identity.ID{})
}

func (r Reader) frozenProofSpan(stop cfg.Point, container path.Path) (SourceSpan, bool) {
	graph := r.result.Graph()
	if graph == nil || container.IsEmpty() {
		return SourceSpan{}, false
	}
	for _, point := range graph.RPO() {
		if point == stop {
			break
		}
		outcome, ok := r.result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.FrozenTables) == 0 {
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok {
			continue
		}
		bindings := r.callBindings(site)
		for _, fact := range outcome.NormalReturnFacts.FrozenTables {
			target, ok := fact.Target.Substitute(bindings)
			if !ok || !target.Equal(container) {
				continue
			}
			return sourceSpanFromFactflow(site.CallSpan()), true
		}
	}
	return SourceSpan{}, false
}

// ForEachLifecycleObligation visits typestate resources whose obligations remain
// open at function exit, with reachable lifecycle fact sites attached as
// renderer-independent evidence.
func (r Reader) ForEachLifecycleObligation(visit func(LifecycleObligation) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	exit, ok := r.result.ExitState()
	if !ok {
		return false
	}
	obligations := exit.OpenTypestateObligations()
	if len(obligations) == 0 {
		return false
	}
	trace := r.newLifecycleTrace()
	visited := false
	for _, obligation := range obligations {
		if obligation.Resource.ID == "" || obligation.Resource.Protocol == "" || obligation.Obligation.Empty() {
			continue
		}
		item := r.lifecycleObligation(obligation, trace)
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) lifecycleObligation(obligation typestate.OpenObligation, trace lifecycleTrace) LifecycleObligation {
	resource := obligation.Resource
	sites := trace.sitesForResource(resource)
	return LifecycleObligation{
		Point:    r.result.Graph().Exit(),
		Resource: resource.ID.String(),
		Protocol: string(resource.Protocol),
		Current:  string(obligation.Current),
		Finals:   lifecycleFinalStateNames(obligation.Obligation),
		Sites:    sites,
	}
}

func lifecycleFinalStateNames(obligation typestate.Obligation) []string {
	states := obligation.FinalStateList()
	if len(states) == 0 {
		return nil
	}
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, state.String())
	}
	return out
}

type lifecycleTrace struct {
	sites []lifecycleTraceSite
	graph cfg.Graph
	reach *cfg.Reachability
	idom  map[cfg.Point]cfg.Point
}

type lifecycleTraceSite struct {
	point cfg.Point
	site  readapi.LifecycleSite
}

func (r Reader) newLifecycleTrace() lifecycleTrace {
	graph := r.result.Graph()
	return lifecycleTrace{
		sites: r.collectLifecycleTraceSites(graph),
		graph: graph,
		reach: cfg.NewReachability(graph),
		idom:  dominance.ComputeImmediateDominatorInfo(graph).Map(),
	}
}

func (t lifecycleTrace) sitesForResource(resource typestate.Resource) []readapi.LifecycleSite {
	acquires := t.latestSites(resource, readapi.LifecycleSiteAcquire)
	transitions := t.latestSites(resource, readapi.LifecycleSiteTransition)
	escapes := t.latestSites(resource, readapi.LifecycleSiteEscape)
	out := make([]readapi.LifecycleSite, 0, len(acquires)+len(transitions)+len(escapes))
	out = append(out, acquires...)
	out = append(out, transitions...)
	out = append(out, escapes...)
	return out
}

func (t lifecycleTrace) latestSites(resource typestate.Resource, kind readapi.LifecycleSiteKind) []readapi.LifecycleSite {
	var selected []lifecycleTraceSite
	for _, site := range t.sites {
		if site.site.Kind == kind && site.site.Resource == resource.ID.String() && site.site.Protocol == string(resource.Protocol) {
			selected = append(selected, site)
		}
	}
	if len(selected) <= 1 || t.graph == nil {
		return lifecycleTraceSites(selected)
	}
	exit := t.graph.Exit()
	out := make([]lifecycleTraceSite, 0, len(selected))
	for i, site := range selected {
		stale := false
		for j, other := range selected {
			if i == j || site.point == other.point {
				continue
			}
			if dominance.Dominates(t.idom, other.point, exit) && t.reach.CanReach(site.point, other.point) {
				stale = true
				break
			}
		}
		if !stale {
			out = append(out, site)
		}
	}
	if len(out) == 0 {
		return lifecycleTraceSites(selected)
	}
	return lifecycleTraceSites(out)
}

func lifecycleTraceSites(sites []lifecycleTraceSite) []readapi.LifecycleSite {
	if len(sites) == 0 {
		return nil
	}
	out := make([]readapi.LifecycleSite, 0, len(sites))
	for _, site := range sites {
		out = append(out, site.site)
	}
	return out
}

func (r Reader) collectLifecycleTraceSites(graph cfg.Graph) []lifecycleTraceSite {
	if graph == nil {
		return nil
	}
	var out []lifecycleTraceSite
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		outcome, ok := r.result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok {
			continue
		}
		bindings := r.callBindings(site)
		span := sourceSpanFromFactflow(site.CallSpan())
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			if fact.Kind == callboundary.LifecycleNone || fact.Protocol == "" {
				continue
			}
			target, ok := fact.Target.Substitute(bindings)
			if !ok || target.IsEmpty() {
				continue
			}
			resource, ok := r.result.TypestateResourceAtBoundary(point, target, fact.Protocol)
			if !ok {
				continue
			}
			kind, ok := lifecycleSiteKind(fact.Kind)
			if !ok {
				continue
			}
			out = append(out, lifecycleTraceSite{
				point: point,
				site: readapi.LifecycleSite{
					Point:       point,
					Kind:        kind,
					Resource:    resource.ID.String(),
					Protocol:    string(resource.Protocol),
					From:        string(fact.From),
					To:          string(fact.To),
					TargetLabel: target.String(),
					Span:        span,
				},
			})
		}
	}
	return out
}

func lifecycleSiteKind(kind callboundary.LifecycleKind) (readapi.LifecycleSiteKind, bool) {
	switch kind {
	case callboundary.LifecycleAcquire:
		return readapi.LifecycleSiteAcquire, true
	case callboundary.LifecycleTransition:
		return readapi.LifecycleSiteTransition, true
	case callboundary.LifecycleEscape:
		return readapi.LifecycleSiteEscape, true
	default:
		return 0, false
	}
}

// ForEachUnusedLocal visits local bindings whose symbol has no reachable read.
func (r Reader) ForEachUnusedLocal(visit func(UnusedLocal) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	graph := r.result.Graph()
	readsByPoint := r.reachableSymbolReads(graph)
	visited := false
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || ignoredUnusedLocalName(fact.Name) {
			continue
		}
		if r.symbolHasReachableRead(readsByPoint, fact.Symbol) {
			continue
		}
		item := UnusedLocal{
			Point: point,
			Name:  fact.Name,
			Key:   strconv.Itoa(int(fact.Symbol)),
			Span:  sourceSpanFromAST(localNameSourceSpan(fact.Stmt, fact.Index, fact.Name)),
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

// ForEachDeadAssignment visits writes whose assigned value is discarded before
// any reachable read on every path.
func (r Reader) ForEachDeadAssignment(visit func(DeadAssignment) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	graph := r.result.Graph()
	view := r.newDeadAssignmentView(graph)
	if len(view.writes) == 0 {
		return false
	}
	bySymbol := make(map[symbol.ID][]readmodelDeadAssignmentWrite)
	for _, write := range view.writes {
		bySymbol[write.symbol] = append(bySymbol[write.symbol], write)
	}
	var items []DeadAssignment
	for _, writes := range bySymbol {
		items = append(items, view.deadAssignmentsForSymbol(writes)...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].WriteSpan.StartLine != items[j].WriteSpan.StartLine {
			return items[i].WriteSpan.StartLine < items[j].WriteSpan.StartLine
		}
		if items[i].WriteSpan.StartCol != items[j].WriteSpan.StartCol {
			return items[i].WriteSpan.StartCol < items[j].WriteSpan.StartCol
		}
		return items[i].Name < items[j].Name
	})
	for _, item := range items {
		if !visit(item) {
			return true
		}
	}
	return len(items) > 0
}

// ForEachChannelSelectExhaustiveness visits channel.select elseif chains that
// do not handle every selectable case and do not have a select default.
func (r Reader) ForEachChannelSelectExhaustiveness(visit func(ChannelSelectExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	graph := r.result.Graph()
	selects := r.channelSelectInfos(graph)
	if len(selects) == 0 {
		return false
	}
	branches := r.channelSelectBranchConditions(graph)
	if len(branches) == 0 {
		return false
	}
	cases := newReadmodelChannelSelectCaseIndex(selects)
	ifs := make([]*ast.IfStmt, 0, len(branches))
	for _, branch := range branches {
		if branch.fact.If != nil {
			ifs = append(ifs, branch.fact.If)
		}
	}
	nested := readmodelNestedElseIfStatements(ifs)
	byIf := make(map[*ast.IfStmt]readmodelChannelSelectBranch, len(branches))
	for _, branch := range branches {
		if branch.fact.If != nil {
			byIf[branch.fact.If] = branch
		}
	}
	reachability := cfg.NewReachability(graph)
	visited := false
	for _, branch := range branches {
		if branch.fact.If == nil || nested[branch.fact.If] || !readmodelHasElseIf(branch.fact.If) {
			continue
		}
		item, ok := r.channelSelectChainExhaustiveness(graph, reachability, branch.fact.If, byIf, selects, cases)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

// ForEachUnresolvedValueReference visits reachable identifier reads that remain
// implicit globals after binding and are not known type-syntax references.
func (r Reader) ForEachUnresolvedValueReference(visit func(UnresolvedValueReference) bool) bool {
	if visit == nil || r.result == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[*ast.IdentExpr]struct{})
	emit := func(point cfg.Point, expr ast.Expr) bool {
		return r.walkUnresolvedValueExpr(point, expr, seen, func(ref UnresolvedValueReference) bool {
			visited = true
			return visit(ref)
		})
	}
	emitExprs := func(point cfg.Point, exprs []ast.Expr) bool {
		for _, expr := range exprs {
			if !emit(point, expr) {
				return false
			}
		}
		return true
	}
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !emit(point, fact.Expr) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !r.walkUnresolvedValueAssignmentTarget(point, fact.Target, seen, func(ref UnresolvedValueReference) bool {
				visited = true
				return visit(ref)
			}) {
				return true
			}
			if !emit(point, fact.Value) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok {
			if !emit(point, fact.Call) {
				return true
			}
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			if !emitExprs(point, fact.Exprs) {
				return true
			}
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			if !emit(point, fact.Condition) {
				return true
			}
		}
	}
	return visited
}

// ForEachUnresolvedTypeReference visits annotation type names that did not bind
// in the lexical/module type namespace. The query deliberately reports binding
// facts, not lowered type strings, so obligation producers do not need a second
// annotation resolver.
func (r Reader) ForEachUnresolvedTypeReference(visit func(UnresolvedTypeReference) bool) bool {
	if visit == nil || r.result == nil {
		return false
	}
	scope := r.unresolvedTypeScope()
	visited := false
	seenRefs := make(map[*ast.TypeRefExpr]struct{})
	seenPrimitives := make(map[*ast.PrimitiveTypeExpr]struct{})
	emit := func(point cfg.Point, expr ast.TypeExpr) bool {
		return r.walkUnresolvedTypeExpr(point, expr, scope, seenRefs, seenPrimitives, func(ref UnresolvedTypeReference) bool {
			visited = true
			return visit(ref)
		})
	}
	emitMany := func(point cfg.Point, exprs []ast.TypeExpr) bool {
		for _, expr := range exprs {
			if !emit(point, expr) {
				return false
			}
		}
		return true
	}
	if fn := r.result.Function(); fn != nil {
		for _, param := range fn.TypeParams {
			if !emit(0, param.Constraint) {
				return true
			}
		}
		if fn.ParList != nil {
			if !emitMany(0, fn.ParList.Types) {
				return true
			}
			if !emit(0, fn.ParList.VarargType) {
				return true
			}
		}
		if !emitMany(0, fn.ReturnTypes) {
			return true
		}
	}
	graph := r.result.Graph()
	if graph == nil {
		return visited
	}
	for _, point := range graph.RPO() {
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !emit(point, fact.Type) {
				return true
			}
		}
		if fact, ok := r.result.TypeDefinition(point); ok {
			if !r.emitUnresolvedTypeDefinitionRefs(point, fact, emit) {
				return true
			}
		}
		if fact, ok := r.result.Call(point); ok && fact.Call != nil {
			if !emitMany(point, fact.Call.TypeArgs) {
				return true
			}
		}
	}
	return visited
}

type unresolvedTypeScope struct {
	known map[string]struct{}
}

func (r Reader) unresolvedTypeScope() unresolvedTypeScope {
	known := make(map[string]struct{})
	collect := func(result *body.Result) {
		if result == nil || result.Graph() == nil {
			return
		}
		for _, point := range result.Graph().RPO() {
			fact, ok := result.TypeDefinition(point)
			if !ok {
				continue
			}
			switch fact.Kind {
			case cfgfacts.TypeDefinitionAlias:
				if fact.Type != nil && fact.Type.Name != "" {
					known[fact.Type.Name] = struct{}{}
				}
			case cfgfacts.TypeDefinitionInterface:
				if fact.Interface != nil && fact.Interface.Name != "" {
					known[fact.Interface.Name] = struct{}{}
				}
			}
		}
	}
	collect(r.result)
	for _, parent := range r.parents {
		collect(parent)
	}
	return unresolvedTypeScope{known: known}
}

func (r Reader) emitUnresolvedTypeDefinitionRefs(point cfg.Point, fact cfgfacts.TypeDefinitionFact, emit func(cfg.Point, ast.TypeExpr) bool) bool {
	switch fact.Kind {
	case cfgfacts.TypeDefinitionAlias:
		if fact.Type == nil {
			return true
		}
		for _, param := range fact.Type.TypeParams {
			if !emit(point, param.Constraint) {
				return false
			}
		}
		return emit(point, fact.Type.Type)
	case cfgfacts.TypeDefinitionInterface:
		if fact.Interface == nil {
			return true
		}
		for _, ref := range fact.Interface.Extends {
			if !emit(point, ref) {
				return false
			}
		}
		for _, field := range fact.Interface.Fields {
			if !emit(point, field.Type) {
				return false
			}
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil && !emit(point, method.Type) {
				return false
			}
		}
	}
	return true
}

func (r Reader) walkUnresolvedTypeExpr(
	point cfg.Point,
	expr ast.TypeExpr,
	scope unresolvedTypeScope,
	seenRefs map[*ast.TypeRefExpr]struct{},
	seenPrimitives map[*ast.PrimitiveTypeExpr]struct{},
	visit func(UnresolvedTypeReference) bool,
) bool {
	if expr == nil {
		return true
	}
	keepGoing := true
	typeresolve.WalkTypeNameExpr(expr, func(ref *ast.TypeRefExpr) bool {
		if ref == nil || !keepGoing {
			return keepGoing
		}
		if _, ok := seenRefs[ref]; ok {
			return true
		}
		seenRefs[ref] = struct{}{}
		if r.typeRefResolved(ref, scope) {
			return true
		}
		keepGoing = visit(r.unresolvedTypeReference(point, ref, typeRefName(ref)))
		return keepGoing
	}, func(prim *ast.PrimitiveTypeExpr) bool {
		if prim == nil || !keepGoing || typ.BuiltinPrimitiveName(prim.Name) {
			return keepGoing
		}
		if _, ok := seenPrimitives[prim]; ok {
			return true
		}
		seenPrimitives[prim] = struct{}{}
		if r.primitiveTypeResolved(prim, scope) {
			return true
		}
		keepGoing = visit(r.unresolvedTypeReference(point, prim, prim.Name))
		return keepGoing
	})
	return keepGoing
}

func (r Reader) typeRefResolved(ref *ast.TypeRefExpr, scope unresolvedTypeScope) bool {
	if ref == nil || len(ref.Path) == 0 {
		return false
	}
	if len(ref.Path) != 1 {
		return r.qualifiedTypeRefResolved(ref.Path)
	}
	if _, ok := r.result.TypeRef(ref); ok {
		return true
	}
	if _, ok := scope.known[ref.Path[0]]; ok {
		return false
	}
	return true
}

func (r Reader) primitiveTypeResolved(expr *ast.PrimitiveTypeExpr, scope unresolvedTypeScope) bool {
	if expr == nil || typ.BuiltinPrimitiveName(expr.Name) {
		return true
	}
	if _, ok := r.result.PrimitiveTypeRef(expr); ok {
		return true
	}
	if _, ok := scope.known[expr.Name]; ok {
		return false
	}
	return true
}

func (r Reader) qualifiedTypeRefResolved(path []string) bool {
	if len(path) < 2 || r.result == nil {
		return false
	}
	moduleRefs := r.result.ModuleTypes()
	if modulePath, ok := r.result.RequireAliasModulePath(path[0]); ok {
		if _, resolved := moduleRefs.ResolveTypeRefWithModulePrefix(modulePath, path[1:]); resolved {
			return true
		}
	}
	_, resolved := moduleRefs.ResolveTypeRef(path)
	return resolved
}

func (r Reader) unresolvedTypeReference(point cfg.Point, node ast.PositionHolder, name string) UnresolvedTypeReference {
	if name == "" {
		name = "<missing>"
	}
	span := sourceSpanFromAST(ast.SpanOf(node))
	return UnresolvedTypeReference{
		Point: point,
		Name:  name,
		Key:   "type:" + name + ":" + strconv.Itoa(span.StartLine) + ":" + strconv.Itoa(span.StartCol),
		Span:  span,
	}
}

func typeRefName(ref *ast.TypeRefExpr) string {
	if ref == nil || len(ref.Path) == 0 {
		return "<missing>"
	}
	return strings.Join(ref.Path, ".")
}

func (r Reader) walkUnresolvedValueAssignmentTarget(point cfg.Point, target ast.Expr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReference) bool) bool {
	switch t := target.(type) {
	case nil:
		return true
	case *ast.AttrGetExpr:
		if !r.walkUnresolvedValueExpr(point, t.Object, seen, visit) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkUnresolvedValueExpr(point, t.Key, seen, visit)
		}
		return true
	case *ast.CastExpr:
		return r.walkUnresolvedValueAssignmentTarget(point, t.Expr, seen, visit)
	case *ast.NonNilAssertExpr:
		return r.walkUnresolvedValueAssignmentTarget(point, t.Expr, seen, visit)
	default:
		return r.walkUnresolvedValueExpr(point, target, seen, visit)
	}
}

func (r Reader) walkUnresolvedValueExpr(point cfg.Point, expr ast.Expr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReference) bool) bool {
	if expr == nil {
		return true
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return r.visitUnresolvedValueIdent(point, e, seen, visit)
	case *ast.AttrGetExpr:
		if !r.walkUnresolvedValueExpr(point, e.Object, seen, visit) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return r.walkUnresolvedValueExpr(point, e.Key, seen, visit)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				if !r.walkUnresolvedValueExpr(point, field.Key, seen, visit) {
					return false
				}
			}
			if !r.walkUnresolvedValueExpr(point, field.Value, seen, visit) {
				return false
			}
		}
	case *ast.FuncCallExpr:
		if !r.typeSyntaxCallee(e) {
			if !r.walkUnresolvedValueExpr(point, e.Func, seen, visit) {
				return false
			}
		}
		if !r.typeSyntaxReceiver(e) {
			if !r.walkUnresolvedValueExpr(point, e.Receiver, seen, visit) {
				return false
			}
		}
		for _, arg := range e.Args {
			if !r.walkUnresolvedValueExpr(point, arg, seen, visit) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.RelationalOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.StringConcatOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.ArithmeticOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.UnaryMinusOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryNotOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryLenOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryBNotOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.CastExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.NonNilAssertExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	}
	return true
}

func (r Reader) visitUnresolvedValueIdent(point cfg.Point, ident *ast.IdentExpr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReference) bool) bool {
	if ident == nil {
		return true
	}
	if _, ok := seen[ident]; ok {
		return true
	}
	seen[ident] = struct{}{}
	if !r.result.IsImplicitGlobalUse(ident) {
		return true
	}
	if sym, ok := r.result.SymbolOfIdent(ident); ok && r.result.IsFunctionDefinitionTarget(sym) {
		return true
	}
	if r.identifierResolvesTypeName(ident) {
		return true
	}
	span := sourceSpanFromAST(ast.SpanOf(ident))
	name := ident.Value
	return visit(UnresolvedValueReference{
		Point: point,
		Name:  name,
		Key:   "value:" + name + ":" + strconv.Itoa(span.StartLine) + ":" + strconv.Itoa(span.StartCol),
		Span:  span,
	})
}

func (r Reader) typeSyntaxCallee(call *ast.FuncCallExpr) bool {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	return ok && r.identifierResolvesTypeName(ident)
}

func (r Reader) typeSyntaxReceiver(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method == "" || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Receiver.(*ast.IdentExpr)
	return ok && r.identifierResolvesTypeName(ident)
}

func (r Reader) identifierResolvesTypeName(ident *ast.IdentExpr) bool {
	if ident == nil || ident.Value == "" || r.result == nil {
		return false
	}
	if typ.BuiltinPrimitiveName(ident.Value) {
		return true
	}
	switch ident.Value {
	case "int", "bool":
		return true
	}
	if _, ok := r.result.TypeValueRef(ident); ok {
		return true
	}
	if decl, ok := r.result.TypeRef(&ast.TypeRefExpr{Path: []string{ident.Value}}); ok && decl.ID != 0 {
		return true
	}
	return false
}

type readmodelSelectInfo struct {
	point      cfg.Point
	result     path.Path
	cases      []readmodelSelectCase
	hasDefault bool
}

type readmodelSelectCase struct {
	path path.Path
	name string
}

type readmodelChannelSelectCaseIndex map[readmodelChannelSelectCaseKey][]readmodelChannelSelectCaseMatch

type readmodelChannelSelectCaseKey struct {
	resultChannel path.PathKey
	channel       path.PathKey
}

type readmodelChannelSelectCaseMatch struct {
	selectIndex int
	caseIndex   int
}

type readmodelChannelSelectBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

func (r Reader) channelSelectInfos(graph cfg.Graph) []readmodelSelectInfo {
	var out []readmodelSelectInfo
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		call, ok := r.result.Call(point)
		if !ok || !call.HasChannelSelect || !call.ChannelSelect.ResultTarget.HasPath {
			continue
		}
		selectFact := call.ChannelSelect
		if selectFact.ResultTarget.Path.IsEmpty() || len(selectFact.Cases) == 0 {
			continue
		}
		info := readmodelSelectInfo{point: point, result: selectFact.ResultTarget.Path, hasDefault: selectFact.HasDefault}
		for _, c := range selectFact.Cases {
			if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
				continue
			}
			name := c.ChannelPath.DisplayRoot(r.result.SymbolName)
			if name == "" {
				name = c.ChannelPath.String()
			}
			info.cases = append(info.cases, readmodelSelectCase{
				path: c.ChannelPath,
				name: name,
			})
		}
		if len(info.cases) > 0 {
			out = append(out, info)
		}
	}
	return out
}

func (r Reader) channelSelectBranchConditions(graph cfg.Graph) []readmodelChannelSelectBranch {
	var out []readmodelChannelSelectBranch
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		branch, ok := r.result.BranchCondition(point)
		if !ok || branch.If == nil {
			continue
		}
		out = append(out, readmodelChannelSelectBranch{point: point, fact: branch})
	}
	return out
}

func (r Reader) channelSelectChainExhaustiveness(
	graph cfg.Graph,
	reachability *cfg.Reachability,
	head *ast.IfStmt,
	byIf map[*ast.IfStmt]readmodelChannelSelectBranch,
	selects []readmodelSelectInfo,
	cases readmodelChannelSelectCaseIndex,
) (ChannelSelectExhaustiveness, bool) {
	chain := readmodelIfElseIfChain(head)
	headBranch, ok := byIf[head]
	if !ok {
		return ChannelSelectExhaustiveness{}, false
	}
	handledBySelect := make(map[int]map[int]bool)
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok || branch.fact.Check.Kind != branchcond.CheckPathEqual {
			continue
		}
		for _, match := range cases.matchesForCheck(branch.fact.Check) {
			if match.selectIndex < 0 || match.selectIndex >= len(selects) {
				continue
			}
			if !readmodelSelectCanReachBranch(selects[match.selectIndex], headBranch.point, graph, reachability) {
				continue
			}
			handled := handledBySelect[match.selectIndex]
			if handled == nil {
				handled = make(map[int]bool)
				handledBySelect[match.selectIndex] = handled
			}
			handled[match.caseIndex] = true
		}
	}
	selected, handled, ok := readmodelBestChannelSelectCandidate(handledBySelect)
	if !ok {
		return ChannelSelectExhaustiveness{}, false
	}
	info := selects[selected]
	if info.hasDefault {
		return ChannelSelectExhaustiveness{}, false
	}
	if len(handled) >= len(info.cases) {
		return ChannelSelectExhaustiveness{}, false
	}
	var handledNames []string
	var missing []string
	for i, c := range info.cases {
		if handled[i] {
			handledNames = appendUniqueReadmodelString(handledNames, c.name)
		} else {
			missing = appendUniqueReadmodelString(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return ChannelSelectExhaustiveness{}, false
	}
	return ChannelSelectExhaustiveness{
		Point:         headBranch.point,
		Span:          sourceSpanFromAST(ast.SpanOf(head.Condition)),
		ResultChannel: info.result.Field(channelselect.ResultChannelField).String(),
		Handled:       handledNames,
		Missing:       missing,
		HasDefault:    info.hasDefault,
	}, true
}

func readmodelSelectCanReachBranch(info readmodelSelectInfo, branchPoint cfg.Point, graph cfg.Graph, reachability *cfg.Reachability) bool {
	if graph == nil {
		return true
	}
	if info.point == branchPoint {
		return true
	}
	if reachability != nil {
		return reachability.CanReach(info.point, branchPoint)
	}
	return cfg.PointCanReach(graph, info.point, branchPoint)
}

func readmodelBestChannelSelectCandidate(handledBySelect map[int]map[int]bool) (int, map[int]bool, bool) {
	selected := -1
	var selectedHandled map[int]bool
	for selectIndex, handled := range handledBySelect {
		if len(handled) == 0 {
			continue
		}
		if selected == -1 || len(handled) > len(selectedHandled) || len(handled) == len(selectedHandled) && selectIndex > selected {
			selected = selectIndex
			selectedHandled = handled
		}
	}
	return selected, selectedHandled, selected != -1
}

func newReadmodelChannelSelectCaseIndex(selects []readmodelSelectInfo) readmodelChannelSelectCaseIndex {
	out := make(readmodelChannelSelectCaseIndex)
	for selectIndex, info := range selects {
		resultChannel := info.result.Field(channelselect.ResultChannelField)
		resultKey := resultChannel.Key()
		if resultKey == "" {
			continue
		}
		for caseIndex, c := range info.cases {
			channelKey := c.path.Key()
			if channelKey == "" {
				continue
			}
			key := readmodelChannelSelectCaseKey{resultChannel: resultKey, channel: channelKey}
			out[key] = append(out[key], readmodelChannelSelectCaseMatch{
				selectIndex: selectIndex,
				caseIndex:   caseIndex,
			})
		}
	}
	return out
}

func (idx readmodelChannelSelectCaseIndex) matchesForCheck(check branchcond.Check) []readmodelChannelSelectCaseMatch {
	matches := idx[readmodelChannelSelectCaseKey{resultChannel: check.Path.Key(), channel: check.OtherPath.Key()}]
	if len(matches) == 0 {
		matches = idx[readmodelChannelSelectCaseKey{resultChannel: check.OtherPath.Key(), channel: check.Path.Key()}]
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func appendUniqueReadmodelString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func readmodelNestedElseIfStatements(stmts []*ast.IfStmt) map[*ast.IfStmt]bool {
	nested := make(map[*ast.IfStmt]bool)
	for _, stmt := range stmts {
		if child := firstElseIf(stmt); child != nil {
			nested[child] = true
		}
	}
	return nested
}

func readmodelHasElseIf(stmt *ast.IfStmt) bool {
	return firstElseIf(stmt) != nil
}

func firstElseIf(stmt *ast.IfStmt) *ast.IfStmt {
	if stmt == nil || len(stmt.Else) == 0 {
		return nil
	}
	child, _ := stmt.Else[0].(*ast.IfStmt)
	return child
}

func readmodelIfElseIfChain(head *ast.IfStmt) []*ast.IfStmt {
	var out []*ast.IfStmt
	for stmt := head; stmt != nil; stmt = firstElseIf(stmt) {
		out = append(out, stmt)
	}
	return out
}

type readmodelDeadAssignmentWrite struct {
	point  cfg.Point
	stmt   ast.Stmt
	symbol symbol.ID
	name   string
	write  SourceSpan
}

type readmodelDeadAssignmentExit struct {
	point cfg.Point
	span  SourceSpan
}

type readmodelDeadAssignmentView struct {
	reader        Reader
	graph         cfg.Graph
	writes        []readmodelDeadAssignmentWrite
	writesByPoint map[cfg.Point][]readmodelDeadAssignmentWrite
	readsByPoint  map[cfg.Point]map[symbol.ID]struct{}
	exitsByPoint  map[cfg.Point]readmodelDeadAssignmentExit
}

func (r Reader) newDeadAssignmentView(graph cfg.Graph) readmodelDeadAssignmentView {
	writes := r.deadAssignmentWrites(graph)
	view := readmodelDeadAssignmentView{
		reader:        r,
		graph:         graph,
		writes:        writes,
		writesByPoint: make(map[cfg.Point][]readmodelDeadAssignmentWrite),
		readsByPoint:  r.reachableSymbolReads(graph),
		exitsByPoint:  r.deadAssignmentExits(graph),
	}
	for _, write := range writes {
		view.writesByPoint[write.point] = append(view.writesByPoint[write.point], write)
	}
	return view
}

func (r Reader) deadAssignmentWrites(graph cfg.Graph) []readmodelDeadAssignmentWrite {
	var writes []readmodelDeadAssignmentWrite
	for _, point := range cfg.RPOReadOnly(graph) {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			write, ok := r.localDeadAssignmentWrite(point, fact)
			if ok {
				writes = append(writes, write)
			}
			continue
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			write, ok := r.ordinaryDeadAssignmentWrite(point, fact)
			if ok {
				writes = append(writes, write)
			}
		}
	}
	return writes
}

func (r Reader) localDeadAssignmentWrite(point cfg.Point, fact semantics.LocalAssignmentFact) (readmodelDeadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Expr == nil || ignoredUnusedLocalName(fact.Name) {
		return readmodelDeadAssignmentWrite{}, false
	}
	if !r.deadAssignmentSymbolKind(fact.Symbol) {
		return readmodelDeadAssignmentWrite{}, false
	}
	write := sourceSpanFromAST(localNameSourceSpan(fact.Stmt, fact.Index, fact.Name))
	if !sourceSpanValid(write) {
		return readmodelDeadAssignmentWrite{}, false
	}
	return readmodelDeadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   fact.Name,
		write:  write,
	}, true
}

func (r Reader) ordinaryDeadAssignmentWrite(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (readmodelDeadAssignmentWrite, bool) {
	if !fact.HasSymbol || fact.Value == nil {
		return readmodelDeadAssignmentWrite{}, false
	}
	ident, ok := fact.Target.(*ast.IdentExpr)
	if !ok || ignoredUnusedLocalName(ident.Value) {
		return readmodelDeadAssignmentWrite{}, false
	}
	if !r.deadAssignmentSymbolKind(fact.Symbol) {
		return readmodelDeadAssignmentWrite{}, false
	}
	write := sourceSpanFromAST(ast.SpanOf(ident))
	if !sourceSpanValid(write) {
		write = sourceSpanFromAST(ast.SpanOf(fact.Target))
	}
	if !sourceSpanValid(write) {
		return readmodelDeadAssignmentWrite{}, false
	}
	return readmodelDeadAssignmentWrite{
		point:  point,
		stmt:   fact.Stmt,
		symbol: fact.Symbol,
		name:   ident.Value,
		write:  write,
	}, true
}

func (r Reader) deadAssignmentSymbolKind(id symbol.ID) bool {
	kind, ok := r.result.SymbolKind(id)
	return ok && (kind == symbol.Local || kind == symbol.Param)
}

func (r Reader) deadAssignmentExits(graph cfg.Graph) map[cfg.Point]readmodelDeadAssignmentExit {
	out := make(map[cfg.Point]readmodelDeadAssignmentExit)
	if graph == nil {
		return out
	}
	exit := graph.Exit()
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) || point == exit {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, point)
		if len(successors) == 0 {
			continue
		}
		allExit := true
		for _, succ := range successors {
			if succ != exit {
				allExit = false
				break
			}
		}
		if !allExit {
			continue
		}
		var span SourceSpan
		if fact, ok := r.result.ReturnFact(point); ok {
			span = sourceSpanFromAST(ast.SpanOf(fact.Stmt))
		}
		out[point] = readmodelDeadAssignmentExit{point: point, span: span}
	}
	return out
}

func (v readmodelDeadAssignmentView) deadAssignmentsForSymbol(writes []readmodelDeadAssignmentWrite) []DeadAssignment {
	var out []DeadAssignment
	for _, previous := range writes {
		if readmodelAmbiguousSameStatementWrite(previous, writes) {
			continue
		}
		overwrites, exits, ok := v.firstOverwritesBeforeRead(previous, writes)
		if !ok {
			continue
		}
		item := DeadAssignment{
			Point:     previous.point,
			Name:      previous.name,
			Key:       strconv.Itoa(int(previous.symbol)),
			WriteSpan: previous.write,
		}
		for _, overwrite := range overwrites {
			item.Overwrites = append(item.Overwrites, DeadAssignmentOverwrite{Point: overwrite.point, Span: overwrite.write})
		}
		for _, exit := range exits {
			item.Exits = append(item.Exits, DeadAssignmentExit{Point: exit.point, Span: exit.span})
		}
		out = append(out, item)
	}
	return out
}

func readmodelAmbiguousSameStatementWrite(write readmodelDeadAssignmentWrite, writes []readmodelDeadAssignmentWrite) bool {
	if write.stmt == nil {
		return false
	}
	for _, other := range writes {
		if other.point != write.point && other.stmt == write.stmt {
			return true
		}
	}
	return false
}

type readmodelDeadAssignmentProof struct {
	ok         bool
	frontier   map[cfg.Point]readmodelDeadAssignmentWrite
	exitPoints map[cfg.Point]readmodelDeadAssignmentExit
}

func (v readmodelDeadAssignmentView) firstOverwritesBeforeRead(previous readmodelDeadAssignmentWrite, writes []readmodelDeadAssignmentWrite) ([]readmodelDeadAssignmentWrite, []readmodelDeadAssignmentExit, bool) {
	if v.graph == nil {
		return nil, nil, false
	}
	successors := cfg.SuccessorsReadOnly(v.graph, previous.point)
	if len(successors) == 0 {
		return nil, nil, false
	}
	memo := make(map[cfg.Point]readmodelDeadAssignmentProof)
	visiting := make(map[cfg.Point]bool)
	var walk func(cfg.Point) readmodelDeadAssignmentProof
	walk = func(point cfg.Point) readmodelDeadAssignmentProof {
		if !v.pointReachable(point) {
			return readmodelDeadAssignmentProof{ok: true}
		}
		if v.pointReadsSymbol(point, previous.symbol) {
			return readmodelDeadAssignmentProof{}
		}
		if overwrite, ok := v.pointOverwrite(point, previous.symbol, previous.point, writes); ok {
			return readmodelDeadAssignmentProof{
				ok:       true,
				frontier: map[cfg.Point]readmodelDeadAssignmentWrite{overwrite.point: overwrite},
			}
		}
		if v.pointWritesSymbol(point, previous.symbol) {
			return readmodelDeadAssignmentProof{}
		}
		if exit, ok := v.pointExit(point); ok {
			return readmodelDeadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]readmodelDeadAssignmentExit{exit.point: exit},
			}
		}
		if point == v.graph.Exit() {
			return readmodelDeadAssignmentProof{
				ok:         true,
				exitPoints: map[cfg.Point]readmodelDeadAssignmentExit{point: {point: point}},
			}
		}
		if cached, ok := memo[point]; ok {
			return cached
		}
		if visiting[point] {
			return readmodelDeadAssignmentProof{}
		}
		visiting[point] = true
		successors := cfg.SuccessorsReadOnly(v.graph, point)
		proof := readmodelDeadAssignmentProof{
			ok:         len(successors) > 0,
			frontier:   make(map[cfg.Point]readmodelDeadAssignmentWrite),
			exitPoints: make(map[cfg.Point]readmodelDeadAssignmentExit),
		}
		for _, succ := range successors {
			child := walk(succ)
			if !child.ok {
				proof = readmodelDeadAssignmentProof{}
				break
			}
			for point, overwrite := range child.frontier {
				proof.frontier[point] = overwrite
			}
			for point, exit := range child.exitPoints {
				proof.exitPoints[point] = exit
			}
		}
		delete(visiting, point)
		if proof.ok && len(proof.frontier) == 0 && len(proof.exitPoints) == 0 {
			proof = readmodelDeadAssignmentProof{}
		}
		memo[point] = proof
		return proof
	}

	frontier := make(map[cfg.Point]readmodelDeadAssignmentWrite)
	exitPoints := make(map[cfg.Point]readmodelDeadAssignmentExit)
	for _, succ := range successors {
		child := walk(succ)
		if !child.ok {
			return nil, nil, false
		}
		for point, overwrite := range child.frontier {
			frontier[point] = overwrite
		}
		for point, exit := range child.exitPoints {
			exitPoints[point] = exit
		}
	}
	overwrites := sortedReadmodelDeadAssignmentWrites(frontier)
	if len(overwrites) == 0 {
		return nil, nil, false
	}
	return overwrites, sortedReadmodelDeadAssignmentExits(exitPoints), true
}

func (v readmodelDeadAssignmentView) pointReachable(point cfg.Point) bool {
	return v.reader.result.PointNormallyReachable(point)
}

func (v readmodelDeadAssignmentView) pointReadsSymbol(point cfg.Point, id symbol.ID) bool {
	reads := v.readsByPoint[point]
	if len(reads) == 0 {
		return false
	}
	_, ok := reads[id]
	return ok
}

func (v readmodelDeadAssignmentView) pointExit(point cfg.Point) (readmodelDeadAssignmentExit, bool) {
	exit, ok := v.exitsByPoint[point]
	return exit, ok
}

func (v readmodelDeadAssignmentView) pointWritesSymbol(point cfg.Point, id symbol.ID) bool {
	for _, write := range v.writesByPoint[point] {
		if write.symbol == id {
			return true
		}
	}
	return false
}

func (v readmodelDeadAssignmentView) pointOverwrite(point cfg.Point, id symbol.ID, ignoredPoint cfg.Point, writes []readmodelDeadAssignmentWrite) (readmodelDeadAssignmentWrite, bool) {
	for _, write := range v.writesByPoint[point] {
		if write.point == ignoredPoint {
			continue
		}
		if write.symbol == id && !readmodelAmbiguousSameStatementWrite(write, writes) {
			return write, true
		}
	}
	return readmodelDeadAssignmentWrite{}, false
}

func sortedReadmodelDeadAssignmentWrites(frontier map[cfg.Point]readmodelDeadAssignmentWrite) []readmodelDeadAssignmentWrite {
	out := make([]readmodelDeadAssignmentWrite, 0, len(frontier))
	for _, write := range frontier {
		out = append(out, write)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].write.StartLine != out[j].write.StartLine {
			return out[i].write.StartLine < out[j].write.StartLine
		}
		if out[i].write.StartCol != out[j].write.StartCol {
			return out[i].write.StartCol < out[j].write.StartCol
		}
		return out[i].point < out[j].point
	})
	return out
}

func sortedReadmodelDeadAssignmentExits(exits map[cfg.Point]readmodelDeadAssignmentExit) []readmodelDeadAssignmentExit {
	out := make([]readmodelDeadAssignmentExit, 0, len(exits))
	for _, exit := range exits {
		out = append(out, exit)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].span, out[j].span
		if sourceSpanValid(left) != sourceSpanValid(right) {
			return sourceSpanValid(left)
		}
		if sourceSpanValid(left) {
			if left.StartLine != right.StartLine {
				return left.StartLine < right.StartLine
			}
			if left.StartCol != right.StartCol {
				return left.StartCol < right.StartCol
			}
		}
		return out[i].point < out[j].point
	})
	return out
}

func ignoredUnusedLocalName(name string) bool {
	return name == "" || strings.HasPrefix(name, "_")
}

func (r Reader) symbolHasReachableRead(readsByPoint map[cfg.Point]map[symbol.ID]struct{}, id symbol.ID) bool {
	for _, reads := range readsByPoint {
		if _, ok := reads[id]; ok {
			return true
		}
	}
	return false
}

func (r Reader) reachableSymbolReads(graph cfg.Graph) map[cfg.Point]map[symbol.ID]struct{} {
	reads := make(map[cfg.Point]map[symbol.ID]struct{})
	functionCaptures := r.functionCaptureReads()
	add := func(point cfg.Point, id symbol.ID) {
		if id == 0 {
			return
		}
		if reads[point] == nil {
			reads[point] = make(map[symbol.ID]struct{})
		}
		reads[point][id] = struct{}{}
	}
	for _, point := range graph.RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		collector := readmodelSymbolReadCollector{result: r.result, functionCaptures: functionCaptures, add: func(id symbol.ID) { add(point, id) }}
		if fact, ok := r.result.LocalAssignment(point); ok {
			collector.exprs(fact.Exprs)
			collector.typeExprs(fact.Types)
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			collector.exprs(fact.Rhs)
			collector.lvalues(fact.Lhs)
		}
		if fact, ok := r.result.Call(point); ok {
			collector.expr(fact.Func)
			collector.expr(fact.Receiver)
			collector.exprs(fact.Args)
		}
		if fact, ok := r.result.ReturnFact(point); ok {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := r.result.BranchCondition(point); ok {
			collector.expr(fact.Condition)
		}
		if fact, ok := r.result.NumericFor(point); ok {
			collector.expr(fact.Init)
			collector.expr(fact.Limit)
			collector.expr(fact.Step)
		}
		if fact, ok := r.result.GenericFor(point); ok && fact.Role == cfgfacts.GenericForRoleCheck {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := r.result.TypeDefinition(point); ok {
			collector.typeDefinition(fact)
		}
		if fact, ok := r.result.FunctionDefinition(point); ok {
			collector.functionNameReads(fact.Name)
			collector.expr(fact.Func)
		}
	}
	return reads
}

func (r Reader) functionCaptureReads() map[*ast.FunctionExpr][]symbol.ID {
	sets := make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	var walk func(*body.Result) map[symbol.ID]struct{}
	walk = func(parent *body.Result) map[symbol.ID]struct{} {
		out := make(map[symbol.ID]struct{})
		if parent == nil {
			return out
		}
		for _, child := range parent.FunctionResults() {
			childSet := make(map[symbol.ID]struct{})
			for _, capture := range parent.DirectCaptures(child.Function()) {
				if capture.Captured != 0 {
					childSet[capture.Captured] = struct{}{}
				}
			}
			for id := range walk(child) {
				childSet[id] = struct{}{}
			}
			if len(childSet) > 0 && child.Function() != nil {
				sets[child.Function()] = childSet
			}
			for id := range childSet {
				out[id] = struct{}{}
			}
		}
		return out
	}
	walk(r.result)
	out := make(map[*ast.FunctionExpr][]symbol.ID, len(sets))
	for fn, set := range sets {
		ids := make([]symbol.ID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		out[fn] = ids
	}
	return out
}

type readmodelSymbolReadCollector struct {
	result           *body.Result
	functionCaptures map[*ast.FunctionExpr][]symbol.ID
	add              func(symbol.ID)
}

func (c readmodelSymbolReadCollector) exprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.expr(expr)
	}
}

func (c readmodelSymbolReadCollector) typeExprs(exprs []ast.TypeExpr) {
	for _, expr := range exprs {
		c.typeExpr(expr)
	}
}

func (c readmodelSymbolReadCollector) typeParams(params []ast.TypeParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Constraint)
	}
}

func (c readmodelSymbolReadCollector) functionParams(params []ast.FunctionParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Type)
	}
}

func (c readmodelSymbolReadCollector) typeDefinition(fact cfgfacts.TypeDefinitionFact) {
	if fact.Type != nil {
		c.typeParams(fact.Type.TypeParams)
		c.typeExpr(fact.Type.Type)
	}
	if fact.Interface != nil {
		for _, field := range fact.Interface.Fields {
			c.typeExpr(field.Type)
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil {
				c.typeExpr(method.Type)
			}
		}
	}
}

func (c readmodelSymbolReadCollector) functionTypeExprs(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	c.typeParams(fn.TypeParams)
	if fn.ParList != nil {
		c.typeExprs(fn.ParList.Types)
		c.typeExpr(fn.ParList.VarargType)
	}
	c.typeExprs(fn.ReturnTypes)
}

func (c readmodelSymbolReadCollector) typeExpr(expr ast.TypeExpr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.PrimitiveTypeExpr, *ast.SelfTypeExpr, *ast.LiteralTypeExpr, *ast.TypeRefExpr:
		return
	case *ast.OptionalTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.UnionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.IntersectionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.ArrayTypeExpr:
		c.typeExpr(e.Element)
	case *ast.MapTypeExpr:
		c.typeExpr(e.Key)
		c.typeExpr(e.Value)
	case *ast.RecordTypeExpr:
		for _, field := range e.Fields {
			c.typeExpr(field.Type)
		}
	case *ast.FunctionTypeExpr:
		c.typeParams(e.TypeParams)
		c.functionParams(e.Params)
		c.typeExpr(e.Variadic)
		c.typeExprs(e.Returns)
	case *ast.AssertsTypeExpr:
		c.typeExpr(e.NarrowTo)
	case *ast.GenericTypeExpr:
		c.typeExprs(e.Args)
	case *ast.MetaTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.TupleTypeExpr:
		c.typeExprs(e.Elements)
	case *ast.TypeOfExpr:
		c.expr(e.Expr)
	case *ast.KeyOfExpr:
		c.typeExpr(e.Inner)
	case *ast.IndexAccessExpr:
		c.typeExpr(e.Object)
		c.typeExpr(e.Index)
	case *ast.ConditionalTypeExpr:
		c.typeExpr(e.Check)
		c.typeExpr(e.Extends)
		c.typeExpr(e.Then)
		c.typeExpr(e.Else)
	}
}

func (c readmodelSymbolReadCollector) lvalues(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.lvalue(expr)
	}
}

func (c readmodelSymbolReadCollector) lvalue(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	default:
		c.expr(expr)
	}
}

func (c readmodelSymbolReadCollector) functionNameReads(name *ast.FuncName) {
	if name == nil {
		return
	}
	c.lvalue(name.Func)
	c.expr(name.Receiver)
}

func (c readmodelSymbolReadCollector) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		if id, ok := c.result.SymbolOfIdent(e); ok {
			c.add(id)
		}
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.expr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.expr(e.Func)
		c.expr(e.Receiver)
		c.exprs(e.Args)
	case *ast.LogicalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.expr(e.Expr)
	case *ast.CastExpr:
		c.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		c.functionTypeExprs(e)
		for _, id := range c.functionCaptures[e] {
			c.add(id)
		}
	}
}

func localNameSourceSpan(stmt *ast.LocalAssignStmt, index int, name string) source.Span {
	if stmt != nil && index >= 0 && index < len(stmt.NamePositions) {
		pos := stmt.NamePositions[index]
		if pos.Valid() {
			endLine, endCol := pos.EndLine, pos.EndColumn
			if endLine == 0 {
				endLine = pos.Line
			}
			if endCol == 0 {
				endCol = pos.Column + len(name)
			}
			return source.Span{
				StartLine: pos.Line,
				StartCol:  pos.Column,
				EndLine:   endLine,
				EndCol:    endCol,
			}
		}
	}
	return ast.SpanOf(stmt)
}

func (r Reader) returnObjectLiteralEntry(point cfg.Point, index int, source sourceprovenance.ASTSource, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return Return{}, false
	}
	expected, ok := r.ValueType(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteral(source.Expr)
	if !ok {
		return Return{}, false
	}
	for _, entry := range literal.Entries {
		entryExpected, ok := luatypeprojection.ExpectedTypeAtSegments(expected, entry.Suffix.Segments)
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			continue
		}
		value, ok := r.SourceValue(point, entry.Source)
		if !ok {
			continue
		}
		actual, _ := r.ValueTypeWithPresence(value)
		untrustedTopOrigin := r.ValueHasUntrustedTopOrigin(value)
		if actual == nil || (r.IsSubtype(actual, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		label := returnExpectedLabel(index) + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = label
		}
		ret := Return{
			Point:              point,
			Index:              index,
			Value:              value,
			ValueHash:          r.ValueHash(value),
			TypeWithPresence:   actual,
			Expected:           entryExpected,
			ExpectedLabel:      label,
			SourceLabel:        sourceLabel,
			SourceSpan:         sourceSpanFromSemantic(entry.ValueSpan),
			DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, index),
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
		}
		ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
			Return:              ret,
			ValueAdmissible:     r.ValueProofAdmissible(value, entryExpected),
			ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, entryExpected),
			IsSubtype:           r.IsSubtype,
		})
		return ret, true
	}
	return Return{}, false
}

func (r Reader) returnValue(point cfg.Point, index int, expr ast.Expr, source sourceprovenance.ASTSource, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	expected, ok := r.ValueType(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	value, ok := r.SourceValue(point, source)
	if !ok {
		return Return{}, false
	}
	actual, _ := r.ValueTypeWithPresence(value)
	ret := Return{
		Point:              point,
		Index:              index,
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   actual,
		Expected:           expected,
		ExpectedLabel:      returnExpectedLabel(index),
		SourceLabel:        returnSourceExprLabel(expr),
		SourceSpan:         sourceSpanFromAST(ast.SpanOf(expr)),
		DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, index),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:              ret,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		IsSubtype:           r.IsSubtype,
	})
	return ret, true
}

func returnSourceAt(fact semantics.ReturnFact, index int) sourceprovenance.ASTSource {
	if index >= 0 && index < len(fact.Sources) {
		return fact.Sources[index]
	}
	return sourceprovenance.NewUnknownSource(sourceprovenance.NoSourceIndex)
}

func returnExpectedLabel(index int) string {
	return "returned value " + strconv.Itoa(index+1)
}

func returnSourceExprLabel(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return strconv.Quote(e.Value)
	case *ast.NumberExpr:
		return e.Value
	case *ast.TrueExpr:
		return "true"
	case *ast.FalseExpr:
		return "false"
	case *ast.NilExpr:
		return "nil"
	default:
		return assignmentSourceLabel(expr)
	}
}

func readmodelSourceSpanAt(spans []SourceSpan, index int) SourceSpan {
	if index < 0 || index >= len(spans) {
		return SourceSpan{}
	}
	return spans[index]
}

func (r Reader) forEachLocalAssignment(point cfg.Point, fact semantics.LocalAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Type == nil || (fact.Expr == nil && fact.Source.Kind == sourceprovenance.SourceExpression) {
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
	sourceExpr := fact.Expr
	if sourceExpr == nil {
		sourceExpr = fact.Source.Expr
	}
	t, _ := r.assignmentSourceType(point, value)
	missingField, missingFieldOK := assignmentMissingRequired(point, r, fact, expected)
	if missingFieldOK {
		if shape, shapeOK := r.assignmentObjectLiteralShapeType(point, fact); shapeOK {
			t = shape
		}
	}
	assignment := Assignment{
		Point:              point,
		TargetLabel:        fact.Name,
		SourceLabel:        assignmentSourceLabel(sourceExpr),
		TargetKey:          assignmentTargetKey(fact),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		ExpectedLabel:      assignmentExpectedLabel(fact.Type),
		ExpectedSource:     readapi.AssignmentExpectedDeclared,
		SourceSpan:         sourceSpanFromAST(ast.SpanOf(sourceExpr)),
		DeclarationSpan:    sourceSpanFromAST(ast.SpanOf(fact.Type)),
		NilableAccesses:    r.assignmentNilableAccessEvidence(point, fact.Expr),
		SourceContributors: r.assignmentSourceContributors(point, fact.Expr),
		CallResult:         r.assignmentCallResultSource(fact.Source),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
	}
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

func (r Reader) assignmentCallResultSource(source sourceprovenance.ASTSource) readapi.CallResultAssignmentSource {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return readapi.CallResultAssignmentSource{}
	}
	site, ok := r.result.CallSite(source.CallPoint)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return readapi.CallResultAssignmentSource{}
	}
	ret, ok := contract.Contract.ResultAt(source.ResultIndex)
	name := contract.Source.Name
	if name == "" {
		name = r.callContractSourceName(site)
	}
	if !ok {
		return readapi.CallResultAssignmentSource{
			Present:       true,
			CallableName:  name,
			ResultIndex:   source.ResultIndex,
			UnderSupplied: true,
		}
	}
	if !ret.Explicit || !readapi.ObligationTypeReportable(ret.Type) {
		return readapi.CallResultAssignmentSource{
			Present:      true,
			CallableName: name,
			ResultIndex:  source.ResultIndex,
		}
	}
	return readapi.CallResultAssignmentSource{
		Present:      true,
		CallableName: name,
		ResultIndex:  source.ResultIndex,
		ReturnSpan:   contract.Source.ResultSpan(source.ResultIndex),
	}
}

// CallResultSourceType projects a source-provenance call result through the
// canonical solved call contract. It is the readmodel-owned replacement for
// diagnostics re-lowering function contracts from syntax.
func (r Reader) CallResultSourceType(source sourceprovenance.ASTSource) (typ.Type, bool) {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return nil, false
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return nil, false
	}
	ret, ok := contract.Contract.ResultAt(source.ResultIndex)
	if !ok {
		return typ.Nil, true
	}
	if ret.Type == nil || refinement.ContainsFreeTypeParam(ret.Type) {
		return nil, false
	}
	return ret.Type, true
}

func (r Reader) callResultSourceUnderSupplied(source sourceprovenance.ASTSource) bool {
	if r.result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return false
	}
	contract, ok := r.callContractAt(source.CallPoint)
	if !ok {
		return false
	}
	_, ok = contract.Contract.ResultAt(source.ResultIndex)
	return !ok
}

func (r Reader) localAssignmentSourceValue(point cfg.Point, fact semantics.LocalAssignmentFact) (product.Value, bool) {
	var value product.Value
	var ok bool
	value, ok = r.SourceValue(point, fact.Source)
	if ok {
		return r.withMemberReadNilWitness(point, fact.Expr, value), true
	}
	if r.callResultSourceUnderSupplied(fact.Source) {
		if reg := r.result.Registry(); reg != nil {
			return product.Absent(reg), true
		}
	}
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Expr != nil {
		if value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr); ok {
			return r.withMemberReadNilWitness(point, fact.Expr, value), true
		}
	}
	if !ok {
		if fact.Expr != nil {
			value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr)
			if ok {
				return r.withMemberReadNilWitness(point, fact.Expr, value), true
			}
		}
		return product.Value{}, false
	}
	return r.withMemberReadNilWitness(point, fact.Expr, value), true
}

func (r Reader) withMemberReadNilWitness(point cfg.Point, expr ast.Expr, value product.Value) product.Value {
	if r.result == nil || r.result.Registry() == nil || expr == nil ||
		!r.memberReadCanMissOrDeclaredNilable(point, expr) ||
		r.result.ExpressionReadProvenPresentBeforeBoundary(point, expr) ||
		r.memberReadHasExactPathProof(point, expr) ||
		r.memberReadHasExactHeapProof(point, expr) ||
		r.memberReadHasLengthFloorProof(point, expr) ||
		r.memberReadHasLiteralDeclarationProof(point, expr) {
		return value
	}
	got, ok := r.ValueTypeWithPresence(value)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) {
		if declared, declaredOK := r.declaredExprTypeAt(point, expr); declaredOK &&
			declared != nil &&
			!typ.IsAny(declared) &&
			!typ.IsUnknown(declared) {
			value = typevalue.WithWitness(r.result.Registry(), value, declared)
			if typevalue.TypeIncludesNil(declared) {
				return product.WithPresence(r.result.Registry(), value, presence.Maybe())
			}
			got = declared
		} else {
			return value
		}
	}
	if typevalue.TypeIncludesNil(got) {
		return value
	}
	value = product.WithPresence(r.result.Registry(), value, presence.Maybe())
	return typevalue.WithWitness(r.result.Registry(), value, normalize.Optional(got))
}

func (r Reader) memberReadCanMissOrDeclaredNilable(point cfg.Point, expr ast.Expr) bool {
	if r.memberReadCanMiss(point, expr) {
		return true
	}
	if _, ok := expr.(*ast.AttrGetExpr); !ok {
		return false
	}
	if current, ok := r.expressionTypeBeforeBoundary(point, expr); ok &&
		current != nil &&
		!typevalue.TypeIncludesNil(current) {
		return false
	}
	declared, ok := r.declaredExprTypeAt(point, expr)
	return ok && typevalue.TypeIncludesNil(declared)
}

func (r Reader) memberReadCanMiss(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return false
	}
	container, ok := r.expressionTypeBeforeBoundary(point, attr.Object)
	if !ok {
		container, ok = r.declaredExprTypeAt(point, attr.Object)
	}
	if !ok || container == nil {
		return false
	}
	var key typ.Type
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return false
		}
		key = typ.LiteralString(name)
	} else {
		key, ok = r.expressionTypeBeforeBoundary(point, attr.Key)
		if !ok {
			key, ok = assignmentLiteralSourceType(attr.Key)
		}
	}
	if key == nil {
		return false
	}
	got, ok := access.RuntimeIndex(container, key)
	return ok && typevalue.TypeIncludesNil(got)
}

func (r Reader) memberReadHasExactPathProof(point cfg.Point, expr ast.Expr) bool {
	if r.result == nil {
		return false
	}
	p, ok := r.result.ExpressionPath(expr)
	if !ok || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) == 0 {
		return false
	}
	st, ok := r.result.StateAt(point)
	if !ok {
		return false
	}
	return readCanonicalPathStaticMember(st, r.result.KeySpace(), p.Key())
}

func (r Reader) memberReadHasExactHeapProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || r.result == nil || r.result.Registry() == nil {
		return false
	}
	suffix, ok := memberReadSuffix(attr)
	if !ok {
		return false
	}
	object, ok := r.result.ExpressionValueBeforeBoundary(point, attr.Object)
	if !ok {
		return false
	}
	id, ok := product.Get(r.result.Registry(), object, identity.Key).ID()
	if !ok {
		return false
	}
	st, ok := r.result.StateAt(point)
	if !ok {
		return false
	}
	memberKey, ok := heapidentity.StaticMemberSuffixKey(r.result.KeySpace(), suffix)
	if !ok {
		return false
	}
	table := st.ReadHeapTableObject(r.result.Registry(), id)
	if _, ok := table.StaticMember(memberKey); ok {
		return true
	}
	if canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(r.result.KeySpace(), suffix); ok {
		if _, ok := table.StaticMember(canonical); ok {
			return true
		}
	}
	return false
}

func (r Reader) memberReadHasLengthFloorProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || attr.KeySyntax != ast.AttrKeyIndex || r.result == nil {
		return false
	}
	key, ok := attr.Key.(*ast.NumberExpr)
	if !ok || strings.ContainsAny(key.Value, ".eE") {
		return false
	}
	index, err := strconv.ParseInt(key.Value, 10, 64)
	if err != nil || index < 1 {
		return false
	}
	arrayPath, ok := r.result.ExpressionPath(attr.Object)
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	floor, ok := r.result.LengthFloorAtBoundary(point, arrayPath)
	return ok && floor >= index
}

func (r Reader) memberReadHasLiteralDeclarationProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || r.result == nil {
		return false
	}
	suffix, ok := memberReadSuffix(attr)
	if !ok {
		return false
	}
	objectPath, ok := r.result.ExpressionPath(attr.Object)
	if !ok || objectPath.IsEmpty() || objectPath.Symbol == 0 {
		return false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, objectPath)
	if !ok || declaration.Source.Kind != factflow.ValueSourceExpression || !declaration.Source.HasExpr {
		return false
	}
	literal, ok := r.result.ObjectLiteralExpr(declaration.Source.ExprRef)
	if !ok {
		return false
	}
	found := false
	literal.View().ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if pathSegmentsEqual(entry.SuffixSegmentsView(), suffix) {
			found = true
			return false
		}
		return true
	})
	return found
}

func pathSegmentsEqual(left, right []segment.Segment) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func memberReadSuffix(attr *ast.AttrGetExpr) ([]segment.Segment, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return []segment.Segment{{Kind: segment.SegmentField, Name: name}}, true
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return []segment.Segment{{Kind: segment.SegmentIndexString, Name: key.Value}}, true
	case *ast.NumberExpr:
		if strings.ContainsAny(key.Value, ".eE") {
			return nil, false
		}
		index, err := strconv.Atoi(key.Value)
		if err != nil {
			return nil, false
		}
		return []segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}, true
	default:
		return nil, false
	}
}

func readCanonicalPathStaticMember(st statePathStaticMemberReader, ks *keyspace.KeySpace, pathKey path.PathKey) bool {
	if ks == nil || pathKey == "" {
		return false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return false
	}
	if _, ok := st.ReadLocalPathStaticMember(localKey); ok {
		return true
	}
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		if _, ok := st.ReadLocalPathStaticMember(canonical); ok {
			return true
		}
	}
	if stable, ok := stableStaticMemberKey(ks, localKey); ok {
		if _, ok := st.ReadLocalPathStaticMember(stable); ok {
			return true
		}
		if canonical, ok := ks.FieldCanonical(stable); ok {
			if _, ok := st.ReadLocalPathStaticMember(canonical); ok {
				return true
			}
		}
	}
	return false
}

type statePathStaticMemberReader interface {
	ReadLocalPathStaticMember(keyspace.Key) (product.Value, bool)
}

func stableStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.FromStableSymbol(localKey.Sym, segments)
}

func (r Reader) declaredExprType(expr ast.Expr) (typ.Type, bool) {
	if r.result == nil || expr == nil {
		return nil, false
	}
	p, ok := r.result.ExpressionPath(expr)
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	return r.declaredPathType(p)
}

func (r Reader) declaredExprTypeAt(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if t, ok := r.declaredExprType(expr); ok {
		return t, true
	}
	if r.result != nil && expr != nil {
		if p, ok := r.result.ExpressionPath(expr); ok && !p.IsEmpty() && p.Symbol != 0 {
			if t, ok := r.dominatingDeclarationSourcePathType(point, p); ok {
				return t, true
			}
		}
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.declaredExprTypeAt(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return access.Field(container, name)
	}
	key, ok := assignmentLiteralSourceType(attr.Key)
	if !ok {
		key, ok = r.expressionTypeBeforeBoundary(point, attr.Key)
	}
	if !ok || key == nil {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
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

func (r Reader) assignmentSourceContributors(point cfg.Point, expr ast.Expr) []readapi.AssignmentSourceContribution {
	if r.result == nil || expr == nil {
		return nil
	}
	readPath, ok := r.result.ExpressionPath(expr)
	if !ok || readPath.Symbol == 0 || len(readPath.Segments) != 1 {
		return nil
	}
	field := staticMemberSegmentName(readPath.Segments[0])
	if field == "" {
		return nil
	}
	rootLabel := readPath.Root
	if rootLabel == "" {
		rootLabel = assignmentSourceRootLabel(expr)
	}
	readLabel := assignmentSourceLabel(expr)
	var out []readapi.AssignmentSourceContribution
	for _, candidate := range r.result.Graph().RPO() {
		if candidate == point {
			break
		}
		fact, ok := r.result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != readPath.Symbol {
			continue
		}
		if fact.HasPath && len(fact.Path.Segments) != 0 {
			continue
		}
		table, ok := sourceprovenance.ProofInner(fact.Value)
		if !ok {
			continue
		}
		lit, ok := table.(*ast.TableExpr)
		if !ok {
			continue
		}
		for _, entry := range lit.Fields {
			if entry == nil || ast.KeyName(entry.Key) != field || entry.Value == nil {
				continue
			}
			t, ok := r.expressionValueTypeAtBoundary(candidate, entry.Value)
			if !ok {
				continue
			}
			out = append(out, readapi.AssignmentSourceContribution{
				RootLabel: rootLabel,
				ReadLabel: readLabel,
				Type:      t,
				Span:      sourceSpanFromAST(ast.SpanOf(fact.Value)),
			})
		}
	}
	return out
}

func staticMemberSegmentName(seg segment.Segment) string {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name
	default:
		return ""
	}
}

func assignmentSourceRootLabel(expr ast.Expr) string {
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.Object != nil {
		return assignmentSourceLabel(attr.Object)
	}
	return assignmentSourceLabel(expr)
}

func (r Reader) expressionValueTypeAtBoundary(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if t, ok := assignmentLiteralSourceType(expr); ok {
		return t, true
	}
	value, ok := r.result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return nil, false
	}
	return r.ValueType(value)
}

func (r Reader) forEachOrdinaryAssignment(point cfg.Point, fact semantics.OrdinaryAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Target == nil || fact.Value == nil {
		return true
	}
	expected, ok := r.ordinaryAssignmentTargetType(point, fact)
	if !ok || !readapi.ObligationTypeReportable(expected) {
		return true
	}
	value, ok := r.ordinaryAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	if literal, ok := r.result.ObjectLiteral(fact.Value); ok {
		if entry, ok := r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
			Label:          assignmentSourceLabel(fact.Target),
			Key:            assignmentTargetKeyForOrdinary(point, fact),
			ExpectedSpan:   sourceSpanFromAST(ast.SpanOf(fact.Target)),
			ExpectedSource: readapi.AssignmentExpectedDynamicTarget,
		}); ok {
			*visited = true
			return visit(entry)
		}
	}
	t, _ := r.assignmentSourceType(point, value)
	assignment := Assignment{
		Point:              point,
		TargetLabel:        assignmentSourceLabel(fact.Target),
		SourceLabel:        assignmentSourceLabel(fact.Value),
		TargetKey:          assignmentTargetKeyForOrdinary(point, fact),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		ExpectedSource:     ordinaryAssignmentExpectedSource(fact.Target),
		SourceSpan:         sourceSpanFromAST(ast.SpanOf(fact.Value)),
		DeclarationSpan:    sourceSpanFromAST(ast.SpanOf(fact.Target)),
		NilableAccesses:    r.assignmentNilableAccessEvidence(point, fact.Value),
		SourceContributors: r.assignmentSourceContributors(point, fact.Value),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
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

func ordinaryAssignmentExpectedSource(target ast.Expr) readapi.AssignmentExpectedSource {
	attr, ok := assignmentTargetAttrExpr(target)
	if !ok || attr == nil || attr.KeySyntax != ast.AttrKeyIndex || attr.Key == nil {
		return readapi.AssignmentExpectedDeclared
	}
	switch attr.Key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return readapi.AssignmentExpectedDeclared
	default:
		return readapi.AssignmentExpectedDynamicTarget
	}
}

func (r Reader) ordinaryAssignmentSourceValue(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (product.Value, bool) {
	if value, ok := r.SourceValue(point, fact.Source); ok {
		return r.withMemberReadNilWitness(point, fact.Value, value), true
	}
	if fact.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Value); ok {
			return r.withMemberReadNilWitness(point, fact.Value, value), true
		}
	}
	if t, ok := assignmentLiteralSourceType(fact.Value); ok && r.result != nil && r.result.Registry() != nil {
		return typevalue.FromType(r.result.Registry(), t), true
	}
	return product.Value{}, false
}

func (r Reader) assignmentSourceType(point cfg.Point, value product.Value) (typ.Type, bool) {
	if fn, ok := r.result.FunctionValueTypeForValueAtBoundary(point, value); ok {
		return fn, true
	}
	return r.ValueTypeWithPresence(value)
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

func (r Reader) assignmentObjectLiteralShapeType(point cfg.Point, fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return nil, false
	}
	builder := typetable.NewConstructorBuilder()
	seen := false
	for _, entry := range literal.Entries {
		path, ok := luatypeprojection.ConstructorPathFromSegments(entry.Suffix.Segments)
		if !ok {
			continue
		}
		t, literalOK := assignmentLiteralSourceType(entry.Value)
		if !literalOK {
			value, valueOK := r.SourceValue(point, entry.Source)
			if !valueOK {
				continue
			}
			if loweredSource, ok := sourcebridge.ValueSourceFromASTSource(entry.Source); ok {
				if before, beforeOK := r.result.SourceValueBeforeBoundary(point, loweredSource); beforeOK {
					value = before
				}
			}
			t, ok = luasourcevalue.ObjectLiteralEntryType(r.result.Registry(), r.typeValues, value)
			if !ok {
				t, ok = r.ValueTypeWithPresence(value)
				if !ok || t == nil {
					continue
				}
			}
		}
		if !builder.Add(path, t) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func (r Reader) assignmentObjectLiteralEntry(point cfg.Point, fact semantics.LocalAssignmentFact, expected typ.Type) (Assignment, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return Assignment{}, false
	}
	return r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
		Label:           fact.Name,
		Key:             assignmentTargetKey(fact),
		ExpectedSpan:    sourceSpanFromAST(ast.SpanOf(fact.Type)),
		ExpectedSource:  readapi.AssignmentExpectedDeclared,
		ExpectedTypeAST: fact.Type,
	})
}

type assignmentObjectEntryTarget struct {
	Label           string
	Key             string
	ExpectedSpan    SourceSpan
	ExpectedSource  readapi.AssignmentExpectedSource
	ExpectedTypeAST ast.TypeExpr
}

func (r Reader) assignmentObjectLiteralEntryCandidate(point cfg.Point, literal semantics.ObjectLiteralFact, expected typ.Type, target assignmentObjectEntryTarget) (Assignment, bool) {
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
				if !luasourcevalue.ObjectLiteralEntryHasUntrustedTopOrigin(r.result.Registry(), entryValue) {
					continue
				}
				if projected, projectedOK := r.ValueTypeWithPresence(entryValue); projectedOK && projected != nil {
					t = projected
				} else {
					t = typ.Unknown
				}
			}
		}
		untrustedTopOrigin := valueOK && r.ValueHasUntrustedTopOrigin(value)
		explicitTopOrigin := valueOK && r.ValueHasExplicitTopOrigin(value)
		if t == nil || (subtype.IsSubtype(t, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		targetLabel := target.Label + segment.FormatSegments(entry.Suffix.Segments)
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
			TargetKey:          target.Key + ":" + segment.FormatSegments(entry.Suffix.Segments),
			Value:              value,
			ValueHash:          assignmentValueHash(r, value, valueOK),
			TypeWithPresence:   t,
			Expected:           entryExpected,
			ExpectedSource:     target.ExpectedSource,
			SourceSpan:         sourceSpanFromSemantic(entry.ValueSpan),
			DeclarationSpan:    target.ExpectedSpan,
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  explicitTopOrigin,
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
	if key, ok := attr.Key.(*ast.StringExpr); ok && key != nil {
		t, ok := access.Field(container, key.Value)
		return r.ordinaryWritableTargetType(point, fact.Target, t), ok
	}
	keyType, ok := r.expressionTypeBeforeBoundary(point, attr.Key)
	if !ok {
		keyType, ok = assignmentLiteralSourceType(attr.Key)
	}
	if !ok || keyType == nil {
		return nil, false
	}
	t, ok := luatypeprojection.DynamicWriteValueType(container, keyType)
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
		origin := obligation.Origin
		if !origin.HasOrigin {
			site, _ := r.result.CallSite(point)
			origin = readapi.CallArgumentObligationOrigin{
				HasOrigin:    true,
				FunctionName: r.callContractSourceName(site),
				SubjectLabel: callArgumentLabel(obligation.Index),
			}
		}
		plan.OutcomeParams = append(plan.OutcomeParams, readapi.IndexedCallArgumentObligation{
			Index: obligation.Index,
			Obligation: CallArgumentObligation{
				Type:          obligation.Type,
				ExpectedLabel: CallContractSource{}.ParameterLabel(obligation.Index),
				ExpectedSpan:  callArgumentSpanByIndex(args, obligation.Index),
				Origin:        origin,
			},
		})
	}
	return readapi.PlanCallArgumentReports(plan)
}

func callArgumentSpanByIndex(args []CallArgument, index int) SourceSpan {
	for _, arg := range args {
		if arg.Index == index {
			return arg.Span
		}
	}
	return SourceSpan{}
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
	if r.result == nil {
		return CallCalleeReport{}
	}
	memberAccess := site.CalleeMemberAccess()
	var receiverPath path.Path
	var member segment.Segment
	var hasMemberPath bool
	p := site.CalleePathRef()
	if p.IsEmpty() {
		if report, ok := r.expressionReceiverMethodCalleeReport(point, site); ok {
			return report
		}
		receiverPath, member, hasMemberPath = site.CalleeMemberAccessPath()
		if !hasMemberPath || receiverPath.IsEmpty() {
			return CallCalleeReport{}
		}
		p = receiverPath.Append(member)
	} else if memberAccess {
		receiverPath, member, hasMemberPath = site.CalleeMemberAccessPath()
	}
	if memberAccess && hasMemberPath {
		if report, ok := r.missingMemberCalleeReport(point, site, receiverPath, member); ok {
			return report
		}
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
	if memberAccess && typevalue.TypeIncludesNil(t) {
		if receiverType, ok := r.memberReceiverNilableAtCall(point, site); ok {
			t = receiverType
		} else {
			if calleeTypeCallableIgnoringNil(t) {
				return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
					CallableName: r.callContractSourceName(site),
					Type:         t,
					Callable:     true,
					MemberAccess: true,
					Span:         sourceSpanFromFactflow(site.CalleeSpan()),
					CallSpan:     sourceSpanFromFactflow(site.CallSpan()),
				})
			}
			return CallCalleeReport{}
		}
	}
	callable := calleeTypeCallable(t)
	return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
		CallableName:                 r.callContractSourceName(site),
		Type:                         t,
		Callable:                     callable,
		MemberAccess:                 memberAccess,
		ImpreciseMemberRequiresProof: r.impreciseMemberCalleeRequiresProof(point, site),
		Span:                         sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:                     sourceSpanFromFactflow(site.CallSpan()),
	})
}

func (r Reader) impreciseMemberCalleeRequiresProof(point cfg.Point, site factflow.CallSite) bool {
	if !site.CalleeMemberAccess() {
		return false
	}
	receiver, ok := r.callReceiverType(point, site)
	return ok && receiver != nil && !typ.IsAny(receiver) && !typ.IsUnknown(receiver) && !typ.IsNever(receiver)
}

func (r Reader) missingMemberCalleeReport(point cfg.Point, site factflow.CallSite, receiverPath path.Path, member segment.Segment) (CallCalleeReport, bool) {
	receiverType, ok := r.callReceiverType(point, site)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return CallCalleeReport{}, false
	}
	if typevalue.TypeIncludesNil(receiverType) {
		return CallCalleeReport{}, false
	}
	_, status, ok := callcontract.MemberCall(receiverType, member)
	if !ok || status != callcontract.MemberCallMissing {
		return CallCalleeReport{}, false
	}
	if !r.reportMissingMemberShape(point, receiverPath, member, receiverType) {
		return CallCalleeReport{}, false
	}
	memberName := callCalleeMemberSegmentDisplay(member)
	if memberName == "" {
		return CallCalleeReport{}, false
	}
	name := r.callContractSourceName(site)
	if name == "" {
		name = "receiver"
	}
	return CallCalleeReport{
		Kind:         readapi.CallCalleeReportMissingMember,
		CallableName: name,
		Type:         receiverType,
		MemberAccess: true,
		MemberName:   memberName,
		Span:         sourceSpanFromFactflow(site.CallSpan()),
	}, true
}

func (r Reader) reportMissingMemberShape(point cfg.Point, receiver path.Path, member segment.Segment, receiverType typ.Type) bool {
	if member.Kind == segment.SegmentIndexInt {
		return true
	}
	if callCalleeUnionReceiver(receiverType) {
		return true
	}
	if !callCalleeClosedConcreteRecord(receiverType) {
		return false
	}
	if r.result != nil && !receiver.IsEmpty() {
		if value, ok := r.result.PathValueAtBoundary(point, receiver); ok && r.ValueHasExactIdentity(value) {
			return false
		}
	}
	if receiver.Symbol == 0 || r.result == nil {
		return false
	}
	if _, ok := r.result.SymbolTypeAnnotation(receiver.Symbol); ok {
		return true
	}
	kind, ok := r.result.SymbolKind(receiver.Symbol)
	return ok && (kind == symbol.Param || kind == symbol.Global)
}

func callCalleeUnionReceiver(t typ.Type) bool {
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return true
	case *typ.Alias:
		return callCalleeUnionReceiver(v.UnaliasedTarget())
	default:
		return false
	}
}

func callCalleeClosedConcreteRecord(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil || rec.Open || rec.HasMapComponent() {
		return false
	}
	return len(rec.Fields) != 0 || len(rec.StaticMembers) != 0
}

func callCalleeMemberSegmentDisplay(member segment.Segment) string {
	switch member.Kind {
	case segment.SegmentField:
		return member.Name
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		return segment.FormatSegments([]segment.Segment{member})
	default:
		return ""
	}
}

func (r Reader) walkAssignmentTargetMissingMemberReads(point cfg.Point, target ast.Expr, seen map[*ast.AttrGetExpr]struct{}, visit func(MissingMemberRead) bool, visited *bool, depth int) bool {
	if target == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		if !r.walkMissingMemberReads(point, t.Object, seen, visit, visited, depth+1, false) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkMissingMemberReads(point, t.Key, seen, visit, visited, depth+1, false)
		}
	case *ast.CastExpr:
		return r.walkAssignmentTargetMissingMemberReads(point, t.Expr, seen, visit, visited, depth+1)
	case *ast.NonNilAssertExpr:
		return r.walkAssignmentTargetMissingMemberReads(point, t.Expr, seen, visit, visited, depth+1)
	}
	return true
}

func (r Reader) walkMissingMemberReads(point cfg.Point, expr ast.Expr, seen map[*ast.AttrGetExpr]struct{}, visit func(MissingMemberRead) bool, visited *bool, depth int, allowExactNilRead bool) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !r.walkMissingMemberReads(point, e.Object, seen, visit, visited, depth+1, false) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex && !r.walkMissingMemberReads(point, e.Key, seen, visit, visited, depth+1, false) {
			return false
		}
		if _, ok := seen[e]; ok {
			return true
		}
		seen[e] = struct{}{}
		if item, ok := r.missingMemberRead(point, e, allowExactNilRead); ok {
			*visited = true
			return visit(item)
		}
	case *ast.FuncCallExpr:
		if callee, ok := e.Func.(*ast.AttrGetExpr); ok && callee.KeySyntax == ast.AttrKeyDot {
			if !r.walkMissingMemberReads(point, callee.Object, seen, visit, visited, depth+1, false) {
				return false
			}
		} else if !r.walkMissingMemberReads(point, e.Func, seen, visit, visited, depth+1, false) {
			return false
		}
		if !r.walkMissingMemberReads(point, e.Receiver, seen, visit, visited, depth+1, false) {
			return false
		}
		for _, arg := range e.Args {
			if !r.walkMissingMemberReads(point, arg, seen, visit, visited, depth+1, false) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !r.walkMissingMemberReads(point, field.Key, seen, visit, visited, depth+1, false) {
				return false
			}
			if !r.walkMissingMemberReads(point, field.Value, seen, visit, visited, depth+1, false) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		if !r.walkMissingMemberReads(point, e.Lhs, seen, visit, visited, depth+1, e.Operator == "or") {
			return false
		}
		return r.walkMissingMemberReads(point, e.Rhs, seen, visit, visited, depth+1, false)
	case *ast.RelationalOpExpr:
		return r.walkMissingMemberReads(point, e.Lhs, seen, visit, visited, depth+1, false) &&
			r.walkMissingMemberReads(point, e.Rhs, seen, visit, visited, depth+1, false)
	case *ast.StringConcatOpExpr:
		return r.walkMissingMemberReads(point, e.Lhs, seen, visit, visited, depth+1, false) &&
			r.walkMissingMemberReads(point, e.Rhs, seen, visit, visited, depth+1, false)
	case *ast.ArithmeticOpExpr:
		return r.walkMissingMemberReads(point, e.Lhs, seen, visit, visited, depth+1, false) &&
			r.walkMissingMemberReads(point, e.Rhs, seen, visit, visited, depth+1, false)
	case *ast.UnaryMinusOpExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	case *ast.UnaryNotOpExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	case *ast.UnaryLenOpExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	case *ast.UnaryBNotOpExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	case *ast.CastExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	case *ast.NonNilAssertExpr:
		return r.walkMissingMemberReads(point, e.Expr, seen, visit, visited, depth+1, false)
	}
	return true
}

func (r Reader) missingMemberRead(point cfg.Point, expr *ast.AttrGetExpr, allowExactNilRead bool) (MissingMemberRead, bool) {
	_, memberName, ok := staticMemberReadSegment(expr)
	if !ok {
		return MissingMemberRead{}, false
	}
	if allowExactNilRead && r.exactLocalMissingFieldReadsNil(point, expr, memberName) {
		return MissingMemberRead{}, false
	}
	if r.memberReadReceiverHasUntrustedTopOrigin(point, expr.Object) {
		return MissingMemberRead{}, false
	}
	receiverType, ok := r.expressionTypeBeforeBoundary(point, expr.Object)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return MissingMemberRead{}, false
	}
	report := readmodelUnionArmRejectsFieldRead(receiverType, memberName)
	if !report {
		broad, broadOK := r.declaredExprTypeAt(point, expr.Object)
		if !broadOK || broad == nil || !inspect.IsMultiArmUnion(broad) {
			return MissingMemberRead{}, false
		}
		fieldBroad := broad
		if withoutNil := readmodelProjectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
			fieldBroad = withoutNil
		}
		if _, ok := access.Field(fieldBroad, memberName); !ok || !readmodelFieldProvablyAbsent(receiverType, memberName) {
			return MissingMemberRead{}, false
		}
		report = true
	}
	if !report {
		return MissingMemberRead{}, false
	}
	return MissingMemberRead{
		Point:        point,
		ReadLabel:    assignmentSourceLabel(expr),
		MemberName:   memberName,
		ReceiverType: receiverType,
		Span:         sourceSpanFromAST(ast.SpanOf(expr)),
	}, true
}

type resultShapeCase struct {
	index   int
	name    string
	literal typ.Type
}

type resultShapeLiteralDomain struct {
	target path.Path
	suffix []segment.Segment
	cases  []resultShapeCase
}

func (r Reader) resultShapeRead(point cfg.Point, expr *ast.AttrGetExpr) (ResultShapeExhaustiveness, bool) {
	_, member, ok := staticMemberReadSegment(expr)
	if !ok || member == "ok" {
		return ResultShapeExhaustiveness{}, false
	}
	receiverPath, ok := r.result.ExpressionPath(expr.Object)
	if !ok || receiverPath.Symbol == 0 {
		return ResultShapeExhaustiveness{}, false
	}
	readPath, ok := r.result.ExpressionPath(expr)
	if !ok || readPath.Symbol == 0 {
		return ResultShapeExhaustiveness{}, false
	}
	receiverType, ok := r.resultShapeBroadReceiverType(point, receiverPath)
	if !ok {
		return ResultShapeExhaustiveness{}, false
	}
	discriminant, required, ok := resultShapeRequiredCaseForMember(receiverPath, receiverType, member)
	if !ok {
		return ResultShapeExhaustiveness{}, false
	}
	if r.resultShapeRequiredCaseProven(point, discriminant, required) {
		return ResultShapeExhaustiveness{}, false
	}
	if r.resultShapeOtherCaseDominates(point, discriminant, required) {
		return ResultShapeExhaustiveness{}, false
	}
	return ResultShapeExhaustiveness{
		Point:         point,
		ReceiverLabel: receiverPath.String(),
		ReadLabel:     readPath.String(),
		Discriminant:  discriminant.String(),
		RequiredCase:  required.name,
		Span:          sourceSpanFromAST(ast.SpanOf(expr)),
	}, true
}

func (r Reader) resultShapeBroadReceiverType(point cfg.Point, receiver path.Path) (typ.Type, bool) {
	if r.result == nil || receiver.IsEmpty() || receiver.Symbol == 0 {
		return nil, false
	}
	t, ok := r.resultShapeRootType(point, receiver.RootOnly())
	if !ok || t == nil {
		return nil, false
	}
	for _, seg := range receiver.Segments {
		next, ok := resultShapeExpressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, true
}

func (r Reader) resultShapeRootType(point cfg.Point, root path.Path) (typ.Type, bool) {
	if root.Symbol == 0 {
		return nil, false
	}
	if annotated, ok := r.symbolDeclaredType(root.Symbol); ok {
		return r.resultShapeTransparentComparableType(annotated), true
	}
	value, ok := r.result.SymbolValueAtBoundary(point, root.Symbol)
	if !ok {
		return nil, false
	}
	return r.FullVariantOriginType(value)
}

func resultShapeRequiredCaseForMember(receiver path.Path, receiverType typ.Type, member string) (path.Path, resultShapeCase, bool) {
	_, cases, ok := variant.OriginCasesOfType(receiverType)
	if !ok || len(cases) < 2 {
		return path.Path{}, resultShapeCase{}, false
	}
	requiredIndex, ok := resultShapeSingleOriginCaseWithField(cases, member)
	if !ok {
		return path.Path{}, resultShapeCase{}, false
	}
	for _, domain := range resultShapeLiteralDomainsForCases(receiver, cases) {
		for _, c := range domain.cases {
			if c.index == requiredIndex {
				return domain.target, c, true
			}
		}
	}
	return path.Path{}, resultShapeCase{}, false
}

func resultShapeSingleOriginCaseWithField(cases []variant.OriginCase, member string) (int, bool) {
	required := -1
	for _, c := range cases {
		if _, ok := access.Field(c.Type, member); !ok {
			continue
		}
		if required >= 0 {
			return 0, false
		}
		required = c.Index
	}
	return required, required >= 0
}

func resultShapeLiteralDomainsForCases(receiver path.Path, cases []variant.OriginCase) []resultShapeLiteralDomain {
	if len(cases) == 0 {
		return nil
	}
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	out := make([]resultShapeLiteralDomain, 0, len(domains))
	for _, domain := range domains {
		suffix := domain.Suffix
		target := receiver.AppendSegments(suffix)
		domainCases, ok := resultShapeLiteralCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, resultShapeLiteralDomain{
			target: target,
			suffix: append([]segment.Segment(nil), suffix...),
			cases:  domainCases,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func resultShapeLiteralCasesFor(target path.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]resultShapeCase, bool) {
	out := make([]resultShapeCase, 0, len(cases))
	var seen []typ.Type
	for _, c := range cases {
		lit, ok := resultShapeDiscriminantCaseLiteral(c.Type, suffix)
		if !ok || !resultShapeLiteralSupported(lit) {
			return nil, false
		}
		for _, previous := range seen {
			if typ.TypeEquals(previous, lit) {
				return nil, false
			}
		}
		seen = append(seen, lit)
		out = append(out, resultShapeCase{
			index:   c.Index,
			name:    resultShapeDiscriminantCaseName(target, suffix, c.Type),
			literal: lit,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out, true
}

func resultShapeDiscriminantCaseName(target path.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + typeformat.Short(field)
	}
	return typeformat.Short(caseType)
}

func resultShapeDiscriminantCaseLiteral(caseType typ.Type, suffix []segment.Segment) (*typ.Literal, bool) {
	field, ok := variant.FieldAtPath(caseType, suffix)
	if !ok {
		return nil, false
	}
	lit, ok := field.(*typ.Literal)
	return lit, ok
}

func resultShapeLiteralSupported(lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	switch lit.Value.(type) {
	case bool, string:
		return true
	default:
		return false
	}
}

func (r Reader) resultShapeRequiredCaseProven(point cfg.Point, discriminant path.Path, required resultShapeCase) bool {
	if required.literal == nil {
		return false
	}
	if r.result.DominatingBranchProvesLiteral(point, discriminant, required.literal) {
		return true
	}
	lit, ok := r.result.PathLiteralTypeAtBoundary(point, discriminant)
	return ok && typ.TypeEquals(lit, required.literal)
}

func (r Reader) resultShapeOtherCaseDominates(point cfg.Point, discriminant path.Path, required resultShapeCase) bool {
	if required.literal == nil {
		return false
	}
	proven, _, ok := r.result.DominatingLiteralBranchForPath(point, discriminant)
	return ok && !typ.TypeEquals(proven, required.literal)
}

func resultShapeExpressionSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if field, ok := access.Field(t, seg.Name); ok {
			return field, true
		}
		if access.MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		key, ok := luatypeprojection.SegmentKeyType(seg)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(t, key)
	default:
		return nil, false
	}
}

func (r Reader) resultShapeTransparentComparableType(t typ.Type) typ.Type {
	t = resultShapeTransparentExpectedType(t)
	if r.result == nil {
		return t
	}
	moduleTypes := r.result.ModuleTypes()
	if len(moduleTypes.Manifests) == 0 {
		return t
	}
	resolved := transform.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		ref, ok := node.(*typ.Ref)
		if !ok || ref.Module == "" {
			return nil, false
		}
		if resolved, ok := moduleTypes.Lookup(ref.Module, ref.Name); ok {
			return resolved, true
		}
		if modulePath, ok := r.result.RequireAliasModulePath(ref.Module); ok {
			if resolved, ok := moduleTypes.Lookup(modulePath, ref.Name); ok {
				return resolved, true
			}
		}
		return nil, false
	})
	return resultShapeTransparentExpectedType(resolved)
}

func resultShapeTransparentExpectedType(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return typ.Unknown
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return t
			}
			t = tt.Body
		case *typ.Instantiated:
			next := resultShapeShallowExpandExpectedInstantiated(tt)
			if next == nil || next == t {
				return t
			}
			t = next
		default:
			return t
		}
	}
	return t
}

func resultShapeShallowExpandExpectedInstantiated(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil || len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
		return inst
	}
	body := subst.Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
	if body == nil {
		return inst
	}
	return subst.Self(body, inst)
}

func staticMemberReadSegment(expr *ast.AttrGetExpr) (segment.Segment, string, bool) {
	if expr == nil || expr.Key == nil {
		return segment.Segment{}, "", false
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(expr.Key)
		if name == "" {
			return segment.Segment{}, "", false
		}
		return segment.Segment{Kind: segment.SegmentField, Name: name}, name, true
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			if key.Value == "" {
				return segment.Segment{}, "", false
			}
			return segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value}, key.Value, true
		case *ast.NumberExpr:
			if strings.ContainsAny(key.Value, ".eE") {
				return segment.Segment{}, "", false
			}
			index, err := strconv.Atoi(key.Value)
			if err != nil {
				return segment.Segment{}, "", false
			}
			return segment.Segment{Kind: segment.SegmentIndexInt, Index: index}, key.Value, true
		}
	}
	return segment.Segment{}, "", false
}

func (r Reader) memberReadReceiverHasUntrustedTopOrigin(point cfg.Point, obj ast.Expr) bool {
	if obj == nil {
		return false
	}
	value, ok := r.result.ExpressionValueAtBoundary(point, obj)
	return ok && r.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) exactLocalMissingFieldReadsNil(point cfg.Point, expr *ast.AttrGetExpr, name string) bool {
	if expr == nil || expr.Object == nil || name == "" {
		return false
	}
	value, ok := r.result.ExpressionValueBeforeBoundary(point, expr.Object)
	if !ok || !r.ValueHasLocalExclusiveExactIdentity(point, value) {
		return false
	}
	receiver, ok := r.ValueType(value)
	return ok && readmodelRecordProvablyMissesField(receiver, name)
}

func readmodelUnionArmRejectsFieldRead(t typ.Type, name string) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || typevalue.TypeIncludesNil(t) {
		return false
	}
	union, ok := unwrap.Annotated(t).(*typ.Union)
	if !ok || len(union.Members) < 2 {
		return false
	}
	carriesField := false
	rejectingArm := false
	for _, member := range union.Members {
		if _, ok := access.Field(member, name); ok {
			carriesField = true
			continue
		}
		if access.MissingFieldReadsNil(member) {
			continue
		}
		rejectingArm = true
	}
	return carriesField && rejectingArm
}

func readmodelFieldProvablyAbsent(t typ.Type, name string) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || typevalue.TypeIncludesNil(t) {
		return false
	}
	if _, ok := access.Field(t, name); ok {
		return false
	}
	return readmodelClosedRecordLacksField(t, name, 0)
}

func readmodelRecordProvablyMissesField(t typ.Type, name string) bool {
	return readmodelClosedRecordLacksField(t, name, 0)
}

func readmodelClosedRecordLacksField(t typ.Type, name string, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return readmodelClosedRecordWithoutField(v, name)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !readmodelClosedRecordLacksField(member, name, depth+1) {
				return false
			}
		}
		return true
	case *typ.Alias:
		return readmodelClosedRecordLacksField(v.UnaliasedTarget(), name, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return readmodelClosedRecordLacksField(v.Body, name, depth+1)
	default:
		return false
	}
}

func readmodelClosedRecordWithoutField(r *typ.Record, name string) bool {
	if r == nil || r.Open || r.HasMapComponent() || r.Metatable != nil {
		return false
	}
	if r.GetField(name) != nil {
		return false
	}
	if r.GetStaticStringIndex(name) != nil {
		return false
	}
	return true
}

func readmodelProjectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}

func (r Reader) expressionReceiverMethodCalleeReport(point cfg.Point, site factflow.CallSite) (CallCalleeReport, bool) {
	if !site.CalleeMemberAccess() || site.MethodName() == "" {
		return CallCalleeReport{}, false
	}
	if _, _, ok := site.CalleeMemberAccessPath(); ok {
		return CallCalleeReport{}, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok || receiver == nil || typ.IsAny(receiver) || typ.IsUnknown(receiver) || typ.IsNever(receiver) {
		return CallCalleeReport{}, false
	}
	if !typevalue.TypeIncludesNil(receiver) {
		return CallCalleeReport{}, false
	}
	return readapi.PlanCallCalleeReport(readapi.CallCalleeReportPlan{
		CallableName: r.callContractSourceName(site),
		Type:         receiver,
		Callable:     false,
		MemberAccess: true,
		Span:         sourceSpanFromFactflow(site.CalleeSpan()),
		CallSpan:     sourceSpanFromFactflow(site.CallSpan()),
	}), true
}

func (r Reader) memberReceiverNilableAtCall(point cfg.Point, site factflow.CallSite) (typ.Type, bool) {
	if site.MethodName() == "" {
		return nil, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok || receiver == nil || typ.IsAny(receiver) || typ.IsUnknown(receiver) || typ.IsNever(receiver) {
		return nil, false
	}
	if !typevalue.TypeIncludesNil(receiver) {
		return nil, false
	}
	return receiver, true
}

func calleeTypeCallable(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if _, ok := callcontract.Callable(t); ok {
		return true
	}
	if union, ok := t.(*typ.Union); ok && len(union.Members) != 0 {
		for _, member := range union.Members {
			if !calleeTypeCallable(member) {
				return false
			}
		}
		return true
	}
	return false
}

func calleeTypeCallableIgnoringNil(t typ.Type) bool {
	return calleeTypeCallable(unwrap.Optional(t))
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
	if fn, ok := r.memberCallFunctionType(point, site); ok {
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
			Source:                      CallContractSource{Kind: CallContractSourceFunctionValue, Name: r.callContractSourceName(site), ResultSpans: r.localFunctionReturnTypeSpans(site)},
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
		Source:                      CallContractSource{Kind: CallContractSourceLocalFunction, Name: r.callContractSourceName(site), ParameterSpans: r.localFunctionParamTypeSpans(site), ResultSpans: r.localFunctionReturnTypeSpans(site)},
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

func (r Reader) localFunctionReturnTypeSpans(site factflow.CallSite) []SourceSpan {
	if r.result == nil {
		return nil
	}
	if spans := r.result.FunctionReturnTypeSpansForCalleePath(site.View().CalleePathKey()); len(spans) != 0 {
		out := make([]SourceSpan, len(spans))
		for i, span := range spans {
			out[i] = sourceSpanFromFactflow(span)
		}
		return out
	}
	spans := r.result.FunctionReturnTypeSpansForTargetPath(site.CalleePathRef())
	if len(spans) == 0 {
		if fn, ok := r.result.FunctionBySymbol(site.CalleeSymbol()); ok && fn != nil {
			return functionReturnTypeSpans(fn)
		}
		if callee := site.CalleePathRef(); callee.Symbol != 0 {
			if fn, ok := r.result.FunctionBySymbol(callee.Symbol); ok && fn != nil {
				return functionReturnTypeSpans(fn)
			}
		}
		return nil
	}
	out := make([]SourceSpan, len(spans))
	for i, span := range spans {
		out[i] = sourceSpanFromFactflow(span)
	}
	return out
}

func functionReturnTypeSpans(fn *ast.FunctionExpr) []SourceSpan {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(fn.ReturnTypes))
	for i, ret := range fn.ReturnTypes {
		out[i] = sourceSpanFromAST(ast.SpanOf(ret))
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
	return false
}

func (r Reader) memberCallFunctionType(point cfg.Point, site factflow.CallSite) (*typ.Function, bool) {
	method, ok := memberCallableName(site)
	if !ok {
		return nil, false
	}
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil, false
	}
	fn, status, ok := callcontract.MemberCallable(receiver, method)
	return fn, ok && status == callcontract.MemberCallOK && fn != nil
}

func memberCallableName(site factflow.CallSite) (string, bool) {
	if method := site.MethodName(); method != "" {
		return method, true
	}
	_, member, ok := site.CalleeMemberAccessPath()
	if !ok {
		return "", false
	}
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return member.Name, member.Name != ""
	default:
		return "", false
	}
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
		return r.memberReceiverPathType(point, p)
	}
	if p, _, ok := site.CalleeMemberAccessPath(); ok && !p.IsEmpty() {
		return r.memberReceiverPathType(point, p)
	}
	return nil, false
}

func (r Reader) memberReceiverPathType(point cfg.Point, p path.Path) (typ.Type, bool) {
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

func sourceSpanValid(span SourceSpan) bool {
	return span.StartLine > 0 && span.StartCol > 0
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

func (r Reader) ValueHasExplicitTopOrigin(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return proof.New(r.result.Registry(), r.typeValues).ValueHasExplicitTopOrigin(value)
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
