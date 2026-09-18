import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@/test-utils";
import { ToastProvider, useToast } from "./ToastContext";
import { ToastContainer } from "@/shared/components";

// Test component that uses the toast hook
function TestComponent() {
  const { showToast, clearToasts, toasts } = useToast();

  return (
    <div>
      <span data-testid="toast-count">{toasts.length}</span>
      <button
        onClick={() => showToast({ message: "Success!", type: "success" })}
      >
        Show Success
      </button>
      <button onClick={() => showToast({ message: "Error!", type: "error" })}>
        Show Error
      </button>
      <button
        onClick={() => showToast({ message: "Warning!", type: "warning" })}
      >
        Show Warning
      </button>
      <button onClick={() => showToast({ message: "Info!", type: "info" })}>
        Show Info
      </button>
      <button
        onClick={() =>
          showToast({ message: "Long toast", type: "info", duration: 10000 })
        }
      >
        Show Long Toast
      </button>
      <button
        onClick={() =>
          showToast({ message: "No auto-dismiss", type: "info", duration: 0 })
        }
      >
        Show Persistent Toast
      </button>
      <button onClick={clearToasts}>Clear All</button>
    </div>
  );
}

describe("ToastProvider", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("should render children", () => {
    render(
      <ToastProvider>
        <div>Test content</div>
      </ToastProvider>,
    );

    expect(screen.getByText("Test content")).toBeInTheDocument();
  });

  it("should throw error when useToast is used outside provider", () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    expect(() => {
      render(<TestComponent />);
    }).toThrow("useToast must be used within a ToastProvider");

    consoleError.mockRestore();
  });

  it("should add toast when showToast is called", () => {
    render(
      <ToastProvider>
        <TestComponent />
      </ToastProvider>,
    );

    expect(screen.getByTestId("toast-count")).toHaveTextContent("0");

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));

    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");
  });

  it("should auto-remove toast after default duration", async () => {
    render(
      <ToastProvider defaultDuration={3000}>
        <TestComponent />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));
    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");

    act(() => {
      vi.advanceTimersByTime(3000);
    });

    expect(screen.getByTestId("toast-count")).toHaveTextContent("0");
  });

  it("should respect custom duration on individual toast", async () => {
    render(
      <ToastProvider defaultDuration={1000}>
        <TestComponent />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show long toast/i }));
    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");

    // After default duration, toast should still be there
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");

    // After custom duration, toast should be removed
    act(() => {
      vi.advanceTimersByTime(9000);
    });
    expect(screen.getByTestId("toast-count")).toHaveTextContent("0");
  });

  it("should not auto-remove toast with duration 0", async () => {
    render(
      <ToastProvider defaultDuration={1000}>
        <TestComponent />
      </ToastProvider>,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /show persistent toast/i }),
    );
    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");

    // After long time, toast should still be there
    act(() => {
      vi.advanceTimersByTime(60000);
    });
    expect(screen.getByTestId("toast-count")).toHaveTextContent("1");
  });

  it("should limit toasts to maxToasts", () => {
    render(
      <ToastProvider maxToasts={2}>
        <TestComponent />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));
    fireEvent.click(screen.getByRole("button", { name: /show error/i }));
    fireEvent.click(screen.getByRole("button", { name: /show warning/i }));

    // Should only have 2 toasts (oldest removed)
    expect(screen.getByTestId("toast-count")).toHaveTextContent("2");
  });

  it("should clear all toasts when clearToasts is called", () => {
    render(
      <ToastProvider>
        <TestComponent />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));
    fireEvent.click(screen.getByRole("button", { name: /show error/i }));
    expect(screen.getByTestId("toast-count")).toHaveTextContent("2");

    fireEvent.click(screen.getByRole("button", { name: /clear all/i }));
    expect(screen.getByTestId("toast-count")).toHaveTextContent("0");
  });
});

describe("ToastContainer", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("should not render when there are no toasts", () => {
    const { container } = render(
      <ToastProvider>
        <ToastContainer />
      </ToastProvider>,
    );

    // ToastContainer returns null when no toasts
    expect(
      container.querySelector(".MuiSnackbar-root"),
    ).not.toBeInTheDocument();
  });

  it("should render toast messages", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));

    expect(screen.getByText("Success!")).toBeInTheDocument();
  });

  it("should render success toast with correct severity", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));

    const alert = screen.getByRole("alert");
    expect(alert).toHaveClass("MuiAlert-filledSuccess");
  });

  it("should render error toast with correct severity", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show error/i }));

    const alert = screen.getByRole("alert");
    expect(alert).toHaveClass("MuiAlert-filledError");
  });

  it("should render warning toast with correct severity", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show warning/i }));

    const alert = screen.getByRole("alert");
    expect(alert).toHaveClass("MuiAlert-filledWarning");
  });

  it("should render info toast with correct severity", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show info/i }));

    const alert = screen.getByRole("alert");
    expect(alert).toHaveClass("MuiAlert-filledInfo");
  });

  it("should remove toast when close button is clicked", () => {
    render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));
    expect(screen.getByText("Success!")).toBeInTheDocument();

    // Find and click close button
    const closeButton = screen.getByRole("button", { name: /close/i });
    fireEvent.click(closeButton);

    // After clicking close, toast should be removed
    expect(screen.queryByText("Success!")).not.toBeInTheDocument();
  });

  it("should render multiple toasts stacked", () => {
    render(
      <ToastProvider maxToasts={3}>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));
    fireEvent.click(screen.getByRole("button", { name: /show error/i }));

    expect(screen.getByText("Success!")).toBeInTheDocument();
    expect(screen.getByText("Error!")).toBeInTheDocument();
  });

  it("should render toasts in a container", () => {
    const { container } = render(
      <ToastProvider>
        <TestComponent />
        <ToastContainer />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /show success/i }));

    // ToastContainer renders a Box with toasts
    const alerts = container.querySelectorAll(".MuiAlert-root");
    expect(alerts.length).toBe(1);
  });
});
