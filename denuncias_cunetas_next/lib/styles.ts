const estados: Record<string, string> = {
  REGISTRADA: "bg-blue-50 text-blue-800",
  EN_REVISION: "bg-amber-50 text-amber-800",
  EN_TRAMITE: "bg-violet-50 text-violet-800",
  ATENDIDA: "bg-teal-50 text-teal-800",
  CERRADA: "bg-green-50 text-green-800",
  DESCARTADA: "bg-rose-50 text-rose-800",
};
export function estadoClasses(codigo: string): string {
  return `inline-block whitespace-nowrap rounded-lg px-2.5 py-1.5 text-[10px] font-extrabold ${estados[codigo] || "bg-slate-100 text-slate-700"}`;
}
