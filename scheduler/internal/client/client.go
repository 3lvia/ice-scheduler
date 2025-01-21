package client

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ice-scheduler/scheduler/internal/tracemsg"
	"github.com/ice-scheduler/scheduler/scheduler"
	"github.com/nats-io/nats.go"
)

type Client struct {
	nc *nats.Conn
}

func NewClient(nc *nats.Conn) (*Client, error) {
	return &Client{
		nc: nc,
	}, nil
}

func (c *Client) Install(ctx context.Context, name string, at time.Time, payload []byte) error {
	s := scheduler.ScheduledMessage{
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

func (c *Client) Uninstall(ctx context.Context, name string) error {
	s := scheduler.ScheduledMessage{
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