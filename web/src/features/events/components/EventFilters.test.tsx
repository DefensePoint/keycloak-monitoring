import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EventFilters } from "./EventFilters";

describe("EventFilters", () => {
  const renderFilters = (
    props: Partial<React.ComponentProps<typeof EventFilters>> = {},
  ) => {
    const onChange = vi.fn();
    render(
      <EventFilters
        value={{}}
        onChange={onChange}
        showMinRisk
        typeOptions={["LOGIN", "LOGIN_ERROR"]}
        {...props}
      />,
    );
    return onChange;
  };

  it("reports a chosen severity", async () => {
    const user = userEvent.setup();
    const onChange = renderFilters({ value: { type: "LOGIN" } });

    await user.click(screen.getByRole("combobox", { name: /severity/i }));
    await user.click(screen.getByRole("option", { name: "Critical" }));

    expect(onChange).toHaveBeenCalledWith({
      type: "LOGIN",
      severity: "critical",
    });
  });

  it("clears severity when All is chosen", async () => {
    const user = userEvent.setup();
    const onChange = renderFilters({ value: { severity: "error" } });

    await user.click(screen.getByRole("combobox", { name: /severity/i }));
    await user.click(screen.getByRole("option", { name: "All" }));

    expect(onChange).toHaveBeenCalledWith({ severity: undefined });
  });

  it("reports a minimum risk level as a number", async () => {
    const user = userEvent.setup();
    const onChange = renderFilters();

    await user.click(screen.getByRole("combobox", { name: /min risk/i }));
    await user.click(screen.getByRole("option", { name: "3+" }));

    expect(onChange).toHaveBeenCalledWith({ minRisk: 3 });
  });

  it("keeps the select labels shrunk so they never overlap All/Any", () => {
    renderFilters();

    // Found on the Events page in a real browser: an unshrunk label sat on
    // top of the "All" placeholder and read "Allverity".
    for (const label of ["Severity", "Min risk"]) {
      const el = screen
        .getAllByText(label)
        .find((node) => node.tagName === "LABEL");
      expect(el).toHaveAttribute("data-shrink", "true");
    }
  });

  it("hides the minimum risk control for realms without AMFA", () => {
    renderFilters({ showMinRisk: false });

    expect(
      screen.queryByRole("combobox", { name: /min risk/i }),
    ).not.toBeInTheDocument();
  });

  it("commits a typed event type on Enter, trimmed, not on each keystroke", async () => {
    const user = userEvent.setup();
    const onChange = renderFilters();

    const input = screen.getByRole("combobox", { name: /event type/i });
    await user.type(input, "  UPDATE_PASSWORD ");
    expect(onChange).not.toHaveBeenCalled();

    await user.keyboard("{Enter}");
    expect(onChange).toHaveBeenLastCalledWith({ type: "UPDATE_PASSWORD" });
  });

  it("reports a suggested event type when picked", async () => {
    const user = userEvent.setup();
    const onChange = renderFilters();

    await user.click(screen.getByRole("combobox", { name: /event type/i }));
    await user.click(screen.getByRole("option", { name: "LOGIN_ERROR" }));

    expect(onChange).toHaveBeenLastCalledWith({ type: "LOGIN_ERROR" });
  });
});
