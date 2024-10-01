package cmd

import (
	"flag"
	"fmt"
	"strings"
)

func RegisterFlagsLegacy(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.ID, "id", "", "ID of the agent")
	fs.StringVar(&c.Registry.User, "registry-username", "", "Registry username")
	fs.StringVar(&c.Registry.Pass, "registry-password", "", "Registry password")
	fs.StringVar(&c.Registry.Name, "docker-registry", "", "Registry name")
	fs.StringVar(&c.Transport.GRPC.ServerAddrPort, "tinkerbell-grpc-authority", "", "Tink server GRPC IP:Port")
	fs.BoolVar(&c.Transport.GRPC.TLSInsecure, "tinkerbell-insecure-tls", false, "Tink server GRPC insecure TLS")
	fs.BoolVar(&c.Transport.GRPC.TLSEnabled, "tinkerbell-tls", true, "Tink server GRPC use TLS")
}

func RegisterRootFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.ID, "id", "", "ID of the agent")
	fs.StringVar(&c.LogLevel, "log-level", "info", "Log level")
	fs.StringVar(&c.Registry.Name, "registry-name", "", "Registry name")
	fs.StringVar(&c.Registry.User, "registry-user", "", "Registry user")
	fs.StringVar(&c.Registry.Pass, "registry-pass", "", "Registry pass")
	fs.StringVar(&c.Proxy.HTTPProxy, "http-proxy", "", "HTTP proxy")
	fs.StringVar(&c.Proxy.HTTPSProxy, "https-proxy", "", "HTTPS proxy")
	fs.StringVar(&c.Proxy.NoProxy, "no-proxy", "", "No proxy")
}

func RegisterRuntimeFlags(c *Config, fs *flag.FlagSet) {
	fs.Func("runtime", fmt.Sprintf("Runtime, must be one of [%s, %s]", DockerRuntimeType, ContainerdRuntimeType), func(s string) error {
		switch strings.ToLower(s) {
		case "docker":
			c.RuntimeSelected = DockerRuntimeType
		case "containerd":
			c.RuntimeSelected = ContainerdRuntimeType
		default:
			c.RuntimeSelected = DockerRuntimeType
			// return fmt.Errorf("invalid runtime, must be one of: [%s, %s]", DockerRuntimeType, ContainerdRuntimeType)
		}
		return nil
	})
	RegisterDockerRuntimeFlags(c, fs)
	RegisterContainerdRuntimeFlags(c, fs)
}

func RegisterGRPCTransportFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.Transport.GRPC.ServerAddrPort, "grpc-server", "", "gRPC server address:port")
	fs.BoolVar(&c.Transport.GRPC.TLSEnabled, "grpc-tls", false, "gRPC TLS enabled")
	fs.BoolVar(&c.Transport.GRPC.TLSInsecure, "grpc-insecure-tls", false, "gRPC insecure TLS")
	fs.IntVar(&c.Transport.GRPC.RetryInterval, "grpc-retry-interval", 5, "gRPC retry interval")
}

func RegisterFileTransportFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.Transport.File.WorkflowPath, "workflow-pah", "", "Workflow file path")
}

func RegisterNATSTransportFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.Transport.NATS.ServerAddrPort, "nats-server", "", "NATS server address:port")
	fs.StringVar(&c.Transport.NATS.StreamName, "nats-stream", "tinkerbell", "NATS stream name")
	fs.StringVar(&c.Transport.NATS.EventsSubject, "nats-events", "workflow_status", "NATS events subject")
	fs.StringVar(&c.Transport.NATS.ActionsSubject, "nats-actions", "workflow_actions", "NATS actions subject")
}

func RegisterDockerRuntimeFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.Runtime.Docker.SocketPath, "docker-socket", "/var/run/docker.sock", "Docker socket path")
}
func RegisterContainerdRuntimeFlags(c *Config, fs *flag.FlagSet) {
	fs.StringVar(&c.Runtime.Containerd.Namespace, "containerd-namespace", "tinkerbell", "Containerd namespace")
	fs.StringVar(&c.Runtime.Containerd.SocketPath, "containerd-socket", "/run/containerd/containerd.sock", "Containerd socket path")
}
