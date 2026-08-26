// Package step redeems one sealed schedule entry against one committed
// relation root. It is the composition boundary between the mount-owned
// physical plan and typed relational kernels: it never resolves a logical
// expression, chooses an access path, or imports a domain.
package step
