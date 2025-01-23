package scheduler

import "errors"

var (
	ErrKeyNotFound         = errors.New("key not found")
	ErrFailedToFingerprint = errors.New("failed to fingerprint message")

	ErrInvalidName     = errors.New("invalid name")
	ErrInvalidSubject  = errors.New("invalid subject")
	ErrInvalidInterval = errors.New("invalid interval")

	ErrRevisionConflict    = errors.New("revision conflict")
	ErrFingerprintConflict = errors.New("fingerprint conflict")

	ErrFailedToGetMessage    = errors.New("failed to get message from store")
	ErrFailedToPutMessage    = errors.New("failed to put message in store")
	ErrFailedToUpdateMessage = errors.New("failed to update message in store")
	ErrFailedToPurgeMessage  = errors.New("failed to purge message from store")
)