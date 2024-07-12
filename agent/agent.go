package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jacobweinstock/rerun/spec"
)

// TransportReader provides a method to read an action
type TransportReader interface {
	// Read blocks until an action is available or an error occurs
	Read(ctx context.Context) (spec.Action, error)
}

// RuntimeExecutor provides a method to execute an action
type RuntimeExecutor interface {
	// Execute blocks until the action is completed or an error occurs
	Execute(ctx context.Context, action spec.Action) error
}

// TransportWriter provides a method to write an event
type TransportWriter interface {
	// Write blocks until the event is written or an error occurs
	Write(ctx context.Context, event spec.Event) error
}

type Config struct {
	TransportReader
	RuntimeExecutor
	TransportWriter
}

func (c *Config) Run(ctx context.Context, log *slog.Logger) {
	// All steps are synchronous and blocking
	// 1. get an action from the input transport
	// 3. send running/starting event to the output transport
	// 4. send the action to the runtime for execution
	// 5. send the result event to the output transport
	// 6. go to step 1

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		action, err := c.TransportReader.Read(ctx)
		if err != nil {
			log.Info("error reading/retrieving action", "error", err)
			continue
		}

		log.Info("received action", "action", action)
		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "running action", State: spec.StateRunning}); err != nil {
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", spec.StateRunning)

		state := spec.StateSuccess
		retries := action.Retries
		if retries == 0 {
			retries = 1
		}
		dur := time.Duration(action.TimeoutSeconds) * time.Second

		timeoutCtx, _ := context.WithTimeout(ctx, dur)
		for i := 1; i <= retries; i++ {
			if err := c.RuntimeExecutor.Execute(timeoutCtx, action); err != nil {
				log.Info("error executing action", "error", err, "maxRetries", retries, "currentRetry", i)
				state = spec.StateFailure
				if errors.Is(err, context.DeadlineExceeded) {
					state = spec.StateTimeout
					break
				}
				if i == retries {
					break
				}
				continue
			}
			state = spec.StateSuccess
			log.Info("executed action", "action", action)
			break
		}

		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "action completed", State: state}); err != nil {
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", state)

	}
}
