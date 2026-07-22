package transformer

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCompileEquationIRWalksEnvironmentWriteThroughExemplar(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	formalEnvironmentWriteSealRootCarrier(t, base)
	arena := base.bodies[0].relation.code.terms
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepEnvironmentWrite, slot: statekey.SymbolValue(101), value: arena.Root(Root{Kind: RootParam, Index: 0})}}, next: 2},
		{kind: relationNodeBottom},
	})
	compiler, err := equation.Skeleton().With("environment-write", equation.BindExistingKernel("transformer/formal-environment-write/v1"))
	if err != nil {
		t.Fatal(err)
	}
	// This tiny existing relation also has its structural nonreturning terminal.
	// It is bound only to let the test reach the environment-write exemplar;
	// Skeleton itself remains fail-closed for every non-exemplar family.
	compiler, err = compiler.With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		calls++
		contract, contractErr := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, uint64(calls)))
		if contractErr != nil {
			return equation.Draft{}, contractErr
		}
		body := equation.BodyID(occurrence.Body)
		entry := equation.EntryParameter{Body: body, Name: "entry"}
		operands := make([]equation.Operand, 0, len(contract.Operands))
		for _, role := range contract.Operands {
			operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte(role))})
		}
		return equation.Draft{Target: equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"}, Entry: entry, Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())}, Operands: operands}, nil
	})
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if calls != 2 || len(artifact.Equations) != 2 {
		t.Fatalf("walker calls/artifact = %d/%d, want 2/2", calls, len(artifact.Equations))
	}
	var environment equation.Equation
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == "environment-write" {
			environment = lowered
		}
	}
	if environment.KernelID != "transformer/formal-environment-write/v1" {
		t.Fatalf("environment kernel = %q", environment.KernelID)
	}
}

func TestCompileEquationIRWalksApplyThroughExistingFactApplyKernel(t *testing.T) {
	program := formalApplyEquationTestProgram(t)
	compiler, err := equation.Skeleton().With("apply", equation.BindExistingKernel("transformer/formal-apply/v1"))
	if err != nil {
		t.Fatal(err)
	}
	// The representative relation has caller and callee Outcome terminals in
	// addition to its Apply step. They are bound mechanically so the test can
	// reach the Apply occurrence without making Skeleton permissive.
	compiler, err = compiler.With("outcome", equation.BindExistingKernel("transformer/formal-outcome/v1"))
	if err != nil {
		t.Fatal(err)
	}
	compiler, err = compiler.With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	binder := func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		calls++
		contract, contractErr := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, uint64(calls)))
		if contractErr != nil {
			return equation.Draft{}, contractErr
		}
		if occurrence.Kind == OperatorApply {
			contract.Reads = []ContractSelector{
				{Role: AccessFlow, Name: "caller-predecessor"},
				{Role: AccessCalleeOutcome, Name: "stabilized-callee-outcome"},
				{Role: AccessGuard, Name: "application-guard"},
				{Role: AccessBoundary, Name: "apply-frame"},
				{Role: AccessAllocation, Name: "boundary-allocation"},
			}
			contract.Writes = []ContractSelector{
				{Role: AccessState, Name: "caller-result"},
				{Role: AccessState, Name: "caller-heap"},
				{Role: AccessDiagnostic, Name: "boundary-diagnostic"},
			}
			contract.Outcomes = []OutcomeKind{OutcomeNormal}
			contract.Dependencies = []ContractDependency{{Kind: "operator-contract-catalog", ID: FrozenOperatorContractCatalog().ContentID()}}
		}
		body := equation.BodyID(occurrence.Body)
		entry := equation.EntryParameter{Body: body, Name: "entry"}
		operands := make([]equation.Operand, 0, len(contract.Operands))
		for _, role := range contract.Operands {
			operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte("apply/" + string(role)))})
		}
		return equation.Draft{
			Target:     equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"},
			Entry:      entry,
			Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
			Operands:   operands,
		}, nil
	}
	artifact, err := program.CompileEquationIR(compiler, binder)
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if calls != 5 || len(artifact.Equations) != 5 {
		t.Fatalf("walker calls/artifact = %d/%d, want 5/5", calls, len(artifact.Equations))
	}
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorApply) {
			if lowered.KernelID != "transformer/formal-apply/v1" {
				t.Fatalf("apply kernel = %q", lowered.KernelID)
			}
			return
		}
	}
	t.Fatal("relation-template walk omitted Apply")
}

func TestApplyLoweringIsFailClosedUntilItsHookIsInstalled(t *testing.T) {
	body := equation.BodyID(lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("apply-missing-hook"))))
	draft := equation.Draft{
		Target:     equation.Coordinate{Body: body, Name: "apply-output"},
		Entry:      equation.EntryParameter{Body: body, Name: "entry"},
		Occurrence: equation.Occurrence{Kind: string(OperatorApply), ContractID: equation.ContentID(FrozenOperatorContractCatalog().ContentID())},
		Operands: []equation.Operand{
			{Role: string(AccessFlow), Term: equation.ClosedTerm([]byte("caller-flow"))},
			{Role: string(AccessCalleeOutcome), Term: equation.ClosedTerm([]byte("callee-outcome"))},
			{Role: string(AccessBoundary), Term: equation.ClosedTerm([]byte("apply-frame"))},
		},
	}
	_, err := equation.Skeleton().Compile(equation.Source{Drafts: []equation.Draft{draft}})
	if !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("missing Apply hook error = %v", err)
	}
}

func TestApplyEquationArtifactIdentityTracksOnlySemanticBindings(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("apply-equation-identity")))
	contract, err := NewOperatorContract(OperatorApply, formal.NewOccurrenceID(owner, 1))
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := equation.Skeleton().With("apply", equation.BindExistingKernel("transformer/formal-apply/v1"))
	if err != nil {
		t.Fatal(err)
	}
	body := equation.BodyID(owner)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	draft := func(name string, contractID equation.ContentID, flow string, guards []equation.Guard) equation.Draft {
		return equation.Draft{
			Target: equation.Coordinate{Body: body, Name: name}, Entry: entry, Guards: guards,
			Occurrence: equation.Occurrence{Kind: string(OperatorApply), ContractID: contractID},
			Operands: []equation.Operand{
				{Role: string(AccessFlow), Term: equation.ClosedTerm([]byte(flow))},
				{Role: string(AccessCalleeOutcome), Term: equation.ClosedTerm([]byte("callee-outcome"))},
				{Role: string(AccessBoundary), Term: equation.ClosedTerm([]byte("apply-frame"))},
			},
		}
	}
	contractID := equation.ContentID(contract.ContentID())
	left := draft("left", contractID, "caller-flow", nil)
	right := draft("right", contractID, "caller-flow", nil)
	first, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{left, right}})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{right, left}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentID() != reordered.ContentID() {
		t.Fatal("reordering Apply declarations changed the equation artifact identity")
	}
	for _, changed := range []equation.Draft{
		draft("left", contractID, "different-caller-flow", nil),
		draft("left", contractID, "caller-flow", []equation.Guard{{Body: body, Encoding: []byte("application-guard")}}),
	} {
		artifact, compileErr := compiler.Compile(equation.Source{Drafts: []equation.Draft{changed, right}})
		if compileErr != nil || artifact.ContentID() == first.ContentID() {
			t.Fatalf("semantic Apply binding did not change identity: %v", compileErr)
		}
	}
	changedContract, err := NewOperatorContract(OperatorApply, formal.NewOccurrenceID(owner, 2))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{draft("left", equation.ContentID(changedContract.ContentID()), "caller-flow", nil), right}})
	if err != nil || artifact.ContentID() == first.ContentID() {
		t.Fatalf("Apply contract identity did not affect artifact: %v", err)
	}
}

func formalApplyEquationTestProgram(t *testing.T) *RelationProgram {
	t.Helper()
	registry := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callerID, calleeID := lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1)
	callerTerms, calleeTerms := NewArena(registry), NewArena(registry)
	if !callerTerms.bindLexicalOwner(callerID) || !calleeTerms.bindLexicalOwner(calleeID) {
		t.Fatal("bind lexical owners")
	}
	shape := Shape{Params: 1, Results: 1}
	callerValue := callerTerms.Root(Root{Kind: RootParam})
	callerPath := callerTerms.Path(Root{Kind: RootParam})
	point := cfg.Point(17)
	if callerValue == 0 || callerPath == 0 || callerTerms.bindCallResult(point, 0) == 0 {
		t.Fatal("bind caller Apply vocabulary")
	}
	frame := callerTerms.relationFrame(2, point, 1, shape, []ValueTerm{callerValue}, []PathTerm{callerPath}, 1)
	if frame == 0 {
		t.Fatal("bind Apply frame")
	}
	bindFormalApplyTestEnvironment(t, callerTerms, shape, 100)
	bindFormalApplyTestEnvironment(t, calleeTerms, shape, 110)
	if err := callerTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := calleeTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	bindFormalApplyTestInputs(t, callerTerms, shape, 100)
	bindFormalApplyTestInputs(t, calleeTerms, shape, 110)

	callerEffects, calleeEffects := NewEffectArena(callerTerms), NewEffectArena(calleeTerms)
	callerReturn, exact := factapply.PlanReturnTransactionSources(factflow.Facts{}, 1, nil)
	if !exact {
		t.Fatal("freeze caller Outcome transaction")
	}
	callerCode := &relationCode{
		terms: callerTerms, effects: callerEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: frame}}}, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {returnTransaction: returnTransactionTerm{transaction: callerReturn}}}, contributions: []semanticContribution{{}},
	}
	calleeValue := calleeTerms.Root(Root{Kind: RootParam})
	calleeCode := &relationCode{
		terms: calleeTerms, effects: calleeEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {returnTransaction: testReturnTransactionTerm(t, 1, calleeValue)}}, contributions: []semanticContribution{{}},
	}
	closeAndFreezeRelationGuardTestForest(t, []*relationCode{callerCode, calleeCode})
	callerTerms.Seal()
	calleeTerms.Seal()
	callerEffects.Seal()
	calleeEffects.Seal()
	callerCode.sealed, calleeCode.sealed = true, true

	productDomain := state.RegisteredProductDomain(registry)
	program := &RelationProgram{
		registry: registry,
		bodies: []relationProgramBody{
			{body: callerID, variable: 1, keys: keyspace.New(), relation: Relation{shape: shape, arena: callerTerms, effects: callerEffects, descriptors: callerCode.descriptors, code: callerCode, root: 1}, productDomain: productDomain},
			{body: calleeID, variable: 2, keys: keyspace.New(), relation: Relation{shape: shape, arena: calleeTerms, effects: calleeEffects, descriptors: calleeCode.descriptors, code: calleeCode, root: 1}, productDomain: productDomain},
		},
		byBody: map[lexicalidentity.StableLexicalBodyID]relationVar{callerID: 1, calleeID: 2},
	}
	for index := range program.bodies {
		body := &program.bodies[index]
		resolver := visibility.NewResolver(visibility.NewTable(nil))
		paths := factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
		body.keys = resolver.KeySpace()
		body.domain = state.Domain(registry)
		body.pathSemantics = paths
		body.returns = factapply.NewReturnAuthority(paths, factflow.Facts{})
	}
	prepareFormalApplyTestProgram(t, program, frame)
	for index := range program.bodies {
		body := &program.bodies[index]
		body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph)
	}
	slots, err := freezeSlotSpace(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalSlots = slots
	program.formalFibers, err = freezeFormalFiberInventoryWithSlots(program, slots)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents, err = freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion, err = freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalTemplate, err = freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestCompileEquationIRWalksChannelSelectThroughExistingKernel(t *testing.T) {
	program := channelSelectEquationTestProgram(t)
	compiler, err := equation.Skeleton().With("channel-select", equation.BindExistingKernel("transformer/formal-channel-select/v1"))
	if err != nil {
		t.Fatal(err)
	}
	// The representative relation ends in the existing nonreturning terminal;
	// bind it only so the walk can reach the channel-select occurrence.
	compiler, err = compiler.With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var channelContract OperatorContract
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		contract, contractErr := channelSelectEquationContract(occurrence)
		if contractErr != nil {
			return equation.Draft{}, contractErr
		}
		if occurrence.Kind == OperatorChannelSelect {
			channelContract = contract
		}
		return channelSelectEquationDraft(occurrence, contract), nil
	})
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if len(artifact.Equations) != 2 {
		t.Fatalf("equation count = %d, want 2", len(artifact.Equations))
	}
	if !channelContract.ContentID().Valid() || len(channelContract.Operands) != 3 ||
		len(channelContract.Reads) != 5 || len(channelContract.Writes) != 2 {
		t.Fatalf("channel-select contract = %#v", channelContract)
	}
	var channel equation.Equation
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorChannelSelect) {
			channel = lowered
		}
	}
	if channel.KernelID != "transformer/formal-channel-select/v1" {
		t.Fatalf("channel-select kernel = %q", channel.KernelID)
	}
	if len(channel.Guards) != 1 || string(channel.Guards[0].Encoding) != "select-guard" {
		t.Fatalf("channel-select guards = %#v", channel.Guards)
	}
}

func TestCompileEquationIRChannelSelectFailsWithoutLowering(t *testing.T) {
	compiler, err := equation.Skeleton().With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = channelSelectEquationTestProgram(t).CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		contract, contractErr := channelSelectEquationContract(occurrence)
		if contractErr != nil {
			return equation.Draft{}, contractErr
		}
		return channelSelectEquationDraft(occurrence, contract), nil
	})
	if !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("error = %v, want unimplemented channel-select lowering", err)
	}
}

func TestChannelSelectLoweringArtifactIdentityUsesSemanticBindings(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("channel-select-equation-identity")))
	occurrence := RelationEquationOccurrence{Body: body, Kind: OperatorChannelSelect}
	contract, err := channelSelectEquationContract(occurrence)
	if err != nil {
		t.Fatal(err)
	}
	left := channelSelectEquationDraft(occurrence, contract)
	right := channelSelectEquationDraft(occurrence, contract)
	for index := 0; index < len(right.Operands)/2; index++ {
		other := len(right.Operands) - 1 - index
		right.Operands[index], right.Operands[other] = right.Operands[other], right.Operands[index]
	}
	compiler, err := equation.Skeleton().With("channel-select", equation.BindExistingKernel("transformer/formal-channel-select/v1"))
	if err != nil {
		t.Fatal(err)
	}
	leftArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{left}})
	if err != nil {
		t.Fatal(err)
	}
	rightArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{right}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() != rightArtifact.ContentID() {
		t.Fatal("channel-select artifact retained operand declaration order")
	}
	for index := range right.Operands {
		if right.Operands[index].Role == string(AccessState) {
			right.Operands[index].Term = equation.ClosedTerm([]byte("different-state-binding"))
			break
		}
	}
	changedArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{right}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == changedArtifact.ContentID() {
		t.Fatal("channel-select artifact omitted a semantic operand binding")
	}
	changedGuard := left
	changedGuard.Guards[0].Encoding = []byte("different-guard")
	guardArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{changedGuard}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == guardArtifact.ContentID() {
		t.Fatal("channel-select artifact omitted a semantic guard")
	}
	changedContract := contract
	changedContract.GuardAtoms = []string{"different-guard"}
	contractDraft := channelSelectEquationDraft(occurrence, changedContract)
	contractArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{contractDraft}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == contractArtifact.ContentID() {
		t.Fatal("channel-select artifact omitted its contract content")
	}
}

func channelSelectEquationTestProgram(t *testing.T) *RelationProgram {
	t.Helper()
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	point := cfg.Point(41)
	path := pathdom.NewPath(symbol.ID(101), "param").Field("channel")
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID: "equation-select", Kind: factflow.ChannelSelectReceive, Index: 2,
			ResultPath: path, HasResultPath: true, CasePath: path, HasCasePath: true,
			PayloadValue: typevalue.LiteralString(reg, "payload"), HasPayloadValue: true,
		})),
	}})
	return formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{
			kind: boundaryStepChannelSelect, guard: guard,
			channel: factapply.PlanChannelSelectTransaction(facts, point),
		}}, next: 2},
		{kind: relationNodeBottom},
	})
}

func channelSelectEquationContract(occurrence RelationEquationOccurrence) (OperatorContract, error) {
	ordinal := uint64(2)
	if occurrence.Kind == OperatorChannelSelect {
		ordinal = 1
	}
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, ordinal))
	if err != nil || occurrence.Kind != OperatorChannelSelect {
		return contract, err
	}
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "predecessor"},
		{Role: AccessState, Name: "channel-facts"},
		{Role: AccessState, Name: "path-values"},
		{Role: AccessState, Name: "values"},
		{Role: AccessGuard, Name: "select-guard"},
	}
	contract.Writes = []ContractSelector{
		{Role: AccessState, Name: "channel-facts"},
		{Role: AccessState, Name: "result-values"},
	}
	contract.GuardAtoms = []string{"select-guard"}
	return contract, nil
}

func channelSelectEquationDraft(occurrence RelationEquationOccurrence, contract OperatorContract) equation.Draft {
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		term := equation.ClosedTerm([]byte(role))
		if role == AccessGuard && occurrence.Kind == OperatorChannelSelect {
			term = equation.ClosedTerm([]byte("select-guard"))
		}
		operands = append(operands, equation.Operand{Role: string(role), Term: term})
	}
	draft := equation.Draft{
		Target: equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"},
		Entry:  entry,
		Occurrence: equation.Occurrence{
			Kind:       string(contract.Kind),
			ContractID: equation.ContentID(contract.ContentID()),
		},
		Operands: operands,
	}
	if occurrence.Kind == OperatorChannelSelect {
		draft.Guards = []equation.Guard{{Body: body, Encoding: []byte("select-guard")}}
	}
	return draft
}
