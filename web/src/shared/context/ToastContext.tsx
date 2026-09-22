import {
  createContext,
  useContext,
  useState,
  useCallback,
  ReactNode,
} from "react";

export type ToastType = "success" | "error" | "info" | "warning";

export interface ToastMessage {
  id: string;
  message: string;
  type: ToastType;
  duration?: number;
}

interface ToastContextValue {
  /** Currently visible toasts */
  toasts: ToastMessage[];
  /** Show a new toast notification */
  showToast: (toast: Omit<ToastMessage, "id">) => void;
  /** Remove a toast by ID */
  removeToast: (id: string) => void;
  /** Remove all toasts */
  clearToasts: () => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

interface ToastProviderProps {
  children: ReactNode;
  /** Maximum number of toasts to show at once */
  maxToasts?: number;
  /** Default duration in milliseconds */
  defaultDuration?: number;
}

/**
 * Generate unique ID for toasts
 */
function generateId(): string {
  return `toast-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

/**
 * ToastProvider - Context provider for app-wide toast notifications
 *
 * Manages a queue of toast notifications with automatic dismissal.
 *
 * @example
 * ```tsx
 * // In App.tsx or providers
 * <ToastProvider maxToasts={3}>
 *   <App />
 * </ToastProvider>
 *
 * // In any component
 * const { showToast } = useToast();
 * showToast({ message: "Operation successful", type: "success" });
 * ```
 */
export function ToastProvider({
  children,
  maxToasts = 3,
  defaultDuration = 5000,
}: ToastProviderProps) {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
  }, []);

  const showToast = useCallback(
    (toast: Omit<ToastMessage, "id">) => {
      const id = generateId();
      const newToast: ToastMessage = {
        ...toast,
        id,
        duration: toast.duration ?? defaultDuration,
      };

      setToasts((prev) => {
        // Remove oldest toasts if we exceed maxToasts
        const updated = [...prev, newToast];
        if (updated.length > maxToasts) {
          return updated.slice(-maxToasts);
        }
        return updated;
      });

      // Auto-remove after duration
      if (newToast.duration && newToast.duration > 0) {
        setTimeout(() => {
          removeToast(id);
        }, newToast.duration);
      }
    },
    [maxToasts, defaultDuration, removeToast],
  );

  const clearToasts = useCallback(() => {
    setToasts([]);
  }, []);

  return (
    <ToastContext.Provider
      value={{ toasts, showToast, removeToast, clearToasts }}
    >
      {children}
    </ToastContext.Provider>
  );
}

/**
 * useToast - Hook to access toast functionality
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { showToast } = useToast();
 *
 *   const handleError = () => {
 *     showToast({
 *       message: "Failed to save changes",
 *       type: "error",
 *     });
 *   };
 *
 *   return <button onClick={handleError}>Save</button>;
 * }
 * ```
 */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
}
