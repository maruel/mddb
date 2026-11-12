// Test support that answers the startup session probe from an /auth/me response.

/** Returns the /auth/session response for the account an /auth/me response describes. */
export async function sessionFromMe(me: Response): Promise<Response> {
  if (me.status === 401) return Response.json({ user: null });
  if (!me.ok) return me;
  return Response.json({ user: await me.json() });
}
