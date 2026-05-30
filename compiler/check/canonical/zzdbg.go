package canonical

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// zzScopeDbg reports whether the lexical-scope debug probe is active (ZZSCOPE set).
func zzScopeDbg() bool { return os.Getenv("ZZSCOPE") != "" }

// zzScopef prints a lexical-scope probe line when ZZSCOPE is set.
func zzScopef(format string, args ...any) {
	if !zzScopeDbg() {
		return
	}
	fmt.Printf("[ZZSCOPE] "+format+"\n", args...)
}

// zzDumpCaptures prints every captured record value carrying a metatable when the
// ZZCLASS env var is set. Debug probe for class-instance data-field enrichment.
func zzDumpCaptures(out map[cfg.SymbolID]typ.Type) {
	if os.Getenv("ZZCLASS") == "" {
		return
	}
	for sym, t := range out {
		rec, ok := t.(*typ.Record)
		if !ok || rec.Metatable == nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "[ZZ capture-meta sym=%d] %s\n", uint64(sym), t.String())
	}
}

// zdbg prints a formatted trace line to stderr when the ZSIB env var is set.
// Debug probe for sibling-correlation / NoReturn bind derivation.
func zdbg(format string, args ...interface{}) {
	if os.Getenv("ZSIB") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[ZSIB] "+format+"\n", args...)
}

// zcap prints a formatted trace line to stderr when the ZCAP env var is set.
// Debug probe for upvalue/module-capture Env seeding.
func zcap(format string, args ...interface{}) {
	if os.Getenv("ZCAP") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[ZCAP] "+format+"\n", args...)
}

// zzDumpType prints recursive-family identity details for a resolved type when
// the ZZCLASS env var is set. Debug probe for cross-module class identity.
func zzDumpType(label string, t typ.Type) {
	if os.Getenv("ZZCLASS") == "" || t == nil {
		return
	}
	zzDumpTypeInner(label, t, 0)
}

func zzDumpTypeInner(label string, t typ.Type, depth int) {
	if depth > 3 || t == nil {
		return
	}
	switch v := t.(type) {
	case *typ.Recursive:
		fk, keyed := typ.FamilyKeyOf(v)
		body := "<nil>"
		if v.Body != nil {
			body = v.Body.String()
		}
		fmt.Fprintf(os.Stderr, "[ZZ %s] Recursive ptr=%p name=%q keyed=%v key=%v body=%s\n",
			label, v, v.Name, keyed, fk, body)
	case *typ.Alias:
		fmt.Fprintf(os.Stderr, "[ZZ %s] Alias -> %s\n", label, v.UnaliasedTarget().String())
		zzDumpTypeInner(label+"/target", v.UnaliasedTarget(), depth+1)
	default:
		fmt.Fprintf(os.Stderr, "[ZZ %s] %T: %s\n", label, t, t.String())
	}
}
