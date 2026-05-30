package canonical

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/types/typ"
)

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
