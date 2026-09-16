package infrastructure

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type deploymentSchedulePolicyModel struct {
	EnvironmentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Enabled       bool
	TimeZone      string
	WeeklyWindows datatypes.JSON `gorm:"type:jsonb"`
	Blackouts     datatypes.JSON `gorm:"type:jsonb"`
	Version       uint64
	CreatedBy     uuid.UUID
	UpdatedBy     uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (deploymentSchedulePolicyModel) TableName() string { return "deployment_schedule_policies" }

type deploymentScheduleCommandModel struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ActorID            uuid.UUID
	EnvironmentID      uuid.UUID
	IdempotencyKey     string
	RequestFingerprint string
	ResponsePolicy     datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt          time.Time
}

func (deploymentScheduleCommandModel) TableName() string { return "deployment_schedule_commands" }

type deploymentScheduleWindowDocument struct {
	DayOfWeek   int `json:"dayOfWeek"`
	StartMinute int `json:"startMinute"`
	EndMinute   int `json:"endMinute"`
}

type deploymentScheduleBlackoutDocument struct {
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

type deploymentSchedulePolicyDocument struct {
	EnvironmentID uuid.UUID                            `json:"environmentId"`
	Enabled       bool                                 `json:"enabled"`
	TimeZone      string                               `json:"timeZone"`
	WeeklyWindows []deploymentScheduleWindowDocument   `json:"weeklyWindows"`
	Blackouts     []deploymentScheduleBlackoutDocument `json:"blackouts"`
	Version       uint64                               `json:"version"`
}
