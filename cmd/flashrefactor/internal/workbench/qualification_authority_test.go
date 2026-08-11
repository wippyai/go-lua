package workbench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/generate"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

// TestSyntheticCrossPackageRejectsAuthorityForms proves that the acceptance
// fixture never turns source-level authority hazards into a partial cut. Each
// case starts from a fresh physical fixture, so a rejected prepare must leave
// exactly that case's declared source bytes intact.
func TestSyntheticCrossPackageRejectsAuthorityForms(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		prepare func(*testing.T, string, *cutplan.Intent)
	}{
		{
			name: "reflection-field-by-name",
			want: "reflect-import",
			prepare: func(t *testing.T, root string, _ *cutplan.Intent) {
				qualificationAddCoreImportAndBody(t, root, `"reflect"`, `
func reflectedCount(link Link) any {
	return reflect.ValueOf(link).FieldByName("Count")
}
`)
			},
		},
		{
			name: "method-value-and-expression",
			want: "method-value",
			prepare: func(t *testing.T, root string, intent *cutplan.Intent) {
				qualificationAppendCoreSource(t, root, `
func (Link) OldMethod() {}

func methodForms(link Link) {
	_ = link.OldMethod
	_ = Link.OldMethod
}
`)
				core := qualificationModule + "/core"
				flow := qualificationModule + "/flow"
				intent.Operations[0].Bindings = append(intent.Operations[0].Bindings, cutplan.Binding{
					Consumer: "core/link.go",
					From:     cutplan.SymbolRef{Object: core + "#type:Link/method:OldMethod"},
					To:       cutplan.SymbolRef{Object: flow + "#type:State/method:OldMethod"},
					Form:     cutplan.BindingMethodCall,
					Receiver: []cutplan.ReceiverPathStep{{
						Kind:   cutplan.ReceiverField,
						Object: cutplan.SymbolRef{Object: core + "#type:Link/field:state"},
					}},
				})
			},
		},
		{
			name: "dot-import",
			want: "dot and blank imports",
			prepare: func(t *testing.T, root string, _ *cutplan.Intent) {
				qualificationAddCoreImportAndBody(t, root, `. "strings"`, `
var _ = TrimSpace
`)
			},
		},
		{
			name: "blank-import",
			want: "dot and blank imports",
			prepare: func(t *testing.T, root string, _ *cutplan.Intent) {
				qualificationAddCoreImportAndBody(t, root, `_ "strings"`, "")
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, intent := qualificationFixture(t)
			test.prepare(t, root, &intent)
			before := qualificationSourceDigest(t, root)
			bench := qualificationBench(t, root)

			if _, err := bench.Prepare(context.Background(), intent); err == nil {
				t.Fatal("authority-bearing fixture prepared a lock")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepare error %q does not identify %q", err, test.want)
			}
			qualificationSourceUnchanged(t, before, qualificationSourceDigest(t, root), "rejected prepare")
		})
	}
}

func qualificationBench(t *testing.T, root string) Bench {
	t.Helper()
	registry, err := generate.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	bench, err := New(Config{
		Root:      root,
		Semantic:  semantic.Config{Root: root, Flashrefactor: "flashrefactor-v3"},
		Registry:  registry,
		Toolchain: cutplan.Toolchain{HelperBuild: "flashrefactor-v3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return bench
}

func qualificationAppendCoreSource(t *testing.T, root, suffix string) {
	t.Helper()
	path := filepath.Join(root, "core", "link.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	qualificationWrite(t, root, "core/link.go", string(data)+suffix)
}

func qualificationAddCoreImportAndBody(t *testing.T, root, imported, body string) {
	t.Helper()
	path := filepath.Join(root, "core", "link.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const packageClause = "package core\n"
	source := string(data)
	if !strings.HasPrefix(source, packageClause) {
		t.Fatalf("core source lost package clause: %q", source)
	}
	source = strings.Replace(source, packageClause, packageClause+"\nimport "+imported+"\n", 1)
	qualificationWrite(t, root, "core/link.go", source+body)
}
