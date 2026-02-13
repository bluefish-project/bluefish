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

**Hierarchical processing** (3 levels):

1. **Split by `/`** into segments (e.g., `["Systems", "1", "Status:Health"]`)
2. **For each segment**:
   - **If contains `:`**: Split by `:` and navigate property tree
   - **Else**: Check `Children` first, then `Properties` (with `[n]` array indexing)
3. **If PropertyLink + more segments**: Follow link, continue in target resource

**Example trace**: `/redfish/v1/Systems/1/Status:Health`

```
Split by /: ["redfish", "v1", "Systems", "1", "Status:Health"]

Process "redfish"  → Child nav → /redfish
Process "v1"       → Child nav → /redfish/v1
Process "Systems"  → Child nav → /redfish/v1/Systems (Collection)
Process "1"        → Children["1"] → fetch /redfish/v1/Systems/1 (ComputerSystem)
Process "Status:Health":
  ├─ Contains : → split ["Status", "Health"]
  ├─ Properties["Status"] → PropertyObject
  └─ .Children["Health"] → PropertySimple
```

### Why Path Structure Matters

**Segment boundaries** (where `/` splits) determine navigation mode:

✅ **VALID**: `/Systems/1/Status:Health`
- Segments: `["Systems", "1", "Status:Health"]`
- Navigate to Child "1", THEN access property "Status:Health"

❌ **INVALID**: `/Systems/1:Status:Health`
- Segments: `["Systems", "1:Status:Health"]`
- Tries to access Property "1" (doesn't exist, "1" is a Child!)

**The key**: In real Redfish systems, `/redfish/v1/Systems` has:
```json
{
  "Members": [{"@odata.id": "/redfish/v1/Systems/1"}]
}
```
This creates `Children["1"]`, NOT `Properties["1"]`.

### Client Code Rules

1. **NEVER parse paths** with `strings.Split`, `strings.Contains(path, ":")`, regex, etc.
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

❌ Parsing paths yourself: `strings.SplitN(path, ":", 2)`
❌ Type checking via strings: `if strings.Contains(path, ":")`
❌ Assuming structure: "anything with `:` is a property"
❌ Manual navigation: `resource.Properties[segments[0]].Children[segments[1]]`

✅ Use ResolveTarget for ALL path navigation
✅ Use returned `Type` enum to determine what you got
✅ Use typed fields (`Resource`, `Property`, `ResourcePath`)
✅ Let VFS handle all path complexity
