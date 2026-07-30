package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// Options configures lexical binding.
type Options struct {
	Globals []string
}

// Result records lexical declaration identities for identifier occurrences.
type Result struct {
	identSymbols          map[*ast.IdentExpr]ID
	readIdents            map[ID][]*ast.IdentExpr
	writeIdents           map[ID][]*ast.IdentExpr
	declarations          map[ID]Declaration
	occurrences           map[ID][]Occurrence
	implicitGlobalUses    map[*ast.IdentExpr]struct{}
	implicitGlobalSymbols map[ID]struct{}
	typeValueRefs         map[*ast.IdentExpr]TypeDecl

	directCalls            map[ID][]*ast.FuncCallExpr
	runtimeUseScanComplete bool

	nextSymbolID ID

	names map[ID]string
	kinds map[ID]Kind

	globals map[string]globalSymbol

	functionSymbols   map[*ast.FunctionExpr]ID
	functionsBySymbol map[ID]*ast.FunctionExpr
	functions         []*ast.FunctionExpr
	nestedFunctions   map[*ast.FunctionExpr][]*ast.FunctionExpr
	functionOrigins   map[*ast.FunctionExpr]FunctionOrigin
	// functionTargetIndex maps a value binding to the sole function identity
	// introduced for that binding. A present zero value is an ambiguity marker.
	// It is built once with the function-origin table and is never exposed.
	functionTargetIndex map[ID]ID
	functionIndex       map[*ast.FunctionExpr]int
	functionSubtreeEnd  map[*ast.FunctionExpr]int
	declaringFunctions  map[ID]*ast.FunctionExpr
	directCaptures      map[*ast.FunctionExpr][]Capture
	directCaptureSeen   map[*ast.FunctionExpr]map[ID]struct{}
	directGlobalReads   map[*ast.FunctionExpr][]ID
	directGlobalSeen    map[*ast.FunctionExpr]map[ID]struct{}
	chunkGlobalReads    []ID
	chunkGlobalSeen     map[ID]struct{}

	paramSymbols      map[*ast.FunctionExpr][]ID
	varargSymbols     map[*ast.FunctionExpr]ID
	paramSlots        map[*ast.FunctionExpr][]ParamSlot
	localSymbols      map[*ast.LocalAssignStmt][]ID
	numForSymbols     map[*ast.NumberForStmt]ID
	genericForSymbols map[*ast.GenericForStmt][]ID

	gotoTargets   map[*ast.GotoStmt]*ast.LabelStmt
	controlIssues []ControlIssue

	nextTypeDeclID       TypeDeclID
	typeRefs             map[*ast.TypeRefExpr]TypeDecl
	primitiveTypeRefs    map[*ast.PrimitiveTypeExpr]TypeDecl
	typeDefDecls         map[*ast.TypeDefStmt]TypeDecl
	interfaceDecls       map[*ast.InterfaceDefStmt]TypeDecl
	typeDefParams        map[*ast.TypeDefStmt][]TypeDecl
	functionTypeParams   map[*ast.FunctionExpr][]TypeDecl
	methodReceiverTypes  map[*ast.FunctionExpr]TypeDecl
	assertedParams       map[*ast.AssertsTypeExpr]int
	qualifiedTypeRefs    map[*ast.TypeRefExpr]QualifiedTypeAlias
	qualifiedTypeAliases map[qualifiedTypeAliasKey]QualifiedTypeAlias
	// qualifiedTypeRoots records value-namespace roots used only by qualified
	// type references in a function (for example protocol.User where protocol
	// is an outer local initialized by require("protocol")).
	qualifiedTypeRoots map[*ast.FunctionExpr]map[string]ID
}

func newResult(opts Options) *Result {
	r := &Result{
		identSymbols:          make(map[*ast.IdentExpr]ID),
		readIdents:            make(map[ID][]*ast.IdentExpr),
		writeIdents:           make(map[ID][]*ast.IdentExpr),
		declarations:          make(map[ID]Declaration),
		occurrences:           make(map[ID][]Occurrence),
		implicitGlobalUses:    make(map[*ast.IdentExpr]struct{}),
		implicitGlobalSymbols: make(map[ID]struct{}),
		typeValueRefs:         make(map[*ast.IdentExpr]TypeDecl),
		names:                 make(map[ID]string),
		kinds:                 make(map[ID]Kind),
		globals:               make(map[string]globalSymbol),
		functionSymbols:       make(map[*ast.FunctionExpr]ID),
		functionsBySymbol:     make(map[ID]*ast.FunctionExpr),
		nestedFunctions:       make(map[*ast.FunctionExpr][]*ast.FunctionExpr),
		functionOrigins:       make(map[*ast.FunctionExpr]FunctionOrigin),
		functionTargetIndex:   make(map[ID]ID),
		functionIndex:         make(map[*ast.FunctionExpr]int),
		functionSubtreeEnd:    make(map[*ast.FunctionExpr]int),
		declaringFunctions:    make(map[ID]*ast.FunctionExpr),
		directCaptures:        make(map[*ast.FunctionExpr][]Capture),
		directCaptureSeen:     make(map[*ast.FunctionExpr]map[ID]struct{}),
		directGlobalReads:     make(map[*ast.FunctionExpr][]ID),
		directGlobalSeen:      make(map[*ast.FunctionExpr]map[ID]struct{}),
		chunkGlobalSeen:       make(map[ID]struct{}),
		paramSymbols:          make(map[*ast.FunctionExpr][]ID),
		varargSymbols:         make(map[*ast.FunctionExpr]ID),
		paramSlots:            make(map[*ast.FunctionExpr][]ParamSlot),
		localSymbols:          make(map[*ast.LocalAssignStmt][]ID),
		numForSymbols:         make(map[*ast.NumberForStmt]ID),
		genericForSymbols:     make(map[*ast.GenericForStmt][]ID),
		typeRefs:              make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:     make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:          make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:        make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:         make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams:    make(map[*ast.FunctionExpr][]TypeDecl),
		methodReceiverTypes:   make(map[*ast.FunctionExpr]TypeDecl),
		qualifiedTypeRefs:     make(map[*ast.TypeRefExpr]QualifiedTypeAlias),
		qualifiedTypeAliases:  make(map[qualifiedTypeAliasKey]QualifiedTypeAlias),
		qualifiedTypeRoots:    make(map[*ast.FunctionExpr]map[string]ID),
	}
	r.runtimeUseScanComplete = true
	for _, name := range normalizeNames(opts.Globals) {
		r.global(name, true)
	}
	return r
}
