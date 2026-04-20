package example

import "testing"

func TestGreet(t *testing.T) {
	if Greet("Go") != "Hello, Go!" {
		t.Fatal("wrong greeting")
	}
}

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("wrong sum")
	}
}
