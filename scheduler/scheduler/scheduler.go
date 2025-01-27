package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler/internal/observability"
	"github.com/3lvia/ice-scheduler/scheduler/internal/tracemsg"
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

type Scheduler struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	consumer  jetstream.Consumer
	tracer    trace.Tracer
	store     *Store
	installer *Installer
}

func New(ctx context.Context, nc *nats.Conn) (*Scheduler, error) {
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

	store, err := NewStore(ctx, js)
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		nc:        nc,
		js:        js,
		consumer:  consumer,
		tracer:    otel.Tracer(observability.TraceServiceName),
		store:     store,
		installer: NewInstaller(store),
	}, nil
}

func (s *Scheduler) install(ctx context.Context, msg *nats.Msg) error {
	var scheduledMsg ScheduledMessage
	err := json.Unmarshal(msg.Data, &scheduledMsg)
	if err != nil {
		return errors.Join(errors.New("failed to unmarshal message"), err)
	}

	return s.installSchedule(ctx, &scheduledMsg)
}

func (s *Scheduler) installSchedule(ctx context.Context, scheduledMsg *ScheduledMessage) error {
	err := s.installer.Install(ctx, scheduledMsg)
	if err != nil {
		return errors.Join(errors.New("failed to install message"), err)
	}

	slog.InfoContext(ctx, "message installed", "name", scheduledMsg.Name, "rev", scheduledMsg.Rev)

	_, err = s.js.PublishMsg(ctx, &nats.Msg{
		Subject: "scheduled." + scheduledMsg.Name,
	})
	if err != nil {
		if err = s.store.Purge(ctx, scheduledMsg.Name); err != nil {
			return errors.Join(errors.New("failed to publish message"), errors.New("failed to purge message from store"))
		}
		return errors.New("failed to publish message")
	}
	return nil
}

func (s *Scheduler) uninstall(ctx context.Context, msg *nats.Msg) error {
	// remove the "scheduler.uninstall." prefix from the subject
	name := msg.Subject[18:]

	err := s.installer.Uninstall(ctx, name)
	if err != nil {
		return err
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

		if err = msg.Nak(); err != nil {
			span.SetStatus(codes.Error, "Failed to NAK message for missing metadata")
			slog.ErrorContext(ctx, "failed to NAK message for missing metadata", "error", err)
		}
		return
	}
	slog.DebugContext(ctx, "received a JetStream message", "seq", meta.Sequence.Stream)

	// remove the "scheduled." prefix from the name
	name := msg.Subject()[10:]

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

	delay := s.newSchedule(scheduledMsg)

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
		if err = msg.Nak(); err != nil {
			span.SetStatus(codes.Error, "Failed to NAK message")
			slog.ErrorContext(ctx, "failed to NAK message", "error", err)
		}
		return
	}

	// If there is no repeat policy, or this was the last repeat count, purge the message
	// Note: a negative Times means the message is sent indefinitely
	if scheduledMsg.RepeatPolicy == nil || scheduledMsg.RepeatPolicy.Times == 0 {
		if err = s.store.Purge(ctx, scheduledMsg.Name); err != nil {
			slog.ErrorContext(ctx, "failed to purge message from store", "error", err)
		}
		slog.InfoContext(ctx, "message purged", "name", scheduledMsg.Name)
		return
	}

	rescheduledMsg, changed, err := s.reschedule(ctx, scheduledMsg, msg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to reschedule message", "error", err)
		return
	}

	if changed {
		_, err = s.store.UpdateWithFingerprint(ctx, rescheduledMsg.Name, &rescheduledMsg, rev)
		if err != nil {
			slog.ErrorContext(ctx, "failed to reschedule message", "error", err)
			if err = msg.TermWithReason("Failed to reschedule message"); err != nil {
				span.SetStatus(codes.Error, "Failed to NAK message")
				slog.ErrorContext(ctx, "failed to NAK message", "error", err)
			}

			// if the message could not be rescheduled, purge it
			if err = s.store.Purge(ctx, rescheduledMsg.Name); err != nil {
				slog.ErrorContext(ctx, "failed to purge message from store", "error", err)
			}
		}
	}

	slog.InfoContext(ctx, "message rescheduled", "name", rescheduledMsg.Name, "at", rescheduledMsg.At, "repeat", rescheduledMsg.RepeatPolicy)
}

func (s *Scheduler) reschedule(ctx context.Context, scheduledMsg ScheduledMessage, msg jetstream.Msg) (ScheduledMessage, bool, error) {
	ctx, span := s.tracer.Start(ctx, "scheduler.reschedule")
	defer span.End()

	policy := scheduledMsg.RepeatPolicy

	changed := false
	if policy.Times > 0 {
		policy.Times--
		changed = true
	}

	// calculate the next time the message should be sent
	// use the current At time as the base in case of network latency
	scheduledMsg.At = scheduledMsg.At.Add(policy.Interval)

	scheduledMsgData, err := json.Marshal(scheduledMsg)
	if err != nil {
		return scheduledMsg, changed, err
	}

	_, err = s.js.PublishMsg(ctx, &nats.Msg{
		Subject: msg.Subject(),
		Data:    scheduledMsgData,
	})

	return scheduledMsg, changed, err
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

func (s *Scheduler) newSchedule(msg ScheduledMessage) time.Duration {
	if time.Now().Before(msg.At) {
		delay := msg.At.Sub(time.Now())
		return delay
	}

	return 0
}