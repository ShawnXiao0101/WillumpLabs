package gomoku

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	boardSize = 15
	empty     = 0
	black     = 1
	white     = 2
)

type Server struct {
	mu       sync.RWMutex
	sessions map[string]*User
	rooms    map[string]*Room
	hub      *Hub
}

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

type Room struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Board      [boardSize][boardSize]int `json:"board"`
	Players    [2]*User                  `json:"players"`
	Spectators int                       `json:"spectators"`
	Turn       int                       `json:"turn"`
	Winner     int                       `json:"winner"`
	Draw       bool                      `json:"draw"`
	Moves      []Move                    `json:"moves"`
	CreatedAt  int64                     `json:"createdAt"`
	UpdatedAt  int64                     `json:"updatedAt"`
}

type Move struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Stone  int    `json:"stone"`
	UserID string `json:"userId"`
}

type Client struct {
	roomID string
	user   *User
	conn   *websocket.Conn
	send   chan []byte
	hub    *Hub
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]bool
	join  chan *Client
	leave chan *Client
}

type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewServer() *Server {
	server := &Server{
		sessions: make(map[string]*User),
		rooms:    make(map[string]*Room),
		hub:      NewHub(),
	}
	go server.hub.Run()
	return server
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", server.handleLogin)
	mux.HandleFunc("/api/me", server.withUser(server.handleMe))
	mux.HandleFunc("/api/rooms", server.withUser(server.handleRooms))
	mux.HandleFunc("/api/rooms/", server.withUser(server.handleRoomByID))
	mux.HandleFunc("/ws/rooms/", server.withUser(server.handleRoomSocket))

	dist := filepath.Join("web", "dist")
	if _, err := os.Stat(dist); err == nil {
		mux.Handle("/", spaHandler(dist))
	}
	return logRequests(mux)
}

func Run() error {
	server := NewServer()
	addr := ":" + env("PORT", "8080")
	log.Printf("WillumpLabs gomoku server listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, server.Handler())
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[*Client]bool),
		join:  make(chan *Client),
		leave: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.join:
			h.mu.Lock()
			if h.rooms[c.roomID] == nil {
				h.rooms[c.roomID] = make(map[*Client]bool)
			}
			h.rooms[c.roomID][c] = true
			h.mu.Unlock()
		case c := <-h.leave:
			h.remove(c)
		}
	}
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients := h.rooms[c.roomID]; clients != nil {
		if clients[c] {
			delete(clients, c)
			close(c.send)
		}
		if len(clients) == 0 {
			delete(h.rooms, c.roomID)
		}
	}
}

func (h *Hub) Broadcast(roomID string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := h.rooms[roomID]
	for c := range clients {
		select {
		case c.send <- data:
		default:
			go h.remove(c)
		}
	}
	h.mu.RUnlock()
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(input.Name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 16 {
		writeError(w, http.StatusBadRequest, "昵称需要 2 到 16 个字符")
		return
	}
	token := randomID(32)
	user := &User{ID: randomID(10), Name: name, CreatedAt: time.Now().UnixMilli()}

	s.mu.Lock()
	s.sessions[token] = user
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "wl_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 14,
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, user *User) {
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request, user *User) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		rooms := make([]*Room, 0, len(s.rooms))
		for _, room := range s.rooms {
			copy := *room
			rooms = append(rooms, &copy)
		}
		s.mu.RUnlock()
		sort.Slice(rooms, func(i, j int) bool { return rooms[i].UpdatedAt > rooms[j].UpdatedAt })
		writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = user.Name + " 的棋局"
		}
		room := &Room{
			ID:        randomID(8),
			Name:      trimRunes(name, 28),
			Turn:      black,
			CreatedAt: time.Now().UnixMilli(),
			UpdatedAt: time.Now().UnixMilli(),
		}
		room.Players[0] = user
		s.mu.Lock()
		s.rooms[room.ID] = room
		s.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"room": room})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleRoomByID(w http.ResponseWriter, r *http.Request, user *User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rooms/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}
	roomID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		room, ok := s.roomSnapshot(roomID)
		if !ok {
			writeError(w, http.StatusNotFound, "room not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"room": room})
	case r.Method == http.MethodPost && action == "join":
		room, err := s.joinRoom(roomID, user)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.broadcastRoom(roomID)
		writeJSON(w, http.StatusOK, map[string]any{"room": room})
	case r.Method == http.MethodPost && action == "restart":
		room, err := s.restartRoom(roomID, user)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.broadcastRoom(roomID)
		writeJSON(w, http.StatusOK, map[string]any{"room": room})
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (s *Server) handleRoomSocket(w http.ResponseWriter, r *http.Request, user *User) {
	roomID := strings.TrimPrefix(r.URL.Path, "/ws/rooms/")
	if roomID == "" {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}
	s.mu.RLock()
	_, ok := s.rooms[roomID]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{roomID: roomID, user: user, conn: conn, send: make(chan []byte, 16), hub: s.hub}
	s.hub.join <- client
	s.broadcastRoom(roomID)

	go client.writePump()
	client.readPump(s)
}

func (c *Client) readPump(s *Server) {
	defer func() {
		c.hub.leave <- c
		c.conn.Close()
		s.broadcastRoom(c.roomID)
	}()
	c.conn.SetReadLimit(1024)
	c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})
	for {
		var msg wsMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			break
		}
		if msg.Type != "move" {
			continue
		}
		var input struct {
			X int `json:"x"`
			Y int `json:"y"`
		}
		if err := json.Unmarshal(msg.Data, &input); err != nil {
			continue
		}
		if err := s.placeStone(c.roomID, c.user, input.X, input.Y); err != nil {
			c.sendJSON(map[string]any{"type": "error", "message": err.Error()})
			continue
		}
		s.broadcastRoom(c.roomID)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) sendJSON(payload any) {
	data, err := json.Marshal(payload)
	if err == nil {
		c.send <- data
	}
}

func (s *Server) joinRoom(roomID string, user *User) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return nil, errors.New("棋局不存在")
	}
	if room.Players[0] != nil && room.Players[0].ID == user.ID || room.Players[1] != nil && room.Players[1].ID == user.ID {
		copy := *room
		return &copy, nil
	}
	if room.Players[1] == nil {
		room.Players[1] = user
		room.UpdatedAt = time.Now().UnixMilli()
		copy := *room
		return &copy, nil
	}
	return nil, errors.New("棋局已满，可以作为观众进入")
}

func (s *Server) restartRoom(roomID string, user *User) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return nil, errors.New("棋局不存在")
	}
	if !isPlayer(room, user.ID) {
		return nil, errors.New("只有对局玩家可以重开")
	}
	room.Board = [boardSize][boardSize]int{}
	room.Moves = nil
	room.Winner = empty
	room.Draw = false
	room.Turn = black
	room.UpdatedAt = time.Now().UnixMilli()
	copy := *room
	return &copy, nil
}

func (s *Server) placeStone(roomID string, user *User, x, y int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return errors.New("棋局不存在")
	}
	stone := playerStone(room, user.ID)
	if stone == empty {
		return errors.New("观众不能落子")
	}
	if room.Players[0] == nil || room.Players[1] == nil {
		return errors.New("等待另一位玩家加入")
	}
	if room.Winner != empty || room.Draw {
		return errors.New("本局已结束，请重开")
	}
	if room.Turn != stone {
		return errors.New("还没轮到你")
	}
	if x < 0 || x >= boardSize || y < 0 || y >= boardSize {
		return errors.New("落子位置无效")
	}
	if room.Board[y][x] != empty {
		return errors.New("这里已经有棋子")
	}
	room.Board[y][x] = stone
	room.Moves = append(room.Moves, Move{X: x, Y: y, Stone: stone, UserID: user.ID})
	if hasFive(room.Board, x, y, stone) {
		room.Winner = stone
	} else if len(room.Moves) == boardSize*boardSize {
		room.Draw = true
	} else if stone == black {
		room.Turn = white
	} else {
		room.Turn = black
	}
	room.UpdatedAt = time.Now().UnixMilli()
	return nil
}

func hasFive(board [boardSize][boardSize]int, x, y, stone int) bool {
	dirs := [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		count := 1
		count += countDir(board, x, y, d[0], d[1], stone)
		count += countDir(board, x, y, -d[0], -d[1], stone)
		if count >= 5 {
			return true
		}
	}
	return false
}

func countDir(board [boardSize][boardSize]int, x, y, dx, dy, stone int) int {
	count := 0
	for {
		x += dx
		y += dy
		if x < 0 || x >= boardSize || y < 0 || y >= boardSize || board[y][x] != stone {
			return count
		}
		count++
	}
}

func (s *Server) roomSnapshot(roomID string) (*Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return nil, false
	}
	copy := *room
	copy.Spectators = s.hub.SpectatorCount(roomID, room)
	return &copy, true
}

func (h *Hub) SpectatorCount(roomID string, room *Room) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for c := range h.rooms[roomID] {
		if !isPlayer(room, c.user.ID) {
			count++
		}
	}
	return count
}

func (s *Server) broadcastRoom(roomID string) {
	room, ok := s.roomSnapshot(roomID)
	if ok {
		s.hub.Broadcast(roomID, map[string]any{"type": "room", "room": room})
	}
}

func playerStone(room *Room, userID string) int {
	if room.Players[0] != nil && room.Players[0].ID == userID {
		return black
	}
	if room.Players[1] != nil && room.Players[1].ID == userID {
		return white
	}
	return empty
}

func isPlayer(room *Room, userID string) bool {
	return playerStone(room, userID) != empty
}

func (s *Server) withUser(next func(http.ResponseWriter, *http.Request, *User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("wl_session")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		s.mu.RLock()
		user := s.sessions[cookie.Value]
		s.mu.RUnlock()
		if user == nil {
			writeError(w, http.StatusUnauthorized, "登录已过期")
			return
		}
		next(w, r, user)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:n]
}

func trimRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func spaHandler(dist string) http.Handler {
	files := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}
		requestPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(filepath.Separator))
		path := filepath.Join(dist, requestPath)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
}
