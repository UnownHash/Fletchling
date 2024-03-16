package logging

import (
	"errors"
	"fmt"
	"io"
	"log"
	"path"

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
	Debug      bool   `koanf:"debug" json:"debug"`
	Filename   string `koanf:"-" json:"-"`
	LogDir     string `koanf:"log_dir" json:"log_dir"`
	MaxSizeMB  int    `koanf:"max_size" json:"max_size"` // MB
	MaxBackups int    `koanf:"max_backups" json:"max_backups"`
	MaxAgeDays int    `koanf:"max_age" json:"max_age"` // Days
	Compress   bool   `koanf:"compress" json:"compress"`
}

func (cfg *Config) FilePath() string {
	return path.Join(cfg.LogDir, cfg.Filename)
}

func (cfg *Config) Validate() error {
	return nil
}

func (cfg *Config) CreateLogger(rotate bool, wrapStdlibDefault bool, teeWriter io.Writer) (*logrus.Logger, error) {
	output := teeWriter

	if cfg.Filename != "" && cfg.LogDir != "" {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.FilePath(),
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
			LocalTime:  true,
		}

		if rotate {
			lumberjackLogger.Rotate()
		}

		if output != nil {
			// Fork writing into two outputs
			output = io.MultiWriter(output, lumberjackLogger)
		} else {
			output = lumberjackLogger
		}
	} else if output == nil {
		return nil, errors.New("CreateLogger: teeWriter is required when not Filename and LogDir are not set")
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

	return logger, nil
}
