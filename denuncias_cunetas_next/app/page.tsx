"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";

import { api, message } from "../lib/api";
import type { Sesion } from "../lib/types";
import { useAuth, SessionCheck } from "../components/AuthProvider";
import { PasswordChange } from "../components/PasswordChange";

export default function LoginPage() {
  const auth = useAuth();
  const router = useRouter();

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [visible, setVisible] = useState(false);

  const requiereCambioPassword =
    auth.session?.requiere_cambio_password ||
    auth.session?.usuario.requiere_cambio_password;

  useEffect(() => {
    if (auth.status === "ready" && !requiereCambioPassword) {
      router.replace("/denuncias");
    }
  }, [auth.status, requiereCambioPassword, router]);

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (busy) return;

    const form = event.currentTarget;
    const data = new FormData(form);

    setError("");
    setBusy(true);

    try {
      const session = await api<Sesion>(
        "/admin/auth/login",
        {
          method: "POST",
          body: JSON.stringify({
            usuario: String(data.get("usuario") ?? "").trim(),
            password: String(data.get("password") ?? ""),
          }),
        },
        undefined,
        false,
      );

      form.reset();
      auth.accept(session);
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  }

  if (auth.status === "ready" && requiereCambioPassword) {
    return <PasswordChange />;
  }

  if (auth.status === "ready") {
    return <SessionCheck />;
  }

  return (
    <main className="grid min-h-screen bg-slate-100 lg:grid-cols-2">
      <aside className="flex flex-col justify-between gap-12 bg-[#061B4E] p-6 text-white lg:px-[10%] lg:py-12">
        <div className="flex items-center gap-3 text-xs font-extrabold tracking-wider">
          <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#0A3D91] text-2xl font-extrabold text-white">
            C
          </span>

          <span>GESTIÓN DE CUNETAS</span>
        </div>

        <div className="hidden lg:block">
          <p className="mb-5 max-w-md text-[11px] font-bold uppercase leading-6 tracking-[0.16em] text-blue-200">
            GAD PROVINCIAL DE SANTO DOMINGO DE LOS TSÁCHILAS
          </p>

          <h1 className="mb-6 text-5xl font-extrabold leading-tight tracking-tight text-white">
            Una mejor atención,
            <br />
            desde el primer reporte.
          </h1>

          <p className="max-w-md text-sm leading-8 text-blue-200">
            Revise las denuncias ciudadanas, coordine su atención y mantenga
            informado al ciudadano.
          </p>
        </div>

        <span className="hidden text-xs text-blue-200 lg:block">
          Administración institucional
        </span>
      </aside>

      <section className="flex items-center justify-center px-5 py-10 sm:p-10">
        <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-7 shadow-sm sm:p-9">
          <p className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[#0A3D91]">
            ACCESO ADMINISTRATIVO
          </p>

          <h2 className="mb-3 text-3xl font-bold leading-tight text-[#061B4E]">
            Bienvenido
          </h2>

          <p className="mb-6 text-sm leading-6 text-slate-500">
            Ingrese con su cuenta institucional.
          </p>

          <form onSubmit={login} aria-busy={busy}>
            <fieldset
              disabled={busy}
              className="m-0 flex min-w-0 flex-col gap-5 border-0 p-0"
            >
              <div>
                <label
                  htmlFor="usuario"
                  className="block text-[13px] font-semibold text-slate-700"
                >
                  Usuario
                </label>

                <input
                  id="usuario"
                  name="usuario"
                  type="text"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  maxLength={50}
                  placeholder="Ingrese su usuario"
                  required
                  autoFocus
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                />
              </div>

              <div>
                <label
                  htmlFor="password"
                  className="block text-[13px] font-semibold text-slate-700"
                >
                  Contraseña
                </label>

                <div className="relative mt-2">
                  <input
                    id="password"
                    name="password"
                    type={visible ? "text" : "password"}
                    autoComplete="current-password"
                    placeholder="Ingrese su contraseña"
                    required
                    className="min-h-11 w-full rounded-xl border border-slate-300 bg-white py-3 pl-3.5 pr-20 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                  />

                  <button
                    type="button"
                    aria-label={
                      visible ? "Ocultar contraseña" : "Mostrar contraseña"
                    }
                    aria-pressed={visible}
                    onClick={() => setVisible((value) => !value)}
                    className="absolute inset-y-1 right-2 cursor-pointer rounded-lg bg-transparent px-2 text-xs font-bold text-[#0A3D91] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {visible ? "Ocultar" : "Mostrar"}
                  </button>
                </div>
              </div>

              {error && (
                <p
                  role="alert"
                  className="whitespace-pre-wrap wrap-break-word rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800"
                >
                  {error}
                </p>
              )}

              <button
                type="submit"
                className="inline-flex min-h-11 w-full cursor-pointer items-center justify-center gap-2 rounded-xl bg-[#061B4E] px-4 py-3 text-sm font-bold text-white transition enabled:hover:bg-[#0A3D91] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {busy ? (
                  <>
                    <span
                      aria-hidden="true"
                      className="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white"
                    />
                    Ingresando…
                  </>
                ) : (
                  "Iniciar sesión →"
                )}
              </button>
            </fieldset>
          </form>

          <p className="mt-6 border-t border-slate-200 pt-6 text-xs leading-7 text-slate-500">
            Si necesita recuperar su acceso, comuníquese con el administrador
            del sistema.
          </p>
        </div>
      </section>
    </main>
  );
}