package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	prompt := fs.String("p", "", "single prompt or slash command")
	model := fs.String("m", "", "model override or alias (e.g. gpt-5.5, claude-3-7-sonnet)")
	prov := fs.String("provider", "", "connection target override (e.g. openai, claude, codex, local)")
	sessionID := fs.String("session", "", "persistent session ID (load if it exists, create otherwise)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*conf)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	r, err := discoverExtensions(cfg, cwd)
	if err != nil {
		return err
	}
	if *prompt == "/help" {
		printREPLHelp(r, cfg)
		return nil
	}
	if *prompt == "/model" || *prompt == "/models" {
		printREPLModels(cfg, *prov, *model)
		return nil
	}

	cfg, err = loadSessionConfig(*conf)
	if err != nil {
		return err
	}
	state, store, err := openSession(*sessionID, cwd)
	if err != nil {
		return err
	}
	r, err = discoverExtensions(cfg, state.Workspace)
	if err != nil {
		return err
	}
	runtime, err := makeRuntime(cfg, r, state, store, *prov, *model)
	if err != nil {
		return err
	}

	if *prompt != "" {
		streamed := false
		runtime.OnEvent = func(kind string, payload any) {
			if kind != "assistant_message_chunk" {
				return
			}
			data, ok := payload.(map[string]any)
			if !ok {
				return
			}
			text, _ := data["text"].(string)
			if text == "" {
				return
			}
			streamed = true
			fmt.Print(text)
		}
		out, err := runtime.Prompt(context.Background(), *prompt)
		if err == nil {
			if streamed {
				if out != "" && !strings.HasSuffix(out, "\n") {
					fmt.Println()
				}
			} else {
				fmt.Print(out)
				if out != "" && !strings.HasSuffix(out, "\n") {
					fmt.Println()
				}
			}
		} else if streamed {
			fmt.Println()
		}
		return err
	}

	streamed := false
	runtime.OnEvent = func(kind string, payload any) {
		if kind != "assistant_message_chunk" {
			return
		}
		data, ok := payload.(map[string]any)
		if !ok {
			return
		}
		text, _ := data["text"].(string)
		if text == "" {
			return
		}
		streamed = true
		fmt.Print(text)
	}
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("kei> ")
		if !in.Scan() {
			return in.Err()
		}
		q := strings.TrimSpace(in.Text())
		if q == "" {
			continue
		}
		if q == "/exit" || q == "/quit" {
			return nil
		}
		if q == "/help" {
			printREPLHelp(r, cfg)
			continue
		}
		if q == "/model" || q == "/models" {
			printREPLModels(cfg, *prov, *model)
			continue
		}
		streamed = false
		out, err := runtime.Prompt(context.Background(), q)
		if err != nil {
			if streamed {
				fmt.Println()
			}
			fmt.Fprintln(os.Stderr, "error:", err)
			continue
		}
		if streamed {
			if out != "" && !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
		} else {
			fmt.Print(out)
			if out != "" && !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
		}
	}
}
