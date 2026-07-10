package callboundary

import "github.com/wippyai/go-lua/analysis/domain/typestate"

// ProtectedCallTypestate carries callback lifecycle snapshots to a protected
// caller. Normal and exceptional exits remain separate until pcall/xpcall
// catches them; only then may the caller join their states.
type ProtectedCallTypestate struct {
	Normal         typestate.Store
	Exceptional    typestate.Store
	HasNormal      bool
	HasExceptional bool
}

func (p ProtectedCallTypestate) Empty() bool {
	return !p.HasNormal && !p.HasExceptional
}

func (p ProtectedCallTypestate) Clone() ProtectedCallTypestate {
	return ProtectedCallTypestate{
		Normal:         p.Normal.Clone(),
		Exceptional:    p.Exceptional.Clone(),
		HasNormal:      p.HasNormal,
		HasExceptional: p.HasExceptional,
	}
}

func (p ProtectedCallTypestate) Equal(other ProtectedCallTypestate) bool {
	return p.HasNormal == other.HasNormal &&
		p.HasExceptional == other.HasExceptional &&
		typestate.Equal(p.Normal, other.Normal) &&
		typestate.Equal(p.Exceptional, other.Exceptional)
}

func (p ProtectedCallTypestate) LessOrEq(other ProtectedCallTypestate) bool {
	if p.HasNormal && (!other.HasNormal || !typestate.LessOrEq(p.Normal, other.Normal)) {
		return false
	}
	if p.HasExceptional && (!other.HasExceptional || !typestate.LessOrEq(p.Exceptional, other.Exceptional)) {
		return false
	}
	return true
}

func JoinProtectedCallTypestate(a, b ProtectedCallTypestate) ProtectedCallTypestate {
	out := ProtectedCallTypestate{HasNormal: a.HasNormal || b.HasNormal, HasExceptional: a.HasExceptional || b.HasExceptional}
	if a.HasNormal && b.HasNormal {
		out.Normal = typestate.Join(a.Normal, b.Normal)
	} else if a.HasNormal {
		out.Normal = a.Normal.Clone()
	} else if b.HasNormal {
		out.Normal = b.Normal.Clone()
	}
	if a.HasExceptional && b.HasExceptional {
		out.Exceptional = typestate.Join(a.Exceptional, b.Exceptional)
	} else if a.HasExceptional {
		out.Exceptional = a.Exceptional.Clone()
	} else if b.HasExceptional {
		out.Exceptional = b.Exceptional.Clone()
	}
	return out
}
