// @ts-check

import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import starlightThemeFlexoki from "starlight-theme-flexoki";
import Icons from "unplugin-icons/vite";

// https://astro.build/config
export default defineConfig({
	site: "https://wirelesscar.github.io",
	base: "/nauth",
	publicDir: "../../assets",
	vite: {
		plugins: [Icons({ compiler: "astro" })],
	},
	integrations: [
		starlight({
			favicon: "./src/assets/nauth.svg",
			plugins: [
				starlightThemeFlexoki({
					accentColor: "blue",
				}),
			],
			title: "Nauth",
			description: "Kubernetes operator for NATS decentralized authentication",
			logo: {
				src: "./src/assets/nauth.svg",
			},
			social: [{ icon: "github", label: "GitHub", href: "https://github.com/wirelesscar/nauth" }],
			// Route middleware will handle all edit links
			routeMiddleware: './src/routeMiddleware.ts',
			// Disable default edit link - middleware will handle everything
			// editLink: {
			//   baseUrl: 'https://github.com/wirelesscar/nauth/edit/main/www/src/content/docs/',
			// },
			// Enable credits to show "Built with Starlight" in footer
			credits: true,
			components: {
				Hero: "./src/components/CustomHero.astro",
				Footer: "./src/components/CustomFooter.astro",
			},
			sidebar: [
				{
					label: "Getting Started",
					items: [
						{ label: "Getting Started", slug: "guides/getting-started" },
						{ label: "Installation", slug: "guides/installation" },
						{ label: "Basic Setup", slug: "guides/basic-setup" },
					],
				},
				{
					label: "User Guides",
					items: [
						{ label: "Account Management", slug: "guides/account-management" },
						{ label: "User Management", slug: "guides/user-management" },
						{ label: "Advanced Scenarios", slug: "guides/advanced-scenarios" },
					],
				},
				{
					label: "Reference",
					items: [
						{ label: "API Reference", slug: "crds" },
						{ label: "Quick Start", slug: "guides/quick-start" },
						{ label: "Troubleshooting", slug: "guides/troubleshooting" },
					],
				},
			],
		}),
	],
});