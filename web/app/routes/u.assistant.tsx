import { redirect } from '@remix-run/node';

// AI runs as an external MCP client; keep old bookmarks navigable.
export function loader() {
  return redirect('/u/dashboard');
}
