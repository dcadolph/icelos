import SwiftUI

/// BalancedGrid lays out equal-width cells in rows of even length. It finds the fewest rows the
/// width allows, then spreads the cells across them, so ten cells that fit six to a row become
/// two rows of five rather than six and four.
struct BalancedGrid: Layout {
    /// minCellWidth is the narrowest a cell may be.
    var minCellWidth: CGFloat
    /// spacing is the gap between cells in both directions.
    var spacing: CGFloat

    /// columns returns the column count for count cells across width.
    func columns(count: Int, width: CGFloat) -> Int {
        guard count > 0 else { return 1 }
        let fit = max(1, Int((width + spacing) / (minCellWidth + spacing)))
        let rows = (count + min(fit, count) - 1) / min(fit, count)
        return (count + rows - 1) / rows
    }

    /// metrics returns the cell width and the height of each row.
    private func metrics(width: CGFloat, subviews: Subviews) -> (CGFloat, [CGFloat]) {
        let cols = columns(count: subviews.count, width: width)
        let cell = max(0, (width - spacing * CGFloat(cols - 1)) / CGFloat(cols))
        var heights: [CGFloat] = []
        for start in stride(from: 0, to: subviews.count, by: cols) {
            let row = subviews[start..<min(start + cols, subviews.count)]
            heights.append(row.map { $0.sizeThatFits(ProposedViewSize(width: cell, height: nil)).height }.max() ?? 0)
        }
        return (cell, heights)
    }

    /// sizeThatFits fills the proposed width and stacks the rows.
    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? (minCellWidth + spacing) * CGFloat(subviews.count) - spacing
        let (_, heights) = metrics(width: width, subviews: subviews)
        return CGSize(width: width, height: heights.reduce(0, +) + spacing * CGFloat(max(0, heights.count - 1)))
    }

    /// placeSubviews places each cell in its row at the cell width and the row's height.
    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let (cell, heights) = metrics(width: bounds.width, subviews: subviews)
        let cols = columns(count: subviews.count, width: bounds.width)
        var y = bounds.minY
        for (r, height) in heights.enumerated() {
            for c in 0..<cols {
                let i = r * cols + c
                guard i < subviews.count else { break }
                let x = bounds.minX + CGFloat(c) * (cell + spacing)
                subviews[i].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(width: cell, height: height))
            }
            y += height + spacing
        }
    }
}
