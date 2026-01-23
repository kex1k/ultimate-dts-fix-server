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

type AudioInfo struct {
	CodecName     string `json:"codecName"`
	ChannelLayout string `json:"channelLayout"`
	Channels      int    `json:"channels"`
	SampleRate    string `json:"sampleRate"`
	BitRate       string `json:"bitRate"`
}

type Task struct {
	ID          string     `json:"id"`
	FilePath    string     `json:"filePath"`
	OutputPath  string     `json:"outputPath"`
	Status      TaskStatus `json:"status"`
	Progress    int        `json:"progress"`
	Error       string     `json:"error,omitempty"`
	AudioInfo   *AudioInfo `json:"audioInfo,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
