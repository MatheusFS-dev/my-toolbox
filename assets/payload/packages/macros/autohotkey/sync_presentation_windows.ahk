#Requires AutoHotkey v2.0
#SingleInstance Force
Persistent

global notesWindow := 0
global slidesWindow := 0
global syncing := false
global setup := Gui(, "Sync Presentation Windows")
setup.SetFont("s10", "Segoe UI")
setup.AddText("w500", "Keep your presentation and notes in two separate windows. This tool sends the same slide key to both windows when either one is focused.")
setup.AddText("xm y+16 w500", "1. Click Capture notes, then click the notes window within 3 seconds.")
global notesLabel := setup.AddText("xm y+6 w500", "Notes: not selected")
setup.AddButton("xm y+6 w150", "Capture notes").OnEvent("Click", (*) => CaptureWindow("notes"))
setup.AddText("xm y+16 w500", "2. Click Capture presentation, then click the slide window within 3 seconds.")
global slidesLabel := setup.AddText("xm y+6 w500", "Presentation: not selected")
setup.AddButton("xm y+6 w150", "Capture presentation").OnEvent("Click", (*) => CaptureWindow("slides"))
setup.AddText("xm y+16 w500", "3. Click Start syncing. Use Page Up for next slide and Page Down for previous slide.")
setup.AddText("xm y+6 w500", "Only keys pressed in a selected window are shared. Focus briefly switches to the other window, then returns. A small flash may be visible.")
global statusLabel := setup.AddText("xm y+16 w500", "Status: stopped")
global toggleButton := setup.AddButton("xm y+8 w150", "Start syncing")
toggleButton.OnEvent("Click", ToggleSync)
setup.AddButton("x+12 w110", "Exit").OnEvent("Click", (*) => ExitApp())
setup.OnEvent("Close", (*) => ExitApp())
setup.Show()

#HotIf ShouldSync()
$PgUp::ForwardKey("{PgDn}")
$PgDn::ForwardKey("{PgUp}")
#HotIf

CaptureWindow(which) {
    global setup, notesWindow, slidesWindow, notesLabel, slidesLabel, syncing
    if syncing {
        MsgBox "Stop syncing before changing either window.", "Sync Presentation Windows", "Iconi"
        return
    }
    setup.Hide()
    ToolTip "Click the " (which = "notes" ? "notes" : "presentation") " window now. Capturing in 3 seconds..."
    SetTimer FinishCapture.Bind(which), -3000
}

FinishCapture(which) {
    global setup, notesWindow, slidesWindow, notesLabel, slidesLabel
    ToolTip()
    window := WinGetID("A")
    if !window || window = setup.Hwnd || window = A_ScriptHwnd {
        setup.Show()
        MsgBox "No other window was selected. Try again and click the target window before the countdown ends.", "Sync Presentation Windows", "Icon!"
        return
    }
    if (which = "notes" && window = slidesWindow) || (which = "slides" && window = notesWindow) {
        setup.Show()
        MsgBox "Choose two different windows. Browser tabs in one window cannot be selected separately.", "Sync Presentation Windows", "Icon!"
        return
    }
    title := WinGetTitle("ahk_id " window)
    if which = "notes" {
        notesWindow := window
        notesLabel.Text := "Notes: " (title != "" ? title : "Window " window)
    } else {
        slidesWindow := window
        slidesLabel.Text := "Presentation: " (title != "" ? title : "Window " window)
    }
    setup.Show()
}

ToggleSync(*) {
    global notesWindow, slidesWindow, syncing, statusLabel, toggleButton
    if !syncing {
        if !notesWindow || !slidesWindow || !WinExist("ahk_id " notesWindow) || !WinExist("ahk_id " slidesWindow) {
            MsgBox "Capture both open windows before starting. If a window was closed, capture it again.", "Sync Presentation Windows", "Icon!"
            return
        }
    }
    syncing := !syncing
    statusLabel.Text := syncing ? "Status: syncing. Keep this window open; use Stop syncing when finished." : "Status: stopped"
    toggleButton.Text := syncing ? "Stop syncing" : "Start syncing"
}

ShouldSync() {
    global notesWindow, slidesWindow, syncing
    return syncing && notesWindow && slidesWindow
        && (WinActive("ahk_id " notesWindow) || WinActive("ahk_id " slidesWindow))
}

ForwardKey(key) {
    global notesWindow, slidesWindow, syncing, statusLabel, toggleButton
    if !syncing
        return
    original := WinGetID("A")
    target := original = notesWindow ? slidesWindow : notesWindow
    if !WinExist("ahk_id " target) {
        syncing := false
        statusLabel.Text := "Status: stopped. A selected window was closed; capture it again."
        toggleButton.Text := "Start syncing"
        return
    }
    try {
        Send key
        WinActivate "ahk_id " target
        if !WinWaitActive("ahk_id " target, , 1)
            throw Error("Windows could not focus the second window.")
        Send key
    }
    catch as error {
        syncing := false
        statusLabel.Text := "Status: stopped. Could not send a key to the other window."
        toggleButton.Text := "Start syncing"
        MsgBox "Could not send the slide key: " error.Message, "Sync Presentation Windows", "Icon!"
    } finally {
        if WinExist("ahk_id " original)
            WinActivate "ahk_id " original
    }
}
