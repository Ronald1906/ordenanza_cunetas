import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AuthProvider } from "../components/AuthProvider";
import "./globals.css";
export const metadata: Metadata = {
  title: "Administración de denuncias | Cunetas",
  description: "Gestión administrativa de denuncias ciudadanas",
  robots: { index: false, follow: false },
};
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="es">
      <body className="m-0 bg-slate-100 font-sans text-sm leading-relaxed text-slate-800 antialiased">
        <AuthProvider>{children}</AuthProvider>
      </body>
    </html>
  );
}
