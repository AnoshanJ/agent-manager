import { useCallback, useState } from "react";

import { AgentError, sendMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Message } from "@/lib/types";
import { ChatHeader } from "@/components/ChatHeader";
import { Composer } from "@/components/Composer";
import { ConfigError } from "@/components/ConfigError";
import { LoginScreen } from "@/components/LoginScreen";
import { MessageList } from "@/components/MessageList";

export default function App() {
  const auth = useAuth();
  const [sessionId, setSessionId] = useState<string>(() => crypto.randomUUID());
  const [messages, setMessages] = useState<Message[]>([]);
  const [pending, setPending] = useState(false);

  const send = useCallback(
    async (text: string) => {
      setMessages((prev) => [...prev, { id: crypto.randomUUID(), role: "user", text }]);
      setPending(true);
      try {
        const token = await auth.getAccessToken();
        const reply = await sendMessage({ message: text, sessionId, token });
        setSessionId(reply.session_id);
        setMessages((prev) => [
          ...prev,
          { id: crypto.randomUUID(), role: "agent", text: reply.response },
        ]);
      } catch (err) {
        const failure = err instanceof AgentError ? err.message : (err as Error).message;
        setMessages((prev) => [
          ...prev,
          { id: crypto.randomUUID(), role: "error", text: failure },
        ]);
      } finally {
        setPending(false);
      }
    },
    [auth, sessionId],
  );

  if (auth.status === "error") return <ConfigError message={auth.error ?? "Unknown"} />;
  if (auth.status === "loading") return null;
  if (auth.status === "signed-out") return <LoginScreen />;

  return (
    <div className="mx-auto flex h-full max-w-app flex-col border-x border-border-subtle bg-surface">
      <ChatHeader />
      <MessageList messages={messages} pending={pending} onSuggestion={send} />
      <Composer disabled={pending} onSend={send} />
    </div>
  );
}
