import { describe, it, expect } from "vitest";
import { render } from "@/test-utils";
import { LoadingSkeleton } from "./LoadingSkeleton";

describe("LoadingSkeleton", () => {
  describe("Table variant", () => {
    it("should render table skeleton with default rows", () => {
      const { container } = render(<LoadingSkeleton variant="table" />);

      // Default rows is 5, count skeletons per row (5 columns)
      // Header has 5 skeletons, each data row has 5 skeletons
      // Total: 5 header + 25 data = 30 skeletons
      const skeletons = container.querySelectorAll(".MuiSkeleton-root");
      expect(skeletons.length).toBe(30);
    });

    it("should render table skeleton with custom rows", () => {
      const { container } = render(
        <LoadingSkeleton variant="table" rows={10} />,
      );

      // 5 header skeletons + 10 rows * 5 columns = 55 skeletons
      const skeletons = container.querySelectorAll(".MuiSkeleton-root");
      expect(skeletons.length).toBe(55);
    });

    it("should render 5 columns per row", () => {
      const { container } = render(
        <LoadingSkeleton variant="table" rows={1} />,
      );

      // Count skeletons in header row (5 columns)
      const skeletons = container.querySelectorAll(".MuiSkeleton-root");
      // 5 header + 5 data row = 10
      expect(skeletons.length).toBe(10);
    });
  });

  describe("Card variant", () => {
    it("should render card skeleton with default count", () => {
      const { container } = render(<LoadingSkeleton variant="card" />);

      // Default count is 3
      const cards = container.querySelectorAll(
        ".MuiBox-root > .MuiBox-root > .MuiBox-root",
      );
      expect(cards.length).toBeGreaterThanOrEqual(3);
    });

    it("should render card skeleton with custom count", () => {
      const { container } = render(
        <LoadingSkeleton variant="card" count={6} />,
      );

      // Check for circular skeletons (one per card header)
      const circularSkeletons = container.querySelectorAll(
        ".MuiSkeleton-circular",
      );
      expect(circularSkeletons.length).toBe(6);
    });

    it("should render card with circular avatar skeleton", () => {
      const { container } = render(
        <LoadingSkeleton variant="card" count={1} />,
      );

      const circularSkeleton = container.querySelector(".MuiSkeleton-circular");
      expect(circularSkeleton).toBeInTheDocument();
    });
  });

  describe("List variant", () => {
    it("should render list skeleton with default items", () => {
      const { container } = render(<LoadingSkeleton variant="list" />);

      // Default items is 5
      const circularSkeletons = container.querySelectorAll(
        ".MuiSkeleton-circular",
      );
      expect(circularSkeletons.length).toBe(5);
    });

    it("should render list skeleton with custom items", () => {
      const { container } = render(
        <LoadingSkeleton variant="list" items={8} />,
      );

      // Each list item has a circular skeleton
      const circularSkeletons = container.querySelectorAll(
        ".MuiSkeleton-circular",
      );
      expect(circularSkeletons.length).toBe(8);
    });

    it("should render list item with circular and text skeletons", () => {
      const { container } = render(
        <LoadingSkeleton variant="list" items={1} />,
      );

      const circularSkeleton = container.querySelector(".MuiSkeleton-circular");
      const textSkeletons = container.querySelectorAll(".MuiSkeleton-text");
      const roundedSkeleton = container.querySelector(".MuiSkeleton-rounded");

      expect(circularSkeleton).toBeInTheDocument();
      expect(textSkeletons.length).toBeGreaterThanOrEqual(2);
      expect(roundedSkeleton).toBeInTheDocument();
    });
  });

  describe("Text variant", () => {
    it("should render text skeleton with default lines", () => {
      const { container } = render(<LoadingSkeleton variant="text" />);

      // Default lines is 4
      const skeletons = container.querySelectorAll(".MuiSkeleton-text");
      expect(skeletons.length).toBe(4);
    });

    it("should render text skeleton with custom lines", () => {
      const { container } = render(
        <LoadingSkeleton variant="text" lines={6} />,
      );

      const skeletons = container.querySelectorAll(".MuiSkeleton-text");
      expect(skeletons.length).toBe(6);
    });

    it("should render last line with shorter width", () => {
      const { container } = render(
        <LoadingSkeleton variant="text" lines={3} />,
      );

      const skeletons = container.querySelectorAll(".MuiSkeleton-text");
      const lastSkeleton = skeletons[skeletons.length - 1] as HTMLElement;

      // Last line should have 60% width
      expect(lastSkeleton).toHaveStyle({ width: "60%" });
    });
  });

  describe("Common props", () => {
    it("should apply custom sx props", () => {
      const { container } = render(
        <LoadingSkeleton variant="table" sx={{ margin: 2 }} />,
      );

      const wrapper = container.firstChild as HTMLElement;
      expect(wrapper).toHaveStyle({ margin: "16px" });
    });

    it("should render within a Box wrapper", () => {
      const { container } = render(<LoadingSkeleton variant="text" />);

      const wrapper = container.firstChild as HTMLElement;
      expect(wrapper).toHaveClass("MuiBox-root");
    });

    it("should render nothing for unknown variant", () => {
      // @ts-expect-error Testing invalid variant
      const { container } = render(<LoadingSkeleton variant="invalid" />);

      // Should still render the wrapper Box
      expect(container.firstChild).toHaveClass("MuiBox-root");
    });
  });
});
