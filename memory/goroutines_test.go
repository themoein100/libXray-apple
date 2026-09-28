package memory

import (
	"strings"
	"testing"
	"time"
)

func TestGoroutineSummary(t *testing.T) {
	stop := make(chan struct{})
	for i := 0; i < 5; i++ {
		go parkForTest(stop)
	}
	time.Sleep(10 * time.Millisecond)
	s := GoroutineSummary(5)
	close(stop)
	if !strings.HasPrefix(s, "total=") || !strings.Contains(s, "5×") {
		t.Fatal(s)
	}
	t.Log(s)
}

func parkForTest(stop chan struct{}) { <-stop }
