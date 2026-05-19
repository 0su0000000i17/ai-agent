import { useEffect, useRef, useState } from "react";
import "./App.css";
import {
  SendMessage,
  MinimizeHUD,
  ExpandHUD,
  CloseApp,
} from "../wailsjs/go/main/App";

type Message = {
  role: "assistant" | "user" | "error";
  text: string;
};

function App() {
  const [messages, setMessages] = useState<Message[]>([
    {
      role: "assistant",
      text: "JARVIS online.",
    },
  ]);

  const [input, setInput] = useState<string>("");
  const [busy, setBusy] = useState<boolean>(false);
  const [collapsed, setCollapsed] = useState<boolean>(false);

  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const logRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!collapsed) {
      inputRef.current?.focus();
    }
  }, [collapsed]);

  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [messages, collapsed]);

  async function send() {
    const text = input.trim();

    if (!text || busy) {
      return;
    }

    setInput("");
    setBusy(true);

    setMessages((prev) => [
      ...prev,
      {
        role: "user",
        text,
      },
    ]);

    try {
      const response = await SendMessage(text);

      setMessages((prev) => [
        ...prev,
        {
          role: "assistant",
          text: response || "Done.",
        },
      ]);
    } catch (err) {
      setMessages((prev) => [
        ...prev,
        {
          role: "error",
          text: String(err),
        },
      ]);
    } finally {
      setBusy(false);
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void send();
    }

    if (e.key === "Escape") {
      void collapse();
    }
  }

  async function collapse() {
    setCollapsed(true);
    await MinimizeHUD();
  }

  async function expand() {
    setCollapsed(false);
    await ExpandHUD();

    window.setTimeout(() => {
      inputRef.current?.focus();
    }, 80);
  }

  if (collapsed) {
    return (
      <div className="hud hud-collapsed">
        <div className="drag-region compact-drag">
          <div className="orb" />
          <div className="compact-title">JARVIS</div>
        </div>

        <button className="icon-btn" onClick={() => void expand()} title="Expand">
          ⌃
        </button>
      </div>
    );
  }

  return (
    <div className="hud">
      <div className="topbar drag-region">
        <div className="brand">
          <div className="orb" />
          <div>
            <div className="title">JARVIS</div>
            <div className="subtitle">
              {busy ? "thinking / acting..." : "computer core"}
            </div>
          </div>
        </div>

        <div className="window-actions">
          <button className="icon-btn" onClick={() => void collapse()} title="Collapse">
            —
          </button>
          <button className="icon-btn danger" onClick={() => void CloseApp()} title="Close">
            ×
          </button>
        </div>
      </div>

      <div className="log" ref={logRef}>
        {messages.map((message, index) => (
          <div key={index} className={`message ${message.role}`}>
            <div className="message-role">
              {message.role === "user" ? "you" : "jarvis"}
            </div>
            <pre>{message.text}</pre>
          </div>
        ))}
      </div>

      <div className="composer">
        <textarea
          ref={inputRef}
          value={input}
          placeholder="Ask JARVIS..."
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={onKeyDown}
          disabled={busy}
        />

        <button
          className="send-btn"
          onClick={() => void send()}
          disabled={busy || !input.trim()}
        >
          {busy ? "..." : "Send"}
        </button>
      </div>

      <div className="hint">
        Dev: /observe · /screenshot · /ui-after 3000 · /click x y · /type text · /hotkey ctrl+l
      </div>
    </div>
  );
}

export default App;