"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, message, SESSION_INVALID } from "../lib/api";
import type { Sesion } from "../lib/types";
type Auth = {
  session: Sesion | null;
  status: "loading" | "ready" | "guest" | "error";
  error: string;
  refresh: () => Promise<void>;
  accept: (session: Sesion) => void;
  clear: () => void;
};
const Context = createContext<Auth | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Sesion | null>(null);
  const [status, setStatus] = useState<Auth["status"]>("loading");
  const [error, setError] = useState("");
  const generation = useRef(0);
  const clear = useCallback(() => {
    generation.current++;
    setSession(null);
    setStatus("guest");
    setError("");
  }, []);
  const accept = useCallback((value: Sesion) => {
    generation.current++;
    setSession(value);
    setStatus("ready");
    setError("");
  }, []);
  const refresh = useCallback(async () => {
    const version = ++generation.current;
    try {
      const value = await api<Sesion>("/admin/auth/me", {}, undefined, false);
      if (version === generation.current) {
        setSession(value);
        setStatus("ready");
        setError("");
      }
    } catch (error) {
      if (version !== generation.current) return;
      setSession(null);
      if (error instanceof ApiError && error.status === 401) {
        setStatus("guest");
        setError("");
      } else {
        setStatus("error");
        setError(message(error));
      }
    }
  }, []);
  useEffect(() => {
    void refresh();
    const invalid = () => {
      clear();
      void refresh();
    };
    const focus = () => {
      void refresh();
    };
    window.addEventListener(SESSION_INVALID, invalid);
    window.addEventListener("focus", focus);
    return () => {
      generation.current++;
      window.removeEventListener(SESSION_INVALID, invalid);
      window.removeEventListener("focus", focus);
    };
  }, [refresh, clear]);
  useEffect(() => {
    if (!session) return;
    const remaining = new Date(session.expira_en).getTime() - Date.now();
    if (!Number.isFinite(remaining)) {
      clear();
      return;
    }
    const timer = setTimeout(
      clear,
      Math.max(0, Math.min(remaining, 2147483647)),
    );
    return () => clearTimeout(timer);
  }, [session, clear]);
  return (
    <Context.Provider
      value={{ session, status, error, refresh, accept, clear }}
    >
      {children}
    </Context.Provider>
  );
}
export function useAuth() {
  const auth = useContext(Context);
  if (!auth) throw new Error("AuthProvider no está disponible.");
  return auth;
}
export function SessionCheck() {
  const auth = useAuth();
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 p-6">
      <section
        className="rounded-2xl border border-slate-200 bg-white shadow-sm w-full max-w-lg p-9 text-center [&>div]:mx-auto [&>div]:mb-6 [&>h1]:text-2xl"
        aria-live="polite"
      >
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#0A3D91] text-2xl font-extrabold text-white">
          C
        </div>
        <h1 className="mb-2.5 text-3xl font-extrabold leading-tight tracking-tight text-[#061B4E] sm:text-4xl">
          {auth.status === "error"
            ? "No pudimos verificar la sesión"
            : "Verificando sesión…"}
        </h1>
        {auth.status === "error" && (
          <>
            <p
              className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800"
              role="alert"
            >
              {auth.error}
            </p>
            <button
              className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 bg-[#061B4E] text-white enabled:hover:bg-[#0A3D91]"
              onClick={() => void auth.refresh()}
            >
              Volver a intentar
            </button>
          </>
        )}
      </section>
    </main>
  );
}
