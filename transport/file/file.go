package file

import (
	"context"
	"log/slog"
	"os"

	"github.com/jacobweinstock/rerun/spec"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Log     *slog.Logger
	Actions chan spec.Action
	FileLoc string
}

// func(yield func(spec.Action) bool)
func (c *Config) Start(ctx context.Context) error {
	c.Log.Info("file transport starting")
	contents, err := os.ReadFile(c.FileLoc)
	if err != nil {
		return err
	}
	actions := []spec.Action{}
	if err := yaml.Unmarshal(contents, &actions); err != nil {
		return err
	}
	for _, action := range actions {
		c.Actions <- action
	}

	return nil
}

func (c *Config) Read(ctx context.Context) (spec.Action, error) {
	select {
	case <-ctx.Done():
		return spec.Action{}, context.Canceled
	case v := <-c.Actions:
		return v, nil
	}
}

func (c *Config) Write(ctx context.Context, event spec.Event) error {
	c.Log.Info("writing event", "event", event)
	return nil
}
