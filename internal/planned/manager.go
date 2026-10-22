// Package planned implements scheduled (planned) tasks: user-defined
// actions triggered periodically by a cron expression or a fixed interval.
package planned

import (
	"fmt"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/setting"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

// specParser accepts 5-field and 6-field (with seconds) cron expressions,
// @descriptors (e.g. @every 1h) and the CRON_TZ= prefix.
var specParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour |
	cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

type Manager struct {
	cron    *cron.Cron
	entries map[uint]cron.EntryID
	running sync.Map // taskID(uint) -> bool, anti-reentrancy guard
	mu      sync.RWMutex
}

var manager *Manager

// Init starts the scheduler: loads enabled tasks from DB and registers them.
// Called once from bootstrap after InitTaskManager.
func Init() error {
	manager = &Manager{
		cron:    cron.New(cron.WithParser(specParser)),
		entries: make(map[uint]cron.EntryID),
	}
	// reset tasks left in "running" state by an unclean shutdown
	if err := db.MarkInterruptedPlannedTasks(); err != nil {
		log.Errorf("failed to reset interrupted planned tasks: %+v", err)
	}
	tasks, err := db.GetEnabledPlannedTasks()
	if err != nil {
		return errors.WithStack(err)
	}
	for i := range tasks {
		if err := manager.register(&tasks[i]); err != nil {
			log.Errorf("failed to register planned task %d(%s): %+v", tasks[i].ID, tasks[i].Name, err)
		}
	}
	// daily cleanup of expired execution records
	cleanupExpiredRecords()
	_, _ = manager.cron.AddFunc("@daily", cleanupExpiredRecords)
	manager.cron.Start()
	log.Infof("planned task scheduler started, %d task(s) registered", len(manager.entries))
	return nil
}

// buildSpec converts the task's schedule config into a cron spec string.
func buildSpec(t *model.PlannedTask) (string, error) {
	switch t.ScheduleType {
	case model.PlannedTaskScheduleCron:
		if t.CronExpr == "" {
			return "", errors.New("cron expression is empty")
		}
		spec := t.CronExpr
		if t.Timezone != "" {
			if _, err := time.LoadLocation(t.Timezone); err != nil {
				return "", errors.Wrapf(err, "invalid timezone %q", t.Timezone)
			}
			spec = "CRON_TZ=" + t.Timezone + " " + spec
		}
		return spec, nil
	case model.PlannedTaskScheduleInterval:
		if t.IntervalSec <= 0 {
			return "", errors.New("interval must be positive seconds")
		}
		return fmt.Sprintf("@every %ds", t.IntervalSec), nil
	case model.PlannedTaskScheduleWatch:
		return "", errors.New("watch schedule is not implemented yet")
	default:
		return "", fmt.Errorf("unknown schedule type %q", t.ScheduleType)
	}
}

// ValidateSchedule checks whether the schedule config is schedulable and,
// if so, returns the next `count` run times from now.
func ValidateSchedule(t *model.PlannedTask, count int) ([]time.Time, error) {
	spec, err := buildSpec(t)
	if err != nil {
		return nil, err
	}
	sched, err := specParser.Parse(spec)
	if err != nil {
		return nil, errors.Wrap(err, "invalid cron expression")
	}
	if count <= 0 {
		return nil, nil
	}
	nexts := make([]time.Time, 0, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		now = sched.Next(now)
		nexts = append(nexts, now)
	}
	return nexts, nil
}

func (m *Manager) register(t *model.PlannedTask) error {
	spec, err := buildSpec(t)
	if err != nil {
		return err
	}
	taskID := t.ID
	entryID, err := m.cron.AddFunc(spec, func() {
		runTask(taskID, false, false)
	})
	if err != nil {
		return errors.Wrap(err, "failed to add cron entry")
	}
	m.mu.Lock()
	// replace existing entry if re-registering
	if old, ok := m.entries[taskID]; ok {
		m.cron.Remove(old)
	}
	m.entries[taskID] = entryID
	m.mu.Unlock()
	// persist the computed next run time
	if next := m.nextRun(taskID); next != nil {
		dbTask, err := db.GetPlannedTaskByID(taskID)
		if err == nil {
			dbTask.NextRunAt = next
			_ = db.UpdatePlannedTaskRunState(dbTask)
		}
	}
	return nil
}

// RegisterTask (re)schedules an enabled task; call after create/update/enable.
func RegisterTask(t *model.PlannedTask) error {
	if manager == nil {
		return errors.New("planned task scheduler not initialized")
	}
	if !t.Enabled {
		UnregisterTask(t.ID)
		return nil
	}
	return manager.register(t)
}

// UnregisterTask removes a task from the scheduler; call after delete/disable.
func UnregisterTask(id uint) {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if entryID, ok := manager.entries[id]; ok {
		manager.cron.Remove(entryID)
		delete(manager.entries, id)
	}
}

func (m *Manager) nextRun(id uint) *time.Time {
	m.mu.RLock()
	entryID, ok := m.entries[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	entry := m.cron.Entry(entryID)
	if entry.Next.IsZero() {
		return nil
	}
	next := entry.Next
	return &next
}

// TriggerTask executes the task once immediately (async), regardless of its
// schedule. Returns an error if the task appears to be already running.
func TriggerTask(id uint, dryRun bool) error {
	if manager == nil {
		return errors.New("planned task scheduler not initialized")
	}
	if _, ok := manager.running.Load(id); ok {
		return errors.New("task is already running")
	}
	go runTask(id, true, dryRun)
	return nil
}

// runTask loads the task and executes its action with anti-reentrancy guard.
func runTask(id uint, manual bool, dryRun bool) {
	if manager == nil {
		return
	}
	if _, loaded := manager.running.LoadOrStore(id, true); loaded {
		log.Warnf("planned task %d skipped: previous run still in progress", id)
		record := &model.PlannedTaskRecord{
			TaskID:    id,
			StartTime: time.Now(),
			Skipped:   true,
			Log:       "skipped: previous run still in progress",
		}
		if err := db.AddPlannedTaskRecord(record, recordRetention()); err != nil {
			log.Errorf("failed to save skipped record for planned task %d: %+v", id, err)
		}
		return
	}
	defer manager.running.Delete(id)

	t, err := db.GetPlannedTaskByID(id)
	if err != nil {
		log.Errorf("planned task %d not found: %+v", id, err)
		return
	}

	start := time.Now()
	record := &model.PlannedTaskRecord{TaskID: id, StartTime: start}
	t.LastRunAt = &start
	t.LastResult = model.PlannedTaskResultRunning
	t.NextRunAt = manager.nextRun(id)
	t.RunCount++
	_ = db.UpdatePlannedTaskRunState(t)

	logLine, runErr := Execute(t, dryRun)

	end := time.Now()
	record.EndTime = &end
	record.DurationMs = end.Sub(start).Milliseconds()
	if runErr != nil {
		record.Success = false
		record.Log = logLine + "\nerror: " + runErr.Error()
		t.LastResult = model.PlannedTaskResultFailed
		t.LastError = runErr.Error()
		t.FailCount++
	} else {
		record.Success = true
		record.Log = logLine
		t.LastResult = model.PlannedTaskResultSuccess
		t.LastError = ""
	}
	t.NextRunAt = manager.nextRun(id)
	if err := db.UpdatePlannedTaskRunState(t); err != nil {
		log.Errorf("failed to update run state of planned task %d: %+v", id, err)
	}
	if err := db.AddPlannedTaskRecord(record, recordRetention()); err != nil {
		log.Errorf("failed to save record of planned task %d: %+v", id, err)
	}
	if manual {
		log.Infof("planned task %d(%s) manually triggered, success=%v", id, t.Name, record.Success)
	}
}

func recordRetention() int {
	return setting.GetInt(conf.PlannedTaskRecordRetention, 50)
}

// cleanupExpiredRecords deletes execution records older than the configured
// retention window (planned_task_record_retention_days, default 30 days).
// A value of 0 disables time-based cleanup.
func cleanupExpiredRecords() {
	days := setting.GetInt(conf.PlannedTaskRecordRetentionDays, 30)
	n, err := db.CleanupExpiredPlannedTaskRecords(days)
	if err != nil {
		log.Errorf("failed to cleanup expired planned task records: %+v", err)
		return
	}
	if n > 0 {
		log.Infof("cleaned up %d expired planned task record(s)", n)
	}
}
