package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	retry "github.com/avast/retry-go"
	"github.com/aws/smithy-go/ptr"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/jacobweinstock/rerun/pkg/conv"
	"github.com/jacobweinstock/rerun/spec"
)

const (
	// registryUsernameCmdlineKey is the key for the username for the registry that is found in /proc/cmdline.
	registryUsernameCmdlineKey = "registry_username"
	// registryPasswordCmdlineKey is the key for the password for the registry that is found in /proc/cmdline.
	registryPasswordCmdlineKey = "registry_password"
	// registryCmdlineKey is the key for the registry that is found in /proc/cmdline.
	registryCmdlineKey = "docker_registry"
)

var (
	// cmdlinePath is the path to the /proc/cmdline file.
	cmdlinePath = "/proc/cmdline"
)

type Config struct {
	Log    *slog.Logger
	Client *client.Client
}

func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	pullImage := func() error {
		// We need the image to be available before we can create a container.
		// TODO(jacobweinstock): add auth
		/*
			var authStr string
			if regAuth != nil {
				encodedJSON, err := json.Marshal(regAuth)
				if err != nil {
					return fmt.Errorf("unable to encode auth config: %w", err)
				}
				authStr = base64.URLEncoding.EncodeToString(encodedJSON)
			}
		*/
		img, err := c.Client.ImagePull(ctx, a.Image, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("docker: %w", err)
		}
		defer img.Close()

		// Docker requires everything to be read from the images ReadCloser for the image to actually
		// be pulled. We may want to log image pulls in a circular buffer somewhere for debugability.
		if _, err = io.Copy(io.Discard, img); err != nil {
			return fmt.Errorf("docker: %w", err)
		}

		return nil
	}

	err := retry.Do(pullImage, retry.Attempts(5), retry.DelayType(retry.BackOffDelay))
	if err != nil {
		return err
	}

	// TODO: Support all the other things on the action such as volumes.
	cfg := container.Config{
		Image: a.Image,
		Env:   conv.ParseEnv(a.Env),
	}

	hostCfg := container.HostConfig{
		Mounts: []mount.Mount{},
	}
	if a.Namespaces.PID != "" {
		hostCfg.PidMode = container.PidMode(a.Namespaces.PID)
	}
	for _, v := range a.Volumes {
		parsed := strings.SplitN(string(v), ":", 3)
		if len(parsed) < 2 {
			continue
		}
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Source: parsed[0],
			Target: parsed[1],
		})
	}

	containerName := conv.ParseName(a.ID, a.Name)

	// Docker uses the entrypoint as the default command. The Tink Action Cmd property is modeled
	// as being the command launched in the container hence it is used as the entrypoint. Args
	// on the action are therefore the command portion in Docker.
	if a.Cmd != "" {
		cfg.Entrypoint = append(cfg.Entrypoint, a.Cmd)
	}
	if len(a.Args) > 0 {
		cfg.Cmd = append(cfg.Cmd, a.Args...)
	}

	// TODO: Figure out container logging. We probably want to save it somewhere for debugability.

	create, err := c.Client.ContainerCreate(ctx, &cfg, &hostCfg, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}

	// Always try to remove the container on exit.
	defer func() {
		// Force remove containers in an attempt to preserve space in memory constraints environments.
		// In rare cases this may create orphaned volumes that the Docker CLI won't clean up.
		opts := container.RemoveOptions{Force: true}

		// We can't use the context passed to Run() as it may have been cancelled so we use Background()
		// instead.
		err := c.Client.ContainerRemove(context.Background(), create.ID, opts)
		if err != nil {
			c.Log.Info("Couldn't remove container", "container_name", containerName, "error", err)
		}
	}()

	// Issue the wait with a 'next-exit' condition so we can await a response originating from
	// ContainerStart().
	waitBody, waitErr := c.Client.ContainerWait(ctx, create.ID, container.WaitConditionNextExit)

	if err := c.Client.ContainerStart(ctx, create.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}

	select {
	case result := <-waitBody:
		if result.StatusCode == 0 {
			return nil
		}
		return fmt.Errorf("got non 0 exit status, see the logs for more information")

	case err := <-waitErr:
		return fmt.Errorf("error while waiting for container: %w", err)

	case <-ctx.Done():
		// We can't use the context passed to Run() as its been cancelled.
		err := c.Client.ContainerStop(context.Background(), create.ID, container.StopOptions{
			Timeout: ptr.Int(5),
		})
		if err != nil {
			c.Log.Info("Failed to gracefully stop container", "error", err)
		}
		return fmt.Errorf("context error: %w", ctx.Err())
	}
}

func parseCmdline(cmdlinePath string) (map[string]string, error) {
	cmdline, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read /proc/cmdline: %w", err)
	}

	cmdlineMap := make(map[string]string)
	for _, arg := range strings.Split(string(cmdline), " ") {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 0 {
			continue
		}
		cmdlineMap[kv[0]] = kv[1]
	}

	return cmdlineMap, nil
}

// toRegAuth right now we only support username and password for a registry
func toRegAuth(cmdline map[string]string) (*registry.AuthConfig, error) {
	var found bool
	authConfig := &registry.AuthConfig{}

	authConfig.Username, found = cmdline[registryUsernameCmdlineKey]
	if !found {
		return nil, fmt.Errorf("unable to find %s in /proc/cmdline", registryUsernameCmdlineKey)
	}
	authConfig.Password, found = cmdline[registryPasswordCmdlineKey]
	if !found {
		return nil, fmt.Errorf("unable to find %s in /proc/cmdline", registryPasswordCmdlineKey)
	}
	authConfig.ServerAddress, found = cmdline[registryCmdlineKey]
	if !found {
		return nil, fmt.Errorf("unable to find %s in /proc/cmdline", registryCmdlineKey)
	}

	return authConfig, nil
}
