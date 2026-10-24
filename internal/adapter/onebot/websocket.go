package onebot

import (
	"sync"
	"uuid"

	"github.com/lxzan/gws"
)

type Server struct {
	gws.BuiltinEventHandler
	rootController *Adapter
	mu             sync.RWMutex
	conns          map[*gws.Conn]string
	RemoteAddr     string
	SelfID         string
}

func NewWsServer(rootController *Adapter) *Server {
	return &Server{
		rootController: rootController,
		mu:             sync.RWMutex{},
		conns:          make(map[*gws.Conn]string),
	}
}

func (r *Server) OnOpen(socket *gws.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	uuid := uuid.NewV4().String()
	r.conns[socket] = uuid
}

func (r *Server) OnClose(socket *gws.Conn, _ error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.conns, socket)
}

func (r *Server) OnMessage(socket *gws.Conn, message *gws.Message) {

}
