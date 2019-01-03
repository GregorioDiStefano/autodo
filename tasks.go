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
	"github.com/GregorioDiStefano/autodo/store"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron"
	log "github.com/sirupsen/logrus"
)

const (
	MAX_WAIT_BEFORE_NOTIFICATIONS = 10
)

func (task *Task) runTaskScript(envVariables map[string]string) (string, error) {
	log.WithField("task", task.Name).Debug("starting")
	lastRun := db.GetTaskHistory(task.Name)

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

	startTime := time.Now().UnixNano()
	err := cmd.Wait()
	endTime := time.Now().UnixNano()

	log.WithField("task", task.Name).Debugf("took %dms to run", (endTime-startTime)/1000000)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			db.AddTaskHistoryEntry(task.Name, uint(lastRun+1), (endTime-startTime)/1000000, out.String(), 9999)
			log.WithField("task", task.Name).Infof("script time out!")
			return "", context.DeadlineExceeded
		}

		log.Debug("task errored with: " + err.Error())
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.WithField("task", task.Name).Infof("script errored with: %s", out.String())
				db.AddTaskHistoryEntry(task.Name, uint(lastRun+1), (endTime-startTime)/1000000, out.String(), status.ExitStatus())
				task.maybeSendNotification(out.String())
				return "", nil
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	}

	for _, notifier := range task.Notifier {
		if notifier.OnSuccess {
			task.maybeSendNotification(out.String())
		}
	}

	log.WithField("task", task.Name).Infof("script ran successfully output: %s", out.String())
	db.AddTaskHistoryEntry(task.Name, uint(lastRun+1), (endTime-startTime)/1000000, out.String(), 0)
	return out.String(), nil
}

func (task *Task) setupCronTask() {
	log.SetLevel(log.DebugLevel)
	log.WithField("task", task.Name).Debug("setting up cronjob")
	c := cron.New()

	j := cron.FuncJob(func() {
		task.runTaskScript(nil)
	})

	c.AddJob(task.Action.Script.Schedule, j)
	c.Start()
}

func (task *Task) setupWebhookRoute(ge *gin.Engine) {
	log.WithFields(log.Fields{"task": task.Name, "task_id": task.Action.Webhook.WebhookID}).Debug("setting up webhook")
	taskID := task.Action.Webhook.WebhookID

	ge.POST("/webhook/"+taskID, func(c *gin.Context) {
		response := make(map[string]string)
		envVariables := make(map[string]string)

		for k, v := range c.Request.URL.Query() {
			envKey := strings.ToUpper(k)
			envVariables[envKey] = v[0]
		}

		payload, _ := ioutil.ReadAll(c.Request.Body)
		envVariables["JSON"] = base64.StdEncoding.EncodeToString(payload)

		startTime := time.Now().UnixNano()
		output, err := task.runTaskScript(envVariables)
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

	})
}

func (t *Task) maybeSendNotification(msg string) {
	if (t.lastMessageSend == time.Time{} || t.lastMessageSend.Unix() < time.Now().Unix()-MAX_WAIT_BEFORE_NOTIFICATIONS) {
		t.lastMessageSend = time.Now()
		for _, notifier := range t.Notifier {
			switch notifier.Type {
			case "pushover":
				msgToSend := strings.Replace(notifier.Text, "%stdout%", msg, 1)
				pushover_notify.SendMessage(msgToSend)
			default:
				log.WithField("notifier", notifier.Type).Warn("unknown notification type")
			}
		}
	}
}
