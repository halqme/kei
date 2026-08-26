package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/halqme/kei/internal/config"
	"github.com/halqme/kei/internal/extension"
)

func helpCmd(args []string) error {
	if len(args) == 0 {
		printGeneralHelp()
		return nil
	}
	subcmd := args[0]
	switch subcmd {
	case "run":
		printRunHelp()
	case "models":
		printModelsHelp()
	case "extensions":
		printExtensionsHelp()
	case "tools":
		printToolsHelp()
	case "commands":
		printCommandsHelp()
	case "exec":
		printExecHelp()
	case "acp":
		printACHelp()
	case "login":
		printLoginHelp()
	case "version":
		printVersionHelp()
	case "help":
		printGeneralHelp()
	default:
		return fmt.Errorf("unknown command %q (run 'kei help' for available commands)", subcmd)
	}
	return nil
}

func printGeneralHelp() {
	fmt.Println("kei - Unix-native harness for coding agents")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kei <command> [flags] [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  run          Run an interactive agent REPL or single prompt")
	fmt.Println("  models       List configured and available connection targets, models, and aliases")
	fmt.Println("  extensions   List discovered extensions")
	fmt.Println("  tools        List discovered tools across extensions")
	fmt.Println("  commands     List discovered slash commands")
	fmt.Println("  exec         Execute an extension tool directly")
	fmt.Println("  acp          Run the ACP (Agent Client Protocol) server over stdin/stdout")
	fmt.Println("  login        Authenticate with providers (OAuth PKCE / API keys)")
	fmt.Println("  version      Print version information")
	fmt.Println("  help         Show help for kei or a specific command")
	fmt.Println()
	fmt.Println("Run 'kei help <command>' for more information about a specific command.")
}

func printRunHelp() {
	fmt.Println("Usage: kei run [flags]")
	fmt.Println()
	fmt.Println("Run an interactive agent REPL or execute a single prompt.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -p string")
	fmt.Println("        single prompt or slash command (non-interactive)")
	fmt.Println("  -m string")
	fmt.Println("        model override or alias (e.g. gpt-5.5, claude-3-7-sonnet, fast)")
	fmt.Println("  -provider string")
	fmt.Println("        connection target override (e.g. openai, claude, codex, local)")
	fmt.Println()
	fmt.Println("Interactive REPL commands:")
	fmt.Println("  /help             Show REPL help and available slash commands")
	fmt.Println("  /model, /models   Show active model and configured providers/aliases")
	fmt.Println("  /exit, /quit      Exit the REPL session")
}

func printModelsHelp() {
	fmt.Println("Usage: kei models [flags]")
	fmt.Println()
	fmt.Println("List configured and available connection targets, models, aliases, and provider types.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -json")
	fmt.Println("        output in JSON format")
}

func printExtensionsHelp() {
	fmt.Println("Usage: kei extensions [flags]")
	fmt.Println()
	fmt.Println("List all discovered extensions, their tool/command counts, and root paths.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -json")
	fmt.Println("        output in JSON format")
}

func printToolsHelp() {
	fmt.Println("Usage: kei tools [flags]")
	fmt.Println()
	fmt.Println("List all discovered agent tools across extensions.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -json")
	fmt.Println("        output in JSON format")
}

func printCommandsHelp() {
	fmt.Println("Usage: kei commands [flags]")
	fmt.Println()
	fmt.Println("List all discovered slash commands across extensions.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -json")
	fmt.Println("        output in JSON format")
}

func printExecHelp() {
	fmt.Println("Usage: kei exec [flags] <extension.tool>")
	fmt.Println()
	fmt.Println("Execute an extension tool directly with JSON input.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -input string")
	fmt.Println("        JSON object input for the tool (default: \"{}\")")
}

func printACHelp() {
	fmt.Println("Usage: kei acp [flags]")
	fmt.Println()
	fmt.Println("Run the ACP (Agent Client Protocol) server over standard input and output.")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string")
	fmt.Println("        config path")
	fmt.Println("  -m string")
	fmt.Println("        model override or alias")
	fmt.Println("  -provider string")
	fmt.Println("        connection target override")
}

func printLoginHelp() {
	fmt.Println("Usage: kei login [provider] [flags]")
	fmt.Println()
	fmt.Println("Authenticate with LLM providers using OAuth PKCE or store API keys locally.")
	fmt.Println()
	fmt.Println("Providers:")
	fmt.Println("  codex       OpenAI Codex (ChatGPT subscription OAuth PKCE / device code)")
	fmt.Println("  openai      OpenAI API key")
	fmt.Println("  anthropic   Anthropic Claude API key")
	fmt.Println("  gemini      Google Gemini API key")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -device")
	fmt.Println("        use device code authentication flow (headless/SSH)")
	fmt.Println("  -browser")
	fmt.Println("        force browser OAuth PKCE authentication flow")
	fmt.Println("  -out string")
	fmt.Println("        custom path to save auth credentials")
}

func printVersionHelp() {
	fmt.Println("Usage: kei version")
	fmt.Println()
	fmt.Println("Print the version of kei.")
}

func printREPLHelp(r *extension.Registry, cfg config.Config) {
	fmt.Println("Interactive REPL Commands:")
	fmt.Println("  /help             Show this help message")
	fmt.Println("  /model, /models   Show active model and configured providers/aliases")
	fmt.Println("  /exit, /quit      Exit the REPL session")

	if r != nil && r.Commands != nil {
		cmds := r.Commands.List()
		if len(cmds) > 0 {
			fmt.Println("\nDiscovered Slash Commands:")
			for _, cmd := range cmds {
				fmt.Printf("  /%-24s %s\n", cmd.QualifiedName, cmd.Description)
			}
		}
	}
	if r != nil && r.Tools != nil {
		tools := r.Tools.List()
		if len(tools) > 0 {
			fmt.Printf("\nDiscovered Tools: %d tool(s) available\n", len(tools))
		}
	}
	fmt.Println("\nType any prompt to interact with the agent.")
}

func printREPLModels(cfg config.Config, providerOverride, modelOverride string) {
	cfg = withAvailableProviders(cfg)
	p, err := cfg.ResolveProvider(providerOverride, modelOverride)
	fmt.Println("Active Connection:")
	if err != nil {
		fmt.Printf("  (none: %v)\n", err)
	} else {
		fmt.Printf("  Name:     %s\n", p.Name)
		fmt.Printf("  Provider: %s\n", p.Type)
		fmt.Printf("  Model:    %s\n", p.Model)
		if len(p.Models) > 0 {
			fmt.Printf("  Available Models: %s\n", strings.Join(p.Models, ", "))
		}
	}

	if len(cfg.Providers) > 0 {
		fmt.Println("\nConnection Targets:")
		for i, target := range cfg.Providers {
			name := target.Name
			if name == "" {
				name = target.Type
			}
			marker := "  "
			if i == 0 {
				marker = "* "
			}
			fmt.Printf("%s%-12s (%s) -> %s\n", marker, name, target.Type, target.Model)
		}
	}

	if len(cfg.Models) > 0 {
		fmt.Println("\nModel Aliases:")
		aliases := make([]string, 0, len(cfg.Models))
		for alias := range cfg.Models {
			aliases = append(aliases, alias)
		}
		slices.Sort(aliases)
		for _, alias := range aliases {
			fmt.Printf("  %-10s -> %s\n", alias, cfg.Models[alias])
		}
	}
}
