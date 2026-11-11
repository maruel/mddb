// Preserves field-level record edit intent across acknowledged local changes.

function equalValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    return (
      Array.isArray(left) &&
      Array.isArray(right) &&
      left.length === right.length &&
      left.every((v, i) => equalValue(v, right[i]))
    );
  }
  if (!left || !right || typeof left !== "object" || typeof right !== "object") return false;
  const a = left as Record<string, unknown>;
  const b = right as Record<string, unknown>;
  const keys = Object.keys(a);
  return (
    keys.length === Object.keys(b).length && keys.every((key) => Object.hasOwn(b, key) && equalValue(a[key], b[key]))
  );
}

// Apply only fields changed by the user, including explicit removals and empty values.
export function rebaseRecordEdit(
  observed: Record<string, unknown>,
  submitted: Record<string, unknown>,
  latest: Record<string, unknown>,
) {
  const result = { ...latest };
  for (const key of new Set([...Object.keys(observed), ...Object.keys(submitted)])) {
    if (!Object.hasOwn(submitted, key)) {
      delete result[key];
    } else if (!Object.hasOwn(observed, key) || !equalValue(observed[key], submitted[key])) {
      Object.defineProperty(result, key, {
        value: submitted[key],
        enumerable: true,
        writable: true,
        configurable: true,
      });
    }
  }
  return result;
}
