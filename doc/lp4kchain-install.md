# lp4kchain — Installation Guide

**lp4kchain** generates Mermaid diagrams from lp4k CSV output. The Go binary itself has no external dependencies and is built with `make tools`.

To render PNG/SVG/PDF output directly, `lp4kchain` invokes [mermaid-cli (mmdc)](https://github.com/mermaid-js/mermaid-cli) which requires Node.js and a headless Chrome browser.

## Install mmdc

### macOS (Homebrew)

```bash
brew install mermaid-cli
```

Then install the required headless Chrome:

```bash
npx puppeteer browsers install chrome-headless-shell
```

### npm (all platforms)

```bash
npm install -g @mermaid-js/mermaid-cli
npx puppeteer browsers install chrome-headless-shell
```

## Chrome version mismatch

If `mmdc` fails with `Could not find Chrome (ver. X.Y.Z)`, the installed Chrome version may differ from what your mmdc version expects. Set the `PUPPETEER_EXECUTABLE_PATH` environment variable to point to the installed binary:

```bash
# Find installed browser
ls ~/.cache/puppeteer/chrome-headless-shell/

# Export the path (adjust version to match your installation)
export PUPPETEER_EXECUTABLE_PATH=~/.cache/puppeteer/chrome-headless-shell/mac_arm-150.0.7871.24/chrome-headless-shell-mac-arm64/chrome-headless-shell
```

## Usage without mmdc

If you don't want to install mmdc, `lp4kchain` can still generate `.mmd` (Mermaid source) files:

```bash
./bin/lp4kchain lp4k-output.csv output.mmd
```

You can then render the diagram using:
- [mermaid.live](https://mermaid.live) — paste the `.mmd` content in the browser
- GitHub — `.mmd` files render natively in GitHub markdown and code view
- VS Code — install the "Mermaid Markdown Syntax Highlighting" extension

## Usage with mmdc

When mmdc is installed and available in `$PATH`, `lp4kchain` can render PNG, SVG, or PDF directly. When the output file ends in `.png`, `.svg`, or `.pdf`, the tool:

1. Writes a `.mmd` source file alongside the output (e.g. `replacement-chains.mmd`)
2. Invokes `mmdc` to render it to the requested format
3. If `mmdc` is not found, exits with an error pointing to the already-written `.mmd` file

```bash
# Full pipeline: parse logs, generate PNG
./bin/lp4k karpenter-logs.json > output.csv
./bin/lp4kchain output.csv replacement-chains.png
```
