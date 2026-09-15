import { redirect } from '@remix-run/node';

export const loader = () => redirect('/login');
export default function RegistrySignInRedirect() { return null; }
