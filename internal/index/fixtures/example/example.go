package example

import "fmt"

// Greet returns a greeting.
func Greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}

type Server struct{}

// Start starts the server.
func (s *Server) Start() error {
	Greet("world")
	return nil
}
