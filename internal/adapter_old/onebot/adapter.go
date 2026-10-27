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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lxzan/gws"

	"JuanNiang-Ops/internal/adapter_old/common"
	"JuanNiang-Ops/pkg/uerr"
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

// peer 一个接入的 OneBot 客户端 (机器人端)
type peer struct {
	conn     *gws.Conn
	session  string // 接入时分配的会话 ID, 上报 self_id 前作为占位
	selfID   string // 机器人自身 ID, 首个事件上报后绑定
	lastBeat time.Time
}

// defaultNoticeBuffer 通知管道默认缓冲大小
const defaultNoticeBuffer = 64

// Adapter OneBot 正向 WebSocket 适配器.
// 内部维护两条接收管道与一条发送管道:
//   - inbound:  接收消息管道, OneBot 消息事件 -> 统一消息 common.Message -> 业务层
//   - notices:  通知管道, OneBot notice/lifecycle 事件 (原始 JSON) -> 业务层
//   - outbound: 发送指令管道, 业务层 -> OneBot 实现端
type Adapter struct {
	cfg Config

	inbound  chan common.Message
	notices  chan UnionEvent
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
	pending map[string]chan *callResult
}

// callResult API 调用结果: 远端回执走 result, 本地失败 (路由/序列化/写连接) 走 err 以保留错误链
type callResult struct {
	result *ApiResult
	err    error
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
		pending: make(map[string]chan *callResult),
	}
}

// Start 启动适配器: 监听端口, 建立收发管道. 重复调用返回 ErrRunning.
func (r *Adapter) Start() error {
	if !r.cfg.Enable {
		return uerr.ErrDisabled
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return uerr.ErrRunning
	}

	listener, err := net.Listen("tcp", r.cfg.Addr)
	if err != nil {
		r.mu.Unlock()
		return fmt.Errorf("onebot: listen %s: %w", r.cfg.Addr, err)
	}

	r.inbound = make(chan common.Message, r.cfg.MessageBuffer)
	r.notices = make(chan UnionEvent, defaultNoticeBuffer)
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
// 退出后 Inbound 会被关闭, 消费方可通过 range 退出; 之后的投递会返回 uerr.ErrNotRunning.
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
	r.failPending(uerr.ErrNotRunning)

	// 4. 等待内部 goroutine 退出
	r.wg.Wait()

	// 5. 关闭接收管道 (消息与通知), 通知下游; 清空连接表以便下次 Start
	close(r.inbound)
	close(r.notices)
	r.mu.Lock()
	r.peers = make(map[string]*peer)
	r.conns = make(map[*gws.Conn]*peer)
	r.mu.Unlock()

	log.Printf("[onebot] adapter stopped")
	return nil
}

// Done 返回停止信号通道: Stop 时被关闭. 与管道配合使用:
func (r *Adapter) Done() <-chan struct{} { return r.done }

// Inbound 接收消息管道 (OneBot 消息事件 -> 统一消息 -> 业务层), Start 后可用, Stop 时关闭
func (r *Adapter) Inbound() <-chan common.Message { return r.inbound }

// Notices 通知管道 (OneBot notice/lifecycle 事件, 原始 JSON -> 业务层), Start 后可用, Stop 时关闭.
// 业务层可使用 PokeEvent / LifeCycleEvent 等结构自行解析 EventData.
func (r *Adapter) Notices() <-chan UnionEvent { return r.notices }

// Outbound 发送指令管道 (业务层 -> OneBot), 只写方向. Start 后可用.
// 适配器停止后其剩余内容会被排空丢弃; 建议优先使用 Send/Call.
func (r *Adapter) Outbound() chan<- *Action { return r.outbound }

// Send 投递一条不需要回执的指令 (fire-and-forget)
func (r *Adapter) Send(action string, params map[string]any, target string) error {
	return r.SendAction(&Action{Action: action, Params: params, Target: target})
}

// GetCurrentState 获取适配器当前状态 (含各客户端状态快照)
func (r *Adapter) GetCurrentState() CurrentState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make(map[string]PeerState, len(r.peers))
	for selfID, p := range r.peers {
		peers[selfID] = PeerState{
			Session:    p.session,
			SelfID:     p.selfID,
			RemoteAddr: p.conn.RemoteAddr().String(),
			LastBeat:   p.lastBeat,
		}
	}

	return CurrentState{
		IsRunning: r.running,
		PeerList:  peers,
		PeerCount: len(peers),
	}
}

// GetCurrentConfig 获取当前配置
func (r *Adapter) GetCurrentConfig() Config {
	return r.cfg
}

// SendAction 投递指令到发送管道, 立即返回; 适配器停止后返回 ErrNotRunning
func (r *Adapter) SendAction(act *Action) error {
	r.mu.RLock()
	running, outbound := r.running, r.outbound
	r.mu.RUnlock()
	if !running || outbound == nil {
		return uerr.ErrNotRunning
	}
	select {
	case <-r.done:
		return uerr.ErrNotRunning
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
		return nil, uerr.ErrNotRunning
	}

	echo := uuid.NewString()
	ch := r.registerPending(echo)
	if err := r.SendAction(&Action{Action: action, Params: params, Echo: echo, Target: target}); err != nil {
		r.removePending(echo)
		return nil, err
	}

	select {
	case cr := <-ch:
		if cr.err != nil {
			return nil, cr.err
		}
		if cr.result.RetCode != 0 {
			return cr.result, fmt.Errorf("onebot: api %q failed: retcode=%d status=%s message=%s: %w",
				action, cr.result.RetCode, cr.result.Status, cr.result.Message, uerr.ErrApiFailed)
		}
		return cr.result, nil
	case <-ctx.Done():
		r.removePending(echo)
		return nil, ctx.Err()
	case <-r.done:
		r.removePending(echo)
		return nil, uerr.ErrNotRunning
	}
}

// SendGroupMsg 发送群消息: 统一消息部件 -> OneBot 消息段 -> send_group_msg
func (r *Adapter) SendGroupMsg(target string, groupID int64, parts []common.MessagePart) error {
	segments, err := PartsToSegments(parts)
	if err != nil {
		return err
	}
	return r.Send(
		"send_group_msg",
		map[string]any{
			"group_id": groupID,
			"message":  segments,
		},
		target,
	)
}

// SendPrivateMsg 发送私聊消息: 统一消息部件 -> OneBot 消息段 -> send_private_msg
func (r *Adapter) SendPrivateMsg(target string, userID int64, parts []common.MessagePart) error {
	segments, err := PartsToSegments(parts)
	if err != nil {
		return err
	}
	return r.Send(
		"send_private_msg",
		map[string]any{
			"user_id": userID,
			"message": segments,
		},
		target,
	)
}

// ---------- 实现 adapter.MessageSender 接口 (统一发送入口) ----------

// SendGroup 实现 MessageSender: 群消息 (chatID 为群号)
func (r *Adapter) SendGroup(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error) {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("onebot: invalid group chat_id %q: %w", chatID, err)
	}
	result, err := r.Call(ctx, selfID, "send_group_msg", map[string]any{
		"group_id": id,
		"message":  mustPartsToSegments(parts),
	})
	if err != nil {
		return nil, err
	}
	return r.receiptOf(result)
}

// SendPrivate 实现 MessageSender: 私聊消息 (chatID 为对方 QQ 号)
func (r *Adapter) SendPrivate(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error) {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("onebot: invalid private chat_id %q: %w", chatID, err)
	}
	result, err := r.Call(ctx, selfID, "send_private_msg", map[string]any{
		"user_id": id,
		"message": mustPartsToSegments(parts),
	})
	if err != nil {
		return nil, err
	}
	return r.receiptOf(result)
}

// mustPartsToSegments 统一部件转 OneBot 段; 转换失败以 error 值形式随结果返回
func mustPartsToSegments(parts []common.MessagePart) (segments []Segment) {
	segments, err := PartsToSegments(parts)
	if err != nil {
		return []Segment{{Type: "__invalid__", Data: map[string]any{"error": err.Error()}}}
	}
	return segments
}

// receiptOf 从 API 回执提取统一发送回执 (message_id)
func (r *Adapter) receiptOf(result *ApiResult) (*common.SendReceipt, error) {
	receipt := &common.SendReceipt{Reply: result}
	if len(result.Data) > 0 {
		var data struct {
			MessageID json.Number `json:"message_id"`
		}
		if err := json.Unmarshal(result.Data, &data); err == nil && data.MessageID != "" {
			receipt.MessageID = data.MessageID.String()
		}
	}
	return receipt, nil
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
			r.reply(act, nil, uerr.ErrNotRunning)
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
		return nil, uerr.ErrUnknownPeer
	}
	switch len(r.peers) {
	case 0:
		return nil, uerr.ErrNoPeer
	case 1:
		for _, p := range r.peers {
			return p.conn, nil
		}
	}
	return nil, uerr.ErrAmbiguousPeer
}

func (r *Adapter) registerPending(echo string) chan *callResult {
	ch := make(chan *callResult, 1)
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
	if echo == "" || result == nil {
		return
	}
	r.callMu.Lock()
	ch, ok := r.pending[echo]
	delete(r.pending, echo)
	r.callMu.Unlock()
	if ok {
		ch <- &callResult{result: result}
	}
}

// resolveErr 根据 echo 分发本地失败 (保留原始错误链)
func (r *Adapter) resolveErr(echo string, err error) {
	if echo == "" || err == nil {
		return
	}
	r.callMu.Lock()
	ch, ok := r.pending[echo]
	delete(r.pending, echo)
	r.callMu.Unlock()
	if ok {
		ch <- &callResult{err: err}
	}
}

// failPending 令所有等待中的 API 调用立即失败
func (r *Adapter) failPending(err error) {
	r.callMu.Lock()
	pending := r.pending
	r.pending = make(map[string]chan *callResult)
	r.callMu.Unlock()
	for _, ch := range pending {
		ch <- &callResult{err: err}
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
		// 回执中保留原始错误链; 同时携带 retcode=-1 的标准回执信息
		result = &ApiResult{Status: "failed", RetCode: -1, Message: err.Error(), Echo: act.Echo}
		r.resolveErr(act.Echo, fmt.Errorf("%w (retcode=-1, message=%s)", err, result.Message))
		return
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
		r.onMessageEvent(conn, data, probe.SelfID)
	case "meta_event":
		r.onMetaEvent(conn, data)
	case "notice":
		r.onNoticeEvent(data, probe.SelfID)
	default:
		log.Printf("[onebot] unknown post_type %q", probe.PostType)
	}
}

// onMessageEvent 处理私聊/群聊消息事件, 转换为统一消息后投递接收管道
func (r *Adapter) onMessageEvent(conn *gws.Conn, data []byte, selfID int64) {
	var ev MessageEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[onebot] discard invalid message event: %v", err)
		return
	}

	// 上报未携带 self_id 时, 用连接已绑定的身份兜底
	self := fmt.Sprint(ev.SelfID)
	if ev.SelfID == 0 {
		self = r.connSelfID(conn)
	}
	if self == "" || self == "0" {
		log.Printf("[onebot] discard message event without self_id")
		return
	}

	msgType := common.Group
	chatID := fmt.Sprint(ev.GroupID)
	if ev.MessageType == "private" {
		msgType = common.Private
		chatID = fmt.Sprint(ev.UserID)
	}

	content, mentionedSelf := convertSegments(ev.Message, self)

	name := ev.Sender.Card
	if name == "" {
		name = ev.Sender.Nickname
	}

	msg := common.Message{
		MessageID:   fmt.Sprint(ev.MessageID),
		Platform:    common.Onebot,
		MessageType: msgType,
		ChatID:      chatID,
		SelfID:      self,
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

// connSelfID 查询连接已绑定的 self_id, 未绑定返回空串
func (r *Adapter) connSelfID(conn *gws.Conn) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.conns[conn]; ok {
		return p.selfID
	}
	return ""
}

// onMetaEvent 处理心跳与生命周期事件: 心跳仅刷新 lastBeat, lifecycle 投递通知管道
func (r *Adapter) onMetaEvent(conn *gws.Conn, data []byte) {
	var meta struct {
		MetaEventType string `json:"meta_event_type"`
		SubType       string `json:"sub_type"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}
	switch meta.MetaEventType {
	case "heartbeat":
		var beat struct {
			SelfID int64 `json:"self_id"`
		}
		if err := json.Unmarshal(data, &beat); err == nil {
			r.touchPeer(fmt.Sprint(beat.SelfID))
		}
	case "lifecycle":
		r.pushNotice(UnionEvent{
			EventType: MetaEventType,
			EventData: data,
			SelfID:    r.connSelfIDInt(conn),
		})
	default:
		log.Printf("[onebot] meta_event %s sub_type=%s", meta.MetaEventType, meta.SubType)
	}
}

// onNoticeEvent 处理通知类事件 (戳一戳等), 投递通知管道 (原始 JSON, 业务层自行解析)
func (r *Adapter) onNoticeEvent(data []byte, selfID int64) {
	r.pushNotice(UnionEvent{
		EventType: NotifyEventType,
		EventData: data,
		SelfID:    selfID,
	})
}

// touchPeer 更新客户端最近心跳时间
func (r *Adapter) touchPeer(selfID string) {
	r.mu.Lock()
	if p, ok := r.peers[selfID]; ok {
		p.lastBeat = time.Now()
	}
	r.mu.Unlock()
}

// push 投递统一消息到接收管道; 管道满时阻塞 (背压), 适配器停止后丢弃
func (r *Adapter) push(msg common.Message) {
	select {
	case <-r.done:
	case r.inbound <- msg:
	}
}

// pushNotice 投递事件到通知管道; 管道满时阻塞 (背压), 适配器停止后丢弃
func (r *Adapter) pushNotice(ev UnionEvent) {
	select {
	case <-r.done:
	case r.notices <- ev:
	}
}

// connSelfIDInt 查询连接已绑定的 self_id (数值形式), 未绑定返回 0
func (r *Adapter) connSelfIDInt(conn *gws.Conn) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.conns[conn]; ok {
		id, err := strconv.ParseInt(p.selfID, 10, 64)
		if err != nil {
			return 0
		}
		return id
	}
	return 0
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
