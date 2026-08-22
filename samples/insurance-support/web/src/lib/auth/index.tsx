import { createContext, useContext, type ReactNode } from "react";

import { CONFIG, configError } from "@/lib/config";
import { useNoneAuth } from "./none";
import { useOidcAuth } from "./oidc";
import type { Auth } from "./types";

const AuthContext = createContext<Auth | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const none = useNoneAuth();
  const oidc = useOidcAuth();
  const problem = configError();

  let value: Auth = CONFIG.authMode === "oidc" ? oidc : none;
  if (problem) {
    value = { ...value, status: "error", error: problem };
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): Auth {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

export type { Auth, AuthStatus } from "./types";
