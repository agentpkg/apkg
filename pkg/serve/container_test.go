package serve

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestContainerName(t *testing.T) {
	tests := map[string]struct {
		name string
		want string
	}{
		"simple name": {
			name: "postgres",
			want: "apkg-postgres",
		},
		"hyphenated name": {
			name: "my-server",
			want: "apkg-my-server",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mc := &managedContainer{name: tc.name}
			got := mc.containerName()
			if got != tc.want {
				t.Errorf("containerName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFreePort(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort() error: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("freePort() = %d, want valid port number", port)
	}

	// The port should be available for binding.
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Errorf("port %d not bindable: %v", port, err)
	} else {
		l.Close()
	}
}

func TestManagedContainerInitialStatus(t *testing.T) {
	mc := &managedContainer{
		name:          "test",
		image:         "test:latest",
		containerPort: 8080,
	}
	if mc.status != statusStopped {
		t.Errorf("initial status = %d, want %d (statusStopped)", mc.status, statusStopped)
	}
	if mc.hostPort != 0 {
		t.Errorf("initial hostPort = %d, want 0", mc.hostPort)
	}
	if mc.proxy != nil {
		t.Error("initial proxy should be nil")
	}
}

func TestTouch(t *testing.T) {
	mc := &managedContainer{name: "test"}

	before := time.Now()
	mc.touch()
	after := time.Now()

	mc.mu.Lock()
	lastUsed := mc.lastUsed
	mc.mu.Unlock()

	if lastUsed.Before(before) || lastUsed.After(after) {
		t.Errorf("lastUsed = %v, want between %v and %v", lastUsed, before, after)
	}
}

func TestStopIfIdleNotRunning(t *testing.T) {
	mc := &managedContainer{
		name:   "test",
		status: statusStopped,
	}
	// Should return false since container isn't running.
	if mc.stopIfIdle(nil, nil, time.Millisecond) {
		t.Error("stopIfIdle returned true for stopped container")
	}
}

func TestStopIfIdleNotYetIdle(t *testing.T) {
	mc := &managedContainer{
		name:     "test",
		status:   statusRunning,
		lastUsed: time.Now(),
	}
	// Should return false since container was just used.
	if mc.stopIfIdle(nil, nil, time.Hour) {
		t.Error("stopIfIdle returned true for recently used container")
	}
}
