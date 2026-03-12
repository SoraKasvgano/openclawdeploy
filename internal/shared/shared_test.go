package shared

import "testing"

func TestNormalizeDeviceID(t *testing.T) {
	value := "likeqi|00:15:5d:a3:46:da|2026-03-11 17:07:15|100.64.0.3"
	want := "likeqi00155da346da202603111707151006403"

	if got := NormalizeDeviceID(value); got != want {
		t.Fatalf("unexpected normalized device id: %s", got)
	}
}
