package main

import (
	"github.com/Grey-Magic/kunji/cmd"
	"github.com/pterm/pterm"
)

func main() {
	pterm.EnableStyling()
	cmd.Execute()
}
