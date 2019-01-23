package main

import (
	"time"
)

const (
	ContainerTask = iota
	ScriptTask
)

type Task struct {
	Name   string `json:"name"`
	Action struct {
		Script struct {
			File     string `json:"file"`
			Timeout  int    `json:"timeout"`
		} `json:"script"`

		Container struct {
			Image    string `json:"image"`
			Volume   string `json:"volume"`
			Timeout  int    `json:"timeout"`
		} `json:"container"`


	} `json:"action"`



		Webhook struct {
			BasicAuthUsernamePassword string `json:"basic_auth_creds"`
			WebhookID                 string `json:"webhook_id"`
			ShowScriptStdout          bool   `json:"show_stdout"`
		} `json:"webhook"`

		Schedule struct {
			Cron string `json:"cron"`
		} `json:"schedule"`


	Notifier []struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		OnSuccess bool   `json:"on_success"`
	} `json:"notifier"`

	//internally used
	lastMessageSend time.Time
	exitCode        int
	taskActionType  int
}
