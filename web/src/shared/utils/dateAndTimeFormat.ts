/**
 * Date and time formatting utilities
 */

/**
 * Formats uptime in milliseconds to human-readable format
 * @param millis - Uptime in milliseconds
 * @returns Formatted string (e.g., "2d 5h", "12h 30m", "45m")
 * @example
 * formatUptime(172800000) // "2d 0h"
 * formatUptime(45000000) // "12h 30m"
 */
export function formatUptime(millis: number): string {
  const days = Math.floor(millis / 1000 / 60 / 60 / 24);
  const hours = Math.floor((millis / 1000 / 60 / 60) % 24);
  const minutes = Math.floor((millis / 1000 / 60) % 60);

  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

/**
 * Formats a date/time to locale string
 * @param date - Date to format
 * @returns Formatted date string (e.g., "Jan 15, 02:30 PM")
 */
export function formatDateTime(date: Date): string {
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Formats a date for datetime-local input
 * @param date - Date to format
 * @returns Formatted string in YYYY-MM-DDTHH:mm format
 */
export function formatDateTimeInput(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}
