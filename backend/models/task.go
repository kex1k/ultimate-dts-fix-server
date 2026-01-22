package models

import (
	"time"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusProcessing TaskStatus = "processing"
	StatusCompleted  TaskStatus = "completed"
	StatusError      TaskStatus = "error"
)

type Task struct {
	ID          string     `json:"id"`
	FilePath    string     `json:"filePath"`
	OutputPath  string     `json:"outputPath"`
	Status      TaskStatus `json:"status"`
	Progress    int        `json:"progress"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}