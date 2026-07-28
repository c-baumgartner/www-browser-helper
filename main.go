package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

// version is set at build time via -ldflags (GoReleaser). For `go install`
// builds it stays "dev" and is resolved from the module build info instead.
var version = "dev"

// resolveVersion returns the ldflags value when set, otherwise the module
// version recorded by `go install module@vX.Y.Z` (falls back to "dev").
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

const usageText = `www-browser-helper - open URLs from inside a VS Code remote session

Usage:
  www-browser-helper [flags] <url|path>

It asks the VS Code server to open <url> on your local machine. When
VSCODE_IPC_HOOK_CLI is set it talks to the server's IPC socket directly;
otherwise it falls back to forwarding the argument to $BROWSER.

Point BROWSER at this binary to make tools that honour $BROWSER (git, gh,
xdg-open wrappers, ...) open links locally without spawning Node.

Flags:
  -h, --help       show this help
  -v, --version    show version
  -V, --verbose    trace what happens to stderr (alias: --debug)

Environment:
  VSCODE_IPC_HOOK_CLI        VS Code server IPC socket; preferred transport
  BROWSER                    fallback command used to open the URL
  WWW_BROWSER_HELPER_DEBUG   set to 1/true/yes/on to force --verbose

Tools that invoke this program as $BROWSER cannot pass flags; use
WWW_BROWSER_HELPER_DEBUG=1 to trace those calls.
`

// ipcTimeout bounds a single openExternal round trip.
const ipcTimeout = 10 * time.Second

// debugEnabled turns on tracing to stderr.
var debugEnabled bool

// debugf writes a trace line to stderr when verbose mode is on. Output goes to
// stderr so it never mixes into anything a caller might be parsing on stdout.
func debugf(format string, args ...any) {
	if !debugEnabled {
		return
	}
	fmt.Fprintf(os.Stderr, "www-browser-helper: "+format+"\n", args...)
}

// truthy reports whether an env var value means "on".
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// absoluteURI matches the arguments the VS Code CLI treats as URIs rather than
// as filesystem paths. Kept identical to the check in server-cli.js.
var absoluteURI = regexp.MustCompile(`^[a-z-]+://.+`)

// normalizeURI mirrors the VS Code CLI: absolute URIs are normalized, anything
// else is resolved against the working directory and turned into a file: URI.
func normalizeURI(arg string) (string, error) {
	arg = strings.TrimSpace(arg)

	if absoluteURI.MatchString(arg) {
		u, err := url.Parse(arg)
		if err != nil {
			return "", fmt.Errorf("invalid url %q: %w", arg, err)
		}
		// Match what url.parse().href produces on the Node side: lowercase
		// host, and a bare authority gets a root path.
		u.Host = strings.ToLower(u.Host)
		if u.Path == "" && u.Host != "" {
			u.Path = "/"
		}
		debugf("argument looks like an absolute URI (scheme %q)", u.Scheme)
		return u.String(), nil
	}

	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", fmt.Errorf("resolving path %q: %w", arg, err)
	}
	cwd, _ := os.Getwd()
	debugf("argument is not an absolute URI; resolved against cwd %q", cwd)
	return (&url.URL{Scheme: "file", Path: abs}).String(), nil
}

// openExternal asks the VS Code server to open uri on the user's local machine.
// The wire format is the one server-cli.js uses: a JSON POST to / over the
// server's Unix domain socket.
func openExternal(sock, uri string) error {
	body, err := json.Marshal(struct {
		Type string   `json:"type"`
		URIs []string `json:"uris"`
	}{Type: "openExternal", URIs: []string{uri}})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				debugf("dialing unix socket %s", sock)
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), ipcTimeout)
	defer cancel()

	// The host in the URL is ignored; the transport always dials the socket.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://vscode/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	debugf("POST / (timeout %s) body: %s", ipcTimeout, body)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to VS Code server socket %q: %w", sock, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	debugf("response %s in %s, content-type %q, body: %s",
		resp.Status, time.Since(start).Round(time.Millisecond),
		resp.Header.Get("Content-Type"), strings.TrimSpace(string(payload)))

	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(payload))
		if detail == "" {
			detail = "no response body"
		}
		return fmt.Errorf("VS Code server returned %s: %s", resp.Status, detail)
	}

	debugf("VS Code accepted the openExternal request")
	return nil
}

// execute forwards url to the command named by the BROWSER env var.
func execute(browserScript, url string) error {
	debugf("exec: %s %s", browserScript, url)

	cmd := exec.Command(browserScript, url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %q: %w", browserScript, err)
	}
	debugf("child pid %d started, waiting", cmd.Process.Pid)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("running %q: %w", browserScript, err)
	}
	debugf("child exited 0 after %s", time.Since(start).Round(time.Millisecond))
	return nil
}

// isSelf reports whether cmd resolves to this binary, which would make the
// $BROWSER fallback recurse forever.
func isSelf(cmd string) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	target, err := exec.LookPath(cmd)
	if err != nil {
		return false
	}
	return sameFile(self, target)
}

func sameFile(a, b string) bool {
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

func usage(w *os.File) {
	fmt.Fprint(w, usageText)
}

// open picks a transport and hands the argument over to it.
func open(arg string) error {
	sock := os.Getenv("VSCODE_IPC_HOOK_CLI")
	browserScript := os.Getenv("BROWSER")

	debugf("VSCODE_IPC_HOOK_CLI=%s", quoteEnv(sock))
	debugf("BROWSER=%s", quoteEnv(browserScript))

	if sock != "" {
		debugf("transport: VS Code server IPC socket (no Node involved)")
		uri, err := normalizeURI(arg)
		if err != nil {
			return err
		}
		debugf("normalized %q -> %q", arg, uri)
		return openExternal(sock, uri)
	}

	switch {
	case browserScript == "":
		return fmt.Errorf("neither VSCODE_IPC_HOOK_CLI nor BROWSER is set; " +
			"run this inside a VS Code remote session (Codespaces, dev container, Remote-SSH)")
	case isSelf(browserScript):
		return fmt.Errorf("BROWSER points at this binary (%s) and VSCODE_IPC_HOOK_CLI is not set; "+
			"refusing to recurse", browserScript)
	}

	debugf("transport: $BROWSER fallback (VSCODE_IPC_HOOK_CLI is unset)")
	return execute(browserScript, arg)
}

// quoteEnv renders an env var value for a trace line, marking absent ones.
func quoteEnv(v string) string {
	if v == "" {
		return "<unset>"
	}
	return fmt.Sprintf("%q", v)
}

// parseArgs splits argv into flags and the single positional argument. It
// returns the URL, or an empty string when a flag already handled everything.
func parseArgs(args []string) (string, error) {
	var positional []string

	for _, a := range args {
		switch a {
		case "-h", "--help":
			usage(os.Stdout)
			os.Exit(0)
		case "-v", "--version":
			fmt.Println(resolveVersion())
			os.Exit(0)
		case "-V", "--verbose", "--debug":
			debugEnabled = true
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return "", fmt.Errorf("unknown flag %q", a)
			}
			positional = append(positional, a)
		}
	}

	switch {
	case len(positional) == 0:
		return "", fmt.Errorf("no URL provided")
	case len(positional) > 1:
		return "", fmt.Errorf("too many arguments; expected a single URL")
	}

	if strings.TrimSpace(positional[0]) == "" {
		return "", fmt.Errorf("URL is empty")
	}
	return positional[0], nil
}

func main() {
	if truthy(os.Getenv("WWW_BROWSER_HELPER_DEBUG")) {
		debugEnabled = true
	}

	arg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		usage(os.Stderr)
		os.Exit(2)
	}

	debugf("version %s, pid %d, argv %q", resolveVersion(), os.Getpid(), os.Args)

	if err := open(arg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
