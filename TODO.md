# TODO

## Parser: URI String Properties Should Be PropertyLinks

### Problem

Currently, string properties containing URIs are parsed as `PropertySimple` instead of `PropertyLink`, making them non-navigable:

```bash
# Current behavior (broken):
ll @Redfish.ActionInfo
# Shows: "/redfish/v1/Systems/1/ResetActionInfo" (just a string)
# Cannot: cd, open, or ll into it

# Expected behavior:
ll @Redfish.ActionInfo
# Shows: link → /redfish/v1/Systems/1/ResetActionInfo
# Can: open @Redfish.ActionInfo (navigates to target)
```

### Root Cause

Parser only detects PropertyLink for objects with `@odata.id`. String URIs are treated as simple values.

Affected properties:
- `@Redfish.ActionInfo` - always a URI string
- `*Uri` / `*URI` suffix properties - per DMTF spec convention
- `Target` in Actions - action endpoint URI

### DMTF Specification

From Redfish spec (DSP0266):
> "Non-resource reference properties provide a URI to services or documents that are not Redfish-defined resources, and these properties **shall include the Uri or URI term in their property name** and **shall be of type string**."

### Proposed Solution

Detect URI strings by property name convention:

```go
if property is string AND (
    name ends with "Uri" OR
    name ends with "URI" OR
    name == "@Redfish.ActionInfo" OR
    name == "Target" (when in Actions context)
) → PropertyLink (not PropertySimple)
```

**Benefits:**
- ✅ Spec-compliant (uses DMTF naming convention)
- ✅ Robust (contract-based, not pattern matching URI content)
- ✅ Maintainable (clear rule, easy to extend)

### Implementation

File: `rvfs/parser.go`

In `parseProperty()`, before `default:` case for simple values, add:

```go
// Check if string property name indicates it's a URI (should be PropertyLink)
if dataType == jsonparser.String {
    nameIsURI := strings.HasSuffix(name, "Uri") ||
                 strings.HasSuffix(name, "URI") ||
                 name == "@Redfish.ActionInfo" ||
                 name == "Target" // TODO: verify we're in Actions context

    if nameIsURI {
        strValue := string(value)
        // Strip quotes
        if len(strValue) >= 2 && strValue[0] == '"' && strValue[len(strValue)-1] == '"' {
            strValue = strValue[1 : len(strValue)-1]
        }
        prop.Type = PropertyLink
        prop.LinkTarget = strValue
        return prop
    }
}
```

### Testing

Add test cases:
- `@Redfish.ActionInfo` string → PropertyLink
- `AssemblyBinaryDataUri` → PropertyLink
- `ImageURI` → PropertyLink
- Regular strings → PropertySimple (unchanged)
- Verify navigation works: `open @Redfish.ActionInfo`

### Open Questions

1. `Target` detection - only in Actions, or broader?
2. Other URI patterns we're missing?
3. Should we validate the string looks like a path, or trust the name?

---

## TUI: Background Graph Crawling

### Problem

Currently, nodes are loaded lazily as the user navigates. This means:
- Following a link to an already-loaded (but not yet traversed) node won't have a parent pointer
- "Up" navigation from such nodes won't work even though a parent exists in RVFS cache
- We can't build a complete graph visualization without manual traversal

### Proposed Solution

Implement background graph crawler in TUI:

```go
// Start background goroutine that:
1. Walks entire tree from /redfish/v1
2. Loads all reachable resources
3. Populates nodeMap with all nodes
4. Links parent pointers for all nodes
5. Periodically refreshes stale cache entries
6. Discovers cross-links and updates the graph
```

**Benefits:**
- ✅ Complete graph in memory for visualization
- ✅ All parent pointers valid (enables "up" from any node)
- ✅ Fast navigation (everything pre-loaded)
- ✅ Cache stays fresh automatically
- ✅ Can build graph visualizations (like graphui experiments)

**Implementation Notes:**
- Use channels to communicate updates to main UI thread
- Show progress indicator during initial crawl
- Make it optional (flag to disable for slow BMCs)
- Respect rate limits to avoid overwhelming BMC
- Handle errors gracefully (some paths may 404)

**Priority:** Medium (orphan loading solves immediate problem)

---

## Other TODOs

*(Add other items here as needed)*
