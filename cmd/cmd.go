package cmd

import (
	"context"
	"errors"
	"flag"

	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

type TransportType string
type RuntimeType string

const (
	GRPCTransportType TransportType = "grpc"
	FileTransportType TransportType = "file"
	NATSTransportType TransportType = "nats"

	DockerRuntimeType     RuntimeType = "docker"
	ContainerdRuntimeType RuntimeType = "containerd"
)

type Config struct {
	ID        string
	LogLevel  string
	Transport struct {
		GRPC GRPCTransport
		File FileTransport
		NATS NATSTransport
	}
	Runtime struct {
		Docker     DockerRuntime
		Containerd ContainerdRuntime
	}
	Registry          Registry
	Proxy             Proxy
	TransportSelected TransportType
	RuntimeSelected   RuntimeType
}

type Registry struct {
	Name string
	User string
	Pass string
}

type Proxy struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

type GRPCTransport struct {
	ServerAddrPort string
	TLSEnabled     bool
	TLSInsecure    bool
	RetryInterval  int
}
type FileTransport struct {
	WorkflowPath string
}
type NATSTransport struct {
	ServerAddrPort string
	StreamName     string
	EventsSubject  string
	ActionsSubject string
}

type DockerRuntime struct {
	SocketPath string
}
type ContainerdRuntime struct {
	Namespace  string
	SocketPath string
}

func (c *Config) RootCommand(ctx context.Context, fs *flag.FlagSet) *ffcli.Command {
	RegisterFlagsLegacy(c, fs)
	name := fs.Name()
	cli := &ffcli.Command{
		Name:        name,
		ShortUsage:  "tink-agent [flags] <subcommand> [flags]",
		LongHelp:    "Tink Agent runs the workflows.",
		FlagSet:     fs,
		Options:     []ff.Option{ff.WithEnvVarNoPrefix()},
		Subcommands: []*ffcli.Command{TransportCommand(c)},
		Exec: func(ctx context.Context, args []string) error {
			// This is legacy mode. Only GRPC transport and Docker runtime are supported.
			c.TransportSelected = GRPCTransportType
			c.RuntimeSelected = DockerRuntimeType

			return nil
		},
	}

	return cli
}

func TransportCommand(c *Config) *ffcli.Command {
	fs := flag.NewFlagSet("transport", flag.ExitOnError)
	RegisterRuntimeFlags(c, fs)
	RegisterRootFlags(c, fs)

	// Transport command has subcommands
	g := GRPCCommand(c)
	RegisterRootFlags(c, g.FlagSet)
	RegisterRuntimeFlags(c, g.FlagSet)

	f := FileCommand(c)
	RegisterRootFlags(c, f.FlagSet)
	RegisterRuntimeFlags(c, f.FlagSet)

	n := NATSCommand(c)
	RegisterRootFlags(c, n.FlagSet)
	RegisterRuntimeFlags(c, n.FlagSet)

	cli := &ffcli.Command{
		Name:        "transport",
		ShortUsage:  "tink-agent [flags] transport [flags] <subcommand> [flags]",
		LongHelp:    "Tink Agent runs the workflows.",
		FlagSet:     fs,
		Options:     []ff.Option{ff.WithEnvVarPrefix("tink-agent")},
		Subcommands: []*ffcli.Command{g, f, n},
		Exec: func(ctx context.Context, args []string) error {
			return errors.New("please call a subcommand")
		},
	}

	return cli
}

func GRPCCommand(c *Config) *ffcli.Command {
	fs := flag.NewFlagSet("grpc", flag.ExitOnError)
	RegisterGRPCTransportFlags(c, fs)
	cli := &ffcli.Command{
		Name:       "grpc",
		ShortUsage: "tink-agent [flags] grpc [flags]",
		LongHelp:   "grpc run the agent using the gRPC transport.",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix("tink-agent")},
		Exec: func(ctx context.Context, args []string) error {
			c.TransportSelected = GRPCTransportType
			return nil
		},
	}

	return cli
}

func FileCommand(c *Config) *ffcli.Command {
	fs := flag.NewFlagSet("file", flag.ExitOnError)
	RegisterFileTransportFlags(c, fs)
	cli := &ffcli.Command{
		Name:       "file",
		ShortUsage: "tink-agent [flags]",
		LongHelp:   "Tink Agent runs the workflows.",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix("tink-agent")},
		Exec: func(ctx context.Context, args []string) error {
			c.TransportSelected = FileTransportType
			return nil
		},
	}

	return cli
}

func NATSCommand(c *Config) *ffcli.Command {
	fs := flag.NewFlagSet("nats", flag.ExitOnError)
	RegisterNATSTransportFlags(c, fs)
	cli := &ffcli.Command{
		Name:       "nats",
		ShortUsage: "tink-agent [flags]",
		LongHelp:   "Tink Agent runs the workflows.",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix("tink-agent")},
		Exec: func(ctx context.Context, args []string) error {
			c.TransportSelected = NATSTransportType
			return nil
		},
	}

	return cli
}
