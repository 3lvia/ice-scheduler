# About

GO client library for [ICE Scheduler](https://github.com/3lvia/ice-scheduler).

## Installation

```bash
go get github.com/3lvia/ice-scheduler/scheduler.go
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/3lvia/ice-scheduler/scheduler.go"
	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		panic(err)
	}
	defer nc.Close()

	ctx := context.Background()

	sched := scheduler.New(nc)

	err = sched.Install(ctx, "my-job", "my-subject", time.Now().Add(time.Minute), scheduler.WithPayload([]byte("hello")))
	if err != nil {
		slog.Error("failed to install job", "error", err)
		panic(err)
	}

	slog.Info("job installed")

	wg := sync.WaitGroup{}
	sub, err := nc.Subscribe("my-subject", func(msg *nats.Msg) {
		fmt.Println("received message:", string(msg.Data))
		wg.Done()
	})
	if err != nil {
		slog.Error("failed to subscribe", "error", err)
		panic(err)
	}
	defer sub.Unsubscribe()

	slog.Info("subscribed to my-subject")

	wg.Add(1)
	wg.Wait()

	slog.Info("job executed")
}
```