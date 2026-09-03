package groups

import "testing"

func TestModelRouteStatus(t *testing.T) {
	tests := []struct {
		name                                   string
		groupStatus                            string
		totalRoutes, available, degraded, seen int
		want                                   string
	}{
		{name: "disabled group", groupStatus: StatusDisabled, totalRoutes: 1, available: 1, seen: 1, want: "disabled"},
		{name: "no routes", groupStatus: StatusActive, want: "unavailable"},
		{name: "all routes unavailable", groupStatus: StatusActive, totalRoutes: 2, available: 0, seen: 2, want: "unavailable"},
		{name: "partial routes", groupStatus: StatusActive, totalRoutes: 2, available: 1, seen: 2, want: "degraded"},
		{name: "degraded route", groupStatus: StatusActive, totalRoutes: 1, available: 1, degraded: 1, seen: 1, want: "degraded"},
		{name: "not observed", groupStatus: StatusActive, totalRoutes: 2, available: 2, seen: 1, want: "pending"},
		{name: "normal", groupStatus: StatusActive, totalRoutes: 2, available: 2, seen: 2, want: "normal"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if got := modelRouteStatus(item.groupStatus, item.totalRoutes, item.available, item.degraded, item.seen); got != item.want {
				t.Fatalf("model route status = %q, want %q", got, item.want)
			}
		})
	}
}

func TestGroupRouteStatus(t *testing.T) {
	models := []ModelStatus{{Status: "normal"}, {Status: "unavailable"}}
	if got := groupRouteStatus(StatusActive, models); got != "degraded" {
		t.Fatalf("mixed group status = %q, want degraded", got)
	}
	if got := groupRouteStatus(StatusActive, []ModelStatus{{Status: "pending"}}); got != "pending" {
		t.Fatalf("pending group status = %q, want pending", got)
	}
	if got := groupRouteStatus(StatusDisabled, models); got != "disabled" {
		t.Fatalf("disabled group status = %q, want disabled", got)
	}
	if got := groupRouteStatus(StatusActive, []ModelStatus{{Status: "degraded"}}); got != "degraded" {
		t.Fatalf("degraded group status = %q, want degraded", got)
	}
}

func TestModelMonitorRecentRequestLimits(t *testing.T) {
	for _, limit := range []int{30, 60, 120} {
		request := ModelMonitorMutation{
			GroupID:            testMonitorGroupID,
			Name:               "Request history",
			RecentRequestLimit: limit,
		}
		got, err := request.validate()
		if err != nil {
			t.Fatalf("limit %d returned error: %v", limit, err)
		}
		if got.RecentRequestLimit != limit {
			t.Fatalf("limit %d normalized to %d", limit, got.RecentRequestLimit)
		}
	}
}
