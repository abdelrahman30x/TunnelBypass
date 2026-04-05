package health

import "testing"

func TestPingStatus(t *testing.T) {
	t.Parallel()
	if pingStatus(100, nil, 200, nil) != "HEALTHY" {
		t.Fatal("expected HEALTHY")
	}
	if pingStatus(600, nil, 100, nil) != "DEGRADED" {
		t.Fatal("expected DEGRADED for high tcp")
	}
	if pingStatus(2500, nil, 100, nil) != "UNHEALTHY" {
		t.Fatal("expected UNHEALTHY")
	}
	if pingStatus(100, nil, 2000, nil) != "DEGRADED" {
		t.Fatal("expected DEGRADED for high http")
	}
}
