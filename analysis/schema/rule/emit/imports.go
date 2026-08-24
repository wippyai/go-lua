package emit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// importSet is the emitted file's import block and the one place a Go type or
// owner symbol is spelled. A symbol issued by the package the family is
// emitted into is spelled unqualified; everything else is imported under a
// deterministic alias, so regenerating one declaration produces one file.
type importSet struct {
	self  string
	alias map[string]string
	taken map[string]struct{}
}

func newImportSet(self string) *importSet {
	return &importSet{self: self, alias: map[string]string{}, taken: map[string]struct{}{}}
}

// use registers one import path and answers the qualifier the emitted source
// spells its symbols with. The empty qualifier is the emitted package's own.
func (set *importSet) use(path string) string {
	if path == "" || path == set.self {
		return ""
	}
	if alias, present := set.alias[path]; present {
		return alias
	}
	elements := strings.Split(path, "/")
	base := elements[len(elements)-1]
	alias := base
	for index := len(elements) - 2; index >= 0; index-- {
		if _, collides := set.taken[alias]; !collides {
			break
		}
		alias = elements[index] + alias
	}
	for ordinal := 2; ; ordinal++ {
		if _, collides := set.taken[alias]; !collides {
			break
		}
		alias = fmt.Sprintf("%s%d", base, ordinal)
	}
	set.alias[path] = alias
	set.taken[alias] = struct{}{}
	return alias
}

// reserve keeps one identifier out of the alias pool. The emitted package's
// own name is reserved so an import never shadows it.
func (set *importSet) reserve(name string) {
	if name != "" {
		set.taken[name] = struct{}{}
	}
}

// typeName spells one declared Go type in the emitted file.
func (set *importSet) typeName(typ definition.GoType) string {
	name := typ.Name
	if qualifier := set.use(typ.PackagePath); qualifier != "" {
		name = qualifier + "." + name
	}
	if typ.Pointer {
		return "*" + name
	}
	return name
}

// zeroValue spells the zero value of one declared Go type. A pointer carrier's
// zero is nil; every other carrier's is its composite literal, which is what a
// refusing fold answers beside its disposition.
func (set *importSet) zeroValue(typ definition.GoType) string {
	if typ.Pointer {
		return "nil"
	}
	if typ.PackagePath == "" {
		switch typ.Name {
		case "string":
			return `""`
		case "bool":
			return "false"
		default:
			return "0"
		}
	}
	return set.typeName(typ) + "{}"
}

// call spells one direct owner symbol. receiver is the expression a
// receiver-bearing symbol is invoked on and is ignored for a free function.
func (set *importSet) call(symbol definition.GoSymbol, receiver string, args ...string) string {
	if symbol.Receiver.Name != "" {
		return receiver + "." + symbol.Name + "(" + strings.Join(args, ", ") + ")"
	}
	prefix := ""
	if qualifier := set.use(symbol.PackagePath); qualifier != "" {
		prefix = qualifier + "."
	}
	return prefix + symbol.Name + "(" + strings.Join(args, ", ") + ")"
}

// methodValue spells one receiver-bearing symbol as a method value: the
// closure a sealed carry write is handed.
func (set *importSet) methodValue(symbol definition.GoSymbol, receiver string) string {
	return receiver + "." + symbol.Name
}

// block renders the emitted file's import declaration.
func (set *importSet) block() string {
	if len(set.alias) == 0 {
		return ""
	}
	paths := make([]string, 0, len(set.alias))
	for path := range set.alias {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var out strings.Builder
	out.WriteString("import (\n")
	for _, path := range paths {
		alias := set.alias[path]
		elements := strings.Split(path, "/")
		if alias == elements[len(elements)-1] {
			fmt.Fprintf(&out, "\t%q\n", path)
			continue
		}
		fmt.Fprintf(&out, "\t%s %q\n", alias, path)
	}
	out.WriteString(")\n\n")
	return out.String()
}
