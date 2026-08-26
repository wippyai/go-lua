package authority

// Provider is the optional surface hook for an owner-local relational
// attachment. The Catalog already carries the exact sealed owner token, so a
// provider returns one statement of ownership rather than a second token
// parameter. A consumer compares Catalog.Owner().Entry and Token with the
// sealed entry it is adapting before projecting rows into relcompile.
type Provider interface {
	Authority() (Catalog, bool)
}
