package db

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type TaskHistory struct {
	gorm.Model
	Run           uint
	TaskName      string
	ExecutionTime int64
	ExitCode      int
	Output        string
}

var db *gorm.DB

func Setup() {
	var err error
	db, err = gorm.Open("sqlite3", "tasks.db")

	if err != nil {
		panic("failed to connect database: " + err.Error())
	}

	// Migrate the schema
	db.AutoMigrate(&TaskHistory{})
}

func AddTaskHistoryEntry(taskName string, run uint, executionTime int64, output string, exitCode int) {
	db.Create(&TaskHistory{
		Run:           run,
		TaskName:      taskName,
		ExecutionTime: executionTime,
		Output:        output,
		ExitCode:      exitCode})
}

func GetTaskHistory(taskName string) int {
	var taskHistory []TaskHistory
	db.Where("task_name = ?", taskName).Find(&taskHistory)

	largestRunNumber := uint(0)
	for _, run := range taskHistory {
		if run.Run > largestRunNumber {
			largestRunNumber = run.Run
		}

	}

	return int(largestRunNumber)
}
