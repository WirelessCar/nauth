import { defineRouteMiddleware } from '@astrojs/starlight/route-data';

export const onRequest = defineRouteMiddleware((context) => {
  const route = context.locals.starlightRoute;
  
  if (route?.entry?.data) {
    const filePath = route.entry.id;
    
    // Check if this file is from symlinked content (guides directory or crds.md)
    if (filePath.startsWith('guides/')) {

      const correctEditUrl = `https://github.com/wirelesscar/nauth/edit/main/docs/${filePath}.md`;

      route.editUrl = new URL(correctEditUrl);
      route.entry.data.editUrl = correctEditUrl;
    } else if (filePath === 'crds') {

      const correctEditUrl = `https://github.com/wirelesscar/nauth/edit/main/docs/crds.md`;
      route.editUrl = new URL(correctEditUrl);
      route.entry.data.editUrl = correctEditUrl;
    } else if (filePath === '') {

      const correctEditUrl = `https://github.com/wirelesscar/nauth/edit/main/www/src/content/docs/index.mdx`;
      route.editUrl = new URL(correctEditUrl);
      route.entry.data.editUrl = correctEditUrl;
    } else {
      // For other content, disable edit link since we don't know where it should point
      route.editUrl = undefined;
      route.entry.data.editUrl = false;
    }
  }
});