import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { PageSizeSelector } from "./PageSizeSelector";

describe("PageSizeSelector", () => {
  it("should render with default value and label", () => {
    render(<PageSizeSelector value={25} onChange={vi.fn()} />);

    expect(
      screen.getByRole("button", { name: /25 items/i }),
    ).toBeInTheDocument();
  });

  it("should render with custom label", () => {
    render(<PageSizeSelector value={50} onChange={vi.fn()} label="rows" />);

    expect(
      screen.getByRole("button", { name: /50 rows/i }),
    ).toBeInTheDocument();
  });

  it("should open menu when clicked", async () => {
    const user = userEvent.setup();
    render(<PageSizeSelector value={25} onChange={vi.fn()} />);

    const button = screen.getByRole("button", { name: /25 items/i });
    await user.click(button);

    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("should display default options in menu", async () => {
    const user = userEvent.setup();
    render(<PageSizeSelector value={25} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button"));

    expect(
      screen.getByRole("menuitem", { name: /25 items/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /50 items/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /100 items/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /200 items/i }),
    ).toBeInTheDocument();
  });

  it("should display custom options in menu", async () => {
    const user = userEvent.setup();
    render(
      <PageSizeSelector
        value={10}
        onChange={vi.fn()}
        options={[10, 20, 30]}
        label="entries"
      />,
    );

    await user.click(screen.getByRole("button"));

    expect(
      screen.getByRole("menuitem", { name: /10 entries/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /20 entries/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /30 entries/i }),
    ).toBeInTheDocument();
  });

  it("should call onChange when option is selected", async () => {
    const user = userEvent.setup();
    const mockOnChange = vi.fn();
    render(<PageSizeSelector value={25} onChange={mockOnChange} />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByRole("menuitem", { name: /100 items/i }));

    expect(mockOnChange).toHaveBeenCalledWith(100);
  });

  it("should close menu after selection", async () => {
    const user = userEvent.setup();
    render(<PageSizeSelector value={25} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button"));
    expect(screen.getByRole("menu")).toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: /50 items/i }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("should highlight currently selected option", async () => {
    const user = userEvent.setup();
    render(<PageSizeSelector value={50} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button"));

    const selectedItem = screen.getByRole("menuitem", { name: /50 items/i });
    expect(selectedItem).toHaveClass("Mui-selected");
  });

  it("should apply custom className", () => {
    const { container } = render(
      <PageSizeSelector
        value={25}
        onChange={vi.fn()}
        className="custom-class"
      />,
    );

    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });
});
