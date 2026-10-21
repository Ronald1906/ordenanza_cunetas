"use client";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api, evidenceURL, message } from "../lib/api";
import { fecha, type Detalle, type Estado } from "../lib/types";
import { useAuth } from "./AuthProvider";
function Archivo({ item }: { item: Detalle["evidencias"][number] }) {
  let url: string;
  try {
    url = evidenceURL(item.url);
  } catch {
    return (
      <p className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800">
        No fue posible resolver la evidencia.
      </p>
    );
  }
  return (
    <figure className="m-0 overflow-hidden rounded-xl border border-slate-200 [&_img]:block [&_img]:h-52.5 [&_img]:w-full [&_img]:bg-slate-100 [&_img]:object-contain [&_video]:block [&_video]:h-52.5 [&_video]:w-full [&_video]:bg-slate-100 [&_video]:object-contain [&_figcaption]:wrap-break-word [&_figcaption]:p-3 [&_figcaption]:text-[11px] [&_a]:mt-1 [&_a]:block [&_a]:text-[#0A3D91]">
      {item.tipo_mime === "video/mp4" ? (
        <video
          controls
          preload="metadata"
          crossOrigin="use-credentials"
          src={url}
        />
      ) : (
        <img
          src={url}
          crossOrigin="use-credentials"
          alt={item.nombre_original}
          loading="lazy"
        />
      )}
      <figcaption>
        {item.nombre_original} · {(item.tamano_bytes / 1048576).toFixed(2)} MB{" "}
        <a href={url} target="_blank" rel="noopener noreferrer">
          Abrir archivo ↗
        </a>
      </figcaption>
    </figure>
  );
}
export function DenunciaDetalle({
  id,
  estados,
  onClose,
  onChanged,
}: {
  id: number;
  estados: Estado[];
  onClose: () => void;
  onChanged: () => void;
}) {
  const auth = useAuth();
  const dialog = useRef<HTMLDialogElement>(null);
  const request = useRef<AbortController | null>(null);
  const [data, setData] = useState<Detalle | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState("");
  const [estadoID, setEstadoID] = useState("");
  const [observacion, setObservacion] = useState("");
  const [respuesta, setRespuesta] = useState("");
  const [publico, setPublico] = useState(false);
  const target = estados.find((item) => String(item.id) === estadoID);
  const load = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    try {
      const result = await api<{ data: Detalle }>(
        `/admin/gestion/denuncias/${id}`,
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) {
        setData(result.data);
        setEstadoID("");
      }
    } catch (error) {
      if (!controller.signal.aborted) setError(message(error));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [id]);
  useEffect(() => {
    dialog.current?.showModal();
    void load();
    return () => request.current?.abort();
  }, [load]);
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!data || !auth.session || saving) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await api(
        `/admin/gestion/denuncias/${id}/estado`,
        {
          method: "PATCH",
          body: JSON.stringify({
            estado_id: Number(estadoID),
            estado_actual_id: data.estado.id,
            observacion: observacion.trim().toUpperCase(),
            respuesta: respuesta.trim().toUpperCase(),
            es_publico: publico,
          }),
        },
        auth.session.csrf_token,
      );
      setObservacion("");
      setRespuesta("");
      setPublico(false);
      setNotice("Estado actualizado correctamente.");
      onChanged();
      await load();
    } catch (error) {
      setError(message(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className="fixed inset-0 m-auto max-h-[90vh] w-[calc(100%-2rem)] max-w-250 overflow-y-auto rounded-2xl border border-slate-200 bg-white p-0 text-slate-800 shadow-2xl backdrop:bg-[#061B4E]/60 backdrop:backdrop-blur-sm"
      aria-labelledby="detail-title"
      onCancel={(event) => {
        event.preventDefault();
        if (!saving) onClose();
      }}
    >
      <header className="sticky top-0 z-10 flex items-center justify-between gap-5 border-b border-slate-200 bg-white p-5 sm:px-7 sm:py-6 [&_h2]:m-0 [&_h2]:text-xl [&_p]:text-[10px]">
        <div>
          <p className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[#0A3D91]">
            DETALLE DE LA DENUNCIA
          </p>
          <h2
            className="mb-3 text-2xl font-bold leading-tight text-[#061B4E]"
            id="detail-title"
          >
            {data?.numero_denuncia || "Cargando información…"}
          </h2>
        </div>
        <button
          className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 border-slate-300 bg-white text-slate-700 enabled:hover:bg-slate-100"
          disabled={saving}
          onClick={onClose}
          aria-label="Cerrar detalle"
        >
          Cerrar ✕
        </button>
      </header>
      <div className="p-5 sm:p-7" aria-busy={loading || saving}>
        {notice && (
          <p
            role="status"
            className="rounded-xl border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800"
          >
            {notice}
          </p>
        )}
        {error && (
          <div
            role="alert"
            className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm whitespace-pre-wrap wrap-break-word text-red-800"
          >
            {error}{" "}
            <button
              className="cursor-pointer border-0 bg-transparent font-bold text-inherit underline"
              disabled={saving}
              onClick={() => void load()}
            >
              Actualizar detalle
            </button>
          </div>
        )}
        {loading ? (
          <p className="mb-4 text-sm leading-6 text-slate-500" role="status">
            Consultando denuncia…
          </p>
        ) : (
          data && (
            <>
              <div className="flex flex-col items-start justify-between gap-2 text-xs text-slate-500 sm:flex-row sm:items-center">
                <span className={estadoClasses(data.estado.codigo)}>
                  {data.estado.nombre}
                </span>
                <span>
                  {fecha(data.fecha_denuncia)} · {data.tipo_denuncia}
                </span>
              </div>
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Problema reportado
                </h3>
                <div className="flex flex-wrap gap-2">
                  {data.problemas.map((item) => (
                    <span
                      key={item.id}
                      className="rounded-lg border border-blue-100 bg-blue-50 px-2.5 py-1 text-[11px] text-slate-700"
                    >
                      {item.nombre}
                    </span>
                  ))}
                </div>
                <p className="mt-4 whitespace-pre-wrap wrap-break-word text-slate-700">
                  {data.descripcion}
                </p>
              </section>
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Ubicación
                </h3>
                <dl className="m-0 grid grid-cols-1 gap-5 sm:grid-cols-2 [&_dt]:mb-1 [&_dt]:text-[11px] [&_dt]:text-slate-500 [&_dd]:m-0 [&_dd]:text-sm [&_dd]:wrap-break-word">
                  <div>
                    <dt>Cantón</dt>
                    <dd>{data.canton.nombre}</dd>
                  </div>
                  <div>
                    <dt>Parroquia</dt>
                    <dd>{data.parroquia}</dd>
                  </div>
                  <div>
                    <dt>Sector</dt>
                    <dd>{data.sector.nombre}</dd>
                  </div>
                  <div>
                    <dt>Barrio / recinto / comunidad</dt>
                    <dd>{data.barrio}</dd>
                  </div>
                  <div>
                    <dt>Referencia</dt>
                    <dd>{data.referencia_ubicacion || "Sin referencia"}</dd>
                  </div>
                  <div>
                    <dt>Coordenadas</dt>
                    <dd>
                      {data.latitud}, {data.longitud}
                    </dd>
                  </div>
                </dl>
              </section>
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Denunciante
                </h3>
                {data.denunciante ? (
                  <dl className="m-0 grid grid-cols-1 gap-5 sm:grid-cols-2 [&_dt]:mb-1 [&_dt]:text-[11px] [&_dt]:text-slate-500 [&_dd]:m-0 [&_dd]:text-sm [&_dd]:wrap-break-word">
                    <div>
                      <dt>Nombres o razón social</dt>
                      <dd>{data.denunciante.nombres_razon_social}</dd>
                    </div>
                    <div>
                      <dt>Cédula / RUC</dt>
                      <dd>{data.denunciante.identificacion}</dd>
                    </div>
                    <div>
                      <dt>Teléfono</dt>
                      <dd>{data.denunciante.telefono}</dd>
                    </div>
                    <div>
                      <dt>Correo electrónico</dt>
                      <dd>{data.denunciante.email || "No registrado"}</dd>
                    </div>
                  </dl>
                ) : (
                  <p className="mb-4 text-sm leading-6 text-slate-500">
                    Denuncia anónima. No se registraron datos personales.
                  </p>
                )}
              </section>
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Evidencias{" "}
                  <span className="text-sm font-normal text-slate-500">
                    ({data.evidencias.length})
                  </span>
                </h3>
                {data.evidencias.length ? (
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    {data.evidencias.map((item) => (
                      <Archivo key={item.id} item={item} />
                    ))}
                  </div>
                ) : (
                  <p className="mb-4 text-sm leading-6 text-slate-500">
                    No se adjuntaron imágenes ni videos.
                  </p>
                )}
              </section>
              {data.respuesta_final && (
                <section className="mt-7 rounded-xl border border-green-200 bg-green-50 p-5 [&_h3]:m-0 [&_h3]:text-green-800">
                  <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                    Respuesta final al ciudadano
                  </h3>
                  <p className="mt-4 whitespace-pre-wrap wrap-break-word text-slate-700">
                    {data.respuesta_final}
                  </p>
                </section>
              )}
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Historial de atención
                </h3>
                {data.historial.length ? (
                  <ol className="m-0 list-none p-0 [&>li]:ml-1 [&>li]:border-l-2 [&>li]:border-blue-100 [&>li]:pb-6 [&>li]:pl-5 [&>li:last-child]:pb-0 [&_p]:mt-2 [&_p]:text-xs [&_small]:text-[11px]">
                    {data.historial.map((item) => (
                      <li key={item.id}>
                        <div className="mb-1 flex flex-col items-start justify-between gap-1 sm:flex-row sm:items-center">
                          <strong>{item.estado_nuevo.nombre}</strong>
                          <small className="text-xs text-slate-500">
                            {fecha(item.fecha, true)}
                          </small>
                        </div>
                        <small className="text-xs text-slate-500">
                          {item.nombres || item.usuario || "REGISTRO CIUDADANO"}{" "}
                          ·{" "}
                          {item.es_publico
                            ? "Evento público"
                            : "Evento interno"}
                        </small>
                        {item.observacion && (
                          <p className="mt-4 whitespace-pre-wrap wrap-break-word text-slate-700">
                            <strong>
                              {item.respuesta || !item.es_publico
                                ? "Nota interna: "
                                : "Observación pública: "}
                            </strong>
                            {item.observacion}
                          </p>
                        )}
                        {item.respuesta && (
                          <p className="mt-4 whitespace-pre-wrap wrap-break-word text-slate-700">
                            <strong>Respuesta al ciudadano: </strong>
                            {item.respuesta}
                          </p>
                        )}
                      </li>
                    ))}
                  </ol>
                ) : (
                  <p className="mb-4 text-sm leading-6 text-slate-500">
                    Sin movimientos registrados.
                  </p>
                )}
              </section>
              <section className="border-b border-slate-200 py-7 last:border-0 last:pb-0">
                <h3 className="mb-4 text-lg font-bold leading-tight text-[#061B4E]">
                  Actualizar estado
                </h3>
                <form onSubmit={save}>
                  <fieldset
                    className="m-0 flex min-w-0 flex-col gap-5 border-0 p-0"
                    disabled={saving || loading}
                  >
                    <label className="block text-[13px] font-semibold text-slate-700">
                      Nuevo estado
                      <select
                        className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100"
                        required
                        value={estadoID}
                        onChange={(event) => setEstadoID(event.target.value)}
                      >
                        <option value="">Seleccione un estado</option>
                        {estados
                          .filter((item) => item.id !== data.estado.id)
                          .map((item) => (
                            <option key={item.id} value={item.id}>
                              {item.nombre}
                            </option>
                          ))}
                      </select>
                    </label>
                    <label className="block text-[13px] font-semibold text-slate-700">
                      Observación administrativa
                      <textarea
                        value={observacion}
                        onChange={(event) => setObservacion(event.target.value)}
                        maxLength={2000}
                        rows={3}
                        className="mt-2 w-full resize-y rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 uppercase"
                        placeholder="Detalle de la gestión realizada"
                      />
                    </label>
                    <label className="block text-[13px] font-semibold text-slate-700">
                      Respuesta al ciudadano{" "}
                      {target?.es_final && (
                        <span className="text-red-700">*</span>
                      )}
                      <textarea
                        value={respuesta}
                        onChange={(event) => setRespuesta(event.target.value)}
                        maxLength={4000}
                        rows={4}
                        required={target?.es_final}
                        className="mt-2 w-full resize-y rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 uppercase"
                        placeholder="Información que recibirá el ciudadano"
                      />
                    </label>
                    <small className="text-xs text-slate-500">
                      La respuesta siempre será pública. Al registrar una
                      respuesta, la observación administrativa se conserva como
                      nota interna.
                    </small>
                    {!respuesta.trim() && (
                      <label className="flex items-start gap-3 text-sm font-normal [&_input]:mt-1 [&_input]:h-4 [&_input]:min-h-0 [&_input]:w-4 [&_input]:shrink-0 [&_input]:accent-[#0A3D91]">
                        <input
                          className="mt-2 min-h-11 w-full rounded-xl border border-slate-300 bg-white px-3.5 py-3 text-sm text-slate-800 outline-none transition placeholder:text-slate-400 focus:border-[#0A3D91] focus:ring-4 focus:ring-blue-100 disabled:opacity-60"
                          type="checkbox"
                          checked={publico}
                          onChange={(event) => setPublico(event.target.checked)}
                        />{" "}
                        Publicar la observación en el seguimiento ciudadano
                      </label>
                    )}
                    <div className="flex justify-end">
                      <button
                        type="submit"
                        className="inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-xl border px-4 py-2.5 text-[13px] font-bold no-underline transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 bg-[#061B4E] text-white enabled:hover:bg-[#0A3D91]"
                        disabled={!estadoID || !estados.length}
                      >
                        {saving ? "Guardando…" : "Guardar cambio de estado"}
                      </button>
                    </div>
                  </fieldset>
                </form>
              </section>
            </>
          )
        )}
      </div>
    </dialog>
  );
}
