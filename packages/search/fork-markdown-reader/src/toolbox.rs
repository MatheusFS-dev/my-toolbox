//! Internal contract used by the Go toolbox markdown preview.

use anyhow::{Context, Result, ensure};
use std::path::{Path, PathBuf};

pub const FOOTER: &str =
    "Esc/q:back j/k:scroll d/u:page gg/G:ends c:copy f:links o:outline Enter:open/close";

/// Resolve and read the single embedded Markdown document before terminal setup.
pub fn read_embedded_file(path: &Path) -> Result<(PathBuf, String)> {
    let path = path
        .canonicalize()
        .with_context(|| format!("could not resolve embedded file: {}", path.display()))?;
    ensure!(
        path.is_file(),
        "embedded input must be a regular file: {}",
        path.display()
    );
    ensure!(
        path.extension().is_some_and(|ext| {
            ext.eq_ignore_ascii_case("md") || ext.eq_ignore_ascii_case("markdown")
        }),
        "embedded input must be a Markdown file: {}",
        path.display()
    );
    let content = std::fs::read_to_string(&path)
        .with_context(|| format!("could not read embedded file: {}", path.display()))?;
    Ok((path, content))
}

/// Exercise the bundled renderers without terminal setup, clipboard, or user state.
pub fn self_check() -> Result<()> {
    use crate::config::{MathMode, MermaidTextBackend};
    use crate::markdown::{DocBlock, highlight};
    use crate::theme::{Palette, Theme};

    for theme in Theme::ALL.iter().copied().chain([Theme::ToolboxGithubDark]) {
        let palette = Palette::from_theme(theme);
        let blocks = crate::markdown::renderer::render_markdown(
            "# Check\n\n**text** [anchor](#check)\n\n```rust\nlet answer = 42;\n```\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n```mermaid\ngraph LR; A-->B\n```\n\n$$x^2$$\n",
            &palette,
            theme,
            MathMode::Image,
        );
        ensure!(
            blocks.iter().any(|b| matches!(b, DocBlock::Text { .. }))
                && blocks.iter().any(|b| matches!(b, DocBlock::Table(_)))
                && blocks.iter().any(|b| matches!(b, DocBlock::Mermaid { .. }))
                && blocks.iter().any(|b| matches!(b, DocBlock::Math { .. })),
            "Markdown parser self-check failed"
        );
        ensure!(
            highlight::SYNTAX_SET.find_syntax_by_token("rust").is_some(),
            "Rust grammar missing"
        );
        ensure!(
            highlight::THEME_SET
                .themes
                .contains_key(theme.syntax_theme_name()),
            "syntax theme missing"
        );
        let highlighted = highlight::highlight_code(
            "let answer = 42;",
            Some("rust"),
            theme.syntax_theme_name(),
            palette.foreground,
            palette.background,
        );
        ensure!(
            highlighted.len() == 1
                && highlighted[0].len() > 1
                && highlighted[0]
                    .iter()
                    .any(|(_, style)| style.fg != Some(palette.foreground)),
            "syntax highlighting self-check failed"
        );
    }

    let diagram = "graph LR\nA[Start] --> B[End]";
    let text = crate::mermaid::try_text_render_public(diagram, Some(80), MermaidTextBackend::Auto)
        .map_err(anyhow::Error::msg)?;
    ensure!(
        text.contains("Start") && text.contains("End"),
        "Mermaid text self-check failed"
    );
    let svg =
        mermaid_rs_renderer::render(diagram).map_err(|e| anyhow::anyhow!("Mermaid SVG: {e}"))?;
    ensure!(svg.contains("<svg"), "Mermaid SVG self-check failed");
    let image = crate::mermaid::svg_to_image(&svg, (13, 17, 23)).map_err(anyhow::Error::msg)?;
    ensure!(
        image.width() > 1 && image.height() > 1,
        "Mermaid raster self-check failed"
    );

    ensure!(
        crate::markdown::math::latex_to_unicode(r"\alpha^2") == "α²",
        "Unicode math self-check failed"
    );
    // RaTeX renders PNG directly. Its SVG-backed KaTeX accents become Path
    // display items: verify that geometry, then exercise the actual embedded-font
    // rasterizer with the same formula. No separate math SVG renderer is needed.
    let formula = r"\widehat{abc} + \frac{1}{2}";
    let ast = ratex_parser::parser::parse(formula)
        .map_err(|e| anyhow::anyhow!("math parse: {}", e.message))?;
    let layout = ratex_layout::layout(&ast, &ratex_layout::LayoutOptions::default());
    let display = ratex_layout::to_display_list(&layout);
    ensure!(
        display.items.iter().any(|item| matches!(item,
            ratex_types::DisplayItem::Path { commands, .. } if !commands.is_empty()
        )),
        "math SVG geometry self-check failed"
    );
    let png =
        crate::math_image::render_png(formula, (201, 209, 217)).map_err(anyhow::Error::msg)?;
    let image = image::load_from_memory_with_format(&png, image::ImageFormat::Png)?;
    ensure!(
        image.width() > 1
            && image.height() > 1
            && image.to_rgba8().pixels().any(|pixel| pixel[3] != 0),
        "math embedded-font self-check failed"
    );
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toolbox_embedded_reader_canonicalizes_and_reads_content() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("readme.md");
        std::fs::write(&path, "# Read me\n").unwrap();
        let (canonical, content) = read_embedded_file(&dir.path().join("./readme.md")).unwrap();
        assert_eq!(canonical, path.canonicalize().unwrap());
        assert_eq!(content, "# Read me\n");
        assert!(read_embedded_file(dir.path()).is_err());
        let wrong = dir.path().join("readme.txt");
        std::fs::write(&wrong, "# Read me").unwrap();
        assert!(read_embedded_file(&wrong).is_err());
        std::fs::write(&path, [0xff]).unwrap();
        assert!(read_embedded_file(&path).is_err());
    }

    #[cfg(unix)]
    #[test]
    fn toolbox_embedded_reader_rejects_unreadable_file_and_resolves_symlink() {
        use std::os::unix::fs::{PermissionsExt, symlink};
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("readme.md");
        std::fs::write(&path, "# Read me").unwrap();
        let link = dir.path().join("link.md");
        symlink(&path, &link).unwrap();
        assert_eq!(
            read_embedded_file(&link).unwrap().0,
            path.canonicalize().unwrap()
        );
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0)).unwrap();
        let result = read_embedded_file(&path);
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o600)).unwrap();
        assert!(result.is_err());
    }
}
