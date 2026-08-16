// Package directbinding seals exact lexical binding selectors.
//
// The package owns one compact parent-chain for exact Read selections.  The
// same chain is projected through the typed BindingSelections,
// PublicationPaths, and DirectCalls views; it never publishes a generic path
// intermediate representation or retains any of the Source, Flow, Static,
// or Module owners used while sealing.
package directbinding
