package e2e

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func listenBacklogLimit() (int, string, error) {
	var b []byte
	var err error
	var key string
	switch runtime.GOOS {
	case "linux":
		key = "net.core.somaxconn"
		b, err = os.ReadFile("/proc/sys/net/core/somaxconn")
	case "freebsd":
		key = "kern.ipc.soacceptqueue"
		b, err = inspectionOutput("/sbin/sysctl", "-n", key)
	case "netbsd":
		key = "kern.somaxconn"
		b, err = inspectionOutput("/sbin/sysctl", "-n", key)
	case "solaris":
		key = "tcp_conn_req_max_q"
		b, err = inspectionOutput("/usr/sbin/ndd", "-get", "/dev/tcp", key)
	default:
		return 0, "", fmt.Errorf("kernel listen limit not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return 0, key, err
	}
	limit, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, key, fmt.Errorf("%s returned %q: %w", key, b, err)
	}
	return limit, key, nil
}
