# Go Pulse

A simple, dependency-free tool that automatically rebuilds and runs your Go program whenever files are changed.

## Features

- Monitors your Go project files for changes
- Automatically rebuilds and reruns your program when changes are detected
- Configurable via simple JSON file
- No external dependencies - uses the Go standard library only

## Installation

```
go get -tool github.com/cc-jj/pulse@latest

```

## Usage

Run it from any Go project directory

```
# initialize pulse with a default config in the current working directory
go tool -init

# print the verison
go tool pulse -v

# run specifying the config path
go tool pulse -c=/path/to/pulse.json

# run using the default config path (./pulse.json)
go tool pulse
```

## Configuration

The tool can be customized via a json file. Here's an example:

```json
{
  "main_file": "main.go",
  "binary_name": "app",
  "watch_dir": ".",
  "watch_exts": [".go", ".mod", ".sum"],
  "watch_interval": "1s"
}
```

## Configuration Options

| Option           | Description                                                 | Default                   |
| ---------------- | ----------------------------------------------------------- | ------------------------- |
| `main_file`      | The main Go file to build and run                           | `"main.go"`               |
| `binary_name`    | The name of the compiled binary                             | `"app"`                   |
| `watch_dir`      | The directory to watch for changes                          | `"."`                     |
| `watch_exts`     | File extensions to watch for changes                        | `[".go", ".mod", ".sum"]` |
| `watch_interval` | How often to check for file changes (in Go duration format) | `"1s"`                    |
| `max_watchers`   | Prevent watching more than this many files                  | `100`                     |

Note that all paths (`main_file`, `binary_name`, and `watch_dir`) as relative to the current working directory.

The `watch_interval` accepts standard Go duration strings like "500ms", "1s", "2.5s", "1m", etc. The minimum allowed interval is 500ms and the maximum is 1 hour.

The minimum allowed `max_watchers` is 1. The maximum is 500.

## How It Works

1. The tool recursively watches the specified directory for file changes
2. When a file with a matching extension is modified, added, or deleted
3. The tool stops any running process from the previous build
4. It rebuilds your Go program
5. It runs the newly built binary

## License

MIT
