import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { BrowserRouter } from "react-router-dom";
import { UserDetailsHeader } from "./UserDetailsHeader";

const mockNavigate = vi.fn();

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const renderWithRouter = (component: React.ReactElement) => {
  return render(<BrowserRouter>{component}</BrowserRouter>);
};

describe("UserDetailsHeader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render header title", () => {
    renderWithRouter(<UserDetailsHeader />);

    expect(screen.getByText("User Details")).toBeInTheDocument();
  });

  it("should render header description", () => {
    renderWithRouter(<UserDetailsHeader />);

    expect(
      screen.getByText("View user information and related events"),
    ).toBeInTheDocument();
  });

  it("should render back button", () => {
    renderWithRouter(<UserDetailsHeader />);

    const backButton = screen.getByRole("button");
    expect(backButton).toBeInTheDocument();
  });

  it("should call navigate(-1) when back button is clicked", async () => {
    const user = userEvent.setup();
    renderWithRouter(<UserDetailsHeader />);

    const backButton = screen.getByRole("button");
    await user.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith(-1);
  });

  it("should render title with correct styling", () => {
    renderWithRouter(<UserDetailsHeader />);

    const title = screen.getByText("User Details");
    expect(title).toHaveClass("MuiTypography-h4");
  });

  it("should render description with correct styling", () => {
    renderWithRouter(<UserDetailsHeader />);

    const description = screen.getByText(
      "View user information and related events",
    );
    expect(description).toHaveClass("MuiTypography-body2");
  });
});
