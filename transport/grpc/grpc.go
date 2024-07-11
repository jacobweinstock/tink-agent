package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"crypto/tls"

	"github.com/jacobweinstock/rerun/proto"
	"github.com/jacobweinstock/rerun/spec"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	Log              *slog.Logger
	TinkServerClient proto.WorkflowServiceClient
	WorkerID         string
	RetryInterval    time.Duration
	Actions          chan spec.Action
}

func (c *Config) Start(ctx context.Context) error {
	c.Log.Info("grpc transport starting")
LOOP1:
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		stream, err := c.TinkServerClient.GetWorkflowContexts(ctx, &proto.WorkflowContextRequest{WorkerId: c.WorkerID})
		if err != nil {
			<-time.After(c.RetryInterval)
			continue
		}
		for {
			request, err := stream.Recv()
			switch {
			case err != nil:
				<-time.After(c.RetryInterval)
				goto LOOP1
			}
			if request == nil || request.GetCurrentWorker() != c.WorkerID {
				goto LOOP1
			}
			switch request.GetCurrentActionState() {
			// only get actions whose state is STATE_PENDING
			case proto.State_STATE_SUCCESS, proto.State_STATE_RUNNING, proto.State_STATE_FAILED, proto.State_STATE_TIMEOUT:
				continue
			}

			actions, err := c.TinkServerClient.GetWorkflowActions(ctx, &proto.WorkflowActionsRequest{WorkflowId: request.GetWorkflowId()})
			if err != nil {
				continue
			}
			for _, act := range actions.GetActionList() {
				if request.GetCurrentActionState() == proto.State_STATE_PENDING {
					action := spec.Action{
						TaskName:   request.GetCurrentTask(),
						ID:         request.GetWorkflowId(),
						Name:       act.Name,
						Image:      act.Image,
						Cmd:        act.Command[0],
						Args:       act.Command[1:],
						Env:        []spec.Env{},
						Volumes:    []spec.Volume{},
						Namespaces: spec.Namespaces{},
						Retries:    0,
					}
					for _, v := range act.Volumes {
						action.Volumes = append(action.Volumes, spec.Volume(v))
					}
					for _, v := range act.GetEnvironment() {
						kv := strings.Split(v, "=")
						env := spec.Env{
							Key:   kv[0],
							Value: kv[1],
						}
						action.Env = append(action.Env, env)
					}
					action.Namespaces.PID = act.GetPid()

					c.Actions <- action
				}
			}
		}
	}
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
	ar := &proto.WorkflowActionStatus{
		WorkflowId:   event.Action.ID,
		TaskName:     event.Action.TaskName,
		ActionName:   event.Action.Name,
		ActionStatus: event.State,
		Seconds:      0,
		Message:      event.Message,
		WorkerId:     c.WorkerID,
	}
	_, err := c.TinkServerClient.ReportActionStatus(ctx, ar)
	if err != nil {
		return fmt.Errorf("error reporting action: %v: %w", ar, err)
	}
	// I don't like this but it seems to be necessary in order to avoid "reported action name does not match the current action details" errors.
	// The errors don't seem to affect the outcome of the action, but I'm not entirely certain of that.
	// This sleep give Tink server/Kubernetes time to update the status of the action.
	time.Sleep(time.Second * 2)
	return nil
}

func NewClientConn(authority string, tlsEnabled bool, tlsInsecure bool) (*grpc.ClientConn, error) {
	var creds grpc.DialOption
	if tlsEnabled { // #nosec G402
		creds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: tlsInsecure}))
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.Dial(authority, creds, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, fmt.Errorf("dial tinkerbell server: %w", err)
	}

	return conn, nil
}
