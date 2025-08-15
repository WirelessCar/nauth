import { defineCollection, z } from "astro:content";
import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";

export const collections = {
	docs: defineCollection({
		loader: docsLoader(),
		schema: docsSchema({
			extend: z.object({
				// Extend hero configuration to support component slots
				heroExtended: z
					.object({
						taglineParts: z
							.array(
								z.union([
									z.string(),
									z.object({
										type: z.enum(["text", "nats-icon", "k8s-icon", "component"]),
										text: z.string().optional(),
										component: z.string().optional(),
									}),
								])
							)
							.optional(),
					})
					.optional(),
			}),
		}),
	}),
};
