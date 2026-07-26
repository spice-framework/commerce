package main

import (
	"os"

	commerce "github.com/StevenBuglione/spice/internal/spicegen/commerce"
)

func main() {
	os.Exit(commerce.Main(os.Args[1:]))
}
