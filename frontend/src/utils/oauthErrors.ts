// Explains the OAuth failure that the server's callback reports in the oauth_error query parameter.

import {
  ErrorCodeConflict,
  ErrorCodeEmailInUse,
  ErrorCodeEmailNotVerified,
  ErrorCodeExpired,
  ErrorCodeInvalidProvider,
  ErrorCodeOAuthError,
} from "@sdk/types.gen";

/** Query parameter that carries the ErrorCode of a failed OAuth sign-in or link. */
export const OAUTH_ERROR_PARAM = "oauth_error";

// Codes with their own explanation under auth.oauthErrors; others use its default.
const explained: readonly string[] = [
  ErrorCodeConflict,
  ErrorCodeEmailInUse,
  ErrorCodeEmailNotVerified,
  ErrorCodeExpired,
  ErrorCodeInvalidProvider,
  ErrorCodeOAuthError,
];

/** Returns the localized explanation of an OAuth failure code. */
export function oauthErrorMessage(t: (key: string) => string, code: string): string {
  return t(`auth.oauthErrors.${explained.includes(code) ? code : "default"}`);
}
