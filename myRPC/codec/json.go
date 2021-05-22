package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

// 使用编译器来检测 *JsonCodec 是否实现了 Codec 接口
var _ Codec = (*JsonCodec)(nil)

type JsonCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	enc  *json.Encoder
	dec  *json.Decoder
}

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf:  buf,
		enc:  json.NewEncoder(buf),
		dec:  json.NewDecoder(conn),
	}
}

func (c *JsonCodec) Close() error {
	return c.conn.Close()
}

func (c *JsonCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *JsonCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *JsonCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		// 使得缓存的内容写入conn
		_ = c.buf.Flush()
		// 有错误就关闭链接
		if err != nil {
			_ = c.Close()
		}
	}()

	if err := c.enc.Encode(h); err != nil {
		log.Printf("codec json: json can not encoding header: %v", err)
		return err
	}
	if err := c.enc.Encode(body); err != nil {
		log.Printf("codec json: json can not encoding body: %v", err)
		return err
	}
	return nil
}
