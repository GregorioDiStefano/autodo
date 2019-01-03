package main

import "time"

type Task struct {
	Name   string `json:"name"`
	Action struct {
		Script struct {
			File     string `json:"file"`
			Schedule string `json:"schedule"`
			Timeout  int    `json:"timeout"`
		} `json:"script"`
		Webhook struct {
			WebhookID        string `json:"webhook_id"`
			ShowScriptStdout bool   `json:"show_stdout"`
		} `json:"webhook"`
	} `json:"action"`
	Notifier []struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		OnSuccess bool   `json:"on-success"`
	} `json:"notifier"`

	//internally used
	lastMessageSend time.Time
	exitCode        int
}
