package main

import (
	"log"

	"willumplabs/internal/gomoku"
)

func main() {
	if err := gomoku.Run(); err != nil {
		log.Fatal(err)
	}
}
