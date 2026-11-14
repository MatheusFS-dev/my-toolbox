#Requires AutoHotkey v2.0
#SingleInstance Force

installedScript := A_AppData "\Lightshot\Lightshot-keys.ahk"
startupShortcut := A_Startup "\Lightshot-keys.lnk"

if StrLower(A_ScriptFullPath) != StrLower(installedScript) {
    choice := MsgBox("Yes: install and start at sign-in.`nNo: uninstall and stop the keys.", "Lightshot keys", "YesNoCancel")
    if choice = "Yes" {
        TrayTip "Installing Lightshot keys", "Lightshot keys"
        DirCreate A_AppData "\Lightshot"
        FileCopy A_ScriptFullPath, installedScript, 1
        FileCreateShortcut A_AhkPath, startupShortcut, A_AppData "\Lightshot", '"' installedScript '"', "Lightshot key remapping"
        Run '"' A_AhkPath '" "' installedScript '"'
        MsgBox "Lightshot keys installed and set to start at sign-in.", "Lightshot keys"
    } else if choice = "No" {
        Uninstall(true)
    }
    ExitApp
}

A_TrayMenu.Add("Uninstall Lightshot keys", (*) => Uninstall())

Uninstall(confirmed := false) {
    installedScript := A_AppData "\Lightshot\Lightshot-keys.ahk"
    startupShortcut := A_Startup "\Lightshot-keys.lnk"
    if !confirmed && MsgBox("Remove Lightshot keys and its sign-in shortcut?", "Lightshot keys", "YesNo") != "Yes"
        return
    TrayTip "Uninstalling Lightshot keys", "Lightshot keys"
    DetectHiddenWindows true
    for hwnd in WinGetList("ahk_class AutoHotkey") {
        if InStr(StrLower(WinGetTitle(hwnd)), StrLower(installedScript)) && hwnd != A_ScriptHwnd
            WinClose hwnd
    }
    if FileExist(startupShortcut)
        FileDelete startupShortcut
    if FileExist(installedScript)
        FileDelete installedScript
    MsgBox "Lightshot keys removed from startup.", "Lightshot keys"
    if A_ScriptFullPath = installedScript
        ExitApp
}

*PrintScreen::Send "^!{F9}"
#+s::Send "^!{F9}"
#s::Run "ms-search:"
