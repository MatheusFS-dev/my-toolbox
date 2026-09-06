use super::App;
use crate::action::Action;
use crate::markdown::{DocBlock, TextBlockId};
use crate::ui::tabs::TabId;
use ratatui::layout::{Position, Rect};

/// A copy control in terminal coordinates, rebuilt from the visible frame.
#[derive(Debug, Clone, Copy)]
pub struct CodeCopyHitbox {
    pub tab_id: TabId,
    pub block_id: usize,
    pub text_id: TextBlockId,
    pub rect: Rect,
}

/// Feedback belongs to one code card and one copy attempt.
#[derive(Debug, Clone)]
pub struct CodeCopyFeedback {
    pub tab_id: TabId,
    pub block_id: usize,
    pub text_id: TextBlockId,
    pub generation: u64,
    pub success: bool,
}

impl CodeCopyFeedback {
    pub fn label(&self, width: u16) -> &'static str {
        match (self.success, width >= 13) {
            (true, true) => "[ Copied! ]",
            (false, true) => "[ Failed ]",
            (true, false) => "[Copied]",
            (false, false) => "[Failed]",
        }
    }
}

impl App {
    pub(super) fn try_copy_code_click(&mut self, column: u16, row: u16) -> bool {
        let hit = self
            .code_copy_hitboxes
            .iter()
            .find(|hit| hit.rect.contains(Position::new(column, row)))
            .copied();
        if let Some(hit) = hit {
            self.copy_code(hit.tab_id, hit.block_id, hit.text_id);
            return true;
        }
        false
    }

    /// Copy the first code card intersecting the viewport, even if its header is above it.
    pub(super) fn copy_first_visible_code(&mut self) {
        let Some(tab) = self.tabs.active_tab() else {
            return;
        };
        let mut start = 0;
        let mut target = None;
        let viewport_end = tab.view.scroll_offset.saturating_add(self.tabs.view_height);
        for block in &tab.view.rendered {
            let end = start + block.height();
            if end > tab.view.scroll_offset
                && start < viewport_end
                && let DocBlock::Text {
                    id,
                    code: Some(code),
                    ..
                } = block
            {
                target = Some((tab.id, code.block_id, *id));
                break;
            }
            start = end;
        }
        if let Some((tab_id, block_id, text_id)) = target {
            self.copy_code(tab_id, block_id, text_id);
        }
    }

    fn copy_code(&mut self, tab_id: TabId, block_id: usize, text_id: TextBlockId) {
        let Some(tab) = self.tabs.active_tab().filter(|tab| tab.id == tab_id) else {
            return;
        };
        let Some(raw) = tab.view.rendered.iter().find_map(|block| match block {
            DocBlock::Text {
                id,
                code: Some(code),
                ..
            } if *id == text_id && code.block_id == block_id => Some(code.raw.clone()),
            _ => None,
        }) else {
            return;
        };
        let success = self.clipboard.copy(&raw).is_ok();
        self.code_copy_generation = self.code_copy_generation.wrapping_add(1);
        let generation = self.code_copy_generation;
        self.code_copy_feedback = Some(CodeCopyFeedback {
            tab_id,
            block_id,
            text_id,
            generation,
            success,
        });
        if let Some(tx) = self.action_tx.clone()
            && let Ok(runtime) = tokio::runtime::Handle::try_current()
        {
            runtime.spawn(async move {
                tokio::time::sleep(std::time::Duration::from_millis(1500)).await;
                let _ = tx.send(Action::CodeCopyFeedbackExpired { generation });
            });
        }
    }
}
