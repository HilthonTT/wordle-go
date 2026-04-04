package main

import (
	"fmt"
	"os"

	"github.com/hilthontt/wordle-go/internal/game"
)

func main() {
	a := game.Attempt{Answer: "Hello"}
	if err := a.ComputeResult("Hella"); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Attempt: %v\n", a)
}
