package myRPC

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"rpc/myRPC/codec"
	"strings"
	"sync"
	"time"
)

// !+ Coding format negotiation
const MagicNumber = 0x3bef5c

var invalidRequest = struct{}{}

type Option struct {
	MagicNumber       int           // MagicNumber marks this's a geerpc request
	CodeType          codec.Type    // client may choose different Codec to encode body
	ConnectionTimeout time.Duration // 0 means no limit
	HandleTimeout     time.Duration
}

var DefaultOption = Option{
	MagicNumber:       MagicNumber,
	CodeType:          codec.JsonType,
	ConnectionTimeout: 10 * time.Second,
}

// !+ implement server

// // Server represents an RPC Server.
type Server struct {
	serviceMap sync.Map // map[string]*service
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("server: accept: %v", err)
		}
		go server.ServerConn(conn)
	}
}

// Accept accepts connections on the listener and serves requests
// to DefaultServer for each incoming connection.
// Accept blocks; the caller typically invokes it in a go statement.
func Accept(lis net.Listener) { DefaultServer.Accept(lis) }

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
func (server *Server) ServerConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Printf("server: ServerConn: decode option eror:%v", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("server: ServerConn: Unknown MagicNumber:%v", opt.MagicNumber)
		return
	}
	f := codec.NewCodeFuncMap[opt.CodeType]
	if f == nil {
		log.Printf("server: ServerConn: Unknown Codec type:%v", opt.CodeType)
		return
	}
	server.serverCodec(f(conn), &opt)
}

// serverCodec is like ServeConn but uses the specified codec to
// decode requests and encode responses.
func (server *Server) serverCodec(cc codec.Codec, opt *Option) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
		continue
	}
	// We've seen that there are no more requests.
	// Wait for responses to be sent before closing codec.
	wg.Wait()
	_ = cc.Close()
}

// request stores all information of a call
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
	svc          *service
	mtype        *methodType
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			log.Println("server: read header error:", err)
		}
		err = errors.New("server cannot decode request: " + err.Error())
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}

	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	if err != nil {
		return nil, err
	}

	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	// make sure that argvi is a pointer, ReadBody need a pointer as parameter
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	err = cc.ReadBody(argvi)
	if err != nil {
		log.Println("rpc server: read argv err:", err)
		return nil, err
	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	err := cc.Write(h, body)
	if err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{}, 1)
	sent := make(chan struct{}, 1)
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()

	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		server.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

// Register publishes in the server the set of methods of the
func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}

	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}

// findService Find services and methods according to ServiceMethod
func (server *Server) findService(ServiceMethod string) (*service, *methodType, error) {
	dot := strings.LastIndex(ServiceMethod, ".")
	if dot < 0 {
		err := errors.New("rpc: service/method request ill-formed: " + ServiceMethod)
		return nil, nil, err
	}

	serviceName := ServiceMethod[:dot]
	methodName := ServiceMethod[dot+1:]

	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err := errors.New("rpc: can't find service " + ServiceMethod)
		return nil, nil, err
	}

	svc := svci.(*service)
	mtype := svc.method[methodName]
	if mtype == nil {
		err := errors.New("rpc: can't find method " + ServiceMethod)
		return nil, nil, err
	}
	return svc, mtype, nil
}
