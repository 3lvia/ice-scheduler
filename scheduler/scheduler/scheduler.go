package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler/config"
	"github.com/3lvia/libraries-go/pkg/elvia/tracemsg"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// InstallSubject is the subject for installing a message.
	InstallSubject = "scheduler.install"

	// UninstallSubject is the subject for uninstalling a message.
	UninstallSubject = "scheduler.uninstall.*"
)

type Opt func(*Config)

type Config struct {
	Store       Store
	MinInterval time.Duration
}

func defaultConfig() Config {
	return Config{
		Store:       nil,
		MinInterval: 1 * time.Minute,
	}
}

func WithStore(store Store) Opt {
	return func(cfg *Config) {
		cfg.Store = store
	}
}

func WithMinInterval(minInterval time.Duration) Opt {
	return func(cfg *Config) {
		cfg.MinInterval = minInterval
	}
}

type Scheduler struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	consumer  jetstream.Consumer
	tracer    trace.Tracer
	store     Store
	installer *Installer

	startTime time.Time
}

func New(ctx context.Context, nc *nats.Conn, opts ...Opt) (*Scheduler, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.Store == nil {
		var err error
		cfg.Store, err = NewJetstreamStore(ctx, nc)
		if err != nil {
			return nil, err
		}
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	s, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "scheduled",
		Subjects:    []string{"scheduled.>"},
		Description: "Scheduled messages",
		Discard:     jetstream.DiscardNew,
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, err
	}

	consumer, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:   "scheduler",
		AckPolicy: jetstream.AckExplicitPolicy,
		// The maximum number of messages that can be scheduled is tied to the maximum number of messages that can be pending
		// Increase this value to allow more messages to be scheduled, or set to -1 for unlimited
		MaxAckPending: 1000,
	})
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		nc:        nc,
		js:        js,
		consumer:  consumer,
		tracer:    otel.Tracer(config.TracerName),
		store:     cfg.Store,
		installer: NewInstaller(cfg.Store, cfg.MinInterval),
		startTime: time.Now(),
	}, nil
}

func (s *Scheduler) install(ctx context.Context, msg *nats.Msg) error {
	var scheduledMsg ScheduledMessage
	err := json.Unmarshal(msg.Data, &scheduledMsg)
	if err != nil {
		return errors.Join(ErrFailedToUnmarshal, err)
	}

	return s.installSchedule(ctx, &scheduledMsg)
}

func (s *Scheduler) installSchedule(ctx context.Context, scheduledMsg *ScheduledMessage) error {
	updated, err := s.installer.Install(ctx, scheduledMsg)
	if err != nil {
		return errors.Join(ErrFailedToInstall, err)
	}

	if !updated {
		slog.InfoContext(ctx, "message already installed", "name", scheduledMsg.Name)
		return nil
	}

	slog.InfoContext(ctx, "message installed", "name", scheduledMsg.Name, "rev", scheduledMsg.Rev)

	err = s.publishToScheduleStream(ctx, scheduledMsg)
	if err != nil {
		if err = s.installer.Uninstall(ctx, scheduledMsg.Name); err != nil {
			return errors.Join(ErrFailedToPublish, ErrFailedToUninstall, err)
		}
		return ErrFailedToPublish
	}
	return nil
}

func (s *Scheduler) publishToScheduleStream(ctx context.Context, scheduledMsg *ScheduledMessage) error {
	header := make(nats.Header)
	header["rev"] = []string{fmt.Sprintf("%d", scheduledMsg.Rev)}
	_, err := s.js.PublishMsg(ctx, &nats.Msg{
		Subject: "scheduled." + scheduledMsg.Name,
		Header:  header,
	})
	return err
}

func (s *Scheduler) uninstall(ctx context.Context, msg *nats.Msg) error {
	// remove the "scheduler.uninstall." prefix from the subject
	name := msg.Subject[18:]

	err := s.installer.Uninstall(ctx, name)
	if err != nil {
		return errors.Join(ErrFailedToUninstall, err)
	}

	slog.InfoContext(ctx, "message uninstalled", "name", name)
	return nil
}

func (s *Scheduler) Start() (func(), error) {
	installSub, err := s.nc.Subscribe(InstallSubject, func(msg *nats.Msg) {
		ctx := tracemsg.Extract(context.Background(), msg.Header)
		ctx, span := s.tracer.Start(ctx, "scheduler.install")
		defer span.End()

		err := s.install(ctx, msg)

		header := nats.Header{}
		if err != nil {
			header["status"] = []string{"error"}
			header["reason"] = []string{err.Error()}

			slog.ErrorContext(ctx, "failed to install message", "error", err)
		} else {
			header["status"] = []string{"ok"}
		}

		err = msg.RespondMsg(&nats.Msg{
			Header: tracemsg.Inject(ctx, header),
		})

		if err != nil {
			slog.ErrorContext(ctx, "failed to respond to install message", "error", err)
		}

		if err = msg.Ack(); err != nil {
			slog.ErrorContext(ctx, "failed to ACK message", "error", err)
		}
	})

	if err != nil {
		return nil, err
	}

	uninstallSub, err := s.nc.Subscribe(UninstallSubject, func(msg *nats.Msg) {
		ctx := tracemsg.Extract(context.Background(), msg.Header)
		ctx, span := s.tracer.Start(ctx, "scheduler.uninstall")
		defer span.End()

		err := s.uninstall(ctx, msg)

		header := nats.Header{}
		if err != nil {
			header["status"] = []string{"error"}
			header["reason"] = []string{err.Error()}

			slog.ErrorContext(ctx, "failed to uninstall message", "error", err)
		} else {
			header["status"] = []string{"ok"}
		}

		err = msg.RespondMsg(&nats.Msg{
			Header: tracemsg.Inject(ctx, header),
		})

		if err != nil {
			slog.ErrorContext(ctx, "failed to respond to uninstall message", "error", err)
		}

		if err = msg.Ack(); err != nil {
			slog.ErrorContext(ctx, "failed to ACK message", "error", err)
		}
	})

	if err != nil {
		return nil, err
	}

	c, err := s.consumer.Consume(func(msg jetstream.Msg) {
		ctx := tracemsg.Extract(context.Background(), msg.Headers())
		s.handle(ctx, msg)
	})
	if err != nil {
		return nil, err
	}

	return func() {
		_ = installSub.Unsubscribe()
		_ = uninstallSub.Unsubscribe()
		c.Stop()
	}, nil
}

func (s *Scheduler) handle(ctx context.Context, msg jetstream.Msg) {
	ctx, span := s.tracer.Start(ctx, "scheduler.handle")
	defer span.End()

	meta, err := msg.Metadata()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get message metadata", "error", err)

		if err = msg.TermWithReason("Failed to get jetstream metadata"); err != nil {
			span.SetStatus(codes.Error, "Failed to TERM message for missing metadata")
			slog.ErrorContext(ctx, "failed to TERM message for missing metadata", "error", err)
		}
		return
	}
	slog.DebugContext(ctx, "received a JetStream message", "seq", meta.Sequence.Stream)

	// remove the "scheduled." prefix from the name
	name := msg.Subject()[10:]

	extractRevHeader := func(header nats.Header) (uint32, error) {
		var rev uint64
		var revHeader []string
		if revHeader = header["rev"]; len(revHeader) == 0 {
			return 0, errors.New("missing rev header")
		}

		rev, err := strconv.ParseUint(revHeader[0], 10, 32)
		if err != nil {
			return 0, errors.New("failed to parse rev header")
		}

		return uint32(rev), nil
	}

	msgRev, err := extractRevHeader(msg.Headers())
	if err != nil {
		slog.ErrorContext(ctx, "failed to parse message rev", "error", err)

		if err = msg.TermWithReason("Failed to parse message rev"); err != nil {
			span.SetStatus(codes.Error, "Failed to TERM message for missing rev")
			slog.ErrorContext(ctx, "failed to TERM message for missing rev", "error", err)
		}
		return
	}

	stored, rev, err := s.store.Get(ctx, name)

	// if the key is not found, it has been uninstalled
	// therefore, we ignore the message
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		slog.InfoContext(ctx, "message was uninstalled", "name", name)
		if err = msg.Ack(); err != nil {
			span.SetStatus(codes.Error, "Failed to ACK message")
			slog.ErrorContext(ctx, "failed to ACK message", "error", err)
		}
		return
	}

	if err != nil {
		slog.ErrorContext(ctx, "failed to get message from store", "error", err)
		if err = msg.TermWithReason("Failed to get message from store"); err != nil {
			span.SetStatus(codes.Error, "Failed to TERM message")
			slog.ErrorContext(ctx, "failed to TERM message", "error", err)
		}
		return
	}

	scheduledMsg := stored.ScheduledMessage

	if scheduledMsg.Rev != msgRev {
		slog.InfoContext(ctx, "message rev mismatch, ignored", "name", scheduledMsg.Name, "storedRev", scheduledMsg.Rev, "msgRev", msgRev)

		if err = msg.Ack(); err != nil {
			span.SetStatus(codes.Error, "Failed to ACK message")
			slog.ErrorContext(ctx, "failed to ACK message", "error", err)
		}
		return
	}

	s.correctStateForDowntime(&stored.State, scheduledMsg.RepeatPolicy)
	delay := s.newSchedule(&stored.State)

	// if the message is embargoed, delay it
	if delay > 0 {
		slog.InfoContext(ctx, "message is embargoed", "delay", delay)

		if err = msg.NakWithDelay(delay); err != nil {
			span.SetStatus(codes.Error, "Failed to NAK message with delay")
			slog.ErrorContext(ctx, "failed to NAK message with delay", "error", err)
		}
		return
	}

	// The message is ready to be sent, so we ACK and forward it
	slog.DebugContext(ctx, "forwarding message", "name", scheduledMsg.Name, "rev", scheduledMsg.Rev)
	if err = msg.DoubleAck(ctx); err != nil {
		span.SetStatus(codes.Error, "Failed to double ACK message")
		slog.ErrorContext(ctx, "failed to double ACK message", "error", err)
		return
	}

	if err = s.forward(ctx, scheduledMsg.Subject, scheduledMsg.Payload); err != nil {
		span.SetStatus(codes.Error, "Failed to publish message")
		slog.ErrorContext(ctx, "failed to publish message", "error", err)
		return
	}

	// If there is no repeat policy, or this was the last repeat count, purge the message
	// Note: a negative Times means the message is sent indefinitely
	if scheduledMsg.RepeatPolicy == nil || stored.State.Times == 0 {
		slog.InfoContext(ctx, "schedule completed, uninstalling", "name", scheduledMsg.Name)

		if err = s.installer.Uninstall(ctx, scheduledMsg.Name); err != nil {
			slog.ErrorContext(ctx, "failed to uninstall message", "error", err)
		}
		return
	}

	err = s.reschedule(ctx, scheduledMsg, stored.State, rev)
	if err != nil {
		span.SetStatus(codes.Error, "Failed to reschedule message")
		slog.ErrorContext(ctx, "failed to reschedule message, uninstalling", "error", err)

		// if the message could not be rescheduled, uninstall it
		if err = s.installer.Uninstall(ctx, scheduledMsg.Name); err != nil {
			slog.ErrorContext(ctx, "failed to uninstall message", "error", err)
		}
		return
	}
}

func (s *Scheduler) reschedule(ctx context.Context, scheduledMsg ScheduledMessage, state State, rev uint64) error {
	ctx, span := s.tracer.Start(ctx, "scheduler.reschedule")
	defer span.End()

	// calculate the next time the message should be sent
	// use the current state time as the base in case of network latency
	state.At = state.At.Add(scheduledMsg.RepeatPolicy.Interval)

	// if Times started positive, decrement it
	// but keep it above 0 since negative means infinite
	if state.Times > 0 {
		state.Times--
	}

	_, err := s.installer.Update(ctx, scheduledMsg.Name, rev, &scheduledMsg, &state)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "message rescheduled", "name", scheduledMsg.Name, "at", state.At, "times", state.Times)

	err = s.publishToScheduleStream(ctx, &scheduledMsg)
	return err
}

func (s *Scheduler) forward(ctx context.Context, subject string, payload []byte) error {
	ctx, span := s.tracer.Start(ctx, "scheduler.forward")
	defer span.End()

	err := s.nc.PublishMsg(&nats.Msg{
		Subject: subject,
		Data:    payload,
		Header:  tracemsg.NewHeader(ctx),
	})
	if err != nil {
		return err
	}

	// TODO support forward on JetStream
	// _, err = s.js.PublishMsg(ctx, &nats.Msg{
	// 	Subject: subject,
	// 	Data:    payload,
	// 	Header:  tracemsg.NewHeader(ctx),
	// })
	// if err != nil {
	// 	return err
	// }

	return nil
}

func (s *Scheduler) correctStateForDowntime(state *State, policy *RepeatPolicy) {
	// If there are no schedulers available to handle embargoed message, the state.At will become outdated
	// This means, the At time could have passed

	// If the At time has passed, there is a repeat policy, and we're outside the threshold,
	// we calculate a new delay based of the startTime
	if policy == nil {
		return
	}

	threshold := -1 * time.Minute
	if time.Now().Add(threshold).Before(state.At) {
		return
	}

	// Calculate the number of intervals that have passed
	intervals := s.startTime.Sub(state.At) / policy.Interval

	// Calculate the new At based on the number of intervals that have passed
	state.At = state.At.Add(policy.Interval*intervals + policy.Interval)
}

func (s *Scheduler) newSchedule(state *State) time.Duration {
	if time.Now().Before(state.At) {
		delay := state.At.Sub(time.Now())
		return delay
	}
	return 0
}