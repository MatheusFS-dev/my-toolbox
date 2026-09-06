//! Clipboard boundary: native clipboard first, then a direct OSC 52 write.

use anyhow::Result;
use base64::Engine;
use std::io::Write;

/// Destination for exact code payloads. Tests inject a sink without a display server.
pub trait ClipboardSink {
    fn copy(&mut self, text: &str) -> Result<()>;
}

/// Retains native clipboard ownership for the lifetime of the application.
#[derive(Default)]
pub struct SystemClipboard {
    native: Option<arboard::Clipboard>,
}

impl ClipboardSink for SystemClipboard {
    fn copy(&mut self, text: &str) -> Result<()> {
        copy_with_fallback(
            text,
            |text| {
                if self.native.is_none() {
                    self.native = Some(arboard::Clipboard::new()?);
                }
                if let Some(native) = self.native.as_mut() {
                    native.set_text(text.to_owned())?;
                }
                Ok(())
            },
            &mut std::io::stdout().lock(),
        )
    }
}

/// Try native copying, then emit and flush OSC 52 through the supplied writer.
fn copy_with_fallback(
    text: &str,
    native: impl FnOnce(&str) -> Result<()>,
    output: &mut impl Write,
) -> Result<()> {
    if native(text).is_ok() {
        return Ok(());
    }
    let encoded = base64::engine::general_purpose::STANDARD.encode(text.as_bytes());
    write!(output, "\x1b]52;c;{encoded}\x07")?;
    output.flush()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toolbox_clipboard_native_success_skips_osc() {
        let mut output = Vec::new();
        let mut native_payload = String::new();
        copy_with_fallback(
            "x\n",
            |text| {
                native_payload = text.to_owned();
                Ok(())
            },
            &mut output,
        )
        .unwrap();
        assert_eq!(native_payload, "x\n");
        assert!(output.is_empty());
    }

    #[test]
    fn toolbox_clipboard_native_failure_emits_exact_osc_bytes() {
        let mut output = Vec::new();
        copy_with_fallback("a\n", |_| anyhow::bail!("no display"), &mut output).unwrap();
        assert_eq!(output, b"\x1b]52;c;YQo=\x07");
    }

    struct BrokenOutput {
        fail_flush: bool,
    }
    impl Write for BrokenOutput {
        fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
            if self.fail_flush {
                Ok(bytes.len())
            } else {
                Err(std::io::Error::other("closed"))
            }
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Err(std::io::Error::other("flush failed"))
        }
    }

    #[test]
    fn toolbox_clipboard_reports_write_and_flush_failures() {
        for fail_flush in [false, true] {
            assert!(
                copy_with_fallback(
                    "x",
                    |_| anyhow::bail!("no display"),
                    &mut BrokenOutput { fail_flush }
                )
                .is_err()
            );
        }
    }
}
