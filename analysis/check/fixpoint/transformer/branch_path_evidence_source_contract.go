package transformer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

type branchPathEvidenceSourceVerdict uint8

const (
	branchPathEvidenceSourceNotRegistered branchPathEvidenceSourceVerdict = iota
	branchPathEvidenceSourceEntailed
	branchPathEvidenceSourcePathMismatch
	branchPathEvidenceSourcePolarityMismatch
)

type branchPathEvidenceSourceContract struct {
	kind    factflow.BranchPathEvidenceKind
	entails func(planCompileContext, factflow.ValueSource, ValueTerm, factflow.BranchPathEvidence, bool) branchPathEvidenceSourceVerdict
}

// branchPathEvidenceSourceContracts is the exhaustive symbolic source law for
// proof kinds whose semantics are a relation, rather than a scalar refinement,
// over paths. Concrete edge application remains owned by factapply/State.
var branchPathEvidenceSourceContracts = [...]branchPathEvidenceSourceContract{
	{kind: factflow.BranchPathEvidenceIndexInRange, entails: indexInRangeEvidenceSourceEntails},
	{kind: factflow.BranchPathEvidenceFrozenTable, entails: frozenTableEvidenceSourceEntails},
}

func validateBranchPathEvidenceSource(
	ctx planCompileContext,
	conditionSource factflow.ValueSource,
	condition ValueTerm,
	proof factflow.BranchPathEvidence,
	predicateTruthy bool,
) branchPathEvidenceSourceVerdict {
	for _, contract := range branchPathEvidenceSourceContracts {
		if contract.kind == proof.Kind() {
			return contract.entails(ctx, conditionSource, condition, proof, predicateTruthy)
		}
	}
	return branchPathEvidenceSourceNotRegistered
}

// frozenTableEvidenceSourceEntails closes the binding-certified provenance
// carried by NewBranchFrozenTableEvidenceOnEdge. The fact producer has already
// proved the callee is the canonical table.isfrozen intrinsic; this contract
// proves that the selected Boolean result is from that exact call and that its
// sole argument is the same structural path receiving the persistent proof.
// No callee identity or path is reconstructed from names here.
func frozenTableEvidenceSourceEntails(
	ctx planCompileContext,
	_ factflow.ValueSource,
	_ ValueTerm,
	proof factflow.BranchPathEvidence,
	predicateTruthy bool,
) branchPathEvidenceSourceVerdict {
	producer, hasProducer := proof.ProducerPoint()
	if !hasProducer || proof.PathRef().IsEmpty() {
		return branchPathEvidenceSourcePathMismatch
	}
	if !predicateTruthy {
		return branchPathEvidenceSourcePolarityMismatch
	}
	site, exactSite := ctx.facts.CallSiteView(producer)
	point, exactPoint := site.Point()
	argument, exactArgument := site.ArgumentSourceAt(0)
	if !exactSite || !exactPoint || point != producer || site.Context() != factflow.CallSiteContextCondition ||
		!exactArgument || site.ArgumentSourceCount() != 1 ||
		!valueSourceNamesPath(ctx, argument, proof.PathRef()) {
		return branchPathEvidenceSourcePathMismatch
	}
	return branchPathEvidenceSourceEntailed
}

// indexInRangeEvidenceSourceEntails recognizes the canonical Boolean relation
// emitted for CheckIndexInRange: index <= #array, or its exact negation
// index > #array. Paths are matched against the registered ValueSource DAG;
// they are not re-read from a second row-local environment.
func indexInRangeEvidenceSourceEntails(
	ctx planCompileContext,
	conditionSource factflow.ValueSource,
	condition ValueTerm,
	proof factflow.BranchPathEvidence,
	predicateTruthy bool,
) branchPathEvidenceSourceVerdict {
	arrayPath, hasArray := proof.OtherPathRef()
	if !hasArray || proof.PathRef().IsEmpty() || arrayPath.IsEmpty() ||
		conditionSource.Kind != factflow.ValueSourceExpression || !conditionSource.HasExpr {
		return branchPathEvidenceSourcePathMismatch
	}
	if verdict := indexInRangeTermEntails(ctx, condition, proof.PathRef(), arrayPath, predicateTruthy); verdict != branchPathEvidenceSourcePathMismatch {
		return verdict
	}
	return indexInRangeSourceEntailsActive(
		ctx, conditionSource, proof.PathRef(), arrayPath, predicateTruthy,
		make(map[factflow.ExprRef]bool),
	)
}

func indexInRangeTermEntails(ctx planCompileContext, condition ValueTerm, indexPath, arrayPath pathdom.Path, truthy bool) branchPathEvidenceSourceVerdict {
	indexTerms := exactPredicatePathTerms(ctx, condition, indexPath)
	arrayTerms := exactPredicatePathTerms(ctx, condition, arrayPath)
	indexes := make(map[ValueTerm]struct{}, len(indexTerms))
	for _, term := range indexTerms {
		indexes[term] = struct{}{}
	}
	arrays := make(map[ValueTerm]struct{}, len(arrayTerms))
	for _, term := range arrayTerms {
		arrays[term] = struct{}{}
	}
	collectStructuralEnvironmentPathTerms(ctx.builder.Arena(), condition, indexPath, indexes, make(map[ValueTerm]bool))
	collectStructuralEnvironmentPathTerms(ctx.builder.Arena(), condition, arrayPath, arrays, make(map[ValueTerm]bool))
	if len(indexes) == 0 || len(arrays) == 0 {
		return branchPathEvidenceSourcePathMismatch
	}
	entailed, found := booleanTermEntailsIndexRange(ctx.builder.Arena(), condition, truthy, indexes, arrays, make(map[booleanEntailmentVisit]bool))
	if entailed {
		return branchPathEvidenceSourceEntailed
	}
	if found {
		return branchPathEvidenceSourcePolarityMismatch
	}
	return branchPathEvidenceSourcePathMismatch
}

func collectStructuralEnvironmentPathTerms(arena *Arena, root ValueTerm, path pathdom.Path, out map[ValueTerm]struct{}, active map[ValueTerm]bool) {
	if arena == nil || root == 0 || int(root) >= len(arena.values) || path.Symbol == 0 || path.Version != 0 || len(path.Segments) != 0 || active[root] {
		return
	}
	active[root] = true
	defer delete(active, root)
	node := arena.values[root]
	if node.op == valueEnvironment && node.slot == statekey.SymbolValue(path.Symbol) {
		out[root] = struct{}{}
	}
	for _, child := range node.args {
		collectStructuralEnvironmentPathTerms(arena, child, path, out, active)
	}
}

type booleanEntailmentVisit struct {
	term   ValueTerm
	truthy bool
}

func booleanTermEntailsIndexRange(arena *Arena, condition ValueTerm, truthy bool, indexes, arrays map[ValueTerm]struct{}, active map[booleanEntailmentVisit]bool) (entailed, found bool) {
	visit := booleanEntailmentVisit{term: condition, truthy: truthy}
	if arena == nil || condition == 0 || int(condition) >= len(arena.values) || active[visit] {
		return false, false
	}
	active[visit] = true
	defer delete(active, visit)
	node := arena.values[condition]
	if node.op == valueBinaryOperation && len(node.args) == 2 {
		_, indexMatch := indexes[node.args[0]]
		if !indexMatch {
			goto structural
		}
		right := arena.values[node.args[1]]
		if right.op == valueUnaryOperation && right.operator == "#" && len(right.args) == 1 {
			_, arrayMatch := arrays[right.args[0]]
			if !arrayMatch {
				goto structural
			}
			if node.operator == "<=" {
				return truthy, true
			}
			if node.operator == ">" {
				return !truthy, true
			}
		}
	}

structural:
	if node.op == valueUnaryOperation && node.operator == "not" && len(node.args) == 1 {
		return booleanTermEntailsIndexRange(arena, node.args[0], !truthy, indexes, arrays, active)
	}
	if node.op != valueBinaryOperation || len(node.args) != 2 ||
		(node.operator != "and" || !truthy) && (node.operator != "or" || truthy) {
		return false, found
	}
	leftEntailed, leftFound := booleanTermEntailsIndexRange(arena, node.args[0], truthy, indexes, arrays, active)
	if leftEntailed {
		return true, true
	}
	rightEntailed, rightFound := booleanTermEntailsIndexRange(arena, node.args[1], truthy, indexes, arrays, active)
	return rightEntailed, leftFound || rightFound
}

// indexInRangeSourceEntailsActive follows only Boolean contexts whose selected
// result entails a child result: truthy AND, falsy OR, and NOT with reversed
// polarity. This is exact short-circuit algebra, not a bounded syntax walk.
func indexInRangeSourceEntailsActive(
	ctx planCompileContext,
	source factflow.ValueSource,
	indexPath, arrayPath pathdom.Path,
	truthy bool,
	active map[factflow.ExprRef]bool,
) branchPathEvidenceSourceVerdict {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || active[source.ExprRef] {
		return branchPathEvidenceSourcePathMismatch
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)
	op, ok := ctx.facts.ExpressionOperation(source.ExprRef)
	if !ok {
		return branchPathEvidenceSourcePathMismatch
	}
	if op.Kind() == factflow.ExpressionOperationUnary && op.Op() == "not" {
		return indexInRangeSourceEntailsActive(ctx, op.Left(), indexPath, arrayPath, !truthy, active)
	}
	if op.Kind() != factflow.ExpressionOperationBinary {
		return branchPathEvidenceSourcePathMismatch
	}
	if op.Op() == "and" && truthy || op.Op() == "or" && !truthy {
		left := indexInRangeSourceEntailsActive(ctx, op.Left(), indexPath, arrayPath, truthy, active)
		if left == branchPathEvidenceSourceEntailed {
			return left
		}
		right := indexInRangeSourceEntailsActive(ctx, op.Right(), indexPath, arrayPath, truthy, active)
		if right == branchPathEvidenceSourceEntailed {
			return right
		}
		if left == branchPathEvidenceSourcePolarityMismatch || right == branchPathEvidenceSourcePolarityMismatch {
			return branchPathEvidenceSourcePolarityMismatch
		}
		return branchPathEvidenceSourcePathMismatch
	}
	if !valueSourceNamesPath(ctx, op.Left(), indexPath) || !valueSourceNamesLengthOfPath(ctx, op.Right(), arrayPath) {
		return branchPathEvidenceSourcePathMismatch
	}
	if op.Op() == "<=" && truthy || op.Op() == ">" && !truthy {
		return branchPathEvidenceSourceEntailed
	}
	if op.Op() == "<=" || op.Op() == ">" {
		return branchPathEvidenceSourcePolarityMismatch
	}
	return branchPathEvidenceSourcePathMismatch
}

func valueSourceNamesLengthOfPath(ctx planCompileContext, source factflow.ValueSource, want pathdom.Path) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	length, ok := ctx.facts.ExpressionOperation(source.ExprRef)
	return ok && length.Kind() == factflow.ExpressionOperationUnary && length.Op() == "#" &&
		valueSourceNamesPath(ctx, length.Left(), want)
}

func valueSourceNamesPath(ctx planCompileContext, source factflow.ValueSource, want pathdom.Path) bool {
	if want.IsEmpty() || want.Symbol == 0 {
		return false
	}
	switch source.Kind {
	case factflow.ValueSourcePath:
		got, ok := compilerResolverPath(source.PathKey)
		return ok && got.Equal(want)
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return false
		}
		got, ok := ctx.facts.ExpressionPathRef(source.ExprRef)
		return ok && got.Equal(want)
	default:
		return false
	}
}
