package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/wippyai/go-lua/analysis/embedding"
)

// DocumentCodec owns URI/document identity translation. Product hosts can
// replace FileDocumentCodec with a registry-aware codec without changing the
// server's overlay, scheduler, or checker calls.
type DocumentCodec interface {
	DocumentForURI(string) (embedding.DocumentID, error)
	URIForDocument(embedding.DocumentID) (string, error)
}

// FileDocumentCodec is deliberately strict: it accepts only canonical local
// file URIs, making URI <-> DocumentID a bijection. It performs no filesystem
// normalization, symlink lookup, case folding, or I/O.
type FileDocumentCodec struct{}

func (FileDocumentCodec) DocumentForURI(raw string) (embedding.DocumentID, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return embedding.DocumentID{}, fmt.Errorf("lsp: invalid document URI: %w", err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return embedding.DocumentID{}, errors.New("lsp: only canonical local file URIs are supported")
	}
	document := embedding.FileDocument(parsed.Path)
	canonical, err := (FileDocumentCodec{}).URIForDocument(document)
	if err != nil {
		return embedding.DocumentID{}, err
	}
	if raw != canonical {
		return embedding.DocumentID{}, fmt.Errorf("lsp: non-canonical file URI %q; use %q", raw, canonical)
	}
	return document, nil
}

func (FileDocumentCodec) URIForDocument(document embedding.DocumentID) (string, error) {
	if document.Scheme != embedding.DocumentSchemeFile || document.OpaqueKey == "" || !strings.HasPrefix(document.OpaqueKey, "/") {
		return "", fmt.Errorf("lsp: cannot encode non-absolute file document %q", document)
	}
	return (&url.URL{Scheme: "file", Path: document.OpaqueKey}).String(), nil
}
