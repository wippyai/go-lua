// Package completion provides code completion for LSP.
package completion

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/lsp"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/typ"
)

// Kind represents completion item kind (matches LSP CompletionItemKind).
type Kind int

const (
	KindText Kind = iota + 1
	KindMethod
	KindFunction
	KindConstructor
	KindField
	KindVariable
	KindClass
	KindInterface
	KindModule
	KindProperty
	KindUnit
	KindValue
	KindEnum
	KindKeyword
	KindSnippet
	KindColor
	KindFile
	KindReference
	KindFolder
	KindEnumMember
	KindConstant
	KindStruct
	KindEvent
	KindOperator
	KindTypeParameter
)

// Item represents a completion suggestion.
type Item struct {
	Label         string   // display text
	Kind          Kind     // item kind
	Detail        string   // type signature
	Documentation string   // doc comment
	InsertText    string   // text to insert (may differ from label)
	IsSnippet     bool     // if InsertText contains $1, $2 placeholders
	SortText      string   // for ordering
	FilterText    string   // for filtering
	Deprecated    bool     // whether this item is deprecated
	Type          typ.Type // the type of the completed item
}

// TriggerKind describes how completion was triggered.
type TriggerKind int

const (
	TriggerInvoked TriggerKind = iota + 1
	TriggerCharacter
	TriggerIncomplete
)

// ContextKind describes what kind of completion is needed.
type ContextKind int

const (
	ContextUnknown    ContextKind = iota
	ContextIdentifier             // local variable, function name
	ContextMember                 // after "." - field/method access
	ContextKeyword                // language keywords
)

// Context describes the completion context.
type Context struct {
	File        string
	Line        int
	Col         int
	Trigger     TriggerKind
	TriggerChar string
	Prefix      string      // text before cursor on current "word"
	Kind        ContextKind // detected context kind
}

// TypeFormatter formats a type for display.
type TypeFormatter func(any) string

// Provider handles completion requests.
type Provider struct {
	symbols    *index.SymbolIndex
	formatType TypeFormatter
}

// NewProvider creates a completion provider.
func NewProvider(symbols *index.SymbolIndex) *Provider {
	return &Provider{symbols: symbols}
}

// SetTypeFormatter sets a custom type formatter for completion details.
func (p *Provider) SetTypeFormatter(f TypeFormatter) {
	p.formatType = f
}

// Complete returns completion items for the given context.
func (p *Provider) Complete(ctx *Context) []Item {
	if ctx == nil {
		return nil
	}

	var items []Item

	switch ctx.Kind {
	case ContextKeyword:
		items = p.keywordCompletions(ctx.Prefix)
	case ContextMember:
		items = p.memberCompletions(ctx)
	default:
		items = p.identifierCompletions(ctx)
	}

	// Sort items by relevance
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortText != "" && items[j].SortText != "" {
			return items[i].SortText < items[j].SortText
		}
		return items[i].Label < items[j].Label
	})

	return items
}

// identifierCompletions returns completions for identifiers (variables, functions).
func (p *Provider) identifierCompletions(ctx *Context) []Item {
	var items []Item
	prefix := strings.ToLower(ctx.Prefix)

	// Get all symbols in the file
	if p.symbols != nil {
		syms := p.symbols.SymbolsInFile(ctx.File)
		for _, sym := range syms {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), prefix) {
				continue
			}

			item := p.symbolToItem(sym)
			item.SortText = "a" + sym.Name // sort symbols first
			items = append(items, item)
		}
	}

	// Add keywords if prefix matches
	for _, kw := range lsp.LuaKeywords {
		if prefix == "" || strings.HasPrefix(kw, prefix) {
			items = append(items, Item{
				Label:    kw,
				Kind:     KindKeyword,
				SortText: "z" + kw, // sort keywords last
			})
		}
	}

	return items
}

// memberCompletions returns completions for member access (after ".").
func (p *Provider) memberCompletions(ctx *Context) []Item {
	// Without AST access, we can't determine the type before the dot
	// Return field symbols as a fallback
	if p.symbols == nil {
		return nil
	}

	var items []Item
	prefix := strings.ToLower(ctx.Prefix)

	syms := p.symbols.SymbolsInFile(ctx.File)
	for _, sym := range syms {
		if sym.Kind != index.SymbolField {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), prefix) {
			continue
		}
		items = append(items, p.symbolToItem(sym))
	}

	return items
}

// keywordCompletions returns Lua keyword completions.
func (p *Provider) keywordCompletions(prefix string) []Item {
	var items []Item
	prefix = strings.ToLower(prefix)

	for _, kw := range lsp.LuaKeywords {
		if prefix == "" || strings.HasPrefix(kw, prefix) {
			items = append(items, Item{
				Label: kw,
				Kind:  KindKeyword,
			})
		}
	}

	return items
}

// symbolToItem converts a symbol to a completion item.
func (p *Provider) symbolToItem(sym *index.Symbol) Item {
	item := Item{
		Label:      sym.Name,
		Kind:       symbolKindToCompletionKind(sym.Kind),
		FilterText: sym.Name,
	}

	if sym.Type != nil {
		if t, ok := sym.Type.(typ.Type); ok {
			item.Type = t
			if p.formatType != nil {
				item.Detail = p.formatType(t)
			} else {
				item.Detail = t.String()
			}
		}
	}

	return item
}

// symbolKindToCompletionKind maps symbol kinds to completion kinds.
func symbolKindToCompletionKind(kind index.SymbolKind) Kind {
	switch kind {
	case index.SymbolFunction:
		return KindFunction
	case index.SymbolMethod:
		return KindMethod
	case index.SymbolVariable:
		return KindVariable
	case index.SymbolParameter:
		return KindVariable
	case index.SymbolField:
		return KindField
	case index.SymbolType:
		return KindClass
	default:
		return KindVariable
	}
}

// ResolveItem adds detail to a completion item (lazy loading).
func (p *Provider) ResolveItem(item *Item) *Item {
	if item == nil {
		return nil
	}
	// Currently all info is populated upfront
	return item
}
