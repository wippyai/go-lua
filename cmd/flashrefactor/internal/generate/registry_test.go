package generate

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func TestRenderBindsExactDeclaredBytesAndProviderEvidence(t *testing.T) {
	registry := testRegistry(t, Provider{
		Name: "fixture", Identity: "fixture-v1",
		Render: func(request Request) ([]byte, error) {
			if request.Destination.Path != "generated/out.go" {
				t.Fatalf("destination = %q", request.Destination.Path)
			}
			if len(request.Inputs) != 2 || request.Inputs[0].Path != "source/a.go" || request.Inputs[1].Path != "source/b.go" {
				t.Fatalf("inputs = %#v", request.Inputs)
			}
			request.Inputs[0].Bytes[0] = 'X' // Must not corrupt the second invocation.
			return append(append([]byte(nil), request.Inputs[0].Bytes...), request.Inputs[1].Bytes...), nil
		},
	})
	edit := cutplan.Generate{Provider: "fixture", Inputs: []string{"source/a.go", "source/b.go"}, Destination: "generated/out.go"}
	inputs := []Input{{Path: "source/a.go", Bytes: []byte("a")}, {Path: "source/b.go", Bytes: []byte("b")}}
	result, err := registry.Render(edit, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result.Bytes), "Xb"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got, want := result.Evidence, (cutplan.ProviderEvidence{Name: "fixture", Identity: "fixture-v1"}); got != want {
		t.Fatalf("evidence = %#v, want %#v", got, want)
	}
	if got, want := string(inputs[0].Bytes), "a"; got != want {
		t.Fatalf("renderer mutated caller input: %q", got)
	}
}

func TestRenderFailsClosedOnRegistryAndRequestViolations(t *testing.T) {
	render := func(Request) ([]byte, error) { return []byte("ok"), nil }
	for _, providers := range [][]Provider{
		{{Name: "same", Identity: "one", Render: render}, {Name: "same", Identity: "two", Render: render}},
		{{Name: "first", Identity: "one", Render: render}, {Name: "second", Identity: "one", Render: render}},
		{{Name: "bad name", Identity: "one", Render: render}},
		{{Name: "valid", Identity: "bad identity", Render: render}},
		{{Name: "valid", Identity: "one"}},
	} {
		if _, err := NewRegistry(providers); err == nil {
			t.Fatalf("invalid registry accepted: %#v", providers)
		}
	}
	registry := testRegistry(t, Provider{Name: "known", Identity: "v1", Render: render})
	for _, test := range []struct {
		name   string
		edit   cutplan.Generate
		inputs []Input
	}{
		{"unknown", cutplan.Generate{Provider: "missing", Destination: "out.go"}, nil},
		{"destination", cutplan.Generate{Provider: "known", Destination: "../out.go"}, nil},
		{"count", cutplan.Generate{Provider: "known", Inputs: []string{"a.go"}, Destination: "out.go"}, nil},
		{"path", cutplan.Generate{Provider: "known", Inputs: []string{"a.go"}, Destination: "out.go"}, []Input{{Path: "b.go"}}},
		{"duplicate", cutplan.Generate{Provider: "known", Inputs: []string{"a.go", "a.go"}, Destination: "out.go"}, []Input{{Path: "a.go"}, {Path: "a.go"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := registry.Render(test.edit, test.inputs); err == nil {
				t.Fatal("invalid render accepted")
			}
		})
	}
}

func TestRenderFailsClosedOnNondeterminismPanicAndError(t *testing.T) {
	sequence := 0
	nondeterministic := testRegistry(t, Provider{Name: "nondeterministic", Identity: "v1", Render: func(Request) ([]byte, error) {
		sequence++
		return []byte{byte(sequence)}, nil
	}})
	edit := cutplan.Generate{Provider: "nondeterministic", Destination: "out.go"}
	if _, err := nondeterministic.Render(edit, nil); err == nil || !strings.Contains(err.Error(), "nondeterministic") {
		t.Fatalf("nondeterministic provider accepted: %v", err)
	}
	panicRegistry := testRegistry(t, Provider{Name: "panic", Identity: "v1", Render: func(Request) ([]byte, error) { panic("boom") }})
	if _, err := panicRegistry.Render(cutplan.Generate{Provider: "panic", Destination: "out.go"}, nil); err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("panic accepted: %v", err)
	}
	errorRegistry := testRegistry(t, Provider{Name: "error", Identity: "v1", Render: func(Request) ([]byte, error) { return nil, errors.New("bad") }})
	if _, err := errorRegistry.Render(cutplan.Generate{Provider: "error", Destination: "out.go"}, nil); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error accepted: %v", err)
	}
}

func testRegistry(t *testing.T, values ...Provider) Registry {
	t.Helper()
	registry, err := NewRegistry(values)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
