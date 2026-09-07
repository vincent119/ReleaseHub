package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/vincent119/ReleaseHub/Server/internal/bootstrap"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ReleaseHub startup failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("api, worker, or migrate command is required")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := flags.String("config", config.DefaultPath, "configuration file path")
	apiAddress := &optionalString{}
	logLevel := &optionalString{}
	flags.Var(apiAddress, "api-address", "override API listen address")
	flags.Var(logLevel, "log-level", "override log level")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse command line: %w", err)
	}

	cfg, err := config.Load(*configPath, config.Overrides{
		APIAddress: apiAddress.pointer(),
		LogLevel:   logLevel.pointer(),
	})
	if err != nil {
		return err
	}
	switch command {
	case "api":
		return bootstrap.RunAPI(cfg, version)
	case "worker":
		return bootstrap.RunWorker(cfg)
	case "migrate":
		return bootstrap.RunMigrate(cfg)
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string { return o.value }

func (o *optionalString) Set(value string) error {
	o.value = value
	o.set = true
	return nil
}

func (o *optionalString) pointer() *string {
	if !o.set {
		return nil
	}
	return &o.value
}
