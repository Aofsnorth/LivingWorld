package harness

import "testing"

func TestHealthReportAggregatesWorst(t *testing.T) {
	r := newHealthReporter()
	r.Register("disk", func(Runtime) Health { return Health{Status: HealthUp} })
	r.Register("db", func(Runtime) Health { return Health{Status: HealthDegraded, Message: "slow"} })
	comps := []Component{
		&fakeComponent{key: "svc", health: Health{Status: HealthUp}},
		&fakeComponent{key: "queue", health: Health{Status: HealthDown, Message: "drained"}},
	}
	report := r.Report(nilRuntime{}, comps)
	if report.Status != HealthDown {
		t.Fatalf("expected down (worst), got %s", report.Status)
	}
	if report.Components["queue"].Status != HealthDown {
		t.Fatal("queue probe missing")
	}
	if report.Checks["db"].Status != HealthDegraded {
		t.Fatal("db check missing")
	}
}

func TestHealthReportRecoversPanic(t *testing.T) {
	r := newHealthReporter()
	r.Register("bad", func(Runtime) Health { panic("nope") })
	report := r.Report(nilRuntime{}, nil)
	if report.Checks["bad"].Status != HealthDown {
		t.Fatalf("expected panicking probe reported as down, got %s", report.Checks["bad"].Status)
	}
	if report.Status != HealthDown {
		t.Fatalf("aggregate should be down, got %s", report.Status)
	}
}
