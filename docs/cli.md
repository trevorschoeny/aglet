# CLI Reference

## Install

```bash
go install github.com/trevorschoeny/aglet@latest
```

Requires Go 1.22+. The binary is installed to `$GOPATH/bin/aglet` (typically `~/go/bin/aglet`).

## Commands

### `aglet run`

Find and execute a Block by name.

```bash
echo '{"url":"https://example.com"}' | aglet run FetchPage
aglet run FetchPage input.json
```

Scans the project for a Block with the given name, resolves its runner from `domain.yaml`, and executes it. Works with process and reasoning Blocks. Input comes from a file argument, stdin, or defaults to `{}`.

### `aglet reason`

Execute a reasoning Block directly from its directory.

```bash
aglet reason ./TagBookmark input.json
```

Skips discovery — you point directly at the Block directory. Useful during development when you want to iterate on a reasoning Block's prompt.

### `aglet pipe`

Execute a pipeline by following `calls` edges.

```bash
echo '{"url":"https://example.com"}' | aglet pipe FetchPage
echo '{"url":"https://example.com"}' | aglet pipe FetchPage SaveBookmark
```

With one argument, follows `calls` edges linearly to a terminal Block. With two arguments, finds the shortest path via BFS. Each Block's stdout feeds into the next Block's stdin.

### `aglet serve`

Start an HTTP dev server from a Surface's contract.

```bash
aglet serve
aglet serve --port 3001
```

Maps each contract dependency in `surface.yaml` to a `POST /contract/<DependencyName>` endpoint. The Surface makes standard HTTP requests — it never knows whether `aglet serve` or production infrastructure is answering.

### `aglet validate`

Check project integrity and auto-fix issues.

```bash
aglet validate
```

Scans the entire project, runs structural checks, and auto-fixes what it can in a single pass. See the [README](../README.md#aglet-validate) for details on what it checks and fixes.
