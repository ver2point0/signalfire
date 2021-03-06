package main

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/starkandwayne/signalfire/bosh"
	"github.com/starkandwayne/signalfire/config"
	"github.com/starkandwayne/signalfire/core"
	"github.com/starkandwayne/signalfire/log"
	"github.com/starkandwayne/signalfire/server"
)

const (
	ConfigPathEnvvar = "CONFIG_PATH"
	DefaultConfPath  = "sf_conf.yml"
)

func main() {
	//LOGGING
	logger := log.Logger{Output: os.Stderr, Level: log.LevelDebug}

	//CONFIG
	cfgPath := os.Getenv(ConfigPathEnvvar)
	if cfgPath == "" {
		cfgPath = DefaultConfPath
	}

	logger.Info("Using configuration file at `%s'", cfgPath)
	f, err := os.Open(cfgPath)
	if err != nil {
		logger.Fatal("Error opening config file at `%s': %s", cfgPath, err)
	}

	cfg, err := config.Parse(f)
	if err != nil {
		logger.Fatal("Error parsing config YAML at `%s': %s", cfgPath, err)
	}

	logLevel, err := parseLogLevel(cfg.Log.Level)
	if err != nil {
		logger.Fatal("Error parsing log level: %s", err.Error())
	}
	logger.SetLogLevel(logLevel)

	//BOSH PARSING
	boshes := make([]core.BOSH, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		b, err := bosh.NewClient(t, &logger)
		if err != nil {
			logger.Fatal("Error initializing BOSH client for URL `%s': %s", t.URL, err)
		}

		err = b.Connect()
		if err != nil {
			logger.Fatal("Could not start BOSH client: %s", err)
		}

		boshes = append(boshes, core.BOSH{
			Client:       b,
			PollInterval: time.Duration(t.PollInterval) * time.Second,
		})
	}

	//Configure the core logic orchestration
	cache := core.NewCache()
	collator := core.NewCollator(&logger)
	//TODO: Make the rules configurable
	collator.AddRule(core.DeploymentRegexCaptureRule{Match: regexp.MustCompile(`.*-(.*)`)})
	collator.AddRule(core.DeploymentRegexCaptureRule{Match: regexp.MustCompile(`(.*)`)})
	collator.WatchAsync(cache)

	scheduler := core.Scheduler{
		Boshes: boshes,
		Cache:  cache,
		Logger: &logger,
	}
	scheduler.Start()

	//Start up the HTTP API
	serv, err := server.New(cfg.Server, server.Components{
		Collator: collator,
		Cache:    cache,
		Log:      &logger,
	})
	if err != nil {
		logger.Fatal("Could not initialize server: %s", err)
	}

	err = serv.Run()
	if err != nil {
		logger.Fatal("Server exited: %s", err)
	}

}

func parseLogLevel(level string) (ret uint, err error) {
	switch level {
	case config.LogLevelDebug:
		ret = log.LevelDebug
	case config.LogLevelInfo:
		ret = log.LevelInfo
	case config.LogLevelError:
		ret = log.LevelError
	case config.LogLevelFatal:
		ret = log.LevelFatal
	default:
		err = fmt.Errorf("Unknown log level `%s'", level)
	}

	return
}
