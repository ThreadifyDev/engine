// Keep these rules aligned with threadify-go/shared/management/validation/auth.go.
export const PASSWORD_REQUIREMENTS =
  'Use at least 12 characters, including uppercase, lowercase, a number, and a special character. No spaces.';

export function validateSignupPassword(password: string): string | null {
  if (!password) return 'Password is required';
  if (password.trim() !== password) return 'Password must not have leading or trailing spaces';
  if (/\p{White_Space}/u.test(password)) return 'Password must not contain whitespace';
  const length = new TextEncoder().encode(password).length;
  if (length < 12) return 'Password must be at least 12 characters';
  if (length > 128) return 'Password exceeds maximum length';
  if (/\p{Cc}/u.test(password)) return 'Password contains invalid characters';
  if (!/\p{Lu}/u.test(password) || !/\p{Ll}/u.test(password) ||
      !/\p{Nd}/u.test(password) || !/[^\p{Lu}\p{Ll}\p{Nd}]/u.test(password)) {
    return 'Password must include upper, lower, number, and special character';
  }
  return null;
}
