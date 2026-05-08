package util

import (
	"time"

	log "github.com/sirupsen/logrus"
)

func LogDuration(name string) func() {
	now := time.Now()
	return func() {
		log.Debug(name, " ", time.Since(now).String())
	}
}
