/**
 * Extract error message from an API error response.
 * Axios wraps the response in error.response.data.
 * Our backend always returns { "error": "..." }.
 */
export function extractApiError(error: any, fallback: string): string {
  return error?.response?.data?.error || error?.message || fallback
}
