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
	if len(received) != 3 {
		t.Fatalf("received %v, want every configured model", received)
	}
	if received[0] != "primary" || received[1] != "fallback" || received[2] != "never" {
		t.Fatalf("probe order = %v, want [primary fallback never]", received)
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

func TestShouldSyncUpstreamAccount(t *testing.T) {
	tests := []struct {
		name        string
		integration string
		hasSecret   bool
		want        bool
	}{
		{name: "newapi with secret", integration: relay.UpstreamIntegrationNewAPI, hasSecret: true, want: true},
		{name: "sub2api with secret", integration: " SUB2API ", hasSecret: true, want: true},
		{name: "official with secret", integration: relay.UpstreamIntegrationOfficial, hasSecret: true, want: false},
		{name: "other with secret", integration: relay.UpstreamIntegrationOther, hasSecret: true, want: false},
		{name: "newapi without secret", integration: relay.UpstreamIntegrationNewAPI, hasSecret: false, want: false},
		{name: "unknown with secret", integration: "custom", hasSecret: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := relay.ChannelSummary{
				UpstreamIntegration:          test.integration,
				HasUpstreamAccountCredential: test.hasSecret,
			}
			if got := shouldSyncUpstreamAccount(channel); got != test.want {
				t.Fatalf("shouldSyncUpstreamAccount() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRunUpstreamAccountSyncOnceOnlySyncsSupportedConfiguredChannels(t *testing.T) {
	service := &upstreamAccountSyncServiceFake{
		channels: []relay.ChannelSummary{
			{ID: "newapi", UpstreamIntegration: relay.UpstreamIntegrationNewAPI, HasUpstreamAccountCredential: true},
			{ID: "sub2api", UpstreamIntegration: relay.UpstreamIntegrationSub2API, HasUpstreamAccountCredential: true},
			{ID: "official", UpstreamIntegration: relay.UpstreamIntegrationOfficial, HasUpstreamAccountCredential: true},
			{ID: "other", UpstreamIntegration: relay.UpstreamIntegrationOther, HasUpstreamAccountCredential: true},
			{ID: "missing-secret", UpstreamIntegration: relay.UpstreamIntegrationNewAPI},
		},
	}

	runUpstreamAccountSyncOnce(context.Background(), service)

	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.syncedIDs) != 2 {
		t.Fatalf("synced IDs = %v, want two supported configured channels", service.syncedIDs)
	}
	synced := map[string]bool{}
	for _, id := range service.syncedIDs {
		synced[id] = true
	}
	for _, id := range []string{"newapi", "sub2api"} {
		if !synced[id] {
			t.Fatalf("synced IDs = %v, missing %q", service.syncedIDs, id)
		}
	}
}

type modelMonitorProbeFunc func(context.Context, string, string) error

func (f modelMonitorProbeFunc) ProbeModel(ctx context.Context, groupID, model string) error {
	return f(ctx, groupID, model)
}

type upstreamAccountSyncServiceFake struct {
	mu        sync.Mutex
	channels  []relay.ChannelSummary
	syncedIDs []string
}

func (s *upstreamAccountSyncServiceFake) ListChannels(context.Context) ([]relay.ChannelSummary, error) {
	return append([]relay.ChannelSummary(nil), s.channels...), nil
}

func (s *upstreamAccountSyncServiceFake) SyncChannelAccount(_ context.Context, _, channelID string) (relay.ChannelSummary, error) {
	s.mu.Lock()
	s.syncedIDs = append(s.syncedIDs, channelID)
	s.mu.Unlock()
	return relay.ChannelSummary{ID: channelID}, nil
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
