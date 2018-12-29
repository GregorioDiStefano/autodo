package pushover_notify

import (
	"log"

	"github.com/gregdel/pushover"
)

func SendMessage(msg string) {
	app := pushover.New("auwgg66vo7s3iqnahoqrtopjc76f4g")
	recipient := pushover.NewRecipient("uidk5AmyKjyF2o8xU4FU991aNM94hh")

	message := pushover.NewMessage(msg)

	resp, err := app.SendMessage(message, recipient)
	if err != nil {
		log.Panic(err)
	}
	log.Print(resp)
}
