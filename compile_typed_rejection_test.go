package lua

import (
	"strings"
	"testing"
)

func TestTemporaryCompilerRejectsTypedSyntax(t *testing.T) {
	tests := map[string]string{
		"type definition":    `type User = {name: string}`,
		"interface":          "interface Serializable\nfunction serialize(self: any): string\nend",
		"local annotation":   `local value: number = 1`,
		"function signature": `function identity<T>(value: T): T return value end`,
		"vararg annotation":  `local function collect(...: string) return ... end`,
		"cast":               `local value = source as string`,
		"non-nil assertion":  `local value = source!`,
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := CompileString(source, name); err == nil {
				t.Fatal("CompileString accepted typed syntax")
			} else if !strings.Contains(err.Error(), "typed syntax is not supported by this compiler") {
				t.Fatalf("CompileString error = %q, want explicit typed-syntax rejection", err)
			}
		})
	}
}
