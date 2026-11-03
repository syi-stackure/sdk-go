package stackure

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

const maxEvents = 100

type orgClosedFunc func(ctx context.Context, accountID string) error

var (
	orgClosed atomic.Pointer[orgClosedFunc]
	draining  atomic.Bool
	unhandled atomic.Bool
)

type appEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		AccountID string `json:"account_id"`
	} `json:"data"`
}

type appEvents struct {
	Events []appEvent `json:"events"`
	More   bool       `json:"more"`
}

func OnOrgClosed(fn func(ctx context.Context, accountID string) error) {
	f := orgClosedFunc(fn)
	orgClosed.Store(&f)
	drainEvents()
}

func noteEvents(waiting int) {
	if waiting <= 0 {
		return
	}
	if orgClosed.Load() == nil {
		if unhandled.CompareAndSwap(false, true) {
			log.Printf("stackure: %s waiting for this app to delete their data. Register stackure.OnOrgClosed to delete it.", closedOrgs(waiting))
		}
		return
	}
	drainEvents()
}

func closedOrgs(n int) string {
	if n == 1 {
		return "1 closed organization is"
	}
	return fmt.Sprintf("%d closed organizations are", n)
}

func drainEvents() {
	if orgClosed.Load() == nil || !draining.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer draining.Store(false)
		if err := processEvents(context.Background(), *orgClosed.Load()); err != nil {
			log.Printf("stackure: closed organizations not deleted yet: %v", err)
		}
	}()
}

func processEvents(ctx context.Context, fn orgClosedFunc) error {
	var ack []string
	for {
		res, err := takeEvents(ctx, ack, maxEvents)
		if err != nil {
			return err
		}
		ack = nil
		for _, e := range res.Events {
			if e.Type != "org.closed" {
				continue
			}
			if err = runOrgClosed(ctx, fn, e.Data.AccountID); err != nil {
				break
			}
			ack = append(ack, e.ID)
		}
		if err == nil && res.More {
			continue
		}
		if len(ack) > 0 {
			if _, ackErr := takeEvents(ctx, ack, 0); err == nil {
				err = ackErr
			}
		}
		return err
	}
}

func runOrgClosed(ctx context.Context, fn orgClosedFunc, accountID string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("deleting organization %s panicked: %v", accountID, p)
		}
	}()
	if err = fn(ctx, accountID); err != nil {
		err = fmt.Errorf("deleting organization %s: %w", accountID, err)
	}
	return err
}

func takeEvents(ctx context.Context, ack []string, max int) (*appEvents, error) {
	id, err := appID()
	if err != nil {
		return nil, err
	}
	st, b, err := request(ctx, http.MethodPost, "/api/public/events", callOpts{body: map[string]any{"app_id": id, "ack": ack, "max_events": max}})
	if err != nil {
		return nil, err
	}
	out := &appEvents{}
	if err := handleResponse(st, b, out); err != nil {
		return nil, err
	}
	return out, nil
}
