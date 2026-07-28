# www-browser-helper

A tiny helper that opens URLs on your local machine from inside a VS Code remote
session — [GitHub Codespaces](https://github.com/features/codespaces), dev
containers, or Remote-SSH.

## Why

Many command-line tools (for example `git`, `gh`, `xdg-open`, or Debian's
`sensible-browser`) try to open a URL by running a program called
`www-browser`. Inside a Codespace there is no local browser, so those tools
fail or hang.

VS Code remote sessions expose an IPC socket (`VSCODE_IPC_HOOK_CLI`) that the
server uses to relay `openExternal` requests to the editor, which then opens the
link in **your** browser. `www-browser-helper` talks to that socket directly:
install it as `www-browser`, and any tool that shells out to `www-browser` will
transparently open the URL locally.

The `BROWSER` variable that Codespaces sets points at a shell script that spawns
Node and the bundled VS Code CLI to do the same thing. Going straight to the
socket avoids that process tree — and the `DEP0169 url.parse()` deprecation
warning the bundled CLI prints on every call.

## How it works

```
tool → www-browser <url> → $VSCODE_IPC_HOOK_CLI socket → VS Code → your local browser
```

1. Reads `VSCODE_IPC_HOOK_CLI` (the VS Code server's Unix domain socket).
2. Normalizes the argument: absolute URIs are passed through, anything else is
   resolved against the working directory as a `file:` URI.
3. `POST`s `{"type":"openExternal","uris":["…"]}` to `/` over that socket and
   waits for the server's response.

If `VSCODE_IPC_HOOK_CLI` is not set it falls back to running `$BROWSER <url>`,
which is the classic Codespaces path. It refuses the fallback when `$BROWSER`
resolves to this same binary, so a `BROWSER=www-browser` setup cannot recurse.

The URL is passed as a separate process argument (no shell is invoked), so URL
contents cannot inject shell commands.

## Install

### From a release (recommended)

Download a prebuilt `linux` binary from the
[latest release](https://github.com/c-baumgartner/www-browser-helper/releases/latest).
Pick the archive matching your architecture (`x86_64`, `arm64`, or `i386`):

```sh
VERSION=2.0.1
curl -sSfL -o www-browser-helper.tar.gz \
  "https://github.com/c-baumgartner/www-browser-helper/releases/download/v${VERSION}/www-browser-helper_Linux_x86_64.tar.gz"
tar -xzf www-browser-helper.tar.gz www-browser-helper
sudo install www-browser-helper /usr/local/bin/www-browser
```

Checksums are published alongside the archives (`www-browser-helper_${VERSION}_checksums.txt`).

### From source

```sh
go install github.com/c-baumgartner/www-browser-helper/v2@latest
```

Then make it discoverable as `www-browser` on your `PATH`, e.g.:

```sh
sudo ln -s "$(go env GOPATH)/bin/www-browser-helper" /usr/local/bin/www-browser
```

### Optional: replace `$BROWSER` too

Tools that honour `BROWSER` rather than `www-browser` can be pointed at this
binary as well, which skips the Node-based helper script entirely:

```sh
export BROWSER=/usr/local/bin/www-browser
```

## Usage

```
www-browser-helper [flags] <url|path>

Flags:
  -h, --help       show this help
  -v, --version    show version
  -V, --verbose    trace what happens to stderr (alias: --debug)
```

Examples:

```sh
www-browser-helper https://example.com
www-browser-helper ./docs/index.html   # opened as file:///…/docs/index.html
```

### Which transport am I on?

Both paths just open a tab, so they look identical from the outside. `--verbose`
traces the decision to stderr:

```sh
$ www-browser-helper --verbose https://example.com
www-browser-helper: version v2.0.0, pid 4711, argv ["www-browser-helper" "--verbose" "https://example.com"]
www-browser-helper: VSCODE_IPC_HOOK_CLI="/tmp/vscode-ipc-….sock"
www-browser-helper: BROWSER="/vscode/…/bin/helpers/browser.sh"
www-browser-helper: transport: VS Code server IPC socket (no Node involved)
www-browser-helper: argument looks like an absolute URI (scheme "https")
www-browser-helper: normalized "https://example.com" -> "https://example.com/"
www-browser-helper: POST / (timeout 10s) body: {"type":"openExternal","uris":["https://example.com/"]}
www-browser-helper: dialing unix socket /tmp/vscode-ipc-….sock
www-browser-helper: response 200 OK in 2ms, content-type "application/json", body: {}
www-browser-helper: VS Code accepted the openExternal request
```

The `transport:` line is the one that answers the question. On the fallback path
it reads `$BROWSER fallback (VSCODE_IPC_HOOK_CLI is unset)` and is followed by
the `exec:` line and the child's pid.

Tools that invoke this program as `$BROWSER` (or as `www-browser`) can't pass
flags, so use the env var instead:

```sh
WWW_BROWSER_HELPER_DEBUG=1 gh browse
```

Traces go to stderr only, so stdout stays clean for anything parsing it.

### Environment

| Variable                   | Required | Description                                                              |
| -------------------------- | -------- | ------------------------------------------------------------------------ |
| `VSCODE_IPC_HOOK_CLI`      | no\*     | VS Code server IPC socket; the preferred transport (set automatically).   |
| `BROWSER`                  | no\*     | Fallback command used to open the URL when the IPC socket is unavailable. |
| `WWW_BROWSER_HELPER_DEBUG` | no       | Set to `1`/`true`/`yes`/`on` to force verbose tracing.                   |

\* At least one of the two must be set.

### Exit codes

| Code | Meaning                                                                  |
| ---- | ------------------------------------------------------------------------ |
| `0`  | Success (or `--help` / `--version`).                                     |
| `1`  | No transport available, IPC request failed, or `$BROWSER` failed.         |
| `2`  | Bad invocation (no URL, empty URL, too many args).                        |

## Development

```sh
go build ./...
go vet ./...

# fallback path
BROWSER=echo go run . https://example.com

# native path, inside a VS Code remote session
go run . https://example.com
```

Releases are built with [GoReleaser](https://goreleaser.com/) via GitHub Actions
on tag push.

## License

[MIT](./LICENSE) © Christian Baumgartner
