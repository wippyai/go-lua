package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanOptions configures a Scanner.
type ScanOptions struct {
	Seen ScanSeen

	// MaxEnters limits distinct nodes accepted by Enter after seen pruning.
	MaxEnters int
}

// Scanner holds reusable traversal state for recursive type scans.
type Scanner struct {
	seen      ScanSeen
	maxEnters int
	enters    int
}

// NewScanner creates a traversal scanner.
func NewScanner(opts ScanOptions) *Scanner {
	return &Scanner{
		seen:      opts.Seen,
		maxEnters: opts.MaxEnters,
	}
}

// Enter applies the scanner's seen and unique-node budget policies.
func (s *Scanner) Enter(t typ.Type) bool {
	if s == nil {
		return true
	}
	if s.seen != nil {
		if s.seen.Contains(t) {
			return false
		}
	}
	if s.maxEnters > 0 && s.enters >= s.maxEnters {
		return false
	}
	if s.seen != nil {
		s.seen.Remember(t)
	}
	s.enters++
	return true
}

// WalkChildren visits the canonical child type slots of t in stable order.
// It returns true when visit reports a match and traversal should stop.
func WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	if visit == nil || t == nil {
		return false
	}
	visitType := func(child typ.Type) bool {
		return child != nil && visit(child)
	}
	t = unwrapTransparent(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *typ.Optional:
		return visitType(n.Inner)
	case *typ.Union:
		return walkEachType(n.Members, visitType)
	case *typ.Intersection:
		return walkEachType(n.Members, visitType)
	case *typ.Array:
		return visitType(n.Element)
	case *typ.Map:
		return visitType(n.Key) || visitType(n.Value)
	case *typ.ReadonlyMap:
		return visitType(n.Key) || visitType(n.Value)
	case *typ.Tuple:
		return walkEachType(n.Elements, visitType)
	case *typ.Function:
		for _, param := range n.TypeParams {
			if param != nil && visitType(param.Constraint) {
				return true
			}
		}
		for _, param := range n.Params {
			if visitType(param.Type) {
				return true
			}
		}
		if visitType(n.Variadic) {
			return true
		}
		for _, ret := range n.Returns {
			if visitType(ret) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range n.Fields {
			if visitType(field.Type) {
				return true
			}
		}
		for _, member := range n.StaticMembers {
			if visitType(member.Type) {
				return true
			}
		}
		if visitType(n.Metatable) {
			return true
		}
		if n.HasMapComponent() {
			if visitType(n.MapKey) || visitType(n.MapValue) {
				return true
			}
		}
	case *typ.Alias:
		return visitType(n.Target)
	case *typ.Meta:
		return visitType(n.Of)
	case *typ.Generic:
		for _, param := range n.TypeParams {
			if param != nil && visitType(param.Constraint) {
				return true
			}
		}
		return visitType(n.Body)
	case *typ.Instantiated:
		if visitType(n.Generic) {
			return true
		}
		return walkEachType(n.TypeArgs, visitType)
	case *typ.TypeParam:
		return visitType(n.Constraint)
	case *typ.Interface:
		for _, method := range n.Methods {
			if visitType(method.Type) {
				return true
			}
		}
	case *typ.Recursive:
		return visitType(n.Body)
	}
	return false
}

func walkEachType(types []typ.Type, visit func(typ.Type) bool) bool {
	for _, t := range types {
		if visit(t) {
			return true
		}
	}
	return false
}

func unwrapTransparent(t typ.Type) typ.Type {
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}
