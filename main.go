package main

import (
	"context"
	"fmt"
	"github.com/doitintl/cluster-scheduler/internal/scheduler"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/doitintl/cluster-scheduler/internal/scheduler/aws"
	"github.com/doitintl/cluster-scheduler/internal/scheduler/gke"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	log "github.com/sirupsen/logrus"
)

var (
	// main context
	mainCtx context.Context
	// cluster scheduler runner
	runner scheduler.Runner
	// Version contains the current version.
	Version = "dev"
	// BuildDate contains a string with the build date.
	BuildDate = "unknown"
	// GitCommit git commit SHA
	GitCommit = "dirty"
	// GitBranch git branch
	GitBranch = "master"
)

func init() {
	// set log level
	log.SetLevel(log.WarnLevel)
	log.SetFormatter(&log.TextFormatter{})
}

func before(c *cli.Context) error {
	// set debug log level
	switch level := c.String("log-level"); level {
	case "debug", "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "info", "INFO":
		log.SetLevel(log.InfoLevel)
	case "warning", "WARNING":
		log.SetLevel(log.WarnLevel)
	case "error", "ERROR":
		log.SetLevel(log.ErrorLevel)
	case "fatal", "FATAL":
		log.SetLevel(log.FatalLevel)
	case "panic", "PANIC":
		log.SetLevel(log.PanicLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}
	// set log formatter to JSON
	if c.Bool("json") {
		log.SetFormatter(&log.JSONFormatter{})
	}
	// set default scheduler runner
	var err error
	switch cluster := c.String("cluster"); cluster {
	case "gke":
		runner, err = gke.NewGkeScheduler(mainCtx)
	case "eks":
		runner, err = aws.NewEksScheduler(mainCtx)
	default:
		runner, err = gke.NewGkeScheduler(mainCtx)
	}

	return err
}

func listCmd(c *cli.Context) error {
	log.Debug("list clusters")
	clusters, err := runner.List(mainCtx)
	if err != nil {
		return errors.Wrap(err, "failed list clusters")
	}
	for _, c := range clusters {
		//log.WithFields()
		err := runner.DecideOnStatus(c)
		if err != nil {
			return errors.Wrap(err, "failed to decide on next cluster status")
		}
	}
	return nil
}

func stopCmd(c *cli.Context) error {
	clusters, err := runner.List(mainCtx)
	if err != nil {
		return errors.Wrap(err, "failed list clusters")
	}
	log.Debug("stopping clusters")
	for _, c := range clusters {
		err := runner.Stop(mainCtx, c)
		if err != nil {
			return errors.Wrap(err, "failed to stop cluster")
		}
	}
	return nil
}

func restartCmd(c *cli.Context) error {
	clusters, err := runner.List(mainCtx)
	if err != nil {
		return errors.Wrap(err, "failed list clusters")
	}
	log.Debug("restarting clusters")
	for _, c := range clusters {
		err := runner.Restart(mainCtx, c)
		if err != nil {
			return errors.Wrap(err, "failed to restart cluster")
		}
	}
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
			&cli.StringFlag{
				Name:    "cluster",
				Aliases: nil,
				Usage:   "specify cluster type (eks, gke)",
				Value:   "gke",
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
