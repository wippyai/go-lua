package lsp

import "encoding/json"

const (
	methodInitialize            = "initialize"
	methodInitialized           = "initialized"
	methodShutdown              = "shutdown"
	methodExit                  = "exit"
	methodDidOpen               = "textDocument/didOpen"
	methodDidChange             = "textDocument/didChange"
	methodDidClose              = "textDocument/didClose"
	methodPublishDiagnostics    = "textDocument/publishDiagnostics"
	methodPullDiagnostics       = "textDocument/diagnostic"
	methodHover                 = "textDocument/hover"
	methodDefinition            = "textDocument/definition"
	methodReferences            = "textDocument/references"
	methodDocumentHighlight     = "textDocument/documentHighlight"
	methodDocumentSymbol        = "textDocument/documentSymbol"
	methodPrepareRename         = "textDocument/prepareRename"
	methodRename                = "textDocument/rename"
	methodCodeAction            = "textDocument/codeAction"
	methodSemanticTokensFull    = "textDocument/semanticTokens/full"
	textDocumentSyncIncremental = 2
)

// Position is an LSP UTF-16 position. Lines and characters are zero-indexed.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int64  `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int64  `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength *int   `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

type didOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type pullDiagnosticsParams struct {
	TextDocument     TextDocumentIdentifier `json:"textDocument"`
	PreviousResultID string                 `json:"previousResultId,omitempty"`
}

type hoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type textDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type referencesParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type documentHighlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind,omitempty"`
}

type documentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type renameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type workspaceTextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// textDocumentEdit is a version-fenced LSP WorkspaceEdit entry. Mutating
// edits must use this shape rather than the unversioned changes map so the
// client can reject an edit computed for an older overlay.
type textDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []workspaceTextEdit             `json:"edits"`
}

type workspaceEdit struct {
	DocumentChanges []textDocumentEdit `json:"documentChanges,omitempty"`
}

type codeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      codeActionContext      `json:"context"`
}

type codeActionContext struct {
	Only []string `json:"only,omitempty"`
}

type codeAction struct {
	Title string        `json:"title"`
	Kind  string        `json:"kind,omitempty"`
	Edit  workspaceEdit `json:"edit"`
}

type semanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// semanticTokensResult is the LSP full-token wire shape. Data is a stream of
// five-element uint32 tuples: deltaLine, deltaStart, length, tokenType, and
// tokenModifier bitset.
type semanticTokensResult struct {
	Data []uint32 `json:"data"`
}

type diagnosticRelatedInformation struct {
	Location Location `json:"location"`
	Message  string   `json:"message"`
}

type protocolDiagnostic struct {
	Range              Range                          `json:"range"`
	Severity           int                            `json:"severity,omitempty"`
	Code               string                         `json:"code,omitempty"`
	Source             string                         `json:"source,omitempty"`
	Message            string                         `json:"message"`
	RelatedInformation []diagnosticRelatedInformation `json:"relatedInformation,omitempty"`
	Data               any                            `json:"data,omitempty"`
}

type publishDiagnosticsParams struct {
	URI string `json:"uri"`
	// Version is deliberately required by this server. LSP permits clients to
	// use version zero, so omitempty would make a valid, tagged publication
	// appear unversioned on the wire.
	Version     int64                `json:"version"`
	Diagnostics []protocolDiagnostic `json:"diagnostics"`
}

type diagnosticReport struct {
	Kind     string               `json:"kind"`
	ResultID string               `json:"resultId,omitempty"`
	Items    []protocolDiagnostic `json:"items"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

// initializeParams contains only the client capabilities used to select a
// protocol response shape. Unknown capabilities remain forward-compatible.
type initializeParams struct {
	Capabilities clientCapabilities `json:"capabilities"`
}

type clientCapabilities struct {
	Workspace struct {
		WorkspaceEdit struct {
			DocumentChanges bool `json:"documentChanges"`
		} `json:"workspaceEdit"`
	} `json:"workspace"`
	TextDocument struct {
		Rename struct {
			PrepareSupport bool `json:"prepareSupport"`
		} `json:"rename"`
	} `json:"textDocument"`
}

type serverInfo struct {
	Name string `json:"name"`
}

// serverCapabilities intentionally lists only Wave 2A/2B-ready behavior.
// Fields not present are not advertised.
type serverCapabilities struct {
	TextDocumentSync          textDocumentSyncOptions `json:"textDocumentSync"`
	DiagnosticProvider        diagnosticProvider      `json:"diagnosticProvider"`
	HoverProvider             bool                    `json:"hoverProvider"`
	DefinitionProvider        bool                    `json:"definitionProvider"`
	ReferencesProvider        bool                    `json:"referencesProvider"`
	DocumentHighlightProvider bool                    `json:"documentHighlightProvider"`
	DocumentSymbolProvider    bool                    `json:"documentSymbolProvider"`
	RenameProvider            any                     `json:"renameProvider"`
	CodeActionProvider        bool                    `json:"codeActionProvider"`
	SemanticTokensProvider    semanticTokensOptions   `json:"semanticTokensProvider"`
}

type renameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

type textDocumentSyncOptions struct {
	OpenClose bool `json:"openClose"`
	Change    int  `json:"change"`
}

type diagnosticProvider struct {
	Identifier            string `json:"identifier"`
	InterFileDependencies bool   `json:"interFileDependencies"`
	WorkspaceDiagnostics  bool   `json:"workspaceDiagnostics"`
}

type semanticTokensOptions struct {
	Legend semanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
	Range  bool                 `json:"range"`
}

type semanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

var semanticTokenLegend = semanticTokensLegend{
	TokenTypes: []string{
		"variable",
		"parameter",
		"function",
	},
	// These are the two go-lua semantic extensions. "placement" denotes an
	// allocation site licensed as frame-local or decomposable; the exact class
	// stays in the service PlacementPlan rather than becoming LSP-local logic.
	TokenModifiers: []string{
		"typestate-tracked",
		"placement",
	},
}

func defaultInitializeResult(client clientCapabilities) initializeResult {
	renameProvider := any(true)
	if client.TextDocument.Rename.PrepareSupport {
		renameProvider = renameOptions{PrepareProvider: true}
	}
	return initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync: textDocumentSyncOptions{OpenClose: true, Change: textDocumentSyncIncremental},
			DiagnosticProvider: diagnosticProvider{
				Identifier:            "go-lua",
				InterFileDependencies: false,
				WorkspaceDiagnostics:  false,
			},
			HoverProvider:             true,
			DefinitionProvider:        true,
			ReferencesProvider:        true,
			DocumentHighlightProvider: true,
			DocumentSymbolProvider:    true,
			RenameProvider:            renameProvider,
			CodeActionProvider:        true,
			SemanticTokensProvider: semanticTokensOptions{
				Legend: semanticTokenLegend,
				Full:   true,
				Range:  false,
			},
		},
		ServerInfo: serverInfo{Name: "go-lua-lsp"},
	}
}

func decodeParams(data json.RawMessage, target any) error {
	if len(data) == 0 || string(data) == "null" {
		return json.Unmarshal([]byte("{}"), target)
	}
	return json.Unmarshal(data, target)
}
