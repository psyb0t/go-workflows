package models

type Attribute struct {
	ID          string `gorm:"column:event_id"`
	InstanceID  string
	ExecutionID string
	Data        []byte
}

func (Attribute) TableName() string { return "attributes" }
