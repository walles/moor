// Package ptr provides a way to take the address of an expression's value, for
// cases where Go doesn't let you take the address directly.
//
// Can be replaced with the builtin new(expr) once the Go version in go.mod
// reaches 1.26 or higher.
package ptr

// To returns a pointer to a copy of v.
//
// Replaceable by the builtin new(expr) on Go 1.26 or higher.
func To[T any](v T) *T {
	return &v
}
