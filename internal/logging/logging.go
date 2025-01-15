package logging

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

func SetupLogging(loglevel string) error {

	log.SetOutput(os.Stdout)

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	level, err := log.ParseLevel(loglevel)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %v", level, err)
	}

	log.SetLevel(level)

	return nil
}
