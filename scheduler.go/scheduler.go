package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler.go/tracemsg"
	"github.com/nats-io/nats.go"
)

// Scheduler is the interface to interact with the scheduler
type Scheduler struct {
	nc *nats.Conn
}

// New creates a new instance of Scheduler
func New(nc *nats.Conn) (*Scheduler, error) {
	return &Scheduler{
		nc: nc,
	}, nil
}

// Install installs a new scheduled message
func (c *Scheduler) Install(ctx context.Context, name string, at time.Time, payload []byte) error {
	s := ScheduledMessage{
		Name:         name,
		Rev:          0,
		At:           at,
		RepeatPolicy: nil,
		Payload:      payload,
	}

	// if err := s.Validate(); err != nil {

	d, err := json.Marshal(s)
	if err != nil {
		return err
	}

	resp, err := c.nc.RequestMsgWithContext(ctx, &nats.Msg{
		Subject: "scheduler.install",
		Data:    d,
		Header:  tracemsg.NewHeader(ctx),
	})
	if err != nil {
		return err
	}

	if resp.Header.Get("status") != "ok" {
		reason := resp.Header.Get("reason")
		return errors.Join(errors.New("failed to install"), errors.New(reason))
	}

	return nil
}

// Uninstall uninstalls a scheduled message
func (c *Scheduler) Uninstall(ctx context.Context, name string) error {
	s := ScheduledMessage{
		Name: name,
	}

	d, err := json.Marshal(s)
	if err != nil {
		return err
	}

	resp, err := c.nc.RequestMsgWithContext(ctx, &nats.Msg{
		Subject: "scheduler.uninstall",
		Data:    d,
		Header:  tracemsg.NewHeader(ctx),
	})
	if err != nil {
		return err
	}

	if resp.Header.Get("status") != "ok" {
		reason := resp.Header.Get("reason")
		return errors.Join(errors.New("failed to uninstall"), errors.New(reason))
	}

	return nil
}