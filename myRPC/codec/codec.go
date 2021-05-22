package codec

import "io"

// Header 请求和响应的头部信息
type Header struct {
	// 服务名和方法名
	// format "Service.Method"
	ServiceMethod string
	// 请求的序号
	Seq uint64
	// 错误信息
	Error string
}

// 消息编码解码接口
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCodeFunc func(closer io.ReadWriteCloser) Codec

type Type string

const (
	GobType  Type = "application/gob" // not implemented
	JsonType Type = "application/json"
)

var NewCodeFuncMap map[Type]NewCodeFunc

func init() {
	NewCodeFuncMap = make(map[Type]NewCodeFunc)
	NewCodeFuncMap[JsonType] = NewJsonCodec
}
