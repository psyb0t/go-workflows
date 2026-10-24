package models

import "time"

type Activity struct {
	RowID int64  `gorm:"column:id"`
	ID    string `gorm:"column:activity_id"`

	InstanceID      string
	ExecutionID     string
	EventType       uint
	Timestamp       time.Time
	ScheduleEventID int64
	VisibleAt       *time.Time
	Queue           string
	LockedUntil     *time.Time
	Worker          *string
}

func (Activity) TableName() string { return "activities" }
