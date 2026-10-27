package onebot

import (
	"github.com/lxzan/gws"
)

// Server gws 事件处理器, 负责连接事件与原始消息的接收
type Server struct {
	gws.BuiltinEventHandler
	root *Adapter
}

// NewWsServer 创建 gws 事件处理器
func NewWsServer(root *Adapter) *Server {
	return &Server{root: root}
}

// OnOpen 连接建立
func (s *Server) OnOpen(socket *gws.Conn) {
	s.root.onConnOpen(socket)
}

// OnClose 连接关闭
func (s *Server) OnClose(socket *gws.Conn, err error) {
	s.root.onConnClose(socket, err)
}

// OnMessage 收到原始上报数据
func (s *Server) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	s.root.onPayload(socket, message.Bytes())
}
