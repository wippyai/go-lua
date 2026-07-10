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

func defaultInitializeResult() initializeResult {
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
