package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/noahmagill/webhook-rev-shell/internal/config"
	"github.com/noahmagill/webhook-rev-shell/internal/operatorinput"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"github.com/noahmagill/webhook-rev-shell/internal/store"
)

var ErrSessionCapReached = errors.New("session cap reached")

type OperatorConn struct {
	ID     string
	Send   chan []byte
	Done   chan struct{}
	Ctx    context.Context
	Cancel context.CancelFunc
}

type TargetLink struct {
	ID        string
	Info      *protocol.Target
	CmdQueue  chan []byte // commands to send to target
	Done      chan struct{}
	lastSeen  time.Time
	seenMu    sync.Mutex
}

func (tl *TargetLink) Touch() {
	tl.seenMu.Lock()
	tl.lastSeen = time.Now()
	tl.seenMu.Unlock()
}

func (tl *TargetLink) LastSeen() time.Time {
	tl.seenMu.Lock()
	defer tl.seenMu.Unlock()
	return tl.lastSeen
}

// TargetStaleTimeout is how long without poll/push/WS activity before a beacon is removed.
// Must exceed poll long-poll duration (30s) plus reconnect slack.
const TargetStaleTimeout = 45 * time.Second

type Room struct {
	Session    *protocol.Session
	Targets    sync.Map // target_id -> *TargetLink
	Operators  sync.Map // conn_id -> *OperatorConn
	Transcript *RingBuffer
	mu         sync.RWMutex
	claimMu    sync.Mutex
	claimed    *beaconClaim
}

func (r *Room) BroadcastToOperators(data []byte) {
	r.Operators.Range(func(_, val any) bool {
		op := val.(*OperatorConn)
		select {
		case op.Send <- data:
		default:
		}
		return true
	})
}

func (r *Room) SendToTarget(targetID string, data []byte) bool {
	val, ok := r.Targets.Load(targetID)
	if !ok {
		return false
	}
	tl := val.(*TargetLink)
	mode := ""
	if tl.Info != nil {
		mode = tl.Info.Mode
	}
	data = operatorinput.ForCommandMode(mode, data)
	if len(data) == 0 {
		return true
	}
	select {
	case tl.CmdQueue <- data:
		return true
	default:
		return false
	}
}

func (r *Room) FirstTargetID() string {
	var id string
	r.Targets.Range(func(key, _ any) bool {
		id = key.(string)
		return false
	})
	return id
}

type Broker struct {
	mu    sync.RWMutex
	rooms map[string]*Room
	store *store.Store
}

func New(s *store.Store) *Broker {
	return &Broker{
		rooms: make(map[string]*Room),
		store: s,
	}
}

func (b *Broker) Store() *store.Store {
	return b.store
}

func (b *Broker) CreateSession(name, ownerWorkspaceID string, cfg config.Auth) (*protocol.Session, string, error) {
	if ownerWorkspaceID == "" {
		return nil, "", fmt.Errorf("workspace id required")
	}
	if err := authCanCreate(b.store, cfg, ownerWorkspaceID); err != nil {
		return nil, "", err
	}

	id := uuid.New().String()
	secret := uuid.New().String()

	now := time.Now()
	sess := &protocol.Session{
		ID:               id,
		Name:             name,
		Secret:           secret,
		OwnerWorkspaceID: ownerWorkspaceID,
		CreatedAt:        now,
		LastActivity:     now,
		State:            protocol.StateWaiting,
	}

	if err := b.store.PutSession(sess); err != nil {
		return nil, "", err
	}

	browserToken := uuid.New().String()
	bt := &protocol.BrowserToken{
		Token:     browserToken,
		SessionID: id,
		CreatedAt: now,
		ExpiresAt: now.Add(cfg.SessionBrowserTokenTTL),
	}
	if err := b.store.PutBrowserToken(bt); err != nil {
		return nil, "", err
	}

	b.mu.Lock()
	b.rooms[id] = &Room{
		Session:    sess,
		Transcript: NewRingBuffer(1024),
	}
	b.mu.Unlock()

	return sess, browserToken, nil
}

func (b *Broker) GetRoom(sessionID string) *Room {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.rooms[sessionID]
}

func (b *Broker) GetOrLoadRoom(sessionID string) (*Room, error) {
	b.mu.RLock()
	room := b.rooms[sessionID]
	b.mu.RUnlock()
	if room != nil {
		return room, nil
	}

	sess, err := b.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.State == protocol.StateExpired || sess.State == protocol.StateKilled {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if r := b.rooms[sessionID]; r != nil {
		return r, nil
	}
	room = &Room{
		Session:    sess,
		Transcript: NewRingBuffer(1024),
	}
	b.rooms[sessionID] = room
	return room, nil
}

func (b *Broker) ListSessions() ([]*protocol.Session, error) {
	return b.store.ListSessions()
}

func (b *Broker) ListSessionsForWorkspace(workspaceID string) ([]*protocol.Session, error) {
	return b.store.ListSessionsByWorkspace(workspaceID)
}

func authCanCreate(s *store.Store, cfg config.Auth, workspaceID string) error {
	n, err := s.CountActiveSessionsForWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if n >= cfg.MaxSessionsPerWorkspace {
		return ErrSessionCapReached
	}
	return nil
}

func (b *Broker) KillSession(id string) error {
	b.mu.Lock()
	room := b.rooms[id]
	delete(b.rooms, id)
	b.mu.Unlock()

	if room != nil {
		room.Operators.Range(func(_, val any) bool {
			op := val.(*OperatorConn)
			op.Cancel()
			return true
		})
		room.Targets.Range(func(_, val any) bool {
			tl := val.(*TargetLink)
			close(tl.Done)
			return true
		})
	}

	sess, err := b.store.GetSession(id)
	if err != nil {
		return err
	}
	sess.State = protocol.StateKilled
	return b.store.PutSession(sess)
}

func (b *Broker) Touch(sessionID string) {
	b.mu.RLock()
	room := b.rooms[sessionID]
	b.mu.RUnlock()
	if room != nil {
		room.mu.Lock()
		room.Session.LastActivity = time.Now()
		room.mu.Unlock()
	}
	_ = b.store.TouchSession(sessionID)
}

func (b *Broker) RegisterTarget(sessionID string, info *protocol.Target) (*TargetLink, error) {
	room, err := b.GetOrLoadRoom(sessionID)
	if err != nil {
		return nil, err
	}

	if err := room.TryClaim(info.ID); err != nil {
		return nil, err
	}

	info.LastSeen = time.Now()
	tl := &TargetLink{
		ID:       info.ID,
		Info:     info,
		CmdQueue: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	tl.Touch()
	room.Targets.Store(info.ID, tl)

	room.mu.Lock()
	if room.Session.State == protocol.StateWaiting {
		room.Session.State = protocol.StateActive
		_ = b.store.PutSession(room.Session)
	}
	room.mu.Unlock()

	_ = b.store.PutTarget(info)
	b.updateSessionTarget(sessionID, info)
	b.Touch(sessionID)
	return tl, nil
}

func (b *Broker) updateSessionTarget(sessionID string, info *protocol.Target) {
	sess, err := b.store.GetSession(sessionID)
	if err != nil {
		return
	}
	sess.LastTarget = &protocol.TargetSnapshot{
		User:   info.User,
		Host:   info.Host,
		OS:     info.OS,
		Arch:   info.Arch,
		System: info.System,
		Mode:   info.Mode,
	}
	_ = b.store.PutSession(sess)
	if room := b.GetRoom(sessionID); room != nil {
		room.mu.Lock()
		room.Session = sess
		room.mu.Unlock()
	}
}

func (b *Broker) AddOperator(sessionID string) (*OperatorConn, *Room, error) {
	room, err := b.GetOrLoadRoom(sessionID)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	op := &OperatorConn{
		ID:     uuid.New().String()[:8],
		Send:   make(chan []byte, 256),
		Done:   done,
		Ctx:    ctx,
		Cancel: func() {
			cancel()
			select {
			case <-done:
			default:
				close(done)
			}
		},
	}
	room.Operators.Store(op.ID, op)
	b.Touch(sessionID)
	return op, room, nil
}

func (b *Broker) RemoveOperator(sessionID, opID string) {
	b.mu.RLock()
	room := b.rooms[sessionID]
	b.mu.RUnlock()
	if room != nil {
		room.Operators.Delete(opID)
	}
}

func (b *Broker) RemoveTarget(sessionID, targetID string) {
	b.mu.RLock()
	room := b.rooms[sessionID]
	b.mu.RUnlock()
	if room != nil {
		wasClaimed := room.WasClaimedBy(targetID)
		room.ReleaseClaim(targetID)
		room.Targets.Delete(targetID)
		if wasClaimed {
			room.DisconnectOperators()
		}
	}
	_ = b.store.DeleteTarget(sessionID, targetID)
}

// PruneStaleTargets removes beacons with no recent poll/push/WS activity and notifies operators.
func (b *Broker) PruneStaleTargets() {
	b.mu.RLock()
	rooms := make([]*Room, 0, len(b.rooms))
	for _, room := range b.rooms {
		rooms = append(rooms, room)
	}
	b.mu.RUnlock()

	cutoff := time.Now().Add(-TargetStaleTimeout)
	for _, room := range rooms {
		var stale []string
		room.Targets.Range(func(key, val any) bool {
			tl := val.(*TargetLink)
			if tl.LastSeen().Before(cutoff) {
				stale = append(stale, key.(string))
			}
			return true
		})
		for _, id := range stale {
			b.RemoveTarget(room.Session.ID, id)
		}
	}
}

func (b *Broker) Sweep(maxIdle time.Duration) {
	expired, _ := b.store.ExpireSessions(maxIdle)
	b.mu.Lock()
	for _, id := range expired {
		if room := b.rooms[id]; room != nil {
			room.Operators.Range(func(_, val any) bool {
				op := val.(*OperatorConn)
				op.Cancel()
				return true
			})
			room.Targets.Range(func(_, val any) bool {
				tl := val.(*TargetLink)
				close(tl.Done)
				return true
			})
		}
		delete(b.rooms, id)
	}
	b.mu.Unlock()
	_ = b.store.PruneBrowserTokens()
	_ = b.store.PruneWorkspaceBrowserTokens()
	_ = b.store.PruneOperatorTokens()
}
