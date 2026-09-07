package bootstrap

import (
	"context"

	"github.com/vincent119/commons/graceful"
)

func parallelTasks(tasks ...graceful.Task) graceful.Task {
	return func(ctx context.Context) error {
		child, cancel := context.WithCancel(ctx)
		defer cancel()
		errors := make(chan error, len(tasks))
		for _, task := range tasks {
			task := task
			go func() { errors <- task(child) }()
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-errors:
			return err
		}
	}
}
