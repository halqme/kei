package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/halqme/kei/internal/auth"
)

func login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	device := fs.Bool("device", false, "use device code authentication flow (headless/SSH)")
	browser := fs.Bool("browser", false, "force browser OAuth PKCE authentication flow")
	savePath := fs.String("out", "", "custom path to save auth credentials")
	if err := fs.Parse(args); err != nil {
		return err
	}

	auths := auth.List()
	if fs.NArg() == 0 {
		fmt.Println("Available authentication providers:")
		for _, a := range auths {
			fmt.Printf("  %-12s %s\n", a.Name(), a.Description())
		}
		fmt.Println("\nUsage: kei login <provider> [flags]")
		return nil
	}

	providerName := strings.ToLower(fs.Arg(0))
	authenticator, ok := auth.Get(providerName)
	if !ok {
		var names []string
		for _, a := range auths {
			names = append(names, a.Name())
		}
		return fmt.Errorf("unknown provider %q (available: %s)", providerName, strings.Join(names, ", "))
	}

	targetPath := *savePath
	if targetPath == "" {
		var err error
		targetPath, err = authenticator.DefaultSavePath()
		if err != nil {
			return err
		}
	}

	ctx := context.Background()
	opts := auth.LoginOptions{
		Device:  *device,
		Browser: *browser,
		OutPath: targetPath,
		Notify: func(msg string) {
			fmt.Println(msg)
		},
	}

	creds, err := authenticator.Login(ctx, opts)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	fmt.Printf("\nSuccessfully authenticated %s (saved to %s)\n", providerName, targetPath)
	if creds != nil && creds.AccountID != "" {
		fmt.Printf("Account ID: %s\n", creds.AccountID)
	}
	return nil
}
