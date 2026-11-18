package e2e

import (
	"testing"
	"time"
)

// sleepUntilWall returns when the wall clock has passed at (a time without a monotonic
// reading, such as one from Truncate or a file's modification time). diablo looks at
// dnewsfeeds once per wall-clock minute (time(NULL)/60), but time.Sleep counts monotonic
// time, and on a VM whose wall clock runs behind it the sleep ends before diablo's minute
// has turned: the test would then act in the old minute and fail as if the fix were
// missing. After the sleep it waits in short steps until the wall clock reaches at, for
// 30 s at most.
func sleepUntilWall(t *testing.T, at time.Time) {
	t.Helper()
	time.Sleep(time.Until(at))
	start := time.Now()
	lag := at.Sub(start.Round(0))
	if lag <= 0 {
		return
	}
	for now := start.Round(0); now.Before(at); now = time.Now().Round(0) {
		if time.Since(start) > 30*time.Second {
			t.Fatalf("the wall clock did not reach %s within 30 s after the sleep (it is %s)",
				at.Format("15:04:05.000"), now.Format("15:04:05.000"))
		}
		step := at.Sub(now)
		if step > 100*time.Millisecond {
			step = 100 * time.Millisecond
		}
		time.Sleep(step)
	}
	t.Logf("the wall clock was %s behind the sleep; waited %s more",
		lag.Round(time.Millisecond), time.Since(start).Round(time.Millisecond))
}
