// Command omnisnap compares how every installed browser engine lays out a page.
package main

import (
	"os"

	"github.com/GerMP21/Omnisnap/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args, os.Stdout, os.Stderr))
}
