import type { LoaderFunctionArgs } from "@remix-run/node";

/**
 * Sitemap generator for Threadify
 * This is a Remix resource route that returns a valid sitemap.xml
 */
export async function loader({ request }: LoaderFunctionArgs) {
  const baseUrl = "https://threadify.dev";

  // Define public routes that should be indexed
  const publicRoutes = [
    "",
    "/pricing",
    "/login",
    "/signup",
    "/auth/forgot-password",
  ];

  const sitemap = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  ${publicRoutes
    .map((route) => {
      return `
  <url>
    <loc>${baseUrl}${route}</loc>
    <lastmod>${new Date().toISOString().split('T')[0]}</lastmod>
    <changefreq>daily</changefreq>
    <priority>${route === "" ? "1.0" : "0.8"}</priority>
  </url>`;
    })
    .join("")}
</urlset>`;

  return new Response(sitemap, {
    headers: {
      "Content-Type": "application/xml",
      "Cache-Control": "public, max-age=3600",
    },
  });
}
