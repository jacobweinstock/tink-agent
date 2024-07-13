package containerd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containers/image/v5/pkg/shortnames"
	"github.com/containers/image/v5/types"
	"github.com/jacobweinstock/rerun/pkg/container"
	"github.com/jacobweinstock/rerun/spec"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type Config struct {
	Namespace string
	Client    *containerd.Client
	Log       *slog.Logger
}

func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	ctx = namespaces.WithNamespace(ctx, "tinkerbell")
	// Pull the image
	imageName := a.Image
	r, err := shortnames.Resolve(&types.SystemContext{PodmanOnlyShortNamesIgnoreRegistriesConfAndForceDockerHub: true}, imageName)
	if err != nil {
		c.Log.Info("unable to resolve image fully qualified name", "error", err)
	}
	if r != nil && len(r.PullCandidates) > 0 {
		imageName = r.PullCandidates[0].Value.String()
	}
	// set up a containerd namespace
	ctx = namespaces.WithNamespace(ctx, "tinkerbell")
	image, err := c.Client.GetImage(ctx, imageName)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		image, err = c.Client.Pull(ctx, imageName, containerd.WithPullUnpack, containerd.WithResolver(docker.NewResolver(docker.ResolverOptions{})))
		if err != nil {
			return fmt.Errorf("error pulling image: %w", err)
		}
	}
	c.Log.Info("image pulled", "image", image)

	// create a container
	tainer, err := c.createContainer(ctx, image, a)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	defer tainer.Delete(ctx, containerd.WithSnapshotCleanup)

	// create the task
	task, err := tainer.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("error creating task: %w", err)
	}
	defer task.Delete(ctx)

	var statusC <-chan containerd.ExitStatus
	statusC, err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("error waiting on task: %w", err)
	}

	// start the task
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		return fmt.Errorf("error starting task: %w", err)
	}

	exitStatus := <-statusC
	if exitStatus.ExitCode() != 0 {
		return fmt.Errorf("task exited with non-zero code: %d, error: %w", exitStatus.ExitCode(), exitStatus.Error())
	}
	return nil
}

func (c *Config) createContainer(ctx context.Context, image containerd.Image, action spec.Action) (containerd.Container, error) {
	newOpts := []containerd.NewContainerOpts{}
	args := []string{action.Cmd}
	args = append(args, action.Args...)
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithPrivileged,
		oci.WithEnv(container.Env(action.Env)),
		oci.WithProcessArgs(args...),
	}
	if action.Namespaces.PID == "host" {
		specOpts = append(specOpts, oci.WithHostNamespace(specs.PIDNamespace))
	}
	name := container.Name(action.ID, action.Name)
	newOpts = append(newOpts, containerd.WithNewSnapshot(name, image))
	newOpts = append(newOpts, containerd.WithNewSpec(specOpts...))

	return c.Client.NewContainer(ctx, name, newOpts...)
}

func NewConfig(client *containerd.Client, log *slog.Logger) (*Config, error) {
	c := &Config{Log: log}
	if client == nil {
		client, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			return nil, fmt.Errorf("error creating containerd client: %w", err)
		}
		c.Client = client
	}
	return c, nil
}
