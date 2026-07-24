package front

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// ErrUnsupportedInstruction reports a WIR operation outside the front's
// admitted family. CompileBody never omits such an operation.
var ErrUnsupportedInstruction = errors.New("front: unsupported WIR instruction")

const (
	entryKernel                 = "front/entry/v1"
	writeKernel                 = "front/environment-write/v1"
	allocationTemplateKernel    = "front/allocation-template/v1"
	objectMaterializationKernel = "front/object-materialization/v1"
	pathReplacementKernel       = "front/path-replacement/v1"
	dynamicIndexReadKernel      = "front/dynamic-index-read/v1"
	pathInvalidationKernel      = "front/path-invalidation/v1"
	indexMutationKernel         = "front/index-mutation/v1"
	branchKernel                = "front/branch-relations/v1"
	applyKernel                 = "front/apply/v1"
	resultsKernel               = "front/call-results/v1"
	externalCallKernel          = "front/external-call/v1"
	genericForKernel            = "front/generic-for/v1"
	selectKernel                = "front/channel-select/v1"
	publicationKernel           = "front/publication/v1"
	claimKernel                 = "front/claim/v1"
	expressionKernel            = "front/expression/v1"
	entryName                   = "entry"
)

// CompileBody parses source and lowers its chunk through bind, cfgbuild, and
// wirlower before compiling the resulting complete equation source. The
// walking skeleton admits only the structural entry operation; later families
// are added explicitly rather than being skipped.
// Compilation is the front's complete admission result.  Artifact is always
// present; Cyclic is present exactly when the source CFG has a recurrence and
// carries the source-frozen WTO certificate for that same artifact.
type Compilation struct {
	Artifact equation.Artifact
	Cyclic   *equation.CyclicArtifact
	// Frozen is retained for every admitted body, including acyclic bodies.
	// Cyclic remains the execution-path signal; consumers that need a stable
	// interprocedural identity must use this certificate instead of rebuilding
	// a schedule from the artifact.
	Frozen equation.CyclicArtifact
	// WIR is the immutable lowered body that owns source topology.  Consumers
	// may inspect it only as descriptive input; evaluation remains exclusively
	// owned by Artifact and the engine kernels.
	WIR *wir.Body
	// Body is the stable lexical identity for this independently admitted WIR
	// body. Evaluation still starts exclusively at the root artifact.
	Body          equation.BodyID
	Prototype     wir.FunctionSymbolID
	PrototypeName string
	LexicalPath   []uint32
	Boundary      wir.BodyBoundary
	// RebindsBoundary records ordinary assignment to a formal or captured
	// declaration. Member/index writes are deliberately excluded: their heap
	// transport is a separate interprocedural boundary concern.
	RebindsBoundary bool
	// Nested holds the independently admitted lexical bodies owned by closure
	// allocations in Artifact. They retain the same WIR-derived equation form
	// and publication path as the enclosing body; a caller decides which body
	// entries are available to evaluate.
	Nested                    []Compilation
	ClaimSpans                map[string]wir.Span
	ClaimTargetSpans          map[string]wir.Span
	CallSpans                 map[string]wir.Span
	BranchSpans               map[string]wir.Span
	EffectSpans               map[string]wir.Span
	ExpressionSpans           map[string]wir.Span
	ReturnSpans               map[string]wir.Span
	QualifiedClaimSpans       map[SpanKey]wir.Span
	QualifiedClaimTargetSpans map[SpanKey]wir.Span
	QualifiedCallSpans        map[SpanKey]wir.Span
	QualifiedBranchSpans      map[SpanKey]wir.Span
	QualifiedEffectSpans      map[SpanKey]wir.Span
	// Catalog on the root indexes every complete lexical body. It remains
	// passive until child admission is enabled by the engine.
	Catalog BodyCatalog
	// ControlDiagnostics are parser-admitted, lexical control facts. They are
	// retained with the compiled body so the engine can publish them through the
	// same source-diagnostic boundary as equation facts.
	ControlDiagnostics []ControlDiagnostic
	// PolicyDiagnostics are complete lexical facts that are returned to the
	// project adapter but never enter the engine's unconditional diagnostic
	// stream. The adapter applies the caller's explicit diagnostic policy.
	PolicyDiagnostics []ControlDiagnostic
	// TypeDefinitions are the top-level declarations resolved by the exact
	// resolver that lowered WIR. Module publication uses these values directly:
	// reconstructing them with a second resolver would allocate a different
	// recursive identity graph from the declaration that annotates exported
	// values.
	TypeDefinitions map[string]typ.Type
}

type ControlDiagnostic struct {
	Key      string
	Code     string
	Message  string
	Span     wir.Span
	Evidence []ControlDiagnosticEvidence
	Help     string
	Labels   []ControlDiagnosticLabel
}

// ControlDiagnosticEvidence and ControlDiagnosticLabel retain source-owned
// lexical evidence without making the front depend on a presentation package.
type ControlDiagnosticEvidence struct {
	Span    wir.Span
	Kind    string
	Trust   string
	Message string
}

type ControlDiagnosticLabel struct {
	Span    wir.Span
	Message string
}

// SpanKey makes source anchors unambiguous across lexical bodies, whose local
// operation names all begin at op-00000000.
type SpanKey struct {
	Body       equation.BodyID
	Occurrence string
}

type BodyCatalog map[equation.BodyID]BodyCatalogEntry

// BodyCatalogEntry retains the frozen body data for later demand-driven
// admission. No incomplete child is inserted into a returned catalog.
type BodyCatalogEntry struct {
	Body          equation.BodyID
	Prototype     wir.FunctionSymbolID
	PrototypeName string
	LexicalPath   []uint32
	Boundary      wir.BodyBoundary
	Artifact      equation.Artifact
	Cyclic        *equation.CyclicArtifact
}

// Compile parses and lowers one complete body, retaining cyclic control-flow
// as a frozen equation certificate rather than rejecting it at the front door.
func Compile(source string) (Compilation, error) {
	return CompileWithResolver(source, nil)
}

// CompileWithResolver lowers source with the exact external type definitions
// admitted at the module boundary. Runtime imports remain explicit entry facts;
// the resolver is used only for annotation rehydration.
func CompileWithResolver(source string, external typeannotation.Resolver) (Compilation, error) {
	stmts, err := parse.ParseString(source, "<front>")
	if err != nil {
		return Compilation{}, fmt.Errorf("front: parse body: %w", err)
	}
	controlDiagnostics := validateControl(stmts)
	// channel is the ambient runtime module whose select form has a dedicated
	// WIR operation.  A local binding still shadows it, so arbitrary
	// user-authored .select methods remain ordinary calls.
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	// The sealed call-order view keeps recognized channel.select case calls
	// inside the select operation rather than emitting independent calls before
	// the select transaction.
	built := cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: true})
	if built == nil || built.Graph == nil {
		return Compilation{}, fmt.Errorf("front: build CFG")
	}
	resolver := typeresolve.NewWithExternal(bindings, external)
	typeDefinitions := resolveTopLevelTypeDefinitions(stmts, bindings, resolver)
	body := wirlower.LowerWithResolver("chunk", stmts, bindings, built, resolver)
	if body == nil {
		return Compilation{}, fmt.Errorf("front: lower WIR")
	}
	rootBody := bodyID(source)
	artifact, err := compileWIRForBody(rootBody, body, built.Graph, assignmentSnapshotStarts(stmts, built))
	if err != nil {
		return Compilation{}, err
	}
	claimSpans, claimTargetSpans := claimSpans(body, artifact)
	effects := effectSpans(body, artifact)
	compilation := newCompilation(rootBody, 0, "", body.LexicalPath(), body.Boundary(), body, artifact, mergeSpans(claimSpans, effectValueSpans(body, artifact)), mergeSpans(claimTargetSpans, effectTargetSpans(body, artifact)), callSpans(body, artifact), branchSpans(body, artifact), effects, expressionSpans(body, artifact))
	cyclic, err := freezeCyclicArtifact(artifact, body, built.Graph)
	if err != nil {
		return Compilation{}, err
	}
	compilation.Frozen = cyclic
	if graphHasCycle(built.Graph) {
		compilation.Cyclic = &cyclic
	}
	nested, err := compileNestedBodies(body, rootBody)
	if err != nil {
		return Compilation{}, err
	}
	compilation.Nested = nested
	compilation.ControlDiagnostics = append(compilation.ControlDiagnostics, adviceControlDiagnostics(body, built.Graph)...)
	compilation.PolicyDiagnostics = append(compilation.PolicyDiagnostics, advicePolicyDiagnostics(body, built.Graph)...)
	for _, child := range nested {
		compilation.ControlDiagnostics = append(compilation.ControlDiagnostics, child.ControlDiagnostics...)
		compilation.PolicyDiagnostics = append(compilation.PolicyDiagnostics, child.PolicyDiagnostics...)
	}
	catalog, err := catalogBodies(compilation)
	if err != nil {
		return Compilation{}, err
	}
	compilation.Catalog = catalog
	compilation.ControlDiagnostics = append(compilation.ControlDiagnostics, controlDiagnostics...)
	compilation.ControlDiagnostics = append(compilation.ControlDiagnostics, unresolvedReferenceDiagnostics(stmts, bindings, resolver)...)
	compilation.TypeDefinitions = typeDefinitions
	return compilation, nil
}

// resolveTopLevelTypeDefinitions resolves each provider declaration before WIR
// lowering. The shared resolver is the declaration authority for both the
// exported runtime shape and qualified consumer annotations, including
// recursive graphs and generic arguments.
func resolveTopLevelTypeDefinitions(stmts []ast.Stmt, bindings *bind.Result, resolver *typeresolve.Resolver) map[string]typ.Type {
	if bindings == nil || resolver == nil {
		return nil
	}
	definitions := make(map[string]typ.Type)
	for _, statement := range stmts {
		var declaration bind.TypeDecl
		var found bool
		switch typed := statement.(type) {
		case *ast.TypeDefStmt:
			declaration, found = bindings.TypeDef(typed)
		case *ast.InterfaceDefStmt:
			declaration, found = bindings.InterfaceDef(typed)
		}
		if !found || declaration.Name == "" {
			continue
		}
		if resolved, ok := resolver.Decl(declaration); ok && resolved != nil {
			definitions[declaration.Name] = resolved
		}
	}
	return definitions
}

func validateControl(stmts []ast.Stmt) []ControlDiagnostic {
	var diagnostics []ControlDiagnostic
	var visitFunction func([]ast.Stmt)
	visitFunction = func(body []ast.Stmt) {
		labels := make(map[string]bool)
		var collect func([]ast.Stmt)
		collect = func(items []ast.Stmt) {
			for _, item := range items {
				switch stmt := item.(type) {
				case *ast.LabelStmt:
					if labels[stmt.Name] {
						diagnostics = append(diagnostics, controlDiagnostic("duplicate_label", "duplicate label "+stmt.Name, ast.SpanOf(stmt)))
					}
					labels[stmt.Name] = true
				case *ast.DoBlockStmt:
					collect(stmt.Stmts)
				case *ast.WhileStmt:
					collect(stmt.Stmts)
				case *ast.RepeatStmt:
					collect(stmt.Stmts)
				case *ast.NumberForStmt:
					collect(stmt.Stmts)
				case *ast.GenericForStmt:
					collect(stmt.Stmts)
				case *ast.IfStmt:
					collect(stmt.Then)
					collect(stmt.Else)
				}
			}
		}
		collect(body)
		var visit func([]ast.Stmt, int)
		visit = func(items []ast.Stmt, loops int) {
			for _, item := range items {
				switch stmt := item.(type) {
				case *ast.BreakStmt:
					if loops == 0 {
						diagnostics = append(diagnostics, controlDiagnostic("break_outside_loop", "break is only valid inside a loop", ast.SpanOf(stmt)))
					}
				case *ast.GotoStmt:
					if !labels[stmt.Label] {
						diagnostics = append(diagnostics, controlDiagnostic("undefined_label", "undefined label "+stmt.Label, ast.SpanOf(stmt)))
					}
				case *ast.DoBlockStmt:
					visit(stmt.Stmts, loops)
				case *ast.WhileStmt:
					visit(stmt.Stmts, loops+1)
				case *ast.RepeatStmt:
					visit(stmt.Stmts, loops+1)
				case *ast.NumberForStmt:
					for _, bound := range []ast.Expr{stmt.Init, stmt.Limit, stmt.Step} {
						if numericForBoundIsRefuted(bound) {
							diagnostics = append(diagnostics, controlDiagnostic("numeric_for_bound_type", "numeric for loop bound must be a number", ast.SpanOf(bound)))
						}
					}
					visit(stmt.Stmts, loops+1)
				case *ast.GenericForStmt:
					visit(stmt.Stmts, loops+1)
				case *ast.IfStmt:
					visit(stmt.Then, loops)
					visit(stmt.Else, loops)
				case *ast.FuncDefStmt:
					if stmt.Func != nil {
						visitFunction(stmt.Func.Stmts)
					}
				case *ast.LocalAssignStmt:
					for _, expr := range stmt.Exprs {
						visitNestedFunctions(expr, visitFunction)
					}
				case *ast.AssignStmt:
					for _, expr := range stmt.Rhs {
						visitNestedFunctions(expr, visitFunction)
					}
				}
			}
		}
		visit(body, 0)
	}
	visitFunction(stmts)
	return diagnostics
}

// numericForBoundIsRefuted recognizes only source-literal bounds whose Lua
// runtime class cannot be numeric. Non-literals remain a normal equation
// obligation, so this lexical check never substitutes a guessed type for a
// variable, call result, or computed expression.
func numericForBoundIsRefuted(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch expr.(type) {
	case *ast.NumberExpr:
		return false
	case *ast.StringExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr:
		return true
	default:
		return false
	}
}

func controlDiagnostic(kind, message string, span ast.Span) ControlDiagnostic {
	return ControlDiagnostic{Key: "control." + kind, Message: message, Span: wir.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}}
}

// unresolvedReferenceDiagnostics reports lexical misses before equation
// evaluation.  A missing value still has Lua's nil read semantics, while a
// missing type has no type witness at all; neither condition is an engine
// transaction failure.  We intentionally report only a bare value use: calls
// and namespace receivers retain their existing provider/stdlib boundaries.
func unresolvedReferenceDiagnostics(stmts []ast.Stmt, bindings *bind.Result, resolver *typeresolve.Resolver) []ControlDiagnostic {
	if bindings == nil || resolver == nil {
		return nil
	}
	var out []ControlDiagnostic
	seen := make(map[string]bool)
	declaredTypes := declaredTypeNames(stmts)
	add := func(code, name, message, help string, span ast.Span) {
		if name == "" || !span.Valid() {
			return
		}
		key := code + "/" + strconv.Itoa(span.StartLine) + "/" + strconv.Itoa(span.StartCol)
		if seen[key] {
			return
		}
		seen[key] = true
		wireSpan := wir.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}
		out = append(out, ControlDiagnostic{
			Key: key, Code: code, Message: message, Span: wireSpan, Help: help,
			Evidence: []ControlDiagnosticEvidence{{Span: wireSpan, Kind: "abstract fact", Trust: "proven", Message: evidenceMessage(code, name)}},
			Labels:   []ControlDiagnosticLabel{{Span: wireSpan, Message: message}},
		})
	}
	var visitType func(ast.TypeExpr)
	visitType = func(expr ast.TypeExpr) {
		if expr == nil {
			return
		}
		switch item := expr.(type) {
		case *ast.PrimitiveTypeExpr:
			if !typ.BuiltinPrimitiveName(item.Name) {
				if _, ambientType := ambient.Lookup(item.Name); !ambientType && declaredTypes[item.Name] {
					if _, found := bindings.PrimitiveTypeRef(item); !found {
						add("type.reference.unresolved", item.Name, "unknown type "+item.Name, "Declare the type in scope", ast.SpanOf(item))
					}
				}
			}
		case *ast.TypeRefExpr:
			if len(item.Path) == 1 && declaredTypes[item.Path[0]] {
				if _, found := bindings.TypeRef(item); !found {
					add("type.reference.unresolved", item.Path[0], "unknown type "+item.Path[0], "Declare the type in scope", ast.SpanOf(item))
				}
			}
		}
		switch item := expr.(type) {
		case *ast.OptionalTypeExpr:
			visitType(item.Inner)
		case *ast.UnionTypeExpr:
			for _, value := range item.Types {
				visitType(value)
			}
		case *ast.IntersectionTypeExpr:
			for _, value := range item.Types {
				visitType(value)
			}
		case *ast.ArrayTypeExpr:
			visitType(item.Element)
		case *ast.MapTypeExpr:
			visitType(item.Key)
			visitType(item.Value)
		case *ast.RecordTypeExpr:
			for _, field := range item.Fields {
				visitType(field.Type)
			}
		case *ast.FunctionTypeExpr:
			for _, param := range item.TypeParams {
				visitType(param.Constraint)
			}
			for _, param := range item.Params {
				visitType(param.Type)
			}
			visitType(item.Variadic)
			for _, value := range item.Returns {
				visitType(value)
			}
		case *ast.GenericTypeExpr:
			for _, value := range item.Args {
				visitType(value)
			}
		case *ast.MetaTypeExpr:
			visitType(item.Inner)
		case *ast.TupleTypeExpr:
			for _, value := range item.Elements {
				visitType(value)
			}
		}
	}
	var visitStmts func([]ast.Stmt)
	var visitExpr func(ast.Expr, bool)
	visitExpr = func(expr ast.Expr, reportBare bool) {
		switch item := expr.(type) {
		case *ast.IdentExpr:
			if reportBare && bindings.IsImplicitGlobalUse(item) {
				if _, typeValue := bindings.TypeValueRef(item); typeValue {
					return
				}
				add("value.reference.unresolved", item.Value, "unknown value "+item.Value, "Declare the value", ast.SpanOf(item))
			}
		case *ast.AttrGetExpr:
			visitExpr(item.Object, false)
			if item.KeySyntax != ast.AttrKeyDot {
				visitExpr(item.Key, false)
			}
		case *ast.FuncCallExpr:
			visitExpr(item.Func, false)
			visitExpr(item.Receiver, false)
			for _, arg := range item.Args {
				visitExpr(arg, false)
			}
			for _, arg := range item.TypeArgs {
				visitType(arg)
			}
		case *ast.LogicalOpExpr:
			visitExpr(item.Lhs, false)
			visitExpr(item.Rhs, false)
		case *ast.RelationalOpExpr:
			visitExpr(item.Lhs, false)
			visitExpr(item.Rhs, false)
		case *ast.StringConcatOpExpr:
			visitExpr(item.Lhs, false)
			visitExpr(item.Rhs, false)
		case *ast.ArithmeticOpExpr:
			visitExpr(item.Lhs, true)
			visitExpr(item.Rhs, true)
		case *ast.UnaryMinusOpExpr:
			visitExpr(item.Expr, true)
		case *ast.UnaryNotOpExpr:
			visitExpr(item.Expr, false)
		case *ast.UnaryLenOpExpr:
			visitExpr(item.Expr, false)
		case *ast.UnaryBNotOpExpr:
			visitExpr(item.Expr, true)
		case *ast.CastExpr:
			visitExpr(item.Expr, false)
			visitType(item.Type)
		case *ast.NonNilAssertExpr:
			visitExpr(item.Expr, false)
		case *ast.TableExpr:
			for _, field := range item.Fields {
				if field != nil {
					visitExpr(field.Key, false)
					visitExpr(field.Value, false)
				}
			}
		case *ast.FunctionExpr:
			for _, value := range item.ReturnTypes {
				visitType(value)
			}
			visitStmts(item.Stmts)
		}
	}
	visitStmts = func(items []ast.Stmt) {
		for _, statement := range items {
			switch item := statement.(type) {
			case *ast.LocalAssignStmt:
				for _, value := range item.Types {
					visitType(value)
				}
				for _, value := range item.Exprs {
					visitExpr(value, false)
				}
			case *ast.AssignStmt:
				for _, value := range item.Lhs {
					visitExpr(value, false)
				}
				for _, value := range item.Rhs {
					visitExpr(value, false)
				}
			case *ast.FuncCallStmt:
				visitExpr(item.Expr, false)
			case *ast.ReturnStmt:
				for _, value := range item.Exprs {
					visitExpr(value, false)
				}
			case *ast.DoBlockStmt:
				visitStmts(item.Stmts)
			case *ast.WhileStmt:
				visitExpr(item.Condition, false)
				visitStmts(item.Stmts)
			case *ast.RepeatStmt:
				visitStmts(item.Stmts)
				visitExpr(item.Condition, false)
			case *ast.IfStmt:
				visitExpr(item.Condition, false)
				visitStmts(item.Then)
				visitStmts(item.Else)
			case *ast.NumberForStmt:
				visitExpr(item.Init, false)
				visitExpr(item.Limit, false)
				visitExpr(item.Step, false)
				visitStmts(item.Stmts)
			case *ast.GenericForStmt:
				for _, value := range item.Exprs {
					visitExpr(value, false)
				}
				visitStmts(item.Stmts)
			case *ast.FuncDefStmt:
				if item.Func != nil {
					visitExpr(item.Func, false)
				}
			case *ast.TypeDefStmt:
				visitType(item.Type)
			case *ast.InterfaceDefStmt:
				for _, parent := range item.Extends {
					visitType(parent)
				}
				for _, field := range item.Fields {
					visitType(field.Type)
				}
				for _, method := range item.Methods {
					if method.Type != nil {
						visitType(method.Type)
					}
				}
			}
		}
	}
	visitStmts(stmts)
	return out
}

// declaredTypeNames provides the minimal lexical-miss boundary needed here:
// only a name that exists in the file but is unavailable at this use site is
// a source-local unresolved reference. Other unbound names remain an explicit
// opaque type boundary, preserving the existing gradual annotation contract.
func declaredTypeNames(stmts []ast.Stmt) map[string]bool {
	names := make(map[string]bool)
	var visitExpr func(ast.Expr)
	var visit func([]ast.Stmt)
	visitExpr = func(expr ast.Expr) {
		switch item := expr.(type) {
		case *ast.FunctionExpr:
			visit(item.Stmts)
		case *ast.FuncCallExpr:
			visitExpr(item.Func)
			visitExpr(item.Receiver)
			for _, arg := range item.Args {
				visitExpr(arg)
			}
		case *ast.TableExpr:
			for _, field := range item.Fields {
				if field != nil {
					visitExpr(field.Key)
					visitExpr(field.Value)
				}
			}
		}
	}
	visit = func(items []ast.Stmt) {
		for _, statement := range items {
			switch item := statement.(type) {
			case *ast.TypeDefStmt:
				names[item.Name] = true
			case *ast.InterfaceDefStmt:
				names[item.Name] = true
			case *ast.DoBlockStmt:
				visit(item.Stmts)
			case *ast.WhileStmt:
				visit(item.Stmts)
			case *ast.RepeatStmt:
				visit(item.Stmts)
			case *ast.IfStmt:
				visit(item.Then)
				visit(item.Else)
			case *ast.NumberForStmt:
				visit(item.Stmts)
			case *ast.GenericForStmt:
				visit(item.Stmts)
			case *ast.FuncDefStmt:
				if item.Func != nil {
					visitExpr(item.Func)
				}
			case *ast.LocalAssignStmt:
				for _, expr := range item.Exprs {
					visitExpr(expr)
				}
			case *ast.AssignStmt:
				for _, expr := range item.Rhs {
					visitExpr(expr)
				}
			}
		}
	}
	visit(stmts)
	return names
}

func evidenceMessage(code, name string) string {
	if code == "type.reference.unresolved" {
		return "no type named " + name + " is declared in this scope"
	}
	return "no value named " + name + " is declared, predeclared, imported, or configured global in this scope"
}

func visitNestedFunctions(expr ast.Expr, visit func([]ast.Stmt)) {
	switch value := expr.(type) {
	case *ast.FunctionExpr:
		visit(value.Stmts)
	case *ast.FuncCallExpr:
		visitNestedFunctions(value.Func, visit)
		visitNestedFunctions(value.Receiver, visit)
		for _, arg := range value.Args {
			visitNestedFunctions(arg, visit)
		}
	case *ast.TableExpr:
		for _, field := range value.Fields {
			if field != nil {
				visitNestedFunctions(field.Key, visit)
				visitNestedFunctions(field.Value, visit)
			}
		}
	}
}

func newCompilation(body equation.BodyID, prototype wir.FunctionSymbolID, prototypeName string, lexicalPath []uint32, boundary wir.BodyBoundary, wirBody *wir.Body, artifact equation.Artifact, claims, claimTargets, calls, branches, effects, expressions map[string]wir.Span) Compilation {
	return Compilation{
		Artifact: artifact, WIR: wirBody, Body: body, Prototype: prototype, PrototypeName: prototypeName,
		LexicalPath: append([]uint32(nil), lexicalPath...), Boundary: boundary, RebindsBoundary: bodyRebindsBoundary(wirBody, boundary),
		ClaimSpans: claims, ClaimTargetSpans: claimTargets, CallSpans: calls,
		BranchSpans: branches, EffectSpans: effects, ExpressionSpans: expressions,
		ReturnSpans:               returnSpans(wirBody, artifact),
		QualifiedClaimSpans:       qualifySpans(body, claims),
		QualifiedClaimTargetSpans: qualifySpans(body, claimTargets),
		QualifiedCallSpans:        qualifySpans(body, calls),
		QualifiedBranchSpans:      qualifySpans(body, branches),
		QualifiedEffectSpans:      qualifySpans(body, effects),
	}
}

func bodyRebindsBoundary(body *wir.Body, boundary wir.BodyBoundary) bool {
	if body == nil {
		return false
	}
	for _, parameter := range boundary.Parameters {
		if body.SymbolHasWrite(parameter.Symbol) {
			return true
		}
	}
	for _, capture := range boundary.Captures {
		if body.SymbolHasWrite(capture.Symbol) {
			return true
		}
	}
	return false
}

func qualifySpans(body equation.BodyID, spans map[string]wir.Span) map[SpanKey]wir.Span {
	if len(spans) == 0 {
		return nil
	}
	qualified := make(map[SpanKey]wir.Span, len(spans))
	for occurrence, span := range spans {
		qualified[SpanKey{Body: body, Occurrence: occurrence}] = span
	}
	return qualified
}

// compileNestedBodies admits every complete WIR lexical child through the
// ordinary equation front. The parent WIR already owns the child body and CFG,
// so this is neither a second source traversal nor a child evaluator.
func compileNestedBodies(parent *wir.Body, root equation.BodyID) ([]Compilation, error) {
	if parent == nil {
		return nil, nil
	}
	protos := parent.Protos()
	children := make([]Compilation, 0, len(protos))
	for _, proto := range protos {
		if proto.Body == nil || proto.Graph == nil || proto.Name == "" {
			return nil, fmt.Errorf("front: nested prototype is incomplete")
		}
		childBody := lexicalBodyID(root, proto.LexicalPath)
		artifact, err := compileWIRForBody(childBody, proto.Body, proto.Graph, nil)
		if err != nil {
			return nil, fmt.Errorf("front: nested body %q: %w", proto.Name, err)
		}
		claimSpans, claimTargetSpans := claimSpans(proto.Body, artifact)
		effects := effectSpans(proto.Body, artifact)
		child := newCompilation(childBody, proto.Symbol, prototypeIdentity(proto), proto.LexicalPath, proto.Boundary, proto.Body, artifact, mergeSpans(claimSpans, effectValueSpans(proto.Body, artifact)), mergeSpans(claimTargetSpans, effectTargetSpans(proto.Body, artifact)), callSpans(proto.Body, artifact), branchSpans(proto.Body, artifact), effects, expressionSpans(proto.Body, artifact))
		cyclic, err := freezeCyclicArtifact(artifact, proto.Body, proto.Graph)
		if err != nil {
			return nil, fmt.Errorf("front: nested body %q: %w", proto.Name, err)
		}
		child.Frozen = cyclic
		if graphHasCycle(proto.Graph) {
			child.Cyclic = &cyclic
		}
		child.Nested, err = compileNestedBodies(proto.Body, root)
		if err != nil {
			return nil, err
		}
		child.ControlDiagnostics = append(child.ControlDiagnostics, adviceControlDiagnostics(proto.Body, proto.Graph)...)
		child.PolicyDiagnostics = append(child.PolicyDiagnostics, advicePolicyDiagnostics(proto.Body, proto.Graph)...)
		for _, nested := range child.Nested {
			child.ControlDiagnostics = append(child.ControlDiagnostics, nested.ControlDiagnostics...)
			child.PolicyDiagnostics = append(child.PolicyDiagnostics, nested.PolicyDiagnostics...)
		}
		children = append(children, child)
	}
	return children, nil
}

// prototypeIdentity is lexical, not display-based: nested compiler-generated
// names repeat across bodies, while a function symbol remains stable.
func prototypeIdentity(proto wir.FuncProto) string {
	return fmt.Sprintf("%s#%d", proto.Name, proto.Symbol)
}

func catalogBodies(root Compilation) (BodyCatalog, error) {
	catalog := make(BodyCatalog)
	var add func(Compilation) error
	add = func(compilation Compilation) error {
		if !compilation.Body.Valid() {
			return fmt.Errorf("front: catalog body has no lexical identity")
		}
		if _, exists := catalog[compilation.Body]; exists {
			return fmt.Errorf("front: duplicate lexical body identity")
		}
		catalog[compilation.Body] = BodyCatalogEntry{Body: compilation.Body, Prototype: compilation.Prototype, PrototypeName: compilation.PrototypeName, LexicalPath: append([]uint32(nil), compilation.LexicalPath...), Boundary: compilation.Boundary, Artifact: compilation.Artifact, Cyclic: compilation.Cyclic}
		for _, child := range compilation.Nested {
			if err := add(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := add(root); err != nil {
		return nil, err
	}
	return catalog, nil
}

// effectSpans retains WIR's operation anchors for facts owned by a mutation
// or call effect. It is source metadata only; the effect facts themselves stay
// entirely in the equation closure.
func effectSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	static, dynamic, reads, calls := make([]wir.Instruction, 0), make([]wir.Instruction, 0), make([]wir.Instruction, 0), make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpStaticMemberWrite:
			static = append(static, instruction)
		case wir.OpDynamicIndexWrite:
			dynamic = append(dynamic, instruction)
		case wir.OpAssign:
			if instruction.A.Kind == wir.OperandPath && len(body.Path(wir.PathRef(instruction.A.Ref)).Segments) != 0 {
				reads = append(reads, instruction)
			}
		case wir.OpCall:
			calls = append(calls, instruction)
		}
	}
	out := make(map[string]wir.Span, len(static)+len(dynamic)+len(reads)+len(calls))
	staticIndex, dynamicIndex, readIndex, callIndex := 0, 0, 0, 0
	for _, operation := range artifact.Equations {
		switch operation.Occurrence.Kind {
		case "path-replacement":
			if staticIndex < len(static) && static[staticIndex].TargetSpan.Valid() {
				out[operation.Target.Name] = static[staticIndex].TargetSpan
			}
			staticIndex++
		case "index-mutation":
			if dynamicIndex < len(dynamic) {
				span := dynamic[dynamicIndex].ContainerSpan
				if !span.Valid() {
					span = dynamic[dynamicIndex].TargetSpan
				}
				if span.Valid() {
					out[operation.Target.Name] = span
				}
			}
			dynamicIndex++
		case "environment-write":
			isStaticRead := false
			for _, operand := range operation.Operands {
				isStaticRead = isStaticRead || operand.Role == "source-display"
			}
			if isStaticRead && readIndex < len(reads) && reads[readIndex].ExprSpan.Valid() {
				out[operation.Target.Name] = reads[readIndex].ExprSpan
			}
			if isStaticRead {
				readIndex++
			}
		case "apply":
			if callIndex < len(calls) {
				span := calls[callIndex].CallSpan
				if !span.Valid() {
					span = calls[callIndex].CalleeSpan
				}
				if span.Valid() {
					out[operation.Target.Name] = span
				}
			}
			callIndex++
		}
	}
	return out
}

// effectTargetSpans retain the assignment-target side of a dynamic mutation.
// ClaimTargetSpans is the existing secondary-label channel, so merging these
// source-only anchors keeps effect diagnostics equally precise without adding
// a parallel diagnostic publication path.
func effectTargetSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	dynamic := make([]wir.Instruction, 0)
	static := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpDynamicIndexWrite {
			dynamic = append(dynamic, instruction)
		}
		if instruction.Op == wir.OpStaticMemberWrite {
			static = append(static, instruction)
		}
	}
	out := make(map[string]wir.Span, len(dynamic)+len(static))
	dynamicIndex, staticIndex := 0, 0
	for _, operation := range artifact.Equations {
		switch operation.Occurrence.Kind {
		case "index-mutation":
			if dynamicIndex < len(dynamic) && dynamic[dynamicIndex].ContainerSpan.Valid() {
				out[operation.Target.Name] = dynamic[dynamicIndex].ContainerSpan
			}
			dynamicIndex++
		case "path-replacement":
			if staticIndex < len(static) && static[staticIndex].TargetSpan.Valid() {
				out[operation.Target.Name] = static[staticIndex].TargetSpan
			}
			staticIndex++
		}
	}
	return out
}

// effectValueSpans supply the primary source anchor for type assignments that
// are owned by an index-mutation operation. Other effect diagnostics retain
// their container anchor through EffectSpans.
func effectValueSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	dynamic := make([]wir.Instruction, 0)
	static := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpDynamicIndexWrite {
			dynamic = append(dynamic, instruction)
		}
		if instruction.Op == wir.OpStaticMemberWrite {
			static = append(static, instruction)
		}
	}
	out := make(map[string]wir.Span, len(dynamic)+len(static))
	dynamicIndex, staticIndex := 0, 0
	for _, operation := range artifact.Equations {
		switch operation.Occurrence.Kind {
		case "index-mutation":
			if dynamicIndex < len(dynamic) && dynamic[dynamicIndex].TargetSpan.Valid() {
				out[operation.Target.Name] = dynamic[dynamicIndex].TargetSpan
			}
			dynamicIndex++
		case "path-replacement":
			if staticIndex < len(static) && static[staticIndex].ExprSpan.Valid() {
				out[operation.Target.Name] = static[staticIndex].ExprSpan
			}
			staticIndex++
		}
	}
	return out
}

func mergeSpans(first, second map[string]wir.Span) map[string]wir.Span {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	out := make(map[string]wir.Span, len(first)+len(second))
	for key, span := range first {
		out[key] = span
	}
	for key, span := range second {
		if _, exists := out[key]; !exists {
			out[key] = span
		}
	}
	return out
}

// returnSpans retains each publication's authored return expression span. A
// return contract is a publication fact, so its diagnostic must use the same
// operation identity rather than reconstructing an AST location at the host.
func returnSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	returns := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpReturn {
			returns = append(returns, instruction)
		}
	}
	out := make(map[string]wir.Span, len(returns))
	returnIndex := 0
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		if returnIndex < len(returns) && returns[returnIndex].ExprSpan.Valid() {
			out[operation.Target.Name] = returns[returnIndex].ExprSpan
		}
		returnIndex++
	}
	return out
}

// expressionSpans binds expression-owned diagnostics to the source span of
// the exact operand whose already-closed value refuted the operation. The
// metadata is carried by WIR, not rediscovered from source after evaluation.
func expressionSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	expressions := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpConcat {
			expressions = append(expressions, instruction)
		}
	}
	out := make(map[string]wir.Span)
	expressionIndex := 0
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "expression" || !expressionIsConcat(operation) || expressionIndex >= len(expressions) {
			continue
		}
		instruction := expressions[expressionIndex]
		expressionIndex++
		for index, meta := range body.ConcatOperandMeta(instruction.ConcatOperands) {
			if meta.Span.Valid() {
				out[operation.Target.Name+"/"+indexedRole("value", index)] = meta.Span
			}
		}
	}
	return out
}

func expressionIsConcat(operation equation.Equation) bool {
	for _, operand := range operation.Operands {
		if operand.Role == "kind" && string(operand.Term.Encoding) == strconv.Itoa(int(wir.OpConcat)) {
			return true
		}
	}
	return false
}

// callSpans binds source call anchors to their apply operations.  The apply
// occurrence is the equation point that proves call-contract violations; WIR
// remains the sole source authority for its position.
func callSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	calls := make([]wir.Instruction, 0)
	literalMembers := make(map[uint32]map[string]wir.Span)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandTemp {
			members := make(map[string]wir.Span)
			for _, entry := range body.TableEntries(instruction.TableEntries) {
				if entry.ValueSpan.Valid() {
					members[segment.FormatSegments(entry.Suffix.Segments)] = entry.ValueSpan
				}
			}
			if len(members) != 0 {
				literalMembers[instruction.Dst.Ref] = members
			}
		}
		if instruction.Op == wir.OpCall {
			calls = append(calls, instruction)
		}
	}
	out := make(map[string]wir.Span, len(calls))
	callIndex := 0
	for _, item := range artifact.Equations {
		if item.Occurrence.Kind != "apply" || callIndex >= len(calls) {
			continue
		}
		call := calls[callIndex]
		span := call.CallSpan
		if !span.Valid() {
			span = call.CalleeSpan
		}
		if span.Valid() {
			out[item.Target.Name+"/call"] = span
		}
		if call.CalleeSpan.Valid() {
			out[item.Target.Name+"/callee"] = call.CalleeSpan
		}
		for index, argument := range body.CallArgumentMeta(call.CallArgs) {
			if argument.Span.Valid() {
				out[item.Target.Name+"/"+indexedRole("argument", index)] = argument.Span
			}
		}
		for index, argument := range body.Operands(call.List) {
			if argument.Kind != wir.OperandTemp {
				continue
			}
			for suffix, span := range literalMembers[argument.Ref] {
				out[item.Target.Name+"/"+indexedRole("argument", index)+suffix] = span
			}
		}
		callIndex++
	}
	return out
}

// branchSpans retains the whole-condition anchor for a branch-owned fact.
func branchSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	branches := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		if instruction := body.Instr(index); instruction.Op == wir.OpBranch {
			branches = append(branches, instruction)
		}
	}
	out := make(map[string]wir.Span, len(branches))
	branchIndex := 0
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" || branchIndex >= len(branches) {
			continue
		}
		if span := branches[branchIndex].ExprSpan; span.Valid() {
			out[operation.Target.Name] = span
		}
		branchIndex++
	}
	return out
}

// claimSpans retains the source anchors needed to render claim failures after
// equation evaluation. Equation facts name their owning operation, while WIR
// remains the authority for source coordinates.
func claimSpans(body *wir.Body, artifact equation.Artifact) (map[string]wir.Span, map[string]wir.Span) {
	if body == nil {
		return nil, nil
	}
	claims := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpClaim {
			claims = append(claims, instruction)
		}
	}
	spans := make(map[string]wir.Span, len(claims))
	targets := make(map[string]wir.Span, len(claims))
	claimIndex := 0
	for _, item := range artifact.Equations {
		if item.Occurrence.Kind != "claim" || claimIndex >= len(claims) {
			continue
		}
		claim := claims[claimIndex]
		span := claim.TargetSpan
		if (claim.Claim == wir.ClaimAnnotation || claim.Claim == wir.ClaimCast) && claim.ExprSpan.Valid() {
			span = claim.ExprSpan
		}
		if !span.Valid() {
			span = claim.ExprSpan
		}
		if !span.Valid() {
			span = claim.CallSpan
		}
		if span.Valid() {
			spans[item.Target.Name] = span
		}
		if claim.DeclaredSpan.Valid() {
			targets[item.Target.Name] = claim.DeclaredSpan
		} else if claim.TargetSpan.Valid() {
			targets[item.Target.Name] = claim.TargetSpan
		}
		claimIndex++
	}
	return spans, targets
}

// CompileBody is retained for consumers that only need the equation artifact.
// Check uses Compile so it can select the acyclic or cyclic execution path.
func CompileBody(source string) (equation.Artifact, error) {
	compilation, err := Compile(source)
	if err != nil {
		return equation.Artifact{}, err
	}
	return compilation.Artifact, nil
}

type operation struct {
	instruction     wir.Instruction
	target          equation.Coordinate
	family          string
	allocationSite  string
	allocationEntry *wir.TableEntry
	callResults     bool
	callApply       equation.Coordinate
	external        equation.Term
}

func compileWIR(source string, body *wir.Body, graph cfg.Graph, snapshots map[cfg.Point]cfg.Point) (equation.Artifact, error) {
	return compileWIRForBody(bodyID(source), body, graph, snapshots)
}

func compileWIRForBody(bodyID equation.BodyID, body *wir.Body, graph cfg.Graph, snapshots map[cfg.Point]cfg.Point) (equation.Artifact, error) {
	if body == nil || graph == nil {
		return equation.Artifact{}, fmt.Errorf("front: nil WIR body")
	}
	if !bodyID.Valid() {
		return equation.Artifact{}, fmt.Errorf("front: invalid lexical body identity")
	}
	entry := equation.EntryParameter{Body: bodyID, Name: entryName}
	loopBindings, err := genericForBindings(body, graph)
	if err != nil {
		return equation.Artifact{}, err
	}
	loopBindingPoints := make(map[cfg.Point]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingPoints[binding.point] = true
		}
	}
	for point := range numericForBindingPoints(body) {
		loopBindingPoints[point] = true
	}
	operations := make([]operation, 0, body.Len())
	entries := 0
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpEntry, wir.OpStaticMemberWrite, wir.OpBranch, wir.OpClaim, wir.OpSelect, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpLogical:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
			if instruction.Op == wir.OpEntry {
				entries++
			}
		case wir.OpDynamicIndexRead:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, family: "dynamic-index-read"})
		case wir.OpAssign:
			if instruction.A.Kind == wir.OperandNone {
				if loopBindingPoints[instruction.Point] {
					continue
				}
				return equation.Artifact{}, fmt.Errorf("front: assignment at point %d has no value source", instruction.Point)
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpIterate:
			if instruction.Iter == wir.IterGeneric && len(loopBindings[instruction.Point]) == 0 {
				return equation.Artifact{}, fmt.Errorf("front: generic-for at point %d has no bound variables", instruction.Point)
			}
			if instruction.Iter != wir.IterGeneric && instruction.Iter != wir.IterNumeric {
				return equation.Artifact{}, fmt.Errorf("front: iterate at point %d has unknown kind %d", instruction.Point, instruction.Iter)
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpDynamicIndexWrite:
			// A dynamic store has two inseparable semantic occurrences: the
			// mutation itself and the invalidation of every path below the
			// dynamically addressed container.  Keep both occurrences or fail
			// the whole body; emitting only either half would be unsound.
			operations = append(operations,
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, family: "path-invalidation"},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 1)}, family: "index-mutation"},
			)
		case wir.OpMakeTable, wir.OpClosure:
			site := operationName(len(operations))
			operations = append(operations,
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: site}, family: "allocation-template", allocationSite: site},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 1)}, family: "object-materialization", allocationSite: site},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 2)}, family: "allocation-write"},
			)
			// Constructors publish a completed value just as assignments do. The
			// allocation pair records topology; these writes close value chains for
			// later reads and returns.
			if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandPath {
				for _, entry := range body.TableEntries(instruction.TableEntries) {
					entry := entry
					operations = append(operations, operation{
						instruction:     instruction,
						target:          equation.Coordinate{Body: bodyID, Name: operationName(len(operations))},
						family:          "allocation-entry-write",
						allocationEntry: &entry,
					})
				}
			}
		case wir.OpCall:
			// Calls exclusively own their application/result pair.  An external
			// provider contributes one sealed boundary factor between those two
			// occurrences; it never manufactures or owns result slots.
			apply := equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}
			operations = append(operations, operation{instruction: instruction, target: apply})
			provider, external := externalProvider(body, instruction)
			if external {
				operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, callApply: apply, external: provider})
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, callResults: true, callApply: apply, external: provider})
		case wir.OpReturn:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpExit, wir.OpNoop:
			// These WIR operations carry no transfer occurrence. They are CFG
			// structure, so retaining them as equations would invent semantics.
		default:
			return equation.Artifact{}, fmt.Errorf("%w: %d at instruction %d", ErrUnsupportedInstruction, instruction.Op, index)
		}
	}
	if entries != 1 {
		return equation.Artifact{}, fmt.Errorf("front: WIR body has %d entry operations, want one", entries)
	}
	drafts := make([]equation.Draft, 0, len(operations))
	hasDynamicIndexRead := false
	for _, operation := range operations {
		hasDynamicIndexRead = hasDynamicIndexRead || operation.instruction.Op == wir.OpDynamicIndexRead
	}
	loopBindingRoots := make(map[string]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingRoots[string(binding.term.Encoding)] = true
		}
	}
	branchTargets := make(map[cfg.Point]branchGuardTarget)
	for _, operation := range operations {
		if operation.instruction.Op == wir.OpBranch {
			if _, duplicate := branchTargets[operation.instruction.Point]; duplicate {
				return equation.Artifact{}, fmt.Errorf("front: multiple branches at CFG point %d", operation.instruction.Point)
			}
			check := body.Check(operation.instruction.Check)
			branchTargets[operation.instruction.Point] = branchGuardTarget{
				target:              operation.target,
				literalDiscriminant: literalLoopDiscriminant(check, loopBindingRoots),
			}
		}
	}
	guardReachability := newReachabilityCache(graph)
	suspensionLives := suspensionLiveAllocations(body, graph, guardReachability)
	for index, operation := range operations {
		instruction := operation.instruction
		draft := equation.Draft{Target: operation.target, Entry: entry}
		if index != 0 {
			draft.Dependencies = []equation.Coordinate{operations[index-1].target}
		}
		switch {
		case operation.family == "allocation-template" || operation.family == "object-materialization":
			terms, err := allocationOperands(body, instruction, operation.allocationSite)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: %s %s: %w", operation.family, operation.target.Name, err)
			}
			draft.Occurrence = occurrence(operation.family)
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			if operation.family == "allocation-template" {
				draft.Operands = terms.template
			} else {
				draft.Operands = terms.materialization
			}
		case operation.family == "allocation-write":
			operands, err := allocationWriteOperands(body, instruction, operation, operations)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: allocation write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case operation.family == "allocation-entry-write":
			operands, err := allocationEntryWriteOperands(body, instruction, operation, operations)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: allocation entry write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case operation.family == "path-invalidation" || operation.family == "index-mutation":
			container := equation.ClosedTerm([]byte("scalar/top"))
			if instruction.Dst.Kind != wir.OperandNone {
				var err error
				container, err = pathStoreTerm(body, instruction.Dst)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: %w", operation.target.Name, err)
				}
			}
			key, err := dynamicStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: key: %w", operation.target.Name, err)
			}
			value, err := dynamicStoreTerm(body, instruction.B)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: value: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence(operation.family)
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			if operation.family == "path-invalidation" {
				draft.Operands = []equation.Operand{
					{Role: "container", Term: container},
					{Role: "key", Term: key},
					{Role: "suffix", Term: suffixTerm(body, instruction.DynamicSuffix)},
				}
			} else {
				draft.Operands = []equation.Operand{
					{Role: "container", Term: container},
					{Role: "key", Term: key},
					{Role: "suffix", Term: suffixTerm(body, instruction.DynamicSuffix)},
					{Role: "value", Term: value},
				}
				if instruction.DynamicTargetDisplay != "" {
					draft.Operands = append(draft.Operands, equation.Operand{Role: "display", Term: equation.ClosedTerm([]byte(instruction.DynamicTargetDisplay))})
				}
				if instruction.DynamicValueDisplay != "" {
					draft.Operands = append(draft.Operands, equation.Operand{Role: "source-display", Term: equation.ClosedTerm([]byte(instruction.DynamicValueDisplay))})
				}
				if subject, display, ok := frozenTableSubject(body, instruction.Dst, true); ok {
					draft.Operands = append(draft.Operands,
						equation.Operand{Role: "freeze-subject", Term: subject},
						equation.Operand{Role: "freeze-display", Term: equation.ClosedTerm([]byte(display))},
					)
				}
			}
		case instruction.Op == wir.OpEntry:
			draft.Occurrence = occurrence("entry")
			draft.Operands = append([]equation.Operand{{Role: "entry", Term: equation.EntryTerm(entry)}}, entryDeclaredOperands(body)...)
		case instruction.Op == wir.OpAssign:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			display := string(target.Encoding)
			if instruction.Dst.Kind == wir.OperandPath {
				_, display, err = pathTerm(body, instruction.Dst)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
				}
			}
			value, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			readBefore, err := readBeforeTerm(operation, operations, snapshots)
			if instruction.Dst.Kind == wir.OperandTemp {
				// Temporary assignments are expression-internal steps, not Lua
				// statement targets. They must read the immediately preceding
				// operation rather than demand a statement snapshot they cannot own.
				readBefore, err = precedingReadBoundary(operation, operations)
			}
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			absence := assignmentAbsencePolicy(body, instruction.A)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
				{Role: "read-before", Term: readBefore},
				{Role: "absence", Term: equation.ClosedTerm([]byte(absence))},
			}
			if instruction.A.Kind == wir.OperandPath {
				source := body.Path(wir.PathRef(instruction.A.Ref))
				if len(source.Segments) != 0 && source.String() != "" {
					sourceDisplay := source.String()
					last := source.Segments[len(source.Segments)-1]
					if last.Kind == segment.SegmentIndexString {
						suffix := "[" + last.Name + "]"
						sourceDisplay = strings.TrimSuffix(sourceDisplay, suffix) + "[" + strconv.Quote(last.Name) + "]"
						draft.Operands = append(draft.Operands, equation.Operand{Role: "source-display", Term: equation.ClosedTerm([]byte(sourceDisplay))})
					}
				}
			}
		case instruction.Op == wir.OpStaticMemberWrite:
			target, display, err := memberPathTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: static member write %s: %w", operation.target.Name, err)
			}
			value, err := pathStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: static member write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("path-replacement")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
			}
			// WIR records a signature on typed dotted function definitions. Carry
			// only that resolved, closed type as the member's declaration contract;
			// ordinary static writes have no such authority.
			if instruction.Type != 0 {
				if declared, ok := shapefact.EncodeTarget(body.Type(instruction.Type)); ok {
					draft.Operands = append(draft.Operands, equation.Operand{Role: "declared-type", Term: equation.ClosedTerm(declared)})
				}
			}
			// table.freeze is deliberately shallow. A direct member write can
			// mutate the frozen table itself; deeper writes affect a child table.
			if subject, rootDisplay, ok := frozenTableSubject(body, instruction.Dst, false); ok {
				draft.Operands = append(draft.Operands,
					equation.Operand{Role: "freeze-subject", Term: subject},
					equation.Operand{Role: "freeze-display", Term: equation.ClosedTerm([]byte(rootDisplay))},
				)
			}
		case instruction.Op == wir.OpDynamicIndexRead:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: %w", operation.target.Name, err)
			}
			container, err := pathStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: container: %w", operation.target.Name, err)
			}
			key, err := pathStoreTerm(body, instruction.B)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: key: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("dynamic-index-read")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "container", Term: container},
				{Role: "key", Term: key},
			}
		case instruction.Op == wir.OpClaim:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: target: %w", operation.target.Name, err)
			}
			value, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: value: %w", operation.target.Name, err)
			}
			claimType, err := claimTypeTerm(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("claim")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "value", Term: value},
				{Role: "kind", Term: equation.ClosedTerm([]byte("claim-kind/" + strconv.Itoa(int(instruction.Claim))))},
				{Role: "type", Term: claimType},
			}
			if target, ok := shapefact.EncodeTarget(body.Type(instruction.Type)); ok {
				draft.Operands = append(draft.Operands, equation.Operand{Role: "shape-target", Term: equation.ClosedTerm(target)})
			}
			if instruction.Dst.Kind == wir.OperandPath {
				display := body.Path(wir.PathRef(instruction.Dst.Ref)).String()
				if display == "" {
					return equation.Artifact{}, fmt.Errorf("front: claim %s: empty path target", operation.target.Name)
				}
				draft.Operands = append(draft.Operands, equation.Operand{Role: "display", Term: equation.ClosedTerm([]byte(display))})
			}
			if instruction.ClaimSourceDisplay != "" {
				draft.Operands = append(draft.Operands, equation.Operand{Role: "source-display", Term: equation.ClosedTerm([]byte(instruction.ClaimSourceDisplay))})
				if instruction.ClaimSourceMethodSelector {
					draft.Operands = append(draft.Operands, equation.Operand{Role: "source-method-selector", Term: equation.ClosedTerm([]byte("scalar/bool/true"))})
				}
			} else if instruction.A.Kind == wir.OperandPath {
				sourceDisplay := diagnosticPathDisplay(body.Path(wir.PathRef(instruction.A.Ref)))
				if sourceDisplay == "" {
					return equation.Artifact{}, fmt.Errorf("front: claim %s: empty path source", operation.target.Name)
				}
				draft.Operands = append(draft.Operands, equation.Operand{Role: "source-display", Term: equation.ClosedTerm([]byte(sourceDisplay))})
			}
		case instruction.Op == wir.OpBinOp, instruction.Op == wir.OpUnOp, instruction.Op == wir.OpConcat, instruction.Op == wir.OpLogical:
			operands, err := expressionOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: expression %s: %w", operation.target.Name, err)
			}
			draft.Occurrence, draft.Guards, draft.Operands = occurrence("expression"), guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets), operands
		case instruction.Op == wir.OpBranch:
			draft.Occurrence = occurrence("branch-relations")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			operands, err := branchOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: branch %s: %w", operation.target.Name, err)
			}
			if hasDynamicIndexRead {
				operands = append(operands, equation.Operand{Role: "index-presence-consumer", Term: equation.ClosedTerm([]byte("scalar/bool/true"))})
			}
			draft.Operands = operands
		case instruction.Op == wir.OpCall:
			if operation.external.Encoding != nil && !operation.callResults {
				operands, err := externalCallOperands(body, instruction, operation.callApply, operation.external)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: external call %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("external-call")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			} else if !operation.callResults {
				operands, err := applyOperands(body, instruction)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call %s: %w", operation.target.Name, err)
				}
				for liveIndex, term := range suspensionLives[instruction.Point] {
					operands = append(operands, equation.Operand{
						Role: "suspension-live-" + fmt.Sprintf("%08d", liveIndex),
						Term: term,
					})
				}
				draft.Occurrence = occurrence("apply")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			} else {
				operands, err := callResultOperands(body, instruction, operation.callApply, operation.external)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call results %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("call-results")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			}
		case instruction.Op == wir.OpReturn:
			operands, err := publicationOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: return %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("publication")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case instruction.Op == wir.OpIterate:
			var operands []equation.Operand
			var err error
			if instruction.Iter == wir.IterNumeric {
				operands, err = numericForOperands(body, instruction)
			} else {
				operands, err = genericForOperands(body, instruction, loopBindings[instruction.Point])
			}
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: generic-for %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("generic-for")
			draft.Guards = loopHeaderGuards(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case instruction.Op == wir.OpSelect:
			result, err := selectResultTerm(instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: channel select %s: %w", operation.target.Name, err)
			}
			operands := make([]equation.Operand, 0, 2+3*instruction.List.Len)
			operands = append(operands,
				equation.Operand{Role: "result", Term: result},
				equation.Operand{Role: "default", Term: equation.ClosedTerm([]byte("select/default/" + strconv.FormatBool(instruction.SelectDefault)))},
			)
			for caseIndex, candidate := range body.Operands(instruction.List) {
				channel, err := selectCaseTerm(body, candidate)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: channel select %s case %d: %w", operation.target.Name, caseIndex, err)
				}
				caseName := fmt.Sprintf("%08d", caseIndex)
				operands = append(operands,
					equation.Operand{Role: "case-" + caseName, Term: channel},
					equation.Operand{Role: "case-display-" + caseName, Term: equation.ClosedTerm([]byte(selectCaseDisplay(body, candidate)))},
				)
				if payload, ok := selectCasePayloadTerm(body, candidate); ok {
					operands = append(operands, equation.Operand{Role: "payload-type-" + caseName, Term: payload})
				}
			}
			draft.Occurrence = occurrence("channel-select")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		default:
			return equation.Artifact{}, fmt.Errorf("%w: %d", ErrUnsupportedInstruction, instruction.Op)
		}
		drafts = append(drafts, draft)
	}
	compiler, err := equation.Skeleton().With("entry", equation.BindExistingKernel(entryKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure entry compiler: %w", err)
	}
	compiler, err = compiler.With("environment-write", equation.BindExistingKernel(writeKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure assignment compiler: %w", err)
	}
	compiler, err = compiler.With("allocation-template", equation.BindExistingKernel(allocationTemplateKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure allocation template compiler: %w", err)
	}
	compiler, err = compiler.With("object-materialization", equation.BindExistingKernel(objectMaterializationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure object materialization compiler: %w", err)
	}
	compiler, err = compiler.With("path-replacement", equation.BindExistingKernel(pathReplacementKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure path replacement compiler: %w", err)
	}
	compiler, err = compiler.With("dynamic-index-read", equation.BindExistingKernel(dynamicIndexReadKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure dynamic index read compiler: %w", err)
	}
	compiler, err = compiler.With("path-invalidation", equation.BindExistingKernel(pathInvalidationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure path invalidation compiler: %w", err)
	}
	compiler, err = compiler.With("index-mutation", equation.BindExistingKernel(indexMutationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure index mutation compiler: %w", err)
	}
	compiler, err = compiler.With("branch-relations", equation.BindExistingKernel(branchKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure branch compiler: %w", err)
	}
	compiler, err = compiler.With("apply", equation.BindExistingKernel(applyKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure apply compiler: %w", err)
	}
	compiler, err = compiler.With("call-results", equation.BindExistingKernel(resultsKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure call-results compiler: %w", err)
	}
	compiler, err = compiler.With("external-call", equation.BindExistingKernel(externalCallKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure external call compiler: %w", err)
	}
	compiler, err = compiler.With("generic-for", equation.BindExistingKernel(genericForKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure generic-for compiler: %w", err)
	}
	compiler, err = compiler.With("channel-select", equation.BindExistingKernel(selectKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure channel-select compiler: %w", err)
	}
	compiler, err = compiler.With("publication", equation.BindExistingKernel(publicationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure publication compiler: %w", err)
	}
	compiler, err = compiler.With("claim", equation.BindExistingKernel(claimKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure claim compiler: %w", err)
	}
	compiler, err = compiler.With("expression", equation.BindExistingKernel(expressionKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure expression compiler: %w", err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: compile equations: %w", err)
	}
	return artifact, nil
}

// assignmentSnapshotStarts maps each ordinary/local assignment point to the
// first point in its source statement. Lua evaluates every right-hand side
// before writing any left-hand target, so all targets in one statement must
// resolve path operands at that common pre-write boundary.
func assignmentSnapshotStarts(stmts []ast.Stmt, built *cfgbuild.Result) map[cfg.Point]cfg.Point {
	starts := make(map[cfg.Point]cfg.Point)
	if built == nil {
		return starts
	}
	var visit func([]ast.Stmt)
	mark := func(stmt ast.Stmt, targets int) {
		points := built.StmtPoints.PointsFor(stmt)
		if targets == 0 || len(points) < targets {
			return
		}
		assignmentPoints := points[len(points)-targets:]
		for _, point := range assignmentPoints {
			starts[point] = assignmentPoints[0]
		}
	}
	visit = func(items []ast.Stmt) {
		for _, stmt := range items {
			switch node := stmt.(type) {
			case *ast.LocalAssignStmt:
				mark(node, len(node.Names))
			case *ast.AssignStmt:
				mark(node, len(node.Lhs))
			case *ast.IfStmt:
				visit(node.Then)
				visit(node.Else)
			case *ast.DoBlockStmt:
				visit(node.Stmts)
			case *ast.WhileStmt:
				visit(node.Stmts)
			case *ast.RepeatStmt:
				visit(node.Stmts)
			case *ast.NumberForStmt:
				visit(node.Stmts)
			case *ast.GenericForStmt:
				visit(node.Stmts)
			}
		}
	}
	visit(stmts)
	return starts
}

func readBeforeTerm(current operation, operations []operation, snapshots map[cfg.Point]cfg.Point) (equation.Term, error) {
	start, found := snapshots[current.instruction.Point]
	if !found {
		// Nested WIR bodies are already source-normalized but intentionally do
		// not retain an AST statement-point sidecar. Their assignment point is
		// therefore the exact snapshot boundary; its predecessor remains the
		// same admitted operation-order seam used by root-body assignments.
		for index, candidate := range operations {
			if candidate.target == current.target {
				if index == 0 {
					return equation.Term{}, fmt.Errorf("assignment snapshot has no predecessor")
				}
				return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
			}
		}
		return equation.Term{}, fmt.Errorf("assignment operation %s is absent", current.target.Name)
	}
	for index, candidate := range operations {
		if candidate.instruction.Point != start {
			continue
		}
		if index == 0 {
			return equation.Term{}, fmt.Errorf("assignment snapshot has no predecessor")
		}
		return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
	}
	return equation.Term{}, fmt.Errorf("assignment snapshot boundary %d has no operation", start)
}

func implicitGlobalPath(body *wir.Body, operand wir.Operand) bool {
	if body == nil || operand.Kind != wir.OperandPath {
		return false
	}
	return body.IsImplicitGlobalSymbol(body.Path(wir.PathRef(operand.Ref)).Symbol)
}

// assignmentAbsencePolicy makes the source-level distinction between an
// unread implicit global and a path whose producing heap write is outside the
// current scalar model. Lua reads the former as nil; the latter is an unknown
// value and must not be turned into false or rejected as an incomplete fact.
func assignmentAbsencePolicy(body *wir.Body, operand wir.Operand) string {
	if operand.Kind != wir.OperandPath {
		return "front/absence/error"
	}
	if implicitGlobalPath(body, operand) {
		return "front/absence/nil"
	}
	return "front/absence/top"
}

func bodyID(source string) equation.BodyID {
	return equation.BodyID(sha256.Sum256(append([]byte("front/lua-body/v1\x00"), []byte(source)...)))
}

func lexicalBodyID(root equation.BodyID, lexicalPath []uint32) equation.BodyID {
	bytes := make([]byte, 0, len("front/lua-lexical-body/v1\x00")+len(root)+4*len(lexicalPath))
	bytes = append(bytes, []byte("front/lua-lexical-body/v1\x00")...)
	bytes = append(bytes, root[:]...)
	var encoded [4]byte
	for _, ordinal := range lexicalPath {
		binary.BigEndian.PutUint32(encoded[:], ordinal)
		bytes = append(bytes, encoded[:]...)
	}
	return equation.BodyID(sha256.Sum256(bytes))
}

func occurrence(kind string) equation.Occurrence {
	contract, _ := ContractID(kind)
	return equation.Occurrence{Kind: kind, ContractID: contract}
}

func operationName(index int) string { return fmt.Sprintf("op-%08d", index) }

// ContractID returns the contract identity admitted by this front for kind.
// The engine uses this exact content identity when registering its canonical
// kernels; unknown kinds deliberately have no binding.
func ContractID(kind string) (equation.ContentID, bool) {
	switch kind {
	case "entry", "environment-write", "allocation-template", "object-materialization", "path-replacement", "dynamic-index-read", "path-invalidation", "index-mutation", "branch-relations", "apply", "call-results", "external-call", "generic-for", "channel-select", "publication", "claim", "expression":
		return equation.ContentID(sha256.Sum256([]byte("front/contract/v1/" + kind))), true
	default:
		return equation.ContentID{}, false
	}
}

// KernelID returns the canonical kernel identity admitted by this walking
// front for kind. It has no fallback for unsupported equation families.
func KernelID(kind string) (string, bool) {
	switch kind {
	case "entry":
		return entryKernel, true
	case "environment-write":
		return writeKernel, true
	case "allocation-template":
		return allocationTemplateKernel, true
	case "object-materialization":
		return objectMaterializationKernel, true
	case "path-replacement":
		return pathReplacementKernel, true
	case "dynamic-index-read":
		return dynamicIndexReadKernel, true
	case "path-invalidation":
		return pathInvalidationKernel, true
	case "index-mutation":
		return indexMutationKernel, true
	case "branch-relations":
		return branchKernel, true
	case "apply":
		return applyKernel, true
	case "call-results":
		return resultsKernel, true
	case "external-call":
		return externalCallKernel, true
	case "generic-for":
		return genericForKernel, true
	case "channel-select":
		return selectKernel, true
	case "publication":
		return publicationKernel, true
	case "claim":
		return claimKernel, true
	case "expression":
		return expressionKernel, true
	default:
		return "", false
	}
}

type loopBinding struct {
	point   cfg.Point
	term    equation.Term
	display string
}

func genericForBindings(body *wir.Body, graph cfg.Graph) (map[cfg.Point][]loopBinding, error) {
	bindings := make(map[cfg.Point][]loopBinding)
	for index := 0; index < body.Len(); index++ {
		header := body.Instr(index)
		if header.Op != wir.OpIterate || header.Iter != wir.IterGeneric {
			continue
		}
		var next cfg.Point
		found := false
		for _, successor := range graph.Successors(header.Point) {
			condition, branchEdge := graph.EdgeCond(header.Point, successor)
			if branchEdge && condition {
				if found {
					return nil, fmt.Errorf("front: generic-for at point %d has multiple true successors", header.Point)
				}
				next, found = successor, true
			}
		}
		if !found {
			return nil, fmt.Errorf("front: generic-for at point %d has no true successor", header.Point)
		}
		seen := map[cfg.Point]bool{}
		for {
			if seen[next] {
				return nil, fmt.Errorf("front: generic-for at point %d has cyclic binding path", header.Point)
			}
			seen[next] = true
			instructions := body.PointInstructions(next)
			if len(instructions) != 1 || instructions[0].Op != wir.OpAssign || instructions[0].A.Kind != wir.OperandNone {
				break
			}
			term, display, err := pathTerm(body, instructions[0].Dst)
			if err != nil {
				return nil, fmt.Errorf("front: generic-for at point %d: %w", header.Point, err)
			}
			bindings[header.Point] = append(bindings[header.Point], loopBinding{point: next, term: term, display: display})
			successors := graph.Successors(next)
			if len(successors) != 1 {
				break
			}
			next = successors[0]
		}
		if len(bindings[header.Point]) == 0 {
			return nil, fmt.Errorf("front: generic-for at point %d has no binding assignments", header.Point)
		}
	}
	return bindings, nil
}

func numericForBindingPoints(body *wir.Body) map[cfg.Point]bool {
	points := make(map[cfg.Point]bool)
	if body == nil {
		return points
	}
	bindings := make(map[wir.Operand]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric {
			continue
		}
		for _, result := range body.Operands(instruction.Results) {
			bindings[result] = true
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpAssign && instruction.A.Kind == wir.OperandNone && bindings[instruction.Dst] {
			points[instruction.Point] = true
		}
	}
	return points
}

func genericForOperands(body *wir.Body, instruction wir.Instruction, bindings []loopBinding) ([]equation.Operand, error) {
	if instruction.Iter != wir.IterGeneric {
		return nil, fmt.Errorf("iterator kind %d is not generic", instruction.Iter)
	}
	sources := body.Operands(instruction.List)
	if len(sources) == 0 {
		return nil, fmt.Errorf("iterator has no source values")
	}
	roles := []string{"iterator", "state", "control"}
	operands := make([]equation.Operand, 0, len(roles)+2*len(bindings))
	for index, role := range roles {
		term := equation.ClosedTerm([]byte("scalar/nil"))
		// An open iterator tail carries no closed state/control coordinates. It
		// is not nil: retain Top so the loop cannot manufacture a finite tuple.
		if instruction.ListSpread {
			term = equation.ClosedTerm([]byte("scalar/top"))
		}
		if index < len(sources) {
			resolved, err := scalarTerm(body, sources[index])
			if err != nil {
				return nil, fmt.Errorf("%s source: %w", role, err)
			}
			term = resolved
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
	}
	for index, binding := range bindings {
		name := fmt.Sprintf("%08d", index)
		operands = append(operands,
			equation.Operand{Role: "result-" + name, Term: binding.term},
			equation.Operand{Role: "display-" + name, Term: equation.ClosedTerm([]byte(binding.display))},
		)
	}
	return operands, nil
}

func numericForOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	if instruction.Iter != wir.IterNumeric {
		return nil, fmt.Errorf("iterator kind %d is not numeric", instruction.Iter)
	}
	sources := body.Operands(instruction.List)
	results := body.Operands(instruction.Results)
	if len(sources) != 3 || len(results) != 1 {
		return nil, fmt.Errorf("numeric-for has %d bounds and %d bindings, want 3 and 1", len(sources), len(results))
	}
	operands := make([]equation.Operand, 0, 5)
	for index, role := range []string{"iterator", "state", "control"} {
		term, err := scalarTerm(body, sources[index])
		if err != nil {
			return nil, fmt.Errorf("%s bound: %w", role, err)
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
	}
	result, display, err := pathTerm(body, results[0])
	if err != nil {
		return nil, fmt.Errorf("numeric binding: %w", err)
	}
	return append(operands,
		equation.Operand{Role: "result-00000000", Term: result},
		equation.Operand{Role: "display-00000000", Term: equation.ClosedTerm([]byte(display))},
	), nil
}

func selectResultTerm(operand wir.Operand) (equation.Term, error) {
	if operand.Kind != wir.OperandTemp {
		return equation.Term{}, fmt.Errorf("result is operand kind %d, want temporary", operand.Kind)
	}
	return equation.ClosedTerm([]byte("temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
}

func selectCaseTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, fmt.Errorf("case is operand kind %d, want path", operand.Kind)
	}
	return scalarTerm(body, operand)
}

// entryDeclaredOperands retains the closed type evidence available at a
// lexical boundary.  The entry kernel treats a concrete caller seed as the
// value authority, while these paired operands provide only guarded abstract
// type and exact Channel identity evidence when no concrete identity exists.
func entryDeclaredOperands(body *wir.Body) []equation.Operand {
	if body == nil {
		return nil
	}
	type declared struct {
		term string
		typ  typ.Type
	}
	byTerm := make(map[string]typ.Type)
	for _, root := range body.RootTypes() {
		if root.Path.IsEmpty() || root.Path.Key() == "" || body.Type(root.Type) == nil {
			continue
		}
		byTerm["path/"+string(root.Path.Key())] = body.Type(root.Type)
	}
	for _, capture := range body.Boundary().Captures {
		if capture.Symbol == 0 || capture.Type == 0 || body.Type(capture.Type) == nil {
			continue
		}
		byTerm[fmt.Sprintf("path/sym%d", capture.Symbol)] = body.Type(capture.Type)
	}
	items := make([]declared, 0, len(byTerm))
	for term, declaredType := range byTerm {
		items = append(items, declared{term: term, typ: declaredType})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].term < items[j].term })
	operands := make([]equation.Operand, 0, len(items)*2)
	for index, item := range items {
		encoded, ok := shapefact.EncodeTarget(item.typ)
		if !ok {
			continue
		}
		name := fmt.Sprintf("%08d", index)
		operands = append(operands,
			equation.Operand{Role: "declared-root-" + name, Term: equation.ClosedTerm([]byte(item.term))},
			equation.Operand{Role: "declared-type-" + name, Term: equation.ClosedTerm(encoded)},
		)
	}
	return operands
}

func selectCaseDisplay(body *wir.Body, operand wir.Operand) string {
	if body == nil || operand.Kind != wir.OperandPath {
		return ""
	}
	return body.Path(wir.PathRef(operand.Ref)).String()
}

func selectCasePayloadTerm(body *wir.Body, operand wir.Operand) (equation.Term, bool) {
	if body == nil || operand.Kind != wir.OperandPath {
		return equation.Term{}, false
	}
	p := body.Path(wir.PathRef(operand.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return equation.Term{}, false
	}
	var root typ.Type
	for _, candidate := range body.RootTypes() {
		if candidate.Path.Symbol == p.Symbol && candidate.Path.Segments == nil {
			root = body.Type(candidate.Type)
			break
		}
	}
	if root == nil {
		return equation.Term{}, false
	}
	channel := root
	if len(p.Segments) != 0 {
		var ok bool
		channel, ok = luatypeprojection.ApplySegments(root, p.Segments)
		if !ok {
			return equation.Term{}, false
		}
	}
	payload, ok := ambient.ChannelPayloadType(channel)
	if !ok || payload == nil {
		return equation.Term{}, false
	}
	encoded, ok := shapefact.EncodeTarget(payload)
	if !ok {
		return equation.Term{}, false
	}
	return equation.ClosedTerm(encoded), true
}

func pathTerm(body *wir.Body, operand wir.Operand) (equation.Term, string, error) {
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, "", fmt.Errorf("assignment target is operand kind %d, want path", operand.Kind)
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if path.IsEmpty() || path.Key() == "" || path.String() == "" {
		return equation.Term{}, "", fmt.Errorf("empty assignment target path")
	}
	return equation.ClosedTerm([]byte("path/" + path.Key())), path.String(), nil
}

// memberPathTerm rejects a root target: a static member write is never a
// disguised environment write.  Nil remains a valid value operand elsewhere;
// an absent target is always rejected.
func memberPathTerm(body *wir.Body, operand wir.Operand) (equation.Term, string, error) {
	term, display, err := pathTerm(body, operand)
	if err != nil {
		return equation.Term{}, "", err
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if len(path.Segments) == 0 {
		return equation.Term{}, "", fmt.Errorf("static member target has no member path")
	}
	return term, display, nil
}

// frozenTableSubject returns the root table identity carried by a mutation.
// Static member writes are shallow: only root.field mutates root itself. A
// dynamic index always mutates its container, even when the suffix is unknown.
func frozenTableSubject(body *wir.Body, operand wir.Operand, dynamic bool) (equation.Term, string, bool) {
	if body == nil || operand.Kind != wir.OperandPath {
		return equation.Term{}, "", false
	}
	target := body.Path(wir.PathRef(operand.Ref))
	if target.IsEmpty() || target.Key() == "" || target.String() == "" {
		return equation.Term{}, "", false
	}
	if !dynamic && len(target.Segments) != 1 {
		return equation.Term{}, "", false
	}
	root := target.RootOnly()
	if root.IsEmpty() || root.Key() == "" || root.String() == "" {
		return equation.Term{}, "", false
	}
	return equation.ClosedTerm([]byte("path/" + root.Key())), root.String(), true
}

// pathStoreTerm preserves every operand shape this family can consume.  In
// particular, scalar/nil is a real Lua value, while OperandNone is absence and
// therefore an error. A temp uses the same body-local value namespace as every
// other consumer: temp zero is the first valid temporary, not a sentinel.
func pathStoreTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	return scalarTerm(body, operand)
}

// dynamicStoreTerm preserves the fail-closed meaning of an incomplete dynamic
// store.  WIR's absent operand is not Lua nil, so a mutation with an omitted
// key or value cannot contribute a precise heap fact; it must instead retain
// the existing Top boundary.  This keeps the paired invalidation/mutation
// transactions total without inventing a key or value.
func dynamicStoreTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	if operand.Kind == wir.OperandNone {
		return equation.ClosedTerm([]byte("scalar/top")), nil
	}
	return pathStoreTerm(body, operand)
}

func suffixTerm(body *wir.Body, suffix wir.SegmentRange) equation.Term {
	return equation.ClosedTerm([]byte("suffix/" + segment.FormatSegments(body.Segments(suffix))))
}

func expressionOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	result, display, err := pathTerm(body, instruction.Dst)
	if instruction.Dst.Kind != wir.OperandPath {
		result, err = scalarTerm(body, instruction.Dst)
		display = ""
	}
	if err != nil {
		return nil, fmt.Errorf("result: %w", err)
	}
	if instruction.Op != wir.OpConcat && instruction.Operator == wir.OperatorNone {
		return nil, fmt.Errorf("missing operator")
	}
	operands := []equation.Operand{{Role: "result", Term: result}, {Role: "kind", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Op))))}, {Role: "operator", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Operator))))}}
	if display != "" {
		operands = append(operands, equation.Operand{Role: "display", Term: equation.ClosedTerm([]byte(display))})
	}
	appendOperand := func(role string, value wir.Operand) error {
		if value.Kind == wir.OperandNone {
			// This is an unrepresentable source operand, not Lua nil. Keep the
			// expression transaction complete with Top so it cannot invent a
			// concrete value or decide a branch from missing syntax evidence.
			operands = append(operands, equation.Operand{Role: role, Term: equation.ClosedTerm([]byte("scalar/top"))})
			return nil
		}
		// Lua reads an undeclared global as nil.  Keep that semantic value in
		// the expression graph; the lexical front publishes the corresponding
		// unresolved-reference diagnostic separately.
		if implicitGlobalPath(body, value) {
			operands = append(operands, equation.Operand{Role: role, Term: equation.ClosedTerm([]byte("scalar/nil"))})
			return nil
		}
		term, err := scalarTerm(body, value)
		if err != nil {
			return fmt.Errorf("%s: %w", role, err)
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
		return nil
	}
	switch instruction.Op {
	case wir.OpBinOp, wir.OpLogical:
		if err := appendOperand("left", instruction.A); err != nil {
			return nil, err
		}
		if err := appendOperand("right", instruction.B); err != nil {
			return nil, err
		}
	case wir.OpUnOp:
		if err := appendOperand("value", instruction.A); err != nil {
			return nil, err
		}
	case wir.OpConcat:
		values := body.Operands(instruction.List)
		if len(values) < 2 {
			return nil, fmt.Errorf("concat has %d operands", len(values))
		}
		meta := body.ConcatOperandMeta(instruction.ConcatOperands)
		if len(meta) != 0 && len(meta) != len(values) {
			return nil, fmt.Errorf("concat has %d operand anchors for %d operands", len(meta), len(values))
		}
		for i, value := range values {
			if err := appendOperand(indexedRole("value", i), value); err != nil {
				return nil, err
			}
			if len(meta) != 0 && meta[i].Label != "" {
				operands = append(operands, equation.Operand{Role: indexedRole("value-display", i), Term: equation.ClosedTerm([]byte(meta[i].Label))})
			}
		}
	default:
		return nil, fmt.Errorf("not expression")
	}
	return operands, nil
}

func scalarTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	switch operand.Kind {
	case wir.OperandPath:
		path := body.Path(wir.PathRef(operand.Ref))
		if path.IsEmpty() || path.Key() == "" {
			return equation.Term{}, fmt.Errorf("empty path operand")
		}
		return equation.ClosedTerm([]byte("path/" + path.Key())), nil
	case wir.OperandConst:
		constant := body.Const(wir.ConstRef(operand.Ref))
		switch constant.Kind {
		case wir.ConstNil:
			return equation.ClosedTerm([]byte("scalar/nil")), nil
		case wir.ConstBool:
			return equation.ClosedTerm([]byte("scalar/bool/" + strconv.FormatBool(constant.Bool))), nil
		case wir.ConstNumber:
			return equation.ClosedTerm([]byte("scalar/number/" + constant.Number)), nil
		case wir.ConstString:
			return equation.ClosedTerm([]byte("scalar/string/" + strconv.Quote(constant.Str))), nil
		default:
			return equation.Term{}, fmt.Errorf("unknown constant kind %d", constant.Kind)
		}
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte("temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
	case wir.OperandVararg:
		return equation.ClosedTerm([]byte("vararg")), nil
	default:
		return equation.Term{}, fmt.Errorf("operand kind %d is outside the scalar slice", operand.Kind)
	}
}

// claimTypeTerm seals the only type information an OpClaim may carry.  A
// non-nil assertion has no type target; every type-bearing claim must resolve
// to an interned WIR type instead of falling back to source spelling.
func claimTypeTerm(body *wir.Body, instruction wir.Instruction) (equation.Term, error) {
	if instruction.Claim == wir.ClaimAssert {
		if instruction.Type != 0 {
			return equation.Term{}, fmt.Errorf("non-nil assertion has a type target")
		}
		return equation.ClosedTerm([]byte("claim-type/non-nil")), nil
	}
	if instruction.Claim != wir.ClaimCast && instruction.Claim != wir.ClaimAnnotation && instruction.Claim != wir.ClaimAssertsPredicate {
		return equation.Term{}, fmt.Errorf("unknown claim kind %d", instruction.Claim)
	}
	if instruction.Type == 0 || body.Type(instruction.Type) == nil || body.TypeDisplay(instruction.Type) == "" {
		return equation.Term{}, fmt.Errorf("type-bearing claim has no resolved target type")
	}
	display := instruction.ClaimTypeDisplay
	if display == "" {
		display = body.TypeDisplay(instruction.Type)
	}
	return equation.ClosedTerm([]byte("claim-type/" + strconv.Quote(display))), nil
}

// diagnosticPathDisplay derives a quoted string-index display from the
// already-bound path segments. It deliberately does not parse source text or
// infer a key: only a statically classified string segment receives quotes.
func diagnosticPathDisplay(value path.Path) string {
	display := value.String()
	if display == "" || len(value.Segments) == 0 {
		return display
	}
	last := value.Segments[len(value.Segments)-1]
	if last.Kind != segment.SegmentIndexString {
		return display
	}
	suffix := "[" + last.Name + "]"
	return strings.TrimSuffix(display, suffix) + "[" + strconv.Quote(last.Name) + "]"
}

// applyOperands preserves the complete source-side call shape. The kernel,
// rather than this front, owns dispatch and outcome semantics.
func applyOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	operands := make([]equation.Operand, 0, 12+int(instruction.List.Len)+int(instruction.CallTypeArgs.Len))
	if instruction.Call.Method != 0 {
		if instruction.Call.Callee.Kind != wir.OperandNone || instruction.Call.Receiver.Kind == wir.OperandNone {
			return nil, fmt.Errorf("malformed method call shape")
		}
		receiver, err := scalarTerm(body, instruction.Call.Receiver)
		if err != nil {
			return nil, fmt.Errorf("receiver: %w", err)
		}
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, fmt.Errorf("malformed method name")
		}
		operands = append(operands,
			equation.Operand{Role: "receiver", Term: receiver},
			equation.Operand{Role: "method", Term: equation.ClosedTerm([]byte("method/" + strconv.Quote(method.Str)))},
		)
		if instruction.Call.Receiver.Kind == wir.OperandPath {
			if display := body.Path(wir.PathRef(instruction.Call.Receiver.Ref)).String(); display != "" {
				operands = append(operands, equation.Operand{Role: "receiver-display", Term: equation.ClosedTerm([]byte(display))})
			}
		}
	} else {
		if instruction.Call.Callee.Kind == wir.OperandNone || instruction.Call.Receiver.Kind != wir.OperandNone {
			return nil, fmt.Errorf("malformed direct call shape")
		}
		callee, err := scalarTerm(body, instruction.Call.Callee)
		if err != nil {
			return nil, fmt.Errorf("callee: %w", err)
		}
		operands = append(operands, equation.Operand{Role: "callee", Term: callee})
		if instruction.Call.Callee.Kind == wir.OperandPath {
			calleePath := body.Path(wir.PathRef(instruction.Call.Callee.Ref)).String()
			if calleePath != "" {
				operands = append(operands, equation.Operand{Role: "callee-display", Term: equation.ClosedTerm([]byte(calleePath))})
			}
		}
	}
	for index, argument := range body.Operands(instruction.List) {
		term, err := scalarTerm(body, argument)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
		if argument.Kind == wir.OperandPath {
			if display := body.Path(wir.PathRef(argument.Ref)).String(); display != "" {
				operands = append(operands, equation.Operand{Role: indexedRole("argument-display", index), Term: equation.ClosedTerm([]byte(display))})
			}
		}
	}
	if instruction.Type != 0 {
		typeName := body.TypeDisplay(instruction.Type)
		if typeName == "" {
			return nil, fmt.Errorf("empty callee type")
		}
		operands = append(operands, equation.Operand{Role: "callee-type", Term: equation.ClosedTerm([]byte("type/" + strconv.Quote(typeName)))})
	}
	for index, ref := range body.TypeRefs(instruction.CallTypeArgs) {
		typeName := body.TypeDisplay(ref)
		if typeName == "" {
			return nil, fmt.Errorf("empty type argument %d", index)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("type-argument", index), Term: equation.ClosedTerm([]byte("type/" + strconv.Quote(typeName)))})
	}
	if instruction.Check != 0 {
		check, err := callCheckTerm(body.Check(instruction.Check))
		if err != nil {
			return nil, err
		}
		operands = append(operands, equation.Operand{Role: "check", Term: check})
	}
	operands = append(operands,
		equation.Operand{Role: "context", Term: equation.ClosedTerm([]byte("call-context/" + strconv.FormatUint(uint64(instruction.CallContext), 10)))},
		equation.Operand{Role: "result-arity", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Results.Len))))},
		equation.Operand{Role: "list-spread", Term: boolTerm(instruction.ListSpread)},
		equation.Operand{Role: "result-spread", Term: boolTerm(instruction.ResultSpread)},
		equation.Operand{Role: "final", Term: boolTerm(instruction.CallFinal)},
		equation.Operand{Role: "expanded", Term: boolTerm(instruction.CallExpanded)},
		equation.Operand{Role: "adjusted", Term: boolTerm(instruction.CallAdjusted)},
		equation.Operand{Role: "open-tail", Term: boolTerm(instruction.CallOpenTail)},
		equation.Operand{Role: "condition-negated", Term: boolTerm(instruction.CallConditionNegated)},
	)
	return operands, nil
}

// externalProvider recognizes providers whose callable identity lives outside
// this body.  Local closures deliberately remain ordinary calls: converting
// them into an external boundary would erase their body-local identity.
func externalProvider(body *wir.Body, instruction wir.Instruction) (equation.Term, bool) {
	return externalProviderSeen(body, instruction, make(map[uint32]bool))
}

func externalProviderSeen(body *wir.Body, instruction wir.Instruction, seen map[uint32]bool) (equation.Term, bool) {
	if body == nil || instruction.Op != wir.OpCall {
		return equation.Term{}, false
	}
	if instruction.Call.Method != 0 {
		method := body.Const(instruction.Call.Method).Str
		if receiverType, ok := methodReceiverType(body, instruction, seen); ok {
			if name, ok := signaturelookup.StdlibMethodProvider(receiverType, method); ok {
				return equation.ClosedTerm([]byte("provider/global/" + strconv.Quote(name))), true
			}
		}
		// Preserve the generic external boundary for a path receiver (for
		// example library:fetch()).  Only a positively typed stdlib method is
		// reclassified to its canonical library provider.
		instruction.Call.Callee = instruction.Call.Receiver
	}
	operand := instruction.Call.Callee
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, false
	}
	// A closure materialized in this WIR body is an already-published local
	// capability, never a host-global provider merely because its spelling is
	// also legal as a global name. Preserve that closed lexical authority for
	// the apply kernel instead of replacing it with an opaque boundary.
	if localClosurePath(body, operand) {
		return equation.Term{}, false
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if path.IsEmpty() || path.Symbol == 0 {
		return equation.Term{}, false
	}
	root := path.RootOnly()
	if module, ok := body.SymbolRequireModulePath(root.Symbol); ok {
		return moduleProvider(module, segment.FormatSegments(path.Segments))
	}
	kind, global := body.SymbolKind(root.Symbol)
	if module, ok := exactRequireModule(body, instruction, root.Symbol, kind, global); ok {
		return equation.ClosedTerm([]byte("provider/module-load/" + strconv.Quote(module))), true
	}
	if !global && kind != wir.SymbolGlobal && !body.IsImplicitGlobalSymbol(root.Symbol) {
		return equation.Term{}, false
	}
	name := path.String()
	if name == "" {
		return equation.Term{}, false
	}
	return equation.ClosedTerm([]byte("provider/global/" + strconv.Quote(name))), true
}

func localClosurePath(body *wir.Body, operand wir.Operand) bool {
	if body == nil || operand.Kind != wir.OperandPath {
		return false
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpClosure && instruction.Dst == operand {
			return true
		}
	}
	return false
}

type moduleProviderWire struct {
	Module string `json:"module"`
	Suffix string `json:"suffix,omitempty"`
}

// moduleProvider binds an external call to the exact member of a resolved
// require alias.  It carries structural path evidence, never a name allowlist.
func moduleProvider(module, suffix string) (equation.Term, bool) {
	if module == "" {
		return equation.Term{}, false
	}
	wired, err := json.Marshal(moduleProviderWire{Module: module, Suffix: suffix})
	if err != nil {
		return equation.Term{}, false
	}
	return equation.ClosedTerm([]byte("provider/module/v1/" + base64.RawURLEncoding.EncodeToString(wired))), true
}

// exactRequireModule recognizes Lua's direct require("module") form before
// the result is assigned a local alias. Once the alias exists,
// SymbolRequireModulePath carries the same identity for downstream calls.
// Dynamic paths and shadowed locals stay outside this provider boundary.
func exactRequireModule(body *wir.Body, instruction wir.Instruction, symbol wir.SymbolID, symbolKind wir.SymbolKind, global bool) (string, bool) {
	if body == nil || instruction.Op != wir.OpCall || instruction.Call.Method != 0 || instruction.ListSpread ||
		(!global && symbolKind != wir.SymbolGlobal && !body.IsImplicitGlobalSymbol(symbol)) || body.SymbolName(symbol) != "require" {
		return "", false
	}
	arguments := body.Operands(instruction.List)
	if len(arguments) != 1 || arguments[0].Kind != wir.OperandConst {
		return "", false
	}
	constant := body.Const(wir.ConstRef(arguments[0].Ref))
	if constant.Kind != wir.ConstString || constant.Str == "" {
		return "", false
	}
	return constant.Str, true
}

// methodReceiverType recovers a method receiver's declared type directly or
// from an earlier finite stdlib result.  The latter keeps s:lower():upper()
// within the same contract system without any call-site method-name list.
func methodReceiverType(body *wir.Body, instruction wir.Instruction, seen map[uint32]bool) (typ.Type, bool) {
	if instruction.Type != 0 {
		t := body.Type(instruction.Type)
		return t, t != nil
	}
	receiver := instruction.Call.Receiver
	if receiver.Kind == wir.OperandPath {
		path := body.Path(wir.PathRef(receiver.Ref)).RootOnly()
		for _, parameter := range body.Boundary().Parameters {
			if parameter.Symbol != path.Symbol || parameter.Type == 0 {
				continue
			}
			t := body.Type(parameter.Type)
			return t, t != nil
		}
	}
	if receiver.Kind != wir.OperandTemp || seen[receiver.Ref] {
		return nil, false
	}
	seen[receiver.Ref] = true
	defer delete(seen, receiver.Ref)
	var result typ.Type
	body.ForEachCall(func(candidate wir.Instruction) bool {
		for index, slot := range body.Operands(candidate.Results) {
			if slot != receiver {
				continue
			}
			provider, ok := externalProviderSeen(body, candidate, seen)
			if !ok {
				return false
			}
			name, ok := globalProviderName(provider)
			if !ok {
				return false
			}
			result, ok = signaturelookup.StdlibResultSlot(name, index)
			return !ok
		}
		return true
	})
	return result, result != nil
}

func globalProviderName(provider equation.Term) (string, bool) {
	encoded := strings.TrimPrefix(string(provider.Encoding), "provider/global/")
	if encoded == string(provider.Encoding) || encoded == "" {
		return "", false
	}
	name, err := strconv.Unquote(encoded)
	return name, err == nil && name != ""
}

// externalCallOperands closes the external boundary's entire source-side
// input.  Result ownership stays with call-results; this factor proves the
// provider boundary and preserves argument/result-shape distinctions for its
// eventual provider implementation.
func externalCallOperands(body *wir.Body, instruction wir.Instruction, apply equation.Coordinate, provider equation.Term) ([]equation.Operand, error) {
	if provider.Entry || len(provider.Encoding) == 0 || apply.Name == "" {
		return nil, fmt.Errorf("incomplete external call boundary")
	}
	operands := []equation.Operand{
		{Role: "application", Term: equation.ClosedTerm([]byte("call/" + apply.Name))},
		{Role: "provider", Term: provider},
		{Role: "argument-spread", Term: boolTerm(instruction.ListSpread)},
		{Role: "result-arity", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Results.Len))))},
		{Role: "result-spread", Term: boolTerm(instruction.ResultSpread)},
		{Role: "context", Term: equation.ClosedTerm([]byte("call-context/" + strconv.FormatUint(uint64(instruction.CallContext), 10)))},
	}
	for index, argument := range body.Operands(instruction.List) {
		term, err := scalarTerm(body, argument)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
	}
	if instruction.Call.Method != 0 {
		receiver, err := scalarTerm(body, instruction.Call.Receiver)
		if err != nil {
			return nil, fmt.Errorf("receiver: %w", err)
		}
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, fmt.Errorf("malformed method selector")
		}
		operands = append(operands,
			equation.Operand{Role: "receiver", Term: receiver},
			equation.Operand{Role: "method", Term: equation.ClosedTerm([]byte("method/" + strconv.Quote(method.Str)))},
		)
	}
	return operands, nil
}

// publicationOperands resolves the normalized return inventory to stable
// slots. ListSpread records how the final producer was evaluated; the List
// already carries the adjusted result coordinates consumed by publication.
// In particular, the head result of an open tail is a real, exact slot rather
// than a reason to discard the entire return operation.
func publicationOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	values := body.Operands(instruction.List)
	declared := body.DeclaredReturnTypes()
	operands := make([]equation.Operand, 0, len(values)+len(declared))
	for index, value := range values {
		term, err := scalarTerm(body, value)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("return-value", index), Term: term})
	}
	for index, declaredType := range declared {
		encoded, ok := shapefact.EncodeTarget(declaredType)
		if !ok {
			continue
		}
		operands = append(operands, equation.Operand{Role: indexedRole("declared-return", index), Term: equation.ClosedTerm(encoded)})
	}
	return operands, nil
}

func callResultOperands(body *wir.Body, instruction wir.Instruction, apply equation.Coordinate, provider equation.Term) ([]equation.Operand, error) {
	results := body.Operands(instruction.Results)
	targets := make([]wir.CallResultTarget, len(results))
	completeTargets := len(results) != 0
	for index := range results {
		target, ok := body.CallResultTarget(instruction.Point, index)
		if !ok {
			completeTargets = false
			continue
		}
		targets[index] = target
	}
	// A call result carrier is useful even when syntax has no representable
	// consumer (for example, generic-for iterator tuple setup).  Preserve every
	// result as Top, but only emit target metadata when it is complete: a partial
	// target tuple would incorrectly certify a selective result flow.
	operands := make([]equation.Operand, 1, 1+len(results)*2)
	operands[0] = equation.Operand{Role: "application", Term: equation.ClosedTerm([]byte("call/" + apply.Name))}
	// A direct callee's sealed function value is an independently published
	// return contract. Keep that fact beside the result owner; method calls
	// retain their existing receiver/method form below.
	if instruction.Call.Method == 0 {
		callee, err := scalarTerm(body, instruction.Call.Callee)
		if err != nil {
			return nil, fmt.Errorf("call result callee: %w", err)
		}
		operands = append(operands, equation.Operand{Role: "callee", Term: callee})
	}
	if len(provider.Encoding) != 0 {
		if provider.Entry {
			return nil, fmt.Errorf("provider result contract has entry term")
		}
		operands = append(operands, equation.Operand{Role: "provider", Term: provider})
		for index, argument := range body.Operands(instruction.List) {
			term, err := scalarTerm(body, argument)
			if err != nil {
				return nil, fmt.Errorf("provider argument %d: %w", index, err)
			}
			operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
		}
	}
	// A result temporary has no authored spelling of its own. Preserve a direct
	// call's complete display beside the same sealed call-result publication so
	// a later assignment diagnostic can identify the proven result without
	// recovering it from source text or a provider name.
	if display, ok := directCallDisplay(body, instruction); ok {
		operands = append(operands, equation.Operand{Role: "result-display", Term: equation.ClosedTerm([]byte(display))})
	}
	if instruction.Call.Method != 0 {
		receiver, err := scalarTerm(body, instruction.Call.Receiver)
		if err != nil {
			return nil, fmt.Errorf("call result method receiver: %w", err)
		}
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, fmt.Errorf("call result method selector")
		}
		operands = append(operands,
			equation.Operand{Role: "receiver", Term: receiver},
			equation.Operand{Role: "method", Term: equation.ClosedTerm([]byte("method/" + strconv.Quote(method.Str)))},
		)
	}
	if target, ok := typePredicateErrorTarget(body, instruction); ok {
		operands = append(operands, equation.Operand{Role: "type-predicate-error-target", Term: target})
	}
	for index, result := range results {
		term, err := scalarTerm(body, result)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("result", index), Term: term})
		if completeTargets {
			target, err := callResultTargetTerm(targets[index])
			if err != nil {
				return nil, fmt.Errorf("result target %d: %w", index, err)
			}
			operands = append(operands, equation.Operand{Role: indexedRole("target", index), Term: target})
		}
	}
	return operands, nil
}

// typePredicateErrorTarget records the closed type-value `T:is(value)`
// contract that owns its result. The target is a resolved WIR type, never
// recovered from a provider name or source spelling.
func typePredicateErrorTarget(body *wir.Body, instruction wir.Instruction) (equation.Term, bool) {
	if body == nil || instruction.Call.Method == 0 || instruction.Type == 0 || instruction.Results.Len == 0 || len(body.Operands(instruction.List)) != 1 {
		return equation.Term{}, false
	}
	method := body.Const(instruction.Call.Method)
	if method.Kind != wir.ConstString || method.Str != "is" {
		return equation.Term{}, false
	}
	target, ok := shapefact.EncodeTarget(body.Type(instruction.Type))
	if !ok {
		return equation.Term{}, false
	}
	return equation.ClosedTerm(target), true
}

func directCallDisplay(body *wir.Body, instruction wir.Instruction) (string, bool) {
	if body == nil || instruction.Call.Method != 0 || instruction.Call.Callee.Kind != wir.OperandPath {
		return "", false
	}
	callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref)).String()
	if callee == "" {
		return "", false
	}
	// Result displays intentionally omit argument spellings. The call-result
	// publication identifies the exact value relation; arguments can be
	// arbitrary expressions and are not source-facing diagnostic authority.
	return callee + "(...)", true
}

func indexedRole(prefix string, index int) string { return fmt.Sprintf("%s-%08d", prefix, index) }

func boolTerm(value bool) equation.Term {
	return equation.ClosedTerm([]byte("scalar/bool/" + strconv.FormatBool(value)))
}

func callCheckTerm(check wir.Check) (equation.Term, error) {
	if check.Kind == wir.CheckNone {
		return equation.Term{}, fmt.Errorf("empty normalized call check")
	}
	return equation.ClosedTerm([]byte(fmt.Sprintf("check/%d/path/%s/other/%s/type/%q/literal/%q/string/%q/len/%d/floor/%d/ceil/%d/has-ceil/%t/ceil-negated/%t/negated/%t/producer/%d/has-producer/%t",
		check.Kind, check.Path.Key(), check.OtherPath.Key(), check.TypeName, fmt.Sprint(check.Literal), check.LiteralString, check.LenFloor, check.NumFloor, check.NumCeil, check.HasNumCeil, check.NumCeilNegated, check.Negated, check.ProducerPoint, check.HasProducerPoint))), nil
}

func callResultTargetTerm(target wir.CallResultTarget) (equation.Term, error) {
	if target.Index < 0 || target.ResultIndex < 0 {
		return equation.Term{}, fmt.Errorf("negative result index")
	}
	base := fmt.Sprintf("result-target/%d/index/%d/result/%d", target.Kind, target.Index, target.ResultIndex)
	if !target.Path.IsEmpty() {
		if target.Path.Key() == "" {
			return equation.Term{}, fmt.Errorf("empty target path key")
		}
		base += "/path/" + string(target.Path.Key())
	}
	switch target.Kind {
	case wir.CallResultTargetLocalAssignment, wir.CallResultTargetOrdinaryAssignment, wir.CallResultTargetReturn, wir.CallResultTargetExpression:
		return equation.ClosedTerm([]byte(base)), nil
	default:
		return equation.Term{}, fmt.Errorf("unknown result target kind %d", target.Kind)
	}
}

type allocationOperandSets struct {
	template        []equation.Operand
	materialization []equation.Operand
}

// allocationOperands seals the whole syntactic allocation before either of its
// equation occurrences is emitted. An open table tail is admitted as its
// source-owned final producer, but deliberately cannot certify a finite object
// graph: materialization receives its exact open-tail marker and the front
// withholds closed-table shape facts.
func allocationOperands(body *wir.Body, instruction wir.Instruction, allocationSite string) (allocationOperandSets, error) {
	result, err := allocationValueTerm(body, instruction.Dst)
	if err != nil {
		return allocationOperandSets{}, fmt.Errorf("destination: %w", err)
	}
	if allocationSite == "" {
		return allocationOperandSets{}, fmt.Errorf("missing allocation site")
	}
	site := equation.ClosedTerm([]byte("allocation-site/" + allocationSite))
	sets := allocationOperandSets{
		template: []equation.Operand{
			{Role: "site", Term: site},
			{Role: "result", Term: result},
		},
		materialization: []equation.Operand{
			{Role: "site", Term: site},
			{Role: "result", Term: result},
		},
	}
	switch instruction.Op {
	case wir.OpMakeTable:
		if !instruction.StaticStringKeysComplete {
			return allocationOperandSets{}, fmt.Errorf("table constructor has a non-exact key")
		}
		typeTerm, err := allocationTypeTerm(body, instruction.Type)
		if err != nil {
			return allocationOperandSets{}, err
		}
		sets.template = append(sets.template,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("allocation-kind/table"))},
			equation.Operand{Role: "type", Term: typeTerm},
		)
		sets.materialization = append(sets.materialization,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("object-kind/table"))},
			equation.Operand{Role: "list-floor", Term: listFloorTerm(body, instruction)},
		)
		if instruction.ListSpread {
			values := body.Operands(instruction.List)
			if len(values) == 0 {
				return allocationOperandSets{}, fmt.Errorf("table constructor has an empty open final value tail")
			}
			tail, err := allocationValueTerm(body, values[len(values)-1])
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table constructor open tail: %w", err)
			}
			// The marker is part of both frozen allocation occurrences. The
			// materializer can retain the exact source producer without treating
			// unknown arity as an absent member or a closed array bound.
			sets.template = append(sets.template,
				equation.Operand{Role: "open-tail", Term: boolTerm(true)},
				equation.Operand{Role: "tail", Term: tail},
			)
			sets.materialization = append(sets.materialization,
				equation.Operand{Role: "open-tail", Term: boolTerm(true)},
				equation.Operand{Role: "tail", Term: tail},
			)
		} else {
			sets.template = append(sets.template, equation.Operand{Role: "open-tail", Term: boolTerm(false)})
			sets.materialization = append(sets.materialization, equation.Operand{Role: "open-tail", Term: boolTerm(false)})
		}
		for index, entry := range body.TableEntries(instruction.TableEntries) {
			if len(entry.Suffix.Segments) == 0 {
				return allocationOperandSets{}, fmt.Errorf("table entry %d has no exact suffix", index)
			}
			value, err := allocationValueTerm(body, entry.Value)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table entry %d: %w", index, err)
			}
			if isNilConstant(body, entry.Value) {
				// Lua removes a key assigned nil.  Do not encode an object member
				// for it: absence is a distinct state, not a Bottom value.
				continue
			}
			sets.materialization = append(sets.materialization, equation.Operand{
				Role: fmt.Sprintf("member-%08d", index),
				Term: equation.ClosedTerm([]byte("member/" + segment.FormatSegments(entry.Suffix.Segments) + "/" + string(value.Encoding))),
			})
		}
		for index, valueOperand := range body.Operands(instruction.List) {
			value, err := allocationValueTerm(body, valueOperand)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table value %d: %w", index, err)
			}
			sets.template = append(sets.template, equation.Operand{
				Role: fmt.Sprintf("value-%08d", index), Term: value,
			})
		}
	case wir.OpClosure:
		proto := body.Proto(instruction.Func)
		if instruction.Func == 0 || proto.Body == nil || proto.Graph == nil || proto.Name == "" {
			return allocationOperandSets{}, fmt.Errorf("closure has no complete nested prototype")
		}
		sets.template = append(sets.template,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("allocation-kind/closure"))},
			equation.Operand{Role: "prototype", Term: equation.ClosedTerm([]byte("prototype/" + prototypeIdentity(proto)))},
		)
		sets.materialization = append(sets.materialization,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("object-kind/closure"))},
			equation.Operand{Role: "prototype", Term: equation.ClosedTerm([]byte("prototype/" + prototypeIdentity(proto)))},
		)
		for index, capture := range body.Operands(instruction.List) {
			value, err := allocationValueTerm(body, capture)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("closure capture %d: %w", index, err)
			}
			sets.materialization = append(sets.materialization, equation.Operand{
				Role: fmt.Sprintf("capture-%08d", index), Term: value,
			})
		}
	default:
		return allocationOperandSets{}, fmt.Errorf("instruction %d does not allocate an object", instruction.Op)
	}
	return sets, nil
}

func allocationValueTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	if term, err := scalarTerm(body, operand); err == nil {
		return term, nil
	}
	switch operand.Kind {
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte(fmt.Sprintf("temp/%08d", operand.Ref))), nil
	case wir.OperandVararg:
		return equation.ClosedTerm([]byte("vararg")), nil
	default:
		return equation.Term{}, fmt.Errorf("operand kind %d is not a sealed value", operand.Kind)
	}
}

// allocationWriteOperands closes the value produced by every constructor.
func allocationWriteOperands(body *wir.Body, instruction wir.Instruction, current operation, operations []operation) ([]equation.Operand, error) {
	target, err := allocationTargetTerm(body, instruction.Dst)
	if err != nil {
		return nil, err
	}
	allocationResult, err := allocationValueTerm(body, instruction.Dst)
	if err != nil {
		return nil, err
	}
	value := "scalar/table"
	if instruction.Op == wir.OpClosure {
		proto := body.Proto(instruction.Func)
		value = functionValue(proto.Type)
	} else if shape, ok, err := tableShapeTerm(body, instruction); err != nil {
		return nil, err
	} else if ok {
		value = string(shape)
	}
	readBefore, err := precedingReadBoundary(current, operations)
	if err != nil {
		return nil, err
	}
	return []equation.Operand{
		{Role: "target", Term: target},
		{Role: "display", Term: hiddenAllocationDisplay(current.target)},
		{Role: "value", Term: equation.ClosedTerm([]byte(value))},
		{Role: "allocation-result", Term: allocationResult},
		{Role: "read-before", Term: readBefore},
		{Role: "absence", Term: equation.ClosedTerm([]byte("front/absence/error"))},
	}, nil
}

// tableShapeTerm turns the WIR constructor inventory into a closed, finite
// value fact. It deliberately declines an open tail or unclassified key: those
// shapes have no complete member-presence proof.
func tableShapeTerm(body *wir.Body, instruction wir.Instruction) ([]byte, bool, error) {
	if instruction.Op != wir.OpMakeTable || !instruction.StaticStringKeysComplete || instruction.ListSpread {
		return nil, false, nil
	}
	bySuffix := make(map[string]shapefact.Member)
	for _, entry := range body.TableEntries(instruction.TableEntries) {
		suffix := segment.FormatSegments(entry.Suffix.Segments)
		if suffix == "" {
			return nil, false, fmt.Errorf("table member has no suffix")
		}
		member := shapefact.Member{Suffix: suffix}
		if !isNilConstant(body, entry.Value) {
			value, err := allocationValueTerm(body, entry.Value)
			if err != nil {
				return nil, false, err
			}
			member.Present, member.Value = true, string(value.Encoding)
		}
		// Lua constructor writes are ordered; the final duplicate key wins.
		bySuffix[suffix] = member
	}
	members := make([]shapefact.Member, 0, len(bySuffix))
	for _, member := range bySuffix {
		members = append(members, member)
	}
	shape, ok := shapefact.EncodeTable(shapefact.Table{Closed: true, Members: members})
	return shape, ok, nil
}

// functionValue seals the callable shape into the constructor's ordinary
// value fact.  It is deliberately a closed transport term: apply later reads
// that fact through the equation partition, rather than consulting WIR or
// re-analysing source.
func functionValue(t typ.Type) string {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return "scalar/function"
	}
	type typeParam struct {
		Name       string `json:"name"`
		Constraint string `json:"constraint,omitempty"`
	}
	type signature struct {
		Params     []string    `json:"params"`
		Returns    []string    `json:"returns"`
		TypeParams []typeParam `json:"type_params"`
		Required   int         `json:"required"`
		Variadic   bool        `json:"variadic"`
		// VariadicType is the resolved element contract for every argument past
		// Params.  Variadic alone only records arity and cannot prove or reject a
		// tail argument at the apply boundary.
		VariadicType string `json:"variadic_type,omitempty"`
		Canonical    string `json:"canonical,omitempty"`
	}
	wire := signature{
		Params:   make([]string, len(fn.Params)),
		Returns:  make([]string, len(fn.Returns)),
		Variadic: fn.Variadic != nil,
	}
	for _, param := range fn.TypeParams {
		if param == nil || param.Name == "" {
			return "scalar/function"
		}
		item := typeParam{Name: param.Name}
		if param.Constraint != nil {
			item.Constraint = param.Constraint.String()
		}
		wire.TypeParams = append(wire.TypeParams, item)
	}
	for index, param := range fn.Params {
		if param.Type == nil {
			return "scalar/function"
		}
		wire.Params[index] = param.Type.String()
		if bound, ok := unwrap.Annotations(param.Type).(*typ.TypeParam); ok && bound.Name != "" {
			found := false
			for _, existing := range wire.TypeParams {
				found = found || existing.Name == bound.Name
			}
			if !found {
				item := typeParam{Name: bound.Name}
				if bound.Constraint != nil {
					item.Constraint = bound.Constraint.String()
				}
				wire.TypeParams = append(wire.TypeParams, item)
			}
		}
		// Lua's annotated optional parameter surface (T?) is callable with an
		// omitted trailing argument even when the parser has no default-value
		// marker on the parameter slot.
		if !param.Optional && !strings.HasSuffix(wire.Params[index], "?") {
			wire.Required++
		}
	}
	for index, result := range fn.Returns {
		if result == nil {
			return "scalar/function"
		}
		wire.Returns[index] = result.String()
	}
	if fn.Variadic != nil {
		wire.VariadicType = fn.Variadic.String()
		if wire.VariadicType == "" {
			return "scalar/function"
		}
	}
	// Retain the existing callable shape for local apply while attaching the
	// canonical function type for closed publication/export consumers.
	if canonical, err := typ.EncodeCanonical(context.Background(), fn); err == nil && len(canonical) != 0 {
		wire.Canonical = base64.RawURLEncoding.EncodeToString(canonical)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "scalar/function"
	}
	return "scalar/function/" + base64.RawURLEncoding.EncodeToString(encoded)
}

// allocationEntryWriteOperands projects a closed constructor entry onto its
// root path. The lowering layer already flattens nested static table entries.
func allocationEntryWriteOperands(body *wir.Body, instruction wir.Instruction, current operation, operations []operation) ([]equation.Operand, error) {
	if instruction.Dst.Kind != wir.OperandPath || current.allocationEntry == nil {
		return nil, fmt.Errorf("missing static table entry target")
	}
	root := body.Path(wir.PathRef(instruction.Dst.Ref))
	targetPath := root.AppendPathSuffix(current.allocationEntry.Suffix)
	target, display, err := closedPathTerm(targetPath)
	if err != nil {
		return nil, err
	}
	value, err := allocationValueTerm(body, current.allocationEntry.Value)
	if err != nil {
		return nil, fmt.Errorf("entry value: %w", err)
	}
	readBefore, err := precedingReadBoundary(current, operations)
	if err != nil {
		return nil, err
	}
	return []equation.Operand{
		{Role: "target", Term: target},
		{Role: "display", Term: equation.ClosedTerm([]byte("front/hidden/allocation/" + display))},
		{Role: "value", Term: value},
		{Role: "read-before", Term: readBefore},
		{Role: "absence", Term: equation.ClosedTerm([]byte("front/absence/error"))},
	}, nil
}

func allocationTargetTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	switch operand.Kind {
	case wir.OperandPath:
		term, _, err := pathTerm(body, operand)
		return term, err
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte("temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
	default:
		return equation.Term{}, fmt.Errorf("destination is operand kind %d", operand.Kind)
	}
}

func closedPathTerm(value path.Path) (equation.Term, string, error) {
	if value.IsEmpty() || value.Key() == "" || value.String() == "" {
		return equation.Term{}, "", fmt.Errorf("empty static table entry path")
	}
	return equation.ClosedTerm([]byte("path/" + value.Key())), value.String(), nil
}

func precedingReadBoundary(current operation, operations []operation) (equation.Term, error) {
	for index, candidate := range operations {
		if candidate.target != current.target {
			continue
		}
		if index == 0 {
			return equation.Term{}, fmt.Errorf("write has no predecessor")
		}
		return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
	}
	return equation.Term{}, fmt.Errorf("write operation is absent")
}

func hiddenAllocationDisplay(target equation.Coordinate) equation.Term {
	return equation.ClosedTerm([]byte("front/hidden/allocation/" + target.Name))
}

func allocationTypeTerm(body *wir.Body, ref wir.TypeRef) (equation.Term, error) {
	if ref == 0 {
		return equation.ClosedTerm([]byte("type/none")), nil
	}
	display := body.TypeDisplay(ref)
	if display == "" {
		return equation.Term{}, fmt.Errorf("unknown table type")
	}
	return equation.ClosedTerm([]byte("type/" + display)), nil
}

func isNilConstant(body *wir.Body, operand wir.Operand) bool {
	return operand.Kind == wir.OperandConst && body.Const(wir.ConstRef(operand.Ref)).Kind == wir.ConstNil
}

// listFloorTerm reports only a proven contiguous prefix.  It never treats a
// missing, nil, or non-positive element as a list member, preserving the
// distinction between an absent key and a present nil value.
func listFloorTerm(body *wir.Body, instruction wir.Instruction) equation.Term {
	floor := 0
	entries := body.TableEntries(instruction.TableEntries)
	for {
		found := false
		for _, entry := range entries {
			if !exactPositiveIndex(entry, floor+1) || isNilConstant(body, entry.Value) {
				continue
			}
			found = true
			break
		}
		if !found {
			break
		}
		floor++
	}
	return equation.ClosedTerm([]byte(fmt.Sprintf("list-floor/%d", floor)))
}

func exactPositiveIndex(entry wir.TableEntry, index int) bool {
	return len(entry.Suffix.Segments) == 1 &&
		entry.Suffix.Segments[0].Kind == segment.SegmentIndexInt &&
		entry.Suffix.Segments[0].Index == index
}

type branchGuardTarget struct {
	target              equation.Coordinate
	literalDiscriminant bool
}

// literalLoopDiscriminant recognizes the only literal relation whose selected
// arm can cross a cyclic iteration boundary: a field of the value bound by an
// existing generic iterator. Other literal comparisons still have ordinary CFG
// guards, but cannot make a union-arm publication survive a later iteration.
func literalLoopDiscriminant(check wir.Check, loopBindingRoots map[string]bool) bool {
	if (check.Kind != wir.CheckLiteralEqual && check.Kind != wir.CheckLiteralNot) || check.Path.IsEmpty() || len(check.Path.Segments) == 0 {
		return false
	}
	root := check.Path
	root.Segments = nil
	root.Version = 0
	return loopBindingRoots["path/"+string(root.Key())]
}

func guardsForPoint(graph cfg.Graph, reachability *reachabilityCache, point cfg.Point, body equation.BodyID, branches map[cfg.Point]branchGuardTarget) []equation.Guard {
	guards := make([]equation.Guard, 0, len(branches))
	for branch, target := range branches {
		if branch == point {
			continue
		}
		trueReach, falseReach := false, false
		for _, successor := range graph.Successors(branch) {
			condition, isBranchEdge := graph.EdgeCond(branch, successor)
			reaches := reachability.reaches(successor, point)
			if target.literalDiscriminant {
				// A loop back-edge can reach this point only by evaluating the
				// same literal discriminant again. That is a later iteration,
				// not an alternate edge of the current decision, so it must not
				// erase this selected arm's guard.
				reaches = reachability.reachesWithout(successor, point, branch)
			}
			if !isBranchEdge || !reaches {
				continue
			}
			if condition {
				trueReach = true
			} else {
				falseReach = true
			}
		}
		if trueReach == falseReach {
			continue
		}
		edge := "false"
		if trueReach {
			edge = "true"
		}
		guards = append(guards, equation.Guard{Body: body, Encoding: []byte("front/branch/" + target.target.Name + "/" + edge)})
	}
	return guards
}

// loopHeaderGuards removes only branch guards that are proved to feed the
// current iteration header through a back-edge. The iterator transaction is
// the completed write at the start of each iteration, before such a body
// predicate is evaluated. Ordinary outer branch guards remain intact.
func loopHeaderGuards(graph cfg.Graph, reachability *reachabilityCache, point cfg.Point, body equation.BodyID, branches map[cfg.Point]branchGuardTarget) []equation.Guard {
	guards := guardsForPoint(graph, reachability, point, body, branches)
	if len(guards) == 0 {
		return nil
	}
	backEdges := make(map[string]bool)
	for branch, target := range branches {
		if !reachability.reaches(point, branch) {
			continue
		}
		for _, successor := range graph.Successors(branch) {
			if branchEdge, isBranchEdge := graph.EdgeCond(branch, successor); isBranchEdge && reachability.reaches(successor, point) {
				backEdges[target.target.Name+"/"+strconv.FormatBool(branchEdge)] = true
			}
		}
	}
	kept := guards[:0]
	for _, guard := range guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || !backEdges[parts[2]+"/"+parts[3]] {
			kept = append(kept, guard)
		}
	}
	return kept
}

// suspensionLiveAllocations records an allocation root only when the immutable
// WIR and CFG establish all three parts of the lifetime witness: construction
// reaches a receive call, the allocation root is not rebound before that call,
// and a read of that root (or one of its members) is reachable after it.  It is
// deliberately a front-side relation rather than a placement heuristic: later
// stages consume the exact root term that the allocation already published.
func suspensionLiveAllocations(body *wir.Body, graph cfg.Graph, reachability *reachabilityCache) map[cfg.Point][]equation.Term {
	if body == nil || graph == nil || reachability == nil {
		return nil
	}
	type allocation struct {
		index int
		point cfg.Point
		root  wir.Operand
	}
	allocations := make([]allocation, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable && instruction.Op != wir.OpClosure {
			continue
		}
		if instruction.Dst.Kind != wir.OperandPath || body.Path(wir.PathRef(instruction.Dst.Ref)).IsEmpty() {
			continue
		}
		allocations = append(allocations, allocation{index: index, point: instruction.Point, root: instruction.Dst})
	}
	if len(allocations) == 0 {
		return nil
	}
	live := make(map[cfg.Point][]equation.Term)
	for callIndex := 0; callIndex < body.Len(); callIndex++ {
		call := body.Instr(callIndex)
		if !receiveCall(body, call) {
			continue
		}
		for _, candidate := range allocations {
			if candidate.index >= callIndex || !reachability.reaches(candidate.point, call.Point) ||
				reboundRootBefore(body, candidate.root, candidate.index+1, callIndex) ||
				!rootReadAfter(body, reachability, candidate.root, call.Point, callIndex+1) {
				continue
			}
			term, err := scalarTerm(body, candidate.root)
			if err != nil {
				continue
			}
			live[call.Point] = append(live[call.Point], term)
		}
	}
	return live
}

func receiveCall(body *wir.Body, instruction wir.Instruction) bool {
	return instruction.Op == wir.OpCall && instruction.Call.Receiver.Kind != wir.OperandNone &&
		instruction.Call.Method != 0 && body.Const(instruction.Call.Method).Kind == wir.ConstString &&
		body.Const(instruction.Call.Method).Str == "receive"
}

func reboundRootBefore(body *wir.Body, root wir.Operand, start, end int) bool {
	rootPath := body.Path(wir.PathRef(root.Ref))
	for index := start; index < end; index++ {
		instruction := body.Instr(index)
		if !instruction.WritesAssignmentPoint() || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		if body.Path(wir.PathRef(instruction.Dst.Ref)).SameRootIgnoringVersion(rootPath) && len(body.Path(wir.PathRef(instruction.Dst.Ref)).Segments) == 0 {
			return true
		}
	}
	return false
}

func rootReadAfter(body *wir.Body, reachability *reachabilityCache, root wir.Operand, call cfg.Point, start int) bool {
	rootPath := body.Path(wir.PathRef(root.Ref))
	for index := start; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if !reachability.reaches(call, instruction.Point) {
			continue
		}
		if instructionReadsRoot(body, instruction, rootPath) {
			return true
		}
	}
	return false
}

func instructionReadsRoot(body *wir.Body, instruction wir.Instruction, root path.Path) bool {
	reads := func(operand wir.Operand) bool {
		return operand.Kind == wir.OperandPath && body.Path(wir.PathRef(operand.Ref)).SameRootIgnoringVersion(root)
	}
	if reads(instruction.A) || reads(instruction.B) || reads(instruction.Call.Callee) || reads(instruction.Call.Receiver) {
		return true
	}
	for _, operand := range body.Operands(instruction.List) {
		if reads(operand) {
			return true
		}
	}
	return false
}

func graphHasCycle(graph cfg.Graph) bool {
	visiting := make(map[cfg.Point]bool, graph.Size())
	visited := make(map[cfg.Point]bool, graph.Size())
	var visit func(cfg.Point) bool
	visit = func(point cfg.Point) bool {
		if visiting[point] {
			return true
		}
		if visited[point] {
			return false
		}
		visiting[point] = true
		for _, next := range graph.Successors(point) {
			if visit(next) {
				return true
			}
		}
		visiting[point] = false
		visited[point] = true
		return false
	}
	return visit(graph.Entry())
}

// freezeCyclicArtifact translates the already-admitted equation stream and
// CFG topology into a closed cyclic certificate. The resulting WTO is
// computed once here and retained verbatim by the evaluator -- execution
// never discovers or rebuilds a schedule.
func freezeCyclicArtifact(artifact equation.Artifact, body *wir.Body, graph cfg.Graph) (equation.CyclicArtifact, error) {
	if len(artifact.Equations) == 0 {
		return equation.CyclicArtifact{}, fmt.Errorf("front: cannot freeze an empty cyclic artifact")
	}
	cells := make([]equation.CellID, 0, len(artifact.Equations))
	byTarget := make(map[equation.Coordinate]equation.CellID, len(artifact.Equations))
	for _, operation := range artifact.Equations {
		cell := equation.CellID("front/" + operation.Target.Name)
		cells = append(cells, cell)
		byTarget[operation.Target] = cell
	}
	pointCells, err := cyclicOperationCells(artifact, body, graph, byTarget)
	if err != nil {
		return equation.CyclicArtifact{}, err
	}
	edges := make(map[equation.CellID][]equation.CellID, len(cells))
	dependencies := make([]equation.SemanticDependency, 0, len(cells))
	for _, operation := range artifact.Equations {
		to := byTarget[operation.Target]
		for _, target := range operation.Dependencies {
			from, ok := byTarget[target]
			if !ok {
				return equation.CyclicArtifact{}, fmt.Errorf("front: cyclic dependency %s has no cell", target.Name)
			}
			edges[from] = append(edges[from], to)
			dependencies = append(dependencies, equation.SemanticDependency{From: from, To: to, Reason: equation.EdgeContractRead, Evidence: "front/operation-order"})
		}
	}
	for point, sources := range pointCells {
		from := sources[len(sources)-1]
		for _, next := range graph.Successors(point) {
			for _, to := range cyclicReachableOperationCells(graph, next, pointCells) {
				edges[from] = append(edges[from], to)
				dependencies = append(dependencies, equation.SemanticDependency{From: from, To: to, Reason: equation.EdgeContractAdvance, Evidence: "front/cfg-edge"})
			}
		}
	}
	plan := solve.NewWTOPlan(cells, func(cell equation.CellID) []equation.CellID {
		return append([]equation.CellID(nil), edges[cell]...)
	})
	cyclic, err := equation.NewCyclicArtifact(artifact, byTarget, plan, dependencies,
		[]equation.OutputSelector{{ID: "published", Cells: append([]equation.CellID(nil), cells...)}},
		[]equation.CellID{cells[0]}, append([]equation.CellID(nil), cells...))
	if err != nil {
		return equation.CyclicArtifact{}, fmt.Errorf("front: freeze cyclic artifact: %w", err)
	}
	return cyclic, nil
}

// cyclicOperationCells repeats only the front's operation cardinality pass.
// The produced coordinates are the already-compiled operation names, so this
// cannot create a second lowering or infer an alternate equation topology.
func cyclicOperationCells(artifact equation.Artifact, body *wir.Body, graph cfg.Graph, byTarget map[equation.Coordinate]equation.CellID) (map[cfg.Point][]equation.CellID, error) {
	if body == nil || graph == nil || len(artifact.Equations) == 0 {
		return nil, fmt.Errorf("front: cyclic operation map has no body")
	}
	loopBindings, err := genericForBindings(body, graph)
	if err != nil {
		return nil, err
	}
	loopBindingPoints := make(map[cfg.Point]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingPoints[binding.point] = true
		}
	}
	for point := range numericForBindingPoints(body) {
		loopBindingPoints[point] = true
	}
	bodyID := artifact.Equations[0].Target.Body
	points := make(map[cfg.Point][]equation.CellID)
	index := 0
	appendAt := func(point cfg.Point, count int) error {
		for offset := 0; offset < count; offset++ {
			target := equation.Coordinate{Body: bodyID, Name: operationName(index)}
			cell, ok := byTarget[target]
			if !ok {
				return fmt.Errorf("front: cyclic operation %s has no compiled cell", target.Name)
			}
			points[point] = append(points[point], cell)
			index++
		}
		return nil
	}
	for instructionIndex := 0; instructionIndex < body.Len(); instructionIndex++ {
		instruction := body.Instr(instructionIndex)
		count := 0
		switch instruction.Op {
		case wir.OpEntry, wir.OpStaticMemberWrite, wir.OpDynamicIndexRead, wir.OpBranch, wir.OpClaim, wir.OpSelect, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpLogical, wir.OpAssign, wir.OpIterate, wir.OpReturn:
			count = 1
			if instruction.Op == wir.OpAssign && instruction.A.Kind == wir.OperandNone && loopBindingPoints[instruction.Point] {
				count = 0
			}
		case wir.OpDynamicIndexWrite:
			count = 2
		case wir.OpMakeTable, wir.OpClosure:
			count = 3
			if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandPath {
				count += len(body.TableEntries(instruction.TableEntries))
			}
		case wir.OpCall:
			count = 2
			if _, external := externalProvider(body, instruction); external {
				count++
			}
		case wir.OpExit, wir.OpNoop:
		default:
			return nil, fmt.Errorf("front: cyclic operation map: %w: %d", ErrUnsupportedInstruction, instruction.Op)
		}
		if err := appendAt(instruction.Point, count); err != nil {
			return nil, err
		}
	}
	if index != len(artifact.Equations) {
		return nil, fmt.Errorf("front: cyclic operation map has %d cells, want %d", index, len(artifact.Equations))
	}
	return points, nil
}

func cyclicReachableOperationCells(graph cfg.Graph, start cfg.Point, points map[cfg.Point][]equation.CellID) []equation.CellID {
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{start}
	var cells []equation.CellID
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[point] {
			continue
		}
		seen[point] = true
		if atPoint := points[point]; len(atPoint) != 0 {
			cells = append(cells, atPoint[0])
			continue
		}
		stack = append(stack, graph.Successors(point)...)
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i] < cells[j] })
	return cells
}

// reachabilityCache shares each successor's graph walk across every operation
// that needs branch guards. Large straight-line fixtures otherwise repeat the
// same O(branches*points) traversal for every draft.
type reachabilityCache struct {
	graph   cfg.Graph
	from    map[cfg.Point]map[cfg.Point]bool
	without map[reachabilityExclusion]bool
}

type reachabilityExclusion struct {
	from, target, exclude cfg.Point
}

func newReachabilityCache(graph cfg.Graph) *reachabilityCache {
	return &reachabilityCache{
		graph:   graph,
		from:    make(map[cfg.Point]map[cfg.Point]bool),
		without: make(map[reachabilityExclusion]bool),
	}
}

func (cache *reachabilityCache) reaches(from, target cfg.Point) bool {
	reachable, found := cache.from[from]
	if !found {
		reachable = make(map[cfg.Point]bool, cache.graph.Size())
		stack := []cfg.Point{from}
		for len(stack) != 0 {
			point := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if reachable[point] {
				continue
			}
			reachable[point] = true
			stack = append(stack, cache.graph.Successors(point)...)
		}
		cache.from[from] = reachable
	}
	return reachable[target]
}

// reachesWithout answers whether an arm reaches a point before control loops
// back through the branch that selected the arm. This preserves same-iteration
// branch ownership while retaining ordinary reachability for non-cyclic CFGs.
func (cache *reachabilityCache) reachesWithout(from, target, exclude cfg.Point) bool {
	key := reachabilityExclusion{from: from, target: target, exclude: exclude}
	if reachable, found := cache.without[key]; found {
		return reachable
	}
	seen := make(map[cfg.Point]bool, cache.graph.Size())
	stack := []cfg.Point{from}
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if point == exclude || seen[point] {
			continue
		}
		if point == target {
			cache.without[key] = true
			return true
		}
		seen[point] = true
		stack = append(stack, cache.graph.Successors(point)...)
	}
	cache.without[key] = false
	return false
}
