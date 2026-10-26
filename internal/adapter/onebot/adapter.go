package onebot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lxzan/gws"

	"JuanNiang-Ops/internal/adapter/common"
)

// Config OneBot 正向 WebSocket 适配器配置
type Config struct {
	Addr   string   // 监听地址, 格式 host:port
	Token  string   // 接入令牌, 为空则不校验
	Admin  []string // 管理员列表
	Enable bool     // 是否启用

	MessageBuffer int // 接收消息管道缓冲大小, 默认 256
	ActionBuffer  int // 发送指令管道缓冲大小, 默认 64
}

// 适配器状态与路由错误
var (
	ErrDisabled      = errors.New("onebot: adapter is disabled")
	ErrRunning       = errors.New("onebot: adapter is already running")
	ErrNotRunning    = errors.New("onebot: adapter is not running")
	ErrNoPeer        = errors.New("onebot: no client connected")
	ErrUnknownPeer   = errors.New("onebot: target client not connected")
	ErrAmbiguousPeer = errors.New("onebot: multiple clients connected, target required")
)

// Action 发送给 OneBot 实现端的 API 动作
type Action struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Echo   string         `json:"echo,omitempty"`

	Target string `json:"-"` // 目标机器人 self_id, 为空时自动路由
}

// ApiResult OneBot API 调用回执
type ApiResult struct {
	Status  string          `json:"status"`  // ok / async / failed
	RetCode int             `json:"retcode"` // 0 表示成功
	Data    json.RawMessage `json:"data"`    // 回执数据
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

// peer 一个接入的 OneBot 客户端 (机器人端)
type peer struct {
	conn     *gws.Conn
	session  string // 接入时分配的会话 ID, 上报 self_id 前作为占位
	selfID   string // 机器人自身 ID, 首个事件上报后绑定
	lastBeat time.Time
}

// Adapter OneBot 正向 WebSocket 适配器.
// 内部维护两条管道:
//   - inbound:  接收消息管道, OneBot 事件 -> 业务层
//   - outbound: 发送指令管道, 业务层 -> OneBot 实现端
type Adapter struct {
	cfg Config

	inbound  chan common.Message
	outbound chan *Action

	mu    sync.RWMutex
	peers map[string]*peer    // self_id -> peer
	conns map[*gws.Conn]*peer // conn -> peer

	listener net.Listener
	upgrader *gws.Upgrader
	handler  *Server

	running bool
	done    chan struct{}
	wg      sync.WaitGroup

	callMu  sync.Mutex
	pending map[string]chan *ApiResult
}

// NewAdapter 创建适配器, 调用 Start 后管道才会建立
func NewAdapter(cfg Config) *Adapter {
	if cfg.MessageBuffer <= 0 {
		cfg.MessageBuffer = 256
	}
	if cfg.ActionBuffer <= 0 {
		cfg.ActionBuffer = 64
	}
	return &Adapter{
		cfg:     cfg,
		peers:   make(map[string]*peer),
		conns:   make(map[*gws.Conn]*peer),
		pending: make(map[string]chan *ApiResult),
	}
}

// Start 启动适配器: 监听端口, 建立收发管道. 重复调用返回 ErrRunning.
func (r *Adapter) Start() error {
	if !r.cfg.Enable {
		return ErrDisabled
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return ErrRunning
	}

	listener, err := net.Listen("tcp", r.cfg.Addr)
	if err != nil {
		r.mu.Unlock()
		return fmt.Errorf("onebot: listen %s: %w", r.cfg.Addr, err)
	}

	r.inbound = make(chan common.Message, r.cfg.MessageBuffer)
	r.outbound = make(chan *Action, r.cfg.ActionBuffer)
	r.done = make(chan struct{})
	r.listener = listener
	r.handler = NewWsServer(r)
	r.upgrader = gws.NewUpgrader(r.handler, &gws.ServerOption{
		Authorize: r.authorize,
	})
	r.running = true
	r.mu.Unlock()

	r.wg.Add(2)
	go r.runWebSocket()
	go r.dispatchLoop()

	log.Printf("[onebot] websocket server listening on %s", r.cfg.Addr)
	return nil
}

// Stop 停止适配器: 关闭监听与所有连接, 排空指令管道, 阻塞至完全退出.
// 退出后 Inbound 会被关闭, 消费方可通过 range 退出; 之后的投递会返回 ErrNotRunning.
func (r *Adapter) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	listener := r.listener
	close(r.done)
	r.mu.Unlock()

	// 1. 停止接受新连接
	_ = listener.Close()

	// 2. 断开所有已接入的客户端
	for _, conn := range r.snapshotConns() {
		_ = conn.WriteClose(1000, []byte("server shutdown"))
		_ = conn.NetConn().Close() // 确保对端读循环立刻退出
	}

	// 3. 等待中的 API 调用全部回执失败
	r.failPending(ErrNotRunning)

	// 4. 等待内部 goroutine 退出
	r.wg.Wait()

	// 5. 关闭接收管道, 通知下游; 清空连接表以便下次 Start
	close(r.inbound)
	r.mu.Lock()
	r.peers = make(map[string]*peer)
	r.conns = make(map[*gws.Conn]*peer)
	r.mu.Unlock()

	log.Printf("[onebot] adapter stopped")
	return nil
}

// Done 返回停止信号通道: Stop 时被关闭. 与管道配合使用:
//
//	select {
//	case msg := <-a.Inbound():
//	case <-a.Done():
//	}
func (r *Adapter) Done() <-chan struct{} { return r.done }

// Inbound 接收消息管道 (OneBot 事件 -> 业务层), Start 后可用, Stop 时关闭
func (r *Adapter) Inbound() <-chan common.Message { return r.inbound }

// Outbound 发送指令管道 (业务层 -> OneBot), 只写方向. Start 后可用.
// 适配器停止后其剩余内容会被排空丢弃; 建议优先使用 Send/Call.
func (r *Adapter) Outbound() chan<- *Action { return r.outbound }

// Send 投递一条不需要回执的指令 (fire-and-forget)
func (r *Adapter) Send(action string, params map[string]any) error {
	return r.SendAction(&Action{Action: action, Params: params})
}

// SendAction 投递指令到发送管道, 立即返回; 适配器停止后返回 ErrNotRunning
func (r *Adapter) SendAction(act *Action) error {
	r.mu.RLock()
	running, outbound := r.running, r.outbound
	r.mu.RUnlock()
	if !running || outbound == nil {
		return ErrNotRunning
	}
	select {
	case <-r.done:
		return ErrNotRunning
	case outbound <- act:
		return nil
	}
}

// Call 发送指令并等待 OneBot 回执 (请求-响应模式)
func (r *Adapter) Call(ctx context.Context, target, action string, params map[string]any) (*ApiResult, error) {
	r.mu.RLock()
	running := r.running
	r.mu.RUnlock()
	if !running {
		return nil, ErrNotRunning
	}

	echo := uuid.NewString()
	ch := r.registerPending(echo)
	if err := r.SendAction(&Action{Action: action, Params: params, Echo: echo, Target: target}); err != nil {
		r.removePending(echo)
		return nil, err
	}

	select {
	case result := <-ch:
		if result.RetCode != 0 {
			return result, fmt.Errorf("onebot: api %q failed: retcode=%d status=%s message=%s",
				action, result.RetCode, result.Status, result.Message)
		}
		return result, nil
	case <-ctx.Done():
		r.removePending(echo)
		return nil, ctx.Err()
	case <-r.done:
		r.removePending(echo)
		return nil, ErrNotRunning
	}
}

// SendGroupMsg 发送群消息
func (r *Adapter) SendGroupMsg(groupID int64, segments ...Segment) error {
	return r.Send("send_group_msg", map[string]any{
		"group_id": groupID,
		"message":  segments,
	})
}

// SendPrivateMsg 发送私聊消息
func (r *Adapter) SendPrivateMsg(userID int64, segments ...Segment) error {
	return r.Send("send_private_msg", map[string]any{
		"user_id": userID,
		"message": segments,
	})
}

// runWebSocket accept 循环. 不使用 gws.RunListener —— 它在 listener 关闭后
// 不会退出, 会在 accept 错误上无限循环刷日志 (吃 CPU/内存).
func (r *Adapter) runWebSocket() {
	defer r.wg.Done()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("[onebot] accept error: %v", err)
			continue
		}
		go r.handleConn(conn)
	}
}

// handleConn 读取握手请求并升级为 WebSocket 连接, 之后阻塞在读循环
func (r *Adapter) handleConn(conn net.Conn) {
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		log.Printf("[onebot] bad handshake from %s: %v", conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}
	socket, err := r.upgrader.UpgradeFromConn(conn, br, req)
	if err != nil {
		// 升级失败 (含鉴权不通过) 时, UpgradeFromConn 内部已写回错误响应并关闭连接
		log.Printf("[onebot] upgrade from %s: %v", conn.RemoteAddr(), err)
		return
	}
	socket.ReadLoop()
}

// dispatchLoop 消费发送指令管道, 将动作路由给目标客户端
func (r *Adapter) dispatchLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			r.drainOutbound()
			return
		case act := <-r.outbound:
			r.dispatch(act)
		}
	}
}

// drainOutbound 退出前排空剩余指令并回执失败
func (r *Adapter) drainOutbound() {
	for {
		select {
		case act := <-r.outbound:
			r.reply(act, nil, ErrNotRunning)
		default:
			return
		}
	}
}

// dispatch 将动作序列化并写入目标连接
func (r *Adapter) dispatch(act *Action) {
	if act == nil {
		return
	}
	conn, err := r.route(act.Target)
	if err != nil {
		r.reply(act, nil, err)
		return
	}
	payload, err := json.Marshal(act)
	if err != nil {
		r.reply(act, nil, err)
		return
	}
	if err := conn.WriteMessage(gws.OpcodeText, payload); err != nil {
		r.reply(act, nil, err)
	}
}

// route 选择目标连接: 指定 self_id 则精确路由, 否则要求有且仅有一个客户端
func (r *Adapter) route(target string) (*gws.Conn, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if target != "" {
		if p, ok := r.peers[target]; ok {
			return p.conn, nil
		}
		return nil, ErrUnknownPeer
	}
	switch len(r.peers) {
	case 0:
		return nil, ErrNoPeer
	case 1:
		for _, p := range r.peers {
			return p.conn, nil
		}
	}
	return nil, ErrAmbiguousPeer
}

func (r *Adapter) registerPending(echo string) chan *ApiResult {
	ch := make(chan *ApiResult, 1)
	r.callMu.Lock()
	r.pending[echo] = ch
	r.callMu.Unlock()
	return ch
}

func (r *Adapter) removePending(echo string) {
	r.callMu.Lock()
	delete(r.pending, echo)
	r.callMu.Unlock()
}

// resolve 根据 echo 分发 API 回执
func (r *Adapter) resolve(echo string, result *ApiResult) {
	r.callMu.Lock()
	ch, ok := r.pending[echo]
	delete(r.pending, echo)
	r.callMu.Unlock()
	if ok {
		ch <- result
	}
}

// failPending 令所有等待中的 API 调用立即失败
func (r *Adapter) failPending(err error) {
	r.callMu.Lock()
	pending := r.pending
	r.pending = make(map[string]chan *ApiResult)
	r.callMu.Unlock()
	for echo, ch := range pending {
		ch <- &ApiResult{Status: "failed", RetCode: -1, Message: err.Error(), Echo: echo}
	}
}

// reply 回执动作结果; 无 echo 的动作仅记录失败日志
func (r *Adapter) reply(act *Action, result *ApiResult, err error) {
	if act == nil {
		return
	}
	if act.Echo == "" {
		if err != nil {
			log.Printf("[onebot] action %q dropped: %v", act.Action, err)
		}
		return
	}
	if err != nil {
		result = &ApiResult{Status: "failed", RetCode: -1, Message: err.Error(), Echo: act.Echo}
	}
	r.resolve(act.Echo, result)
}

// snapshotConns 复制当前连接列表
func (r *Adapter) snapshotConns() []*gws.Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conns := make([]*gws.Conn, 0, len(r.conns))
	for conn := range r.conns {
		conns = append(conns, conn)
	}
	return conns
}

// onConnOpen 注册新连接 (由 Server.OnOpen 调用)
func (r *Adapter) onConnOpen(conn *gws.Conn) {
	p := &peer{conn: conn, session: uuid.NewString(), lastBeat: time.Now()}
	r.mu.Lock()
	r.conns[conn] = p
	r.mu.Unlock()
	log.Printf("[onebot] client connected from %s session=%s", conn.RemoteAddr(), p.session)
}

// onConnClose 注销连接 (由 Server.OnClose 调用)
func (r *Adapter) onConnClose(conn *gws.Conn, err error) {
	r.mu.Lock()
	p, ok := r.conns[conn]
	if ok {
		delete(r.conns, conn)
		if p.selfID != "" && r.peers[p.selfID] == p {
			delete(r.peers, p.selfID)
		}
	}
	r.mu.Unlock()

	if ok {
		reason := ""
		if err != nil {
			reason = err.Error()
		}
		log.Printf("[onebot] client disconnected session=%s self_id=%s: %s", p.session, p.selfID, reason)
	}
}

// bindSelfID 将连接与其上报的 self_id 绑定; 同一 self_id 的旧连接会被顶替
func (r *Adapter) bindSelfID(conn *gws.Conn, selfID string) {
	r.mu.Lock()
	p, ok := r.conns[conn]
	if !ok || p.selfID == selfID {
		r.mu.Unlock()
		return
	}
	old, exists := r.peers[selfID]
	p.selfID = selfID
	p.lastBeat = time.Now()
	r.peers[selfID] = p
	r.mu.Unlock()

	if exists && old != p {
		r.mu.Lock()
		delete(r.conns, old.conn)
		r.mu.Unlock()
		_ = old.conn.WriteClose(1000, []byte("duplicated connection"))
		log.Printf("[onebot] replace old connection of self_id=%s", selfID)
	}
}

// onPayload 解析并分发客户端上报数据 (由 Server.OnMessage 调用)
func (r *Adapter) onPayload(conn *gws.Conn, data []byte) {
	var probe struct {
		PostType string `json:"post_type"`
		SelfID   int64  `json:"self_id"`
		Echo     string `json:"echo"`
		RetCode  *int   `json:"retcode"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		log.Printf("[onebot] discard invalid payload: %v", err)
		return
	}

	// API 回执
	if probe.RetCode != nil {
		result := &ApiResult{Echo: probe.Echo}
		if err := json.Unmarshal(data, result); err != nil {
			log.Printf("[onebot] discard invalid api response: %v", err)
			return
		}
		if probe.Echo == "" {
			log.Printf("[onebot] api response without echo: retcode=%d", *probe.RetCode)
			return
		}
		r.resolve(probe.Echo, result)
		return
	}

	// 绑定机器人身份
	if probe.SelfID != 0 {
		r.bindSelfID(conn, fmt.Sprint(probe.SelfID))
	}

	switch probe.PostType {
	case "message":
		r.onMessageEvent(data)
	case "meta_event":
		r.onMetaEvent(data)
	case "notice":
		r.onNoticeEvent(data)
	default:
		log.Printf("[onebot] unknown post_type %q", probe.PostType)
	}
}

// onMessageEvent 处理私聊/群聊消息事件, 转换为统一消息后投递接收管道
func (r *Adapter) onMessageEvent(data []byte) {
	var ev MessageEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[onebot] discard invalid message event: %v", err)
		return
	}

	selfID := fmt.Sprint(ev.SelfID)
	msgType := common.Group
	chatID := fmt.Sprint(ev.GroupID)
	if ev.MessageType == "private" {
		msgType = common.Private
		chatID = fmt.Sprint(ev.UserID)
	}

	content, mentionedSelf := convertSegments(ev.Message, selfID)

	name := ev.Sender.Card
	if name == "" {
		name = ev.Sender.Nickname
	}

	msg := common.Message{
		MessageID:   fmt.Sprint(ev.MessageID),
		Platform:    common.Onebot,
		MessageType: msgType,
		ChatID:      chatID,
		SelfID:      selfID,
		Sender: common.Sender{
			UID:     fmt.Sprint(ev.Sender.UserID),
			Name:    name,
			IsBot:   ev.Sender.UserID == ev.SelfID,
			IsAdmin: ev.Sender.Role == "admin" || ev.Sender.Role == "owner",
		},
		Timestamp:     ev.Time,
		MentionedSelf: mentionedSelf,
		Content:       content,
		Original:      json.RawMessage(data),
	}
	r.push(msg)
}

// onMetaEvent 处理心跳与生命周期事件
func (r *Adapter) onMetaEvent(data []byte) {
	var meta struct {
		MetaEventType string `json:"meta_event_type"`
		SubType       string `json:"sub_type"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}
	if meta.MetaEventType == "heartbeat" {
		var beat struct {
			SelfID int64 `json:"self_id"`
		}
		if err := json.Unmarshal(data, &beat); err == nil {
			r.touchPeer(fmt.Sprint(beat.SelfID))
		}
		return
	}
	log.Printf("[onebot] meta_event %s sub_type=%s", meta.MetaEventType, meta.SubType)
}

// onNoticeEvent 处理通知类事件 (戳一戳等); 暂只记录日志, 后续可按需扩展独立管道
func (r *Adapter) onNoticeEvent(data []byte) {
	var ev PokeEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}
	if ev.NoticeType == "notify" && ev.SubType == "poke" {
		log.Printf("[onebot] poke self_id=%d group=%d user=%d target=%d",
			ev.SelfID, ev.GroupID, ev.UserID, ev.TargetID)
	}
}

// touchPeer 更新客户端最近心跳时间
func (r *Adapter) touchPeer(selfID string) {
	r.mu.Lock()
	if p, ok := r.peers[selfID]; ok {
		p.lastBeat = time.Now()
	}
	r.mu.Unlock()
}

// push 投递消息到接收管道; 管道满时阻塞 (背压), 适配器停止后丢弃
func (r *Adapter) push(msg common.Message) {
	select {
	case <-r.done:
	case r.inbound <- msg:
	}
}

// authorize 握手鉴权: 校验 Authorization 头 (Bearer/Token) 或 access_token 查询参数
func (r *Adapter) authorize(req *http.Request, _ gws.SessionStorage) bool {
	if r.cfg.Token == "" {
		return true
	}
	token := req.URL.Query().Get("access_token")
	if token == "" {
		auth := req.Header.Get("Authorization")
		for _, prefix := range []string{"Bearer ", "Token "} {
			if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
				token = strings.TrimSpace(auth[len(prefix):])
				break
			}
		}
	}
	return token == r.cfg.Token
}
