package myRPC

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"rpc/myRPC/codec"
	"sync"
	"time"
)

var ErrShutdown = errors.New("connection is shut down")

// Call represents an active RPC.
type Call struct {
	Seq           uint64
	ServiceMethod string      // format "Service.Method"
	Args          interface{} // The argument to the function (*struct).
	Reply         interface{} // The reply from the function (*struct).
	Error         error       // After completion, the error status.
	Done          chan *Call  // Receives *Call when Go is complete.
}

func (call *Call) done() {
	select {
	case call.Done <- call:
	default:
		// return if block. I think it can not block.
	}
}

// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	cc codec.Codec

	//opt     *Option // set option,i think it may necessary
	sending sync.Mutex // using in send()

	mu       sync.Mutex
	seq      uint64
	pending  map[uint64]*Call
	closing  bool // user has called Close
	shutdown bool // server has told us to stop
}

// IsAvailable return true if the client does work
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}

var _ io.Closer = (*Client)(nil)

// Close calls the underlying codec's Close method. If the connection is already
// shutting down, ErrShutdown is returned.
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

// register Register this call.
func (client *Client) register(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}

	call.Seq = client.seq
	client.seq++
	client.pending[call.Seq] = call
	return call.Seq, nil
}

// removeCall remove Call from pending according seq
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// terminalCall terminal if error
func (client *Client) terminalCall(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

// receive receive reply from server
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		err = client.cc.ReadHeader(&h)
		if err != nil {
			log.Println("rpc: receive: err:", err)
			break
		}

		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			// We've got no pending call. That usually means that
			// WriteRequest partially failed, and call was already
			// removed; response is a server telling us about an
			// error reading request body. We should still attempt
			// to read error body, but there's no one to give it to.
			err = client.cc.ReadBody(nil)
			if err != nil {
				err = errors.New("receive: reading body: " + err.Error())
			}
		case h.Error != "":
			// We've got an error response. Give this to the request;
			// any subsequent requests will get the ReadResponseBody
			// error if there is one.
			call.Error = errors.New(h.Error)
			err = client.cc.ReadBody(nil)
			if err != nil {
				err = errors.New("receive: reading body: " + err.Error())
			}
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("receive: reading body " + err.Error())
			}
			call.done()
		}
	}
	// Terminate pending calls.
	client.terminalCall(err)
}

// NewClient return a new client with default option
func NewClient(conn net.Conn) (*Client, error) {
	return NewClientWithOption(conn, &DefaultOption)
}

func NewClientWithOption(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodeFuncMap[opt.CodeType]
	if f == nil {
		return nil, errors.New("unknown code type")
	}

	err := json.NewEncoder(conn).Encode(opt)
	if err != nil {
		return nil, err
	}
	newClient := &Client{
		cc:       f(conn),
		sending:  sync.Mutex{},
		mu:       sync.Mutex{},
		seq:      1,
		pending:  map[uint64]*Call{},
		closing:  false,
		shutdown: false,
	}
	// start goroutine to receive reply from server
	go newClient.receive()
	return newClient, nil
}

// Dial connects to an RPC server at the specified network address.
func Dial(network, address string, opts ...*Option) (*Client, error) {
	//parts := strings.Split(address, "@")
	////if len(parts) != 2 {
	////	return nil, fmt.Errorf("rpc client err: wrong format '%s', expect protocol@addr", address)
	////}
	//protocol, addr := parts[0], parts[1]
	//switch protocol {
	//case "http":
	//	return DialHTTP("tcp", addr)
	//default:
	//	// tcp, unix or other transport protocol
	//	//return Dial(protocol, addr, opts...)
	//}
	return dialWithTimeout(network, address, opts...)
}

// dialWithTimeout connects to an RPC server and with timeout
func dialWithTimeout(network, address string, opts ...*Option) (client *Client, err error) {
	var conn net.Conn
	var tmpOpt *Option

	if len(opts) == 0 {
		tmpOpt = &DefaultOption
	} else if len(opts) == 1 {
		tmpOpt = opts[0]
	} else {
		err = errors.New("opts is too much,need one")
		return
	}

	// if connection is successful, send struct{}{}
	ch := make(chan struct{}, 1)

	go func() {
		// try to connection with timeout
		conn, err = net.DialTimeout(network, address, tmpOpt.ConnectionTimeout)
		ch <- struct{}{}
	}()

	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	if err != nil {
		return
	}

	select {
	case <-time.After(tmpOpt.ConnectionTimeout):
		err = errors.New("rpc: connect timeout")
		return
	case <-ch:
		client, err = NewClientWithOption(conn, tmpOpt)
		return
	}

}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	seq, err := client.register(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	var h codec.Header
	h.Seq = seq
	h.ServiceMethod = call.ServiceMethod

	err = client.cc.Write(&h, call.Args)
	if err != nil {
		call := client.removeCall(h.Seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go invokes the function asynchronously. It returns the Call structure representing
// the invocation. The done channel will signal when the call is complete by returning
// the same Call object. If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
func (client *Client) Go(serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call {
	call := new(Call)
	call.ServiceMethod = serviceMethod
	call.Args = args
	call.Reply = reply
	if done == nil {
		done = make(chan *Call, 1)
	}
	call.Done = done
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete, and returns its error status.
func (client *Client) Call(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("rpc: Call: " + ctx.Err().Error())
	case <-call.Done:
		return call.Error
	}
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string) (*Client, error) {
	return DialHTTPPath(network, address, DefaultRPCPath)
}

// DialHTTPPath connects to an HTTP RPC server
// at the specified network address and path.
func DialHTTPPath(network, address, path string) (*Client, error) {
	var err error
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		client, err := NewClient(conn)
		if err != nil {
			return nil, err
		}
		return client, nil
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
