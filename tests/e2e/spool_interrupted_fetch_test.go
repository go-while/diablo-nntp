package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the network spool's fast-copy and shutdown decisions in memory.
// The driver records which request is finished and which is returned to the
// queue. Use -count=1 because the C source is outside the Go module.
func TestSpoolInterruptedFetch(t *testing.T) {
	root := wildmatSource(t)
	source := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, "dreaderd", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	function := func(src, name string) string {
		marker := "void\n" + name + "("
		_, tail, ok := strings.Cut(src, marker)
		body, _, end := strings.Cut(tail, "\n}\n")
		if !ok || !end {
			t.Fatalf("definition of %s not found", name)
		}
		return marker + body + "\n}\n"
	}
	spool := source("spool.c")
	response := function(spool, "NNSpoolResponse3")
	i := strings.LastIndex(response, "\n    if (FastCopyOpt")
	j := strings.Index(response, "    /* still waiting for input */")
	if i < 0 || j <= i {
		t.Fatal("network fast-copy block not found")
	}
	prog := interruptedFetchDriver + "\nstatic void publish(ServReq *sreq) {\n" + response[i:j] + "}\n"
	if strings.Contains(spool, "\nNNFinishInterruptedSpool(") {
		prog += function(spool, "NNFinishInterruptedSpool")
	}
	prog += function(source("server.c"), "NNServerTerminate") + interruptedFetchCases
	dir := t.TempDir()
	src, bin := filepath.Join(dir, "driver.c"), filepath.Join(dir, "driver")
	if err := os.WriteFile(src, []byte(prog), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := func(name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.WaitDelay = time.Second
		return cmd.CombinedOutput()
	}
	if out, err := run(wmCC(t), "-o", bin, src); err != nil {
		t.Fatalf("build queue driver: %v\n%s", err, out)
	}
	for _, mode := range []string{"staged", "article", "stat", "overview"} {
		t.Run(mode, func(t *testing.T) {
			if out, err := run(bin, mode); err != nil {
				t.Fatalf("network spool shutdown: %v: %s", err, out)
			}
		})
	}
}

const interruptedFetchDriver = `
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
enum { COF_CLOSESERVER=1, COF_ININIT=2, COF_INPROGRESS=4,
       THREAD_POST=1, THREAD_LIMSIZE=1000, COM_STAT=1 };
typedef struct { int mh_Bytes; } Buf;
typedef struct { int d_Count, d_Type; } Desc;
typedef struct Connection Connection;
typedef struct ServReq {
    struct ServReq *sr_Next;
    Connection *sr_CConn, *sr_SConn;
    time_t sr_Time;
    int sr_SpoolOutput, cacheOpen;
} ServReq;
struct Connection {
    int co_Flags, co_ArtMode;
    Desc *co_Desc;
    ServReq *co_SReq;
    Buf co_ArtBuf, co_TMBuf;
};
static int NReadServAct=2, NReadServers=1, NWriteServers, NWriteServAct;
static int FastCopyOpt, ServersTerminated, finished;
static ServReq *reads, *writes, **PSRead=&reads, **PSWrite=&writes;
static const char *suffix;
static void MBCopy(Buf *from, Buf *to) {
    to->mh_Bytes += from->mh_Bytes;
    from->mh_Bytes = 0;
}
static void NNFinishSReq(Connection *conn, const char *text, int retry) {
    if (retry || !(conn->co_Flags & COF_CLOSESERVER)) abort();
    conn->co_SReq = conn->co_SReq->sr_Next;
    --conn->co_Desc->d_Count;
    --NReadServAct;
    ++finished;
    suffix = text;
}
static void FreeSReq(ServReq *r) { abort(); }
static void requeueDropCache(ServReq *r) { r->cacheOpen = 0; }
static void QueueServerRequests(void) {}
static void NNTerminate(Connection *c) {}
`

const interruptedFetchCases = `
int main(int argc, char **argv) {
    Connection server = {0}, client = {0}, waiting = {0};
    Desc desc = {2, 0};
    ServReq first = {0}, next = {0};
    int staged = !strcmp(argv[1], "staged");
    int overview = !strcmp(argv[1], "overview");
    int stat = !strcmp(argv[1], "stat");
    server.co_Desc = &desc;
    server.co_SReq = &first;
    first.sr_Next = &next;
    first.sr_CConn = &client;
    first.cacheOpen = 1;
    next.sr_CConn = &waiting;
    client.co_ArtMode = stat ? COM_STAT : 0;
    client.co_TMBuf.mh_Bytes = overview ? 10 : 0;
    client.co_ArtBuf.mh_Bytes = overview ? 0 : 3;
    FastCopyOpt = !staged;
    publish(&first);
    if (!staged && !overview) {
        client.co_TMBuf.mh_Bytes = 0;  /* output already flushed */
        client.co_ArtBuf.mh_Bytes = stat ? 0 : 2;
    }
    NNServerTerminate(&server);
    if (staged || overview) {
        if (finished || reads != &first || first.sr_Next != &next) {
            puts("request with no published spool output lost its retry");
            return 1;
        }
    } else {
        if (reads == &first) {
            puts("request with published spool output was requeued");
            return 1;
        }
        if (finished != 1 || !suffix || strcmp(suffix, stat ? "" :
            "(spool server died prior to completion of the article dump)\r\n.\r\n")) {
            puts("wrong completion for published spool output");
            return 1;
        }
        if (reads != &next) {
            puts("unstarted sibling was not requeued");
            return 1;
        }
        if (client.co_TMBuf.mh_Bytes != (stat ? 0 : 2)) {
            puts("pending article data lost at completion");
            return 1;
        }
    }
    if (first.cacheOpen || NReadServAct || NReadServers || writes || next.sr_Next ||
        server.co_SReq || ServersTerminated != 1) {
        puts("server queue or accounting not drained");
        return 1;
    }
    return 0;
}
`
