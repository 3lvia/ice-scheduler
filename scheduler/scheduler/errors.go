package scheduler

import "errors"

var (
	ErrFailedToUnmarshal = errors.New("failed to unmarshal message")
	ErrFailedToMarshal   = errors.New("failed to marshal message")
	ErrFailedToInstall   = errors.New("failed to install schedule")
	ErrFailedToUninstall = errors.New("failed to uninstall schedule")
	ErrFailedToPublish   = errors.New("failed to publish message")

	ErrKeyNotFound         = errors.New("key not found")
	ErrFailedToFingerprint = errors.New("failed to fingerprint message")

	ErrInvalidName     = errors.New("the name can only be alphanumeric, underscore or hyphen")
	ErrInvalidSubject  = errors.New("the subject must follow NATS subject rules")
	ErrInvalidInterval = errors.New("the repeat interval is too short")

	ErrRevisionConflict    = errors.New("revision conflict")
	ErrFingerprintConflict = errors.New("fingerprint conflict")

	ErrFailedToGetMessage    = errors.New("failed to get message from store")
	ErrFailedToPutMessage    = errors.New("failed to put message in store")
	ErrFailedToUpdateMessage = errors.New("failed to update message in store")
	ErrFailedToPurgeMessage  = errors.New("failed to purge message from store")
)