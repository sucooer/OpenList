package model

import "time"

// PlannedTask schedule types
const (
	PlannedTaskScheduleCron     = "cron"
	PlannedTaskScheduleInterval = "interval"
	// PlannedTaskScheduleWatch is reserved for the fsnotify-based
	// file system watcher trigger (not implemented yet).
	PlannedTaskScheduleWatch = "watch"
)

// PlannedTask run results
const (
	PlannedTaskResultSuccess = "success"
	PlannedTaskResultFailed  = "failed"
	PlannedTaskResultRunning = "running"
	PlannedTaskResultSkipped = "skipped"
)

// PlannedTask is a user-defined scheduled task: an action triggered
// periodically by a cron expression or a fixed interval.
type PlannedTask struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	Name    string `json:"name"`
	Remark  string `json:"remark"`
	Enabled bool   `json:"enabled"`

	// schedule
	ScheduleType string `json:"schedule_type"`          // cron / interval (watch reserved)
	CronExpr     string `json:"cron_expr"`              // cron: 6-field with seconds, e.g. "0 0 3 * * *"
	IntervalSec  int64  `json:"interval_sec"`           // interval: seconds between runs
	Timezone     string `json:"timezone"`               // e.g. "Asia/Shanghai", empty = server local

	// action
	Action string `json:"action"`                       // action type, see planned.RegisterAction
	Params string `json:"params" gorm:"type:text"`    // JSON params consumed by the action

	// run state
	LastRunAt  *time.Time `json:"last_run_at"`
	LastResult string     `json:"last_result"`                    // success / failed / running / skipped
	LastError  string     `json:"last_error" gorm:"type:text"`
	NextRunAt  *time.Time `json:"next_run_at"`
	RunCount   int64      `json:"run_count"`
	FailCount  int64      `json:"fail_count"`

	CreatorID uint      `json:"creator_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlannedTaskRecord is one execution history entry of a PlannedTask.
// Only the latest N records per task are retained (see db.AddPlannedTaskRecord).
type PlannedTaskRecord struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	TaskID     uint       `json:"task_id" gorm:"index"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    *time.Time `json:"end_time"`
	DurationMs int64      `json:"duration_ms"`
	Success    bool       `json:"success"`
	Skipped    bool       `json:"skipped"`
	Log        string     `json:"log" gorm:"type:text"`
}
