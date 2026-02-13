# RFSH - Redfish Shell

A filesystem-style shell for navigating and exploring Redfish APIs. RFSH presents Redfish resources as directories and properties as files, letting you use familiar shell commands like `cd`, `ls`, and `ll` to interact with your BMC.

## Features

- **Filesystem metaphor**: Navigate Redfish APIs like a local filesystem
- **Unified `/` paths**: Resources and properties use the same separator
- **Array indexing**: Access array elements with `[n]` syntax
- **Smart caching**: Fetch-on-miss with persistent cache across sessions
- **Tab completion**: Context-aware completion for resources, properties, and array indices
- **TUI**: Tree-based browser with vim-style navigation

## Quick Start

### Installation

```bash
# Build CLI shell
go build -o rfsh ./cmd/rfsh

# Build TUI browser
go build -o rfui ./cmd/tview
```

### Configuration

Create a `config.yaml` file:

```yaml
endpoint: https://10.1.2.3
user: admin
pass: your_password
insecure: true  # Skip TLS verification (optional)
```

### Usage

```bash
./rfsh config.yaml
./rfui config.yaml
```

## Commands

### Navigation

```bash
cd Systems/1              # Navigate to a child resource
cd Status                 # Navigate into a property object
cd ..                     # Go up one level
cd ~                      # Return to /redfish/v1
open Links/Chassis[0]     # Follow a PropertyLink to its target resource
open .                    # Return to containing resource from a property path
pwd                       # Print working directory
```

### Viewing Data

```bash
ls                        # List children and properties
ll Status                 # Show formatted YAML-style output
ll Status/Health          # Show a nested property value
dump                      # Show raw JSON of current resource
dump Status               # Show raw JSON of a property
tree 3                    # Tree view with depth
```

### Search

```bash
find Health               # Search properties recursively (includes child resources)
```

### Other

```bash
cache                     # Show cache stats
cache list                # List cached resource paths
cache clear               # Clear cache
clear                     # Clear screen
help                      # Show command help
exit                      # Exit shell
```

## Path Syntax

All navigation uses `/` as the separator. Array elements use `[n]`.

```bash
# Resource paths
/redfish/v1/Systems/1

# Property paths (relative to current location)
Status/Health

# Array indexing
BootOrder[0]
Links/Drives[2]

# Deep property navigation
Boot/BootSourceOverrideTarget
```

### Special Paths

| Path | Meaning |
|------|---------|
| `.`  | Current location |
| `..` | Parent (resource or property) |
| `~`  | Root (`/redfish/v1`) |

### `cd` vs `open`

- `cd` navigates into resources and property objects/arrays
- `open` follows PropertyLinks to their target resource
- `open .` escapes a property path back to its containing resource

## Tab Completion

Press `Tab` for context-aware completion:

```bash
Systems/<Tab>             # Complete child resources
Status/<Tab>              # Complete property children
BootOrder[<Tab>           # Complete array indices
```

## TUI (rfui)

Tree-based browser with split-pane layout.

| Key | Action |
|-----|--------|
| `h/l` | Collapse/expand nodes |
| `j/k` | Navigate up/down |
| `s` | Subtree (rebase on current node) |
| `b` / Backspace | Go back |
| `u` | Go up to parent |
| `~` | Go to root |
| `r` | Refresh (clear cache, re-fetch) |
| `Enter` | Follow links |
| `J/K` | Scroll details panel |
| `q` | Quit |

## Project Structure

```
.
├── cmd/
│   ├── rfsh/                # CLI shell
│   │   ├── rfsh.go          # REPL, Navigator, commands
│   │   ├── completer.go     # Tab completion
│   │   └── completion_listener.go  # Tab prefetch
│   └── tview/               # TUI browser
│       └── main.go
├── rvfs/                    # Virtual filesystem library
│   ├── vfs.go               # VFS interface and path resolution
│   ├── types.go             # Data structures and error types
│   ├── cache.go             # Resource cache with fetch-on-miss
│   ├── client.go            # HTTP client with session auth
│   ├── parser.go            # JSON to property tree parser
│   └── vfs_test.go          # Tests
└── CLAUDE.md                # Development guidelines
```

## Architecture

```
┌─────────────────────────────────────┐
│      Consumers (rfsh, rfui)         │
│  Commands / Display / Completion    │
└──────────────────┬──────────────────┘
                   │
┌──────────────────▼──────────────────┐
│      RVFS (Virtual Filesystem)      │
│  ResolveTarget / ListAll / Get      │
└──────────────────┬──────────────────┘
                   │
┌──────────────────▼──────────────────┐
│     Cache (Fetch-on-miss)           │
│  Memory + Disk Persistence          │
└───────┬─────────────────────┬───────┘
        │                     │
┌───────▼──────┐      ┌───────▼───────┐
│    Client    │      │    Parser     │
│  HTTP + Auth │      │  JSON → Tree  │
└──────────────┘      └───────────────┘
```

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

## Cache Files

RFSH creates cache files in the current directory:

```
.rfsh_cache_<hostname>.json
```

Cache files are gitignored.
