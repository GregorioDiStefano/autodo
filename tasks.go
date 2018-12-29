package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GregorioDiStefano/autodo/notifiers"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron"
	log "github.com/sirupsen/logrus"
)

const (
	MAX_WAIT_BEFORE_NOTIFICATIONS = 10
)

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
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"notifier"`

	//internally used
	lastMessageSend time.Time
}

func runScript(task *Task, envVariables map[string]string) (string, error) {
	log.WithField("task", task.Name).Debug("starting")

	timeout := 0
	if task.Action.Script.Timeout > 0 {
		timeout = task.Action.Script.Timeout
		log.WithField("task", task.Name).Debugf("script time out set to %d seconds", timeout)
	} else {
		timeout = 60 * 60
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "./tasks/"+task.Action.Script.File)

	var out bytes.Buffer
	cmd.Stdout = &out

	if envVariables != nil {
		cmd.Env = os.Environ()
		for key, val := range envVariables {
			log.WithField(key, val).Debugf("setting env. variable for %s", task.Name)
			cmd.Env = append(cmd.Env, key+"="+val)
		}

	}

	if err := cmd.Start(); err != nil {
		log.WithField("task", task.Name).Fatalf("task failed to start: %s", err.Error())
	}

	if err := cmd.Wait(); err != nil {

		if ctx.Err() == context.DeadlineExceeded {
			log.WithField("task", task.Name).Debugf("script timedout!")
			return "", context.DeadlineExceeded
		}

		log.Debug("task errored with: " + err.Error())
		if exiterr, ok := err.(*exec.ExitError); ok {
			task.maybeSendNotification(out.String())

			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	}

	return out.String(), nil
}

func setupCronTask(task *Task) {
	log.SetLevel(log.DebugLevel)
	log.WithField("script", task.Action.Script.File).Debug("setting up cronjob")
	c := cron.New()

	j := cron.FuncJob(func() {
		runScript(task, nil)
	})

	c.AddJob(task.Action.Script.Schedule, j)
	c.Start()
}

func setupWebhookRoute(tasks *[]Task, ge *gin.Engine) {
	ge.POST("/webhook/:id", func(c *gin.Context) {
		id := c.Param("id")

		for _, task := range *tasks {
			if id == task.Action.Webhook.WebhookID {
				response := make(map[string]string)
				envVariables := make(map[string]string)

				for k, v := range c.Request.URL.Query() {
					envKey := strings.ToUpper(k)
					envVariables[envKey] = v[0]
				}

				payload, _ := ioutil.ReadAll(c.Request.Body)
				envVariables["JSON"] = base64.StdEncoding.EncodeToString(payload)

				startTime := time.Now().UnixNano()
				output, err := runScript(&task, envVariables)
				endTime := time.Now().UnixNano()

				response["runtime"] = strconv.FormatInt((endTime-startTime)/1000000, 10) + "ms"

				if task.Action.Webhook.ShowScriptStdout {
					response["stdout"] = output
				}

				if err != nil {
					response["error"] = err.Error()
				}

				c.JSON(http.StatusOK, response)
				return
			}
		}
	})
}

func (t *Task) maybeSendNotification(msg string) {
	if (t.lastMessageSend == time.Time{} || t.lastMessageSend.Unix() < time.Now().Unix()-MAX_WAIT_BEFORE_NOTIFICATIONS) {
		t.lastMessageSend = time.Now()
		pushover_notify.SendMessage(msg)
	}
}
