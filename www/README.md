# Nauth Docs Site

[![Built with Starlight](https://astro.badg.es/v2/built-with-starlight/tiny.svg)](https://starlight.astro.build)


## 🚀 Project Structure

```
.
├── public/
├── src/
│   ├── assets/
│   ├── content/
│   │   └── docs/
│   └── content.config.ts
├── astro.config.mjs
├── package.json
└── tsconfig.json
```

Starlight looks for `.md` or `.mdx` files in the `src/content/docs/` directory. Each file is exposed as a route based on its file name.

Images can be added to `src/assets/` and embedded in Markdown with a relative link.

Static assets, like favicons, can be placed in the `public/` directory.

## Symlinks

In order to keep the docs folder in the root of the project, we add symlinks like so:

```
/assets/                        (main assets)
└── nauth.png

/docs/                          (main documentation)
├── guides/
├── reference/
└── crds.md

/www/src/                       (Starlight/Astro structure)
├── assets/ → ../../assets      (symlink to main assets)
├── content/
│   ├── docs/
│   │   ├── index.mdx          (landing page)
│   │   ├── guides/ → ../../../../docs/guides (symlink)
│   │   ├── reference/ → ../../../../docs/reference (symlink)
│   │   └── crds.md → ../../../../docs/crds.md (symlink)
│   └── content.config.ts

## 🧞 Commands

All commands are run from the root of the project, from a terminal:

| Command                   | Action                                           |
| :------------------------ | :----------------------------------------------- |
| `bun install`             | Installs dependencies                            |
| `bun dev`             | Starts local dev server at `localhost:4321`      |
| `bun build`           | Build your production site to `./dist/`          |
| `bun preview`         | Preview your build locally, before deploying     |
| `bun astro ...`       | Run CLI commands like `astro add`, `astro check` |
| `bun astro -- --help` | Get help using the Astro CLI                     |
