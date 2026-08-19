package manifesttarget

import (
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

// operations projects the provider-owned default ABI into Target and then
// applies only operational laws: alternative outcomes, callbacks,
// yield/resume, allocation identity, aliases, and rule delegation. Parameter
// and return declarations are never authored here.
func operations(declarations *manifest.Catalogue) (authoredCatalogue, error) {
	if declarations == nil {
		return authoredCatalogue{}, fmt.Errorf("target: nil declaration catalogue")
	}
	var catalogue authoredCatalogue
	functions := declarations.Functions()
	for _, declaration := range functions {
		operation, err := operationFromManifest(declaration)
		if err != nil {
			return authoredCatalogue{}, err
		}
		catalogue.add(declaration.CanonicalPath(), operation)
	}
	for _, declaration := range functions {
		law, ok := declaration.Operation()
		if !ok {
			continue
		}
		if err := applyProducedRelations(&catalogue, declaration.CanonicalPath(), law); err != nil {
			return authoredCatalogue{}, err
		}
	}
	return catalogue, nil
}

func operationFromManifest(declaration manifest.Function) (vocabulary.OperationSpec, error) {
	base := operationFromDeclaration(declaration)
	law, ok := declaration.Operation()
	if !ok {
		return base, nil
	}
	if law.Replace {
		converted, err := convertOperation(law)
		if err != nil {
			return vocabulary.OperationSpec{}, fmt.Errorf("target catalogue: %s: %w", declaration.CanonicalPath(), err)
		}
		converted.Bindings = base.Bindings
		base = converted
	}
	applyOperationAmendments(&base, law)
	// Behavior is a generic amendment to the operation envelope. Apply it
	// after the ordinary outcome amendments so its authored outcome ordinals
	// continue to refer to the final operation shape under both replacement
	// and amendment semantics.
	base.Behavior = convertBehavior(law.Behavior)
	if value := law.SubedgeRelation; value != nil {
		base.SubedgeRelation = &vocabulary.SubedgeRelationSpec{
			Operand: vocabulary.ValueFormal(value.Operand), Selector: value.Selector, Subedge: vocabulary.SubedgeRef(value.Subedge),
			ResultOutcome: value.ResultOutcome, Result: value.Result,
			EffectAliases: append([]uint32(nil), value.EffectAliases...),
		}
	}
	return base, nil
}

func convertOperation(in moduleio.Operation) (vocabulary.OperationSpec, error) {
	out := vocabulary.OperationSpec{
		ValuesVars: in.ValuesVars,
		Input:      convertValues(in.Input),
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowTail(in.Effects.Tail)},
	}
	for _, value := range in.Outcomes {
		out.Outcomes = append(out.Outcomes, convertOutcome(value))
	}
	for _, value := range in.Callbacks {
		out.Callbacks = append(out.Callbacks, convertCallback(value))
	}
	for _, value := range in.Subedges {
		out.Subedges = append(out.Subedges, convertSubedge(value))
	}
	for _, value := range in.Suspensions {
		out.Suspensions = append(out.Suspensions, vocabulary.SuspensionSpec{
			Yield: value.Yield, Reentry: value.Reentry,
			Source: vocabulary.ReentrySource(value.Source), Multiplicity: vocabulary.ReentryMultiplicity(value.Multiplicity),
		})
	}
	for _, value := range in.Spawns {
		spawn := vocabulary.SpawnSpec{
			Function: convertInputSource(value.Function), Child: vocabulary.CallbackRef(value.Child),
			Yield: value.Yield, ParentResume: value.ParentResume, ChildEntry: value.ChildEntry,
		}
		for _, alternative := range value.Alternatives {
			spawn.Alternatives = append(spawn.Alternatives, vocabulary.SpawnSiblingAlternative(alternative))
		}
		out.Spawns = append(out.Spawns, spawn)
	}
	for _, value := range in.Resumes {
		resume := vocabulary.ResumeSpec{
			Source: vocabulary.ResumeSource(value.Source), Carrier: vocabulary.ValueFormal(value.Carrier), Arguments: convertValues(value.Arguments),
		}
		for _, outcome := range value.Outcomes {
			resume.Outcomes = append(resume.Outcomes, vocabulary.ResumeOutcomeSpec{Kind: flowkind.OutcomeKind(outcome.Kind), Outcome: outcome.Outcome})
		}
		out.Resumes = append(out.Resumes, resume)
	}
	out.Behavior = convertBehavior(in.Behavior)
	if value := in.SubedgeRelation; value != nil {
		out.SubedgeRelation = &vocabulary.SubedgeRelationSpec{
			Operand: vocabulary.ValueFormal(value.Operand), Selector: value.Selector, Subedge: vocabulary.SubedgeRef(value.Subedge),
			ResultOutcome: value.ResultOutcome, Result: value.Result,
			EffectAliases: append([]uint32(nil), value.EffectAliases...),
		}
	}
	return out, nil
}

// convertBehavior crosses the portable manifest boundary. Providers carry a
// neutral relation key string because the wire package cannot depend on the
// analyzer schema package. The Lua-domain adapter is the sole place that
// derives the existing opaque structure EntryID from that key; Target stores
// and authenticates the resulting identity without interpreting it.
func convertBehavior(in *moduleio.OperationBehavior) *vocabulary.OperationBehaviorSpec {
	if in == nil || (len(in.Results) == 0 && len(in.Predicates) == 0) {
		return nil
	}
	out := &vocabulary.OperationBehaviorSpec{}
	for _, value := range in.Results {
		out.Results = append(out.Results, vocabulary.OperationResultSpec{
			Outcome: value.Outcome,
			Result:  value.Result,
			Source:  convertInputSource(value.Source),
			Relation: schema.NewEntryID(
				schema.SurfaceKindStructure,
				schema.Key(value.Relation),
			),
		})
	}
	for _, value := range in.Predicates {
		out.Predicates = append(out.Predicates, vocabulary.OperationPredicateSpec{
			Outcome: value.Outcome,
			Result:  value.Result,
			Subject: convertInputSource(value.Subject),
			Relation: schema.NewEntryID(
				schema.SurfaceKindStructure,
				schema.Key(value.Relation),
			),
		})
	}
	return out
}

func convertValues(in moduleio.Values) vocabulary.ValuesSpec {
	var tailType schematype.Type
	if in.TailType != nil {
		tailType = portable(in.TailType)
	}
	return vocabulary.ValuesSpec{
		Fixed: portableList(in.Fixed), Tail: vocabulary.ValuesTail(in.Tail), Var: vocabulary.ValuesVar(in.Var),
		TailType: tailType, Suffix: portableList(in.Suffix),
	}
}

func convertOutcome(in moduleio.Outcome) vocabulary.OutcomeSpec {
	out := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeKind(in.Kind), Values: convertValues(in.Values)}
	for _, value := range in.FreshResults {
		out.FreshResults = append(out.FreshResults, vocabulary.FreshResultSpec{Result: value.Result, Kind: schematype.FreshClass(value.Class)})
	}
	for _, value := range in.CallbackResults {
		out.CallbackResults = append(out.CallbackResults, vocabulary.CallbackResultSpec{Result: value.Result, Callback: vocabulary.CallbackRef(value.Callback)})
	}
	for _, value := range in.ResultAliases {
		out.ResultAliases = append(out.ResultAliases, vocabulary.ResultAliasSpec{Result: value.Result, Source: convertInputSource(value.Source)})
	}
	return out
}

func convertCallback(in moduleio.Callback) vocabulary.CallbackSpec {
	out := vocabulary.CallbackSpec{
		Function: convertInputSource(in.Function), Admission: schematype.CallableAdmission(in.Admission),
		Arguments: convertValues(in.Arguments), Lifecycle: vocabulary.CallbackLifecycle(in.Lifecycle),
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowTail(in.Effects.Tail)},
	}
	for _, value := range in.Outcomes {
		out.Outcomes = append(out.Outcomes, vocabulary.TerminalSpec{Kind: flowkind.OutcomeKind(value.Kind), Values: convertValues(value.Values)})
	}
	return out
}

func convertSubedge(in moduleio.Subedge) vocabulary.SubedgeSpec {
	out := vocabulary.SubedgeSpec{
		Role: in.Role, Family: vocabulary.SubedgeFamily(in.Family), Admission: schematype.CallableAdmission(in.Admission),
		Arguments: convertValues(in.Arguments), RuleEntry: in.RuleEntry,
		Callee: vocabulary.SubedgeCalleeSpec{
			Kind: vocabulary.SubedgeCalleeKind(in.Callee.Kind), Callback: vocabulary.CallbackRef(in.Callee.Callback),
			Read:    vocabulary.CapturedInitialReadSpec{Root: in.Callee.Read.Root, Key: convertLiteral(in.Callee.Read.Key)},
			MetaKey: convertLiteral(in.Callee.MetaKey),
		},
	}
	for _, value := range in.ArgumentOrigins {
		out.ArgumentOrigins = append(out.ArgumentOrigins, vocabulary.ArgumentOrigin{
			Segment: vocabulary.ArgumentSegment(value.Segment), Index: value.Index,
			Kind: vocabulary.ArgumentSource(value.Kind), Source: convertInputSource(value.Source),
		})
	}
	for _, value := range in.Outcomes {
		out.Outcomes = append(out.Outcomes, vocabulary.TerminalSpec{Kind: flowkind.OutcomeKind(value.Kind), Values: convertValues(value.Values)})
	}
	out.AdmissionFailure = vocabulary.AdmissionFailureSpec{
		Values: convertValues(in.AdmissionFailure.Values),
		Route: vocabulary.AdmissionRouteSpec{
			Route: vocabulary.SubedgeRoute(in.AdmissionFailure.Route.Route), Adjustment: vocabulary.Adjustment(in.AdmissionFailure.Route.Adjustment),
			Result: convertValues(in.AdmissionFailure.Route.Result), Placement: vocabulary.Placement(in.AdmissionFailure.Route.Placement),
			Offset: in.AdmissionFailure.Route.Offset, Outcome: in.AdmissionFailure.Route.Outcome,
			Subedge: vocabulary.SubedgeRef(in.AdmissionFailure.Route.Subedge),
		},
	}
	for _, value := range in.Routes {
		out.Routes = append(out.Routes, vocabulary.SubedgeRouteSpec{
			Kind: flowkind.OutcomeKind(value.Kind), Route: vocabulary.SubedgeRoute(value.Route), Adjustment: vocabulary.Adjustment(value.Adjustment),
			Result: convertValues(value.Result), Placement: vocabulary.Placement(value.Placement), Offset: value.Offset,
			Outcome: value.Outcome, Subedge: vocabulary.SubedgeRef(value.Subedge),
		})
	}
	return out
}

func convertInputSource(in moduleio.InputSource) vocabulary.InputSource {
	return vocabulary.InputSource{Kind: vocabulary.InputSourceKind(in.Kind), Ordinal: in.Ordinal}
}

func convertLiteral(in moduleio.Literal) keyspace.LiteralValue {
	if in.Kind == moduleio.LiteralString {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: in.String}
	}
	return keyspace.LiteralValue{}
}

func applyOperationAmendments(operation *vocabulary.OperationSpec, law moduleio.Operation) {
	for _, values := range law.AppendNormal {
		operation.Outcomes = append(operation.Outcomes, vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: convertValues(values)})
	}
	if law.ReplaceNormalSet {
		operation.Outcomes = operation.Outcomes[:0]
		for _, values := range law.ReplaceNormal {
			operation.Outcomes = append(operation.Outcomes, vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: convertValues(values)})
		}
	}
	if law.InputTailType != nil {
		operation.Input.TailType = portable(law.InputTailType)
	}
	for _, tail := range law.OutcomeTailTypes {
		if int(tail.Outcome) < len(operation.Outcomes) {
			operation.Outcomes[tail.Outcome].Values.TailType = portable(tail.Type)
		}
	}
	for _, amendment := range law.OutcomeAmendments {
		if int(amendment.Outcome) >= len(operation.Outcomes) {
			continue
		}
		outcome := &operation.Outcomes[amendment.Outcome]
		for _, value := range amendment.FreshResults {
			outcome.FreshResults = append(outcome.FreshResults, vocabulary.FreshResultSpec{Result: value.Result, Kind: schematype.FreshClass(value.Class)})
		}
		for _, value := range amendment.CallbackResults {
			outcome.CallbackResults = append(outcome.CallbackResults, vocabulary.CallbackResultSpec{Result: value.Result, Callback: vocabulary.CallbackRef(value.Callback)})
		}
		for _, value := range amendment.ResultAliases {
			outcome.ResultAliases = append(outcome.ResultAliases, vocabulary.ResultAliasSpec{Result: value.Result, Source: convertInputSource(value.Source)})
		}
	}
}

func applyProducedRelations(catalogue *authoredCatalogue, producer string, law moduleio.Operation) error {
	producerRef, err := catalogue.require(producer)
	if err != nil {
		return err
	}
	attach := func(outcome uint32, declarations []moduleio.Produced) error {
		operation := catalogue.at(producerRef)
		if int(outcome) >= len(operation.Outcomes) {
			return fmt.Errorf("target catalogue: outcome %d outside %q", outcome, producer)
		}
		for _, declaration := range declarations {
			child, err := catalogue.require(declaration.Operation)
			if err != nil {
				return fmt.Errorf("target catalogue: %s produced relation: %w", producer, err)
			}
			produced := vocabulary.ProducedSpec{Result: declaration.Result, Operation: vocabulary.SpecRef(child)}
			for _, capture := range declaration.Captures {
				produced.Captures = append(produced.Captures, vocabulary.CaptureSpec{Kind: vocabulary.CaptureKind(capture.Kind), Ordinal: capture.Ordinal})
			}
			operation.Outcomes[outcome].Produced = append(operation.Outcomes[outcome].Produced, produced)
		}
		return nil
	}
	for outcome, declaration := range law.Outcomes {
		if err := attach(uint32(outcome), declaration.Produced); err != nil {
			return err
		}
	}
	for _, amendment := range law.OutcomeAmendments {
		if err := attach(amendment.Outcome, amendment.Produced); err != nil {
			return err
		}
	}
	return nil
}

func operationFromDeclaration(declaration manifest.Function) vocabulary.OperationSpec {
	binding := vocabulary.OperationSpec{Bindings: bindingsFromDeclaration(declaration)}
	function := declaration.Signature()
	if function.Type == nil {
		return normal(binding, nil, false, nil, false)
	}
	fixed := make([]typ.Type, 0, len(function.Type.Params))
	optional := make([]typ.Type, 0)
	arguments := make([]typ.Type, len(function.Type.TypeParams))
	for index, parameter := range function.Type.TypeParams {
		arguments[index] = typ.Any
		if parameter != nil && parameter.Constraint != nil {
			arguments[index] = parameter.Constraint
		}
	}
	materialize := func(value typ.Type) typ.Type {
		return subst.Params(value, function.Type.TypeParams, arguments)
	}
	for _, parameter := range function.Type.Params {
		if parameter.Optional {
			optional = append(optional, materialize(parameter.Type))
			continue
		}
		fixed = append(fixed, materialize(parameter.Type))
	}
	open := function.Type.Variadic != nil || len(optional) != 0
	returns := make([]typ.Type, len(function.Type.Returns))
	for index, value := range function.Type.Returns {
		returns[index] = successfulResultType(materialize(value))
	}
	operation := normal(binding, fixed, open, returns, function.ResultTail != nil)
	if function.ResultTail != nil {
		operation.Outcomes[0].Values.TailType = portable(materialize(function.ResultTail))
	}
	if len(function.ResultSuffix) != 0 {
		suffix := make([]typ.Type, len(function.ResultSuffix))
		for index, value := range function.ResultSuffix {
			suffix[index] = materialize(value)
		}
		operation.Outcomes[0].Values.Suffix = portableList(suffix)
	}
	if open {
		tail := append(optional, materialize(function.Type.Variadic))
		filtered := tail[:0]
		for _, value := range tail {
			if value != nil {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 1 {
			operation.Input.TailType = portable(filtered[0])
		} else if len(filtered) > 1 {
			operation.Input.TailType = portable(typ.MaterializeUnion(filtered))
		}
	}
	if len(returns) == 1 && typ.TypeEquals(returns[0], typ.Never) {
		operation.Outcomes = operation.Outcomes[1:]
	}
	return operation
}

func normal(op vocabulary.OperationSpec, in []typ.Type, openIn bool, out []typ.Type, openOut bool) vocabulary.OperationSpec {
	vars := uint32(0)
	inVar, outVar := vocabulary.ValuesVar(0), vocabulary.ValuesVar(0)
	if openIn {
		vars++
	}
	if openOut {
		outVar = vocabulary.ValuesVar(vars)
		vars++
	}
	op.ValuesVars = vars
	op.Input = values(in, openIn, inVar)
	op.Outcomes = []vocabulary.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values(out, openOut, outVar)},
		{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)},
	}
	op.Effects = vocabulary.RowSpec{Tail: vocabulary.RowClosed}
	return op
}

func bindingsFromDeclaration(declaration manifest.Function) []vocabulary.BindingSpec {
	out := make([]vocabulary.BindingSpec, 0, len(declaration.Bindings()))
	for _, binding := range declaration.Bindings() {
		switch binding.Mount() {
		case manifest.MountGlobals:
			out = append(out, vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: binding.Member()})
		case manifest.MountModule:
			out = append(out, vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{binding.ModulePath()}, Member: binding.Member()})
		case manifest.MountDetached:
		}
	}
	return out
}

func successfulResultType(value typ.Type) typ.Type {
	if optional, ok := typ.UnwrapTransparentWrappers(value).(*typ.Optional); ok && optional.Inner != nil {
		return optional.Inner
	}
	return value
}
