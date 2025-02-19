package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Installer struct {
	store  Store
	tracer trace.Tracer

	minInterval time.Duration
}

func NewInstaller(store Store, minInterval time.Duration) *Installer {
	return &Installer{
		store:       store,
		tracer:      otel.Tracer(config.TracerName),
		minInterval: minInterval,
	}
}

func (i *Installer) Validate(message *ScheduledMessage) error {
	if message.Name == "" || !nameRegex.MatchString(message.Name) {
		return ErrInvalidName
	}

	if message.Subject == "" {
		return ErrInvalidSubject
	}

	if message.RepeatPolicy != nil {
		if message.RepeatPolicy.Interval < i.minInterval {
			return ErrInvalidInterval
		}
	}

	return nil
}

func (i *Installer) Install(ctx context.Context, message *ScheduledMessage) (bool, error) {
	ctx, span := i.tracer.Start(ctx, "installer.install")
	defer span.End()

	span.SetAttributes(
		attribute.String("name", message.Name),
		attribute.String("rev", fmt.Sprintf("%d", message.Rev)),
	)

	err := i.Validate(message)
	if err != nil {
		return false, err
	}

	stored, rev, err := i.store.Get(ctx, message.Name)
	if err != nil && !errors.Is(err, ErrKeyNotFound) {
		return false, ErrFailedToGetMessage
	}

	// install new message if it does not exist
	if stored == nil {
		state := NewState(*message)
		fp, err := fingerprint(message)
		if err != nil {
			return false, err
		}

		err = i.store.Put(ctx, message.Name, &StoredMessage{
			Fingerprint:      fp,
			ScheduledMessage: *message,
			State:            state,
		})
		if err != nil {
			return false, ErrFailedToPutMessage
		}
		return true, nil
	}

	// if stored rev is greater than the rev in th message, return an error
	if stored.ScheduledMessage.Rev > message.Rev {
		return false, ErrRevisionConflict
	}

	newFingerprint, err := fingerprint(message)
	if err != nil {
		return false, err
	}

	// if stored rev is equal to the rev in the message and the fingerprint are different, return an error
	if stored.ScheduledMessage.Rev == message.Rev && stored.Fingerprint != newFingerprint {
		return false, ErrFingerprintConflict
	}

	// if stored rev is equal to the rev in the message and the fingerprint are the same, return nil
	if stored.ScheduledMessage.Rev == message.Rev {
		return false, nil
	}

	// update the message in the KV store
	_, err = i.store.Update(ctx, message.Name, &StoredMessage{
		Fingerprint:      newFingerprint,
		ScheduledMessage: *message,
		State:            NewState(*message),
	}, rev)
	if err != nil {
		return false, ErrFailedToUpdateMessage
	}

	return true, nil
}

func (i *Installer) Update(ctx context.Context, name string, rev uint64, message *ScheduledMessage, state *State) (uint64, error) {
	ctx, span := i.tracer.Start(ctx, "installer.update")
	defer span.End()

	span.SetAttributes(
		attribute.String("name", name),
		attribute.String("rev", fmt.Sprintf("%d", rev)),
	)

	err := i.Validate(message)
	if err != nil {
		return 0, err
	}

	fp, err := fingerprint(message)
	if err != nil {
		return 0, err
	}

	rev, err = i.store.Update(ctx, name, &StoredMessage{
		Fingerprint:      fp,
		ScheduledMessage: *message,
		State:            *state,
	}, rev)

	return rev, err
}

func (i *Installer) Uninstall(ctx context.Context, name string) error {
	ctx, span := i.tracer.Start(ctx, "installer.uninstall")
	defer span.End()

	span.SetAttributes(attribute.String("name", name))

	stored, _, err := i.store.Get(ctx, name)
	if errors.Is(err, ErrKeyNotFound) {
		return nil
	}

	if err != nil {
		return ErrFailedToGetMessage
	}

	if stored == nil {
		return nil
	}

	err = i.store.Purge(ctx, name)
	if err != nil {
		return ErrFailedToPurgeMessage
	}

	return nil
}

func fingerprint(msg *ScheduledMessage) ([32]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return [32]byte{}, errors.Join(ErrFailedToFingerprint, err)
	}
	return sha256.Sum256(data), nil
}