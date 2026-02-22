/**
 * Normalize int64 fields from protojson responses.
 *
 * protojson serializes int64 as JSON strings (e.g. "123" instead of 123)
 * because JavaScript's Number cannot represent the full int64 range.
 * Our IDs are well within safe integer range, so we convert them back
 * to numbers at the API boundary to avoid === mismatches everywhere.
 */

type AnyRecord = Record<string, any>

/** Convert specified fields from string to number on a single object. */
export function normalizeInt64<T extends AnyRecord>(obj: T, fields: (keyof T)[]): T {
  for (const f of fields) {
    if (obj[f] != null) {
      (obj as any)[f] = Number(obj[f])
    }
  }
  return obj
}

/** Convert specified fields from string to number on an array of objects. */
export function normalizeInt64Array<T extends AnyRecord>(arr: T[], fields: (keyof T)[]): T[] {
  for (const obj of arr) {
    normalizeInt64(obj, fields)
  }
  return arr
}
