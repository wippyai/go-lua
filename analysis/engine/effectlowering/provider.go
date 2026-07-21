// Package effectlowering lowers declared signature effects into factflow facts
// and factapply call outcomes.
package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureNameFunc maps one call producer in context to a stable signature name.
type SignatureNameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureSiteNameFunc maps read-only call-site evidence in context to a
// stable signature name without materializing a producer DTO.
type SignatureSiteNameFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (string, bool)

// SignatureSiteIntrinsicFunc projects a sealed semantic intrinsic identity
// from the same lexical binding authority used for signature resolution.
// Implementations must fail closed for shadowed, replaced, imported, or
// otherwise non-canonical bindings.
type SignatureSiteIntrinsicFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (signature.Intrinsic, bool)

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// immutableSignatureLookup is implemented by bounded sources that can lend a
// read-only signature. SignatureOutcomeProvider never mutates a looked-up
// signature: receiver binding and generic substitution replace carrier fields
// with newly constructed values. Borrowing therefore removes deep ownership
// clones from fixed-point transfers while preserving SignatureLookup.Lookup's
// ownership contract for all other users.
type immutableSignatureLookup interface {
	LookupView(name string) (signature.Function, bool)
}

// SignatureOutcomeProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureOutcomeProviderConfig struct {
	Signatures       SignatureLookup
	NameFor          SignatureNameFunc
	NameForSite      SignatureSiteNameFunc
	IntrinsicForSite SignatureSiteIntrinsicFunc
	ReturnTypeOps    ReturnTypeOps
	TypeValues       *typevalue.Cache
	Facts            factflow.Facts
	ArgumentTypes    SignatureArgumentTypeProgram
	ReturnValues     SignatureReturnValueProgram
	// KeySpace is the consuming (caller) analysis keyspace into which rehydrated
	// heap allocation templates intern their rootless static-member keys.
	KeySpace *keyspace.KeySpace
	// InputProgram is the registered non-scalar query authority shared with
	// the canonical relation executor. Signature lowering extends it only for
	// queries required by the selected signature.
	InputProgram SignatureOutcomeInputProgram
}

// SignatureOutcomeProvider materializes declared signature return types into
// call outcome return slots.
func SignatureOutcomeProvider(config SignatureOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	signatures := config.Signatures
	nameFor := config.NameFor
	nameForSite := config.NameForSite
	returnTypeOps := config.ReturnTypeOps
	typeValues := config.TypeValues
	facts := config.Facts
	argumentTypes := config.ArgumentTypes
	returnValues := config.ReturnValues
	providerKeySpace := config.KeySpace
	baseInputProgram := config.InputProgram
	maximumExtensionInputProgram, inputErr := UnionSignatureOutcomeInputPrograms(
		mustSignatureInputProgram(argumentTypes.maximumInputProgram()),
		mustSignatureInputProgram(returnValues.maximumInputProgram()),
	)
	if inputErr != nil {
		panic(inputErr)
	}
	maximumInputProgram, inputErr := UnionSignatureOutcomeInputPrograms(baseInputProgram)
	if inputErr != nil {
		panic(inputErr)
	}
	if baseInputProgram.Valid() {
		maximumInputProgram, inputErr = maximumInputProgram.WithHeapMemberQuery()
		if inputErr != nil {
			panic(inputErr)
		}
	}
	maximumInputProgram, inputErr = UnionSignatureOutcomeInputPrograms(maximumInputProgram, maximumExtensionInputProgram)
	if inputErr != nil {
		panic(inputErr)
	}
	lookupSignature := signatures.Lookup
	if views, ok := signatures.(immutableSignatureLookup); ok {
		lookupSignature = views.LookupView
	}
	prepare := func(freezeCtx transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSitePreparation, error) {
		if signatures == nil {
			return callpayload.CallOutcomeSitePreparation{}, nil
		}
		name, named := signatureNameForSite(freezeCtx, site, nameForSite, nameFor)
		var frozenSignature signature.Function
		var frozenInputProgram SignatureOutcomeInputProgram
		var frozenArgumentTypes PreparedSignatureArgumentTypeProgram
		var frozenReturnValues PreparedSignatureReturnValueProgram
		var shape callpayload.CallOutcomeSiteShape
		if !named {
			// Receiver-derived member signatures require input State. Keep the
			// exact provider-family maximum only at those unresolved sites.
			if site.MethodName() == "" {
				return callpayload.CallOutcomeSitePreparation{}, nil
			}
			shape = callpayload.CallOutcomeSiteShape{
				FieldNames: signatureOutcomeMaximumFields, Correlations: maximumReturnPresenceShapes(site),
			}
		} else {
			var ok bool
			frozenSignature, ok = lookupSignature(name)
			if !ok {
				return callpayload.CallOutcomeSitePreparation{}, nil
			}
			var err error
			frozenInputProgram, err = signatureOutcomeIntrinsicInputProgram(baseInputProgram, frozenSignature)
			if err != nil {
				return callpayload.CallOutcomeSitePreparation{}, err
			}
			resolvedSite := SignatureOutcomeSite{Site: site, Name: name, Signature: frozenSignature}
			frozenArgumentTypes, err = argumentTypes.PrepareSite(resolvedSite)
			if err != nil {
				return callpayload.CallOutcomeSitePreparation{}, err
			}
			frozenReturnValues, err = returnValues.PrepareSite(resolvedSite)
			if err != nil {
				return callpayload.CallOutcomeSitePreparation{}, err
			}
			frozenInputProgram, err = UnionSignatureOutcomeInputPrograms(
				frozenInputProgram,
				mustSignatureInputProgram(frozenArgumentTypes.InputProgram()),
				mustSignatureInputProgram(frozenReturnValues.InputProgram()),
			)
			if err != nil {
				return callpayload.CallOutcomeSitePreparation{}, err
			}
			correlations := signatureOutcomeCorrelationShapes(frozenSignature, site)
			shape = callpayload.CallOutcomeSiteShape{
				FieldNames: signatureOutcomeFields(frozenSignature, !frozenReturnValues.Empty()),
				InputLanes: frozenInputProgram.Lanes(), Correlations: correlations,
				ProofSeeds: signatureOutcomeProofSeeds(frozenSignature),
			}
		}
		// Both possible formal argument orderings are structural properties of
		// the lexical call site. Receiver type decides between them at runtime,
		// but constructing either reader inside the fixed point is redundant.
		directArguments := signatureArgumentSources(freezeCtx, facts, site, providerKeySpace)
		methodArguments := signatureMethodArgumentSources(freezeCtx, facts, site, providerKeySpace)
		evaluate := func(ctx transfer.NodeContext, baseInput callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			input := SignatureOutcomeInput{input: baseInput}
			var receiverType typ.Type
			sig, inputProgram, ok := frozenSignature, frozenInputProgram, named
			preparedArgumentTypes, preparedReturnValues := frozenArgumentTypes, frozenReturnValues
			if named && site.MethodName() != "" {
				receiverType, _ = receiverTypeForMethodSite(ctx, input)
			}
			if !named {
				name, receiverType, ok = receiverMethodSignatureName(ctx, site, input)
				if !ok {
					return callpayload.CallOutcome{}, nil
				}
				sig, ok = lookupSignature(name)
				if !ok {
					return callpayload.CallOutcome{}, nil
				}
				var err error
				inputProgram, err = signatureOutcomeIntrinsicInputProgram(baseInputProgram, sig)
				if err != nil {
					return callpayload.CallOutcome{}, err
				}
				resolvedSite := SignatureOutcomeSite{Site: site, Name: name, Signature: sig}
				preparedArgumentTypes, err = argumentTypes.PrepareSite(resolvedSite)
				if err != nil {
					return callpayload.CallOutcome{}, err
				}
				preparedReturnValues, err = returnValues.PrepareSite(resolvedSite)
				if err != nil {
					return callpayload.CallOutcome{}, err
				}
				inputProgram, err = UnionSignatureOutcomeInputPrograms(
					inputProgram,
					mustSignatureInputProgram(preparedArgumentTypes.InputProgram()),
					mustSignatureInputProgram(preparedReturnValues.InputProgram()),
				)
				if err != nil {
					return callpayload.CallOutcome{}, err
				}
			}
			input, err := inputProgram.Bind(baseInput)
			if err != nil {
				return callpayload.CallOutcome{}, err
			}
			sig = bindReceiverSelfSignature(sig, receiverType)
			argSources := directArguments
			if site.MethodName() != "" && signatureConsumesReceiver(sig.Type, receiverType) {
				argSources = methodArguments
			}
			sig = instantiateSignatureForCall(ctx, baseInput, preparedArgumentTypes, sig, argSources, returnTypeOps)
			out := callpayload.CallOutcome{}
			if sig.OperationalEffects != nil {
				out.SuspensionKnown = sig.OperationalEffects.SuspensionKnown
				out.MaySuspend = sig.OperationalEffects.MaySuspend
			}
			if sig.OperationalEffects != nil && !sig.OperationalEffects.IsEmpty() && !operationalEffectsOnlyMaySuspend(sig.OperationalEffects) {
				out = applyOperationalEffects(ctx, out, operationalEffectContext{
					effects:       sig.OperationalEffects,
					signatureType: sig.Type,
					argSources:    argSources,
					input:         baseInput,
					keySpace:      providerKeySpace,
					typeValues:    typeValues,
				})
			} else {
				invalidations := signatureParamPathInvalidationsForReader(sig, argSources)
				lengthFloors := signatureParamLengthFloorsForReader(sig, argSources)
				mutations := signatureParamMutationEffectsForReader(ctx, sig, argSources, baseInput, preparedArgumentTypes, typeValues)
				out.ReturnPresenceRelations = signatureReturnPresenceRelations(sig)
				out.ParamObligations = mutations.Obligations
				out.ParamPathRefinements = signatureParamPathRefinementsForReader(ctx, sig, argSources)
				out.ParamPathWrites = mutations.Writes
				out.ParamLengthFloors = lengthFloors
				out.ParamPathInvalidations = invalidations
				applySignatureNormalReturnFacts(sig, argSources, mutations, &out.NormalReturnFacts)
			}
			out.ReturnPresenceRelations = observableReturnPresenceRelations(site, out.ReturnPresenceRelations)
			if sig.Type == nil || len(sig.Type.Returns) == 0 {
				out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
				return out, nil
			}
			results := make([]callpayload.CallResult, 0, len(sig.Type.Returns))
			for i, ret := range sig.Type.Returns {
				value, ok := product.Value{}, false
				if !preparedReturnValues.Empty() {
					value, ok, err = preparedReturnValues.evaluate(baseInput, SignatureReturnValueInputContext{Node: ctx, Site: site, Name: name, Index: i})
					if err != nil {
						return callpayload.CallOutcome{}, err
					}
				}
				if !ok {
					value, ok = operationalReturnFlowValue(ctx, facts, providerKeySpace, sig, i, argSources, baseInput, input, typeValues)
				}
				if !ok {
					value, ok = signatureReturnValue(ctx, sig, i, argSources, baseInput, returnTypeOps, typeValues)
				}
				if !ok && ret != nil {
					value, ok = returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, ret), true
				}
				if !ok {
					continue
				}
				value = operationalReturnAllocationValue(ctx.Registry, typeValues, sig.OperationalEffects, sig.Type, ctx.Point, i, value)
				results = append(results, callpayload.CallResult{
					Index: i,
					Value: value,
				})
			}
			if !preparedReturnValues.Empty() {
				present := make(map[int]struct{}, len(results))
				for _, result := range results {
					present[result.Index] = struct{}{}
				}
				var resultErr error
				site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
					index := target.ResultIndex()
					if index < len(sig.Type.Returns) {
						return true
					}
					if _, ok := present[index]; ok {
						return true
					}
					value, ok, err := preparedReturnValues.evaluate(baseInput, SignatureReturnValueInputContext{Node: ctx, Site: site, Name: name, Index: index})
					if err != nil {
						resultErr = err
						return false
					}
					if !ok {
						return true
					}
					present[index] = struct{}{}
					results = append(results, callpayload.CallResult{
						Index: index,
						Value: value,
					})
					return true
				})
				if resultErr != nil {
					return callpayload.CallOutcome{}, resultErr
				}
			}
			out.Results = results
			out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
			return out, nil
		}
		return callpayload.CallOutcomeSitePreparation{Shape: shape, Evaluate: evaluate}, nil
	}
	return callpayload.SealPreparedCallOutcomeProgram(
		"signature outcome", signatureOutcomeMaximumFields,
		maximumInputProgram.Lanes(), state.LaneSet{}, prepare,
	)
}

func observableReturnPresenceRelations(site factflow.CallSiteView, in []callpayload.CallReturnPresenceRelation) []callpayload.CallReturnPresenceRelation {
	observable := make(map[int]struct{})
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		observable[target.ResultIndex()] = struct{}{}
		return true
	})
	out := in[:0]
	for _, relation := range in {
		if _, ok := observable[relation.TriggerIndex]; !ok {
			continue
		}
		if _, ok := observable[relation.TargetIndex]; !ok {
			continue
		}
		out = append(out, relation)
	}
	return out
}

var signatureOutcomeMaximumFields = []string{
	"Results", "PostReturnAuthority", "SuspensionKnown", "MaySuspend",
	"NormalReturnFacts", "HeapTableObjects", "Placements", "ParamObligations",
	"TypestateRequirements", "ParamPathRefinements", "ParamPathWrites",
	"ParamLengthFloors", "ParamPathInvalidations", "ReturnPresenceRelations",
}

func signatureOutcomeCorrelationShapes(sig signature.Function, site factflow.CallSiteView) []callpayload.CallOutcomeCorrelationShape {
	var relations []callpayload.CallReturnPresenceRelation
	if effects := sig.OperationalEffects; effects != nil && !effects.IsEmpty() && !operationalEffectsOnlyMaySuspend(effects) {
		relations = operationalReturnPresenceRelations(*effects)
	} else {
		relations = signatureReturnPresenceRelations(sig)
	}
	observable := make(map[int]struct{})
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		observable[target.ResultIndex()] = struct{}{}
		return true
	})
	out := make([]callpayload.CallOutcomeCorrelationShape, 0, len(relations))
	for _, relation := range relations {
		if _, ok := observable[relation.TriggerIndex]; !ok {
			continue
		}
		if _, ok := observable[relation.TargetIndex]; !ok {
			continue
		}
		out = append(out, callpayload.ReturnPresenceShape(relation.TriggerIndex, relation.TriggerPresence, relation.TargetIndex, relation.TargetPresence))
	}
	return out
}

func maximumReturnPresenceShapes(site factflow.CallSiteView) []callpayload.CallOutcomeCorrelationShape {
	var indices []int
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() >= 0 {
			indices = append(indices, target.ResultIndex())
		}
		return true
	})
	values := []presence.Value{presence.Absent(), presence.Present()}
	out := make([]callpayload.CallOutcomeCorrelationShape, 0, len(indices)*len(indices)*4)
	for _, trigger := range indices {
		for _, target := range indices {
			for _, triggerPresence := range values {
				for _, targetPresence := range values {
					out = append(out, callpayload.ReturnPresenceShape(trigger, triggerPresence, target, targetPresence))
				}
			}
		}
	}
	return out
}

func signatureOutcomeProofSeeds(sig signature.Function) []callpayload.CallOutcomeProofSeed {
	if sig.OperationalEffects == nil {
		return nil
	}
	seeds := make([]callpayload.CallOutcomeProofSeed, 0, len(sig.OperationalEffects.NormalReturnPresenceRefinements))
	for _, refinement := range sig.OperationalEffects.NormalReturnPresenceRefinements {
		seeds = append(seeds, callpayload.NormalReturnPathPresenceProofSeed(refinement.Path, refinement.Presence))
	}
	return seeds
}

func signatureOutcomeFields(sig signature.Function, customReturn bool) []string {
	fields := make([]string, 0, len(signatureOutcomeMaximumFields))
	postReturn := false
	if customReturn || sig.Type != nil && len(sig.Type.Returns) != 0 {
		fields = append(fields, "Results")
	}
	if effects := sig.OperationalEffects; effects != nil && !effects.IsEmpty() && !operationalEffectsOnlyMaySuspend(effects) {
		if effects.SuspensionKnown {
			fields = append(fields, "SuspensionKnown")
		}
		if effects.MaySuspend {
			fields = append(fields, "MaySuspend")
		}
		if len(effects.ReturnPresenceRelations) != 0 {
			fields = append(fields, "ReturnPresenceRelations")
			postReturn = true
		}
		if len(effects.TypestateRequirements) != 0 {
			fields = append(fields, "TypestateRequirements")
		}
		if len(effects.ReturnAllocationTemplates) != 0 {
			fields = append(fields, "HeapTableObjects", "Placements")
			postReturn = true
		}
		if operationalEffectsProduceNormalReturnFacts(effects) {
			fields = append(fields, "NormalReturnFacts")
			postReturn = true
		}
	} else {
		// Legacy effect rows have one canonical lowering, but individual facts
		// can depend on whether arguments bind paths. These are its exhaustive
		// possible output roles for this exact signature.
		if len(sig.Effect.Labels) != 0 {
			fields = append(fields,
				"NormalReturnFacts", "ParamObligations", "ParamPathRefinements",
				"ParamPathWrites", "ParamLengthFloors", "ParamPathInvalidations",
				"ReturnPresenceRelations",
			)
			postReturn = true
		}
	}
	if postReturn || containsOutcomeField(fields, "Results") {
		fields = append(fields, "PostReturnAuthority")
	}
	return fields
}

func operationalEffectsProduceNormalReturnFacts(effects *signature.OperationalEffects) bool {
	return effects != nil && (len(effects.NormalReturnPresenceRefinements) != 0 ||
		len(effects.NormalReturnTypeRefinements) != 0 ||
		len(effects.PathPresenceImplications) != 0 ||
		len(effects.PathStaticMembers) != 0 ||
		len(effects.PathStaticMemberDeltas) != 0 ||
		len(effects.PathInvalidations) != 0 ||
		len(effects.BranchProofs) != 0 ||
		len(effects.DynamicIndexFacts) != 0 ||
		len(effects.KeyMemberships) != 0 ||
		len(effects.DynamicValueKeys) != 0 ||
		len(effects.FrozenTables) != 0 ||
		len(effects.EscapeEvents) != 0 ||
		len(effects.StoreRelations) != 0 ||
		len(effects.ParamRelations) != 0 ||
		len(effects.LifecycleEffects) != 0)
}

func containsOutcomeField(fields []string, wanted string) bool {
	for _, field := range fields {
		if field == wanted {
			return true
		}
	}
	return false
}

func operationalEffectsOnlyMaySuspend(effects *signature.OperationalEffects) bool {
	if effects == nil || !effects.MaySuspend {
		return false
	}
	withoutSuspension := effects.Clone()
	withoutSuspension.MaySuspend = false
	return withoutSuspension.IsEmpty()
}

type signatureNormalReturnLaneContext struct {
	sig       signature.Function
	args      signatureArgumentReader
	mutations signatureParamMutationEffects
}

type signatureNormalReturnLaneHandler func(signatureNormalReturnLaneContext, *callboundary.NormalReturnFacts)

var signatureNormalReturnLanes = callboundary.BindNormalReturnFactLanes(
	"signature normal-return",
	map[callboundary.NormalReturnFactLaneID]signatureNormalReturnLaneHandler{
		callboundary.LanePathRefinements:          signatureNormalReturnNoop,
		callboundary.LanePersistentPathWrites:     signatureNormalReturnNoop,
		callboundary.LanePathStaticMembers:        signatureNormalReturnNoop,
		callboundary.LanePathStaticMemberDeltas:   signatureNormalReturnNoop,
		callboundary.LanePathPresenceImplications: signatureNormalReturnNoop,
		callboundary.LanePathInvalidations:        signatureNormalReturnNoop,
		callboundary.LaneDynamicIndexFacts:        signatureNormalReturnDynamicIndexLane,
		callboundary.LaneKeyMemberships:           signatureNormalReturnNoop,
		callboundary.LaneDynamicValueKeys:         signatureNormalReturnNoop,
		callboundary.LaneDynamicAllValues:         signatureNormalReturnNoop,
		callboundary.LaneBranchProofs:             signatureNormalReturnNoop,
		callboundary.LaneChannelSelects:           signatureNormalReturnNoop,
		callboundary.LaneFrozenTables:             signatureNormalReturnFrozenTableLane,
		callboundary.LaneEffectDeltas:             signatureNormalReturnNoop,
		callboundary.LaneEscapeEvents:             signatureNormalReturnEscapeEventLane,
		callboundary.LaneStoreRelations:           signatureNormalReturnStoreRelationLane,
		callboundary.LaneLifecycleFacts:           signatureNormalReturnLifecycleLane,
		callboundary.LaneNumFloors:                signatureNormalReturnNoop,
		callboundary.LaneNumCeils:                 signatureNormalReturnNoop,
		callboundary.LaneRelConstraints:           signatureNormalReturnNoop,
	},
	func(handler signatureNormalReturnLaneHandler) bool { return handler != nil },
)

func signatureNormalReturnNoop(signatureNormalReturnLaneContext, *callboundary.NormalReturnFacts) {
}

func signatureNormalReturnDynamicIndexLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.DynamicIndexFacts = ctx.mutations.DynamicIndexFacts
}

func signatureNormalReturnFrozenTableLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.FrozenTables = signatureFrozenTablesForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnEscapeEventLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.EscapeEvents = signatureEscapeEventsForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnStoreRelationLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.StoreRelations = signatureStoreRelationsForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnLifecycleLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.LifecycleFacts = signatureLifecycleFactsForReader(ctx.sig, ctx.args)
}

func applySignatureNormalReturnFacts(
	sig signature.Function,
	args signatureArgumentReader,
	mutations signatureParamMutationEffects,
	out *callboundary.NormalReturnFacts,
) {
	ctx := signatureNormalReturnLaneContext{
		sig:       sig,
		args:      args,
		mutations: mutations,
	}
	for _, lane := range signatureNormalReturnLanes {
		lane.Value(ctx, out)
	}
}

func signatureNameForSite(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	nameForSite SignatureSiteNameFunc,
	nameFor SignatureNameFunc,
) (string, bool) {
	if nameForSite != nil {
		if name, ok := nameForSite(ctx, site); ok {
			return name, true
		}
	}
	if nameFor == nil {
		return "", false
	}
	return nameFor(ctx, factflow.NewCallProducerFromView(site))
}

func bindReceiverSelfSignature(sig signature.Function, receiverType typ.Type) signature.Function {
	if sig.Type == nil || receiverType == nil {
		return sig
	}
	if bound, ok := subst.Self(sig.Type, receiverType).(*typ.Function); ok && bound != nil {
		sig.Type = bound
	}
	return sig
}

func signatureConsumesReceiver(fn *typ.Function, receiverType typ.Type) bool {
	if fn == nil || len(fn.Params) == 0 {
		return false
	}
	first := fn.Params[0]
	if first.Receiver {
		return true
	}
	if first.Type == nil || receiverType == nil || typ.IsAny(first.Type) || typ.IsUnknown(first.Type) {
		return false
	}
	return subtype.IsSubtype(receiverType, first.Type)
}

func receiverMethodSignatureName(ctx transfer.NodeContext, site factflow.CallSiteView, input SignatureOutcomeInput) (string, typ.Type, bool) {
	method := site.MethodName()
	if method == "" || ctx.Registry == nil {
		return "", nil, false
	}
	receiverType, ok := receiverTypeForMethodSite(ctx, input)
	if !ok {
		return "", nil, false
	}
	if subtype.IsSubtype(receiverType, typ.String) {
		return "string." + method, receiverType, true
	}
	iface, ok := receiverSignatureInterface(receiverType)
	if !ok {
		return "", nil, false
	}
	return iface.Name + "." + method, receiverType, true
}

func receiverTypeForMethodSite(ctx transfer.NodeContext, input SignatureOutcomeInput) (typ.Type, bool) {
	if ctx.Registry == nil {
		return nil, false
	}
	value, ok := input.Receiver()
	if !ok {
		return nil, false
	}
	return receiverTypeFromValue(ctx.Registry, value)
}

func mustSignatureInputProgram(program SignatureOutcomeInputProgram, err error) SignatureOutcomeInputProgram {
	if err != nil {
		panic(err)
	}
	return program
}

func receiverTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	receiverType, ok := typevalue.TypeOf(reg, value)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return nil, false
	}
	return receiverType, true
}

func receiverSignatureInterface(receiverType typ.Type) (*typ.Interface, bool) {
	iface, ok := typ.UnwrapTransparentWrappers(receiverType).(*typ.Interface)
	if !ok || iface == nil || iface.Name == "" {
		return nil, false
	}
	return iface, true
}

func signatureArgumentSources(ctx transfer.NodeContext, facts factflow.Facts, site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	if site.ArgumentSourceCount() != 0 {
		return signatureArgumentsFromViewWithKeySpace(site, ks)
	}
	if factSite, ok := facts.CallSiteView(ctx.Point); ok {
		return signatureArgumentsFromViewWithKeySpace(factSite, ks)
	}
	return signatureArgumentReader{keySpace: ks}
}

func signatureMethodArgumentSources(ctx transfer.NodeContext, facts factflow.Facts, site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	if site.ArgumentSourceCount() != 0 {
		return signatureArgumentsFromMethodViewWithKeySpace(site, ks)
	}
	if factSite, ok := facts.CallSiteView(ctx.Point); ok {
		return signatureArgumentsFromMethodViewWithKeySpace(factSite, ks)
	}
	return signatureArgumentReader{keySpace: ks}
}
