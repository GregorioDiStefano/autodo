package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"

	pushover_notify "github.com/GregorioDiStefano/autodo/notifiers"
	db "github.com/GregorioDiStefano/autodo/store"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron"
	log "github.com/sirupsen/logrus"
)

const (
	MAX_WAIT_BEFORE_NOTIFICATIONS = 10
)

func (task *Task) runTaskContainer(envVariables map[string]string) (string, int, error) {
	log.WithFields(log.Fields{"task": task.Name, "env. variables": envVariables}).Debug("starting container")
	cli, err := client.NewClientWithOpts(client.WithVersion("1.39"))

	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(2*time.Second))
	defer cancel()

	var envString []string
	for key, value := range envVariables {
		envString = append(envString, fmt.Sprintf("%s=%s", key, value))
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: task.Action.Container.Image,
		Tty:   true,
		Env:   envString,
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: task.Action.Container.Volume,
				Target: "/data",
			},
		},
	}, nil, "")

	if err != nil {
		panic(err)
	}

	cro := types.ContainerRemoveOptions{Force: true}
	defer cli.ContainerRemove(context.Background(), resp.ID, cro)

	cso := types.ContainerStartOptions{}
	if err := cli.ContainerStart(ctx, resp.ID, cso); err != nil {
		panic(err)
	}

	waitBody, errChan := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	containerTimeout := time.Duration(10 * time.Second)

	select {
	case <-waitBody:
		logOptions := types.ContainerLogsOptions{ShowStdout: true}
		output, _ := cli.ContainerLogs(context.Background(), resp.ID, logOptions)

		inspect, err := cli.ContainerInspect(context.Background(), resp.ID)
		if err != nil {
			return "", inspect.State.ExitCode, err
		}

		cli.ContainerStop(context.Background(), resp.ID, &containerTimeout)
		logOutput, _ := ioutil.ReadAll(output)
		return string(logOutput), inspect.State.ExitCode, nil
	case <-errChan:
		err := cli.ContainerStop(context.Background(), resp.ID, &containerTimeout)
		return "failed", 0, err
	}

}

func (task *Task) runTaskScript(envVariables map[string]string) (string, int, error) {
	log.WithField("task", task.Name).Debug("starting script")
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

	cmd := exec.CommandContext(ctx, "./"+filepath.Base(task.Action.Script.File))

	if cwd, err := os.Getwd(); err == nil {
		cmd.Dir = filepath.Join(cwd, TASK_DIR, filepath.Dir(task.Action.Script.File))
	} else {
		log.Fatal("failed to get current directory!")
	}

	var out bytes.Buffer
	cmd.Stdout = &out

	cmd.Env = os.Environ()
	if envVariables != nil {
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
			return "", -1, context.DeadlineExceeded
		}

		log.Debug("task errored with: " + err.Error())
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.WithField("task", task.Name).Infof("script errored with: %s", out.String())
				db.AddTaskHistoryEntry(task.Name, uint(lastRun+1), (endTime-startTime)/1000000, out.String(), status.ExitStatus())
				return "", status.ExitStatus(), nil
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	}


	log.WithField("task", task.Name).Infof("script ran successfully output: %s", out.String())
	db.AddTaskHistoryEntry(task.Name, uint(lastRun+1), (endTime-startTime)/1000000, out.String(), 0)
	return out.String(), -1, nil
}

func (task *Task) setupCronTask() {
	log.SetLevel(log.DebugLevel)
	log.WithField("task", task.Name).Debug("setting up cronjob")

	c := cron.New()
	j := cron.FuncJob(func() {
		switch task.taskActionType {
		case ContainerTask:
		  task.handleTask(task.runTaskContainer(nil))
		case ScriptTask:
			task.handleTask(task.runTaskScript(nil))
		}
	})

	c.AddJob(task.Schedule.Cron, j)
	c.Start()
}

func (task *Task) handleTask(output string, exitcode int, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	cleanOutput := strings.Replace(strings.TrimSpace(output), "\n", `\n`, -1)
	log.WithFields(log.Fields{"task": task.Name, "exit-code": exitcode, "error": errStr}).Debug("output: " + cleanOutput)
	task.maybeSendNotification(output)

	for _, notifier := range task.Notifier {
		if notifier.OnSuccess {
			task.maybeSendNotification(output)
		}
	}
}

func (task *Task) setupWebhookRoute(ge *gin.Engine) {
	log.WithFields(log.Fields{"task": task.Name, "task_id": task.Webhook.WebhookID}).Debug("setting up webhook")
	taskID := task.Webhook.WebhookID

	group := &gin.RouterGroup{}
	basicAuth := task.Webhook.BasicAuthUsernamePassword

	if basicAuth != "" {
		userPass := strings.Split(basicAuth, ":")
		if len(userPass) == 2 {
			user := userPass[0]
			pass := userPass[1]

			group = ge.Group("/webhook/"+taskID, gin.BasicAuth(gin.Accounts{user: pass}))
		} else {
			log.Warn("invalid username:password defined for webhook: " + basicAuth)
		}
	} else {
		group = ge.Group("/webhook/" + taskID)
	}

	group.POST("", func(c *gin.Context) {
		response := make(map[string]string)
		envVariables := make(map[string]string)

		for k, v := range c.Request.URL.Query() {
			envKey := strings.ToUpper(k)
			envVariables[envKey] = v[0]
		}

		payload, _ := ioutil.ReadAll(c.Request.Body)
		envVariables["JSON"] = base64.StdEncoding.EncodeToString(payload)

		startTime := time.Now().UnixNano()

		var output string
		var err error

		if task.taskActionType == ScriptTask {
			output, _, err = task.runTaskScript(envVariables)
		} else if task.taskActionType == ContainerTask {
			output, _, err = task.runTaskContainer(envVariables)
		}

		endTime := time.Now().UnixNano()

		response["runtime"] = strconv.FormatInt((endTime-startTime)/1000000, 10) + "ms"

		if task.Webhook.ShowScriptStdout {
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
	} else {
		log.WithField("task", t.Name).Info("notification recently sent, skipping.")
	}
}
