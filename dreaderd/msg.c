#include "defs.h"
#include <poll.h>	/* lib/defs.h includes it only when USE_POLL is set */

Prototype int SendMsg(int to_fd, int send_fd, DnsRes *dres);
Prototype int SendMsgPart(int to_fd, int send_fd, DnsRes *dres, size_t *off);
Prototype int RecvMsg(int from_fd, int *recv_fd, DnsRes *dres);

/*
 * Descriptor passing with the CMSG macros (SCM_RIGHTS).  JG20050830 split
 * SendMsg into two versions; the one for systems without them (msg_accrights)
 * was never compiled any more and is gone.
 */

/*
 * Send what is left of a fixed-size message without waiting for room:
 * from *off on, with the descriptor only while nothing has gone yet (off 0).
 * On the SOCK_STREAM fallback a short write sends the first part, with the
 * descriptor, and the rest needs later calls without control data.
 *
 * Returns 1 when the whole message is sent, 0 when the socket is full or
 * gave ENOBUFS (errno tells which; *off tells how far it got), or -1 on an
 * error (errno set; with *off still 0 nothing was sent).
 */
int
SendMsgPart(int to_fd, int send_fd, DnsRes *dres, size_t *off)
{
    struct msghdr msg;
    struct iovec iov;
    char tbuf[CMSG_SPACE(sizeof(int))];
    struct cmsghdr *cmsg;
    ssize_t res;

    if (*off == 0) {
	if (DebugOpt)
	    printf("SendMsg() pid= %d fd=%d size=%d\n", (int)getpid(), send_fd, (int)sizeof(DnsRes));
#ifdef TCP_NODELAY
	/*
	 * If we do not turn off nagle, lots of little 'group' transactions
	 * can result in major delays even if nagle only kicks in on a few
	 * of them.
	 */
	if (send_fd >= 0) {
	    int one = 1;
	    setsockopt(send_fd, IPPROTO_TCP, TCP_NODELAY, (void *)&one, sizeof(one));
	}
#endif
    }

    while (*off < sizeof(DnsRes)) {
	bzero(&msg, sizeof(msg));
	iov.iov_base = (char *)dres + *off;
	iov.iov_len = sizeof(DnsRes) - *off;
	msg.msg_iov = &iov;
	msg.msg_iovlen = 1;
	if (*off == 0 && send_fd >= 0) {
	    bzero(tbuf, sizeof(tbuf));
	    msg.msg_control = tbuf;
	    msg.msg_controllen = sizeof(tbuf);
	    cmsg = CMSG_FIRSTHDR(&msg);
	    cmsg->cmsg_level = SOL_SOCKET;
	    cmsg->cmsg_type = SCM_RIGHTS;
	    cmsg->cmsg_len = CMSG_LEN(sizeof(int));
	    memcpy((int *)CMSG_DATA(cmsg), &send_fd, sizeof(int));
	    msg.msg_controllen = cmsg->cmsg_len;
	}
	errno = 0;
#ifdef MSG_DONTWAIT
	res = sendmsg(to_fd, &msg, MSG_DONTWAIT);
#else
	{
	    int fl = fcntl(to_fd, F_GETFL, 0);
	    int err;

	    if (fl >= 0 && (fl & O_NONBLOCK) == 0)
		(void)fcntl(to_fd, F_SETFL, fl | O_NONBLOCK);
	    res = sendmsg(to_fd, &msg, 0);
	    err = errno;
	    if (fl >= 0 && (fl & O_NONBLOCK) == 0)
		(void)fcntl(to_fd, F_SETFL, fl);
	    errno = err;
	}
#endif
	if (res > 0) {
	    *off += (size_t)res;
	    continue;
	}
	if (res == 0) {
	    errno = EIO;
	    logit(LOG_ERR, "sendmsg error (%s)", strerror(errno));
	    return(-1);
	}
	if (errno == EINTR)
	    continue;
	if (errno == EAGAIN || errno == EWOULDBLOCK || errno == ENOBUFS)
	    return(0);
	{
	    int err = errno;	/* the caller may look at it */

	    logit(LOG_ERR, "sendmsg error (%s)", strerror(err));
	    errno = err;
	}
	return(-1);
    }
    return(1);
}

/*
 * Send the complete fixed-size message, waiting with poll() while the
 * socket is full instead of spinning at 100% CPU.  Only for a sender whose
 * peer never waits for it in turn: the dreaderd main process uses
 * SendMsgPart() and a queue for its messages to the readers (see
 * ReaderOutSend() in main.c), so a reader may wait here for the main
 * process.
 */
int
SendMsg(int to_fd, int send_fd, DnsRes *dres)
{
    size_t off = 0;
    int r;

    while ((r = SendMsgPart(to_fd, send_fd, dres, &off)) == 0) {
	struct pollfd pfd;

	pfd.fd = to_fd;
	pfd.events = POLLOUT;
	pfd.revents = 0;
	/* ENOBUFS: the socket may well poll writable; just pause */
	(void)poll(&pfd, (errno == ENOBUFS) ? 0 : 1, 50);
    }
    if (r < 0)
	return(-1);
    return((int)sizeof(DnsRes));
}


int
RecvMsg(int from_fd, int *recv_fd, DnsRes *dres)
{
    int result = 0;
    struct msghdr msg;
    struct iovec  iov;
    char tbuf[CMSG_SPACE(sizeof(int))];
    struct cmsghdr *cmsg;
  
    bzero(&msg, sizeof(msg));
  
    iov.iov_base = (void *)dres;
    iov.iov_len = sizeof(DnsRes);
 
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;

    msg.msg_control = (caddr_t)tbuf;
    msg.msg_controllen = sizeof(tbuf);
    msg.msg_flags = 0;

    *recv_fd = -1;
    errno = 0;

    /*
     * Receive message w/ optional file descriptor.  Do not pass MSG_EOR (it
     * is an output flag, not a recvmsg flag).
     */

    for (;;) {
	result = recvmsg(from_fd, &msg, 0);
	if (result < 0 && errno == EINTR)
	    continue;
	break;
    }
    if (result <= 0) {
	if (result < 0 && errno != EAGAIN && errno != EWOULDBLOCK)
		logit(LOG_ERR, "recvmsg error (%s)", strerror(errno));
	return(result);
    }

    if (msg.msg_controllen) {
	if ((cmsg = CMSG_FIRSTHDR(&msg)) != NULL &&
	    cmsg->cmsg_level == SOL_SOCKET &&
	    cmsg->cmsg_type == SCM_RIGHTS &&
	    cmsg->cmsg_len >= CMSG_LEN(sizeof(*recv_fd))) {
	    memcpy(recv_fd, CMSG_DATA(cmsg), sizeof(*recv_fd));
	} else if ((msg.msg_flags & MSG_CTRUNC) == 0) {
	    logit(LOG_ERR, "recvmsg got invalid control message (controllen=%u flags=%d)",
		    (unsigned)msg.msg_controllen, msg.msg_flags);
	    return(-1);
	}
    }
    if (msg.msg_flags & MSG_TRUNC) {
	logit(LOG_ERR, "recvmsg message truncated (flags=%d)", msg.msg_flags);
	if (*recv_fd >= 0) {
	    close(*recv_fd);
	    *recv_fd = -1;
	}
	return(-1);
    }
    if (msg.msg_flags & MSG_CTRUNC) {
	/*
	 * The descriptor was dropped (descriptor limit), but the message
	 * is whole: return it, so the caller can still account for the
	 * connection.
	 */
	logit(LOG_ERR, "recvmsg descriptor dropped (flags=%d)", msg.msg_flags);
	if (*recv_fd >= 0)
	    close(*recv_fd);
	*recv_fd = RECV_FD_LOST;
    }

    /*
     * On the SOCK_STREAM fallback a nonblocking recvmsg() may return only the
     * first part of the DnsRes structure; finish reading the rest so the
     * stream stays aligned and the extracted descriptor is not dropped.
     */
    while ((size_t)result < sizeof(DnsRes)) {
	ssize_t n = read(from_fd, (char *)dres + result,
			 sizeof(DnsRes) - (size_t)result);

	if (n > 0) {
	    result += (int)n;
	    continue;
	}
	if (n < 0 && errno == EINTR)
	    continue;
	if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
	    struct pollfd pfd;

	    pfd.fd = from_fd;
	    pfd.events = POLLIN;
	    pfd.revents = 0;
	    (void)poll(&pfd, 1, 50);
	    continue;
	}
	if (n == 0)
	    errno = EPIPE;
	else
	    logit(LOG_ERR, "recvmsg error (%s)", strerror(errno));
	if (*recv_fd >= 0) {
	    close(*recv_fd);
	    *recv_fd = -1;
	}
	return(-1);
    }

    if (DebugOpt && *recv_fd >= 0)
	printf("RecvMsg() pid=%d fd=%d size=%d\n",
				(int)getpid(), *recv_fd, result);
    return(result);
}
