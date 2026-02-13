# Coding Guidelines for RFSH

## Core Principles

- **No backwards compatibility, legacy code, or adapters** - Always write clean, forward-looking code
- **No "enhanced/improved/optimized" naming** - Use direct, descriptive names without qualifiers
- **Update in-place** - Modify existing files rather than creating new test/script files unless adding new functionality
- **No change history in comments** - Git tracks history; code comments should only explain current implementation
- **No handwaving or simplifications** - All work should be production-grade and meet desgn goals, or explain why you can't

## RVFS Data Model & Path Resolution (CRITICAL - DO NOT FORGET)

### Two-Phase Architecture

1. **Parser (`rvfs/parser.go`)**: Does ALL JSON parsing, builds structured property trees
2. **VFS Clients**: Use typed results, NEVER parse paths or JSON themselves

**Rule: If you're doing string parsing on paths outside of `rvfs/parser.go` or `rvfs/vfs.go`, you're doing it wrong.**

### How Parser Classifies JSON

At parse time, for each top-level JSON key:

1. **Objects with ONLY `@odata.*` keys** → `Child` (navigable link)
   ```json
   "Storage": {"@odata.id": "/redfish/v1/Systems/1/Storage"}
   ```

2. **`Members` array with link-only objects** → `Children` (names extracted from paths)
   ```json
   "Members": [{"@odata.id": "/redfish/v1/Systems/1"}]
   ```
   Creates `Children["1"]` (NOT `Properties["1"]`)

3. **Everything else** → `Property` (recursive tree):
   - Objects with data → `PropertyObject`
   - Nested link-only objects → `PropertyLink`
   - Arrays → `PropertyArray`
   - Primitives → `PropertySimple`

### How ResolveTarget Processes Paths

**Unified `/` navigation** — all paths use `/` as the only separator:

1. **Split by `/`** into segments (e.g., `["Systems", "1", "Status", "Health"]`)
2. **For each segment**:
   - **In resource mode**: Check `Children` first, then `Properties`
   - **In property mode**: Check property `Children` (named fields)
   - `[n]` within a segment handles array indexing (e.g., `BootOrder[0]`)
3. **Mode transitions**:
   - Segment matches a `Property` → enter property mode
   - `PropertyLink` + more segments → follow link, back to resource mode
   - `PropertyObject` + more segments → descend into children, stay in property mode

**Example trace**: `/redfish/v1/Systems/1/Status/Health`

```
Split by /: ["redfish", "v1", "Systems", "1", "Status", "Health"]

Process "redfish"  → Child → /redfish
Process "v1"       → Child → /redfish/v1
Process "Systems"  → Child → /redfish/v1/Systems (Collection)
Process "1"        → Child → fetch /redfish/v1/Systems/1
Process "Status"   → not a Child → Property (PropertyObject) → enter property mode
Process "Health"   → property child → PropertySimple → return
```

**Example with link following**: `Oem/Supermicro/NodeManager/Id`

```
Process "Oem"          → Property (PropertyObject) → property mode
Process "Supermicro"   → property child (PropertyObject) → descend
Process "NodeManager"  → property child (PropertyLink) → follow link → resource mode
Process "Id"           → fetch linked resource → Property (PropertySimple) → return
```

### Why This Works

Children and Properties never collide on the same name. The Redfish spec ensures a top-level key is either a link-only object (`Child`) or a data object (`Property`), never both. So checking Children first, then Properties, is unambiguous.

### Client Code Rules

1. **NEVER parse paths** with `strings.Split`, regex, etc.
2. **ALWAYS use `vfs.ResolveTarget(basePath, targetPath)`**
3. **Use typed results**:
   ```go
   target, err := vfs.ResolveTarget(basePath, path)
   switch target.Type {
   case rvfs.TargetResource:  // Use target.Resource, target.ResourcePath
   case rvfs.TargetProperty:  // Use target.Property
   case rvfs.TargetLink:      // Use target.ResourcePath (link target)
   }
   ```
4. **Navigate via returned structures**:
   - `Resource.Children` - for child resources
   - `Resource.Properties` - for top-level properties
   - `Property.Children` - for nested properties
   - `Property.Elements` - for array elements

### Common Mistakes to AVOID

❌ Parsing paths yourself: `strings.SplitN(path, "/", 2)` then manual lookup
❌ Type checking via strings: `if strings.Contains(path, ":")`
❌ Manual navigation: `resource.Properties[segments[0]].Children[segments[1]]`

✅ Use ResolveTarget for ALL path navigation
✅ Use returned `Type` enum to determine what you got
✅ Use typed fields (`Resource`, `Property`, `ResourcePath`)
✅ Let VFS handle all path complexity
