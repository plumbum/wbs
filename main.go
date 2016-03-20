package main

import (
	"flag"
	"os"

	"gopkg.in/fsnotify.v1"

	shellwords "github.com/mattn/go-shellwords"
)

var shellParser shellwords.Parser

const defaultConfigFile = "wbs.toml"

func init() {
	shellParser.ParseEnv = true
	shellParser.ParseBacktick = true
}

func main() {
	configFile := flag.String("c", "", "configuration file path")
	flag.Parse()

	mainLogger := NewLogFunc("main")
	var (
		config *WbsConfig
		err error
	)
	if *configFile == "" {
		if _, err := os.Stat(defaultConfigFile); !os.IsNotExist(err) {
			*configFile = defaultConfigFile
		}
	}
	if *configFile != "" {
		config, err = NewWbsConfig(*configFile)
		if err != nil {
			mainLogger("failed to create config: %s", err)
			os.Exit(1)
		}
		mainLogger("use config file `%s'", *configFile)
	} else {
		config = NewWbsDefaultConfig()
	}

	watcher, err := NewWbsWatcher(config)
	if err != nil {
		mainLogger("failed to initialize watcher: %s", err)
		os.Exit(1)
	}
	runner, err := NewWbsRunner(config)
	if err != nil {
		mainLogger("failed to initialize runner: %s", err)
		os.Exit(1)
	}
	builder, err := NewWbsBuilder(config)
	if err != nil {
		mainLogger("failed to initialize builder: %s", err)
		os.Exit(1)
	}

	if err := builder.Build(); err != nil {
		mainLogger("failed to build: %s", err)
		os.Exit(1)
	}
	err = runner.Serve()
	if err != nil {
		mainLogger("failed to start server: %s", err)
		os.Exit(1)
	}
	defer runner.Stop()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.w.Events:
				switch {
				case event.Op & fsnotify.Remove == fsnotify.Remove:
					mainLogger("file remove: %s", event.String())
					if err := watcher.w.Remove(event.Name); err != nil {
						mainLogger("remove watching error: %s", err.Error())
					}
					if err := watcher.w.Add(event.Name); err != nil {
						mainLogger("remove watching error: %s", err.Error())
					}
					fallthrough
				case event.Op & fsnotify.Write == fsnotify.Write:
					mainLogger("file modified: %s", event.String())
					if config.RestartProcess {
						runner.Stop()
					}
					builder.Build()
					runner.Serve()
				default:
					mainLogger("Unhandled event: %s", event.String())
				}
			case err := <-watcher.w.Errors:
				mainLogger("error: %s", err)
			}
		}
	}()
	<-done
}
