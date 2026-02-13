# TODO

## Actions: Invoke Redfish Actions from Shell

### Background

Redfish Actions are POST operations on resources. They appear in the `Actions` property:

```json
"Actions": {
    "#ComputerSystem.Reset": {
        "target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset",
        "@Redfish.ActionInfo": "/redfish/v1/Systems/1/ResetActionInfo",
        "ResetType@Redfish.AllowableValues": ["On", "ForceOff", "GracefulShutdown"]
    }
}
```

With URI string detection (now implemented), `target` and `@Redfish.ActionInfo` are already `PropertyLink`s, so `open Actions/#ComputerSystem.Reset/@Redfish.ActionInfo` navigates to the ActionInfo resource which describes allowed parameters.

### What an `action` command could look like

```bash
# List available actions on current resource
action
#   #ComputerSystem.Reset  → ResetType: [On, ForceOff, GracefulShutdown]

# Invoke with arguments
action #ComputerSystem.Reset ResetType=GracefulShutdown
#   POST /redfish/v1/Systems/1/Actions/ComputerSystem.Reset
#   {"ResetType": "GracefulShutdown"}
#   → 200 OK

# Shorthand (strip the #Type. prefix)
action Reset ResetType=ForceOff
```

### Implementation Approach

**Client layer** — add `Post(path string, body []byte) ([]byte, error)` to `client.go`. Similar to `Fetch` but uses POST with JSON body.

**VFS layer** — add `InvokeAction(resourcePath, actionName string, params map[string]any) ([]byte, error)`:
1. Get the resource at `resourcePath`
2. Find the action in `resource.Properties["Actions"]`
3. Extract the `target` PropertyLink
4. Build JSON body from params
5. POST to target via client

**Shell layer** — add `action` command:
1. No args: list actions on current resource (parse Actions property, show names + AllowableValues)
2. With args: parse `Name key=value key=value`, invoke via VFS
3. Tab completion: complete action names, then parameter names from AllowableValues annotations

**Validation** — before POSTing:
- Check parameter names against ActionInfo if available
- Check parameter values against AllowableValues if present
- Show clear error if action doesn't exist

### Open Questions

1. Should `action` require confirmation before POSTing? (Probably yes — it's a write operation)
2. How to handle Actions with no parameters (like simple Reset with default)?
3. Should we display the response body? (Some actions return task URIs for long-running ops)
4. PATCH operations for property writes — same command or separate `set` command?

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

**Implementation Notes:**
- Use channels to communicate updates to main UI thread
- Show progress indicator during initial crawl
- Make it optional (flag to disable for slow BMCs)
- Respect rate limits to avoid overwhelming BMC
- Handle errors gracefully (some paths may 404)

**Priority:** Medium (orphan loading solves immediate problem)
