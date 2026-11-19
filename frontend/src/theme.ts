// Resolves saved account appearance preferences while keeping the browser preference as the default.

import type { UserResponse } from "@sdk/types.gen";

export function accountTheme(user: UserResponse | null): "dark" | "light" | "system" {
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Persisted accounts can omit settings.
  const theme = user?.settings?.theme;
  return theme === "dark" || theme === "light" ? theme : "system";
}
