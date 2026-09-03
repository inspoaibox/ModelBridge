package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ai-token/internal/groups"
	"ai-token/internal/relay"
)

func TestProbeModelMonitorUsesPrimaryThenFallback(t *testing.T) {
	var (
		mu       sync.Mutex
		received []string
	)
	prober := modelMonitorProbeFunc(func(ctx context.Context, groupID, model string) error {
		mu.Lock()
		received = append(received, model)
		mu.Unlock()
		if model == "primary" {
			return errors.New("primary quota exhausted")
		}
		return nil
	})
	monitor := &groups.ModelMonitor{
		GroupID:      "group-1",
		PrimaryModel: "primary",
		ModelNames:   []string{"primary", "fallback", "never"},
	}

	ctx := context.Background()
	status, probeError := probeModelMonitor(ctx, prober, monitor)

	if status != groups.MonitorProbeSuccess {
		t.Fatalf("status = %q, want %q", status, groups.MonitorProbeSuccess)
	}
	if probeError != "" {
		t.Fatalf("probeError = %q, want empty after fallback success", probeError)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("received %v, want primary and fallback only", received)
	}
	if received[0] != "primary" || received[1] != "fallback" {
		t.Fatalf("probe order = %v, want [primary fallback]", received)
	}
}

func TestRunModelMonitorProberDoesNotBlockOtherGroupsBehindSlowGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitors := &modelMonitorServiceFake{
		items: []*groups.ModelMonitor{
			{ID: "slow", GroupID: "group-slow", ModelNames: []string{"slow"}},
			{ID: "fast", GroupID: "group-fast", ModelNames: []string{"fast"}},
		},
	}
	fastFinished := make(chan struct{})
	releaseSlow := make(chan struct{})
	prober := modelMonitorProbeFunc(func(ctx context.Context, _, model string) error {
		if model == "slow" {
			select {
			case <-releaseSlow:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		close(fastFinished)
		return nil
	})

	done := make(chan struct{})
	go func() {
		runModelMonitorProber(ctx, monitors, prober)
		close(done)
	}()

	select {
	case <-fastFinished:
	case <-time.After(time.Second):
		t.Fatal("fast group was blocked behind slow group")
	}
	close(releaseSlow)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("model monitor prober did not stop")
	}
	if len(monitors.completedIDs()) != 2 {
		t.Fatalf("completed IDs = %v, want both groups", monitors.completedIDs())
	}
}

type modelMonitorProbeFunc func(context.Context, string, string) error

func (f modelMonitorProbeFunc) ProbeModel(ctx context.Context, groupID, model string) error {
	return f(ctx, groupID, model)
}

type modelMonitorServiceFake struct {
	mu        sync.Mutex
	items     []*groups.ModelMonitor
	completed []string
}

func (s *modelMonitorServiceFake) ListAdminModelMonitors(context.Context) ([]groups.ModelMonitor, error) {
	return nil, errors.New("not used")
}

func (s *modelMonitorServiceFake) CreateAdminModelMonitor(context.Context, string, groups.ModelMonitorMutation) (groups.ModelMonitor, error) {
	return groups.ModelMonitor{}, errors.New("not used")
}

func (s *modelMonitorServiceFake) UpdateAdminModelMonitor(context.Context, string, string, groups.ModelMonitorMutation) (groups.ModelMonitor, error) {
	return groups.ModelMonitor{}, errors.New("not used")
}

func (s *modelMonitorServiceFake) DeleteAdminModelMonitor(context.Context, string, string) error {
	return errors.New("not used")
}

func (s *modelMonitorServiceFake) ClaimDueActiveModelMonitor(context.Context) (*groups.ModelMonitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return nil, nil
	}
	item := s.items[0]
	s.items = s.items[1:]
	return item, nil
}

func (s *modelMonitorServiceFake) ClaimActiveModelMonitor(context.Context, string) (*groups.ModelMonitor, error) {
	return nil, errors.New("not used")
}

func (s *modelMonitorServiceFake) CompleteActiveModelMonitor(_ context.Context, monitorID, _, _ string) error {
	s.mu.Lock()
	s.completed = append(s.completed, monitorID)
	s.mu.Unlock()
	return nil
}

func (s *modelMonitorServiceFake) completedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.completed...)
}

var _ groups.ModelMonitorService = (*modelMonitorServiceFake)(nil)
var _ relay.ModelProbeService = modelMonitorProbeFunc(nil)
