use crate::markdown::CodeBlock;
use crate::text_layout::{hard_wrap_spans, measure};
use ratatui::text::{Line, Span};
use unicode_width::UnicodeWidthChar;

use super::WrappedTextLayout;

pub const COPY_BUTTON: &str = "[ Copy ]";

/// Build a header that gives the copy control priority over its language label.
pub fn code_header(code: &CodeBlock, width: u16, feedback: Option<&str>) -> Line<'static> {
    if width < 2 {
        return Line::from(if width == 1 { "╭" } else { "" });
    }
    let inner = usize::from(width - 2);
    let button = if inner >= COPY_BUTTON.len() {
        feedback.filter(|s| s.len() <= inner).unwrap_or(COPY_BUTTON)
    } else {
        ""
    };
    let available = inner.saturating_sub(button.len());
    let mut language = String::new();
    let mut used = 0;
    for ch in code.language.as_deref().unwrap_or("text").chars() {
        let cw = ch.width().unwrap_or(0);
        if used + cw > available.saturating_sub(1) {
            break;
        }
        language.push(ch);
        used += cw;
    }
    Line::from(vec![
        Span::styled(
            format!("╭{language}{}", "─".repeat(available - used)),
            code.border_style,
        ),
        Span::styled(button.to_owned(), code.body_style),
        Span::styled("╮", code.border_style),
    ])
}

/// Lay out code cards at the available width and map wrapped rows to source rows.
pub fn layout_code_card(code: &CodeBlock, width: u16) -> WrappedTextLayout {
    let mut wrapped = Vec::new();
    let mut physical_to_logical = Vec::new();
    let mut push = |line: Line<'static>, logical| {
        let row = hard_wrap_spans(&line.spans, width).remove(0);
        wrapped.push(row);
        physical_to_logical.push(logical);
    };
    push(Line::from(""), 0);
    push(code_header(code, width, None), 1);
    for (index, line) in code.highlighted.iter().enumerate() {
        for row in hard_wrap_spans(&line.spans, width.saturating_sub(2)) {
            let mut spans = vec![Span::styled(
                if width > 0 { "│" } else { "" },
                code.border_style,
            )];
            if width > 2 {
                // A wide glyph cannot fit in a one-cell body. Clip that display
                // row without altering the raw copy payload.
                if row.width <= width - 2 {
                    spans.extend(row.to_ratatui_line().spans);
                }
                let used = measure(&spans).saturating_sub(1);
                spans.push(Span::styled(
                    " ".repeat(usize::from((width - 2).saturating_sub(used))),
                    code.body_style,
                ));
            }
            if width > 1 {
                spans.push(Span::styled("│", code.border_style));
            }
            push(Line::from(spans), crate::cast::u32_sat(index + 2));
        }
    }
    let bottom = match width {
        0 => String::new(),
        1 => "╰".to_string(),
        _ => format!("╰{}╯", "─".repeat(usize::from(width - 2))),
    };
    push(
        Line::from(Span::styled(bottom, code.border_style)),
        crate::cast::u32_sat(code.highlighted.len() + 2),
    );
    push(
        Line::from(""),
        crate::cast::u32_sat(code.highlighted.len() + 3),
    );
    WrappedTextLayout {
        wrapped,
        physical_to_logical,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::MathMode;
    use crate::markdown::{DocBlock, renderer::render_markdown};
    use crate::theme::{Palette, Theme};

    #[test]
    fn toolbox_code_card_wrap_keeps_indentation_and_source_row_mapping() {
        let blocks = render_markdown(
            "```longlanguage\n    ab  cd  \n```\n",
            &Palette::from_theme(Theme::Default),
            Theme::Default,
            MathMode::Text,
        );
        let DocBlock::Text {
            code: Some(code), ..
        } = &blocks[0]
        else {
            panic!("code card");
        };
        let layout = layout_code_card(code, 6);
        let rows: Vec<String> = layout
            .wrapped
            .iter()
            .map(|r| r.spans.iter().map(|s| s.content.as_str()).collect())
            .collect();
        assert_eq!(&rows[2..5], ["│    │", "│ab  │", "│cd  │"]);
        assert_eq!(layout.physical_to_logical, [0, 1, 2, 2, 2, 3, 4]);
        assert_eq!(code_header(code, 10, None).to_string(), "╭[ Copy ]╮");
        assert_eq!(code_header(code, 12, None).to_string(), "╭l─[ Copy ]╮");
    }

    #[test]
    fn toolbox_code_card_handles_tiny_widths_and_wide_glyphs_without_overflow() {
        let blocks = render_markdown(
            "```界語long\n界界 a\t \n```\n",
            &Palette::from_theme(Theme::Default),
            Theme::Default,
            MathMode::Text,
        );
        let DocBlock::Text {
            code: Some(code), ..
        } = &blocks[0]
        else {
            panic!("code card");
        };
        for width in 0..50 {
            let layout = layout_code_card(code, width);
            assert!(
                layout.wrapped.iter().all(|row| row.width <= width),
                "width {width}: {layout:?}"
            );
            assert_eq!(layout.wrapped.len(), layout.physical_to_logical.len());
            let header = code_header(code, width, None).to_string();
            assert_eq!(header.contains("[ Copy ]"), width >= 10, "{header}");
        }
        assert_eq!(code.raw, "界界 a\t \n");
    }
}
