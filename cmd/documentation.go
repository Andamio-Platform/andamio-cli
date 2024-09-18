package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// const commandTemplate = `# {{ $cmd := . }}{{ range $i, $v := .CommandPath }}{{ if and (gt $i 0) (lt $i (sub (len .CommandPath) 1)) }} > {{ end }}{{ if gt $i 0 }}{{ $v }}{{ end }}{{ end }}
// const commandTemplate = `# {{ $cmd := . }}{{ range $i, $v := .CommandPath }}{{ if $i }} > {{ end }}{{ $v }}{{ end }}
const commandTemplate = `# {{ if .HasParent }}{{ .Parent.Name }} {{ end }}{{ .Name }}
{{ .Short }}
{{ if .Long }}
{{ .Long }}
{{ end }}
### Usage:
` + "```" + `
{{ .UseLine }}
{{ if .HasAvailableSubCommands }}{{ .CommandPath }} [command]{{ end }}
` + "```" + `
{{ if .HasAvailableSubCommands }}
### Available Commands:
` + "```" + `
{{ range .Commands }}{{ if (or .IsAvailableCommand (eq .Name "help")) }}{{ .Name }}{{ if .Short }}{{ "\t" }}{{ .Short }}{{ end }}
{{ end }}{{ end }}` + "```" + `
{{ end }}
### Options:
` + "```" + `
{{ trimTrailingWhitespaces .LocalFlags.FlagUsages }}
` + "```" + `
{{ if .HasAvailableSubCommands }}
Use "{{ .CommandPath }} [command] --help" for more information about a command.
{{ end }}
`

func GenMarkdownTreeCustom(cmd *cobra.Command, dir string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}

		cmdPath := strings.Split(c.CommandPath(), " ")
		subDir := filepath.Join(dir, filepath.Join(cmdPath[1:]...))
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", subDir, err)
		}

		filename := filepath.Join(subDir, c.Name()+".md")

		var buf bytes.Buffer
		tmpl := template.New("command").Funcs(template.FuncMap{
			"rpad": func(s string, padding int) string {
				return fmt.Sprintf("%-*s", padding, s)
			},
			"trimTrailingWhitespaces": strings.TrimSpace,
		})

		tmpl, err := tmpl.Parse(commandTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}

		if err := tmpl.Execute(&buf, c); err != nil {
			return fmt.Errorf("failed to execute template for %s: %w", c.Name(), err)
		}

		if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filename, err)
		}

		if err := GenMarkdownTreeCustom(c, dir); err != nil {
			return err
		}
	}

	return nil
}

var docCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate Andamio CLI documentation",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		err := GenMarkdownTreeCustom(rootCmd, "./docs")
		if err != nil {
			fmt.Printf("Error generating documentation: %s\n", err)
			os.Exit(1)

		}
	},
}
