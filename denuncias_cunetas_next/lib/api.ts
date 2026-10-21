export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public codigo?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
export function apiBase(): string {
  const value = process.env.NEXT_PUBLIC_API_URL?.trim().replace(/\/+$/, "");
  if (!value)
    throw new ApiError(
      "Configure NEXT_PUBLIC_API_URL en .env.local y reinicie Next.js.",
      0,
    );
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new ApiError("NEXT_PUBLIC_API_URL debe ser una URL absoluta.", 0);
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.search ||
    url.hash ||
    url.username ||
    url.password
  )
    throw new ApiError("Revise la URL base del backend.", 0);
  return value;
}
export const SESSION_INVALID = "cunetas:session-invalid";
export async function api<T>(
  path: string,
  options: RequestInit = {},
  csrf?: string,
  notifySession = true,
): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20000);
  const abort = () => controller.abort();
  options.signal?.addEventListener("abort", abort, { once: true });
  if (options.signal?.aborted) controller.abort();
  try {
    const headers = new Headers(options.headers);
    headers.set("Accept", "application/json");
    if (options.body && !(options.body instanceof FormData))
      headers.set("Content-Type", "application/json");
    if (csrf) headers.set("X-CSRF-Token", csrf);
    const response = await fetch(`${apiBase()}${path}`, {
      ...options,
      headers,
      credentials: "include",
      cache: "no-store",
      signal: controller.signal,
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) {
      if (
        notifySession &&
        (response.status === 401 ||
          data?.codigo === "CAMBIO_PASSWORD_REQUERIDO")
      )
        window.dispatchEvent(new Event(SESSION_INVALID));
      throw new ApiError(
        data?.error || "No fue posible completar la operación.",
        response.status,
        data?.codigo,
      );
    }
    if (data === null)
      throw new ApiError(
        "El servicio devolvió una respuesta incompleta.",
        response.status,
      );
    return data as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (controller.signal.aborted)
      throw new ApiError(
        "La petición se canceló o superó el tiempo de espera. Verifique el resultado antes de repetir una modificación.",
        0,
      );
    throw new ApiError(
      "No se pudo conectar con el servicio. Revise la conexión y los orígenes CORS del backend.",
      0,
    );
  } finally {
    clearTimeout(timeout);
    options.signal?.removeEventListener("abort", abort);
  }
}
export function message(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "Ocurrió un inconveniente inesperado.";
}
export function evidenceURL(path: string): string {
  const base = new URL(apiBase());
  const url = new URL(path, base.origin);
  if (
    url.origin !== base.origin ||
    !url.pathname.startsWith(`${base.pathname}/admin/gestion/denuncias/`)
  )
    throw new Error("La URL de evidencia no pertenece al backend configurado.");
  return url.href;
}
