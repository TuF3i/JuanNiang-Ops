package onebot

import (
	"log"
	"net"
	"sync"

	"github.com/lxzan/gws"
)

type Config struct {
	Addr   string // 监听地址, 格式 host:port
	Port   int
	Token  string
	Admin  []string
	Enable bool
}

type Peer struct {
	Addr   string
	SelfID string
}

type Adapter struct {
	cfg      Config
	peers    map[string]Peer
	server   *Server
	listener *net.Listener
	mu       sync.Mutex
	enable   bool
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{cfg: cfg, peers: make(map[string]Peer)}
}

func (r *Adapter) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	listener, err := net.Listen("tcp", r.cfg.Addr)
	if err != nil {
		return err
	}

	r.listener = &listener

	r.server = new(Server)

	wsLoop := gws.NewServer(r.server, nil)
	go func() {
		if err := wsLoop.RunListener(listener); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	r.enable = true

	return nil
}

func (r *Adapter) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return nil
}
