package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler.go/tracemsg"
	"github.com/nats-io/nats.go"
)

// Scheduler is the interface to interact with the scheduler.
type Scheduler struct {
	nc *nats.Conn
}

// New creates a new instance of Scheduler.
func New(nc *nats.Conn) *Scheduler {
	return &Scheduler{
		nc: nc,
	}
}

// Install installs a new scheduled message.
// name is the unique identifier of the message. Valid characters are [a-zA-Z0-9_-].
// subject is the subject the message will be published to.
// at is the time the message should be sent.
// Use InstallOpt to set additional options.
func (c *Scheduler) Install(ctx context.Context, name string, subject string, at time.Time, opts ...InstallOpt) error {
	s := ScheduledMessage{
		Name:    name,
		Subject: subject,
		Rev:     0,
		At:      at,
	}

	for _, opt := range opts {
		opt(&s)
	}

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
		return errors.Join(ErrInstallFailed, errors.New(reason))
	}

	return nil
}

// Uninstall uninstalls a scheduled message.
func (c *Scheduler) Uninstall(ctx context.Context, name string) error {
	resp, err := c.nc.RequestMsgWithContext(ctx, &nats.Msg{
		Subject: fmt.Sprintf("scheduler.uninstall.%s", name),
		Header:  tracemsg.NewHeader(ctx),
	})
	if err != nil {
		return err
	}

	if resp.Header.Get("status") != "ok" {
		reason := resp.Header.Get("reason")
		return errors.Join(ErrorUninstallFailed, errors.New(reason))
	}

	return nil
}