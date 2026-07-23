// Package front lowers the surviving Lua WIR pipeline into equation artifacts.
//
// It is a new-engine boundary: it neither imports nor adapts the retired
// checker. Every WIR opcode is either lowered through a contract-bound equation
// family or rejected before an artifact is returned.
package front
