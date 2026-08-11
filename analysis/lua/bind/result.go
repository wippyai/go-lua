package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Result records lexical declaration identities for identifier occurrences.
type Result struct {
	identSymbols       map[*ast.IdentExpr]symbol.ID
	implicitGlobalUses map[*ast.IdentExpr]struct{}
	runtimeTypeValues  map[*ast.IdentExpr]RuntimeTypeValue
	directGlobalCalls  []DirectGlobalCall
	globals            globalAuthority

	nextSymbolID symbol.ID

	names map[symbol.ID]string
	kinds map[symbol.ID]symbol.Kind

	functions       []*ast.FunctionExpr
	functionOrigins map[*ast.FunctionExpr]FunctionOrigin
	directCaptures  map[*ast.FunctionExpr][]Capture
	varargSymbols   map[*ast.FunctionExpr]symbol.ID
	paramSlots      map[*ast.FunctionExpr][]ParamSlot
	// symbolAnnotations retains exact declaration syntax independently of
	// runtime-use evidence, including formals of static query Functions.
	symbolAnnotations map[symbol.ID]ast.TypeExpr
	localSymbols      map[*ast.LocalAssignStmt][]symbol.ID
	numForSymbols     map[*ast.NumberForStmt]symbol.ID
	genericForSymbols map[*ast.GenericForStmt][]symbol.ID

	gotoTargets   map[*ast.GotoStmt]*ast.LabelStmt
	controlIssues []ControlIssue

	nextTypeDeclID         TypeDeclID
	typeRefs               map[*ast.TypeRefExpr]TypeDecl
	primitiveTypeRefs      map[*ast.PrimitiveTypeExpr]TypeDecl
	typeDefDecls           map[*ast.TypeDefStmt]TypeDecl
	interfaceDecls         map[*ast.InterfaceDefStmt]TypeDecl
	typeDefParams          map[*ast.TypeDefStmt][]TypeDecl
	functionTypeParams     map[ast.PositionHolder][]TypeDecl
	methodReceiverTypes    map[*ast.FunctionExpr]TypeDecl
	assertedParams         map[*ast.AssertsTypeExpr]int
	qualifiedTypeRefs      map[*ast.TypeRefExpr]QualifiedTypeAlias
	staticTypePublications map[*ast.AssignStmt][]StaticTypePublication
	// qualifiedTypeRootSymbols records the exact lexical value root selected by
	// each authored qualified type-reference occurrence.
	qualifiedTypeRootSymbols map[*ast.TypeRefExpr]symbol.ID
}

// DirectGlobalCall is one plain call whose exact function identifier resolved
// to a global during the binder's canonical source traversal.  It is generic
// binding evidence: consumers select a named global through Global.Matches;
// the binder itself has no module policy.
type DirectGlobalCall struct {
	Call   *ast.FuncCallExpr
	Global GlobalIdentity
}

func newResult() *Result {
	r := &Result{
		identSymbols:             make(map[*ast.IdentExpr]symbol.ID),
		implicitGlobalUses:       make(map[*ast.IdentExpr]struct{}),
		names:                    make(map[symbol.ID]string),
		kinds:                    make(map[symbol.ID]symbol.Kind),
		functionOrigins:          make(map[*ast.FunctionExpr]FunctionOrigin),
		directCaptures:           make(map[*ast.FunctionExpr][]Capture),
		varargSymbols:            make(map[*ast.FunctionExpr]symbol.ID),
		paramSlots:               make(map[*ast.FunctionExpr][]ParamSlot),
		symbolAnnotations:        make(map[symbol.ID]ast.TypeExpr),
		localSymbols:             make(map[*ast.LocalAssignStmt][]symbol.ID),
		numForSymbols:            make(map[*ast.NumberForStmt]symbol.ID),
		genericForSymbols:        make(map[*ast.GenericForStmt][]symbol.ID),
		typeRefs:                 make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:        make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:             make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:           make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:            make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams:       make(map[ast.PositionHolder][]TypeDecl),
		methodReceiverTypes:      make(map[*ast.FunctionExpr]TypeDecl),
		qualifiedTypeRefs:        make(map[*ast.TypeRefExpr]QualifiedTypeAlias),
		qualifiedTypeRootSymbols: make(map[*ast.TypeRefExpr]symbol.ID),
	}
	return r
}
