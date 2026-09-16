export const loader = () => new Response(
  `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://threadify.dev/</loc></url>
  <url><loc>https://threadify.dev/pricing</loc></url>
</urlset>`,
  { headers: { "Content-Type": "application/xml; charset=utf-8" } },
);
