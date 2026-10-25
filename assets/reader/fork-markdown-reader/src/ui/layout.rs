//! Centered-popup layout primitives — one source of truth.
//!
//! Consolidates three previously-duplicated rect helpers:
//!   - `centered_rect`  (fixed cells)  — was copied across 5 popup files
//!   - `centered_pct`   (percentage)   — was copied across 2 modal files
//!   - `percent_rect`   (percentage, different floor values) — search_modal
//!
//! `percent_rect` diverges from `centered_pct` in its minimum dimensions
//! (height floor 4 vs 5, width floor 20 vs 10) so the two are kept separate.

use ratatui::layout::{Constraint, Flex, Layout, Rect};

/// Return a [`Rect`] of exactly `width × height` cells, centred within `area`.
///
/// When the requested size exceeds `area`, ratatui's `Flex::Center` clamps the
/// result to the available space rather than overflowing.
pub(crate) fn centered_rect(width: u16, height: u16, area: Rect) -> Rect {
    let vertical = Layout::vertical([Constraint::Length(height)])
        .flex(Flex::Center)
        .split(area);
    Layout::horizontal([Constraint::Length(width)])
        .flex(Flex::Center)
        .split(vertical[0])[0]
}

/// Return a [`Rect`] sized to `w_pct`% × `h_pct`% of `area`, centred within it.
///
/// Minimum dimensions: 10 cols wide, 5 rows tall.  Used by the mermaid and
/// table modals, which both open at 90 × 90 %.
pub(crate) fn centered_pct(w_pct: u16, h_pct: u16, area: Rect) -> Rect {
    let w = (area.width * w_pct / 100).max(10);
    let h = (area.height * h_pct / 100).max(5);
    let vertical = Layout::vertical([Constraint::Length(h)])
        .flex(Flex::Center)
        .split(area);
    Layout::horizontal([Constraint::Length(w)])
        .flex(Flex::Center)
        .split(vertical[0])[0]
}

/// Return a [`Rect`] sized to `width_pct`% × `height_pct`% of `area`, centred within it.
///
/// Minimum dimensions: 20 cols wide, 4 rows tall.  Used by the search modal,
/// which has larger floor values than [`centered_pct`] to keep the query bar
/// and result list legible at small terminal sizes.
pub(crate) fn percent_rect(width_pct: u16, height_pct: u16, area: Rect) -> Rect {
    let width = (area.width * width_pct / 100).max(20);
    let height = (area.height * height_pct / 100).max(4);
    let vertical = Layout::vertical([Constraint::Length(height)])
        .flex(Flex::Center)
        .split(area);
    Layout::horizontal([Constraint::Length(width)])
        .flex(Flex::Center)
        .split(vertical[0])[0]
}
