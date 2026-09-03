package groups

import "testing"

const testMonitorGroupID = "11111111-1111-4111-8111-111111111111"

func TestModelMonitorMutationValidation(t *testing.T) {
	tests := []struct {
		name    string
		request ModelMonitorMutation
		wantErr bool
		assert  func(*testing.T, ModelMonitorMutation)
	}{
		{
			name: "defaults to all passive monitoring",
			request: ModelMonitorMutation{
				GroupID: testMonitorGroupID,
				Name:    " Primary monitor ",
			},
			assert: func(t *testing.T, got ModelMonitorMutation) {
				if got.Name != "Primary monitor" {
					t.Fatalf("name = %q", got.Name)
				}
				if got.SelectionMode != MonitorSelectionAll || got.Mode != MonitorModePassive {
					t.Fatalf("unexpected defaults: %#v", got)
				}
				if got.ProbeIntervalSeconds != 300 || got.RecentRequestLimit != 60 || len(got.ModelNames) != 0 {
					t.Fatalf("unexpected normalized all-model monitor: %#v", got)
				}
			},
		},
		{
			name: "selected models are normalized and deduplicated",
			request: ModelMonitorMutation{
				GroupID:              testMonitorGroupID,
				Name:                 "Selected",
				SelectionMode:        MonitorSelectionSelected,
				Mode:                 MonitorModeActive,
				ModelNames:           []string{" gpt-5 ", "gpt-5", "claude-sonnet"},
				PrimaryModel:         " claude-sonnet ",
				ProbeIntervalSeconds: 60,
				RecentRequestLimit:   120,
			},
			assert: func(t *testing.T, got ModelMonitorMutation) {
				if len(got.ModelNames) != 2 || got.ModelNames[0] != "gpt-5" || got.ModelNames[1] != "claude-sonnet" {
					t.Fatalf("model names = %#v", got.ModelNames)
				}
				if got.PrimaryModel != "claude-sonnet" {
					t.Fatalf("primary model = %q", got.PrimaryModel)
				}
				if got.RecentRequestLimit != 120 {
					t.Fatalf("recent request limit = %d", got.RecentRequestLimit)
				}
			},
		},
		{
			name: "selected primary model must be monitored",
			request: ModelMonitorMutation{
				GroupID:       testMonitorGroupID,
				Name:          "Invalid primary",
				SelectionMode: MonitorSelectionSelected,
				ModelNames:    []string{"gpt-5"},
				PrimaryModel:  "claude-sonnet",
			},
			wantErr: true,
		},
		{
			name: "selected mode requires a model",
			request: ModelMonitorMutation{
				GroupID:       testMonitorGroupID,
				Name:          "Missing models",
				SelectionMode: MonitorSelectionSelected,
			},
			wantErr: true,
		},
		{
			name: "probe interval has a lower bound",
			request: ModelMonitorMutation{
				GroupID:              testMonitorGroupID,
				Name:                 "Too fast",
				Mode:                 MonitorModeActive,
				ProbeIntervalSeconds: 59,
			},
			wantErr: true,
		},
		{
			name: "recent request limit must be supported",
			request: ModelMonitorMutation{
				GroupID:            testMonitorGroupID,
				Name:               "Unsupported history",
				RecentRequestLimit: 90,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.request.validate()
			if test.wantErr {
				if err == nil {
					t.Fatal("validate() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() error = %v", err)
			}
			if test.assert != nil {
				test.assert(t, got)
			}
		})
	}
}
