package main

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sovkit:", err)
		os.Exit(2)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, "Usage: sovkit catalog\n\nCommands:\n  catalog   Browse launchable recipes and inspect their hardware requirements")
		return err
	}
	if args[0] != "catalog" {
		return fmt.Errorf("unknown command %q (try: sovkit help)", args[0])
	}
	_, err := tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithOutput(output)).Run()
	return err
}
