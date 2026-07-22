package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// version is set at build time via -ldflags (GoReleaser).
var version = "dev"

const usageText = `www-browser-helper - open URLs from inside a GitHub Codespace

Usage:
  www-browser-helper <url>
  www-browser-helper [flags]

It forwards <url> to the command named by the BROWSER environment variable,
which Codespaces sets to a helper that opens the URL on your local machine.

Flags:
  -h, --help       show this help
  -v, --version    show version

Environment:
  BROWSER      command used to open the URL (required)
  CODESPACES   must be "true"; the tool only runs inside a Codespace
`

// checkCodespace reports whether we are running inside a GitHub Codespace.
func checkCodespace() bool {
	return os.Getenv("CODESPACES") == "true"
}

// execute forwards url to the command named by the BROWSER env var.
func execute(url string) error {
	browserScript, ok := os.LookupEnv("BROWSER")
	if !ok || browserScript == "" {
		return fmt.Errorf("BROWSER environment variable is not set")
	}

	cmd := exec.Command(browserScript, url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %q: %w", browserScript, err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("running %q: %w", browserScript, err)
	}
	return nil
}

func usage(w *os.File) {
	fmt.Fprint(w, usageText)
}

func main() {
	if !checkCodespace() {
		fmt.Fprintln(os.Stderr, "error: this program is meant to be run in a GitHub Codespaces environment")
		os.Exit(1)
	}

	args := os.Args[1:]

	switch {
	case len(args) == 0:
		fmt.Fprintln(os.Stderr, "error: no URL provided")
		usage(os.Stderr)
		os.Exit(2)
	case len(args) > 1:
		fmt.Fprintln(os.Stderr, "error: too many arguments; expected a single URL")
		usage(os.Stderr)
		os.Exit(2)
	}

	arg := args[0]
	switch arg {
	case "-h", "--help":
		usage(os.Stdout)
		os.Exit(0)
	case "-v", "--version":
		fmt.Println(version)
		os.Exit(0)
	}

	if strings.TrimSpace(arg) == "" {
		fmt.Fprintln(os.Stderr, "error: URL is empty")
		os.Exit(2)
	}

	if err := execute(arg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
