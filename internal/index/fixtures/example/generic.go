package example

// Pair holds two values of possibly different types.
type Pair[A, B any] struct {
	First  A
	Second B
}

// Swap returns a new Pair with the values swapped.
func (p Pair[A, B]) Swap() Pair[B, A] {
	return Pair[B, A]{First: p.Second, Second: p.First}
}

// Triple holds three values.
type Triple[A, B, C any] struct {
	A A
	B B
	C C
}

// First returns the first element of the Triple.
func (t *Triple[A, B, C]) First() A {
	return t.A
}
