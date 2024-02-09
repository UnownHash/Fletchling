package logging

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type PlainFormatter struct {
	TimestampFormat string
	LevelDesc       []string
}

func (f *PlainFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := fmt.Sprintf(entry.Time.Format(f.TimestampFormat))
	return []byte(fmt.Sprintf("%s %s %s\n", f.LevelDesc[entry.Level], timestamp, entry.Message)), nil
}

type Config struct {
	Debug      bool   `koanf:"debug"`
	Filename   string `koanf:"filename"`
	MaxSizeMB  int    `koanf:"max_size"` // MB
	MaxBackups int    `koanf:"max_backups"`
	MaxAgeDays int    `koanf:"max_age"` // Days
	Compress   bool   `koanf:"compress"`
}

func (cfg *Config) Validate() error {
	return nil
}

func (cfg *Config) CreateLogger(rotate bool, wrapStdlibDefault bool) *logrus.Logger {
	output := io.Writer(os.Stdout)

	if cfg.Filename != "" {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
			LocalTime:  true,
		}

		if rotate {
			lumberjackLogger.Rotate()
		}

		// Fork writing into two outputs
		output = io.MultiWriter(output, lumberjackLogger)
	}

	logFormatter := &PlainFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LevelDesc:       []string{"PANC", "FATL", "ERRO", "WARN", "INFO", "DEBG"},
	}

	logger := logrus.New()
	logger.SetFormatter(logFormatter)
	if cfg.Debug {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}
	logger.SetOutput(output)

	if wrapStdlibDefault {
		log.SetOutput(logger.Writer())
	}

	return logger
}
