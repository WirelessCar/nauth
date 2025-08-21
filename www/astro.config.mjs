// @ts-check

import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import starlightThemeFlexoki from "starlight-theme-flexoki";
import starlightLlmsTxt from "starlight-llms-txt";
import Icons from "unplugin-icons/vite";

// https://astro.build/config
export default defineConfig({
	site: "https://nauth.io",
	vite: {
		plugins: [Icons({ compiler: "astro" })],
	},
	integrations: [
		starlight({
			favicon: "./nauth.svg",
			plugins: [
				starlightThemeFlexoki({
					accentColor: "blue",
				}),
				starlightLlmsTxt(),
			],
			title: "Nauth",
			description: "Kubernetes operator for NATS decentralized authentication",
			logo: {
				src: "./public/nauth.svg",
			},
			social: [{ icon: "github", label: "GitHub", href: "https://github.com/wirelesscar/nauth" }],
			editLink: {
				baseUrl: "https://github.com/wirelesscar/nauth/edit/main/www/src/content/docs/",
			},
			customCss: [
				'./src/styles/custom.css',
			],
			credits: false,
			components: {
				Hero: "./src/components/CustomHero.astro",
				Footer: "./src/components/CustomFooter.astro",
			},
			sidebar: [
				{
					label: "Guides",
					items: [{ label: "Getting Started", slug: "guides/getting-started" }],
				},
				{
					label: "Reference",
					items: [{ label: "API Reference", slug: "crds" }],
				},
			],
		}),
	],
});
