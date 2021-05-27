# 支持Http协议

Web 开发中，我们经常使用 HTTP 协议中的 HEAD、GET、POST 等方式发送请求，等待响应。但 RPC 的消息格式与标准的 HTTP 协议并不兼容，在这种情况下，就需要一个协议的转换过程。HTTP 协议的 CONNECT 方法恰好提供了这个能力，CONNECT 一般用于代理服务。

假设浏览器与服务器之间的 HTTPS 通信都是加密的，浏览器通过代理服务器发起 HTTPS 请求时，由于请求的站点地址和端口号都是加密保存在 HTTPS 请求报文头中的，代理服务器如何知道往哪里发送请求呢？为了解决这个问题，浏览器通过 HTTP 明文形式向代理服务器发送一个 CONNECT 请求告诉代理服务器目标地址和端口，代理服务器接收到这个请求后，会在对应端口与目标站点建立一个 TCP 连接，连接建立成功后返回 HTTP 200 状态码告诉浏览器与该站点的加密通道已经完成。接下来代理服务器仅需透传浏览器和服务器之间的加密数据包即可，代理服务器无需解析 HTTPS 报文。

举一个简单例子：

1. 浏览器向代理服务器发送 CONNECT 请求。

```
CONNECT geektutu.com:443 HTTP/1.0
```

2.代理服务器返回 HTTP 200 状态码表示连接已经建立。

```
HTTP/1.0 200 Connection Established
```

3.之后浏览器和服务器开始 HTTPS 握手并交换加密数据，代理服务器只负责传输彼此的数据包，并不能读取具体数据内容（代理服务器也可以选择安装可信根证书解密 HTTPS 报文）。

事实上，这个过程其实是通过代理服务器将 HTTP 协议转换为 HTTPS 协议的过程。对 RPC 服务端来，需要做的是将 HTTP 协议转换为 RPC 协议，对客户端来说，需要新增通过 HTTP CONNECT 请求创建连接的逻辑。



## 服务端支持Http协议

1. 客户端向 RPC 服务器发送 CONNECT 请求
2. RPC 服务器返回 HTTP 200 状态码表示连接建立。
3. 客户端使用创建好的连接发送 RPC 报文，先发送 Option，再发送 N 个请求报文，服务端处理 RPC 请求并响应。



在 `server.go` 中新增如下的方法：

```go
const (
   // Can connect to RPC service using HTTP CONNECT to rpcPath.
   connected = "200 Connected to Go RPC"
   // Defaults used by HandleHTTP
   DefaultRPCPath   = "/_goRPC_"
   DefaultDebugPath = "/debug/rpc"
)
```

```go
// ServeHTTP implements an http.Handler that answers RPC requests.
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
   if req.Method != "CONNECT" {
      w.Header().Set("Content-Type", "text/plain; charset=utf-8")
      w.WriteHeader(http.StatusMethodNotAllowed)
      io.WriteString(w, "405 must CONNECT\n")
      return
   }
   conn, _, err := w.(http.Hijacker).Hijack()
   if err != nil {
      log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
      return
   }
   io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
   server.ServerConn(conn)
}

// HandleHTTP registers an HTTP handler for RPC messages on rpcPath,
// and a debugging handler on debugPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func (server *Server) HandleHTTP(rpcPath, debugPath string) {
   http.Handle(rpcPath, server)
   http.Handle(debugPath, debugHTTP{server})
}

// HandleHTTP registers an HTTP handler for RPC messages to DefaultServer
// on DefaultRPCPath and a debugging handler on DefaultDebugPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func HandleHTTP() {
   DefaultServer.HandleHTTP(DefaultRPCPath, DefaultDebugPath)
}
```



`defaultDebugPath` 是为后续 DEBUG 页面预留的地址。



### debug 页面处理

```go
const debugText = `<html>
   <body>
   <title>Services</title>
   {{range .}}
   <hr>
   Service {{.Name}}
   <hr>
      <table>
      <th align=center>Method</th><th align=center>Calls</th>
      {{range .Method}}
         <tr>
         <td align=left font=fixed>{{.Name}}({{.Type.ArgType}}, {{.Type.ReplyType}}) error</td>
         <td align=center>{{.Type.NumCalls}}</td>
         </tr>
      {{end}}
      </table>
   {{end}}
   </body>
   </html>`

var debug = template.Must(template.New("RPC debug").Parse(debugText))

// If set, print log statements for internal and I/O errors.
var debugLog = false

type debugMethod struct {
   Type *methodType
   Name string
}

type methodArray []debugMethod

type debugService struct {
   Service *service
   Name    string
   Method  methodArray
}

type serviceArray []debugService

func (s serviceArray) Len() int           { return len(s) }
func (s serviceArray) Less(i, j int) bool { return s[i].Name < s[j].Name }
func (s serviceArray) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (m methodArray) Len() int           { return len(m) }
func (m methodArray) Less(i, j int) bool { return m[i].Name < m[j].Name }
func (m methodArray) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

type debugHTTP struct {
   *Server
}

// Runs at /debug/rpc
func (server debugHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
   // Build a sorted version of the data.
   var services serviceArray
   server.serviceMap.Range(func(snamei, svci interface{}) bool {
      svc := svci.(*service)
      ds := debugService{svc, snamei.(string), make(methodArray, 0, len(svc.method))}
      for mname, method := range svc.method {
         ds.Method = append(ds.Method, debugMethod{method, mname})
      }
      sort.Sort(ds.Method)
      services = append(services, ds)
      return true
   })
   sort.Sort(services)
   err := debug.Execute(w, services)
   if err != nil {
      fmt.Fprintln(w, "rpc: error executing template:", err.Error())
   }
}
```



## 客户端支持Http

服务端已经能够接受 CONNECT 请求，并返回了 200 状态码 ，客户端要做的，发起 CONNECT 请求，检查返回状态码即可成功建立连接。

```go
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
```

通过 HTTP CONNECT 请求建立连接之后，后续的通信过程就交给 NewClient 了。



## 运行

```go
const Addr = "localhost:50001"

func server(addr string) {
   lis, err := net.Listen("tcp", addr)
   if err != nil {
      return
   }
   err = myRPC.Register(&Foo{})
   if err != nil {
      log.Println(err)
   }
   myRPC.HandleHTTP()
   http.Serve(lis, nil)
}

func main() {

   go call()

   // 启动服务端
   server(Addr)

   // 通过网络服务调用，打开后查看在浏览器输入"localhost:50001/debug/rpc"
}

func call() {
   // 新建客户端
   client, err := myRPC.DialHTTP("tcp", Addr)
   if err != nil {
      log.Println(err)
      return
   }

   time.Sleep(1 * time.Second)
   // send request & receive response
   for i := 0; i < 5; i++ {
      ServiceMethod := "Foo.Add"
      args := Args{
         A: i,
         B: i + 1,
      }
      var reply Reply
      err := client.Call(context.Background(), ServiceMethod, &args, &reply)
      if err != nil {
         log.Println(err)
      }

      log.Println("reply:", reply)
   }
}

type Foo struct {
}

type Args struct {
   A, B int
}

type Reply struct {
   Val int
}

func (foo *Foo) Add(args *Args, reply *Reply) error {
   a, b := args.A, args.B
   reply.Val = a + b
   return nil
}
```



打开网页查看效果

Service Foo

------

| Method                             | Calls |
| ---------------------------------- | ----- |
| Add(*main.Args, *main.Reply) error | 5     |

