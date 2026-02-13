# RFSH - Redfish Shell

A filesystem-style shell for navigating and exploring Redfish APIs. RFSH presents Redfish resources as directories and properties as files, letting you use familiar shell commands like `cd`, `ls`, and `ll` to interact with your BMC.

## Features

- **Filesystem metaphor**: Navigate Redfish APIs like a local filesystem
- **Composite paths**: Seamlessly combine resource navigation (`/`) and property access (`:`)
- **Array indexing**: Access array elements with intuitive `[n]` syntax
- **Smart caching**: Automatic fetch-on-miss with persistent cache across sessions
- **Tab completion**: Context-aware completion for resources, properties, and array indices
- **Offline mode**: Browse cached data without network access

## Quick Start

### Installation

```bash
# Build CLI shell
go build -o rfsh ./cmd/rfsh

# Build TUI (coming soon)
go build -o rfui ./cmd/rfui
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
```

## Basic Commands

### Navigation

```bash
# Change to a resource (preserves composite paths)
cd Systems/1

# Navigate to a property path
cd Systems/@Redfish.CollectionCapabilities

# Navigate with array indexing
cd Links:Drives[0]

# Open follows links and canonicalizes paths
open Links:Drives[0]

# Show current location
pwd
```

### Viewing Data

```bash
# List children and properties
ls

# Show detailed YAML-style output
ll Status
ll Status:Health
ll Members[0]

# Show raw JSON
dump

# Tree view with depth
tree 3
```

### Other Commands

```bash
# Search for properties matching pattern
find Health

# Toggle flat vs hierarchical property view
flat

# Cache management
cache          # Show stats
cache list     # List cached paths
cache clear    # Clear cache

# Clear screen
clear

# Exit
exit
```

## Path Syntax

RFSH uses three separators for navigation:

| Separator | Purpose | Example |
|-----------|---------|---------|
| `/` | Navigate resources | `/redfish/v1/Systems/1` |
| `:` | Navigate properties | `Status:Health` |
| `[n]` | Index arrays | `Members[0]` |

### Path Examples

```bash
# Resource path
/redfish/v1/Systems/1

# Property path (relative)
Status:Health

# Composite path (resource + property)
/redfish/v1/Systems/1/Status:Health

# Array indexing
Members[0]
Links:Drives[2]

# Complex composition
Systems/@Redfish.CollectionCapabilities:Capabilities[0]:Links:RelatedItem[0]
```

### Special Paths

- `.` - Current location
- `..` - Parent resource
- `~` - Root (`/redfish/v1`)

## Tab Completion

Press `Tab` for context-aware completion:

```bash
# Complete resource children
Systems/<Tab>

# Complete property names
Status:<Tab>

# Complete array indices
Members[<Tab>
```

**Invalid syntax gets no completions:**
- `ArrayProperty:` → nothing (use `[` for arrays)
- `ObjectProperty[` → nothing (use `:` for properties)

## Project Structure

```
.
├── cmd/
│   ├── rfsh/            # Shell (CLI) implementation
│   │   ├── rfsh.go      # Main REPL, Navigator, commands
│   │   ├── completer.go # Tab completion logic
│   │   └── ...          # Other shell-specific files
│   └── rfui/            # TUI implementation (coming soon)
├── rvfs/                # Shared VFS library
│   ├── rvfs.go          # VFS interface and path resolution
│   ├── cache.go         # Resource cache with fetch-on-miss
│   ├── client.go        # HTTP client with session auth
│   └── parser.go        # JSON to property tree parser
├── config.yaml          # Configuration (not in git)
├── DATA_MODEL.md        # Path syntax and resolution design
├── NAVIGATOR_DESIGN.md  # Shell commands and UI design
└── RVFS_DESIGN.md       # VFS architecture and API design
```

## Architecture

```
┌─────────────────────────────────────┐
│         Navigator (Shell)           │
│  Commands • Display • Completion    │
└──────────────────┬──────────────────┘
                   │
┌──────────────────▼──────────────────┐
│      RVFS (Virtual Filesystem)      │
│  Path Resolution • Target Types     │
└──────────────────┬──────────────────┘
                   │
┌──────────────────▼──────────────────┐
│     Cache (Fetch-on-miss)           │
│  Memory + Disk • Offline Mode       │
└───────┬─────────────────────┬───────┘
        │                     │
┌───────▼──────┐      ┌───────▼───────┐
│    Client    │      │    Parser     │
│  HTTP + Auth │      │  JSON → Tree  │
└──────────────┘      └───────────────┘
```

### Component Responsibilities

| Component | Purpose |
|-----------|---------|
| **Navigator** | Shell REPL, command routing, display formatting |
| **RVFS** | Path resolution, target type determination, unified API |
| **Cache** | Resource storage, transparent fetching, persistence |
| **Client** | HTTP operations, session management, TLS configuration |
| **Parser** | JSON structure analysis, property tree construction |

## Design Documents

For detailed design information:

- **[DATA_MODEL.md](DATA_MODEL.md)** - Path syntax, property types, resolution algorithm
- **[NAVIGATOR_DESIGN.md](NAVIGATOR_DESIGN.md)** - Shell commands, tab completion, display formatting
- **[RVFS_DESIGN.md](RVFS_DESIGN.md)** - VFS architecture, caching, API reference

## Development

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./rvfs -v

# With coverage
go test ./... -cover
```

### Test Structure

- **rvfs/vfs_test.go** - Path resolution, composite paths, array indexing
- **completer_test.go** - Tab completion, separator semantics
- **rfsh_test.go** - Display formatting tests

### Adding Commands

Commands are registered in `rfsh.go`:

1. Add command to registry
2. Implement handler function
3. Add to help text
4. Add tab completion if needed

## Cache Files

RFSH creates cache files in the current directory:

```
.rfsh_cache_<identifier>.json
```

Cache files are excluded from git via `.gitignore`.

## Examples

### Exploring Systems

```bash
/redfish/v1> cd Systems
/redfish/v1/Systems> ls
1    2    @Redfish.CollectionCapabilities

/redfish/v1/Systems> cd 1
/redfish/v1/Systems/1> ll Status
Status:
  Health: OK
  State: Enabled

/redfish/v1/Systems/1> ll Boot:BootOrder[0]
Pxe
```

### Following Links

```bash
/redfish/v1/Systems/1> cd Links:Chassis[0]
/redfish/v1/Systems/1/Links:Chassis[0]> open .
/redfish/v1/Chassis/1>
```

### Composite Path Navigation

```bash
/redfish/v1/Systems> cd @Redfish.CollectionCapabilities:Capabilities[0]
/redfish/v1/Systems/@Redfish.CollectionCapabilities:Capabilities[0]> ll Links
Links:
  TargetCollection:
    link → /redfish/v1/Systems
  RelatedItem:
    - link → /redfish/v1/CompositionService/ResourceZones/1
```

## License

*[Add your license information here]*

## Contributing

*[Add contribution guidelines here]*
