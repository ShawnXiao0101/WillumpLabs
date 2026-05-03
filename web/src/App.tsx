import { Circle, Crown, LogIn, Plus, RefreshCw, Swords, Users } from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";

const BOARD_SIZE = 15;
const EMPTY = 0;
const BLACK = 1;
const WHITE = 2;

type User = {
  id: string;
  name: string;
  createdAt: number;
};

type Move = {
  x: number;
  y: number;
  stone: number;
  userId: string;
};

type Room = {
  id: string;
  name: string;
  board: number[][];
  players: Array<User | null>;
  spectators: number;
  turn: number;
  winner: number;
  draw: boolean;
  moves: Move[] | null;
  createdAt: number;
  updatedAt: number;
};

type Toast = {
  kind: "good" | "bad" | "info";
  text: string;
};

export function App() {
  const [user, setUser] = useState<User | null>(null);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [activeRoom, setActiveRoom] = useState<Room | null>(null);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<Toast | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  const notify = useCallback((text: string, kind: Toast["kind"] = "info") => {
    setToast({ text, kind });
    window.setTimeout(() => setToast(null), 2600);
  }, []);

  const request = useCallback(async <T,>(path: string, options?: RequestInit): Promise<T> => {
    const response = await fetch(path, {
      credentials: "include",
      headers: { "Content-Type": "application/json", ...(options?.headers ?? {}) },
      ...options,
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error ?? "请求失败");
    }
    return payload as T;
  }, []);

  const refreshRooms = useCallback(async () => {
    if (!user) return;
    const payload = await request<{ rooms: Room[] }>("/api/rooms");
    setRooms(payload.rooms);
  }, [request, user]);

  useEffect(() => {
    request<{ user: User }>("/api/me")
      .then((payload) => setUser(payload.user))
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, [request]);

  useEffect(() => {
    if (!user) return;
    refreshRooms().catch((error) => notify(error.message, "bad"));
    const timer = window.setInterval(() => {
      refreshRooms().catch(() => undefined);
    }, 4500);
    return () => window.clearInterval(timer);
  }, [notify, refreshRooms, user]);

  const connectRoom = useCallback(
    (room: Room) => {
      socketRef.current?.close();
      setActiveRoom(room);
      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const socket = new WebSocket(`${protocol}://${window.location.host}/ws/rooms/${room.id}`);
      socketRef.current = socket;
      socket.onmessage = (event) => {
        const payload = JSON.parse(event.data);
        if (payload.type === "room") {
          setActiveRoom(payload.room);
          setRooms((current) => [payload.room, ...current.filter((item) => item.id !== payload.room.id)]);
        }
        if (payload.type === "error") {
          notify(payload.message, "bad");
        }
      };
      socket.onclose = () => {
        if (socketRef.current === socket) socketRef.current = null;
      };
    },
    [notify],
  );

  const login = async (name: string) => {
    const payload = await request<{ user: User }>("/api/login", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    setUser(payload.user);
    notify(`欢迎，${payload.user.name}`, "good");
  };

  const createRoom = async (name: string) => {
    const payload = await request<{ room: Room }>("/api/rooms", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    await refreshRooms();
    connectRoom(payload.room);
  };

  const joinRoom = async (room: Room) => {
    try {
      const payload = await request<{ room: Room }>(`/api/rooms/${room.id}/join`, { method: "POST" });
      connectRoom(payload.room);
      notify("已加入对局", "good");
    } catch (error) {
      connectRoom(room);
      notify(error instanceof Error ? error.message : "以观众身份进入", "info");
    }
  };

  const makeMove = (x: number, y: number) => {
    if (!socketRef.current || socketRef.current.readyState !== WebSocket.OPEN) {
      notify("连接正在建立，请稍等", "info");
      return;
    }
    socketRef.current.send(JSON.stringify({ type: "move", data: { x, y } }));
  };

  const restart = async () => {
    if (!activeRoom) return;
    try {
      const payload = await request<{ room: Room }>(`/api/rooms/${activeRoom.id}/restart`, { method: "POST" });
      setActiveRoom(payload.room);
      notify("新的一局开始了", "good");
    } catch (error) {
      notify(error instanceof Error ? error.message : "重开失败", "bad");
    }
  };

  if (loading) return <Shell><div className="loading">加载棋盘中...</div></Shell>;
  if (!user) return <Shell><LoginPanel onLogin={login} /></Shell>;

  return (
    <Shell toast={toast}>
      <header className="topbar">
        <div>
          <p className="eyebrow">WillumpLabs Gomoku</p>
          <h1>在线五子棋</h1>
        </div>
        <div className="profile">
          <span>{user.name}</span>
          <Circle size={12} fill="#3ddc97" color="#3ddc97" />
        </div>
      </header>

      <main className="layout">
        <section className="lobby">
          <RoomCreator onCreate={createRoom} />
          <div className="section-title">
            <h2>大厅</h2>
            <button className="icon-button" onClick={() => refreshRooms()} aria-label="刷新房间">
              <RefreshCw size={18} />
            </button>
          </div>
          <div className="room-list">
            {rooms.length === 0 ? (
              <div className="empty-state">还没有棋局，创建一个房间开始第一盘。</div>
            ) : (
              rooms.map((room) => (
                <button
                  className={`room-card ${activeRoom?.id === room.id ? "active" : ""}`}
                  key={room.id}
                  onClick={() => joinRoom(room)}
                >
                  <span className="room-name">{room.name}</span>
                  <span className="room-meta">
                    <Users size={15} /> {room.players.filter(Boolean).length}/2
                    <Swords size={15} /> {room.moves?.length ?? 0} 手
                  </span>
                </button>
              ))
            )}
          </div>
        </section>

        <GamePanel room={activeRoom} user={user} onMove={makeMove} onRestart={restart} />
      </main>
    </Shell>
  );
}

function Shell({ children, toast }: { children: React.ReactNode; toast?: Toast | null }) {
  return (
    <div className="app-shell">
      {children}
      {toast && <div className={`toast ${toast.kind}`}>{toast.text}</div>}
    </div>
  );
}

function LoginPanel({ onLogin }: { onLogin: (name: string) => Promise<void> }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await onLogin(name);
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="login-page">
      <section className="login-hero">
        <p className="eyebrow">实时对战</p>
        <h1>WillumpLabs 五子棋</h1>
        <p>进入大厅，创建房间，和朋友在同一张棋盘上实时交锋。</p>
      </section>
      <form className="login-form" onSubmit={submit}>
        <label htmlFor="name">昵称</label>
        <input
          id="name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="输入 2-16 个字符"
          maxLength={16}
        />
        {error && <p className="form-error">{error}</p>}
        <button className="primary-button" disabled={busy}>
          <LogIn size={18} />
          {busy ? "进入中" : "进入大厅"}
        </button>
      </form>
    </main>
  );
}

function RoomCreator({ onCreate }: { onCreate: (name: string) => Promise<void> }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await onCreate(name);
      setName("");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="room-creator" onSubmit={submit}>
      <input value={name} onChange={(event) => setName(event.target.value)} placeholder="房间名，可留空" maxLength={28} />
      <button className="primary-button compact" disabled={busy}>
        <Plus size={17} />
        创建
      </button>
    </form>
  );
}

function GamePanel({
  room,
  user,
  onMove,
  onRestart,
}: {
  room: Room | null;
  user: User;
  onMove: (x: number, y: number) => void;
  onRestart: () => void;
}) {
  if (!room) {
    return (
      <section className="game-panel idle">
        <div>
          <p className="eyebrow">选择棋局</p>
          <h2>从大厅加入一盘棋，或创建新的房间。</h2>
        </div>
      </section>
    );
  }

  const role = playerStone(room, user.id);
  const latest = room.moves?.[room.moves.length - 1] ?? null;
  const status = gameStatus(room, role);

  return (
    <section className="game-panel">
      <div className="game-header">
        <div>
          <p className="eyebrow">{room.name}</p>
          <h2>{status}</h2>
        </div>
        <button className="ghost-button" onClick={onRestart}>
          <RefreshCw size={17} />
          重开
        </button>
      </div>

      <div className="players">
        <PlayerSeat label="黑棋" user={room.players[0]} active={room.turn === BLACK && !room.winner} winner={room.winner === BLACK} />
        <PlayerSeat label="白棋" user={room.players[1]} active={room.turn === WHITE && !room.winner} winner={room.winner === WHITE} />
      </div>

      <Board board={room.board} latest={latest} disabled={role !== room.turn || !!room.winner || room.draw} onMove={onMove} />

      <div className="game-footer">
        <span>你是 {stoneName(role)}</span>
        <span>观众 {room.spectators}</span>
        <span>{room.moves?.length ?? 0} 手</span>
      </div>
    </section>
  );
}

function PlayerSeat({ label, user, active, winner }: { label: string; user: User | null; active: boolean; winner: boolean }) {
  return (
    <div className={`player-seat ${active ? "active" : ""} ${winner ? "winner" : ""}`}>
      <span className={`stone ${label === "黑棋" ? "black" : "white"}`} />
      <div>
        <strong>{user?.name ?? "等待加入"}</strong>
        <small>{label}</small>
      </div>
      {winner && <Crown size={18} />}
    </div>
  );
}

function Board({
  board,
  latest,
  disabled,
  onMove,
}: {
  board: number[][];
  latest: Move | null;
  disabled: boolean;
  onMove: (x: number, y: number) => void;
}) {
  const cells = useMemo(() => {
    const list: Array<{ x: number; y: number; value: number }> = [];
    for (let y = 0; y < BOARD_SIZE; y++) {
      for (let x = 0; x < BOARD_SIZE; x++) {
        list.push({ x, y, value: board[y]?.[x] ?? EMPTY });
      }
    }
    return list;
  }, [board]);

  return (
    <div className={`board ${disabled ? "disabled" : ""}`} aria-label="五子棋棋盘">
      {cells.map((cell) => {
        const isLatest = latest?.x === cell.x && latest?.y === cell.y;
        return (
          <button
            className={`cell ${isLatest ? "latest" : ""}`}
            key={`${cell.x}-${cell.y}`}
            onClick={() => onMove(cell.x, cell.y)}
            disabled={cell.value !== EMPTY}
            aria-label={`${cell.x + 1},${cell.y + 1}`}
          >
            {cell.value !== EMPTY && <span className={`piece ${cell.value === BLACK ? "black" : "white"}`} />}
          </button>
        );
      })}
    </div>
  );
}

function playerStone(room: Room, userID: string) {
  if (room.players[0]?.id === userID) return BLACK;
  if (room.players[1]?.id === userID) return WHITE;
  return EMPTY;
}

function stoneName(stone: number) {
  if (stone === BLACK) return "黑棋";
  if (stone === WHITE) return "白棋";
  return "观众";
}

function gameStatus(room: Room, role: number) {
  if (room.winner) return `${stoneName(room.winner)}获胜`;
  if (room.draw) return "平局";
  if (!room.players[1]) return "等待第二位玩家加入";
  if (role === EMPTY) return `正在观战，当前 ${stoneName(room.turn)} 落子`;
  if (role === room.turn) return "轮到你了";
  return `等待 ${stoneName(room.turn)} 落子`;
}
