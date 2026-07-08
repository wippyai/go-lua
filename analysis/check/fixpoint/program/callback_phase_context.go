package program

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callbackPhaseRules struct {
	registrations map[string][]manifest.CallbackPhaseRegistration
	invocations   map[string][]manifest.CallbackPhaseInvocation
}

func callbackPhaseRulesFromConfig(config body.Config) callbackPhaseRules {
	var out callbackPhaseRules
	for _, m := range config.Signatures.Manifests {
		if m == nil {
			continue
		}
		for _, registration := range m.CallbackPhaseRegistrations {
			if registration.Function == "" || registration.CallbackParam < 0 || registration.Phase == "" {
				continue
			}
			if out.registrations == nil {
				out.registrations = make(map[string][]manifest.CallbackPhaseRegistration)
			}
			for _, name := range callbackPhaseRuleFunctionNames(m, registration.Function) {
				out.registrations[name] = append(out.registrations[name], registration)
			}
		}
		for _, invocation := range m.CallbackPhaseInvocations {
			if invocation.Function == "" || invocation.CallbackParam < 0 || (len(invocation.Before) == 0 && len(invocation.After) == 0) {
				continue
			}
			if out.invocations == nil {
				out.invocations = make(map[string][]manifest.CallbackPhaseInvocation)
			}
			for _, name := range callbackPhaseRuleFunctionNames(m, invocation.Function) {
				out.invocations[name] = append(out.invocations[name], invocation)
			}
		}
	}
	return out
}

func callbackPhaseRuleFunctionNames(m *manifest.Manifest, fn string) []string {
	if m == nil || m.Path == "" || fn == "" ||
		strings.HasPrefix(fn, m.Path+".") ||
		strings.HasPrefix(fn, m.Path+"[") {
		return []string{fn}
	}
	return []string{fn, m.Path + "." + fn}
}

func (r callbackPhaseRules) empty() bool {
	return len(r.registrations) == 0 && len(r.invocations) == 0
}

type callbackPhaseTracker struct {
	keys     *programKeys
	owner    summary.SummaryKey
	prepass  *body.Result
	config   body.Config
	prepared preparedBodies
	rules    callbackPhaseRules
	dom      *dominance.ImmediateDominators

	registrations map[string][]registeredPhaseCallback
	summaries     map[phaseSummaryKey]summary.Summary
}

type registeredPhaseCallback struct {
	phase string
	point cfg.Point
	expr  factflow.ExprRef
	fn    *ast.FunctionExpr
}

type phaseSummaryKey struct {
	point cfg.Point
	fn    *ast.FunctionExpr
}

func newCallbackPhaseTracker(keys *programKeys, owner summary.SummaryKey, prepass *body.Result, config body.Config, prepared preparedBodies) *callbackPhaseTracker {
	rules := callbackPhaseRulesFromConfig(config)
	if rules.empty() || keys == nil || prepass == nil || prepass.Graph() == nil || config.Registry == nil {
		return nil
	}
	return &callbackPhaseTracker{
		keys:     keys,
		owner:    owner,
		prepass:  prepass,
		config:   config,
		prepared: prepared,
		rules:    rules,
		dom:      dominance.ComputeImmediateDominatorInfo(prepass.Graph()),
	}
}

func (t *callbackPhaseTracker) observeRegistration(point cfg.Point, site factflow.CallSiteView) {
	if t == nil {
		return
	}
	name, ok := t.prepass.CallSignatureNameAtPoint(point)
	if !ok {
		return
	}
	for _, rule := range t.rules.registrations[name] {
		fn, expr, ok := callbackPhaseFunctionArg(t.keys, t.prepass, site, rule.CallbackParam)
		if !ok {
			continue
		}
		if t.registrations == nil {
			t.registrations = make(map[string][]registeredPhaseCallback)
		}
		t.registrations[rule.Phase] = append(t.registrations[rule.Phase], registeredPhaseCallback{
			phase: rule.Phase,
			point: point,
			expr:  expr,
			fn:    fn,
		})
	}
}

func (t *callbackPhaseTracker) collectInvocationContext(point cfg.Point, site factflow.CallSiteView) (map[summary.SummaryKey]struct{}, bool) {
	if t == nil {
		return nil, false
	}
	name, ok := t.prepass.CallSignatureNameAtPoint(point)
	if !ok {
		return nil, false
	}
	invocations := t.rules.invocations[name]
	if len(invocations) == 0 {
		return nil, false
	}
	caller, ok := t.prepass.StateAt(point)
	if !ok {
		return nil, true
	}
	entryKeys := t.prepass.KeySpace()
	var changed map[summary.SummaryKey]struct{}
	controlled := false
	for _, invocation := range invocations {
		callbackFn, expr, ok := callbackPhaseFunctionArg(t.keys, t.prepass, site, invocation.CallbackParam)
		if !ok {
			continue
		}
		controlled = true
		entry := state.State{}
		hasPathEntry := false
		if pathEntry, ok := callerPathEntryState(t.config.Registry, entryKeys, caller); ok {
			entry = pathEntry
			hasPathEntry = true
		}
		entry, hasCaptureEntry := applyCapturedClosureEntryState(t.config.Registry, entryKeys, t.keys.bindings, callbackFn, caller, entry, captureValueReaderAt(t.prepass, point))
		entry, hasPhaseEntry := t.applyBeforePhases(point, invocation.Before, caller, entry, entryKeys)
		if len(invocation.Before) != 0 && !hasPhaseEntry {
			continue
		}
		if !hasPathEntry && !hasCaptureEntry && !hasPhaseEntry {
			continue
		}
		callbackType, _ := lowerFunctionExprType(callbackFn, t.keys.bindings, t.config.ModuleTypes)
		if key, ok := addFunctionExpressionContextKey(t.config.Registry, t.keys, t.owner, expr, callbackSymbol(t.keys, t.prepass, expr), callbackFn, entry, entryKeys, callbackType); ok {
			changed = addChangedContextKey(changed, key)
		}
	}
	return changed, controlled
}

func callbackPhaseFunctionArg(keys *programKeys, result *body.Result, site factflow.CallSiteView, index int) (*ast.FunctionExpr, factflow.ExprRef, bool) {
	if keys == nil || result == nil || index < 0 {
		return nil, 0, false
	}
	source, ok := callArgumentSourceAt(site, index)
	if !ok || !source.HasExpr || source.ExprRef == 0 {
		return nil, 0, false
	}
	sym, ok := result.ExpressionFunction(source.ExprRef)
	if !ok || sym == 0 {
		return nil, 0, false
	}
	fn, ok := keys.bindings.FunctionBySymbol(sym)
	if !ok || fn == nil {
		return nil, 0, false
	}
	return fn, source.ExprRef, true
}

func callbackSymbol(keys *programKeys, result *body.Result, expr factflow.ExprRef) symbol.ID {
	if keys == nil || result == nil || expr == 0 {
		return 0
	}
	sym, _ := result.ExpressionFunction(expr)
	return sym
}

func (t *callbackPhaseTracker) applyBeforePhases(point cfg.Point, phases []string, caller state.State, entry state.State, entryKeys *keyspace.KeySpace) (state.State, bool) {
	if t == nil || len(phases) == 0 {
		return entry, false
	}
	seen := false
	for _, phase := range phases {
		for _, registration := range t.registrations[phase] {
			if !t.registrationDominates(registration.point, point) {
				continue
			}
			sum, ok := t.phaseCallbackSummary(point, registration, caller, entryKeys)
			if !ok {
				continue
			}
			var wrote bool
			entry, wrote = applyPersistentPathWritesToEntry(t.config.Registry, entryKeys, entry, sum)
			seen = seen || wrote
		}
	}
	return entry, seen
}

func (t *callbackPhaseTracker) registrationDominates(registration, invocation cfg.Point) bool {
	return t != nil && t.dom != nil && t.dom.Dominates(registration, invocation)
}

func (t *callbackPhaseTracker) phaseCallbackSummary(point cfg.Point, registration registeredPhaseCallback, caller state.State, entryKeys *keyspace.KeySpace) (summary.Summary, bool) {
	if t == nil || registration.fn == nil {
		return summary.Summary{}, false
	}
	key := phaseSummaryKey{point: point, fn: registration.fn}
	if t.summaries != nil {
		if sum, ok := t.summaries[key]; ok {
			return sum, true
		}
	}
	prepared := t.prepared.function(registration.fn)
	if prepared == nil {
		return summary.Summary{}, false
	}
	entry := state.State{}
	if pathEntry, ok := callerPathEntryState(t.config.Registry, entryKeys, caller); ok {
		entry = pathEntry
	}
	entry, _ = applyCapturedClosureEntryState(t.config.Registry, entryKeys, t.keys.bindings, registration.fn, caller, entry, captureValueReaderAt(t.prepass, point))
	callbackConfig := cloneCheckConfig(t.config)
	callbackConfig.EntryState = entry.RekeyPathEvidence(entryKeys, prepared.KeySpace())
	result, err := solvePrepared(prepared, callbackConfig)
	if err != nil || result == nil {
		return summary.Summary{}, false
	}
	sum := summaryprojection.FromResult(result).RekeyHeapTableObjects(entryKeys)
	if t.summaries == nil {
		t.summaries = make(map[phaseSummaryKey]summary.Summary)
	}
	t.summaries[key] = sum
	return sum, true
}

func applyPersistentPathWritesToEntry(reg *axis.Registry, ks *keyspace.KeySpace, entry state.State, sum summary.Summary) (state.State, bool) {
	if reg == nil || ks == nil || len(sum.NormalReturnFacts.PersistentPathWrites) == 0 {
		return entry, false
	}
	out := entry
	for id, object := range sum.HeapTableObjects {
		out = out.WriteHeapTableObject(reg, id, object)
	}
	valueEdit := out.EditValues(reg)
	pathEdit := out.EditPathEvidence(reg)
	seen := false
	for _, fact := range sum.NormalReturnFacts.PersistentPathWrites {
		if fact.Path.Symbol == 0 {
			continue
		}
		if len(fact.Path.Segments) == 0 {
			valueEdit.Write(statekey.SymbolValue(fact.Path.Symbol), fact.Value)
		} else {
			pathEdit.WritePathStaticMember(ks, fact.Path.Key(), fact.Value)
		}
		seen = true
	}
	out = pathEdit.DoneOn(out)
	out = valueEdit.DoneOn(out)
	return out, seen
}
