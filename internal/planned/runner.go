package planned

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/pkg/errors"
)

// ActionFunc executes one run of a planned task.
// params is the decoded JSON object stored in PlannedTask.Params.
// dryRun asks the action to only report what it would do (actions that do
// not support dry-run may ignore it). The returned string is stored as the
// execution log of the run record.
type ActionFunc func(t *model.PlannedTask, params map[string]interface{}, dryRun bool) (string, error)

// ActionInfo describes a registered action for the frontend.
type ActionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var actions = map[string]ActionFunc{}

var actionDescriptions = map[string]string{}

// RegisterAction registers an action type. Panics on duplicate names.
func RegisterAction(name, description string, f ActionFunc) {
	if _, ok := actions[name]; ok {
		panic(fmt.Sprintf("planned action %q already registered", name))
	}
	actions[name] = f
	actionDescriptions[name] = description
}

// ListActions returns all registered actions, sorted by name.
func ListActions() []ActionInfo {
	infos := make([]ActionInfo, 0, len(actions))
	for name := range actions {
		infos = append(infos, ActionInfo{Name: name, Description: actionDescriptions[name]})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// HasAction reports whether the action type exists.
func HasAction(name string) bool {
	_, ok := actions[name]
	return ok
}

// Execute runs the task's action and returns its log output.
func Execute(t *model.PlannedTask, dryRun bool) (string, error) {
	f, ok := actions[t.Action]
	if !ok {
		return "", fmt.Errorf("unknown action %q", t.Action)
	}
	params := map[string]interface{}{}
	if t.Params != "" {
		if err := json.Unmarshal([]byte(t.Params), &params); err != nil {
			return "", errors.Wrap(err, "invalid action params")
		}
	}
	return f(t, params, dryRun)
}
