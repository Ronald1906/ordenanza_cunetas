"use client";
import { estadoClasses } from "../../lib/styles";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "../../components/AuthProvider";
import { DenunciaDetalle } from "../../components/DenunciaDetalle";
import { api, message } from "../../lib/api";
import { fecha, type Estado, type Pagina } from "../../lib/types";
const emptyFilters = {
  search: "",
  parroquia: "",
  sector: "",
  barrio: "",
  estado_id: "",
};
export default function DenunciasPage() {
  const auth = useAuth();
  const router = useRouter();
  const [draft, setDraft] = useState(emptyFilters);
  const [filters, setFilters] = useState(emptyFilters);
  const [page, setPage] = useState(1);
  const [revision, setRevision] = useState(0);
  const [result, setResult] = useState<Pagina | null>(null);
  const [estados, setEstados] = useState<Estado[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [catalogError, setCatalogError] = useState("");
  const [loggingOut, setLoggingOut] = useState(false);
  const [selected, setSelected] = useState<number | null>(null);
  const reload = useCallback(() => setRevision((value) => value + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    setCatalogError("");
    void api<{ data: Estado[] }>("/admin/gestion/denuncias/estados", {
      signal: controller.signal,
    })
      .then((value) => {
        if (!controller.signal.aborted) setEstados(value.data);
      })
      .catch((error) => {
        if (!controller.signal.aborted) setCatalogError(message(error));
      });
    return () => controller.abort();
  }, [revision]);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setResult(null);
    const params = new URLSearchParams({ page: String(page) });
    Object.entries(filters).forEach(([key, value]) => {
      if (value.trim()) params.set(key, value.trim());
    });
    void api<Pagina>(`/admin/gestion/denuncias/?${params}`, {
      signal: controller.signal,
    })
      .then((value) => {
        if (!controller.signal.aborted) setResult(value);
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(message(error));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [page, filters, revision]);
  function filter(event: FormEvent) {
    event.preventDefault();
    setPage(1);
    setFilters({ ...draft });
  }
  async function logout() {
    if (!auth.session || loggingOut) return;
    setLoggingOut(true);
    setError("");
    try {
      await api(
        "/admin/auth/logout",
        { method: "POST" },
        auth.session.csrf_token,
      );
      auth.clear();
      router.replace("/");
    } catch (error) {
      setError(message(error));
    } finally {
      setLoggingOut(false);
    }
  }
  return (
    <div className="min-h-screen bg-slate-100">
      <header className="flex items-start justify-between gap-4 border-b border-slate-200 bg-white px-4 py-5 sm:items-center sm:px-[4%]">
        <a
          href="/denuncias"
          className="flex items-center gap-3 text-xs font-extrabold tracking-wider text-[#061B4E] no-underline [&_small]:mt-0.5 [&_small]:block [&_small]:text-[11px] [&_small]:font-normal [&_small]:tracking-normal"
        >
          <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#0A3D91] text-2xl font-extrabold text-white">
            C
          </span>
          <span>
            GESTIÓN DE CUNETAS
            <small className="text-xs text-slate-500">
              Administración de denuncias
            </small>
          </span>
        </a>
        <div className="flex items-center gap-5 [&>span]:hidden sm:[&>span]:block [&>span]:text-right [&>span]:text-xs [&>span]:font-bold [&_small]:block [&_small]:font-normal">
          <span>
            {auth.session?.usuario.nombres || auth.session?.usuario.usuario}
            <small className="text-xs text-slate-500">
              Sesión administrativa
            </small>
          </span>
          <button
            className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
            onClick={() => void logout()}
            disabled={loggingOut}
          >
            {loggingOut ? "Cerrando…" : "Cerrar sesión"}
          </button>
        </div>
      </header>
      <main className="mx-auto max-w-370 px-4 py-7 sm:px-[4%] sm:py-9">
        <div className="mb-7 flex items-start justify-between gap-5 sm:items-center [&_p]:mb-0">
          <div>
            <p className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[#0A3D91]">
              ATENCIÓN CIUDADANA
            </p>
            <h1 className="mb-2.5 text-3xl font-extrabold leading-tight tracking-tight text-[#061B4E] sm:text-4xl">
              Denuncias
            </h1>
            <p className="mb-4 text-sm leading-6 text-slate-500">
              Consulte los reportes y dé seguimiento a su atención.
            </p>
          </div>
          <button
            className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
            onClick={reload}
            disabled={loading}
          >
            ↻ Actualizar
          </button>
        </div>
        <section
          className="rounded-2xl border border-slate-200 bg-white shadow-sm mb-6 p-5 sm:p-6"
          aria-label="Filtros de denuncias"
        >
          <form onSubmit={filter}>
            <div className="grid grid-cols-2 gap-4 xl:grid-cols-[2fr_1.2fr_1fr_1.2fr_1.2fr]">
              <label className="col-span-2 xl:col-span-1">
                Buscar denuncia
                <input
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                  type="search"
                  maxLength={200}
                  placeholder="Número o contenido de la descripción"
                  value={draft.search}
                  onChange={(event) =>
                    setDraft({ ...draft, search: event.target.value })
                  }
                />
              </label>
              <label className="block text-[13px] font-semibold text-slate-700">
                Parroquia
                <input
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                  maxLength={150}
                  placeholder="Todas las parroquias"
                  value={draft.parroquia}
                  onChange={(event) =>
                    setDraft({ ...draft, parroquia: event.target.value })
                  }
                />
              </label>
              <label className="block text-[13px] font-semibold text-slate-700">
                Sector
                <select
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100"
                  value={draft.sector}
                  onChange={(event) =>
                    setDraft({ ...draft, sector: event.target.value })
                  }
                >
                  <option value="">Todos los sectores</option>
                  <option value="URBANO">URBANO</option>
                  <option value="RURAL">RURAL</option>
                </select>
              </label>
              <label className="block text-[13px] font-semibold text-slate-700">
                Barrio / recinto
                <input
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                  maxLength={200}
                  placeholder="Todos los barrios"
                  value={draft.barrio}
                  onChange={(event) =>
                    setDraft({ ...draft, barrio: event.target.value })
                  }
                />
              </label>
              <label className="block text-[13px] font-semibold text-slate-700">
                Estado
                <select
                  className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100"
                  value={draft.estado_id}
                  onChange={(event) =>
                    setDraft({ ...draft, estado_id: event.target.value })
                  }
                >
                  <option value="">Todos los estados</option>
                  {estados.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.nombre}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="mt-5 flex flex-wrap items-center justify-end gap-2.5 border-t border-slate-100 pt-5 [&>span]:mr-auto [&>span]:w-full [&>span]:text-xs [&>span]:text-slate-500 sm:[&>span]:w-auto">
              <span>10 denuncias por página</span>
              <button
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
                type="button"
                onClick={() => {
                  setDraft(emptyFilters);
                  setFilters({ ...emptyFilters });
                  setPage(1);
                }}
              >
                Limpiar filtros
              </button>
              <button
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 bg-[#061B4E] text-white enabled:hover:bg-[#0A3D91]"
                type="submit"
              >
                Aplicar filtros
              </button>
            </div>
          </form>
        </section>
        {catalogError && (
          <p
            role="alert"
            className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800"
          >
            No se cargaron los estados: {catalogError}{" "}
            <button
              className="cursor-pointer border-0 bg-transparent font-bold text-inherit underline"
              onClick={reload}
            >
              Reintentar
            </button>
          </p>
        )}
        {error && (
          <p
            role="alert"
            className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800"
          >
            {error}{" "}
            <button
              className="cursor-pointer border-0 bg-transparent font-bold text-inherit underline"
              onClick={reload}
            >
              Volver a intentar
            </button>
          </p>
        )}
        <section
          className="rounded-2xl border border-slate-200 bg-white shadow-sm overflow-hidden"
          aria-label="Listado de denuncias"
          aria-busy={loading}
        >
          <div className="flex items-center justify-between gap-4 px-6 py-5 [&>h2]:m-0 [&>h2]:text-base [&>span]:text-xs [&>span]:text-slate-500">
            <h2 className="mb-3 text-2xl font-bold leading-tight text-[#061B4E]">
              Reportes registrados
            </h2>
            <span>
              {loading
                ? "Consultando…"
                : `${result?.paginacion.total_registros ?? 0} resultados`}
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-250 border-collapse text-left">
              <thead>
                <tr className="hover:bg-slate-50/70">
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    Número / fecha
                  </th>
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    Descripción
                  </th>
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    Ubicación
                  </th>
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    Estado
                  </th>
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    Evidencias
                  </th>
                  <th className="border-y border-slate-200 bg-slate-50 px-5 py-3.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr className="hover:bg-slate-50/70">
                    <td
                      colSpan={6}
                      className="px-6 py-16 text-center text-slate-500"
                      role="status"
                    >
                      Cargando denuncias…
                    </td>
                  </tr>
                ) : result?.data.length ? (
                  result.data.map((item) => (
                    <tr className="hover:bg-slate-50/70" key={item.id}>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        <strong className="whitespace-nowrap font-bold text-[#0A3D91]">
                          {item.numero_denuncia}
                        </strong>
                        <small className="text-xs text-slate-500">
                          {fecha(item.fecha_denuncia)}
                        </small>
                      </td>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        <p className="m-0 line-clamp-2 min-w-47.5 max-w-72.5 text-slate-700">
                          {item.descripcion}
                        </p>
                        <small className="text-xs text-slate-500">
                          {item.tipo_denuncia}
                        </small>
                      </td>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        <strong>{item.parroquia}</strong>
                        <small className="text-xs text-slate-500">
                          {item.barrio} · {item.sector.nombre}
                        </small>
                      </td>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        <span className={estadoClasses(item.estado.codigo)}>
                          {item.estado.nombre}
                        </span>
                      </td>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        {item.total_evidencias} archivo
                        {item.total_evidencias === 1 ? "" : "s"}
                      </td>
                      <td className="border-b border-slate-100 px-5 py-5 align-middle text-xs [&_small]:mt-1.5 [&_small]:block [&_small]:max-w-55">
                        <button
                          className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
                          onClick={() => setSelected(item.id)}
                        >
                          Ver detalle →
                        </button>
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr className="hover:bg-slate-50/70">
                    <td
                      colSpan={6}
                      className="px-6 py-16 text-center text-slate-500"
                    >
                      {error
                        ? "No fue posible consultar las denuncias."
                        : "No se encontraron denuncias con los filtros seleccionados."}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <nav
            className="flex flex-wrap items-center justify-between gap-4 px-5 py-4 text-xs text-slate-500 [&>div]:flex [&>div]:gap-2.5"
            aria-label="Paginación"
          >
            <span>
              {result?.paginacion.total_registros
                ? `Página ${page} de ${result.paginacion.total_paginas}`
                : "Sin resultados"}
            </span>
            <div>
              <button
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
                disabled={loading || !result?.paginacion.hay_anterior}
                onClick={() => setPage((value) => value - 1)}
              >
                ← Anterior
              </button>
              <button
                className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
                disabled={loading || !result?.paginacion.hay_siguiente}
                onClick={() => setPage((value) => value + 1)}
              >
                Siguiente →
              </button>
            </div>
          </nav>
        </section>
        <footer className="mt-8 text-center text-[11px] text-slate-500">
          GAD Provincial de Santo Domingo de los Tsáchilas · Gestión de
          denuncias ciudadanas
        </footer>
      </main>
      {selected !== null && (
        <DenunciaDetalle
          key={selected}
          id={selected}
          estados={estados}
          onClose={() => setSelected(null)}
          onChanged={reload}
        />
      )}
    </div>
  );
}
