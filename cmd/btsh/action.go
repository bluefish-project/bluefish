package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bluefish-project/bluefish/rvfs"
)

// ActionInfo describes a Redfish action on a resource
type ActionInfo struct {
	Name      string
	ShortName string
	Target    string
	InfoURI   string
	Allowable map[string][]string
}

// discoverActions finds all actions on the resource at nav.cwd
func discoverActions(nav *Navigator) ([]ActionInfo, error) {
	resolved, err := nav.vfs.ResolveTarget(rvfs.RedfishRoot, nav.cwd)
	if err != nil {
		return nil, err
	}

	var resource *rvfs.Resource
	switch resolved.Type {
	case rvfs.TargetResource, rvfs.TargetLink:
		resource = resolved.Resource
		if resource == nil {
			resource, err = nav.vfs.Get(resolved.ResourcePath)
			if err != nil {
				return nil, err
			}
		}
	case rvfs.TargetProperty:
		resource = resolved.Resource
	}
	if resource == nil {
		return nil, nil
	}

	actionsProp, ok := resource.Properties["Actions"]
	if !ok || actionsProp.Type != rvfs.PropertyObject {
		return nil, nil
	}

	var actions []ActionInfo
	for key, child := range actionsProp.Children {
		if key == "Oem" {
			continue
		}

		info := ActionInfo{
			Name:      key,
			Allowable: make(map[string][]string),
		}

		if idx := strings.LastIndex(key, "."); idx != -1 && strings.HasPrefix(key, "#") {
			info.ShortName = key[idx+1:]
		} else {
			info.ShortName = key
		}

		if child.Type != rvfs.PropertyObject {
			continue
		}

		for childKey, childProp := range child.Children {
			if childKey == "target" && childProp.Type == rvfs.PropertyLink {
				info.Target = childProp.LinkTarget
			} else if childKey == "@Redfish.ActionInfo" && childProp.Type == rvfs.PropertyLink {
				info.InfoURI = childProp.LinkTarget
			} else if strings.HasSuffix(childKey, "@Redfish.AllowableValues") && childProp.Type == rvfs.PropertyArray {
				paramName := strings.TrimSuffix(childKey, "@Redfish.AllowableValues")
				var values []string
				for _, elem := range childProp.Elements {
					if elem.Type == rvfs.PropertySimple {
						if s, ok := elem.Value.(string); ok {
							values = append(values, s)
						}
					}
				}
				info.Allowable[paramName] = values
			}
		}

		if info.Target != "" {
			actions = append(actions, info)
		}
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ShortName < actions[j].ShortName
	})
	return actions, nil
}

// matchAction finds an action by short name or full name (case-insensitive)
func matchAction(actions []ActionInfo, name string) *ActionInfo {
	lower := strings.ToLower(name)
	for i := range actions {
		if strings.ToLower(actions[i].ShortName) == lower {
			return &actions[i]
		}
	}
	for i := range actions {
		if strings.ToLower(actions[i].Name) == lower {
			return &actions[i]
		}
	}
	return nil
}

// formatActionList formats available actions
func formatActionList(actions []ActionInfo) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(errorStyle.Render("Actions"))
	b.WriteString("\n")
	for _, a := range actions {
		line := fmt.Sprintf("  %s", warnStyle.Render(a.ShortName))
		if len(a.Allowable) > 0 {
			var params []string
			for param, vals := range a.Allowable {
				params = append(params, fmt.Sprintf("%s=[%s]", param, strings.Join(vals, "|")))
			}
			sort.Strings(params)
			line += fmt.Sprintf("  %s", strings.Join(params, " "))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// formatActionDetail formats detailed info for one action
func formatActionDetail(nav *Navigator, action *ActionInfo) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(errorStyle.Render(action.Name))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  Target: %s\n", action.Target)

	if action.InfoURI != "" {
		fmt.Fprintf(&b, "  ActionInfo: %s\n", action.InfoURI)

		resource, err := nav.vfs.Get(action.InfoURI)
		if err == nil {
			paramsProp, ok := resource.Properties["Parameters"]
			if ok && paramsProp.Type == rvfs.PropertyArray {
				b.WriteString("\n  Parameters:\n")
				for _, elem := range paramsProp.Elements {
					if elem.Type != rvfs.PropertyObject {
						continue
					}
					name := ""
					dataType := ""
					required := false
					var allowable []string

					if n, ok := elem.Children["Name"]; ok && n.Type == rvfs.PropertySimple {
						name = fmt.Sprintf("%v", n.Value)
					}
					if dt, ok := elem.Children["DataType"]; ok && dt.Type == rvfs.PropertySimple {
						dataType = fmt.Sprintf("%v", dt.Value)
					}
					if r, ok := elem.Children["Required"]; ok && r.Type == rvfs.PropertySimple {
						if bv, ok := r.Value.(bool); ok {
							required = bv
						}
					}
					if av, ok := elem.Children["AllowableValues"]; ok && av.Type == rvfs.PropertyArray {
						for _, v := range av.Elements {
							if v.Type == rvfs.PropertySimple {
								allowable = append(allowable, fmt.Sprintf("%v", v.Value))
							}
						}
					}

					reqStr := ""
					if required {
						reqStr = errorStyle.Render(" (required)")
					}
					fmt.Fprintf(&b, "    %s%s  %s", warnStyle.Render(name), reqStr, dataType)
					if len(allowable) > 0 {
						fmt.Fprintf(&b, "  [%s]", strings.Join(allowable, "|"))
					}
					b.WriteString("\n")
				}
			}
		}
	}

	if len(action.Allowable) > 0 {
		if action.InfoURI == "" {
			b.WriteString("\n  Parameters:\n")
		} else {
			b.WriteString("\n  Allowable values (from annotations):\n")
		}
		for param, vals := range action.Allowable {
			fmt.Fprintf(&b, "    %s: [%s]\n", warnStyle.Render(param), strings.Join(vals, "|"))
		}
	}

	b.WriteString("\n")
	return b.String()
}

// parseActionBody parses key=value arguments into a JSON body
func parseActionBody(action *ActionInfo, args []string) ([]byte, error) {
	body := make(map[string]any)
	for _, arg := range args {
		idx := strings.Index(arg, "=")
		if idx == -1 {
			return nil, fmt.Errorf("invalid argument %q (expected key=value)", arg)
		}
		key := arg[:idx]
		val := arg[idx+1:]

		if allowed, ok := action.Allowable[key]; ok {
			found := false
			for _, a := range allowed {
				if a == val {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("invalid value %q for %s (allowed: %s)", val, key, strings.Join(allowed, ", "))
			}
		}

		if n, err := strconv.ParseFloat(val, 64); err == nil {
			if n == float64(int64(n)) {
				body[key] = int64(n)
			} else {
				body[key] = n
			}
		} else if val == "true" {
			body[key] = true
		} else if val == "false" {
			body[key] = false
		} else {
			body[key] = val
		}
	}

	return json.MarshalIndent(body, "", "  ")
}

// formatActionConfirm formats the confirmation prompt
func formatActionConfirm(action *ActionInfo, jsonBody []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s %s\n", errorStyle.Render("POST"), action.Target)
	if len(jsonBody) > 2 { // Not just "{}"
		b.WriteString(string(jsonBody))
		b.WriteString("\n")
	}
	return b.String()
}

// formatActionResult formats the result of a POST
func formatActionResult(status int, data []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nHTTP %d\n", status)
	if len(data) > 0 {
		var buf bytes.Buffer
		if json.Indent(&buf, data, "", "  ") == nil {
			b.WriteString(buf.String())
		} else {
			b.WriteString(string(data))
		}
		b.WriteString("\n")
	}
	return b.String()
}
