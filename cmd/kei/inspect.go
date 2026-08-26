package main

import (
	"context"
	"encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/halqme/kei/internal/config"
	"github.com/halqme/kei/internal/provider"
	"github.com/halqme/kei/internal/tool"
)

type modelsOutput struct {
	ConfiguredProviders    []config.Provider `json:"configured_providers"`
	AvailableProviders     []config.Provider `json:"available_providers"`
	Aliases                map[string]string `json:"aliases,omitempty"`
	SupportedProviderTypes []string          `json:"supported_provider_types"`
}

func models(args []string) error {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	jsonOut := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*conf)
	if err != nil {
		return err
	}

	available := availableProviderConfigs(context.Background())
	supported := provider.List()

	if *jsonOut {
		out := modelsOutput{
			ConfiguredProviders:    cfg.Providers,
			AvailableProviders:     available,
			Aliases:                cfg.Models,
			SupportedProviderTypes: supported,
		}
		return json.MarshalWrite(os.Stdout, out)
	}

	fmt.Println("Configured Connection Targets:")
	if len(cfg.Providers) == 0 {
		fmt.Println("  (none; falling back to the first available target)")
	} else {
		for i, p := range cfg.Providers {
			name := p.Name
			if name == "" {
				name = p.Type
			}
			marker := "  "
			if i == 0 {
				marker = "* "
			}
			fmt.Printf("%s%s (%s):\n", marker, name, p.Type)
			if p.Model != "" {
				fmt.Printf("    Model: %s\n", p.Model)
			}
			if len(p.Models) > 0 {
				fmt.Printf("    Available Models: %s\n", strings.Join(p.Models, ", "))
			}
			if p.BaseURL != "" {
				fmt.Printf("    Base URL: %s\n", p.BaseURL)
			}
			if p.APIKeyEnv != "" {
				fmt.Printf("    API Key Env: %s\n", p.APIKeyEnv)
			}
		}
	}

	fmt.Println("\nAvailable Connection Targets:")
	if len(available) == 0 {
		fmt.Println("  (none; run 'kei login <provider>' or configure a local provider)")
	} else {
		for _, p := range available {
			fmt.Printf("  %s (%s)\n", p.Name, p.Type)
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

	fmt.Println("\nSupported Provider Types:")
	fmt.Printf("  %s\n", strings.Join(supported, ", "))

	return nil
}

func extensions(args []string) error {
	fs := flag.NewFlagSet("extensions", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	jsonOut := fs.Bool("json", false, "JSON output")
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
	if *jsonOut {
		return json.MarshalWrite(os.Stdout, r.Extensions)
	}
	for _, ext := range r.Extensions {
		fmt.Printf("%-20s tools=%d commands=%d %s\n", ext.ID, len(ext.Tools), len(ext.Commands), ext.Root)
	}
	return nil
}

func tools(args []string) error {
	fs := flag.NewFlagSet("tools", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	jsonOut := fs.Bool("json", false, "JSON output")
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
	ds := r.Tools.List()
	if *jsonOut {
		return json.MarshalWrite(os.Stdout, ds)
	}
	for _, d := range ds {
		fmt.Printf("%-28s %-28s %s\n", d.QualifiedName, d.ModelName, d.Description)
	}
	return nil
}

func commands(args []string) error {
	fs := flag.NewFlagSet("commands", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	jsonOut := fs.Bool("json", false, "JSON output")
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
	ds := r.Commands.List()
	if *jsonOut {
		return json.MarshalWrite(os.Stdout, ds)
	}
	for _, d := range ds {
		fmt.Printf("/%-27s %s\n", d.QualifiedName, d.Description)
	}
	return nil
}

func execTool(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	conf := fs.String("config", "", "config path")
	input := fs.String("input", "{}", "JSON object")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: kei exec [flags] <extension.tool>")
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
	d, ok := r.Tools.Get(fs.Arg(0))
	if !ok {
		return fmt.Errorf("unknown tool %q", fs.Arg(0))
	}
	var in map[string]any
	if err := json.Unmarshal([]byte(*input), &in); err != nil {
		return err
	}
	out, err := tool.Execute(context.Background(), cwd, d, in)
	fmt.Print(out)
	return err
}
