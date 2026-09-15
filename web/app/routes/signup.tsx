import { redirect } from '@remix-run/node';

// Registry owns sign-up, email verification, and account recovery.
export function loader() { return redirect('/login'); }
export default function ManagedIdentityRedirect() { return null; }
