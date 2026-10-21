export type Usuario = {
  id: number;
  usuario: string;
  nombres: string | null;
  requiere_cambio_password: boolean;
};
export type Sesion = {
  usuario: Usuario;
  csrf_token: string;
  expira_en: string;
  requiere_cambio_password: boolean;
};
export type Estado = {
  id: number;
  codigo: string;
  nombre: string;
  es_final: boolean;
};
export type Resumen = {
  id: number;
  numero_denuncia: string;
  tipo_denuncia: string;
  fecha_denuncia: string;
  descripcion: string;
  canton: { codigo: string; nombre: string };
  parroquia: string;
  sector: { codigo: string; nombre: string };
  barrio: string;
  estado: Estado;
  total_evidencias: number;
  fecha_creacion: string;
  fecha_actualizacion: string;
};
export type Pagina = {
  data: Resumen[];
  paginacion: {
    pagina: number;
    por_pagina: number;
    total_registros: number;
    total_paginas: number;
    hay_anterior: boolean;
    hay_siguiente: boolean;
  };
};
export type Detalle = Resumen & {
  denunciante: null | {
    tipo_nombre: string | null;
    identificacion: string | null;
    nombres_razon_social: string | null;
    telefono: string | null;
    email: string | null;
  };
  referencia_ubicacion: string | null;
  latitud: number;
  longitud: number;
  respuesta_final: string | null;
  problemas: { id: number; nombre: string }[];
  evidencias: {
    id: number;
    nombre_original: string;
    tipo_mime: string;
    tamano_bytes: number;
    url: string;
  }[];
  historial: {
    id: number;
    estado_nuevo: Estado;
    nombres: string | null;
    usuario: string | null;
    fecha: string;
    observacion: string | null;
    respuesta: string | null;
    es_publico: boolean;
  }[];
};
export function fecha(value: string, hour = false): string {
  const date = new Date(
    value.length === 10 ? `${value}T12:00:00-05:00` : value,
  );
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("es-EC", {
    timeZone: "America/Guayaquil",
    dateStyle: "medium",
    ...(hour ? { timeStyle: "short" as const } : {}),
  }).format(date);
}
