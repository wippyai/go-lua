package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// Result records lexical declaration identities for identifier occurrences.
type Result struct {
	identSymbols       map[*ast.IdentExpr]Symbol
	implicitGlobalUses map[*ast.IdentExpr]struct{}
	runtimeTypeValues  map[*ast.IdentExpr]RuntimeTypeValue
	directGlobalCalls  []DirectGlobalCall
	callSpellings      map[*ast.FuncCallExpr]string
	globals            globalAuthority

	nextSymbolID Symbol

	symbols map[Symbol]symbolInfo

	functions       []*ast.FunctionExpr
	functionOrigins map[*ast.FunctionExpr]FunctionOrigin
	directCaptures  map[*ast.FunctionExpr][]Capture
	varargSymbols   map[*ast.FunctionExpr]Symbol
	paramSlots      map[*ast.FunctionExpr][]ParamSlot
	// symbolAnnotations retains exact declaration syntax independently of
	// runtime-use evidence, including formals of static query Functions.
	symbolAnnotations map[Symbol]ast.TypeExpr
	localSymbols      map[*ast.LocalAssignStmt][]Symbol
	numForSymbols     map[*ast.NumberForStmt]Symbol
	genericForSymbols map[*ast.GenericForStmt][]Symbol

	gotoTargets   map[*ast.GotoStmt]*ast.LabelStmt
	controlIssues []ControlIssue

	nextTypeDeclID TypeDeclID
	// typeValueRefs retains binder-owned value-position occurrences whose
	// lexical spelling denotes a declared type. Lowering consults this only at
	// a call-argument boundary; ordinary value positions remain reads.
	typeValueRefs          map[*ast.IdentExpr]TypeDecl
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
	qualifiedTypeRootSymbols map[*ast.TypeRefExpr]Symbol
}

// DirectGlobalCall is one plain call whose exact function identifier resolved
// to a global during the binder's canonical source traversal.  It is generic
// binding evidence: consumers select a named global through Global.Matches;
// the binder itself has no module policy.
type DirectGlobalCall struct {
	// Call is retained only as the parser-owned occurrence key. Consumers must
	// use the detached evidence below rather than reopening its syntax.
	Call   *ast.FuncCallExpr
	Global GlobalIdentity

	// ArgumentCount is the authored call arity. HasAuthoredString is true only
	// when the call has exactly one authored string literal; AuthoredString then
	// carries that literal verbatim, including the empty string.
	ArgumentCount     int
	AuthoredString    string
	HasAuthoredString bool
}

func newResult() *Result {
	r := &Result{
		identSymbols:             make(map[*ast.IdentExpr]Symbol),
		implicitGlobalUses:       make(map[*ast.IdentExpr]struct{}),
		callSpellings:            make(map[*ast.FuncCallExpr]string),
		symbols:                  make(map[Symbol]symbolInfo),
		functionOrigins:          make(map[*ast.FunctionExpr]FunctionOrigin),
		directCaptures:           make(map[*ast.FunctionExpr][]Capture),
		varargSymbols:            make(map[*ast.FunctionExpr]Symbol),
		paramSlots:               make(map[*ast.FunctionExpr][]ParamSlot),
		symbolAnnotations:        make(map[Symbol]ast.TypeExpr),
		localSymbols:             make(map[*ast.LocalAssignStmt][]Symbol),
		numForSymbols:            make(map[*ast.NumberForStmt]Symbol),
		genericForSymbols:        make(map[*ast.GenericForStmt][]Symbol),
		typeValueRefs:            make(map[*ast.IdentExpr]TypeDecl),
		typeRefs:                 make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:        make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:             make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:           make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:            make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams:       make(map[ast.PositionHolder][]TypeDecl),
		methodReceiverTypes:      make(map[*ast.FunctionExpr]TypeDecl),
		qualifiedTypeRefs:        make(map[*ast.TypeRefExpr]QualifiedTypeAlias),
		qualifiedTypeRootSymbols: make(map[*ast.TypeRefExpr]Symbol),
	}
	return r
}
