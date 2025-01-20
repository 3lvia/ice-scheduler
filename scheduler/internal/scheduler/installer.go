package scheduler

import (
	"context"
	"errors"
	"regexp"
	"time"

	"elvia.io/scheduler/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Installer struct {
	store  *Store
	tracer trace.Tracer
}

func NewInstaller(store *Store) *Installer {
	return &Installer{
		store:  store,
		tracer: otel.Tracer(observability.TraceServiceName),
	}
}

func (i *Installer) Install(ctx context.Context, message *ScheduledMessage) error {
	ctx, span := i.tracer.Start(ctx, "scheduler.installer.Install")
	defer span.End()

	if message.Name == "" || !nameRegex.MatchString(message.Name) {
		return ErrInvalidName
	}

	if message.Subject == "" {
		return ErrInvalidSubject
	}

	if message.RepeatPolicy != nil {
		if message.RepeatPolicy.Interval <= 5*time.Second {
			return ErrInvalidInterval
		}
	}

	stored, rev, err := i.store.Get(ctx, message.Name)
	if err != nil && !errors.Is(err, ErrKeyNotFound) {
		return ErrFailedToGetMessage
	}

	// install new message if it does not exist
	if stored == nil {
		err = i.store.PutWithFingerprint(ctx, message.Name, message)
		if err != nil {
			return ErrFailedToPutMessage
		}
		return nil
	}

	// if stored rev is greater than the rev in th message, return an error
	if stored.ScheduledMessage.Rev > message.Rev {
		return ErrRevisionConflict
	}

	newFingerprint, err := i.store.Fingerprint(message)
	if err != nil {
		return err
	}

	// if stored rev is equal to the rev in the message and the fingerprint are different, return an error
	if stored.ScheduledMessage.Rev == message.Rev && stored.Fingerprint != newFingerprint {
		return ErrFingerprintConflict
	}

	// update the message in the KV store
	_, err = i.store.Update(ctx, message.Name, &StoredMessage{
		Fingerprint:      newFingerprint,
		ScheduledMessage: *message,
	}, rev)
	if err != nil {
		return ErrFailedToUpdateMessage
	}

	return nil
}

func (i *Installer) Uninstall(ctx context.Context, message *ScheduledMessage) error {
	ctx, span := i.tracer.Start(ctx, "scheduler.installer.uninstall")
	defer span.End()

	stored, _, err := i.store.Get(ctx, message.Name)
	if errors.Is(err, ErrKeyNotFound) {
		return nil
	}

	if err != nil {
		return ErrFailedToGetMessage
	}

	if stored == nil {
		return nil
	}

	err = i.store.Purge(ctx, message.Name)
	if err != nil {
		return ErrFailedToPurgeMessage
	}

	return nil
}