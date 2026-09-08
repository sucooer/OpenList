package planned

import (
	"context"
	"encoding/json"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/task"
)

// decodeParams converts the decoded params map into a typed struct via a
// JSON round-trip. The runner already unmarshals PlannedTask.Params into a
// map[string]interface{}; actions can call this to get typed access.
func decodeParams(params map[string]interface{}, out interface{}) error {
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// actionContext returns a context carrying the task creator so that fs
// operations (transfer/backup) can attribute their underlying tasks to a
// real user. Scheduled tasks run with the creator's identity, or fall back
// to an admin account when no creator is recorded.
func actionContext(t *model.PlannedTask) context.Context {
	ctx := context.Background()
	var creator *model.User
	if t.CreatorID != 0 {
		creator, _ = db.GetUserById(t.CreatorID)
	}
	if creator == nil {
		creator, _ = db.GetUserByRole(model.ADMIN)
	}
	if creator != nil {
		ctx = context.WithValue(ctx, conf.UserKey, creator)
	}
	return ctx
}

// taskSummary renders a short human-readable line for a submitted transfer
// task. A nil info means the operation completed synchronously (same storage).
func taskSummary(info task.TaskExtensionInfo) string {
	if info == nil {
		return "completed immediately (same storage)"
	}
	return "submitted task " + info.GetID()
}
