use crate::app::App;
use crate::ui::layout::centered_rect;
use ratatui::{
    Frame,
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph},
};

/// One entry in the outline picker: the heading text label, its indented display
/// prefix (built from level), and the absolute display line to jump to.
#[derive(Debug, Clone)]
pub struct OutlineEntry {
    /// Human-readable label: the heading anchor slug (or rendered text fallback).
    pub label: String,
    /// ATX heading level (1–6). Controls indentation in the popup.
    pub level: u8,
    /// Absolute 0-indexed display line within the document.
    pub line: u32,
}

impl OutlineEntry {
    /// Build the indented display string for this entry.
    ///
    /// H1 → `# Title`, H2 → `  ## Title`, H3 → `    ### Title`, etc.
    /// Each level adds 2 spaces of leading indent beyond the previous.
    fn display_prefix(&self) -> String {
        // (level - 1) * 2 spaces of indent so H1 has none, H2 has 2, etc.
        let indent = " ".repeat((self.level.saturating_sub(1) as usize) * 2);
        let hashes = "#".repeat(self.level as usize);
        format!("{indent}{hashes} ")
    }
}

/// State for the outline-picker overlay (opened with `o` in the viewer).
#[derive(Debug, Default)]
pub struct OutlinePickerState {
    /// All heading entries collected from the rendered document, in document order.
    pub entries: Vec<OutlineEntry>,
    /// Index of the currently highlighted row (0-based).
    pub cursor: usize,
}

impl OutlinePickerState {
    /// Collect all heading anchors from the active tab's rendered blocks.
    ///
    /// Entries are in document order (as they appear in `heading_anchors`, which
    /// is populated in source order by the renderer). Returns `None` when there is
    /// no active tab.
    pub fn build(app: &App) -> Option<Self> {
        let tab = app.tabs.active_tab()?;
        let entries: Vec<OutlineEntry> = tab
            .view
            .heading_anchors
            .iter()
            .map(|ha| OutlineEntry {
                label: ha.anchor.replace('-', " "),
                level: ha.level,
                line: ha.line,
            })
            .collect();
        Some(Self { entries, cursor: 0 })
    }

    /// Move the selection cursor up by one, clamped to the first entry.
    ///
    /// Unlike the link picker, the outline picker does NOT wrap — reaching the
    /// first or last item clamps there. This mirrors Vim's `:tselect` style so
    /// users always see their position relative to the document.
    pub fn move_up(&mut self) {
        self.cursor = self.cursor.saturating_sub(1);
    }

    /// Move the selection cursor down by one, clamped to the last entry.
    pub fn move_down(&mut self) {
        if !self.entries.is_empty() {
            self.cursor = (self.cursor + 1).min(self.entries.len() - 1);
        }
    }
}

/// Render the outline-picker overlay centered on the frame.
///
/// No-ops when `app.outline_picker` is `None` or contains zero entries.
/// When the picker is open but the document has no headings, a placeholder
/// message is shown (this state is only reached if the caller builds an empty
/// picker, which `App::open_outline_picker` avoids — but we guard here too
/// for correctness).
pub fn draw(f: &mut Frame, app: &mut App) {
    let Some(picker) = &app.outline_picker else {
        return;
    };

    let p = &app.palette;
    let cursor = picker.cursor;
    let entries = picker.entries.clone();

    let area = f.area();

    // Reserve enough height to show all entries (plus 2 for the border), but
    // cap at the terminal height minus 4 rows so the popup never fills the
    // screen completely.
    let content_rows = entries.len().max(1); // at least 1 for the empty message
    let height =
        crate::cast::u16_sat(content_rows.min((area.height as usize).saturating_sub(4)) + 2);
    let width = 72u16.min(area.width.saturating_sub(2));

    let popup_area = centered_rect(width, height, area);
    f.render_widget(Clear, popup_area);

    let block = Block::default()
        .title(" Outline (j/k navigate, Enter jump, Esc dismiss) ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(p.border_focused))
        .style(Style::default().bg(p.help_bg));

    let inner = block.inner(popup_area);
    f.render_widget(block, popup_area);

    let visible_rows = inner.height as usize;

    if entries.is_empty() {
        let msg = Line::from(Span::styled(
            "  no headings in this document",
            Style::default().fg(p.dim),
        ));
        f.render_widget(Paragraph::new(vec![msg]), inner);
        return;
    }

    let scroll_offset = if cursor < visible_rows {
        0
    } else {
        cursor - visible_rows + 1
    };

    let rows: Vec<Line> = entries
        .iter()
        .enumerate()
        .skip(scroll_offset)
        .take(visible_rows)
        .map(|(i, entry)| {
            let is_cursor = i == cursor;

            let bullet_style = if is_cursor {
                Style::default().fg(p.accent).add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(p.dim)
            };
            let prefix_style = if is_cursor {
                Style::default().fg(p.accent).add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(p.dim)
            };
            let text_style = if is_cursor {
                Style::default()
                    .fg(p.selection_fg)
                    .bg(p.selection_bg)
                    .add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(p.foreground)
            };

            let bullet = if is_cursor { " > " } else { "   " };
            let prefix = entry.display_prefix();

            Line::from(vec![
                Span::styled(bullet, bullet_style),
                Span::styled(prefix, prefix_style),
                Span::styled(entry.label.clone(), text_style),
            ])
        })
        .collect();

    f.render_widget(Paragraph::new(rows), inner);
}

/// Handle a key event when the outline picker is focused.
///
/// Returns `true` when the picker should remain open.
pub fn handle_key(app: &mut App, code: crossterm::event::KeyCode) -> bool {
    match code {
        crossterm::event::KeyCode::Char('j') | crossterm::event::KeyCode::Down => {
            if let Some(p) = app.outline_picker.as_mut() {
                p.move_down();
            }
            true
        }
        crossterm::event::KeyCode::Char('k') | crossterm::event::KeyCode::Up => {
            if let Some(p) = app.outline_picker.as_mut() {
                p.move_up();
            }
            true
        }
        crossterm::event::KeyCode::Enter => {
            // Read the target line from the selected entry, close the picker,
            // then jump. We close first so the picker borrow is released before
            // we call `scroll_to_cursor_centered`, which needs `&mut self`.
            let target_line = app
                .outline_picker
                .as_ref()
                .and_then(|p| p.entries.get(p.cursor))
                .map(|e| e.line);
            app.outline_picker = None;
            if let Some(line) = target_line {
                let vh = app.tabs.view_height;
                if let Some(tab) = app.tabs.active_tab_mut() {
                    tab.view.cursor_line = line;
                    tab.view.scroll_to_cursor_centered(vh);
                }
            }
            false
        }
        // `o` closes the outline picker (same key that opened it — a second
        // press is a natural dismiss gesture). `Esc` and `q` also dismiss.
        crossterm::event::KeyCode::Esc
        | crossterm::event::KeyCode::Char('q')
        | crossterm::event::KeyCode::Char('o') => {
            app.outline_picker = None;
            false
        }
        _ => true,
    }
}

// ── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    /// Build a minimal `OutlinePickerState` directly from raw tuples
    /// `(anchor_slug, line, level)` so tests don't need a full `App`.
    fn picker_from_raw(entries: &[(&str, u32, u8)]) -> OutlinePickerState {
        let entries = entries
            .iter()
            .map(|(slug, line, level)| OutlineEntry {
                label: slug.replace('-', " "),
                level: *level,
                line: *line,
            })
            .collect();
        OutlinePickerState { entries, cursor: 0 }
    }

    /// A synthesised set of three headings should produce exactly 3 entries in
    /// document order.
    #[test]
    fn state_built_from_blocks_lists_every_heading() {
        let picker = picker_from_raw(&[
            ("introduction", 0, 1),
            ("background", 5, 2),
            ("results", 12, 2),
        ]);
        assert_eq!(picker.entries.len(), 3);
        assert_eq!(picker.entries[0].label, "introduction");
        assert_eq!(picker.entries[1].label, "background");
        assert_eq!(picker.entries[2].label, "results");
    }

    /// Each entry's `line` must match the source line so `handle_key` jumps to
    /// the correct display row.
    #[test]
    fn entries_carry_line_number_for_jump() {
        let picker = picker_from_raw(&[
            ("first-heading", 0, 1),
            ("second-heading", 7, 2),
            ("third-heading", 15, 3),
        ]);
        assert_eq!(picker.entries[0].line, 0);
        assert_eq!(picker.entries[1].line, 7);
        assert_eq!(picker.entries[2].line, 15);
    }

    /// `move_down` past the last entry clamps; `move_up` past 0 clamps.
    #[test]
    fn cursor_moves_with_jk_clamped_to_bounds() {
        let mut picker = picker_from_raw(&[("a", 0, 1), ("b", 3, 2), ("c", 6, 3)]);

        // Move to the last entry.
        picker.move_down();
        picker.move_down();
        assert_eq!(picker.cursor, 2);

        // One more down should clamp at 2, not wrap.
        picker.move_down();
        assert_eq!(picker.cursor, 2, "cursor must clamp at last entry");

        // Move back to first.
        picker.move_up();
        picker.move_up();
        assert_eq!(picker.cursor, 0);

        // One more up should clamp at 0, not underflow.
        picker.move_up();
        assert_eq!(picker.cursor, 0, "cursor must clamp at first entry");
    }

    /// A document with no headings produces an empty entries list.
    #[test]
    fn empty_doc_produces_zero_entries() {
        let picker = picker_from_raw(&[]);
        assert_eq!(picker.entries.len(), 0);
    }

    /// `display_prefix` produces the correct indentation and hashes per level.
    #[test]
    fn display_prefix_matches_level() {
        let e1 = OutlineEntry {
            label: "x".into(),
            level: 1,
            line: 0,
        };
        let e2 = OutlineEntry {
            label: "x".into(),
            level: 2,
            line: 0,
        };
        let e3 = OutlineEntry {
            label: "x".into(),
            level: 3,
            line: 0,
        };

        assert_eq!(e1.display_prefix(), "# ");
        assert_eq!(e2.display_prefix(), "  ## ");
        assert_eq!(e3.display_prefix(), "    ### ");
    }
}
