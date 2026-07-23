package program

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

var errRelationDefinitionCoordinateAbsent = errors.New("definition coordinate is absent")

// relationProgramExecutionFactories is the exact application-owned execution
// authority for the lexical forest. A Static is immutable preparation; its
// factory owns the per-application State lattice, including lane selection and
// WIR-derived widening thresholds.
type relationProgramExecutionFactories map[lexicalidentity.StableLexicalBodyID]*body.ExecutionFactory

// relationProgramInput is the sole lexical-forest handoff to the replacement
// interprocedural engine. It is intentionally only an input constructor: the
// production driver must switch to the RelationProgram transaction atomically
// once executable application and publication are complete.
//
// The prepared operation plan owns the complete call surface and every
// boundary namespace. No solve-local context, summary key, relation cell, or
// migration catalog participates in this handoff.
func relationProgramInput(prepared preparedBodies, executions relationProgramExecutionFactories, initial transfer.InitialState) ([]transformer.RelationProgramUnit, error) {
	definitions, err := relationProgramDefinitions(prepared)
	if err != nil {
		return nil, err
	}
	resourceReachability, err := relationProgramResourceReachability(prepared)
	if err != nil {
		return nil, err
	}
	for owner, owned := range definitions {
		for index := range owned {
			owned[index].ExternallyReachable = resourceReachability[owned[index].Target]
		}
		definitions[owner] = owned
	}
	contextualSeeds, err := relationProgramContextualParamSeeds(prepared, definitions)
	if err != nil {
		return nil, err
	}
	statics := make([]*body.Static, 0, 1+len(prepared.functions))
	if prepared.root != nil {
		statics = append(statics, prepared.root)
	}
	for _, static := range prepared.functions {
		if static == nil {
			return nil, fmt.Errorf("program: replacement relation input has a nil lexical body")
		}
		statics = append(statics, static)
	}
	if len(statics) == 0 {
		return nil, fmt.Errorf("program: replacement relation input has no lexical bodies")
	}
	sort.Slice(statics, func(i, j int) bool {
		left, right := statics[i].StableLexicalBodyID(), statics[j].StableLexicalBodyID()
		return bytes.Compare(left[:], right[:]) < 0
	})

	units := make([]transformer.RelationProgramUnit, len(statics))
	var previous lexicalidentity.StableLexicalBodyID
	for index, static := range statics {
		bodyID := static.StableLexicalBodyID()
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) || index > 0 && bodyID == previous {
			return nil, fmt.Errorf("program: replacement relation input has a zero or duplicate lexical body")
		}
		plan := static.OperationPlan()
		graph := static.Graph()
		if plan == nil || graph == nil {
			return nil, fmt.Errorf("program: replacement relation input body %s has no graph or operation plan", bodyID)
		}
		captures, globals, contracts, boundaryOK := prepared.callTopology.Boundary(bodyID)
		if !boundaryOK {
			return nil, fmt.Errorf("program: replacement relation input body %s has no closed boundary carrier", bodyID)
		}
		if len(globals) != len(contracts) {
			return nil, fmt.Errorf("program: replacement relation input body %s has mismatched global boundary carriers", bodyID)
		}
		globalPairs := make([]operationplan.BoundaryGlobal, len(globals))
		for index, global := range globals {
			globalPairs[index] = operationplan.BoundaryGlobal{Symbol: global, Contract: contracts[index]}
		}
		plan = plan.WithBoundaryCaptures(captures).WithBoundaryGlobals(globalPairs)
		if !plan.BoundaryCapturesValid() || !plan.BoundaryGlobalsValid() || len(plan.BoundaryGlobalContracts()) != len(globals) {
			return nil, fmt.Errorf("program: replacement relation input body %s has invalid closed boundary carrier", bodyID)
		}
		execution := executions[bodyID]
		if execution == nil || execution.Registry() != static.Registry() || execution.KeySpace() != static.KeySpace() || execution.Graph() != graph {
			return nil, fmt.Errorf("program: replacement relation input body %s has no exact execution factory", bodyID)
		}
		entrySeedPlan := execution.EntrySeedPlan()
		if !entrySeedPlan.Valid() {
			return nil, fmt.Errorf("program: replacement relation input body %s has no prepared entry-seed authority", bodyID)
		}
		if refinements := contextualSeeds[bodyID]; len(refinements) != 0 {
			var refined bool
			entrySeedPlan, refined = entrySeedPlan.Refine(static.Registry(), refinements)
			if !refined {
				return nil, fmt.Errorf("program: replacement relation input body %s has invalid contextual parameter refinements", bodyID)
			}
		}
		paramSlots := make([]statekey.Value, len(plan.BoundaryParams()))
		for paramIndex, param := range plan.BoundaryParams() {
			paramSlots[paramIndex] = statekey.SymbolValue(param)
		}
		paramContracts, present := entrySeedPlan.ValuesForSlots(paramSlots)
		if !present {
			return nil, fmt.Errorf("program: replacement relation input body %s has no prepared parameter contract tuple", bodyID)
		}
		plan = plan.WithBoundaryParamContracts(paramContracts)
		if len(plan.BoundaryParamContracts()) != len(paramContracts) {
			return nil, fmt.Errorf("program: replacement relation input body %s parameter contracts did not seal", bodyID)
		}
		initialStatePlan, err := execution.FreezeInitialStatePlan(initial)
		if err != nil {
			return nil, fmt.Errorf("program: replacement relation input body %s initial-state authority: %w", bodyID, err)
		}
		surface, exact := plan.CallSurface()
		if !exact || !surface.Complete() || surface.Owner() != bodyID || surface.PointCount() != graph.Size() {
			return nil, fmt.Errorf("program: replacement relation input body %s has no exact owned call surface", bodyID)
		}
		owner, owned := prepared.directLexicalOwner(static)
		if !owned {
			return nil, fmt.Errorf("program: replacement relation input body %s has no binder owner", bodyID)
		}
		declarations, err := transformer.SealDirectLexicalDeclarationAuthority(plan, prepared.bindings, owner)
		if err != nil {
			return nil, fmt.Errorf("program: replacement relation input body %s direct lexical declarations: %w", bodyID, err)
		}
		readGraph := execution.ReadGraph()
		nodeReads := make([][]cfg.Point, graph.Size())
		for raw := 0; raw < graph.Size(); raw++ {
			nodeReads[raw] = readGraph.NodeReads(cfg.Point(raw))
		}
		units[index] = transformer.RelationProgramUnit{
			Body:                      bodyID,
			Registry:                  static.Registry(),
			KeySpace:                  static.KeySpace(),
			Graph:                     graph,
			Plan:                      plan,
			Domain:                    execution.ProductDomain(),
			EntrySeedPlan:             entrySeedPlan,
			InitialStatePlan:          initialStatePlan,
			PathSemantics:             static.PathSemanticAuthority(),
			RootAssignments:           execution.RootAssignmentAuthority(),
			Returns:                   execution.ReturnAuthority(),
			ExternalCallOutcome:       execution.ExternalCallOutcomeProgram(),
			CustomExpressionValue:     execution.CustomExpressionValueProvider(),
			GenericForMembership:      execution.GenericForMembership(),
			NodeReads:                 nodeReads,
			DirectLexicalDeclarations: declarations,
			Definitions:               append([]transformer.RelationProgramDefinition(nil), definitions[bodyID]...),
			Shape: transformer.Shape{
				Params:   uint32(len(plan.BoundaryParams())),
				Captures: uint32(len(plan.BoundaryCaptures())),
				Globals:  uint32(len(plan.BoundaryGlobals())),
			},
		}
		previous = bodyID
	}
	return units, nil
}

func relationProgramDefinitions(prepared preparedBodies) (map[lexicalidentity.StableLexicalBodyID][]transformer.RelationProgramDefinition, error) {
	out := make(map[lexicalidentity.StableLexicalBodyID][]transformer.RelationProgramDefinition)
	if prepared.bindings == nil {
		return nil, fmt.Errorf("program: replacement definitions have no binder authority")
	}
	for function, targetStatic := range prepared.functions {
		origin, ok := prepared.bindings.FunctionOrigin(function)
		if !ok || targetStatic == nil || origin.Symbol == 0 {
			return nil, fmt.Errorf("program: replacement definition has no exact lexical origin")
		}
		var ownerStatic *body.Static
		if origin.Parent != nil {
			ownerStatic = prepared.function(origin.Parent)
		} else if prepared.root != nil {
			ownerStatic = prepared.root
		} else {
			// The outer function of RunBoundFunction is the demanded root, not a
			// declaration inside another body in this forest transaction.
			continue
		}
		if ownerStatic == nil {
			return nil, fmt.Errorf("program: replacement definition for %s has no lexical owner", targetStatic.StableLexicalBodyID())
		}
		point, err := relationDefinitionPoint(ownerStatic.OperationPlan().Facts(), ownerStatic.Graph(), origin.Symbol)
		if err != nil {
			// A closure retained only inside an object-literal graph can have no
			// point-owned value source. It remains a lexical body, but this owner
			// has no coordinate at which to publish its definition.
			if errors.Is(err, errRelationDefinitionCoordinateAbsent) {
				continue
			}
			return nil, fmt.Errorf("program: replacement definition for %s: %w", targetStatic.StableLexicalBodyID(), err)
		}
		owner := ownerStatic.StableLexicalBodyID()
		out[owner] = append(out[owner], transformer.RelationProgramDefinition{Target: targetStatic.StableLexicalBodyID(), Point: point})
	}
	for owner := range out {
		sort.Slice(out[owner], func(i, j int) bool {
			if out[owner][i].Point != out[owner][j].Point {
				return out[owner][i].Point < out[owner][j].Point
			}
			return bytes.Compare(out[owner][i].Target[:], out[owner][j].Target[:]) < 0
		})
	}
	return out, nil
}

// relationProgramResourceReachability seals the binder's whole-unit escape
// boundary for lexical definitions. A definition is excluded only when the
// binder positively proves that every runtime use is a direct local call.
// Missing or open use evidence therefore remains externally reachable; it is
// never converted into a frozen declaration-time world.
func relationProgramResourceReachability(prepared preparedBodies) (map[lexicalidentity.StableLexicalBodyID]bool, error) {
	if prepared.bindings == nil {
		return nil, fmt.Errorf("program: definition resource reachability has no binder authority")
	}
	nonEscaping := make(map[symbol.ID]struct{})
	for _, use := range prepared.bindings.LocalFunctionUseClosures() {
		if !use.RuntimeUseScanComplete {
			return nil, fmt.Errorf("program: definition resource reachability has an incomplete runtime-use scan")
		}
		if use.FunctionSymbol != 0 && use.ValueDoesNotEscape {
			nonEscaping[use.FunctionSymbol] = struct{}{}
		}
	}
	out := make(map[lexicalidentity.StableLexicalBodyID]bool, len(prepared.functions))
	for function, static := range prepared.functions {
		if function == nil || static == nil {
			return nil, fmt.Errorf("program: definition resource reachability has a nil lexical body")
		}
		origin, ok := prepared.bindings.FunctionOrigin(function)
		if !ok || origin.Symbol == 0 {
			return nil, fmt.Errorf("program: definition resource reachability has no exact function identity")
		}
		_, closed := nonEscaping[origin.Symbol]
		out[static.StableLexicalBodyID()] = !closed
	}
	return out, nil
}

func relationDefinitionPoint(facts factflow.Facts, graph cfg.Graph, function symbol.ID) (cfg.Point, error) {
	if graph == nil || function == 0 {
		return 0, fmt.Errorf("definition point is unowned")
	}
	matches := make(map[cfg.Point]struct{})
	for raw := 0; raw < graph.Size(); raw++ {
		point := cfg.Point(raw)
		matched := false
		if value, ok := facts.RootAssignment(point); ok && facts.SourceContainsFunction(value.Source(), function) {
			matched = true
		}
		if value, ok := facts.PathAssignment(point); ok && facts.SourceContainsFunction(value.Source(), function) {
			matched = true
		}
		if value, ok := facts.PathStaticMemberWrite(point); ok && facts.SourceContainsFunction(value.Source(), function) {
			matched = true
		}
		if value, ok := facts.DynamicIndexWrite(point); ok && facts.SourceContainsFunction(value.Source(), function) {
			matched = true
		}
		if value, ok := facts.Return(point); ok {
			for _, source := range value.Sources() {
				matched = matched || facts.SourceContainsFunction(source, function)
			}
		}
		if site, ok := facts.CallSiteView(point); ok {
			if receiver, present := site.ReceiverSource(); present && facts.SourceContainsFunction(receiver, function) {
				matched = true
			}
			site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
				matched = matched || facts.SourceContainsFunction(source, function)
				return true
			})
		}
		if matched {
			matches[point] = struct{}{}
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("%w: function %d", errRelationDefinitionCoordinateAbsent, function)
	}
	if len(matches) != 1 {
		return 0, fmt.Errorf("function %d has %d definition coordinates, want one", function, len(matches))
	}
	for point := range matches {
		return point, nil
	}
	return 0, fmt.Errorf("function %d definition coordinate disappeared", function)
}

func relationProgramContextualParamSeeds(prepared preparedBodies, definitions map[lexicalidentity.StableLexicalBodyID][]transformer.RelationProgramDefinition) (map[lexicalidentity.StableLexicalBodyID][]state.ValueSeed, error) {
	out := make(map[lexicalidentity.StableLexicalBodyID][]state.ValueSeed)
	if prepared.bindings == nil {
		return nil, fmt.Errorf("program: contextual definition seeds have no binder authority")
	}
	for function, targetStatic := range prepared.functions {
		origin, ok := prepared.bindings.FunctionOrigin(function)
		if !ok || targetStatic == nil || origin.Symbol == 0 {
			return nil, fmt.Errorf("program: contextual definition seed has no lexical origin")
		}
		var ownerStatic *body.Static
		if origin.Parent != nil {
			ownerStatic = prepared.function(origin.Parent)
		} else {
			ownerStatic = prepared.root
		}
		if ownerStatic == nil {
			continue
		}
		targetID := targetStatic.StableLexicalBodyID()
		point, found := relationDefinitionCoordinate(definitions[ownerStatic.StableLexicalBodyID()], targetID)
		if !found {
			continue
		}
		expected, exact := relationDefinitionExpectedFunction(prepared.bindings, ownerStatic, point, origin.Symbol)
		if !exact {
			continue
		}
		cache := typevalue.NewCache()
		for _, slot := range prepared.bindings.ParamSlots(function) {
			if slot.Symbol == 0 || slot.Type != nil || slot.ImplicitSelf || slot.Vararg || slot.SourceIndex < 0 || slot.SourceIndex >= len(expected.Params) {
				continue
			}
			t := expected.Params[slot.SourceIndex].Type
			if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
				continue
			}
			out[targetID] = append(out[targetID], state.ValueSeed{Slot: statekey.SymbolValue(slot.Symbol), Value: cache.FromTypeWithWitness(targetStatic.Registry(), t)})
		}
	}
	return out, nil
}

func relationDefinitionCoordinate(definitions []transformer.RelationProgramDefinition, target lexicalidentity.StableLexicalBodyID) (cfg.Point, bool) {
	for _, definition := range definitions {
		if definition.Target == target && definition.Point != 0 {
			return definition.Point, true
		}
	}
	return 0, false
}

func relationDefinitionExpectedFunction(bindings *bind.Result, owner *body.Static, point cfg.Point, function symbol.ID) (*typ.Function, bool) {
	if bindings == nil || owner == nil || owner.OperationPlan() == nil || point == 0 || function == 0 {
		return nil, false
	}
	facts := owner.OperationPlan().Facts()
	site, ok := facts.CallSiteView(point)
	if !ok {
		return nil, false
	}
	argument := -1
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		if facts.SourceContainsFunction(source, function) {
			if argument >= 0 {
				argument = -2
				return false
			}
			argument = index
		}
		return true
	})
	if argument < 0 {
		return nil, false
	}
	callable, receiverOffset, ok := relationDeclaredCallFunction(bindings, site)
	if !ok || callable == nil {
		return nil, false
	}
	formal := argument + receiverOffset
	if formal < 0 || formal >= len(callable.Params) {
		return nil, false
	}
	return typecall.ContextualCallable(callable.Params[formal].Type)
}

func relationDeclaredCallFunction(bindings *bind.Result, site factflow.CallSiteView) (*typ.Function, int, bool) {
	resolver := typeresolve.New(bindings)
	if method := site.MethodName(); method != "" {
		receiver, ok := site.ReceiverPath()
		if !ok || receiver.Symbol == 0 || len(receiver.Segments) != 0 {
			return nil, 0, false
		}
		receiverType, ok := relationDeclaredSymbolType(bindings, resolver, receiver.Symbol)
		if !ok {
			return nil, 0, false
		}
		callable, _, ok := typecall.MemberCallable(receiverType, method)
		if !ok {
			return nil, 0, false
		}
		offset := 0
		if typecall.CallableConsumesReceiver(callable, receiverType) {
			offset = 1
		}
		return callable, offset, true
	}
	callee := site.CalleeSymbol()
	if callee == 0 {
		return nil, 0, false
	}
	if declared, ok := relationDeclaredSymbolType(bindings, resolver, callee); ok {
		callable, ok := typecall.Callable(declared)
		return callable, 0, ok
	}
	identity, ok := bindings.StableLocalFunctionIdentity(callee)
	if !ok {
		return nil, 0, false
	}
	function, ok := bindings.FunctionBySymbol(identity)
	if !ok {
		return nil, 0, false
	}
	callable, ok := lowerFunctionExprType(function, bindings, nil)
	return callable, 0, ok
}

func relationDeclaredSymbolType(bindings *bind.Result, resolver *typeresolve.Resolver, id symbol.ID) (typ.Type, bool) {
	if bindings == nil || resolver == nil || id == 0 {
		return nil, false
	}
	annotation, ok := bindings.SymbolTypeAnnotation(id)
	if !ok || annotation == nil {
		return nil, false
	}
	return resolver.Type(annotation)
}

func relationProgramSourcePath(facts factflow.Facts, source factflow.ValueSource) pathdom.PathKey {
	var path pathdom.Path
	switch source.Kind {
	case factflow.ValueSourcePath:
		if id, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey); ok && version == 0 {
			segments, parsed := segment.InternFormattedSegments(suffix)
			if parsed {
				path = pathdom.Path{Symbol: id, Segments: segments}
			}
		} else if id, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
			path = pathdom.Path{Symbol: id, Segments: segments}
		}
	case factflow.ValueSourceExpression:
		if source.HasExpr {
			path, _ = facts.ExpressionPathRef(source.ExprRef)
		}
	}
	if path.Symbol == 0 || path.Version != 0 {
		return ""
	}
	return pathaddr.SymbolPathKey(path.Symbol, path.Segments)
}
