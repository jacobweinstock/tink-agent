package nats

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/jacobweinstock/rerun/spec"
	"github.com/nats-io/nats.go"
	"gopkg.in/yaml.v3"
)

type Config struct {
	StreamName     string
	EventsSubject  string
	ActionsSubject string
	IPPort         netip.AddrPort
	Log            *slog.Logger
	AgentID        string
	Actions        chan spec.Action
	conn           *nats.Conn
	cancel         chan bool
}

func (c *Config) Start(ctx context.Context) error {
	c.cancel = make(chan bool)
	opts := []nats.Option{
		nats.Name(c.AgentID),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	}
	nc, err := nats.Connect(fmt.Sprintf("nats://%v", c.IPPort.String()), opts...)
	if err != nil {
		return err
	}
	defer nc.Close()
	c.conn = nc

	base := fmt.Sprintf("%v.%v", c.StreamName, c.AgentID)
	subj := fmt.Sprintf("%v.%v", base, c.ActionsSubject)
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	c.Log.Info("nats transport starting")
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			continue
		}

		actions := []spec.Action{}
		if err := yaml.Unmarshal(msg.Data, &actions); err != nil {
			continue
		}
		for _, action := range actions {
			select {
			case <-ctx.Done():
			case <-c.cancel:
			case c.Actions <- action:
				continue
			}
			break
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
	if event.State == spec.StateFailure || event.State == spec.StateTimeout {
		c.Actions = make(chan spec.Action)
		c.cancel <- true
	}
	return c.conn.PublishMsg(&nats.Msg{
		Subject: fmt.Sprintf("%v.%v.%v", c.StreamName, c.AgentID, c.EventsSubject),
		Data:    []byte(event.String()),
	})
}
