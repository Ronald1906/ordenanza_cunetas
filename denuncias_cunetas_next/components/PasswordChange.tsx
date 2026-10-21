"use client";
import { useState, type FormEvent } from "react";
import { api, message } from "../lib/api";
import { useAuth } from "./AuthProvider";
export function PasswordChange() {
  const auth = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !auth.session) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    const password = String(data.get("password_nueva"));
    if (
      Array.from(password).length < 12 ||
      new TextEncoder().encode(password).length > 72
    ) {
      setError("Use al menos 12 caracteres y hasta 72 bytes.");
      return;
    }
    if (password !== data.get("confirmacion_password")) {
      setError("Las contraseñas nuevas no coinciden.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(
        "/admin/auth/cambiar-password",
        { method: "POST", body: JSON.stringify(Object.fromEntries(data)) },
        auth.session.csrf_token,
      );
      form.reset();
      setDone(true);
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 p-6">
      <section className="rounded-2xl border border-slate-200 bg-white shadow-sm w-full max-w-md p-7 sm:p-9 [&>p]:mb-6 [&>h2]:text-3xl">
        <p className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[#0A3D91]">
          SEGURIDAD DE LA CUENTA
        </p>
        <h1 className="mb-2.5 text-3xl font-extrabold leading-tight tracking-tight text-[#061B4E] sm:text-4xl">
          {done ? "Contraseña actualizada" : "Actualice su contraseña"}
        </h1>
        {done ? (
          <>
            <p className="mb-4 text-sm leading-6 text-slate-500">
              Las sesiones anteriores se cerraron. Ingrese con su nueva
              contraseña.
            </p>
            <button
              className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 bg-[#061B4E] text-white enabled:hover:bg-[#0A3D91] w-full"
              onClick={() => {
                auth.clear();
                window.location.assign("/");
              }}
            >
              Volver al inicio de sesión
            </button>
          </>
        ) : (
          <>
            <p className="mb-4 text-sm leading-6 text-slate-500">
              Antes de gestionar las denuncias debe cambiar su contraseña
              inicial.
            </p>
            <form onSubmit={submit}>
              <fieldset
                disabled={busy}
                className="m-0 flex min-w-0 flex-col gap-5 border-0 p-0"
              >
                <label className="block text-[13px] font-semibold text-slate-700">
                  Contraseña actual
                  <input
                    className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                    type="password"
                    name="password_actual"
                    autoComplete="current-password"
                    required
                  />
                </label>
                <label className="block text-[13px] font-semibold text-slate-700">
                  Nueva contraseña
                  <input
                    className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                    type="password"
                    name="password_nueva"
                    autoComplete="new-password"
                    minLength={12}
                    required
                  />
                </label>
                <label className="block text-[13px] font-semibold text-slate-700">
                  Confirmar nueva contraseña
                  <input
                    className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                    type="password"
                    name="confirmacion_password"
                    autoComplete="new-password"
                    minLength={12}
                    required
                  />
                </label>
                <small className="text-xs text-slate-500">
                  Al menos 12 caracteres. La contraseña conserva mayúsculas y
                  minúsculas.
                </small>
                {error && (
                  <p
                    role="alert"
                    className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800"
                  >
                    {error}
                  </p>
                )}
                <button
                  className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 bg-[#061B4E] text-white enabled:hover:bg-[#0A3D91] w-full"
                  type="submit"
                >
                  {busy ? "Actualizando…" : "Actualizar contraseña"}
                </button>
              </fieldset>
            </form>
          </>
        )}
      </section>
    </main>
  );
}
