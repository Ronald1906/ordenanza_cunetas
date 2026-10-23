import { defineConfig } from "astro/config";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
	vite: {
		plugins: [tailwindcss()],
		server: {
			allowedHosts: [
				"ordenanza-cunetas.gptsachila.gob.ec",
			],
		},
		preview: {
			allowedHosts: [
				"ordenanza-cunetas.gptsachila.gob.ec",
			],
		},
	},
});