package jobstatus

import "testing"

func TestCanonicalLifecycle(t *testing.T) {
	if !IsClosed(Completed) || !IsClosed(Cancelled) {
		t.Fatal("terminal statuses must be closed")
	}
	if IsClosed(Planning) || IsClosed(Confirmed) {
		t.Fatal("open statuses must not be closed")
	}
	if !IsDispatchable(Confirmed) || IsDispatchable(Planning) {
		t.Fatal("only confirmed jobs may be dispatched")
	}
}
