// Package ptr provides a way to take the address of an expression's value,
// for cases where Go doesn't let you take the address directly.
package ptr

// To returns a pointer to a copy of v.
func To[T any](v T) *T {
	return &v
}
