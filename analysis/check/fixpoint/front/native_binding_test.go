package front

import (
	"strings"
	"testing"
)

func nativeOpenEffectCount(t *testing.T, source string) int {
	t.Helper()
	compilation, err := Compile(source)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	count := 0
	for _, contract := range compilation.NativeContracts {
		if contract.Family == "effect_row" && contract.Value == "exhaustive=false" {
			count++
		}
	}
	return count
}

func TestNativeStdlibRecognitionRequiresGlobalBinding(t *testing.T) {
	tests := []struct {
		name   string
		global string
		shadow string
	}{
		{
			name: "os.clock",
			global: `
local function sample(): number
    return os.clock()
end
return sample`,
			shadow: `
local os = { clock = function(): number return 1 end }
local function sample(): number
    return os.clock()
end
return sample`,
		},
		{
			name: "pcall",
			global: `
local function protect(callback)
    return pcall(callback)
end
return protect`,
			shadow: `
local pcall = function(callback) return callback() end
local function protect(callback)
    return pcall(callback)
end
return protect`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeOpenEffectCount(t, test.global); got == 0 {
				t.Fatal("bound stdlib operation published no open effect")
			}
			if got := nativeOpenEffectCount(t, test.shadow); got != 0 {
				t.Fatalf("shadowed spelling published %d open stdlib effects", got)
			}
		})
	}
}

func TestNativeMethodRecognitionRequiresRegistryEntry(t *testing.T) {
	compilation, err := Compile(`
local function registered(value: string): string
    return value:upper()
end
local function unregistered(value: string): string
    return value:not_a_stdlib_method()
end
return registered, unregistered`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var allocationRows int
	for _, contract := range compilation.NativeContracts {
		if contract.Family == "effect_row" && strings.Contains(contract.Value, "allocation=present") {
			allocationRows++
		}
	}
	if allocationRows != 1 {
		t.Fatalf("registered method allocation rows = %d, want one", allocationRows)
	}
}
