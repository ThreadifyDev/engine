import {
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  useLoaderData,
} from "@remix-run/react";
import type { LinksFunction } from "@remix-run/node";
import { json } from "@remix-run/node";
import stylesheet from "~/styles/tailwind.css?url";
import { QueryProvider } from "~/lib/query-client";
import { getConfig } from "./config.server";

export const links: LinksFunction = () => [
  { rel: "stylesheet", href: stylesheet },
  { rel: "icon", type: "image/svg+xml", href: "/favicon-black.svg" },
];

export const loader = async () => {
  const config = getConfig();
  
  return json({
    ENV: {
      API_URL: config.apiUrl,
    },
  });
};

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <Meta />
        <Links />
      </head>
      <body className="bg-background text-foreground font-sans antialiased">
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  const data = useLoaderData<typeof loader>();
  
  return (
    <QueryProvider>
      {/* Inject ENV into window for client-side access */}
      <script
        dangerouslySetInnerHTML={{
          __html: `window.__ENV__ = ${JSON.stringify(data.ENV)}`,
        }}
      />
      <Outlet />
    </QueryProvider>
  );
}
