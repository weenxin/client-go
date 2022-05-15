# Transports


## network/http Transport

在阅读client-go代码，我们可以先看下http transport是怎么实现的。

### 对象结构

```go
// DefaultTransport is the default implementation of Transport and is
// used by DefaultClient. It establishes network connections as needed
// and caches them for reuse by subsequent calls. It uses HTTP proxies
// as directed by the $HTTP_PROXY and $NO_PROXY (or $http_proxy and
// $no_proxy) environment variables.
var DefaultTransport RoundTripper = &Transport{
	Proxy: ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

type Transport struct {
	idleMu       sync.Mutex
	closeIdle    bool                                // user has requested to close all idle conns
	idleConn     map[connectMethodKey][]*persistConn // most recently used at end
	idleConnWait map[connectMethodKey]wantConnQueue  // waiting getConns
	idleLRU      connLRU

	reqMu       sync.Mutex
	reqCanceler map[cancelKey]func(error)

	altMu    sync.Mutex   // guards changing altProto only
	altProto atomic.Value // of nil or map[string]RoundTripper, key is URI scheme

	connsPerHostMu   sync.Mutex
	connsPerHost     map[connectMethodKey]int
	connsPerHostWait map[connectMethodKey]wantConnQueue // waiting getConns

	// Proxy specifies a function to return a proxy for a given
	// Request. If the function returns a non-nil error, the
	// request is aborted with the provided error.
	//
	// The proxy type is determined by the URL scheme. "http",
	// "https", and "socks5" are supported. If the scheme is empty,
	// "http" is assumed.
	//
	// If Proxy is nil or returns a nil *URL, no proxy is used.
	Proxy func(*Request) (*url.URL, error)

	// DialContext specifies the dial function for creating unencrypted TCP connections.
	// If DialContext is nil (and the deprecated Dial below is also nil),
	// then the transport dials using package net.
	//
	// DialContext runs concurrently with calls to RoundTrip.
	// A RoundTrip call that initiates a dial may end up using
	// a connection dialed previously when the earlier connection
	// becomes idle before the later DialContext completes.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// Dial specifies the dial function for creating unencrypted TCP connections.
	//
	// Dial runs concurrently with calls to RoundTrip.
	// A RoundTrip call that initiates a dial may end up using
	// a connection dialed previously when the earlier connection
	// becomes idle before the later Dial completes.
	//
	// Deprecated: Use DialContext instead, which allows the transport
	// to cancel dials as soon as they are no longer needed.
	// If both are set, DialContext takes priority.
	Dial func(network, addr string) (net.Conn, error)

	// DialTLSContext specifies an optional dial function for creating
	// TLS connections for non-proxied HTTPS requests.
	//
	// If DialTLSContext is nil (and the deprecated DialTLS below is also nil),
	// DialContext and TLSClientConfig are used.
	//
	// If DialTLSContext is set, the Dial and DialContext hooks are not used for HTTPS
	// requests and the TLSClientConfig and TLSHandshakeTimeout
	// are ignored. The returned net.Conn is assumed to already be
	// past the TLS handshake.
	DialTLSContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// DialTLS specifies an optional dial function for creating
	// TLS connections for non-proxied HTTPS requests.
	//
	// Deprecated: Use DialTLSContext instead, which allows the transport
	// to cancel dials as soon as they are no longer needed.
	// If both are set, DialTLSContext takes priority.
	DialTLS func(network, addr string) (net.Conn, error)

	// TLSClientConfig specifies the TLS configuration to use with
	// tls.Client.
	// If nil, the default configuration is used.
	// If non-nil, HTTP/2 support may not be enabled by default.
	TLSClientConfig *tls.Config

	// TLSHandshakeTimeout specifies the maximum amount of time waiting to
	// wait for a TLS handshake. Zero means no timeout.
	TLSHandshakeTimeout time.Duration

	// DisableKeepAlives, if true, disables HTTP keep-alives and
	// will only use the connection to the server for a single
	// HTTP request.
	//
	// This is unrelated to the similarly named TCP keep-alives.
	DisableKeepAlives bool

	// DisableCompression, if true, prevents the Transport from
	// requesting compression with an "Accept-Encoding: gzip"
	// request header when the Request contains no existing
	// Accept-Encoding value. If the Transport requests gzip on
	// its own and gets a gzipped response, it's transparently
	// decoded in the Response.Body. However, if the user
	// explicitly requested gzip it is not automatically
	// uncompressed.
	DisableCompression bool

	// MaxIdleConns controls the maximum number of idle (keep-alive)
	// connections across all hosts. Zero means no limit.
	MaxIdleConns int

	// MaxIdleConnsPerHost, if non-zero, controls the maximum idle
	// (keep-alive) connections to keep per-host. If zero,
	// DefaultMaxIdleConnsPerHost is used.
	MaxIdleConnsPerHost int

	// MaxConnsPerHost optionally limits the total number of
	// connections per host, including connections in the dialing,
	// active, and idle states. On limit violation, dials will block.
	//
	// Zero means no limit.
	MaxConnsPerHost int

	// IdleConnTimeout is the maximum amount of time an idle
	// (keep-alive) connection will remain idle before closing
	// itself.
	// Zero means no limit.
	IdleConnTimeout time.Duration

	// ResponseHeaderTimeout, if non-zero, specifies the amount of
	// time to wait for a server's response headers after fully
	// writing the request (including its body, if any). This
	// time does not include the time to read the response body.
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout, if non-zero, specifies the amount of
	// time to wait for a server's first response headers after fully
	// writing the request headers if the request has an
	// "Expect: 100-continue" header. Zero means no timeout and
	// causes the body to be sent immediately, without
	// waiting for the server to approve.
	// This time does not include the time to send the request header.
	ExpectContinueTimeout time.Duration

	// TLSNextProto specifies how the Transport switches to an
	// alternate protocol (such as HTTP/2) after a TLS ALPN
	// protocol negotiation. If Transport dials an TLS connection
	// with a non-empty protocol name and TLSNextProto contains a
	// map entry for that key (such as "h2"), then the func is
	// called with the request's authority (such as "example.com"
	// or "example.com:1234") and the TLS connection. The function
	// must return a RoundTripper that then handles the request.
	// If TLSNextProto is not nil, HTTP/2 support is not enabled
	// automatically.
	TLSNextProto map[string]func(authority string, c *tls.Conn) RoundTripper

	// ProxyConnectHeader optionally specifies headers to send to
	// proxies during CONNECT requests.
	// To set the header dynamically, see GetProxyConnectHeader.
	ProxyConnectHeader Header

	// GetProxyConnectHeader optionally specifies a func to return
	// headers to send to proxyURL during a CONNECT request to the
	// ip:port target.
	// If it returns an error, the Transport's RoundTrip fails with
	// that error. It can return (nil, nil) to not add headers.
	// If GetProxyConnectHeader is non-nil, ProxyConnectHeader is
	// ignored.
	GetProxyConnectHeader func(ctx context.Context, proxyURL *url.URL, target string) (Header, error)

	// MaxResponseHeaderBytes specifies a limit on how many
	// response bytes are allowed in the server's response
	// header.
	//
	// Zero means to use a default limit.
	MaxResponseHeaderBytes int64

	// WriteBufferSize specifies the size of the write buffer used
	// when writing to the transport.
	// If zero, a default (currently 4KB) is used.
	WriteBufferSize int

	// ReadBufferSize specifies the size of the read buffer used
	// when reading from the transport.
	// If zero, a default (currently 4KB) is used.
	ReadBufferSize int

	// nextProtoOnce guards initialization of TLSNextProto and
	// h2transport (via onceSetNextProtoDefaults)
	nextProtoOnce      sync.Once
	h2transport        h2Transport // non-nil if http2 wired up
	tlsNextProtoWasNil bool        // whether TLSNextProto was nil when the Once fired

	// ForceAttemptHTTP2 controls whether HTTP/2 is enabled when a non-zero
	// Dial, DialTLS, or DialContext func or TLSClientConfig is provided.
	// By default, use of any those fields conservatively disables HTTP/2.
	// To use a custom dialer or TLS config and still attempt HTTP/2
	// upgrades, set this to true.
	ForceAttemptHTTP2 bool
}
```

一切从`RoundTrip`开始

```go
// RoundTrip 实现了  RoundTripper 接口.
//
// 实现了更高层次的  HTTP client 支持 (比如处理  cookies
// 和 重定向)。
//
// Like the RoundTripper interface, the error types returned
// by RoundTrip are unspecified.
func (t *Transport) RoundTrip(req *Request) (*Response, error) {
	return t.roundTrip(req)
}
```

```go
// roundTrip implements a RoundTripper over HTTP.
func (t *Transport) roundTrip(req *Request) (*Response, error) {
//简单起见，我们假设没有启用使用htt请求，并且没有开启trace相关操作
	t.nextProtoOnce.Do(t.onceSetNextProtoDefaults)
	ctx := req.Context()
	trace := httptrace.ContextClientTrace(ctx)

    // 有效性检查
	if req.URL == nil {
		req.closeBody()
		return nil, errors.New("http: nil Request.URL")
	}
	if req.Header == nil {
		req.closeBody()
		return nil, errors.New("http: nil Request.Header")
	}
	scheme := req.URL.Scheme
	isHTTP := scheme == "http" || scheme == "https"
	//检查header信息
	if isHTTP {
		for k, vv := range req.Header {
			if !httpguts.ValidHeaderFieldName(k) {
				req.closeBody()
				return nil, fmt.Errorf("net/http: invalid header field name %q", k)
			}
			for _, v := range vv {
				if !httpguts.ValidHeaderFieldValue(v) {
					req.closeBody()
					return nil, fmt.Errorf("net/http: invalid header field value %q for key %v", v, k)
				}
			}
		}
	}

	origReq := req
	cancelKey := cancelKey{origReq}
	//包装下req的body
	req = setupRewindBody(req)

    //略过
	if altRT := t.alternateRoundTripper(req); altRT != nil {
		if resp, err := altRT.RoundTrip(req); err != ErrSkipAltProtocol {
			return resp, err
		}
		var err error
		req, err = rewindBody(req)
		if err != nil {
			return nil, err
		}
	}
	if !isHTTP {
		req.closeBody()
		return nil, badStringError("unsupported protocol scheme", scheme)
	}
	if req.Method != "" && !validMethod(req.Method) {
		req.closeBody()
		return nil, fmt.Errorf("net/http: invalid method %q", req.Method)
	}
	if req.URL.Host == "" {
		req.closeBody()
		return nil, errors.New("http: no Host in request URL")
	}

    //循环执行
	for {
		select {
		//context 取消了
		case <-ctx.Done():
			req.closeBody()
			return nil, ctx.Err()
		default:
		}

		// treq gets modified by roundTrip, so we need to recreate for each retry.
		treq := &transportRequest{Request: req, trace: trace, cancelKey: cancelKey}
		//cm中有 scheme，targetAddr， proxyUrl，onlyH1
		cm, err := t.connectMethodForRequest(treq)
		if err != nil {
			req.closeBody()
			return nil, err
		}

		// Get the cached or newly-created connection to either the
		// host (for http or https), the http proxy, or the http proxy
		// pre-CONNECTed to https server. In any case, we'll be ready
		// to send it requests.
		//从缓存中获取链接，
		pconn, err := t.getConn(treq, cm)
		if err != nil {
			t.setReqCanceler(cancelKey, nil)
			req.closeBody()
			return nil, err
		}

		var resp *Response
		if pconn.alt != nil {
			// HTTP/2 path.
			t.setReqCanceler(cancelKey, nil) // not cancelable with CancelRequest
			resp, err = pconn.alt.RoundTrip(req)
		} else {
			resp, err = pconn.roundTrip(treq)
		}
		if err == nil {
			resp.Request = origReq
			return resp, nil
		}

		// Failed. Clean up and determine whether to retry.
		if http2isNoCachedConnError(err) {
			if t.removeIdleConn(pconn) {
				t.decConnsPerHost(pconn.cacheKey)
			}
		} else if !pconn.shouldRetryRequest(req, err) {
			// Issue 16465: return underlying net.Conn.Read error from peek,
			// as we've historically done.
			if e, ok := err.(transportReadFromServerError); ok {
				err = e.err
			}
			return nil, err
		}
		testHookRoundTripRetried()

		// Rewind the body if we're able to.
		req, err = rewindBody(req)
		if err != nil {
			return nil, err
		}
	}
}
```


```go
// getConn dials and creates a new persistConn to the target as
// specified in the connectMethod. This includes doing a proxy CONNECT
// and/or setting up TLS.  If this doesn't return an error, the persistConn
// is ready to write requests to.
func (t *Transport) getConn(treq *transportRequest, cm connectMethod) (pc *persistConn, err error) {
	req := treq.Request
	trace := treq.trace
	ctx := req.Context()
	//trace 略过
	if trace != nil && trace.GetConn != nil {
		trace.GetConn(cm.addr())
	}

	w := &wantConn{
		cm:         cm,
		key:        cm.key(),
		ctx:        ctx,
		ready:      make(chan struct{}, 1),
		beforeDial: testHookPrePendingDial,
		afterDial:  testHookPostPendingDial,
	}
	defer func() {
		if err != nil {
		    // 关闭ready chan ，同时将连接关闭或者加入到空闲队列中putOrCloseIdleConn
			w.cancel(t, err)
		}
	}()

	// Queue for idle connection.
	if delivered := t.queueForIdleConn(w); delivered {
		pc := w.pc
		// Trace only for HTTP/1.
		// HTTP/2 calls trace.GotConn itself.
		if pc.alt == nil && trace != nil && trace.GotConn != nil {
			trace.GotConn(pc.gotIdleConnTrace(pc.idleAt))
		}
		// set request canceler to some non-nil function so we
		// can detect whether it was cleared between now and when
		// we enter roundTrip
		t.setReqCanceler(treq.cancelKey, func(error) {})
		return pc, nil
	}

	cancelc := make(chan error, 1)
	t.setReqCanceler(treq.cancelKey, func(err error) { cancelc <- err })

	// Queue for permission to dial.
	t.queueForDial(w)

	// Wait for completion or cancellation.
	select {
	case <-w.ready:
		// Trace success but only for HTTP/1.
		// HTTP/2 calls trace.GotConn itself.
		if w.pc != nil && w.pc.alt == nil && trace != nil && trace.GotConn != nil {
			trace.GotConn(httptrace.GotConnInfo{Conn: w.pc.conn, Reused: w.pc.isReused()})
		}
		if w.err != nil {
			// If the request has been cancelled, that's probably
			// what caused w.err; if so, prefer to return the
			// cancellation error (see golang.org/issue/16049).
			select {
			case <-req.Cancel:
				return nil, errRequestCanceledConn
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case err := <-cancelc:
				if err == errRequestCanceled {
					err = errRequestCanceledConn
				}
				return nil, err
			default:
				// return below
			}
		}
		return w.pc, w.err
	case <-req.Cancel:
		return nil, errRequestCanceledConn
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case err := <-cancelc:
		if err == errRequestCanceled {
			err = errRequestCanceledConn
		}
		return nil, err
	}
}
```


```go
// queueForIdleConn queues w to receive the next idle connection for w.cm.
// As an optimization hint to the caller, queueForIdleConn reports whether
// it successfully delivered an already-idle connection.
func (t *Transport) queueForIdleConn(w *wantConn) (delivered bool) {

    //如果取消了KeepAlives，则直接返回失败
	if t.DisableKeepAlives {
		return false
	}

	t.idleMu.Lock()
	defer t.idleMu.Unlock()

	// Stop closing connections that become idle - we might want one.
	// (That is, undo the effect of t.CloseIdleConnections.)
	t.closeIdle = false

	if w == nil {
		// Happens in test hook.
		return false
	}

	// If IdleConnTimeout is set, calculate the oldest
	// persistConn.idleAt time we're willing to use a cached idle
	// conn.
	//看下最大空闲时间
	var oldTime time.Time
	if t.IdleConnTimeout > 0 {
		oldTime = time.Now().Add(-t.IdleConnTimeout)
	}

	// Look for most recently-used idle connection.
	//基于缓存的key做查询
	if list, ok := t.idleConn[w.key]; ok {
		stop := false
		delivered := false
		for len(list) > 0 && !stop {
			pconn := list[len(list)-1]

			// See whether this connection has been idle too long, considering
			// only the wall time (the Round(0)), in case this is a laptop or VM
			// coming out of suspend with previously cached idle connections.
			tooOld := !oldTime.IsZero() && pconn.idleAt.Round(0).Before(oldTime)
			if tooOld {
				// Async cleanup. Launch in its own goroutine (as if a
				// time.AfterFunc called it); it acquires idleMu, which we're
				// holding, and does a synchronous net.Conn.Close.
				go pconn.closeConnIfStillIdle()
			}
			if pconn.isBroken() || tooOld {
				// If either persistConn.readLoop has marked the connection
				// broken, but Transport.removeIdleConn has not yet removed it
				// from the idle list, or if this persistConn is too old (it was
				// idle too long), then ignore it and look for another. In both
				// cases it's already in the process of being closed.
				list = list[:len(list)-1]
				continue
			}
			delivered = w.tryDeliver(pconn, nil)
			if delivered {
				if pconn.alt != nil {
					// HTTP/2: multiple clients can share pconn.
					// Leave it in the list.
				} else {
					// HTTP/1: only one client can use pconn.
					// Remove it from the list.
					// http 1只支持一个连接同一个时间只能工作一个请求
					// 所以从空闲链表中删除这个连接
					t.idleLRU.remove(pconn)
					list = list[:len(list)-1]
				}
			}
			stop = true
		}
		//如果list中还有空闲连接则设置新的连接，走释放逻辑
		if len(list) > 0 {
			t.idleConn[w.key] = list
		} else {
		    //否则删除这个key
			delete(t.idleConn, w.key)
		}
		if stop {
			return delivered
		}
	}

	// Register to receive next connection that becomes idle.
	// 在需求空闲连接中增加一个队列
	if t.idleConnWait == nil {
		t.idleConnWait = make(map[connectMethodKey]wantConnQueue)
	}
	q := t.idleConnWait[w.key]
	//清理所有已经搞定的item
	q.cleanFront()
	//增加一个元素
	q.pushBack(w)
	//设置队列
	t.idleConnWait[w.key] = q
	// 没有成功获取则，返回没有拿到
	return false
}

```


```go
func (t *Transport) getConn(treq *transportRequest, cm connectMethod) (pc *persistConn, err error) {

//。。。忽略前面的过程
// Queue for idle connection.
	if delivered := t.queueForIdleConn(w); delivered {
		pc := w.pc
		// Trace only for HTTP/1.
		// HTTP/2 calls trace.GotConn itself.
		if pc.alt == nil && trace != nil && trace.GotConn != nil {
			trace.GotConn(pc.gotIdleConnTrace(pc.idleAt))
		}
		// set request canceler to some non-nil function so we
		// can detect whether it was cleared between now and when
		// we enter roundTrip
		t.setReqCanceler(treq.cancelKey, func(error) {})
		return pc, nil
	}

    // 如果没有获取到 persist Connection 则继续执行
    cancelc := make(chan error, 1)
    //异常关闭
	t.setReqCanceler(treq.cancelKey, func(err error) { cancelc <- err })

	// Queue for permission to dial.
	// 开启新的连接
	t.queueForDial(w)
}

```

```go
// queueForDial queues w to wait for permission to begin dialing.
// Once w receives permission to dial, it will do so in a separate goroutine.
func (t *Transport) queueForDial(w *wantConn) {
	w.beforeDial()
	//如果不做限制的话，直接开始拨号，并返回连接
	if t.MaxConnsPerHost <= 0 {
		go t.dialConnFor(w)
		return
	}

	t.connsPerHostMu.Lock()
	defer t.connsPerHostMu.Unlock()
    //否则判断下，是否可以是否超出连接数
	if n := t.connsPerHost[w.key]; n < t.MaxConnsPerHost {
		if t.connsPerHost == nil {
			t.connsPerHost = make(map[connectMethodKey]int)
		}
		t.connsPerHost[w.key] = n + 1
		//开始拨号
		go t.dialConnFor(w)
		return
	}

    //超出范围，就只能等待其他人释放
	if t.connsPerHostWait == nil {
		t.connsPerHostWait = make(map[connectMethodKey]wantConnQueue)
	}
	q := t.connsPerHostWait[w.key]
	q.cleanFront()
	q.pushBack(w)
	t.connsPerHostWait[w.key] = q
}
```


```go
// dialConnFor dials on behalf of w and delivers the result to w.
// dialConnFor has received permission to dial w.cm and is counted in t.connCount[w.cm.key()].
// If the dial is canceled or unsuccessful, dialConnFor decrements t.connCount[w.cm.key()].
func (t *Transport) dialConnFor(w *wantConn) {
	defer w.afterDial()

    //拨号
	pc, err := t.dialConn(w.ctx, w.cm)
	delivered := w.tryDeliver(pc, err)
	//成功拨号，并且没有成功返回连接
	if err == nil && (!delivered || pc.alt != nil) {
		// pconn was not passed to w,
		// or it is HTTP/2 and can be shared.
		// Add to the idle connection pool.
		// 将空闲连接给其他需要的连接
		t.putOrCloseIdleConn(pc)
	}
	if err != nil {
	    //降低连接数
		t.decConnsPerHost(w.key)
	}
}
```

```go
// tryDeliver attempts to deliver pc, err to w and reports whether it succeeded.
func (w *wantConn) tryDeliver(pc *persistConn, err error) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.pc != nil || w.err != nil {
		return false
	}

	w.pc = pc
	w.err = err
	if w.pc == nil && w.err == nil {
		panic("net/http: internal error: misuse of tryDeliver")
	}
	//让等待后去连接的item停止阻塞
	close(w.ready)
	return true
}
```

```go
func (t *Transport) putOrCloseIdleConn(pconn *persistConn) {
	if err := t.tryPutIdleConn(pconn); err != nil {
		pconn.close(err)
	}
}
```

```go
// tryPutIdleConn adds pconn to the list of idle persistent connections awaiting
// a new request.
// If pconn is no longer needed or not in a good state, tryPutIdleConn returns
// an error explaining why it wasn't registered.
// tryPutIdleConn does not close pconn. Use putOrCloseIdleConn instead for that.
func (t *Transport) tryPutIdleConn(pconn *persistConn) error {
    //如果没有开启连接复用则不需要返回连接
	if t.DisableKeepAlives || t.MaxIdleConnsPerHost < 0 {
		return errKeepAlivesDisabled
	}
	//如果连接已经被关闭
	if pconn.isBroken() {
		return errConnBroken
	}
	//设置为重用
	pconn.markReused()

	t.idleMu.Lock()
	defer t.idleMu.Unlock()

	// HTTP/2 (pconn.alt != nil) connections do not come out of the idle list,
	// because multiple goroutines can use them simultaneously.
	// If this is an HTTP/2 connection being “returned,” we're done.
	if pconn.alt != nil && t.idleLRU.m[pconn] != nil {
		return nil
	}

	// Deliver pconn to goroutine waiting for idle connection, if any.
	// (They may be actively dialing, but this conn is ready first.
	// Chrome calls this socket late binding.
	// See https://www.chromium.org/developers/design-documents/network-stack#TOC-Connection-Management.)
	key := pconn.cacheKey
	//有没有请求在等待连接执行
	if q, ok := t.idleConnWait[key]; ok {
		done := false
		if pconn.alt == nil {
			// HTTP/1.
			// Loop over the waiting list until we find a w that isn't done already, and hand it pconn.
			for q.len() > 0 {
				w := q.popFront()
				if w.tryDeliver(pconn, nil) {
					done = true
					break
				}
			}
		} else {
			// HTTP/2.
			// Can hand the same pconn to everyone in the waiting list,
			// and we still won't be done: we want to put it in the idle
			// list unconditionally, for any future clients too.
			for q.len() > 0 {
				w := q.popFront()
				//尝试给w分配连接
				w.tryDeliver(pconn, nil)
			}
		}
		//如果连接数为0，则释放key条目
		if q.len() == 0 {
			delete(t.idleConnWait, key)
		} else {
		//否则减少请求队列等待的条目
			t.idleConnWait[key] = q
		}
		//如果完成了连接复用则直接返回
		if done {
			return nil
		}
	}

    //如果用户请求关闭所有空闲连接，则不连接重用
	if t.closeIdle {
		return errCloseIdle
	}
	//准备记录空闲连接
	if t.idleConn == nil {
		t.idleConn = make(map[connectMethodKey][]*persistConn)
	}
	idles := t.idleConn[key]
	//如果已经存在太多的空闲连接了，则直接返回错误
	if len(idles) >= t.maxIdleConnsPerHost() {
		return errTooManyIdleHost
	}
	//
	for _, exist := range idles {
	    //如果一个连接被回收两次，则panic
		if exist == pconn {
			log.Fatalf("dup idle pconn %p in freelist", pconn)
		}
	}
	//增加连接
	t.idleConn[key] = append(idles, pconn)
	//全部连接数量，不对某个host
	t.idleLRU.add(pconn)
	//如果规定了最大连接数，且空闲连接数太多需要回收
	if t.MaxIdleConns != 0 && t.idleLRU.len() > t.MaxIdleConns {
	    //获取最老的连接
		oldest := t.idleLRU.removeOldest()
		//关闭连接
		oldest.close(errTooManyIdle)
		//删除连接
		t.removeIdleConnLocked(oldest)
	}

	// Set idle timer, but only for HTTP/1 (pconn.alt == nil).
	// The HTTP/2 implementation manages the idle timer itself
	// (see idleConnTimeout in h2_bundle.go).
	// 定时清理连接
	if t.IdleConnTimeout > 0 && pconn.alt == nil {
		if pconn.idleTimer != nil {
			pconn.idleTimer.Reset(t.IdleConnTimeout)
		} else {
			pconn.idleTimer = time.AfterFunc(t.IdleConnTimeout, pconn.closeConnIfStillIdle)
		}
	}
	//设置空闲时间
	pconn.idleAt = time.Now()
	return nil
}
```

### persistConn


```go
func (t *Transport) dialConn(ctx context.Context, cm connectMethod) (pconn *persistConn, err error) {
//................................................................

	go pconn.readLoop()
	go pconn.writeLoop()

}
```

```go
func (pc *persistConn) roundTrip(req *transportRequest) (resp *Response, err error) {
	//................................................................
	startBytesWritten := pc.nwrite
	writeErrCh := make(chan error, 1)
	//write_loop开始工作
	pc.writech <- writeRequest{req, writeErrCh, continueCh}

	resc := make(chan responseAndError)
	//read_loop开始工作
	pc.reqch <- requestAndChan{
		req:        req.Request,
		cancelKey:  req.cancelKey,
		ch:         resc,
		addedGzip:  requestedGzip,
		continueCh: continueCh,
		callerGone: gone,
	}
	//................................................................

	for {

		select {
		case re := <-resc:
			if (re.res == nil) == (re.err == nil) {
				panic(fmt.Sprintf("internal error: exactly one of res or err should be set; nil=%v", re.res == nil))
			}
			if debugRoundTrip {
				req.logf("resc recv: %p, %T/%#v", re.res, re.err, re.err)
			}
			if re.err != nil {
				return nil, pc.mapRoundTripError(req, startBytesWritten, re.err)
			}
			return re.res, nil

		}
	}
	//.................................
}

```


```go
func (pc *persistConn) readLoop() {

	closeErr := errReadLoopExiting // default value, if not changed below
	//流程中有各种错误，都会回收掉连接
	defer func() {
		pc.close(closeErr)
		pc.t.removeIdleConn(pc)
	}()
	//................................................................
}
```


```go
func (pc *persistConn) writeLoop() {
	defer close(pc.writeLoopDone)
	for {
		select {
		case wr := <-pc.writech:
			startBytesWritten := pc.nwrite
			err := wr.req.Request.write(pc.bw, pc.isProxy, wr.req.extra, pc.waitForContinue(wr.continueCh))
			if bre, ok := err.(requestBodyReadError); ok {
				err = bre.error
				// Errors reading from the user's
				// Request.Body are high priority.
				// Set it here before sending on the
				// channels below or calling
				// pc.close() which tears down
				// connections and causes other
				// errors.
				wr.req.setError(err)
			}
			if err == nil {
				err = pc.bw.Flush()
			}
			if err != nil {
				if pc.nwrite == startBytesWritten {
					err = nothingWrittenError{err}
				}
			}
			pc.writeErrCh <- err // to the body reader, which might recycle us
			wr.ch <- err         // to the roundTrip function
			//写请求时遇到各种错误也会停止连接
			if err != nil {
				pc.close(err)
				return
			}
		case <-pc.closech:
			return
		}
	}
}
```

## client-go transport

### 对象结构

```go
// Config holds various options for establishing a transport.
type Config struct {
	// UserAgent is an optional field that specifies the caller of this
	// request.
	UserAgent string

	// The base TLS configuration for this transport.
	TLS TLSConfig

	// Username and password for basic authentication
	Username string
	Password string `datapolicy:"password"`

	// Bearer token for authentication
	BearerToken string `datapolicy:"token"`

	// Path to a file containing a BearerToken.
	// If set, the contents are periodically read.
	// The last successfully read value takes precedence over BearerToken.
	BearerTokenFile string

	// Impersonate is the config that this Config will impersonate using
	Impersonate ImpersonationConfig

	// DisableCompression bypasses automatic GZip compression requests to the
	// server.
	DisableCompression bool

	// Transport may be used for custom HTTP behavior. This attribute may
	// not be specified with the TLS client certificate options. Use
	// WrapTransport for most client level operations.
	Transport http.RoundTripper

	// WrapTransport will be invoked for custom HTTP behavior after the
	// underlying transport is initialized (either the transport created
	// from TLSClientConfig, Transport, or http.DefaultTransport). The
	// config may layer other RoundTrippers on top of the returned
	// RoundTripper.
	//
	// A future release will change this field to an array. Use config.Wrap()
	// instead of setting this value directly.
	WrapTransport WrapperFunc

	// Dial specifies the dial function for creating unencrypted TCP connections.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)

	// Proxy is the proxy func to be used for all requests made by this
	// transport. If Proxy is nil, http.ProxyFromEnvironment is used. If Proxy
	// returns a nil *URL, no proxy is used.
	//
	// socks5 proxying does not currently support spdy streaming endpoints.
	Proxy func(*http.Request) (*url.URL, error)
}
```

### 构建方法

```go
func New(config *Config) (http.RoundTripper, error) {
	// Set transport level security
	if config.Transport != nil && (config.HasCA() || config.HasCertAuth() || config.HasCertCallback() || config.TLS.Insecure) {
		return nil, fmt.Errorf("using a custom transport with TLS certificate options or the insecure flag is not allowed")
	}

	var (
		rt  http.RoundTripper
		err error
	)
    //如果config中指定了Transport，则直接使用目标Transport
	if config.Transport != nil {
		rt = config.Transport
	} else {//否则尝试从cache中取
		rt, err = tlsCache.get(config)
		if err != nil {
			return nil, err
		}
	}
    //无论怎样都封装下Transport
	return HTTPWrappersForConfig(config, rt)
}
```

```go
// HTTPWrappersForConfig wraps a round tripper with any relevant layered
// behavior from the config. Exposed to allow more clients that need HTTP-like
// behavior but then must hijack the underlying connection (like WebSocket or
// HTTP2 clients). Pure HTTP clients should use the RoundTripper returned from
// New.
func HTTPWrappersForConfig(config *Config, rt http.RoundTripper) (http.RoundTripper, error) {

    //如果需要包装下，则进行包装
	if config.WrapTransport != nil {
		rt = config.WrapTransport(rt)
	}

	rt = DebugWrappers(rt)

	// Set authentication wrappers
	// 认证封装
	switch {
	case config.HasBasicAuth() && config.HasTokenAuth():
		return nil, fmt.Errorf("username/password or bearer token may be set, but not both")
	case config.HasTokenAuth():
		var err error
		rt, err = NewBearerAuthWithRefreshRoundTripper(config.BearerToken, config.BearerTokenFile, rt)
		if err != nil {
			return nil, err
		}
	case config.HasBasicAuth():
		rt = NewBasicAuthRoundTripper(config.Username, config.Password, rt)
	}
	//UserAgent封装
	if len(config.UserAgent) > 0 {
		rt = NewUserAgentRoundTripper(config.UserAgent, rt)
	}
	//伪装用户
	//https://kubernetes.io/zh/docs/reference/access-authn-authz/authentication/#user-impersonation
	if len(config.Impersonate.UserName) > 0 ||
		len(config.Impersonate.Groups) > 0 ||
		len(config.Impersonate.Extra) > 0 {
		rt = NewImpersonatingRoundTripper(config.Impersonate, rt)
	}
	return rt, nil
}
```

#### basicAuthRoundTripper

```go
type basicAuthRoundTripper struct {
	username string
	password string `datapolicy:"password"`
	rt       http.RoundTripper
}

// NewBasicAuthRoundTripper will apply a BASIC auth authorization header to a
// request unless it has already been set.
func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &basicAuthRoundTripper{username, password, rt}
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	req = utilnet.CloneRequest(req)
	req.SetBasicAuth(rt.username, rt.password)
	return rt.rt.RoundTrip(req)
}

func (rt *basicAuthRoundTripper) CancelRequest(req *http.Request) {
	tryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *basicAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }
```

#### userAgentRoundTripper

```go

type userAgentRoundTripper struct {
	agent string
	rt    http.RoundTripper
}

// NewUserAgentRoundTripper will add User-Agent header to a request unless it has already been set.
func NewUserAgentRoundTripper(agent string, rt http.RoundTripper) http.RoundTripper {
	return &userAgentRoundTripper{agent, rt}
}

func (rt *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("User-Agent")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	req = utilnet.CloneRequest(req)
	req.Header.Set("User-Agent", rt.agent)
	return rt.rt.RoundTrip(req)
}

func (rt *userAgentRoundTripper) CancelRequest(req *http.Request) {
	tryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *userAgentRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }

```

#### impersonatingRoundTripper


```go
type impersonatingRoundTripper struct {
	impersonate ImpersonationConfig
	delegate    http.RoundTripper
}

// NewImpersonatingRoundTripper will add an Act-As header to a request unless it has already been set.
func NewImpersonatingRoundTripper(impersonate ImpersonationConfig, delegate http.RoundTripper) http.RoundTripper {
	return &impersonatingRoundTripper{impersonate, delegate}
}

func (rt *impersonatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// use the user header as marker for the rest.
	if len(req.Header.Get(ImpersonateUserHeader)) != 0 {
		return rt.delegate.RoundTrip(req)
	}
	req = utilnet.CloneRequest(req)
	req.Header.Set(ImpersonateUserHeader, rt.impersonate.UserName)

	for _, group := range rt.impersonate.Groups {
		req.Header.Add(ImpersonateGroupHeader, group)
	}
	for k, vv := range rt.impersonate.Extra {
		for _, v := range vv {
			req.Header.Add(ImpersonateUserExtraHeaderPrefix+headerKeyEscape(k), v)
		}
	}

	return rt.delegate.RoundTrip(req)
}

func (rt *impersonatingRoundTripper) CancelRequest(req *http.Request) {
	tryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *impersonatingRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.delegate }

type bearerAuthRoundTripper struct {
	bearer string
	source oauth2.TokenSource
	rt     http.RoundTripper
}
```


#### bearerAuthRoundTripper


```go
type bearerAuthRoundTripper struct {
	bearer string
	source oauth2.TokenSource
	rt     http.RoundTripper
}

// NewBearerAuthRoundTripper adds the provided bearer token to a request
// unless the authorization header has already been set.
func NewBearerAuthRoundTripper(bearer string, rt http.RoundTripper) http.RoundTripper {
	return &bearerAuthRoundTripper{bearer, nil, rt}
}

// NewBearerAuthWithRefreshRoundTripper adds the provided bearer token to a request
// unless the authorization header has already been set.
// If tokenFile is non-empty, it is periodically read,
// and the last successfully read content is used as the bearer token.
// If tokenFile is non-empty and bearer is empty, the tokenFile is read
// immediately to populate the initial bearer token.
func NewBearerAuthWithRefreshRoundTripper(bearer string, tokenFile string, rt http.RoundTripper) (http.RoundTripper, error) {
	if len(tokenFile) == 0 {
		return &bearerAuthRoundTripper{bearer, nil, rt}, nil
	}
	source := NewCachedFileTokenSource(tokenFile)
	if len(bearer) == 0 {
		token, err := source.Token()
		if err != nil {
			return nil, err
		}
		bearer = token.AccessToken
	}
	return &bearerAuthRoundTripper{bearer, source, rt}, nil
}

func (rt *bearerAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}

	req = utilnet.CloneRequest(req)
	token := rt.bearer
	if rt.source != nil {
		if refreshedToken, err := rt.source.Token(); err == nil {
			token = refreshedToken.AccessToken
		}
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.rt.RoundTrip(req)
}

func (rt *bearerAuthRoundTripper) CancelRequest(req *http.Request) {
	tryCancelRequest(rt.WrappedRoundTripper(), req)
}

func (rt *bearerAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }
```

```go

// WrapperFunc wraps an http.RoundTripper when a new transport
// is created for a client, allowing per connection behavior
// to be injected.
type WrapperFunc func(rt http.RoundTripper) http.RoundTripper

// Wrappers accepts any number of wrappers and returns a wrapper
// function that is the equivalent of calling each of them in order. Nil
// values are ignored, which makes this function convenient for incrementally
// wrapping a function.
func Wrappers(fns ...WrapperFunc) WrapperFunc {
	if len(fns) == 0 {
		return nil
	}
	// optimize the common case of wrapping a possibly nil transport wrapper
	// with an additional wrapper
	if len(fns) == 2 && fns[0] == nil {
		return fns[1]
	}
	return func(rt http.RoundTripper) http.RoundTripper {
		base := rt
		for _, fn := range fns {
			if fn != nil {
				base = fn(base)
			}
		}
		return base
	}
}
```




```go

type tlsCacheKey struct {
	insecure           bool
	caData             string
	certData           string
	keyData            string `datapolicy:"security-key"`
	certFile           string
	keyFile            string
	serverName         string
	nextProtos         string
	disableCompression bool
}

func (c *tlsTransportCache) get(config *Config) (http.RoundTripper, error) {
    //将config更改为需要hash的键，做缓存
	key, canCache, err := tlsConfigKey(config)
	if err != nil {
		return nil, err
	}
	//查看此config是否可以的transport是否可以做cache
	if canCache {
		// Ensure we only create a single transport for the given TLS options
		c.mu.Lock()
		defer c.mu.Unlock()

		// See if we already have a custom transport for this config
		//从缓存获取
		if t, ok := c.transports[key]; ok {
			return t, nil
		}
	}

	// Get the TLS options for this client config
	// 获取TLs option
	tlsConfig, err := TLSConfigFor(config)
	if err != nil {
		return nil, err
	}
	// The options didn't require a custom TLS config
	if tlsConfig == nil && config.Dial == nil && config.Proxy == nil {
		return http.DefaultTransport, nil
	}

    //建立连接方式
	dial := config.Dial
	if dial == nil {
		dial = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	// If we use are reloading files, we need to handle certificate rotation properly
	// TODO(jackkleeman): We can also add rotation here when config.HasCertCallback() is true
	if config.TLS.ReloadTLSFiles {
	    //
		dynamicCertDialer := certRotatingDialer(tlsConfig.GetClientCertificate, dial)
		//每次建立连接，获取证书都调用动态获取器，换成动态获取器，当重新建立连接时，可以检查证书，当证书异常时，关闭掉所有连接
		tlsConfig.GetClientCertificate = dynamicCertDialer.GetClientCertificate
		//dynamicCertDialer.connDialer 是一个连接管理器，具备一次性关闭所有连接的能力，主要应对证书变化时，可以一次性关闭所有已有的连接，然后重新建立连接
		dial = dynamicCertDialer.connDialer.DialContext
		//开始watch cert的动态更新
		go dynamicCertDialer.Run(wait.NeverStop)
	}

	proxy := http.ProxyFromEnvironment
	if config.Proxy != nil {
		proxy = config.Proxy
	}

	transport := utilnet.SetTransportDefaults(&http.Transport{
		Proxy:               proxy,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig,
		MaxIdleConnsPerHost: idleConnsPerHost,
		DialContext:         dial,
		DisableCompression:  config.DisableCompression,
	})

	if canCache {
		// Cache a single transport for these options
		c.transports[key] = transport
	}

	return transport, nil
}

```


```go
// run starts the controller and blocks until stopCh is closed.
func (c *dynamicClientCert) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.V(3).Infof("Starting client certificate rotation controller")
	defer klog.V(3).Infof("Shutting down client certificate rotation controller")

    //每秒执行一次，直到stopCh收到信号
	go wait.Until(c.runWorker, time.Second, stopCh)

    //每秒向队列扔一个对象，这样定时更新cert
	go wait.PollImmediateUntil(CertCallbackRefreshDuration, func() (bool, error) {
		c.queue.Add(workItemKey)
		return false, nil
	}, stopCh)

	<-stopCh
}

func (c *dynamicClientCert) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *dynamicClientCert) processNextWorkItem() bool {
    //获取对象，如果没有对象会阻塞
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	//从队列中删除对象
	defer c.queue.Done(dsKey)

	_, err := c.loadClientCert()
	if err == nil {
	    //清除限流器对于dsKey的记数
		c.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	//增加限流器计数
	c.queue.AddRateLimited(dsKey)

	return true
}

```


```go
// loadClientCert calls the callback and rotates connections if needed
func (c *dynamicClientCert) loadClientCert() (*tls.Certificate, error) {
    //调用tls配置中的加载方法
	cert, err := c.reload(nil)
	if err != nil {
		return nil, err
	}

	// check to see if we have a change. If the values are the same, do nothing.
	c.certMtx.RLock()
	haveCert := c.clientCert != nil
	//证书一致，不需要关闭所有连接，重新建立连接
	if certsEqual(c.clientCert, cert) {
		c.certMtx.RUnlock()
		return c.clientCert, nil
	}
	c.certMtx.RUnlock()

	c.certMtx.Lock()
	c.clientCert = cert
	c.certMtx.Unlock()

	// The first certificate requested is not a rotation that is worth closing connections for
	// 第一次获取连接，没有其他连接需要关闭
	if !haveCert {
		return cert, nil
	}

	klog.V(1).Infof("certificate rotation detected, shutting down client connections to start using new credentials")
	c.connDialer.CloseAll()

	return cert, nil
}
```
