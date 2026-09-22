/**
 * Number and byte formatting utilities
 */

/**
 * Formats bytes into human-readable format (B, KB, MB, GB)
 * @param bytes - Number of bytes to format
 * @returns Formatted string with appropriate unit
 * @example
 * formatBytes(1024) // "1.00 KB"
 * formatBytes(1048576) // "1.00 MB"
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  if (!Number.isFinite(bytes)) return "0 B";

  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
}

/**
 * Formats a number with thousand separators
 * @param num - Number to format
 * @returns Formatted string with thousand separators
 * @example
 * formatNumber(1234567) // "1,234,567"
 */
export function formatNumber(num: number): string {
  return new Intl.NumberFormat("en-US").format(Math.floor(num));
}
