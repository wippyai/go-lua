package check

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/effectlowering"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/readexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

var (
	ErrRegistryRequired = errors.New("check: registry is required")
	ErrUnsupportedCFG   = errors.New("check: unsupported cfg")
)

type Config struct {
	Registry *axis.Registry
	Globals  []string

	ExpressionValues map[factflow.ExprRef]product.Value
	ExpressionValue  sourcevalue.ExpressionValueProvider
	VarargValue      sourcevalue.VarargValueProvider
	CallResults      factapply.CallResultProvider
	SummaryResults   summary.Reader
	SummaryKeyFor    callresult.KeyFunc
	Signatures       signaturelookup.Source

	Visibility *visibility.Resolver

	EntryState state.State
	Initial    transfer.InitialState

	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int
}

type Checker struct {
	config Config
}

type Result struct {
	registry    *axis.Registry
	bindings    *bind.Result
	cfg         *cfgbuild.Result
	semantics   *semantics.Result
	signatures  signaturelookup.Source
	facts       factflow.Facts
	flow        transfer.Result
	visibility  *visibility.Resolver
	sources     sourcevalue.SourceValues
	callResults factapply.CallResultProvider
	functions   []*Result
}

func (r *Result) Registry() *axis.Registry {
	if r == nil {
		return nil
	}
	return r.registry
}

func (r *Result) Graph() cfg.Graph {
	if r == nil || r.cfg == nil {
		return nil
	}
	return r.cfg.Graph
}

func (r *Result) StateAt(point cfg.Point) (state.State, bool) {
	if r == nil || r.flow == nil {
		return state.State{}, false
	}
	st, ok := r.flow[point]
	if !ok {
		return state.State{}, false
	}
	return st.Clone(), true
}

func (r *Result) ExitState() (state.State, bool) {
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	return r.StateAt(graph.Exit())
}

func (r *Result) EntryState() (state.State, bool) {
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	return r.StateAt(graph.Entry())
}

func (r *Result) ReturnPoints() []cfg.Point {
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	points := graph.RPO()
	out := make([]cfg.Point, 0, len(points))
	for _, point := range points {
		if _, ok := r.ReturnFact(point); ok {
			out = append(out, point)
		}
	}
	return out
}

func (r *Result) ReturnFact(point cfg.Point) (semantics.ReturnFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.ReturnFact{}, false
	}
	return r.semantics.Return(point)
}

func (r *Result) LocalAssignment(point cfg.Point) (semantics.LocalAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.LocalAssignmentFact{}, false
	}
	return r.semantics.LocalAssignment(point)
}

func (r *Result) ObjectLiteral(expr ast.Expr) (semantics.ObjectLiteralFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.ObjectLiteralFact{}, false
	}
	return r.semantics.ObjectLiteral(expr)
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (semantics.OrdinaryAssignmentFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.OrdinaryAssignmentFact{}, false
	}
	return r.semantics.OrdinaryAssignment(point)
}

func (r *Result) Call(point cfg.Point) (semantics.CallFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.CallFact{}, false
	}
	return r.semantics.Call(point)
}

func (r *Result) CallSite(point cfg.Point) (factflow.CallSite, bool) {
	if r == nil {
		return factflow.CallSite{}, false
	}
	return r.facts.CallSite(point)
}

func (r *Result) NoNormalReturn(point cfg.Point) bool {
	if r == nil {
		return false
	}
	return r.facts.NoNormalReturn(point)
}

func (r *Result) BranchCondition(point cfg.Point) (semantics.BranchConditionFact, bool) {
	if r == nil || r.semantics == nil {
		return semantics.BranchConditionFact{}, false
	}
	return r.semantics.BranchCondition(point)
}

func (r *Result) TypeDefinition(point cfg.Point) (cfgfacts.TypeDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.TypeDefinitionFact{}, false
	}
	return r.semantics.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.FunctionDefinitionFact{}, false
	}
	return r.semantics.FunctionDefinition(point)
}

func (r *Result) NumericFor(point cfg.Point) (cfgfacts.NumericForFact, bool) {
	if r == nil || r.semantics == nil {
		return cfgfacts.NumericForFact{}, false
	}
	return r.semantics.NumericFor(point)
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil || r.semantics == nil {
		return nil
	}
	return r.semantics.Function()
}

func (r *Result) FunctionResults() []*Result {
	if r == nil || len(r.functions) == 0 {
		return nil
	}
	return append([]*Result(nil), r.functions...)
}

func (r *Result) SymbolName(id symbol.ID) string {
	if r == nil || r.bindings == nil {
		return ""
	}
	return r.bindings.Name(id)
}

func (r *Result) SymbolTypeAnnotation(id symbol.ID) (ast.TypeExpr, bool) {
	if r == nil || r.bindings == nil || id == 0 {
		return nil, false
	}
	if fn, ok := r.bindings.DeclaringFunction(id); ok {
		for _, slot := range r.bindings.ParamSlots(fn) {
			if slot.Symbol == id && slot.Type != nil {
				return slot.Type, true
			}
		}
	}
	graph := r.Graph()
	if graph == nil {
		return nil, false
	}
	for _, point := range graph.RPO() {
		fact, ok := r.LocalAssignment(point)
		if ok && fact.Symbol == id && fact.Type != nil {
			return fact.Type, true
		}
	}
	return nil, false
}

func (r *Result) ExpressionPath(expr ast.Expr) (path.Path, bool) {
	if r == nil || r.bindings == nil {
		return path.Path{}, false
	}
	return pathexpr.Resolve(expr, r.bindings)
}

func (r *Result) CallSignature(site factflow.CallSite) (signature.Function, bool) {
	if r == nil {
		return signature.Function{}, false
	}
	name, ok := r.stableCalleeName(site.CalleeSymbol(), site.CalleePath())
	if !ok {
		return signature.Function{}, false
	}
	return r.signatures.Lookup(name)
}

func (r *Result) ReturnArity(point cfg.Point) (int, bool) {
	if r == nil {
		return 0, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return 0, false
	}
	return len(fact.Sources()), true
}

func (r *Result) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	if r == nil {
		return nil, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return nil, false
	}
	return fact.Sources(), true
}

func (r *Result) ReturnPresenceRelations(point cfg.Point) []factflow.ReturnPresenceRelation {
	if r == nil {
		return nil
	}
	return r.facts.ReturnPresenceRelations(point)
}

func (r *Result) ExpressionCondition(ref factflow.ExprRef) (factflow.ExpressionCondition, bool) {
	if r == nil {
		return factflow.ExpressionCondition{}, false
	}
	return r.facts.ExpressionCondition(ref)
}

func (r *Result) ParameterValueSlots() []statekey.Value {
	if r == nil || r.bindings == nil {
		return nil
	}
	slots := r.bindings.ParamSlots(r.Function())
	out := make([]statekey.Value, 0, len(slots))
	for _, slot := range slots {
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == "" {
			continue
		}
		out = append(out, valueSlot)
	}
	return out
}

func (r *Result) ReassignedParameterValueSlots() map[statekey.Value]struct{} {
	if r == nil || r.bindings == nil {
		return nil
	}
	params := make(map[statekey.Value]struct{})
	for _, slot := range r.ParameterValueSlots() {
		params[slot] = struct{}{}
	}
	if len(params) == 0 {
		return nil
	}
	out := make(map[statekey.Value]struct{})
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	for _, point := range graph.RPO() {
		assignment, ok := r.facts.OrdinaryAssignment(point)
		if !ok {
			continue
		}
		slot := statekey.SymbolValue(assignment.TargetSymbol())
		if _, ok := params[slot]; ok {
			out[slot] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Result) LocalSymbols(stmt *ast.LocalAssignStmt) []symbol.ID {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.LocalSymbols(stmt)
}

func (r *Result) IsImplicitGlobalUse(ident *ast.IdentExpr) bool {
	if r == nil || r.bindings == nil {
		return false
	}
	return r.bindings.IsImplicitGlobalUse(ident)
}

func (r *Result) TypeRef(ref *ast.TypeRefExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeRef(ref)
}

func (r *Result) PrimitiveTypeRef(expr *ast.PrimitiveTypeExpr) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.PrimitiveTypeRef(expr)
}

func (r *Result) TypeDef(stmt *ast.TypeDefStmt) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.TypeDef(stmt)
}

func (r *Result) InterfaceDef(stmt *ast.InterfaceDefStmt) (bind.TypeDecl, bool) {
	if r == nil || r.bindings == nil {
		return bind.TypeDecl{}, false
	}
	return r.bindings.InterfaceDef(stmt)
}

func (r *Result) TypeDefParams(stmt *ast.TypeDefStmt) []bind.TypeDecl {
	if r == nil || r.bindings == nil {
		return nil
	}
	return r.bindings.TypeDefParams(stmt)
}

func (r *Result) SymbolValueAt(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if r == nil || id == 0 {
		return product.Value{}, false
	}
	st, ok := r.StateAt(point)
	if !ok {
		return product.Value{}, false
	}
	value := st.ReadValue(r.registry, statekey.SymbolValue(id))
	if product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func New(config Config) (*Checker, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}
	return &Checker{config: copyConfig(config)}, nil
}

func CheckChunk(stmts []ast.Stmt, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckChunk(stmts)
}

// CheckBoundChunk checks a chunk using caller-supplied lexical bindings.
func CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckBoundChunk(stmts, bindings)
}

func CheckFunction(fn *ast.FunctionExpr, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckFunction(fn)
}

// CheckBoundFunction checks a function using caller-supplied lexical bindings.
func CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (*Result, error) {
	checker, err := New(config)
	if err != nil {
		return nil, err
	}
	return checker.CheckBoundFunction(fn, bindings)
}

func (c *Checker) CheckChunk(stmts []ast.Stmt) (*Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(c.config)})
	return c.CheckBoundChunk(stmts, bindings)
}

func (c *Checker) CheckBoundChunk(stmts []ast.Stmt, bindings *bind.Result) (*Result, error) {
	summaries, err := c.functionSummaries(stmts, bindings)
	if err != nil {
		return nil, err
	}
	checker := c.withSummaryApplication(summaries)
	return checker.checkBoundChunk(stmts, bindings, summaries)
}

func (c *Checker) checkBoundChunk(stmts []ast.Stmt, bindings *bind.Result, summaries summaryApplication) (*Result, error) {
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract chunk semantics: %w", err)
	}
	result := c.run(bindings, built, sem)
	c.attachFunctionResults(result, bindings, nil, summaries)
	return result, nil
}

func (c *Checker) CheckFunction(fn *ast.FunctionExpr) (*Result, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: configGlobals(c.config)})
	return c.CheckBoundFunction(fn, bindings)
}

func (c *Checker) CheckBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
	var stmts []ast.Stmt
	if fn != nil {
		stmts = fn.Stmts
	}
	summaries, err := c.functionSummaries(stmts, bindings)
	if err != nil {
		return nil, err
	}
	checker := c.withSummaryApplication(summaries)
	result, err := checker.checkBoundFunctionBody(fn, bindings)
	if err != nil {
		return nil, err
	}
	checker.attachFunctionResults(result, bindings, fn, summaries)
	return result, nil
}

func (c *Checker) checkBoundFunctionBody(fn *ast.FunctionExpr, bindings *bind.Result) (*Result, error) {
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		return nil, ErrUnsupportedCFG
	}
	sem, err := semantics.ExtractFunction(fn, bindings, built)
	if err != nil {
		return nil, fmt.Errorf("check: extract function semantics: %w", err)
	}
	return c.run(bindings, built, sem), nil
}

func (c *Checker) run(bindings *bind.Result, built *cfgbuild.Result, sem *semantics.Result) *Result {
	config := c.config
	facts := transferfacts.Lower(sem, built.Graph, transferfacts.Config{Registry: config.Registry, Bindings: bindings})
	if hasSignatures(config.Signatures) {
		facts = effectlowering.WithSignatureRelations(effectlowering.SignatureRelationConfig{
			Graph:      built.Graph,
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
		})
		facts = effectlowering.WithSignatureNoNormalReturns(effectlowering.SignatureNoNormalReturnConfig{
			Graph:      built.Graph,
			Registry:   config.Registry,
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
		})
		facts = effectlowering.WithSignaturePostconditions(effectlowering.SignaturePostconditionConfig{
			Graph:      built.Graph,
			Registry:   config.Registry,
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
		})
		facts = effectlowering.WithSignatureMutations(effectlowering.SignatureMutationConfig{
			Graph:      built.Graph,
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
		})
	}
	if config.SummaryResults != nil && config.SummaryKeyFor != nil {
		facts = effectlowering.WithSummaryPostconditions(effectlowering.SummaryPostconditionConfig{
			Graph:     built.Graph,
			Registry:  config.Registry,
			Summaries: config.SummaryResults,
			KeyFor:    config.SummaryKeyFor,
			Facts:     facts,
		})
	}
	resolver := config.Visibility
	if resolver == nil {
		resolver = defaultVisibilityResolver(bindings, built, facts)
	}
	expressionValue := config.ExpressionValue
	if expressionValue == nil {
		expressionValue = readexpr.Provider(readexpr.Config{
			Registry:   config.Registry,
			Facts:      facts,
			Visibility: resolver,
		})
	}
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:         config.Registry,
		ExpressionValues: config.ExpressionValues,
		ExpressionValue:  expressionValue,
		VarargValue:      config.VarargValue,
	})
	callResults := config.CallResults
	if hasSignatures(config.Signatures) {
		callResults = callresult.Fallback(callResults, effectlowering.SignatureProvider(effectlowering.SignatureProviderConfig{
			Signatures: config.Signatures,
			NameFor:    c.signatureNameForCall(bindings),
			Facts:      facts,
			Sources:    sources,
		}))
	}
	entryState, initial := parameterEntryState(
		config.Registry,
		built.Graph,
		bindings,
		sem.Function(),
		config.EntryState,
		config.Initial,
	)
	flow := transfer.Run(transfer.Config{
		Graph:      built.Graph,
		Registry:   config.Registry,
		EntryState: entryState,
		Initial:    initial,
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sources,
			CallResults: callResults,
			Visibility:  resolver,
		}),
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{
			Facts:      facts,
			Visibility: resolver,
		}),
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	})
	return &Result{
		registry:    config.Registry,
		bindings:    bindings,
		cfg:         built,
		semantics:   sem,
		signatures:  config.Signatures,
		facts:       facts,
		flow:        flow,
		visibility:  resolver,
		sources:     sources,
		callResults: callResults,
	}
}

func (c *Checker) attachFunctionResults(parent *Result, bindings *bind.Result, fn *ast.FunctionExpr, summaries summaryApplication) {
	if parent == nil || bindings == nil {
		return
	}
	for _, nested := range bindings.NestedFunctions(fn) {
		child, ok := c.checkNestedFunction(nested, bindings, summaries)
		if !ok {
			continue
		}
		c.attachFunctionResults(child, bindings, nested, summaries)
		parent.functions = append(parent.functions, child)
	}
}

func (c *Checker) checkNestedFunction(fn *ast.FunctionExpr, bindings *bind.Result, summaries summaryApplication) (*Result, bool) {
	checker := c.withSummaryApplication(summaries)
	result, err := checker.checkBoundFunctionBody(fn, bindings)
	if err != nil {
		return nil, false
	}
	return result, true
}

func copyConfig(config Config) Config {
	config.Globals = append([]string(nil), config.Globals...)
	config.Signatures.Manifests = append([]*manifest.Manifest(nil), config.Signatures.Manifests...)
	if len(config.ExpressionValues) != 0 {
		values := make(map[factflow.ExprRef]product.Value, len(config.ExpressionValues))
		for ref, value := range config.ExpressionValues {
			values[ref] = value
		}
		config.ExpressionValues = values
	}
	return config
}

func (c *Checker) signatureNameForCall(bindings *bind.Result) effectlowering.SignatureNameFunc {
	return func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
		result := Result{bindings: bindings}
		return result.stableCalleeName(call.CalleeSymbol(), call.CalleePath())
	}
}

func (r *Result) stableCalleeName(callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil || r.bindings == nil {
		return "", false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 {
		return "", false
	}
	kind, ok := r.bindings.Kind(root)
	if !ok || kind != symbol.Global {
		return "", false
	}
	name := r.bindings.Name(root)
	if name == "" {
		return "", false
	}
	if len(calleePath.Segments) == 0 {
		return name, true
	}
	var b strings.Builder
	b.WriteString(name)
	for _, seg := range calleePath.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return "", false
			}
			b.WriteByte('.')
			b.WriteString(seg.Name)
		default:
			return "", false
		}
	}
	return b.String(), true
}

func configGlobals(config Config) []string {
	globals := append([]string(nil), config.Globals...)
	if hasSignatures(config.Signatures) {
		for name := range config.Signatures.Signatures() {
			root := name
			if dot := strings.IndexByte(root, '.'); dot >= 0 {
				root = root[:dot]
			}
			if root != "" {
				globals = append(globals, root)
			}
		}
	}
	return globals
}

func hasSignatures(source signaturelookup.Source) bool {
	return source.IncludeStdlib || len(source.Manifests) != 0
}
