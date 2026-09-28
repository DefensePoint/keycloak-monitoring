import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@/test-utils";
import { ErrorBoundary } from "./ErrorBoundary";

// Component that throws an error
function ThrowError({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("Test error");
  }
  return <div>Normal content</div>;
}

// Suppress console.error during tests
const originalConsoleError = console.error;

describe("ErrorBoundary", () => {
  beforeEach(() => {
    console.error = vi.fn();
  });

  afterEach(() => {
    console.error = originalConsoleError;
  });

  it("should render children when no error occurs", () => {
    render(
      <ErrorBoundary>
        <div>Test content</div>
      </ErrorBoundary>,
    );

    expect(screen.getByText("Test content")).toBeInTheDocument();
  });

  it("should render default fallback when error occurs", () => {
    render(
      <ErrorBoundary>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
  });

  it("should render custom ReactNode fallback", () => {
    render(
      <ErrorBoundary fallback={<div>Custom error message</div>}>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Custom error message")).toBeInTheDocument();
  });

  it("should render custom fallback function with props", () => {
    render(
      <ErrorBoundary
        boundaryId="TestBoundary"
        fallback={({ error, resetError, boundaryId }) => (
          <div>
            <p>Error: {error.message}</p>
            <p>Boundary: {boundaryId}</p>
            <button onClick={resetError}>Reset</button>
          </div>
        )}
      >
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Error: Test error")).toBeInTheDocument();
    expect(screen.getByText("Boundary: TestBoundary")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /reset/i })).toBeInTheDocument();
  });

  it("should call onError callback when error is caught", () => {
    const onError = vi.fn();

    render(
      <ErrorBoundary onError={onError}>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(onError).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({
        componentStack: expect.any(String),
      }),
    );
  });

  it("should reset error state when Try Again is clicked", () => {
    const { rerender } = render(
      <ErrorBoundary>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();

    // Rerender with non-throwing component before clicking reset
    rerender(
      <ErrorBoundary>
        <ThrowError shouldThrow={false} />
      </ErrorBoundary>,
    );

    fireEvent.click(screen.getByRole("button", { name: /try again/i }));

    expect(screen.getByText("Normal content")).toBeInTheDocument();
  });

  it("should call onReset callback when resetting", () => {
    const onReset = vi.fn();

    const { rerender } = render(
      <ErrorBoundary onReset={onReset}>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    rerender(
      <ErrorBoundary onReset={onReset}>
        <ThrowError shouldThrow={false} />
      </ErrorBoundary>,
    );

    fireEvent.click(screen.getByRole("button", { name: /try again/i }));

    expect(onReset).toHaveBeenCalled();
  });

  it("should reset when resetKeys change", () => {
    const { rerender } = render(
      <ErrorBoundary resetKeys={["key1"]}>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();

    // Change resetKeys and provide non-throwing child
    rerender(
      <ErrorBoundary resetKeys={["key2"]}>
        <ThrowError shouldThrow={false} />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Normal content")).toBeInTheDocument();
  });

  it("should not reset when resetKeys stay the same", () => {
    const { rerender } = render(
      <ErrorBoundary resetKeys={["key1"]}>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();

    // Rerender with same key but non-throwing child
    rerender(
      <ErrorBoundary resetKeys={["key1"]}>
        <ThrowError shouldThrow={false} />
      </ErrorBoundary>,
    );

    // Still shows error since keys didn't change
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("should include boundaryId in console error message in dev mode", () => {
    render(
      <ErrorBoundary boundaryId="MyComponent">
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    expect(console.error).toHaveBeenCalledWith(
      "[ErrorBoundary:MyComponent]",
      expect.any(Error),
    );
  });

  it("should render default fallback with centered layout", () => {
    const { container } = render(
      <ErrorBoundary>
        <ThrowError shouldThrow />
      </ErrorBoundary>,
    );

    const wrapper = container.querySelector(".MuiBox-root") as HTMLElement;
    expect(wrapper).toHaveStyle({
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
    });
  });
});
