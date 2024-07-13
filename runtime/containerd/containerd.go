package containerd

import (
	"context"
	"log/slog"

	"github.com/containerd/containerd"
	"github.com/jacobweinstock/rerun/spec"
)

type Config struct {
	Namespace string
	Client    *containerd.Client
	Log       *slog.Logger
}

func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	// Pull the image
	imageName := a.Image
	_, err := c.Client.GetImage(ctx, imageName)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		_, err = c.Client.Pull(ctx, imageName, containerd.WithPullUnpack)
		if err != nil {
			return err
		}
	}

	return nil
}
