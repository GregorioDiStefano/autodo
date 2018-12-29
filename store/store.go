package db

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type TaskHistory struct {
	gorm.Model
	Run           uint
	TaskName      string
	ExecutionTime int
}

var db *gorm.DB

func Setup() {
	var err error
	db, err = gorm.Open("sqlite3", "tasks.db")

	if err != nil {
		panic("failed to connect database")
	}
	// Migrate the schema
	db.AutoMigrate(&TaskHistory{})
}

func Add_entry() {
	db.Create(&TaskHistory{
		Run:           1212,
		TaskName:      "1000",
		ExecutionTime: 12})
}
