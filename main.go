package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/GregorioDiStefano/autodo/store"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)
	db.Setup()

	files, err := ioutil.ReadDir("./tasks")

	if err != nil {
		log.Fatal(err)
	}

	var tasks []Task
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			var t Task
			data, err := ioutil.ReadFile("./tasks/" + f.Name())

			if err != nil {
				panic(err)
			}

			err = json.Unmarshal(data, &t)

			if err != nil {
				panic(err)
			}

			log.WithField("script", f.Name()).Debugf("loading script: %s", t.Name)
			tasks = append(tasks, t)
		}

	}

	verifyTasks(tasks)

	gin.SetMode(gin.ReleaseMode)
	ge := gin.Default()

	for idx, _ := range tasks {
		if tasks[idx].Action.Script.Schedule != "" {
			go setupCronTask(&tasks[idx])
		}
	}

	setupWebhookRoute(&tasks, ge)

	ge.Run(":8000")
	for {
		time.Sleep(1 * time.Second)
	}
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

		if _, err := os.Stat("./tasks/" + task.Action.Script.File); err != nil {
			log.Fatalf("unable to stat script %s in %s", task.Action.Script.File, task.Name)
		}

	}

}
