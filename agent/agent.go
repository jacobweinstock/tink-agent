package agent

import (
	"context"
	"log/slog"

	"github.com/jacobweinstock/rerun/proto"
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
		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "running action", State: proto.State_STATE_RUNNING}); err != nil {
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", proto.State_STATE_RUNNING)

		err = c.RuntimeExecutor.Execute(ctx, action)
		if err != nil {
			log.Info("error executing action", "error", err)
		}
		log.Info("executed action", "action", action)

		state := proto.State_STATE_SUCCESS
		if err != nil {
			state = proto.State_STATE_FAILED
		}
		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "action completed", State: state}); err != nil {
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", state)
	}
}
