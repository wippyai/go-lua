package axiscompose

import "github.com/wippyai/go-lua/analysis/domain/lattice"

const toyUniverse uint8 = 0x0f

type toyPayload struct {
	Symbol string
	Bits   uint8
}

// RegisterToyMay adds a finite may-fact lane: subset order and union join.
func RegisterToyMay(c *Catalog, id AxisID) Handle[uint8] {
	return MustRegister(c, Spec[uint8]{
		ID:       id,
		Polarity: May,
		Domain: lattice.Lattice[uint8]{
			Bottom:   func() uint8 { return 0 },
			Top:      func() uint8 { return toyUniverse },
			Equal:    func(a, b uint8) bool { return a == b },
			Same:     func(a, b uint8) bool { return a == b },
			LessOrEq: func(a, b uint8) bool { return a&^b == 0 },
			Join:     func(a, b uint8) uint8 { return a | b },
			Widen:    func(a, b uint8) uint8 { return a | b },
		},
		Boundary: exactToyBoundary(),
		Hash:     func(v uint8) uint64 { return uint64(v) },
	})
}

// RegisterToyMust adds a finite must-fact lane: reverse-inclusion order and
// intersection join. Bottom is the fixed universe and Top is the empty set.
func RegisterToyMust(c *Catalog, id AxisID) Handle[uint8] {
	return MustRegister(c, Spec[uint8]{
		ID:       id,
		Polarity: Must,
		Domain: lattice.Lattice[uint8]{
			Bottom:   func() uint8 { return toyUniverse },
			Top:      func() uint8 { return 0 },
			Equal:    func(a, b uint8) bool { return a == b },
			Same:     func(a, b uint8) bool { return a == b },
			LessOrEq: func(a, b uint8) bool { return b&^a == 0 },
			Join:     func(a, b uint8) uint8 { return a & b },
			Widen:    func(a, b uint8) uint8 { return a & b },
		},
		Boundary: exactToyBoundary(),
		Hash:     func(v uint8) uint64 { return uint64(v) },
	})
}

// RegisterToyUnsupported adds an ordinary finite may lane without boundary
// capability. A used instance must force contextual fallback.
func RegisterToyUnsupported(c *Catalog, id AxisID) Handle[uint8] {
	return MustRegister(c, Spec[uint8]{
		ID:       id,
		Polarity: May,
		Domain: lattice.Lattice[uint8]{
			Bottom:   func() uint8 { return 0 },
			Top:      func() uint8 { return toyUniverse },
			Equal:    func(a, b uint8) bool { return a == b },
			Same:     func(a, b uint8) bool { return a == b },
			LessOrEq: func(a, b uint8) bool { return a&^b == 0 },
			Join:     func(a, b uint8) uint8 { return a | b },
			Widen:    func(a, b uint8) uint8 { return a | b },
		},
		Hash: func(v uint8) uint64 { return uint64(v) },
	})
}

func exactToyBoundary() *Boundary[uint8] {
	return &Boundary[uint8]{
		Project: func(ctx ProjectCtx, value uint8) (any, Support) {
			return toyPayload{Symbol: ctx.Binding.Symbol, Bits: value}, Exact
		},
		Instantiate: func(ctx InstantiateCtx, payload any) (uint8, Support) {
			p, ok := payload.(toyPayload)
			if !ok || p.Symbol == "" || ctx.Binding.Symbol == "" {
				return 0, Contextual
			}
			return p.Bits, Exact
		},
	}
}
