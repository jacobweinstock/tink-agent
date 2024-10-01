package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/jacobweinstock/rerun/agent"
	"github.com/jacobweinstock/rerun/cmd"
	"github.com/jacobweinstock/rerun/runtime/containerd"
	"github.com/jacobweinstock/rerun/runtime/docker"
	"github.com/jacobweinstock/rerun/spec"
	"github.com/jacobweinstock/rerun/transport/file"
	"github.com/jacobweinstock/rerun/transport/grpc"
	"github.com/jacobweinstock/rerun/transport/grpc/proto"
	"github.com/jacobweinstock/rerun/transport/nats"
	"golang.org/x/sync/errgroup"
)

const (
	// dockerClientErrorCode is the exit code that should be used when the Docker client was not created successfully.
	dockerClientErrorCode = 12
	// name is the name of the agent.
	name = "tink-agent"
)

func main() {
	// instantiate the implementation for the transport reader
	// instantiate the implementation for the transport writer
	// instantiate the implementation for the runtime executor
	// instantiate the agent
	// run the agent

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()

	c := &cmd.Config{}
	ce := c.RootCommand(ctx, flag.NewFlagSet(name, flag.ExitOnError))

	// Currently, the Run method only populates config fields. It does not execute anything.
	if err := ce.ParseAndRun(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		ce.FlagSet.Usage()
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: false}))

	eg, ectx := errgroup.WithContext(ctx)
	ctx = ectx
	var tr agent.TransportReader
	var tw agent.TransportWriter
	switch c.TransportSelected {
	case cmd.FileTransportType:
		readWriter := &file.Config{
			Log:     log,
			Actions: make(chan spec.Action),
			FileLoc: "./example/file_template.yaml",
		}
		go func() {
			if err := readWriter.Start(ctx); err != nil {
				log.Info("unable to start file transport", "error", err)
				os.Exit(1)
			}

		}()
		tr = readWriter
		tw = readWriter
	case cmd.GRPCTransportType:
		conn, err := grpc.NewClientConn(c.Transport.GRPC.ServerAddrPort, c.Transport.GRPC.TLSEnabled, c.Transport.GRPC.TLSInsecure)
		if err != nil {
			log.Info("unable to create gRPC client", "error", err)
			os.Exit(1)
		}
		readWriter := &grpc.Config{
			Log:              log,
			TinkServerClient: proto.NewWorkflowServiceClient(conn),
			WorkerID:         c.ID,
			RetryInterval:    time.Second * 5,
			Actions:          make(chan spec.Action),
		}
		eg.Go(func() error {
			readWriter.Start(ctx)
			return nil
		})
		tr = readWriter
		tw = readWriter
	case cmd.NATSTransportType:
		readWriter := &nats.Config{
			StreamName:     c.Transport.NATS.StreamName,
			EventsSubject:  c.Transport.NATS.EventsSubject,
			ActionsSubject: c.Transport.NATS.ActionsSubject,
			IPPort:         netip.MustParseAddrPort(c.Transport.NATS.ServerAddrPort),
			Log:            log,
			AgentID:        c.ID,
			Actions:        make(chan spec.Action),
		}
		eg.Go(func() error {
			return readWriter.Start(ctx)
		})
		tr = readWriter
		tw = readWriter
	}

	var re agent.RuntimeExecutor
	switch c.RuntimeSelected {
	case cmd.DockerRuntimeType:
		opts := []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}
		if c.Runtime.Docker.SocketPath != "" {
			opts = append(opts, client.WithHost(fmt.Sprintf("unix://%s", c.Runtime.Docker.SocketPath)))
		}
		dclient, err := client.NewClientWithOpts(opts...)
		if err != nil {
			log.Info("unable to create Docker client", "error", err)
			os.Exit(dockerClientErrorCode)
		}
		// TODO(jacobweinstock): handle auth
		dockerExecutor := &docker.Config{
			Client: dclient,
			Log:    log,
		}
		re = dockerExecutor
		log.Info("using Docker runtime")
	case cmd.ContainerdRuntimeType:
		opts := []containerd.Opt{}
		if c.Runtime.Containerd.Namespace != "" {
			opts = append(opts, containerd.WithNamespace(c.Runtime.Containerd.Namespace))
		}
		if c.Runtime.Containerd.SocketPath != "" {
			opts = append(opts, containerd.WithSocketPath(c.Runtime.Containerd.SocketPath))
		}
		cd, err := containerd.NewConfig(log, opts...)
		if err != nil {
			log.Info("unable to create containerd config", "error", err)
			os.Exit(1)
		}
		re = cd
		log.Info("using containerd runtime")
	default:
		log.Info("no runtime selected, defaulting to Docker")
		c.RuntimeSelected = cmd.DockerRuntimeType
	}

	a := &agent.Config{
		TransportReader: tr,
		RuntimeExecutor: re,
		TransportWriter: tw,
	}

	eg.Go(func() error {
		a.Run(ctx, log)
		return nil
	})

	_ = eg.Wait()

}
