package factapply

import (
	"fmt"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func rootAssignmentEvidenceTargetPath(target symbol.ID, targetPath pathdom.Path) (pathdom.Path, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return pathdom.Path{}, false
	}
	return rootAssignmentPath(root, targetPath), true
}

func moduloLengthIndexBaseSource(
	reg *axis.Registry,
	facts factflow.Facts,
	keySource factflow.ValueSource,
	tablePath pathdom.Path,
) (factflow.ValueSource, bool) {
	plus, ok := binaryExpressionOperation(facts, keySource, "+")
	if !ok {
		return factflow.ValueSource{}, false
	}
	var modSource factflow.ValueSource
	switch {
	case expressionSourceIsIntegerLiteral(reg, facts, plus.Right(), 1):
		modSource = plus.Left()
	case expressionSourceIsIntegerLiteral(reg, facts, plus.Left(), 1):
		modSource = plus.Right()
	default:
		return factflow.ValueSource{}, false
	}
	mod, ok := binaryExpressionOperation(facts, modSource, "%")
	if !ok || !expressionSourceIsLengthOfPath(facts, mod.Right(), tablePath) {
		return factflow.ValueSource{}, false
	}
	return mod.Left(), true
}

func binaryExpressionOperation(facts factflow.Facts, source factflow.ValueSource, op string) (factflow.ExpressionOperation, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ExpressionOperation{}, false
	}
	exprOp, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || exprOp.Kind() != factflow.ExpressionOperationBinary || exprOp.Op() != op {
		return factflow.ExpressionOperation{}, false
	}
	return exprOp, true
}

func expressionSourceIsIntegerLiteral(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource, want int64) bool {
	if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralInteger {
		return source.Int == want
	}
	if reg == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		return false
	}
	got, ok := typevalue.IntegerLiteralValue(reg, value)
	return ok && got == want
}

func expressionSourceIsLengthOfPath(facts factflow.Facts, source factflow.ValueSource, p pathdom.Path) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationUnary || op.Op() != "#" {
		return false
	}
	operand := op.Left()
	if operand.Kind != factflow.ValueSourceExpression || !operand.HasExpr {
		return false
	}
	got, ok := facts.ExpressionPathRef(operand.ExprRef)
	return ok && got.Equal(p)
}

func rootAssignmentTarget(target symbol.ID, targetPath pathdom.Path) (symbol.ID, bool) {
	if len(targetPath.Segments) != 0 {
		return 0, false
	}
	if target != 0 {
		return target, true
	}
	if targetPath.Symbol != 0 {
		return targetPath.Symbol, true
	}
	return 0, false
}

func rootAssignmentPath(target symbol.ID, targetPath pathdom.Path) pathdom.Path {
	out := targetPath
	if out.Symbol == 0 {
		out.Symbol = target
	}
	return out
}

func writeRootSymbol(ctx transfer.NodeContext, resolver *visibility.Resolver, out state.State, target symbol.ID, targetPath pathdom.Path, value product.Value, preserveIdempotentTarget bool) state.State {
	if target == 0 {
		return out
	}
	if resolver != nil && !(preserveIdempotentTarget && product.Equal(ctx.Registry, out.ReadValue(ctx.Registry, key.SymbolValue(target)), value)) {
		previousValue := out.ReadValue(ctx.Registry, key.SymbolValue(target))
		idempotent := product.Equal(ctx.Registry, previousValue, value)
		if invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
			out = invalidated
		}
		domain := state.RegisteredProductDomain(ctx.Registry)
		mutation, err := PrepareRootAssignmentStablePathEvidence(
			ctx.Registry, domain, resolver.KeySpace(), out.PathPresenceImplicationsSnapshot(resolver.KeySpace()), target, value, idempotent,
		)
		if err != nil {
			panic(fmt.Sprintf("factapply: seal stable-root path evidence: %v", err))
		}
		out, err = domain.ApplyStableRootPathEvidenceMutation(mutation, out)
		if err != nil {
			panic(fmt.Sprintf("factapply: apply stable-root path evidence: %v", err))
		}
	}
	domain := state.RegisteredProductDomain(ctx.Registry)
	write, err := domain.SealRootAssignmentValueWrite(key.SymbolValue(target), value)
	if err != nil {
		panic(fmt.Sprintf("factapply: seal root-assignment Values write: %v", err))
	}
	next, err := domain.ApplyRootAssignmentValueWrite(write, out)
	if err != nil {
		panic(fmt.Sprintf("factapply: apply root-assignment Values write: %v", err))
	}
	return next
}

func closedDynamicRootAssignmentMemberships(
	resolver *visibility.Resolver,
	targetPath pathdom.Path,
	freshEmptyTarget bool,
	freshEmptyPath func(pathdom.Path) bool,
	invariants []ClosedDynamicAllValueInvariant,
) []state.KeyMembership {
	if resolver == nil || len(invariants) == 0 || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return nil
	}
	targetRoot := pathdom.Path{Symbol: targetPath.Symbol}
	memberships := make([]state.KeyMembership, 0, len(invariants))
	for _, invariant := range invariants {
		applies := invariant.Container.Equal(targetRoot) && freshEmptyTarget
		if !applies && invariant.Table.Equal(targetRoot) && freshEmptyPath != nil {
			applies = freshEmptyPath(invariant.Container)
		}
		if !applies {
			continue
		}
		containerKey := resolver.KeySpace().FromPath(invariant.Container)
		if containerKey.Kind == keyspace.KindInvalid {
			continue
		}
		tableKey := resolver.KeySpace().FromPath(invariant.Table)
		if tableKey.Kind == keyspace.KindInvalid {
			continue
		}
		tableStateKey, ok := pathaddr.StateKeyFromPathKey(resolver.KeySpace().Format(tableKey))
		if !ok {
			continue
		}
		memberships = append(memberships, state.DynamicIndexAllValuesKeyMembership(containerKey, tableStateKey))
		if invariant.Site != "" {
			memberships = append(memberships, state.DynamicIndexValueKeyMembership(containerKey, invariant.Site, tableStateKey))
		}
	}
	return memberships
}

func rootPathHasFreshEmptyTable(domain state.ProductDomain, keys *keyspace.KeySpace, st state.State, root pathdom.Path) bool {
	if !domain.Valid() || keys == nil || root.Symbol == 0 || len(root.Segments) != 0 {
		return false
	}
	fresh, err := domain.RootAssignmentFreshEmptyState(keys, st, key.SymbolValue(root.Symbol))
	return err == nil && fresh
}

func rootPathDynamicValueKeyMembershipTables(reg *axis.Registry, st state.State, root pathdom.Path, container keyspace.Key) []pathaddr.StateKey {
	if reg == nil || root.Symbol == 0 || len(root.Segments) != 0 {
		return nil
	}
	value := st.ReadSymbolValue(reg, root.Symbol)
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return nil
	}
	object := st.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return nil
	}
	if !product.Equal(reg, object.Root(), value) || len(object.StaticMembers()) != 0 {
		return nil
	}
	return dynamicIndexValueCommonKeyMembershipTablesFromFacts(reg, st, container, object.DynamicIndexFacts())
}

func dynamicIndexExpressionKeyPath(resolver *visibility.Resolver, facts factflow.Facts, dyn factflow.DynamicIndexExpression) (pathdom.Path, bool) {
	p, ok := sourcePathFromValueSource(resolver, facts, dyn.KeySource())
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return p, true
}

func forEachDynamicIndexTableKeyAt(resolver *visibility.Resolver, point cfg.Point, tablePath pathdom.Path, fn func(keyspace.Key) bool) bool {
	if resolver == nil {
		return true
	}
	return visibility.AddressAt(resolver, point, tablePath).ForEachKeyspaceKey(fn,
		visibility.StateKeyVisible,
		visibility.StateKeyRootOrVisible,
	)
}

func dynamicIndexValueCommonKeyMembershipTablesFromFacts(reg *axis.Registry, st state.State, container keyspace.Key, facts map[dynamicindex.Key]dynamicindex.Fact) []pathaddr.StateKey {
	if len(facts) == 0 {
		return nil
	}
	domain := product.Domain(reg)
	common := map[pathaddr.StateKey]struct{}{}
	foundValueSource := false
	for dynamicKey, fact := range facts {
		if dynamicKey.Table != container || fact.Admission == dynamicindex.AdmissionRejected ||
			domain.Equal(fact.Value, domain.Bottom()) ||
			presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			continue
		}
		tables := st.DynamicIndexValueKeyMembershipTables(container, dynamicKey.Site)
		if len(tables) == 0 {
			return nil
		}
		if !foundValueSource {
			for _, table := range tables {
				common[table] = struct{}{}
			}
			foundValueSource = true
			continue
		}
		next := make(map[pathaddr.StateKey]struct{}, len(common))
		for _, table := range tables {
			if _, ok := common[table]; ok {
				next[table] = struct{}{}
			}
		}
		common = next
		if len(common) == 0 {
			return nil
		}
	}
	if !foundValueSource {
		return nil
	}
	out := make([]pathaddr.StateKey, 0, len(common))
	for table := range common {
		out = append(out, table)
	}
	return out
}

func prepareRootAssignmentCompletion(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	typeValues *typevalue.Cache,
	point cfg.Point,
	targetPath pathdom.Path,
	sourceValue product.Value,
	hasSourceValue bool,
	freshEmptyTarget bool,
	freshEmptyPath func(pathdom.Path) bool,
	invariants []ClosedDynamicAllValueInvariant,
) (state.RootAssignmentCompletion, error) {
	config := state.RootAssignmentCompletionConfig{}
	if resolver != nil && typeValues != nil && hasSourceValue && targetPath.Symbol != 0 && len(targetPath.Segments) == 0 {
		if t, ok := typeValues.TypeOf(reg, sourceValue); ok {
			// A length floor is a sparse keyed coordinate. Its bottom element
			// (zero: no positive lower bound) has no address; publish the key
			// and positive evidence atomically so the completion can never
			// contain a semantically meaningless half-coordinate.
			if floor := staticSequenceLengthFloor(t); floor > 0 {
				if targetKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey(); ok {
					if local, ok := resolver.KeySpace().InternStateKey(targetKey); ok {
						length, err := state.NewRootAssignmentLenFloor(local, floor)
						if err != nil {
							return state.RootAssignmentCompletion{}, err
						}
						config.LenFloor = length
					}
				}
			}
		}
	}
	config.KeyMemberships = closedDynamicRootAssignmentMemberships(resolver, targetPath, freshEmptyTarget, freshEmptyPath, invariants)
	return state.SealRootAssignmentCompletion(config)
}

func staticSequenceLengthFloor(t typ.Type) int64 {
	floor, productive := staticSequenceLengthFloorSeen(t, &typegraph.Path{})
	if !productive {
		return 0
	}
	return floor
}

func staticSequenceLengthFloorSeen(t typ.Type, active *typegraph.Path) (int64, bool) {
	if t == nil {
		return 0, true
	}
	if !active.Enter(t) {
		return 0, false
	}
	defer active.Leave(t)
	switch tt := t.(type) {
	case *typ.Annotated:
		return staticSequenceLengthFloorSeen(tt.Inner, active)
	case *typ.Alias:
		return staticSequenceLengthFloorSeen(tt.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return 0, false
		}
		return staticSequenceLengthFloorSeen(expanded, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return 0, false
		}
		return staticSequenceLengthFloorSeen(tt.Body, active)
	case *typ.Tuple:
		return int64(len(tt.Elements)), true
	case *typ.Record:
		var floor int64
		for i := int64(1); ; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return floor, true
			}
			floor = i
		}
	case *typ.Union:
		if len(tt.Members) == 0 {
			return 0, true
		}
		var min int64
		productive := false
		for _, member := range tt.Members {
			floor, memberProductive := staticSequenceLengthFloorSeen(member, active)
			if !memberProductive {
				continue
			}
			productive = true
			if floor == 0 {
				return 0, true
			}
			if min == 0 || floor < min {
				min = floor
			}
		}
		return min, productive
	default:
		return 0, true
	}
}

func staticSequenceExactLength(t typ.Type) (int64, bool) {
	length, exact, productive := staticSequenceExactLengthSeen(t, &typegraph.Path{})
	return length, exact && productive
}

func staticSequenceExactLengthSeen(t typ.Type, active *typegraph.Path) (int64, bool, bool) {
	if t == nil {
		return 0, false, true
	}
	if !active.Enter(t) {
		return 0, true, false
	}
	defer active.Leave(t)
	switch tt := t.(type) {
	case *typ.Annotated:
		return staticSequenceExactLengthSeen(tt.Inner, active)
	case *typ.Alias:
		return staticSequenceExactLengthSeen(tt.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return 0, true, false
		}
		return staticSequenceExactLengthSeen(expanded, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return 0, true, false
		}
		return staticSequenceExactLengthSeen(tt.Body, active)
	case *typ.Tuple:
		return int64(len(tt.Elements)), true, true
	case *typ.Union:
		if len(tt.Members) == 0 {
			return 0, false, true
		}
		var length int64
		productive := false
		for _, member := range tt.Members {
			memberLength, ok, memberProductive := staticSequenceExactLengthSeen(member, active)
			if !memberProductive {
				continue
			}
			if !ok || (productive && memberLength != length) {
				return 0, false, true
			}
			length = memberLength
			productive = true
		}
		return length, true, productive
	default:
		return 0, false, true
	}
}
