#define FDPASS_DAMN_CMSG_MACROS        1

#include "defs.h"

Prototype int SendMsg(int to_fd, int send_fd, DnsRes *dres);
Prototype int RecvMsg(int from_fd, int *recv_fd, DnsRes *dres);

#if FDPASS_DAMN_CMSG_MACROS

/*
 * I generally don't like #ifdef defining entire functions, but
 * this was getting too damn messy.  Linux apparently wants you
 * to use some nifty new cmsg macros to assemble this sort of
 * message, and it wasn't too compatible with what used to live
 * here.  I've broken the SendMsg into two functions for now
 * so it is easier to see what is going on.  JG20050830
 */

int
SendMsg(int to_fd, int send_fd, DnsRes *dres)
{
    struct msghdr msg;
    struct iovec iov;
    char tbuf[CMSG_SPACE(sizeof(int))];
    struct cmsghdr *cmsg;
    int res = 0;
         
    bzero(&msg, sizeof(msg));

    iov.iov_base = (void *)dres;
    iov.iov_len = sizeof(DnsRes);
                
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;
    msg.msg_flags = 0;
    if (send_fd >= 0) {
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
        
    /*
     * If we do not turn off nagle, lots of little 'group' transactions
     * can result in major delays even if nagle only kicks in on a few
     * of them.
     */
        
#ifdef TCP_NODELAY
    /*
     * Turn on TCP_NODELAY
     */
    if (send_fd >= 0) {
      int one = 1;
      setsockopt(send_fd, IPPROTO_TCP, TCP_NODELAY, (void *)&one, sizeof(one));
    }
#endif
     
    /*
     * Send the complete fixed-size message.  On SOCK_SEQPACKET each sendmsg()
     * is atomic; on the SOCK_STREAM fallback a short write can send the first
     * part (with the descriptor) and need another sendmsg() without control
     * data for the tail.  Wait with poll() on EAGAIN/ENOBUFS instead of
     * spinning at 100% CPU while the peer drains the socketpair.
     */
        
    if (DebugOpt)
      printf("SendMsg() pid= %d fd=%d size=%d\n", (int)getpid(), send_fd, (int)sizeof(DnsRes));
    while (iov.iov_len > 0) {
      res = sendmsg(to_fd, &msg, 0);
      if (res > 0) {
          iov.iov_base = (char *)iov.iov_base + res;
          iov.iov_len -= (size_t)res;
          msg.msg_control = NULL;
          msg.msg_controllen = 0;
          continue;
      }
      if (res == 0) {
          errno = EIO;
          logit(LOG_ERR, "sendmsg error (%s)", strerror(errno));
          return(-1);
      }
      if (errno == EINTR)
          continue;
      if (errno == EAGAIN || errno == EWOULDBLOCK || errno == ENOBUFS) {
          struct pollfd pfd;

          pfd.fd = to_fd;
          pfd.events = POLLOUT;
          pfd.revents = 0;
          (void)poll(&pfd, 1, 50);
          continue;
      }
      logit(LOG_ERR, "sendmsg error (%s)", strerror(errno));
      return(-1);
    }
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
    if (msg.msg_flags & (MSG_CTRUNC | MSG_TRUNC)) {
	logit(LOG_ERR, "recvmsg message truncated (flags=%d)", msg.msg_flags);
	if (*recv_fd >= 0) {
	    close(*recv_fd);
	    *recv_fd = -1;
	}
	return(-1);
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



#else /*FDPASS_DAMN_CMSG_MACROS*/




int
SendMsg(int to_fd, int send_fd, DnsRes *dres)
{
    struct msghdr msg;
    struct iovec iov;
    struct {
#if FDPASS_USES_CMSG
	struct cmsghdr cmsg;
#endif
	int fd;
    } cmsg;
    int res = 0;
    int cmsgsize;
    int sent = 0;
         
    bzero(&msg, sizeof(msg));
    bzero(&cmsg, sizeof(cmsg));

    iov.iov_base = (void *)dres;
    iov.iov_len = sizeof(DnsRes);
                
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;
    if (send_fd >= 0) {
	cmsgsize = sizeof(cmsg);
    } else {
#if FDPASS_USES_CMSG
	cmsgsize = sizeof(struct cmsghdr);
#else
	cmsgsize = 0;
#endif
    }
#if FDPASS_USES_ACC
    msg.msg_accrights = (caddr_t)&cmsg;
    msg.msg_accrightslen = cmsgsize;
#else
    msg.msg_control = (caddr_t)&cmsg;
    msg.msg_controllen = cmsgsize;
#endif
#if FDPASS_USES_CMSG
    msg.msg_flags = 0;
    cmsg.cmsg.cmsg_len = cmsgsize;
    cmsg.cmsg.cmsg_level = SOL_SOCKET;
    cmsg.cmsg.cmsg_type = SCM_RIGHTS;
#endif 
    cmsg.fd = send_fd;

    errno = 0;
        
    /*
     * If we do not turn off nagle, lots of little 'group' transactions
     * can result in major delays even if nagle only kicks in on a few
     * of them.
     */
        
#ifdef TCP_NODELAY
    /*
     * Turn on TCP_NODELAY
     */
    if (send_fd >= 0) {
	int one = 1;
	setsockopt(send_fd, IPPROTO_TCP, TCP_NODELAY, (void *)&one, sizeof(one));
    }
#endif
     
    /*
     * Send the message.  This should never fail.  If it does, try to core
     * the reader subprocess.  Certainly kill it.
     */
        
    if (DebugOpt)
	printf("SendMsg() pid= %d fd=%d size=%d\n", (int)getpid(), cmsg.fd, (int)sizeof(DnsRes));
    while (!sent) {
	res = sendmsg(to_fd, &msg, 0);
	sent = 1;
	if (res < 0) {
	    if (errno == EAGAIN)
		sent = 0;
	    else
		logit(LOG_ERR, "sendmsg error (%s)", strerror(errno));
	}
    }
    return(res);
}

int
RecvMsg(int from_fd, int *recv_fd, DnsRes *dres)
{
    int result = 0;
    struct msghdr msg;
    struct iovec  iov;
    struct {
#if FDPASS_USES_CMSG
	struct cmsghdr cmsg;
#endif
	int fd;
    } cmsg;
  
    bzero(&msg, sizeof(msg));
  
    iov.iov_base = (void *)dres;
    iov.iov_len = sizeof(DnsRes);
 
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;
#if FDPASS_USES_ACC
    msg.msg_accrights = (caddr_t)&cmsg;
    msg.msg_accrightslen = sizeof(cmsg);
#else
    msg.msg_control = (caddr_t)&cmsg;
    msg.msg_controllen = sizeof(cmsg);
#endif
#if FDPASS_USES_CMSG
    msg.msg_flags = 0;
    cmsg.cmsg.cmsg_len = sizeof(cmsg);
#endif
    cmsg.fd = -1;
    errno = 0;

    /*
     * receive message w/ file descriptor   
     */

    result = recvmsg(from_fd, &msg, MSG_EOR|MSG_WAITALL);
    if (result >= 0 || errno == EAGAIN) {
	*recv_fd = cmsg.fd;
	if (DebugOpt && errno != EAGAIN)
	    printf("RecvMsg() pid=%d fd=%d size=%d\n",
					(int)getpid(), cmsg.fd, result);
    } else {
	logit(LOG_ERR, "recvmsg error (%s)", strerror(errno));
    }
    return(result);
}
#endif /*FDPASS_DAMN_CMSG_MACROS*/
