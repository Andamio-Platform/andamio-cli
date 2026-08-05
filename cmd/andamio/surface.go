package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func CommandSurface(cmd *cobra.Command) string {

	var builder strings.Builder
	commands := cmd.Commands()
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name() < commands[j].Name() })

	line := cmd.CommandPath() + " - " + "<flag list (if any)> ... \n"

	builder.WriteString(line)

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		flagline := "name: " + f.Name + " | type: " + f.Value.Type() + "\n"
		builder.WriteString(flagline)
	})

	for _, c := range commands {
		builder.WriteString(CommandSurface(c))
	}

	return builder.String()

}
