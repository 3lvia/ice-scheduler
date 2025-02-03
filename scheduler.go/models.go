package scheduler

import "time"

// Infinite is a constant to indicate that the message should be sent indefinitely.
const Infinite = -1

// RepeatPolicy is the policy for repeating messages.
type RepeatPolicy struct {
	// Interval is the time between each message.
	Interval time.Duration `json:"interval"`

	// Times is the number of times the message should be sent.
	// If Times is negative, the message is sent indefinitely.
	Times int `json:"times"`
}

// ScheduledMessage is the data transfer object for the scheduled messages.
type ScheduledMessage struct {
	// Name identifies the message.
	// If there already exists a message with the same name, it is replaced.
	Name string `json:"name"`

	// Rev is the revision of the message.
	Rev uint32 `json:"rev"`

	// Subject is the subject of the message.
	Subject string `json:"subject"`

	// At the time which the message should be sent.
	// When the time is in the past, the message is sent immediately.
	At time.Time `json:"at"`

	// RepeatPolicy is policy for which the message should be repeated.
	// If RepeatPolicy is nil, the message is sent once.
	// If RepeatPolicy is not nil, the message is sent according to the policy.
	RepeatPolicy *RepeatPolicy `json:"repeatPolicy"`

	// Payload is the message content.
	Payload []byte `json:"payload"`
}