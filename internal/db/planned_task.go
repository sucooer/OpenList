package db

import (
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func plannedTaskDB() *gorm.DB {
	return db.Model(&model.PlannedTask{})
}

// CreatePlannedTask creates a new planned task.
func CreatePlannedTask(t *model.PlannedTask) error {
	return errors.WithStack(db.Create(t).Error)
}

// UpdatePlannedTask updates an existing planned task (all fields).
func UpdatePlannedTask(t *model.PlannedTask) error {
	return errors.WithStack(db.Save(t).Error)
}

// UpdatePlannedTaskRunState only updates the run-state columns of a task,
// used by the scheduler after each execution.
func UpdatePlannedTaskRunState(t *model.PlannedTask) error {
	return errors.WithStack(plannedTaskDB().Where("id = ?", t.ID).Updates(map[string]interface{}{
		"last_run_at": t.LastRunAt,
		"last_result": t.LastResult,
		"last_error":  t.LastError,
		"next_run_at": t.NextRunAt,
		"run_count":   t.RunCount,
		"fail_count":  t.FailCount,
	}).Error)
}

// DeletePlannedTask deletes a planned task and all its records.
func DeletePlannedTask(id uint) error {
	return errors.WithStack(db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&model.PlannedTaskRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.PlannedTask{}, id).Error
	}))
}

// GetPlannedTaskByID returns the task with the given id.
func GetPlannedTaskByID(id uint) (*model.PlannedTask, error) {
	var t model.PlannedTask
	if err := db.First(&t, id).Error; err != nil {
		return nil, errors.Wrapf(err, "failed find planned task %d", id)
	}
	return &t, nil
}

// GetPlannedTasks returns planned tasks page by page, newest first.
func GetPlannedTasks(pageIndex, pageSize int, keyword string) (tasks []model.PlannedTask, count int64, err error) {
	taskDB := plannedTaskDB()
	if keyword != "" {
		taskDB = taskDB.Where("name LIKE ?", "%"+keyword+"%")
	}
	if err = taskDB.Count(&count).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	if err = taskDB.Order(columnName("id") + " desc").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return tasks, count, nil
}

// GetEnabledPlannedTasks returns all enabled tasks, used by the scheduler on boot.
func GetEnabledPlannedTasks() ([]model.PlannedTask, error) {
	var tasks []model.PlannedTask
	if err := db.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return tasks, nil
}

// AddPlannedTaskRecord appends one execution record and prunes old ones,
// keeping at most `retention` records for the task.
func AddPlannedTaskRecord(r *model.PlannedTaskRecord, retention int) error {
	return errors.WithStack(db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		if retention <= 0 {
			return nil
		}
		// delete records beyond the latest `retention` ones
		sub := tx.Model(&model.PlannedTaskRecord{}).
			Select("id").
			Where("task_id = ?", r.TaskID).
			Order(columnName("id") + " desc").
			Limit(retention)
		return tx.Where("task_id = ? AND id NOT IN (?)", r.TaskID, sub).
			Delete(&model.PlannedTaskRecord{}).Error
	}))
}

// GetPlannedTaskRecords returns execution records of a task, newest first.
func GetPlannedTaskRecords(taskID uint, pageIndex, pageSize int) (records []model.PlannedTaskRecord, count int64, err error) {
	recordDB := db.Model(&model.PlannedTaskRecord{}).Where("task_id = ?", taskID)
	if err = recordDB.Count(&count).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	if err = recordDB.Order(columnName("id") + " desc").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return records, count, nil
}

// MarkInterruptedPlannedTasks resets tasks left in "running" state by a
// previous process (e.g. unclean shutdown) so they don't look stuck.
func MarkInterruptedPlannedTasks() error {
	return errors.WithStack(plannedTaskDB().
		Where("last_result = ?", model.PlannedTaskResultRunning).
		Updates(map[string]interface{}{
			"last_result": model.PlannedTaskResultFailed,
			"last_error":  "interrupted by server restart",
		}).Error)
}

// CleanupExpiredPlannedTaskRecords deletes execution records older than the
// given number of days (0 disables cleanup). It returns the number of rows
// removed.
func CleanupExpiredPlannedTaskRecords(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	res := db.Where("start_time < ?", cutoff).Delete(&model.PlannedTaskRecord{})
	if res.Error != nil {
		return 0, errors.WithStack(res.Error)
	}
	return res.RowsAffected, nil
}
