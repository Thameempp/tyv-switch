// tyv — Fast Antigravity Account Manager
//
// tyv is a privacy-first, security-focused CLI for managing and switching
// between multiple Antigravity (agy) profiles/accounts from the terminal.
//
// Usage:
//
//	tyv                    Pick a profile interactively
//	tyv <profile>          Switch to a profile and launch agy
//	tyv add <name>         Create a new profile
//	tyv list               List all profiles
//	tyv current            Show the current profile
//	tyv remove <name>      Remove a profile
//	tyv rename <old> <new> Rename a profile (alias: edit)
//	tyv doctor             Run diagnostics
//	tyv version            Show version
//
// Security principles:
//   - tyv never asks for passwords or OAuth tokens
//   - tyv never stores credentials
//   - tyv never logs credentials
//   - tyv never makes network requests
//   - all authentication is handled exclusively by agy
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/thameem/tyv/internal/config"
	"github.com/thameem/tyv/internal/doctor"
	"github.com/thameem/tyv/internal/exitcode"
	"github.com/thameem/tyv/internal/launcher"
	"github.com/thameem/tyv/internal/platform"
	"github.com/thameem/tyv/internal/providers/antigravity"
	"github.com/thameem/tyv/internal/selector"
	"github.com/thameem/tyv/internal/validation"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	plat := platform.New()

	appDataDir, err := plat.AppDataDir()
	if err != nil {
		errorf("could not determine application data directory: %v", err)
		return exitcode.GeneralError
	}

	mgr := config.NewManager(appDataDir)

	if len(args) == 0 {
		return cmdInteractive(plat, mgr, appDataDir)
	}

	cmd := args[0]
	rest := args[1:]

	// Handle top-level commands first.
	switch cmd {
	case "help", "--help", "-h":
		printUsage()
		return exitcode.Success

	case "version", "--version", "-v":
		fmt.Printf("tyv version %s\n", version)
		return exitcode.Success

	case "add":
		return cmdAdd(plat, mgr, rest)

	case "list", "ls":
		return cmdList(mgr)

	case "current":
		return cmdCurrent(mgr)

	case "remove", "rm":
		return cmdRemove(mgr, rest)

	case "rename", "edit", "mv":
		return cmdRename(mgr, rest)

	case "doctor":
		return cmdDoctor(plat, mgr, appDataDir)
	}

	// If not a recognised command, treat as a profile name.
	// Validate the name before doing any filesystem work.
	if err := validation.ProfileName(cmd); err != nil {
		errorf("%v", err)
		return exitcode.InvalidUsage
	}

	return cmdSwitch(plat, mgr, appDataDir, cmd, rest)
}

// cmdInteractive shows the account picker when tyv is run with no arguments.
// When stdin/stdout is not a terminal, or there are no profiles to pick from,
// it prints the usage text instead.
func cmdInteractive(plat platform.Platform, mgr *config.Manager, appDataDir string) int {
	if !selector.IsTerminal() {
		printUsage()
		return exitcode.Success
	}
	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	cfg := mgr.Get()
	if len(cfg.Profiles) == 0 {
		printUsage()
		return exitcode.Success
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	// Every run fetches fresh figures: all rows show the loading spinner and
	// update as lookups finish (agy takes ~10s per lookup). The cache is only a
	// fallback for when a lookup fails.
	l := launcher.New(plat, mgr)
	cache := antigravity.LoadCache(appDataDir)
	rows := make([]selector.Row, len(names))
	start := 0
	for i, n := range names {
		rows[i] = selector.Row{Name: n, Gemini: selector.Loading, Claude: selector.Loading,
			Email: profileEmail(l, mgr, cfg.Profiles[n])}
		if n == cfg.Current {
			start = i
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan selector.Update, len(rows))
	agyPath, agyErr := l.FindAGY()
	var wg sync.WaitGroup
	var cacheMu sync.Mutex
	for i := range names {
		if agyErr != nil {
			updates <- selector.Update{Index: i, Gemini: selector.Unknown, Claude: selector.Unknown}
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := selector.Update{Index: i, Gemini: selector.Unknown, Claude: selector.Unknown}
			if env, err := l.ProfileEnv(names[i]); err == nil {
				usage, err := antigravity.Fetch(ctx, agyPath, env)
				cacheMu.Lock()
				if err == nil {
					u.Gemini, u.Claude = usage.Gemini, usage.Claude
					cache.Set(names[i], usage)
					cache.Save()
				} else if old, ok := cache.Entries[names[i]]; ok {
					u.Gemini, u.Claude = old.Usage.Gemini, old.Usage.Claude
				}
				cacheMu.Unlock()
			}
			updates <- u
		}(i)
	}

	restore, err := selector.MakeRaw()
	if err != nil {
		errorf("could not enter interactive mode: %v", err)
		return exitcode.GeneralError
	}

	// Leave the terminal usable if tyv is terminated while in raw mode.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		if sig, ok := <-sigs; ok {
			restore()
			fmt.Print("\x1b[?25h\n")
			code := 143
			if sig == syscall.SIGHUP {
				code = 129
			}
			os.Exit(code)
		}
	}()

	res, idx, err := selector.Run(os.Stdin, os.Stdout, rows, start, updates)
	signal.Stop(sigs)
	close(sigs)
	restore()
	cancel() // stop any lookups still running
	wg.Wait()
	if err != nil {
		errorf("%v", err)
		return exitcode.GeneralError
	}

	switch res {
	case selector.Selected:
		return cmdSwitch(plat, mgr, appDataDir, names[idx], nil)
	case selector.Interrupted:
		return exitcode.Interrupted
	}
	return exitcode.Success
}

// cmdAdd creates a new profile.
func cmdAdd(plat platform.Platform, mgr *config.Manager, args []string) int {
	if len(args) == 0 {
		errorf("usage: tyv add <name> [--email <address>]")
		return exitcode.InvalidUsage
	}

	name := args[0]
	if err := validation.ProfileName(name); err != nil {
		errorf("%v", err)
		return exitcode.InvalidUsage
	}

	// Optional: --email <address>
	email := ""
	for i := 1; i < len(args)-1; i++ {
		if args[i] == "--email" || args[i] == "-e" {
			email = args[i+1]
			break
		}
	}

	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	if err := mgr.AddProfile(name, email); err != nil {
		errorf("%v", err)
		return exitcode.GeneralError
	}

	fmt.Printf("Profile %q created.\n", name)
	if email != "" {
		fmt.Printf("Email hint: %s\n", email)
	}

	// Without a terminal there is nobody to sign in; keep scripted use quiet.
	if !selector.IsTerminal() {
		fmt.Printf("\nSign in later by launching Antigravity with this profile:\n  tyv %s\n", name)
		return exitcode.Success
	}

	fmt.Println()
	if !confirm("Open Google authentication in browser?") {
		fmt.Printf("Skipped. Sign in later by launching Antigravity with this profile:\n  tyv %s\n", name)
		return exitcode.Success
	}

	// Sign-in is performed entirely by agy (it opens the browser and handles
	// the OAuth exchange); tyv never sees codes, tokens, or passwords.
	fmt.Println("Launching Antigravity. Complete Google sign-in in your browser,")
	fmt.Println("then exit Antigravity to finish.")
	fmt.Println()

	l := launcher.New(plat, mgr)
	if err := l.RunForProfile(name); err != nil {
		fmt.Println()
		fmt.Fprintf(os.Stderr, "✗ Account configuration failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Profile %q was kept. Retry sign-in with: tyv %s\n", name, name)
		if strings.Contains(err.Error(), "not found") {
			return exitcode.ExecutableNotFound
		}
		return exitcode.GeneralError
	}

	_ = mgr.TouchLastUsed(name)
	fmt.Println()
	fmt.Printf("✓ Account %q configured successfully\n", name)
	if email := antigravity.ReadEmail(l.HomeDir(name)); email != "" {
		_ = mgr.SetEmail(name, email)
		fmt.Printf("  Signed in as %s\n", email)
	} else {
		fmt.Printf("  Note: no signed-in Google account was detected for this profile yet.\n")
		fmt.Printf("  Run `tyv %s` and sign in if agy asks you to.\n", name)
	}
	return exitcode.Success
}

// confirm asks a yes/no question with a default of yes: "<q> [Y/n] ".
// Anything other than a clear no (including EOF) is not treated as yes unless
// the answer is empty or starts with y/Y.
func confirm(question string) bool {
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [Y/n] ", question)
		line, err := r.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "y", "yes":
			if line == "" && err != nil {
				return false // EOF with no input
			}
			return true
		case "n", "no":
			return false
		}
		if err != nil {
			return false
		}
		fmt.Println("Please answer y or n.")
	}
}

// cmdList prints all profiles.
func cmdList(mgr *config.Manager) int {
	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	cfg := mgr.Get()
	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles yet.\n\nCreate one:\n  tyv add personal")
		return exitcode.Success
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == cfg.Current {
			fmt.Printf("%s (current)\n", name)
		} else {
			fmt.Println(name)
		}
	}
	return exitcode.Success
}

// profileEmail returns the Google account the profile is signed into, falling
// back to the email hint given to `tyv add --email`.
func profileEmail(l *launcher.Launcher, mgr *config.Manager, p config.Profile) string {
	if email := antigravity.ReadEmail(l.HomeDir(p.Name)); email != "" {
		// Remember it: agy's logs rotate, the stored value does not.
		_ = mgr.SetEmail(p.Name, email)
		return email
	}
	return p.Email
}

// forgetUsage drops a profile's cached usage so a later profile with the same
// name never shows another account's figures.
func forgetUsage(mgr *config.Manager, name string) {
	dir := filepath.Dir(mgr.ConfigPath())
	c := antigravity.LoadCache(dir)
	c.Delete(name)
	c.Save()
}

// cmdCurrent prints the current profile name.
func cmdCurrent(mgr *config.Manager) int {
	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	cfg := mgr.Get()
	if cfg.Current == "" {
		fmt.Println("(no current profile)")
		return exitcode.Success
	}
	fmt.Println(cfg.Current)
	return exitcode.Success
}

// cmdRemove deletes a profile.
func cmdRemove(mgr *config.Manager, args []string) int {
	if len(args) == 0 {
		errorf("usage: tyv remove <name>")
		return exitcode.InvalidUsage
	}

	name := args[0]
	if err := validation.ProfileName(name); err != nil {
		errorf("%v", err)
		return exitcode.InvalidUsage
	}

	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	if err := mgr.RemoveProfile(name); err != nil {
		errorf("%v", err)
		return exitcode.ProfileNotFound
	}

	forgetUsage(mgr, name)
	fmt.Printf("Profile %q removed.\n", name)
	return exitcode.Success
}

// cmdRename renames a profile.
func cmdRename(mgr *config.Manager, args []string) int {
	if len(args) < 2 {
		errorf("usage: tyv edit <old-name> <new-name>")
		return exitcode.InvalidUsage
	}

	oldName, newName := args[0], args[1]

	if err := validation.ProfileName(oldName); err != nil {
		errorf("old name: %v", err)
		return exitcode.InvalidUsage
	}
	if err := validation.ProfileName(newName); err != nil {
		errorf("new name: %v", err)
		return exitcode.InvalidUsage
	}

	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	if err := mgr.RenameProfile(oldName, newName); err != nil {
		errorf("%v", err)
		return exitcode.GeneralError
	}

	forgetUsage(mgr, oldName)
	fmt.Printf("Profile %q renamed to %q.\n", oldName, newName)
	return exitcode.Success
}

// cmdDoctor runs diagnostics.
func cmdDoctor(plat platform.Platform, mgr *config.Manager, appDataDir string) int {
	_ = mustLoad(mgr) // ignore load errors; doctor reports them

	result := doctor.Run(plat, mgr, appDataDir)
	doctor.Print(result)

	if !result.Ready {
		return exitcode.GeneralError
	}
	return exitcode.Success
}

// cmdSwitch activates a profile and launches agy.
func cmdSwitch(plat platform.Platform, mgr *config.Manager, appDataDir, name string, extraArgs []string) int {
	if err := mustLoad(mgr); err != nil {
		return exitcode.GeneralError
	}

	if !mgr.ProfileExists(name) {
		cfg := mgr.Get()
		names := make([]string, 0, len(cfg.Profiles))
		for n := range cfg.Profiles {
			names = append(names, n)
		}

		if len(names) == 0 {
			errorf(
				"profile %q does not exist\n\n"+
					"No profiles created yet. Create one with:\n  tyv add %s",
				name, name,
			)
		} else {
			errorf(
				"profile %q does not exist\n\nAvailable profiles:\n  %s\n\nCreate it with:\n  tyv add %s",
				name, strings.Join(names, "\n  "), name,
			)
		}
		return exitcode.ProfileNotFound
	}

	// Update last-used timestamp before launching (best effort).
	_ = mgr.TouchLastUsed(name)

	fmt.Fprintf(os.Stderr, "Switching to: %s\n", name)
	fmt.Fprintf(os.Stderr, "Launching Antigravity...\n")

	l := launcher.New(plat, mgr)
	if err := l.LaunchForProfile(name, extraArgs); err != nil {
		errorf("%v", err)

		// Provide specific guidance for executable-not-found.
		if strings.Contains(err.Error(), "not found") {
			return exitcode.ExecutableNotFound
		}
		return exitcode.GeneralError
	}

	// On Unix, LaunchForProfile replaces the process and never returns here.
	// On Windows it calls os.Exit directly. This line is unreachable normally.
	return exitcode.Success
}

// mustLoad calls mgr.Load() and prints a friendly error on failure.
func mustLoad(mgr *config.Manager) error {
	if err := mgr.Load(); err != nil {
		errorf("%v", err)
		return err
	}
	return nil
}

// errorf prints an error message to stderr with an "Error: " prefix.
func errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
}

func printUsage() {
	fmt.Print(`tyv — Fast Antigravity account manager

Usage:
  tyv                        Pick a profile interactively
  tyv <profile>              Switch to profile and launch Antigravity
  tyv add <name>             Create a new profile and sign in via agy
  tyv list                   List all profiles
  tyv current                Show the current profile
  tyv remove <name>          Remove a profile
  tyv rename <old> <new>     Rename a profile (alias: edit)
  tyv doctor                 Run diagnostics
  tyv version                Show version

Examples:
  tyv add personal
  tyv add work --email work@example.com
  tyv personal
  tyv work
  tyv list
  tyv current
  tyv edit work company
  tyv remove company
  tyv doctor

Profile names:
  - lowercase letters, digits, hyphens, underscores
  - 1–63 characters
  - must start with a letter or digit

Security:
  tyv never stores passwords, tokens, or credentials.
  All authentication is handled by Antigravity CLI (agy).

Environment:
  TYV_AGY_PATH   Override path to the agy executable
  TYV_DEBUG=1    Enable debug output (credentials are never printed)
`)
}
