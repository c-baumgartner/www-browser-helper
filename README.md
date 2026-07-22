# www-browser-helper

A tiny wrapper that emulates `www-browser` inside [GitHub Codespaces](https://github.com/features/codespaces).

## Why

Many command-line tools (for example `git`, `gh`, `xdg-open`, or Debian's
`sensible-browser`) try to open a URL by running a program called
`www-browser`. Inside a Codespace there is no local browser, so those tools
fail or hang.

Codespaces instead exposes a `BROWSER` environment variable pointing at a helper
that forwards URLs to the browser on **your** machine. `www-browser-helper` bridges
the two: install it as `www-browser`, and any tool that shells out to `www-browser`
will transparently open the URL locally.

## How it works

```
tool → www-browser <url> → $BROWSER <url> → your local browser
```

1. Verifies it is running in a Codespace (`CODESPACES=true`).
2. Reads the `BROWSER` environment variable.
3. Runs `$BROWSER <url>` and waits for it to finish.

The URL is passed as a separate process argument (no shell is invoked), so URL
contents cannot inject shell commands.

## Install

### From a release (recommended)

Download a prebuilt `linux` binary from the
[latest release](https://github.com/c-baumgartner/www-browser-helper/releases/latest)
(currently [v1.0.0](https://github.com/c-baumgartner/www-browser-helper/releases/tag/v1.0.0)).
Pick the archive matching your architecture (`x86_64`, `arm64`, or `i386`):

```sh
VERSION=1.0.0
curl -sSfL -o www-browser-helper.tar.gz \
  "https://github.com/c-baumgartner/www-browser-helper/releases/download/v${VERSION}/www-browser-helper_Linux_x86_64.tar.gz"
tar -xzf www-browser-helper.tar.gz www-browser-helper
sudo install www-browser-helper /usr/local/bin/www-browser
```

Checksums are published alongside the archives (`www-browser-helper_${VERSION}_checksums.txt`).

### From source

```sh
go install github.com/c-baumgartner/www-browser-helper@latest
```

Then make it discoverable as `www-browser` on your `PATH`, e.g.:

```sh
sudo ln -s "$(go env GOPATH)/bin/www-browser-helper" /usr/local/bin/www-browser
```

## Usage

```
www-browser-helper <url>
www-browser-helper [flags]

Flags:
  -h, --help       show this help
  -v, --version    show version
```

Example:

```sh
www-browser-helper https://example.com
```

### Environment

| Variable     | Required | Description                                                        |
| ------------ | -------- | ------------------------------------------------------------------ |
| `CODESPACES` | yes      | Must be `true`; the tool refuses to run outside a Codespace.       |
| `BROWSER`    | yes      | Command used to open the URL (Codespaces sets this automatically). |

### Exit codes

| Code | Meaning                                            |
| ---- | -------------------------------------------------- |
| `0`  | Success (or `--help` / `--version`).               |
| `1`  | Not in a Codespace, or `$BROWSER` failed.          |
| `2`  | Bad invocation (no URL, empty URL, too many args). |

## Development

```sh
go build ./...
go vet ./...
CODESPACES=true BROWSER=echo go run . https://example.com
```

Releases are built with [GoReleaser](https://goreleaser.com/) via GitHub Actions
on tag push.

## License

[MIT](./LICENSE) © Christian Baumgartner
