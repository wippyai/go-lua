package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/typenarrow"
	"github.com/wippyai/go-lua/analysis/domain/type/channelselect"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

// branchPathRelationDraft is syntax retained only until both structural paths
// and every affected registered factor have been sealed.
type branchPathRelationDraft struct {
	kind        factflow.BranchPathRelationKind
	left, right pathdom.Path
}

type branchChannelSelectRelation struct {
	result, casePath ResolvedStructuralPath
	resultRoot       BranchRelationValueRole
	refinement       *branchValueRefinementFactor
}

// branchPathRelationFactor is the remaining carrier-neutral path relation
// algebra. Ordinary equality already uses PathEqualityFactorProgram. This
// program owns runtime type comparisons, discriminant inequality, and the
// ChannelSelect dependent-result relation without reconstructing State.
type branchPathRelationFactor struct {
	kind                    factflow.BranchPathRelationKind
	left, right             ResolvedStructuralPath
	leftRoot, rightRoot     BranchRelationValueRole
	leftRefine, rightRefine *branchValueRefinementFactor
	equality                *branchPathEqualityFactor
	channel                 *branchChannelSelectRelation
	typeValues              *typevalue.Cache
}

func freezeBranchPathRelationFactor(
	b *branchProgramBuilder,
	coordinateUniverse state.CoordinateFactorInventory,
	draft *branchPathRelationDraft,
	seal *branchProgramSeal,
) (*branchPathRelationFactor, error) {
	if b == nil || draft == nil || seal == nil || draft.left.Symbol == 0 || draft.right.Symbol == 0 {
		return nil, fmt.Errorf("factapply: invalid branch path-relation declaration")
	}
	leftKey, leftOK := b.equalityKey(draft.left)
	rightKey, rightOK := b.equalityKey(draft.right)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("factapply: branch path relation is unresolved")
	}
	left, err := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), leftKey, draft.left.Symbol)
	if err != nil {
		return nil, fmt.Errorf("factapply: branch path-relation left operand: %w", err)
	}
	right, err := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), rightKey, draft.right.Symbol)
	if err != nil {
		return nil, fmt.Errorf("factapply: branch path-relation right operand: %w", err)
	}
	leftRoot, leftRoleOK := newBranchLexicalValueRole(seal, draft.left.Symbol)
	rightRoot, rightRoleOK := newBranchLexicalValueRole(seal, draft.right.Symbol)
	if !leftRoleOK || !rightRoleOK {
		return nil, fmt.Errorf("factapply: branch path relation has unbound Values roots")
	}
	out := &branchPathRelationFactor{
		kind: draft.kind, left: left, right: right, leftRoot: leftRoot, rightRoot: rightRoot,
		typeValues: b.authority.typeValues,
	}
	freezeRefinement := func(path pathdom.Path) (*branchValueRefinementFactor, error) {
		return freezeBranchValueRefinementFactor(b, coordinateUniverse, &branchValueRefinementDraft{path: path}, seal)
	}
	if result, casePath, resultIsLeft, ok := branchChannelSelectRelationPaths(draft.left, draft.right); ok &&
		(draft.kind == factflow.BranchPathRelationEqual || draft.kind == factflow.BranchPathRelationNotEqual) {
		resultKey, resultOK := b.equalityKey(result)
		caseKey, caseOK := b.equalityKey(casePath)
		if !resultOK || !caseOK {
			return nil, fmt.Errorf("factapply: channel-select path relation is unresolved")
		}
		resolvedResult, resultErr := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), resultKey, result.Symbol)
		if resultErr != nil {
			return nil, resultErr
		}
		resolvedCase, caseErr := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), caseKey, casePath.Symbol)
		if caseErr != nil {
			return nil, caseErr
		}
		refinement, refineErr := freezeRefinement(result)
		if refineErr != nil {
			return nil, refineErr
		}
		root := leftRoot
		if !resultIsLeft {
			root = rightRoot
		}
		out.channel = &branchChannelSelectRelation{
			result: resolvedResult, casePath: resolvedCase, resultRoot: root, refinement: refinement,
		}
	}
	switch draft.kind {
	case factflow.BranchPathRelationEqual:
		out.equality, err = freezeBranchPathEqualityFactor(b, coordinateUniverse, &branchPathEqualityDraft{
			left: leftKey, right: rightKey, leftSymbol: draft.left.Symbol, rightSymbol: draft.right.Symbol,
		}, seal)
	case factflow.BranchPathRelationTypeMatch, factflow.BranchPathRelationTypeUnmatch:
		out.leftRefine, err = freezeRefinement(draft.left)
	case factflow.BranchPathRelationNotEqual:
		if len(draft.left.Segments) != 0 {
			out.leftRefine, err = freezeRefinement(draft.left.RootOnly())
		}
		if err == nil && len(draft.right.Segments) != 0 {
			out.rightRefine, err = freezeRefinement(draft.right.RootOnly())
		}
	default:
		return nil, fmt.Errorf("factapply: unsupported residual branch path relation")
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func branchChannelSelectRelationPaths(left, right pathdom.Path) (pathdom.Path, pathdom.Path, bool, bool) {
	if result, ok := channelSelectResultPathFromChannel(left); ok {
		return result, right, true, true
	}
	if result, ok := channelSelectResultPathFromChannel(right); ok {
		return result, left, false, true
	}
	return pathdom.Path{}, pathdom.Path{}, false, false
}

func bindBranchPathRelationAccess(atom *branchAtom, relation *branchPathRelationFactor) error {
	if atom == nil || relation == nil {
		return nil
	}
	if relation.equality != nil {
		piece := branchAtom{equality: relation.equality}
		bindBranchPathEqualityAccess(&piece)
		atom.access = mergeBranchAtomAccess(atom.access, piece.access)
	}
	for _, refinement := range []*branchValueRefinementFactor{
		relation.leftRefine, relation.rightRefine,
		func() *branchValueRefinementFactor {
			if relation.channel != nil {
				return relation.channel.refinement
			}
			return nil
		}(),
	} {
		if refinement == nil {
			continue
		}
		piece := branchAtom{refinement: refinement}
		if err := bindBranchValueRefinementAccess(&piece); err != nil {
			return err
		}
		atom.access = mergeBranchAtomAccess(atom.access, piece.access)
	}
	return nil
}

func branchPathRelationKernel(relation *branchPathRelationFactor) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if relation == nil || current.plan == nil || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		frame := current
		if relation.equality != nil {
			patch, feasible, err := branchPathEqualityKernel(relation.equality)(runtime, BranchRelationFactorFrame{}, frame)
			if err != nil {
				return BranchRelationFactorPatch{}, false, err
			}
			frame = branchRelationFrameFromPatch(frame, patch)
			if !feasible {
				return branchRelationFramePatch(frame), false, nil
			}
		}
		switch relation.kind {
		case factflow.BranchPathRelationEqual:
			if relation.equality == nil {
				return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch path equality is absent")
			}
		case factflow.BranchPathRelationTypeMatch, factflow.BranchPathRelationTypeUnmatch:
			name, ok := resolveBranchRelationPath(runtime, frame, relation.right, relation.rightRoot)
			if !ok {
				return branchRelationIdentityPatch(frame), frame.reachable, nil
			}
			nameType, ok := typevalue.TypeOf(runtime.domain.Registry(), name)
			if !ok {
				return branchRelationIdentityPatch(frame), frame.reachable, nil
			}
			tag, ok := typenarrow.RuntimeKindTagForType(nameType)
			if !ok {
				return branchRelationIdentityPatch(frame), frame.reachable, nil
			}
			refinement := typenarrow.UnmatchRefinement(runtime.domain.Registry(), tag)
			if relation.kind == factflow.BranchPathRelationTypeMatch {
				refinement = typenarrow.MatchRefinement(runtime.domain.Registry(), tag)
			}
			return applyBranchRelationRefinement(runtime, frame, relation.leftRefine, refinement)
		case factflow.BranchPathRelationNotEqual:
			var err error
			if relation.leftRefine != nil {
				frame, err = applyBranchOriginInequality(runtime, frame, relation.left, relation.leftRoot, relation.right, relation.rightRoot, relation.leftRefine)
				if err != nil || !frame.reachable {
					return branchRelationFramePatch(frame), frame.reachable, err
				}
			}
			if relation.rightRefine != nil {
				frame, err = applyBranchOriginInequality(runtime, frame, relation.right, relation.rightRoot, relation.left, relation.leftRoot, relation.rightRefine)
			}
		default:
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: invalid residual branch path relation")
		}
		if relation.channel != nil {
			return applyBranchChannelSelectRelation(runtime, frame, relation)
		}
		return branchRelationFramePatch(frame), frame.reachable, nil
	}
}

func resolveBranchRelationPath(runtime branchAtomFactorRuntime, frame BranchRelationFactorFrame, path ResolvedStructuralPath, root BranchRelationValueRole) (product.Value, bool) {
	rootValue := product.Bottom(runtime.domain.Registry())
	if frame.valuesTop {
		rootValue = product.Top()
	} else {
		for index, role := range frame.plan.layout.currentValues {
			if role == root {
				rootValue = frame.values[index]
				break
			}
		}
	}
	reader := branchResolvedPathReader{
		domain: runtime.domain, keys: path.keys, root: path.root, value: rootValue,
		lanes: make(map[state.LaneID]state.LaneFactor, len(frame.lanes)),
	}
	for _, lane := range frame.lanes {
		reader.lanes[lane.Lane().ID()] = lane
	}
	return ResolveStructuralPathFactorValue(runtime.domain.Registry(), reader, path)
}

func applyBranchOriginInequality(
	runtime branchAtomFactorRuntime,
	current BranchRelationFactorFrame,
	parent ResolvedStructuralPath,
	parentRoot BranchRelationValueRole,
	constraint ResolvedStructuralPath,
	constraintRoot BranchRelationValueRole,
	program *branchValueRefinementFactor,
) (BranchRelationFactorFrame, error) {
	rootPath, ok := resolvedStructuralPrefix(parent, 0)
	if !ok {
		return current, fmt.Errorf("factapply: invalid discriminant relation root")
	}
	root, rootOK := resolveBranchRelationPath(runtime, current, rootPath, parentRoot)
	constraintValue, constraintOK := resolveBranchRelationPath(runtime, current, constraint, constraintRoot)
	if !rootOK || !constraintOK {
		return current, nil
	}
	origin, ok := typevalue.VariantOriginOfValue(runtime.domain.Registry(), program.program.typeValues, root)
	if !ok {
		return current, nil
	}
	cases, ok := narrowOriginCasesByPathConstraint(
		program.program.typeValues, runtime.domain.Registry(), origin, parent.segments, constraintValue, false,
	)
	if !ok {
		return current, nil
	}
	if len(cases) == 0 {
		current.reachable = false
		return current, nil
	}
	narrowed := product.Set(runtime.domain.Registry(), root, variantorigin.Key, variantorigin.Of(origin.Family(), cases))
	patch, _, err := applyBranchRelationRefinement(runtime, current, program, factflow.NewValueConstraint(narrowed))
	if err != nil {
		return current, err
	}
	return branchRelationFrameFromPatch(current, patch), nil
}

func narrowOriginCasesByPathConstraint(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	rootOrigin variantorigin.Value,
	suffix []segment.Segment,
	constraint product.Value,
	equal bool,
) ([]int, bool) {
	if constraintOrigin, ok := typevalue.VariantOriginOfValue(reg, typeValues, constraint); ok {
		return variant.NarrowOriginByPathView(
			rootOrigin.Family(), rootOrigin.CasesView(), suffix,
			constraintOrigin.Family(), constraintOrigin.CasesView(), equal,
		)
	}
	if constraintType, ok := typevalue.TypeOf(reg, constraint); ok {
		return variant.NarrowOriginByPathTypeView(
			rootOrigin.Family(), rootOrigin.CasesView(), suffix, constraintType, equal,
		)
	}
	return nil, false
}

func applyBranchRelationRefinement(
	runtime branchAtomFactorRuntime,
	current BranchRelationFactorFrame,
	program *branchValueRefinementFactor,
	refinement factflow.ValueRefinement,
) (BranchRelationFactorPatch, bool, error) {
	if program == nil {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch relation refinement is absent")
	}
	bound := *program
	bound.program = program.program
	bound.program.refinement = refinement
	return branchValueRefinementKernel(&bound)(runtime, BranchRelationFactorFrame{}, current)
}

func branchRelationFrameFromPatch(current BranchRelationFactorFrame, patch BranchRelationFactorPatch) BranchRelationFactorFrame {
	if len(patch.values) != 0 {
		current.values = append([]product.Value(nil), patch.values...)
		current.valuesTop = patch.valuesTop
	}
	if len(patch.lanes) != 0 {
		current.lanes = append([]state.LaneFactor(nil), patch.lanes...)
	}
	if len(patch.coordinates) != 0 {
		current.coordinates = cloneBranchCoordinateOperands(patch.coordinates)
	}
	current.reachable = patch.reachable
	return current
}

func branchRelationFramePatch(frame BranchRelationFactorFrame) BranchRelationFactorPatch {
	return BranchRelationFactorPatch{
		plan: frame.plan, values: append([]product.Value(nil), frame.values...), valuesTop: frame.valuesTop,
		lanes: append([]state.LaneFactor(nil), frame.lanes...), coordinates: cloneBranchCoordinateOperands(frame.coordinates),
		reachable: frame.reachable,
	}
}

func branchRelationIdentityPatch(frame BranchRelationFactorFrame) BranchRelationFactorPatch {
	return branchRelationFramePatch(frame)
}

func applyBranchChannelSelectRelation(runtime branchAtomFactorRuntime, current BranchRelationFactorFrame, relation *branchPathRelationFactor) (BranchRelationFactorPatch, bool, error) {
	channel := relation.channel
	if channel == nil {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: channel-select relation is absent")
	}
	var snapshot state.ChannelSelectFactsSnapshot
	foundLane := false
	for _, lane := range current.lanes {
		if lane.Lane().ID() != state.LaneChannelSelect {
			continue
		}
		var err error
		snapshot, err = runtime.domain.ChannelSelectFactsFactorSnapshot(lane)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		foundLane = true
		break
	}
	if !foundLane || snapshot.Bottom {
		return branchRelationIdentityPatch(current), current.reachable, nil
	}
	aliases := []keyspace.Key{channel.casePath.local}
	if carrier, ok := openBranchRelationPathCarrier(runtime, current); ok {
		if more, valid := carrier.EquivalentKeys(channel.casePath.local); valid {
			aliases = append(aliases, more...)
		}
	}
	resultKey := pathaddr.StateKey(pathdom.PathKey(channel.result.keys.FormatReadOnly(channel.result.local)))
	caseKeys := make([]pathaddr.StateKey, 0, len(aliases))
	for _, alias := range aliases {
		caseKeys = append(caseKeys, pathaddr.StateKey(pathdom.PathKey(channel.casePath.keys.FormatReadOnly(alias))))
	}
	facts := make([]channelselectfact.Fact, 0)
	for _, fact := range snapshot.Facts {
		if fact.Kind != channelselectfact.FactReceive || !channelSelectStateKeyMatches(fact.Result, resultKey) {
			continue
		}
		for _, candidate := range caseKeys {
			if channelSelectStateKeyMatches(fact.Case, candidate) {
				facts = append(facts, fact)
				break
			}
		}
	}
	if len(facts) == 0 {
		return branchRelationIdentityPatch(current), current.reachable, nil
	}
	result, resultOK := resolveBranchRelationPath(runtime, current, channel.result, channel.resultRoot)
	if !resultOK {
		return branchRelationIdentityPatch(current), current.reachable, nil
	}
	resultType, hasType := valueWitnessType(runtime.domain.Registry(), result)
	var narrowed product.Value
	switch relation.kind {
	case factflow.BranchPathRelationEqual:
		var unionTypes []typ.Type
		missingKnown := false
		if hasType {
			for _, fact := range facts {
				caseType, ok := channelselect.ResultCaseTypeFromValue(resultType, string(fact.Select), fact.Index)
				if ok {
					if payload, payloadOK := channelSelectExactPayloadType(runtime.domain.Registry(), fact); payloadOK {
						caseType = channelselect.ResultCaseType(string(fact.Select), fact.Index, payload)
					}
					unionTypes = append(unionTypes, caseType)
				} else if channelselect.ResultHasSelectID(resultType, string(fact.Select)) {
					missingKnown = true
				}
			}
			if len(unionTypes) == 0 && missingKnown {
				current.reachable = false
				return branchRelationFramePatch(current), false, nil
			}
		} else {
			for _, fact := range facts {
				unionTypes = append(unionTypes, channelSelectPayloadCaseType(runtime.domain.Registry(), fact))
			}
		}
		if len(unionTypes) == 0 {
			return branchRelationIdentityPatch(current), current.reachable, nil
		}
		narrowed = relation.typeValues.FromTypeWithWitness(runtime.domain.Registry(), typeexpr.Union(unionTypes...))
	case factflow.BranchPathRelationNotEqual:
		var next typ.Type
		if hasType {
			next = resultType
			removed := false
			for _, fact := range facts {
				candidate, ok := channelselect.ResultWithoutCase(next, string(fact.Select), fact.Index)
				if ok {
					next = channelSelectTightenRemainingTypeFromSnapshot(runtime.domain.Registry(), snapshot, candidate, fact.Select)
					removed = true
				}
			}
			if !removed {
				return branchRelationIdentityPatch(current), current.reachable, nil
			}
		} else {
			var ok bool
			next, ok = channelSelectRemainingTypeFromSnapshot(runtime.domain.Registry(), snapshot, facts[0].Select, channelSelectFactIndexes(facts))
			if !ok {
				return branchRelationIdentityPatch(current), current.reachable, nil
			}
		}
		narrowed = relation.typeValues.FromTypeWithWitness(runtime.domain.Registry(), next)
	default:
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: invalid channel-select path relation")
	}
	return applyBranchRelationRefinement(runtime, current, channel.refinement, factflow.NewValueConstraint(narrowed))
}

func openBranchRelationPathCarrier(runtime branchAtomFactorRuntime, current BranchRelationFactorFrame) (*state.CoordinatePathEvidenceCarrier[BranchRelationValueRole], bool) {
	family, ok := runtime.domain.PathEvidenceCoordinateFamily()
	if !ok {
		return nil, false
	}
	for index, layout := range current.plan.layout.currentCoordinates {
		if layout.family != family {
			continue
		}
		group := current.coordinates[index]
		carrier, err := state.OpenCoordinatePathEvidenceCarrier(
			runtime.domain, group.Skeleton, group.Scalars,
			state.ValueFactor[BranchRelationValueRole]{}, current.reachable,
			current.plan.pathReadAuthority, state.PathDescendantMutationFactors{},
		)
		return carrier, err == nil
	}
	return nil, false
}

func channelSelectRemainingTypeFromSnapshot(reg *axis.Registry, snapshot state.ChannelSelectFactsSnapshot, selectID channelselectfact.ID, skip map[int]bool) (typ.Type, bool) {
	if snapshot.Bottom {
		return nil, false
	}
	cases := make([]channelselect.ResultCase, 0)
	hasDefault := false
	for _, fact := range snapshot.Facts {
		if fact.Kind == channelselectfact.FactSelect && fact.Select == selectID && fact.HasDefault {
			hasDefault = true
			continue
		}
		if fact.Kind == channelselectfact.FactReceive && fact.Select == selectID && !skip[fact.Index] {
			cases = append(cases, channelselect.ResultCase{Index: fact.Index, Payload: channelSelectPayloadType(reg, fact)})
		}
	}
	if len(cases) == 0 && !hasDefault {
		return typ.Never, true
	}
	return channelselect.ResultValueTypeWithDefault(string(selectID), cases, hasDefault)
}

func channelSelectTightenRemainingTypeFromSnapshot(reg *axis.Registry, snapshot state.ChannelSelectFactsSnapshot, resultType typ.Type, selectID channelselectfact.ID) typ.Type {
	if !channelselect.ResultHasSelectID(resultType, string(selectID)) || snapshot.Bottom {
		return resultType
	}
	cases := make([]channelselect.ResultCase, 0)
	hasDefault := false
	for _, fact := range snapshot.Facts {
		if fact.Kind == channelselectfact.FactSelect && fact.Select == selectID && fact.HasDefault {
			_, hasDefault = channelselect.ResultCaseTypeFromValue(resultType, string(selectID), channelselect.DefaultCaseIndex)
			continue
		}
		if fact.Kind != channelselectfact.FactReceive || fact.Select != selectID {
			continue
		}
		caseType, ok := channelselect.ResultCaseTypeFromValue(resultType, string(selectID), fact.Index)
		if !ok {
			continue
		}
		payload := typ.Unknown
		if current, ok := channelselect.ResultCasePayloadType(caseType); ok {
			payload = current
		}
		if exact, ok := channelSelectExactPayloadType(reg, fact); ok {
			payload = exact
		}
		cases = append(cases, channelselect.ResultCase{Index: fact.Index, Payload: payload})
	}
	if len(cases) == 0 && !hasDefault {
		return resultType
	}
	tightened, ok := channelselect.ResultValueTypeWithDefault(string(selectID), cases, hasDefault)
	if !ok {
		return resultType
	}
	return tightened
}
