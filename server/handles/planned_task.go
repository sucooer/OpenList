package handles

import (
	"strconv"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/planned"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
)

// validatePlannedTask checks name, schedule config and action of a task
// being created or updated.
func validatePlannedTask(c *gin.Context, t *model.PlannedTask) bool {
	if t.Name == "" {
		common.ErrorStrResp(c, "name is required", 400)
		return false
	}
	if t.Action == "" {
		common.ErrorStrResp(c, "action is required", 400)
		return false
	}
	if !planned.HasAction(t.Action) {
		common.ErrorStrResp(c, "unknown action: "+t.Action, 400)
		return false
	}
	if _, err := planned.ValidateSchedule(t, 0); err != nil {
		common.ErrorResp(c, err, 400)
		return false
	}
	return true
}

func ListPlannedTasks(c *gin.Context) {
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	tasks, total, err := db.GetPlannedTasks(req.Page, req.PerPage, c.Query("keyword"))
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, common.PageResp{
		Content: tasks,
		Total:   total,
	})
}

func GetPlannedTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorStrResp(c, "invalid id", 400)
		return
	}
	t, err := db.GetPlannedTaskByID(uint(id))
	if err != nil {
		common.ErrorStrResp(c, "planned task not found", 404)
		return
	}
	common.SuccessResp(c, t)
}

func CreatePlannedTask(c *gin.Context) {
	var req model.PlannedTask
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if !validatePlannedTask(c, &req) {
		return
	}
	if user, ok := c.Request.Context().Value(conf.UserKey).(*model.User); ok {
		req.CreatorID = user.ID
	}
	if err := db.CreatePlannedTask(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	if req.Enabled {
		if err := planned.RegisterTask(&req); err != nil {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	common.SuccessResp(c, req)
}

func UpdatePlannedTask(c *gin.Context) {
	var req model.PlannedTask
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if _, err := db.GetPlannedTaskByID(req.ID); err != nil {
		common.ErrorStrResp(c, "planned task not found", 404)
		return
	}
	if !validatePlannedTask(c, &req) {
		return
	}
	if err := db.UpdatePlannedTask(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	// re-register: RegisterTask handles both enabled and disabled cases
	if err := planned.RegisterTask(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, req)
}

func DeletePlannedTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorStrResp(c, "invalid id", 400)
		return
	}
	if _, err := db.GetPlannedTaskByID(uint(id)); err != nil {
		common.ErrorStrResp(c, "planned task not found", 404)
		return
	}
	planned.UnregisterTask(uint(id))
	if err := db.DeletePlannedTask(uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func SetPlannedTaskEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.Atoi(c.Query("id"))
		if err != nil {
			common.ErrorStrResp(c, "invalid id", 400)
			return
		}
		t, err := db.GetPlannedTaskByID(uint(id))
		if err != nil {
			common.ErrorStrResp(c, "planned task not found", 404)
			return
		}
		t.Enabled = enabled
		if err := db.UpdatePlannedTask(t); err != nil {
			common.ErrorResp(c, err, 500, true)
			return
		}
		if err := planned.RegisterTask(t); err != nil {
			common.ErrorResp(c, err, 500, true)
			return
		}
		common.SuccessResp(c)
	}
}

type RunPlannedTaskReq struct {
	DryRun  bool `json:"dry_run"`
	Confirm bool `json:"confirm"`
}

func RunPlannedTask(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorStrResp(c, "invalid id", 400)
		return
	}
	t, err := db.GetPlannedTaskByID(uint(id))
	if err != nil {
		common.ErrorStrResp(c, "planned task not found", 404)
		return
	}
	var req RunPlannedTaskReq
	// body is optional
	_ = c.ShouldBind(&req)
	// destructive actions require a second confirmation before a real run
	if !req.DryRun && !req.Confirm && planned.IsDangerousTask(t) {
		common.ErrorStrResp(c, "this task contains destructive operations (mirror/delete_source/delete); confirm before running", 400)
		return
	}
	if err := planned.TriggerTask(t.ID, req.DryRun); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	common.SuccessResp(c)
}

func ListPlannedTaskRecords(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorStrResp(c, "invalid id", 400)
		return
	}
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	records, total, err := db.GetPlannedTaskRecords(uint(id), req.Page, req.PerPage)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, common.PageResp{
		Content: records,
		Total:   total,
	})
}

func ListPlannedTaskActions(c *gin.Context) {
	common.SuccessResp(c, planned.ListActions())
}

// ValidatePlannedTaskSchedule validates a schedule config and returns the
// next 5 run times, used by the frontend form for live preview.
func ValidatePlannedTaskSchedule(c *gin.Context) {
	var req model.PlannedTask
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	nexts, err := planned.ValidateSchedule(&req, 5)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	common.SuccessResp(c, nexts)
}

func SetupPlannedTaskRoute(g *gin.RouterGroup) {
	g.GET("/list", ListPlannedTasks)
	g.GET("/get", GetPlannedTask)
	g.POST("/create", CreatePlannedTask)
	g.POST("/update", UpdatePlannedTask)
	g.POST("/delete", DeletePlannedTask)
	g.POST("/enable", SetPlannedTaskEnabled(true))
	g.POST("/disable", SetPlannedTaskEnabled(false))
	g.POST("/run", RunPlannedTask)
	g.GET("/records", ListPlannedTaskRecords)
	g.GET("/actions", ListPlannedTaskActions)
	g.POST("/validate_schedule", ValidatePlannedTaskSchedule)
}
