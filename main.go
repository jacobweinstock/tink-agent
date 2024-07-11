package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/jacobweinstock/rerun/agent"
	"github.com/jacobweinstock/rerun/proto"
	"github.com/jacobweinstock/rerun/runtime/docker"
	"github.com/jacobweinstock/rerun/spec"
	"github.com/jacobweinstock/rerun/transport/file"
	"github.com/jacobweinstock/rerun/transport/grpc"
)

const (
	// imageEnv is the name of the image that should be run for the second fork. This is set by the user.
	imageEnv = "IMAGE"
	// hostnameEnv is the name of the container that is running this process. Docker will set this.
	hostnameEnv = "HOSTNAME"
	// retryCountEnv is the amount of time to wait before running the user image. This is set by the user. Default is 10 seconds.
	retryCountEnv = "RETRY_COUNT"
	// retryMaxElapsedTimeSecondsEnv is the duration that onced reached will stop the retrying of the Action.
	retryMaxElapsedTimeSecondsEnv = "RETRY_DURATION_SECONDS"
	// dockerClientErrorCode is the exit code that should be used when the Docker client was not created successfully.
	dockerClientErrorCode = 12
)

func main() {
	// instantiate the implementation for the transport reader
	// instantiate the implementation for the transport writer
	// instantiate the implementation for the runtime executor
	// instantiate the agent
	// run the agent

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: false}))

	transport := "file"
	var tr agent.TransportReader
	var tw agent.TransportWriter
	switch transport {
	case "file":
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
	case "grpc":
		conn, err := grpc.NewClientConn("192.168.2.113:42113", false, false)
		if err != nil {
			log.Info("unable to create gRPC client", "error", err)
			os.Exit(1)
		}
		readWriter := &grpc.Config{
			Log:              log,
			TinkServerClient: proto.NewWorkflowServiceClient(conn),
			WorkerID:         "52:54:00:0f:2e:67",
			RetryInterval:    time.Second * 5,
			Actions:          make(chan spec.Action),
		}
		go readWriter.Start(ctx)
		tr = readWriter
		tw = readWriter
	}

	dclient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Println("unable to create Docker client", "error", err)
		os.Exit(dockerClientErrorCode)
	}
	dockerExecutor := &docker.Config{
		Client: dclient,
		Log:    log,
	}

	a := &agent.Config{
		TransportReader: tr,
		RuntimeExecutor: dockerExecutor,
		TransportWriter: tw,
	}

	a.Run(ctx, log)

}
