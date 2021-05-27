package xclient

import (
	"context"
	"io"
	"reflect"
	"rpc/myRPC"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *myRPC.Option
	mu      sync.Mutex // protect following
	clients map[string]*myRPC.Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *myRPC.Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, mu: sync.Mutex{}, clients: map[string]*myRPC.Client{}}
}

func (X *XClient) Close() error {
	X.mu.Lock()
	defer X.mu.Unlock()
	for key, client := range X.clients {
		_ = client.Close()
		delete(X.clients, key)
	}
	return nil
}

func (X *XClient) dial(addr string) (*myRPC.Client, error) {
	X.mu.Lock()
	defer X.mu.Unlock()

	client, ok := X.clients[addr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(X.clients, addr)
		client = nil
	}

	if client == nil {
		var err error
		client, err = myRPC.Dial("tcp", addr)
		if err != nil {
			return nil, err
		}
		X.clients[addr] = client
	}
	return client, nil
}

func (X *XClient) call(addr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := X.dial(addr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// xc will choose a proper server.
func (X *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := X.d.Get(X.mode)
	if err != nil {
		return err
	}
	return X.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast invokes the named function for every server registered in discovery
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex // protect e and replyDone
	var e error
	replyDone := reply == nil // if reply is nil, don't need to set value
	ctx, cancel := context.WithCancel(ctx)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // if any call failed, cancel unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
