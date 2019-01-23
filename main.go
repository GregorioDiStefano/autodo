package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"time"
	"path/filepath"
	db "github.com/GregorioDiStefano/autodo/store"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	LISTEN_ADDRESS = "0.0.0.0:8000"
	TASK_DIR = "tasks"
)

func main() {
	log.SetLevel(log.DebugLevel)
	db.Setup()

	files, err := ioutil.ReadDir(TASK_DIR)

	if err != nil {
		log.Fatal(err)
	}

	var tasks []Task
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			var t Task
			data, err := ioutil.ReadFile(filepath.Join(TASK_DIR,f.Name()))
			log.WithField("script", f.Name()).Debugf("loading script: %s", t.Name)


			if err != nil {
				panic(err)
			}

			err = json.Unmarshal(data, &t)

			if err != nil {
				panic(err)
			}


			tasks = append(tasks, t)
		}

	}

	verifyTasks(tasks)
	webhookTasks := []Task{}

	for idx, task := range tasks {
		setTaskType(&tasks[idx])
		if task.Schedule.Cron != "" {
			t := &tasks[idx]
			go t.setupCronTask()
		} else {
			webhookTasks = append(webhookTasks, tasks[idx])
		}
	}

	ge := gin.Default()
	for idx, _ := range webhookTasks {
		t := &webhookTasks[idx]
		t.setupWebhookRoute(ge)
	}

	time.Sleep(1 * time.Second)
	log.WithField("webhook url", "http://"+LISTEN_ADDRESS+"/webhook/<id>").Info("listening for webhooks")

	gin.SetMode(gin.ReleaseMode)
	ge.Run(LISTEN_ADDRESS)
}

func verifyTasks(tasks []Task) {
	taskNames := []string{}

	// verify tasknames are unique
	for _, task := range tasks {
		for _, taskNameSeen := range taskNames {
			if taskNameSeen == task.Name {
				log.Fatal("script with this name already exists")
			}
		}

		taskNames = append(taskNames, task.Name)
		if _, err := os.Stat(filepath.Join(TASK_DIR, task.Action.Script.File)); err != nil {
			log.Fatalf("unable to stat script %s in %s", task.Action.Script.File, task.Name)
		}

		if task.Action.Container.Image != "" && task.Action.Script.File != "" {
			log.WithField("task", task.Name).Debug("task can only have a container or script configured")
		}

	}
}

func setTaskType(task *Task) {
	if task.Action.Container.Image != "" {
		task.taskActionType = ContainerTask
	} else if task.Action.Script.File != "" {
		task.taskActionType = ScriptTask
	}
}
