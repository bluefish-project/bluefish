package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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

// ActionPhase tracks the current action overlay state
type ActionPhase int

const (
	PhaseSelect  ActionPhase = iota // Pick an action
	PhaseParams                     // Edit parameters
	PhaseConfirm                    // Confirm POST
	PhaseResult                     // Show result
)

// ActionParam is a single parameter being edited
type ActionParam struct {
	Name            string
	Value           string
	AllowableValues []string
	AllowableIndex  int // Current index in AllowableValues cycle
}

// ActionModel manages the action overlay
type ActionModel struct {
	phase   ActionPhase
	actions []ActionInfo
	cursor  int

	// Params phase
	selected *ActionInfo
	params   []ActionParam
	paramIdx int // Which param is being edited
	input    textinput.Model

	// Result phase
	resultStatus int
	resultBody   string
	resultErr    error

	width  int
	height int
}

func NewActionModel() ActionModel {
	ti := textinput.New()
	ti.CharLimit = 256
	return ActionModel{
		input: ti,
	}
}

// Open activates action mode after discovery
func (a *ActionModel) Open(actions []ActionInfo) {
	a.actions = actions
	a.cursor = 0
	a.phase = PhaseSelect
	a.selected = nil
	a.params = nil
	a.resultErr = nil
}

// Close resets the action model
func (a *ActionModel) Close() {
	a.input.Blur()
}

func (a *ActionModel) Update(msg tea.Msg) tea.Cmd {
	if a.phase == PhaseParams {
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return cmd
	}
	return nil
}

// SelectAction moves to params phase for the selected action
func (a *ActionModel) SelectAction() {
	if a.cursor < 0 || a.cursor >= len(a.actions) {
		return
	}
	action := a.actions[a.cursor]
	a.selected = &action
	a.phase = PhaseParams
	a.paramIdx = 0

	// Build param list from AllowableValues
	a.params = nil
	paramNames := make([]string, 0, len(action.Allowable))
	for name := range action.Allowable {
		paramNames = append(paramNames, name)
	}
	sort.Strings(paramNames)

	for _, name := range paramNames {
		vals := action.Allowable[name]
		p := ActionParam{
			Name:            name,
			AllowableValues: vals,
		}
		if len(vals) > 0 {
			p.Value = vals[0]
		}
		a.params = append(a.params, p)
	}

	if len(a.params) > 0 {
		a.input.SetValue(a.params[0].Value)
		a.input.Focus()
	}
}

// CycleAllowable cycles through allowable values for current param
func (a *ActionModel) CycleAllowable() {
	if a.phase != PhaseParams || a.paramIdx >= len(a.params) {
		return
	}
	p := &a.params[a.paramIdx]
	if len(p.AllowableValues) == 0 {
		return
	}
	p.AllowableIndex = (p.AllowableIndex + 1) % len(p.AllowableValues)
	p.Value = p.AllowableValues[p.AllowableIndex]
	a.input.SetValue(p.Value)
}

// ConfirmParams saves current param and moves to confirm phase
func (a *ActionModel) ConfirmParams() {
	if a.phase != PhaseParams {
		return
	}
	// Save current input to current param
	if a.paramIdx < len(a.params) {
		a.params[a.paramIdx].Value = a.input.Value()
	}
	a.input.Blur()
	a.phase = PhaseConfirm
}

// NextParam moves to next parameter
func (a *ActionModel) NextParam() {
	if a.paramIdx < len(a.params) {
		a.params[a.paramIdx].Value = a.input.Value()
	}
	a.paramIdx++
	if a.paramIdx >= len(a.params) {
		a.paramIdx = 0
	}
	if len(a.params) > 0 {
		a.input.SetValue(a.params[a.paramIdx].Value)
	}
}

// BuildBody builds the JSON body for the POST
func (a *ActionModel) BuildBody() ([]byte, error) {
	body := make(map[string]any)
	for _, p := range a.params {
		if p.Value == "" {
			continue
		}
		// Type conversion
		if n, err := strconv.ParseFloat(p.Value, 64); err == nil {
			if n == float64(int64(n)) {
				body[p.Name] = int64(n)
			} else {
				body[p.Name] = n
			}
		} else if p.Value == "true" {
			body[p.Name] = true
		} else if p.Value == "false" {
			body[p.Name] = false
		} else {
			body[p.Name] = p.Value
		}
	}
	return json.MarshalIndent(body, "", "  ")
}

// SetResult sets the result of a POST action
func (a *ActionModel) SetResult(status int, body string, err error) {
	a.phase = PhaseResult
	a.resultStatus = status
	a.resultBody = body
	a.resultErr = err
}

// BackPhase goes back one phase or returns false if should close
func (a *ActionModel) BackPhase() bool {
	switch a.phase {
	case PhaseSelect:
		return false // Close overlay
	case PhaseParams:
		a.phase = PhaseSelect
		a.input.Blur()
		return true
	case PhaseConfirm:
		a.phase = PhaseParams
		if len(a.params) > 0 {
			a.input.SetValue(a.params[a.paramIdx].Value)
			a.input.Focus()
		}
		return true
	case PhaseResult:
		a.phase = PhaseSelect
		return true
	}
	return false
}

func (a *ActionModel) MoveUp() {
	if a.phase == PhaseSelect && a.cursor > 0 {
		a.cursor--
	}
}

func (a *ActionModel) MoveDown() {
	if a.phase == PhaseSelect && a.cursor < len(a.actions)-1 {
		a.cursor++
	}
}

func (a *ActionModel) View() string {
	var b strings.Builder

	switch a.phase {
	case PhaseSelect:
		a.viewSelect(&b)
	case PhaseParams:
		a.viewParams(&b)
	case PhaseConfirm:
		a.viewConfirm(&b)
	case PhaseResult:
		a.viewResult(&b)
	}

	return b.String()
}

func (a *ActionModel) viewSelect(b *strings.Builder) {
	b.WriteString(actionTitleStyle.Render("Actions"))
	b.WriteString("\n\n")

	for i, action := range a.actions {
		line := "  " + actionNameStyle.Render(action.ShortName)
		if len(action.Allowable) > 0 {
			var params []string
			for param, vals := range action.Allowable {
				params = append(params, fmt.Sprintf("%s=[%s]", param, strings.Join(vals, "|")))
			}
			sort.Strings(params)
			line += "  " + helpDescStyle.Render(strings.Join(params, " "))
		}

		if i == a.cursor {
			b.WriteString(cursorStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpDescStyle.Render("  j/k:nav  enter:select  esc:close"))
}

func (a *ActionModel) viewParams(b *strings.Builder) {
	b.WriteString(actionTitleStyle.Render(a.selected.ShortName))
	b.WriteString("  ")
	b.WriteString(actionTargetStyle.Render(a.selected.Target))
	b.WriteString("\n\n")

	if len(a.params) == 0 {
		b.WriteString(helpDescStyle.Render("  No parameters"))
		b.WriteString("\n\n")
		b.WriteString(helpDescStyle.Render("  enter:confirm  esc:back"))
		return
	}

	for i, p := range a.params {
		prefix := "  "
		if i == a.paramIdx {
			prefix = cursorStyle.Render("> ")
		}
		b.WriteString(prefix)
		b.WriteString(actionNameStyle.Render(p.Name))
		b.WriteString(" = ")
		if i == a.paramIdx {
			b.WriteString(a.input.View())
		} else {
			b.WriteString(detailValueStyle.Render(p.Value))
		}
		if len(p.AllowableValues) > 0 {
			b.WriteString("  ")
			b.WriteString(helpDescStyle.Render("[" + strings.Join(p.AllowableValues, "|") + "]"))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpDescStyle.Render("  tab:cycle  enter:confirm  esc:back"))
}

func (a *ActionModel) viewConfirm(b *strings.Builder) {
	b.WriteString(actionConfirmStyle.Render("POST "))
	b.WriteString(actionTargetStyle.Render(a.selected.Target))
	b.WriteString("\n\n")

	body, err := a.BuildBody()
	if err != nil {
		b.WriteString(actionErrorStyle.Render(fmt.Sprintf("Error: %v", err)))
	} else {
		b.WriteString(detailValueStyle.Render(string(body)))
	}
	b.WriteString("\n\n")
	b.WriteString(actionConfirmStyle.Render("Execute? "))
	b.WriteString(helpDescStyle.Render("y:yes  n/esc:cancel"))
}

func (a *ActionModel) viewResult(b *strings.Builder) {
	if a.resultErr != nil {
		b.WriteString(actionErrorStyle.Render(fmt.Sprintf("Error: %v", a.resultErr)))
		b.WriteString("\n")
	} else {
		statusStr := fmt.Sprintf("HTTP %d", a.resultStatus)
		if a.resultStatus >= 200 && a.resultStatus < 300 {
			b.WriteString(actionSuccessStyle.Render(statusStr))
		} else {
			b.WriteString(actionErrorStyle.Render(statusStr))
		}
		b.WriteString("\n\n")
		if a.resultBody != "" {
			b.WriteString(detailValueStyle.Render(a.resultBody))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpDescStyle.Render("  esc:close"))
}

// discoverActions finds all actions on a resource
func discoverActions(resource *rvfs.Resource) []ActionInfo {
	if resource == nil {
		return nil
	}

	actionsProp, ok := resource.Properties["Actions"]
	if !ok || actionsProp.Type != rvfs.PropertyObject {
		return nil
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
	return actions
}
