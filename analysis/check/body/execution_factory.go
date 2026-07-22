package body

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// ExecutionFactoryConfig contains only application-session inputs shared by
// every invocation of one prepared lexical body. Entry State remains owned by
// the relation-program root or call equation.
type ExecutionFactoryConfig struct {
	Context                      context.Context
	Session                      *cancellation.Session
	StateLanes                   []state.LaneID
	ClosedDynamicAllValues       []factapply.ClosedDynamicAllValueInvariant
	TypeValues                   *typevalue.Cache
	SignatureArgumentType        effectlowering.SignatureArgumentTypeProgram
	SignatureArgumentTypeFactory SignatureArgumentTypeFactory
}

// ExecutionFactory is the sole prepared concrete-semantics builder. It owns
// session-bound sources, the type-value cache and State domain configuration;
// it owns no solver, schedule, checkpoint, WTO, or invocation entry.
type ExecutionFactory struct {
	prepared              *Static
	context               context.Context
	session               *cancellation.Session
	sources               sourcevalue.SourceValues
	typeValues            *typevalue.Cache
	signatureArgumentType effectlowering.SignatureArgumentTypeProgram
	stateLanes            []state.LaneID
	stateOptions          state.DomainOptions
	domain                lattice.Lattice[state.State]
	productDomain         state.ProductDomain
	closedDynamic         []factapply.ClosedDynamicAllValueInvariant
	reads                 ReadGraph
}

// NewExecutionFactory binds the immutable/session-wide half of concrete body
// semantics exactly once. Routes created from it share prepared reads but not
// mutable point-executor scratch.
func (s *Static) NewExecutionFactory(config ExecutionFactoryConfig) (*ExecutionFactory, error) {
	if s == nil || s.registry == nil || s.cfg == nil || s.cfg.Graph == nil || s.visibility == nil || config.Session == nil {
		return nil, fmt.Errorf("body: execution factory requires prepared Static and an application session")
	}
	config.Context = cancellation.WithSession(config.Context, config.Session)
	sources := sourcevalue.BindSession(s.sources, config.Session)
	if s.readExpressionConfig != nil {
		readConfig := *s.readExpressionConfig
		readConfig.Context = &readexpr.Context{Cancel: config.Session.Token()}
		sources = sourcevalue.WithExpressionValue(sources, readexpr.Provider(readConfig))
	}
	typeValues := s.solveTypeValues(SolveConfig{TypeValues: config.TypeValues})
	options := state.DomainOptions{WidenThresholds: wideningThresholdsFromWIR(s.wir)}
	productDomain, err := state.TryRegisteredProductDomainWithOptionalLanesAndOptions(s.registry, config.StateLanes, options)
	if err != nil {
		return nil, err
	}
	signatureInputs, err := effectlowering.SealSignatureOutcomeOperands(productDomain, s.visibility.KeySpace())
	if err != nil {
		return nil, err
	}
	signatureArgumentType := s.signatureArgumentTypeProvider(SolveConfig{
		SignatureArgumentType: config.SignatureArgumentType, SignatureArgumentTypeFactory: config.SignatureArgumentTypeFactory,
	}, typeValues, signatureInputs)
	domain := productDomain.Lattice()
	return &ExecutionFactory{
		prepared: s, context: config.Context, session: config.Session, sources: sources,
		typeValues: typeValues, signatureArgumentType: signatureArgumentType,
		stateLanes: append([]state.LaneID(nil), config.StateLanes...), stateOptions: options, domain: domain, productDomain: productDomain,
		closedDynamic: append([]factapply.ClosedDynamicAllValueInvariant(nil), config.ClosedDynamicAllValues...),
		reads:         s.readGraph,
	}, nil
}

// ReadGraph returns the immutable intrabody point-dependency graph consumed by
// the replacement fixpoint scheduler. Callee tuple dependencies are separate.
func (f *ExecutionFactory) ReadGraph() ReadGraph {
	if f == nil {
		return ReadGraph{}
	}
	return f.reads
}

// CallOutcomeContext returns the immutable, body-owned call adapter for this
// application factory. Facts and structural callbacks come from Static;
// Sources and TypeValues are the exact session-bound instances used by the
// relation program. The returned value exposes no solver, result cache, or
// mutable Static implementation detail.
func (f *ExecutionFactory) CallOutcomeContext() CallOutcomeContext {
	if f == nil || f.prepared == nil {
		return CallOutcomeContext{}
	}
	return f.prepared.callOutcomeContextWithSources(f.typeValues, f.sources)
}

// ExternalCallOutcomeProgram returns the one prepared external-call semantic
// authority for this lexical body. It contains signature, module, ambient and
// callable-value producers, but no lexical-callee provider: lexical calls are
// owned exclusively by RelationProgram frames. The provider is immutable and
// is invoked only by the canonical external-call instruction.
func (f *ExecutionFactory) ExternalCallOutcomeProgram() callpayload.CallOutcomeProgram {
	if f == nil || f.prepared == nil {
		return callpayload.CallOutcomeProgram{}
	}
	return f.prepared.callOutcomeProvider(SolveConfig{
		SignatureArgumentType: f.signatureArgumentType,
	}, f.typeValues, f.signatureArgumentType, f.productDomain)
}

// CustomExpressionValueProvider returns the caller-supplied expression
// semantic authority, when present. The replacement relation program freezes
// only this typed callback; it never retains a body solver or syntax replay.
func (f *ExecutionFactory) CustomExpressionValueProvider() sourcevalue.ExpressionValueProvider {
	if f == nil || f.prepared == nil || !f.prepared.customExpressionValue {
		return nil
	}
	return f.prepared.customExpressionValueProvider
}

// GenericForMembership returns the prepared non-scalar membership authority.
// Scalar loop-variable projection is frozen into the canonical relation term
// DAG and is never recomputed here.
func (f *ExecutionFactory) GenericForMembership() factapply.GenericForMembershipAuthority {
	if f == nil || f.prepared == nil || f.prepared.visibility == nil {
		return nil
	}
	s := f.prepared
	return &genericForMembershipAuthority{
		facts: s.facts, typeValues: f.typeValues, resolver: s.visibility,
	}
}

// EntrySeedPlan returns the immutable prepared missing-only defaults for this
// lexical body. The detached plan is safe to freeze into a replacement
// application program and is sufficient for both root and Apply invocation
// routes. It owns no route State, sparse point seeds, or call provider.
func (f *ExecutionFactory) EntrySeedPlan() state.EntrySeedPlan {
	if f == nil || f.prepared == nil || !f.prepared.entrySeedsPrepared {
		return state.EntrySeedPlan{}
	}
	return state.NewEntrySeedPlan(f.prepared.entrySeeds)
}

// FreezeInitialStatePlan evaluates a route configuration's arbitrary sparse
// Initial callback exactly once over the prepared CFG and returns a finite,
// immutable equation plan. The replacement scheduler consumes this plan for
// every invocation of the lexical body; it never retains or re-enters the
// callback while solving.
//
// Entry precedence remains an execution rule: if the plan contains the entry
// point, that seed replaces the separately supplied EntryState before the
// body's missing-only EntrySeedPlan is applied. Non-entry seeds are independent
// point-coordinate constants.
func (f *ExecutionFactory) FreezeInitialStatePlan(initial transfer.InitialState) (state.InitialStatePlan, error) {
	if f == nil || f.prepared == nil || f.Graph() == nil || f.prepared.lexicalBodyID == (lexicalidentity.StableLexicalBodyID{}) {
		return state.InitialStatePlan{}, fmt.Errorf("body: initial-state plan requires an execution factory")
	}
	if err := f.Err(); err != nil {
		return state.InitialStatePlan{}, err
	}
	points := cfg.RPOReadOnly(f.Graph())
	ordered := make([]state.InitialCoordinate, len(points))
	for index, point := range points {
		ordered[index] = state.InitialCoordinate(point)
	}
	seeds := make([]state.InitialStateSeed, 0)
	if initial != nil {
		for _, point := range points {
			if err := f.Err(); err != nil {
				return state.InitialStatePlan{}, err
			}
			value, present := initial(point)
			if err := f.Err(); err != nil {
				return state.InitialStatePlan{}, err
			}
			if present {
				seeds = append(seeds, state.NewInitialStateSeed(state.InitialCoordinate(point), value))
			}
		}
	}
	plan, err := state.NewInitialStatePlan(f.prepared.lexicalBodyID, f.Graph().ID(), f.Graph().Size(), ordered, seeds)
	if err != nil {
		return state.InitialStatePlan{}, err
	}
	if err := f.Err(); err != nil {
		return state.InitialStatePlan{}, err
	}
	return plan, nil
}

// ModuleLoadTransaction returns the sole callback-free N0 producer authority
// frozen for point. The transaction retains the operation's exact ValueSource
// and versioned shared export-table identity; the replacement executor remains
// responsible for evaluating that source in its current world.
func (f *ExecutionFactory) ModuleLoadTransaction(point cfg.Point) (factapply.ModuleLoadTransaction, bool) {
	if f == nil || f.prepared == nil || f.prepared.operationPlan == nil {
		return factapply.ModuleLoadTransaction{}, false
	}
	return factapply.PlanModuleLoadTransaction(f.prepared.registry, f.prepared.operationPlan, point)
}

// SeedEntry applies the one canonical prepared entry-seed transaction to a
// dynamic caller boundary. Existing caller values are preserved and
// only missing declared seeds are added by Static.solveEntryState. As in
// transfer's canonical InitialSparse equation, the scheduler must mark the
// result reachable and normalize it through Domain before cell admission; seed
// construction itself does not own lattice or scheduling policy.
func (f *ExecutionFactory) SeedEntry(entry state.State, initial transfer.InitialState) (state.State, transfer.InitialState) {
	if f == nil || f.prepared == nil {
		return entry, initial
	}
	return f.prepared.solveEntryState(f.typeValues, entry, initial)
}

func (f *ExecutionFactory) Graph() cfg.Graph {
	if f == nil || f.prepared == nil || f.prepared.cfg == nil {
		return nil
	}
	return f.prepared.cfg.Graph
}

func (f *ExecutionFactory) Registry() *axis.Registry {
	if f == nil || f.prepared == nil {
		return nil
	}
	return f.prepared.registry
}

func (f *ExecutionFactory) KeySpace() *keyspace.KeySpace {
	if f == nil || f.prepared == nil || f.prepared.visibility == nil {
		return nil
	}
	return f.prepared.visibility.KeySpace()
}

// StructuralSourceIdentityContext exposes only the frozen source/environment
// identity required to assemble a structural computation manifest. It never
// enters a solve or asks the formal engine for an observation.
func (f *ExecutionFactory) StructuralSourceIdentityContext(ctx context.Context) (StructuralSourceIdentity, error) {
	if f == nil || f.prepared == nil {
		return StructuralSourceIdentity{}, fmt.Errorf("body: structural source identity requires an execution factory")
	}
	return f.prepared.StructuralSourceIdentityContext(ctx)
}

// OperationPlan returns the immutable plan owned by this execution factory.
// It is exposed here solely for structural-manifest inspection.
func (f *ExecutionFactory) OperationPlan() *operationplan.Plan {
	if f == nil || f.prepared == nil {
		return nil
	}
	return f.prepared.operationPlan
}

// RootAssignmentAuthority freezes the sole N4 root/value/object transaction
// kernel for this application session. Per-point descriptors carry no provider
// or Facts copy; all body-wide semantic authority is retained here once.
func (f *ExecutionFactory) RootAssignmentAuthority() *factapply.RootAssignmentAuthority {
	if f == nil || f.prepared == nil || f.prepared.operationPlan == nil {
		return nil
	}
	return factapply.NewRootAssignmentAuthority(
		f.prepared.PathSemanticAuthority(), f.prepared.operationPlan.Facts(), f.closedDynamic, f.productDomain,
	)
}

// ReturnAuthority freezes the sole N5 heap/placement/projection/slot kernel
// for this prepared body.
func (f *ExecutionFactory) ReturnAuthority() *factapply.ReturnAuthority {
	if f == nil || f.prepared == nil || f.prepared.operationPlan == nil {
		return nil
	}
	return factapply.NewReturnAuthority(f.prepared.PathSemanticAuthority(), f.prepared.operationPlan.Facts())
}

func (f *ExecutionFactory) Domain() lattice.Lattice[state.State] {
	if f == nil {
		return lattice.Lattice[state.State]{}
	}
	return f.domain
}

func (f *ExecutionFactory) ProductDomain() state.ProductDomain {
	if f == nil {
		return state.ProductDomain{}
	}
	return f.productDomain
}

// Err reports the application session's cooperative cancellation cause. The
// replacement scheduler must check it before and after each atomic coordinate
// transaction, matching the canonical solver's publication rule without importing
// that solver's scheduling semantics.
func (f *ExecutionFactory) Err() error {
	if f == nil || f.session == nil || f.session.Token() == nil {
		return fmt.Errorf("body: execution factory has no application session")
	}
	return f.session.Token().Err()
}

func (f *ExecutionFactory) StateLanes() []state.LaneID {
	if f == nil {
		return nil
	}
	return append([]state.LaneID(nil), f.stateLanes...)
}

func (f *ExecutionFactory) StateOptions() state.DomainOptions {
	if f == nil {
		return state.DomainOptions{}
	}
	out := f.stateOptions
	out.WidenThresholds = append([]int64(nil), out.WidenThresholds...)
	return out
}
