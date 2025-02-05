package scheduler

import "time"

// InstallOpt is an option to install a scheduled message.
type InstallOpt func(*ScheduledMessage)

// WithRev sets the revision of the scheduled message.
// If the revision is not set, it is set to 0.
// To update a scheduled message, the revision must be set to a version higher than the current one.
func WithRev(rev uint32) InstallOpt {
	return func(s *ScheduledMessage) {
		s.Rev = rev
	}
}

// WithAt sets the time the scheduled message should be sent.
// If the time is not set, the message is sent immediately.
func WithAt(at time.Time) InstallOpt {
	return func(s *ScheduledMessage) {
		s.At = &at
	}
}

// WithPayload sets the payload of the scheduled message.
func WithPayload(payload []byte) InstallOpt {
	return func(s *ScheduledMessage) {
		s.Payload = payload
	}
}

// WithRepeatPolicy sets the repeat policy of the scheduled message.
// If the times is 0, the repeat policy is ignored.
func WithRepeatPolicy(interval time.Duration, times int) InstallOpt {
	return func(s *ScheduledMessage) {
		if times == 0 {
			s.RepeatPolicy = nil
		} else {
			s.RepeatPolicy = &RepeatPolicy{
				Interval: interval,
				Times:    times,
			}
		}
	}
}