// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"bufio"
	"encoding/gob"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
)

// ServerError represents an error that has been returned from
// the remote side of the RPC connection.
type ServerError string

func (e ServerError) Error() string {
	return string(e)
}

var ErrShutdown = errors.New("connection is shut down")

// Call represents an active RPC.
// Call 代表一个活动的 RPC
type Call struct {
	ServiceMethod string      // The name of the service and method to call. 调用的 服务.方法 名称
	Args          interface{} // The argument to the function (*struct).     函数参数
	Reply         interface{} // The reply from the function (*struct).      函数返回
	Error         error       // After completion, the error status.		 完成后，错误状态
	Done          chan *Call  // Receives *Call when Go is complete.         Go 完成时，接收 *Call
}

// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
//
// Client 代表一个 RPC 客户端。
// 一个客户端可能有多个未完成的调用，一个客户端可能同时被多个 goroutine 使用。
type Client struct {
	codec ClientCodec

	reqMutex sync.Mutex // protects following
	request  Request

	mutex    sync.Mutex // protects following
	seq      uint64
	pending  map[uint64]*Call
	closing  bool // user has called Close         客户端主动关闭 func (client *Client) Close() error
	shutdown bool // server has told us to stop
}

// A ClientCodec implements writing of RPC requests and
// reading of RPC responses for the client side of an RPC session.
// The client calls WriteRequest to write a request to the connection
// and calls ReadResponseHeader and ReadResponseBody in pairs
// to read responses. The client calls Close when finished with the
// connection. ReadResponseBody may be called with a nil
// argument to force the body of the response to be read and then
// discarded.
// See NewClient's comment for information about concurrent access.
//
// ClientCodec 为 RPC 会话的客户端实现 RPC 请求的写入和 RPC 响应的读取。
//
// 客户端调用 WriteRequest 向连接写入请求，并成对调用 ReadResponseHeader 和 ReadResponseBody 读取响应。
// 连接完成后，客户端调用 Close。
//
// 可以使用 nil 参数调用 ReadResponseBody 以强制读取响应的主体，然后将其丢弃。
//
// 有关并发访问的信息，请参阅 NewClient 的注释。
type ClientCodec interface {
	WriteRequest(*Request, interface{}) error
	ReadResponseHeader(*Response) error
	ReadResponseBody(interface{}) error

	Close() error
}

func (client *Client) send(call *Call) {
	client.reqMutex.Lock()
	defer client.reqMutex.Unlock()

	// Register this call.
	client.mutex.Lock()
	if client.shutdown || client.closing {
		client.mutex.Unlock()
		call.Error = ErrShutdown
		call.done()
		return
	}
	seq := client.seq
	client.seq++
	client.pending[seq] = call
	client.mutex.Unlock()

	// Encode and send the request.
	client.request.Seq = seq
	client.request.ServiceMethod = call.ServiceMethod
	err := client.codec.WriteRequest(&client.request, call.Args)
	if err != nil {
		client.mutex.Lock()
		call = client.pending[seq]
		delete(client.pending, seq)
		client.mutex.Unlock()
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// NewClientWithCodec 启动一个协程调用该方法,来接收 rpc 调用的响应
func (client *Client) input() {
	var err error
	var response Response
	for err == nil {
		response = Response{}
		// 读取响应头
		err = client.codec.ReadResponseHeader(&response)
		if err != nil {
			break
		}

		// 获取rpc请求序列化号
		seq := response.Seq

		client.mutex.Lock()
		// 根据响应带回来的序列号, 从 pending 中获取对应的 call
		// 每一个 call 代表一次正在进行的 rpc 请求
		call := client.pending[seq]
		delete(client.pending, seq)
		client.mutex.Unlock()

		switch {
		case call == nil:
			// We've got no pending call. That usually means that
			// WriteRequest partially failed, and call was already
			// removed; response is a server telling us about an
			// error reading request body. We should still attempt
			// to read error body, but there's no one to give it to.
			// 我们没有待处理的call。
			// 这通常意味着 WriteRequest 部分失败，并且 call 已被删除；
			// response 是服务器告诉我们读取 请求body 的错误。
			// 我们仍然应该尝试读取 erro body，但不能将其交给任何人。
			err = client.codec.ReadResponseBody(nil)
			if err != nil {
				err = errors.New("reading error body: " + err.Error())
			}
		case response.Error != "":
			// We've got an error response. Give this to the request;
			// any subsequent requests will get the ReadResponseBody
			// error if there is one.
			// 我们收到了错误响应。 将此交给请求；
			// 如果有任何后续请求，则会收到 ReadResponseBody 错误。
			call.Error = ServerError(response.Error)
			err = client.codec.ReadResponseBody(nil)
			if err != nil {
				err = errors.New("reading error body: " + err.Error())
			}
			call.done()
		default:
			err = client.codec.ReadResponseBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}

	// Terminate pending calls.
	client.reqMutex.Lock()
	client.mutex.Lock()
	client.shutdown = true
	closing := client.closing
	if err == io.EOF {
		if closing {
			err = ErrShutdown
		} else {
			err = io.ErrUnexpectedEOF
		}
	}
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
	client.mutex.Unlock()
	client.reqMutex.Unlock()
	if debugLog && err != io.EOF && !closing {
		log.Println("rpc: client protocol error:", err)
	}
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		// ok
	default:
		// We don't want to block here. It is the caller's responsibility to make
		// sure the channel has enough buffer space. See comment in Go().
		if debugLog {
			log.Println("rpc: discarding Call reply due to insufficient Done chan capacity")
		}
	}
}

// NewClient returns a new Client to handle requests to the
// set of services at the other end of the connection.
// It adds a buffer to the write side of the connection so
// the header and payload are sent as a unit.
//
// The read and write halves of the connection are serialized independently,
// so no interlocking is required. However each half may be accessed
// concurrently so the implementation of conn should protect against
// concurrent reads or concurrent writes.
//
// NewClient 返回一个新的 Client 来处理对连接另一端的服务集的请求。
// 它向连接的写入端添加一个缓冲区，因此标头和有效负载作为一个单元发送。
//
// 连接的读写部分是独立序列化的，所以不需要互锁。 然而，每一半都可以同时访问，因此 conn 的实现应该防止并发读取或并发写入。
func NewClient(conn io.ReadWriteCloser) *Client {
	encBuf := bufio.NewWriter(conn)
	client := &gobClientCodec{conn, gob.NewDecoder(conn), gob.NewEncoder(encBuf), encBuf}
	return NewClientWithCodec(client)
}

// NewClientWithCodec is like NewClient but uses the specified
// codec to encode requests and decode responses.
func NewClientWithCodec(codec ClientCodec) *Client {
	client := &Client{
		codec:   codec,
		pending: make(map[uint64]*Call),
	}
	go client.input()
	return client
}

type gobClientCodec struct {
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer
}

func (c *gobClientCodec) WriteRequest(r *Request, body interface{}) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return
	}
	if err = c.enc.Encode(body); err != nil {
		return
	}
	return c.encBuf.Flush()
}

func (c *gobClientCodec) ReadResponseHeader(r *Response) error {
	return c.dec.Decode(r)
}

func (c *gobClientCodec) ReadResponseBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *gobClientCodec) Close() error {
	return c.rwc.Close()
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string) (*Client, error) {
	return DialHTTPPath(network, address, DefaultRPCPath)
}

// DialHTTPPath connects to an HTTP RPC server
// at the specified network address and path.
func DialHTTPPath(network, address, path string) (*Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		return NewClient(conn), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  network + " " + address,
		Addr: nil,
		Err:  err,
	}
}

// Dial connects to an RPC server at the specified network address.
// Dial 连接到指定网络地址的 RPC 服务器。
func Dial(network, address string) (*Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), nil
}

// Close calls the underlying codec's Close method. If the connection is already
// shutting down, ErrShutdown is returned.
//
// Close 调用底层编解码器的 Close 方法。 如果连接已经关闭，则返回 ErrShutdown。
func (client *Client) Close() error {
	client.mutex.Lock()
	if client.closing {
		client.mutex.Unlock()
		return ErrShutdown
	}
	client.closing = true
	client.mutex.Unlock()
	return client.codec.Close()
}

// Go invokes the function asynchronously. It returns the Call structure representing
// the invocation. The done channel will signal when the call is complete by returning
// the same Call object. If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
//
// Go 异步调用函数。
// 它返回表示调用的 Call 结构。
// done 通道将通过返回相同的 Call 对象 在调用完成时发出信号。
// 如果 done 为 nil，Go 将分配一个新通道。 如果非零，done 必须被缓冲，否则 Go 会故意崩溃。
func (client *Client) Go(serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call {
	call := new(Call)
	call.ServiceMethod = serviceMethod
	call.Args = args
	call.Reply = reply
	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		// If caller passes done != nil, it must arrange that
		// done has enough buffer for the number of simultaneous
		// RPCs that will be using that channel. If the channel
		// is totally unbuffered, it's best not to run at all.
		//
		// 如果调用者传递了 done != nil，它必须安排 done 有足够的缓冲区来容纳将使用该通道的同时 RPC 的数量。
		// 如果通道完全没有缓冲，最好不要运行。
		if cap(done) == 0 {
			log.Panic("rpc: done channel is unbuffered")
		}
	}
	call.Done = done
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete, and returns its error status.
// Call 调用命名函数，等待它完成，并返回其错误状态。
func (client *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
