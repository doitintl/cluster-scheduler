package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/doitintl/cluster-scheduler/internal/scheduler/gke"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	log "github.com/sirupsen/logrus"
)

var (
	// main context
	mainCtx context.Context
	// Version contains the current version.
	Version = "dev"
	// BuildDate contains a string with the build date.
	BuildDate = "unknown"
	// GitCommit git commit SHA
	GitCommit = "dirty"
	// GitBranch git branch
	GitBranch = "master"

	// internal
	logger *log.Logger
)

func init() {
	logger = log.New()
	// set log level
	logger.SetLevel(log.WarnLevel)
	logger.SetFormatter(&log.TextFormatter{})
}

func before(c *cli.Context) error {
	// set debug log level
	switch level := c.String("log-level"); level {
	case "debug", "DEBUG":
		logger.SetLevel(log.DebugLevel)
	case "info", "INFO":
		logger.SetLevel(log.InfoLevel)
	case "warning", "WARNING":
		logger.SetLevel(log.WarnLevel)
	case "error", "ERROR":
		logger.SetLevel(log.ErrorLevel)
	case "fatal", "FATAL":
		logger.SetLevel(log.FatalLevel)
	case "panic", "PANIC":
		logger.SetLevel(log.PanicLevel)
	default:
		logger.SetLevel(log.WarnLevel)
	}
	// set log formatter to JSON
	if c.Bool("json") {
		logger.SetFormatter(&log.JSONFormatter{})
	}
	return nil
}

func listCmd(c *cli.Context) error {
	log.Debug("list clusters")
	runner, _ := gke.NewGkeScheduler(mainCtx)
	clusters, err := runner.List(mainCtx)
	if err != nil {
		return errors.Wrap(err, "failed list command")
	}
	for _, c := range clusters {
		//log.WithFields()
		runner.DecideOnStatus(c)
	}
	return nil
}

func stopCmd(c *cli.Context) error {
	runner, _ := gke.NewGkeScheduler(mainCtx)
	clusters, err := runner.List(mainCtx)
	if err != nil {
		return errors.Wrap(err, "failed list command")
	}
	for _, c := range clusters {
		log.WithFields(log.Fields{
			"cluster": c.Name,
			"project": c.Project,
		}).Info("stopping cluster")
		err := runner.Stop(mainCtx, c)
		if err != nil {
			return errors.Wrap(err, "failed to stop cluster")
		}
	}
	return nil
}

func restartCmd(c *cli.Context) error {
	return nil
}

func init() {
	// handle termination signal
	mainCtx = handleSignals()
}

func handleSignals() context.Context {
	// Graceful shut-down on SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// create cancelable context
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()
		sid := <-sig
		log.Printf("received signal: %d\n", sid)
		log.Println("canceling main command ...")
	}()

	return ctx
}

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:      "stop",
				Usage:     "stop managed Kubernetes clusters",
				UsageText: "use this command in manual mode only",
				Action:    stopCmd,
			},
			{
				Name:      "restart",
				Usage:     "restart previously stopped managed Kubernetes clusters",
				UsageText: "use this command in manual mode only",
				Action:    restartCmd,
			},
			{
				Name:      "list",
				Usage:     "list managed Kubernetes clusters",
				UsageText: "use this command in manual mode only",
				Action:    listCmd,
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "produce log in JSON format: Logstash and Splunk friendly",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "set log level (debug, info, warning(*), error, fatal, panic)",
				Value: "warning",
			},
		},
		Name:    "cluster-scheduler",
		Usage:   "cluster-scheduler CLI",
		Before:  before,
		Version: Version,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("cluster-scheduler %s\n", Version)
		fmt.Printf("  Build date: %s\n", BuildDate)
		fmt.Printf("  Git commit: %s\n", GitCommit)
		fmt.Printf("  Git branch: %s\n", GitBranch)
		fmt.Printf("  Built with: %s\n", runtime.Version())
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
