package generate

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// NewRegistry validates and copies an explicit finite provider set.  It has
// no global registry: construction is the only point at which providers can
// enter the authority boundary.
func NewRegistry(values []Provider) (Registry, error) {
	providers := make(map[cutplan.Provider]provider, len(values))
	identities := make(map[string]bool, len(values))
	for _, value := range values {
		if !safeName(value.Name) {
			return Registry{}, fmt.Errorf("invalid provider name %q", value.Name)
		}
		if !safeIdentity(value.Identity) {
			return Registry{}, fmt.Errorf("invalid provider identity for %q", value.Name)
		}
		if value.Render == nil {
			return Registry{}, fmt.Errorf("provider %q has no renderer", value.Name)
		}
		if _, exists := providers[value.Name]; exists {
			return Registry{}, fmt.Errorf("duplicate provider name %q", value.Name)
		}
		if identities[value.Identity] {
			return Registry{}, fmt.Errorf("duplicate provider identity %q", value.Identity)
		}
		providers[value.Name] = provider{name: value.Name, identity: value.Identity, render: value.Render}
		identities[value.Identity] = true
	}
	return Registry{providers: providers}, nil
}

// Render validates the request against exactly one cutplan Generate edit, then
// runs the registered renderer twice with separate deep copies.  It returns no
// partial output on failure, so a caller has nothing unsafe to write.
func (registry Registry) Render(edit cutplan.Generate, inputs []Input) (Result, error) {
	if !safeName(edit.Provider) {
		return Result{}, fmt.Errorf("invalid provider name %q", edit.Provider)
	}
	if !safePath(edit.Destination) {
		return Result{}, fmt.Errorf("invalid destination %q", edit.Destination)
	}
	provider, exists := registry.providers[edit.Provider]
	if !exists {
		return Result{}, fmt.Errorf("unknown provider %q", edit.Provider)
	}
	request, err := declaredRequest(edit, inputs)
	if err != nil {
		return Result{}, err
	}
	first, err := render(provider, request)
	if err != nil {
		return Result{}, err
	}
	second, err := render(provider, request)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(first, second) {
		return Result{}, fmt.Errorf("provider %q is nondeterministic", edit.Provider)
	}
	return Result{
		Evidence: cutplan.ProviderEvidence{Name: provider.name, Identity: provider.identity},
		Bytes:    append([]byte(nil), first...),
	}, nil
}

func declaredRequest(edit cutplan.Generate, inputs []Input) (Request, error) {
	if len(edit.Inputs) != len(inputs) {
		return Request{}, fmt.Errorf("provider %q received %d inputs; edit declares %d", edit.Provider, len(inputs), len(edit.Inputs))
	}
	request := Request{Destination: Destination{Path: edit.Destination}, Inputs: make([]Input, len(inputs))}
	seen := make(map[string]bool, len(inputs))
	for index, declared := range edit.Inputs {
		if !safePath(declared) {
			return Request{}, fmt.Errorf("invalid declared input %q", declared)
		}
		if seen[declared] {
			return Request{}, fmt.Errorf("duplicate declared input %q", declared)
		}
		seen[declared] = true
		if inputs[index].Path != declared {
			return Request{}, fmt.Errorf("input %d is %q; edit declares %q", index, inputs[index].Path, declared)
		}
		request.Inputs[index] = Input{Path: declared, Bytes: append([]byte(nil), inputs[index].Bytes...)}
	}
	return request, nil
}

func render(provider provider, request Request) (output []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			output = nil
			err = fmt.Errorf("provider %q panicked: %v", provider.name, recovered)
		}
	}()
	output, err = provider.render(copyRequest(request))
	if err != nil {
		return nil, fmt.Errorf("provider %q: %w", provider.name, err)
	}
	return append([]byte(nil), output...), nil
}

func copyRequest(value Request) Request {
	copyValue := Request{Destination: value.Destination, Inputs: make([]Input, len(value.Inputs))}
	for index, input := range value.Inputs {
		copyValue.Inputs[index] = Input{Path: input.Path, Bytes: append([]byte(nil), input.Bytes...)}
	}
	return copyValue
}

func safeName(value cutplan.Provider) bool { return safeAtom(string(value)) }

func safeIdentity(value string) bool { return safeAtom(value) }

// safePath mirrors cutplan's repository-relative path constraint without
// importing its private validation helpers.
func safePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsAny(value, "*?[]{}\x00\n\r\t") || strings.HasPrefix(value, "/") {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}

func safeAtom(value string) bool {
	if value == "" || strings.ContainsAny(value, " /\\;|&`$*?[]{}\x00\n\r\t") {
		return false
	}
	for _, character := range value {
		if !unicode.IsPrint(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
