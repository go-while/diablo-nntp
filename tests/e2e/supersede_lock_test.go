package e2e

import (
	"strings"
	"testing"
	"time"

	testkit "github.com/go-while/nntp-testkit"
	"github.com/go-while/nntp-testkit/fakeserver"
)

// supersedeLockSite starts a reader site with the given diablo.config directives and a
// dcontrol.ctl that executes cancels.
func supersedeLockSite(tb testing.TB, cfg map[string]string) (*Site, *fakeserver.Store) {
	tb.Helper()
	skipWithoutCanLockMode(tb)
	st, spool := spoolStore(tb)
	cfg["path_generallog"] = "%s/general.log"
	s := NewReaderSite(tb, ReaderOptions{
		Spool:   spool,
		Config:  cfg,
		Files:   map[string]string{"dcontrol.ctl": "cancel:*:*:doit,break\n"},
		NoStart: true,
	})
	_, port, _ := strings.Cut(s.Addr, ":")
	startReader(tb, s, "[127.0.0.1]:"+port)
	return s, st
}

// TestSupersedeLockSeparate: a cancel and a supersede of articles without Cancel-Lock are
// decided by their own option. Fails on main: dreaderd does not know readersupersedelock.
func TestSupersedeLockSeparate(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		cfg                    map[string]string
		cancelGone, supersedes bool // is the cancel target / the superseded article withdrawn
		supLog, canLog         string
	}{
		{"supersedes all, cancels require-auth",
			map[string]string{"readercanlock": "require-auth", "readersupersedelock": "all"},
			false, true, "", "refused, target has no Cancel-Lock"},
		{"supersedes none, cancels all",
			map[string]string{"readercanlock": "all", "readersupersedelock": "none"},
			true, false, "refused (readersupersedelock none)", ""},
		{"supersedes auth, cancels require-auth",
			map[string]string{"readercanlock": "require-auth", "readersupersedelock": "auth"},
			false, true, "target has no Cancel-Lock, executed (readersupersedelock auth)", "refused, target has no Cancel-Lock"},
		{"supersedes unset follows readercanlock none",
			map[string]string{"readercanlock": "none"},
			false, false, "refused (readercanlock none)", "refused (readercanlock none)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st := supersedeLockSite(t, tc.cfg)
			c := s.Reader(t)
			defer c.ExpectCmd(205, "QUIT")
			// article numbers in e2e.test: old 1, target 2, replacement 3
			old, target, replacement := msgID(t), msgID(t), msgID(t)
			feedAndSettle(t, s, st, c,
				newArticle(old, "e2e.test", "old version", "the old version"),
				newArticle(target, "e2e.test", "cancel target", "the cancel target"),
				withHeaders(newArticle(replacement, "e2e.test", "new version", "the new version"), "Supersedes: "+old),
				cancelArticle(t, target, "e2e.test"),
			)
			var log string
			testkit.WaitFor(t, 5*time.Second, "the decisions in the general log", func() bool {
				log = generalLog(s)
				return (tc.supLog == "" || strings.Contains(log, "supersede "+old+" by "+replacement+": "+tc.supLog)) &&
					(tc.canLog == "" || strings.Contains(log, "cancel "+target+": "+tc.canLog))
			})
			if gone := !present(t, c, 1); gone != tc.supersedes {
				t.Errorf("superseded article gone: %v, want %v", gone, tc.supersedes)
			}
			if gone := !present(t, c, 2); gone != tc.cancelGone {
				t.Errorf("canceled article gone: %v, want %v", gone, tc.cancelGone)
			}
			if !present(t, c, 3) {
				t.Errorf("the superseding article was not stored")
			}
			if tc.supLog != "" && !strings.Contains(log, "supersede "+old+" by "+replacement+": "+tc.supLog) {
				t.Errorf("general log lacks the supersede decision %q", tc.supLog)
			}
			if tc.canLog != "" && !strings.Contains(log, "cancel "+target+": "+tc.canLog) {
				t.Errorf("general log lacks the cancel decision %q", tc.canLog)
			}
			if t.Failed() {
				t.Logf("general log:\n%s", log)
			}
		})
	}
}

// TestSupersedeLockTrackonly: like readercanlock, a mode that needs the target's headers is
// refused with readerxover trackonly; the supersede mode is checked on its own.
func TestSupersedeLockTrackonly(t *testing.T) {
	skipWithoutCanLockMode(t)
	for _, tc := range []struct {
		cfg  map[string]string
		ok   bool
		want string
	}{
		{map[string]string{"readercanlock": "all", "readersupersedelock": "require-auth"}, false, "readersupersedelock"},
		{map[string]string{"readercanlock": "all", "readersupersedelock": "none"}, true, ""},
		{map[string]string{"readercanlock": "none", "readersupersedelock": "all"}, true, ""},
	} {
		tc.cfg["readerxover"] = "trackonly"
		_, spool := spoolStore(t)
		s := NewReaderSite(t, ReaderOptions{Spool: spool, Config: tc.cfg, NoStart: true})
		_, port, _ := strings.Cut(s.Addr, ":")
		if tc.ok {
			startReader(t, s, "[127.0.0.1]:"+port)
			s.Reader(t).ExpectCmd(205, "QUIT")
			continue
		}
		out, err := s.TryRun("dreaderd", "-d", "-p", PathHost, "-h", PathHost, "-P", "[127.0.0.1]:"+port)
		if err == nil || !strings.Contains(out, tc.want) {
			t.Errorf("trackonly with %v: err %v, output %q; want a refusal naming %s", tc.cfg, err, out, tc.want)
		}
	}
}

// TestSupersedeLockWarningNoLibcanlock: in a build without libcanlock the warning at start says
// what each option does to its own requests: readercanlock to cancels, readersupersedelock to
// supersedes (it said "no cancel or supersede is executed" for either). Skipped with libcanlock.
func TestSupersedeLockWarningNoLibcanlock(t *testing.T) {
	skipWithoutCanLockMode(t)
	skipWithCanLock(t)
	for _, tc := range []struct {
		name         string
		cfg          map[string]string
		want, notWant string
	}{
		{"cancels require-auth, supersedes all",
			map[string]string{"readercanlock": "require-auth", "readersupersedelock": "all"},
			"readercanlock require-auth: this build has no libcanlock (CANLOCK_ENABLED), so no Cancel-Key can be verified and no cancel is executed",
			"readersupersedelock"},
		{"cancels all, supersedes require-auth",
			map[string]string{"readercanlock": "all", "readersupersedelock": "require-auth"},
			"readersupersedelock require-auth: this build has no libcanlock (CANLOCK_ENABLED), so no Cancel-Key can be verified and no supersede is executed",
			"readercanlock "},
		{"supersedes unset",
			map[string]string{"readercanlock": "require-auth"},
			"readercanlock require-auth: this build has no libcanlock (CANLOCK_ENABLED), so no Cancel-Key can be verified and no cancel or supersede is executed",
			"readersupersedelock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := supersedeLockSite(t, tc.cfg)
			var log string
			testkit.WaitFor(t, 5*time.Second, "the warning in the general log", func() bool {
				log = generalLog(s)
				return strings.Contains(log, "no libcanlock")
			})
			if !strings.Contains(log, tc.want) {
				t.Errorf("general log lacks %q:\n%s", tc.want, log)
			}
			if strings.Contains(log, tc.notWant+" ") && strings.Contains(log, tc.notWant+" "+tc.cfg[strings.TrimSpace(tc.notWant)]+": this build") {
				t.Errorf("general log has a warning for %s:\n%s", tc.notWant, log)
			}
		})
	}
}
