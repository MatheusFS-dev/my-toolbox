#Requires AutoHotkey v2.0
#SingleInstance Force
Persistent

; Copy newly saved screenshot paths automatically using Windows directory notifications.
global ScreenshotPathPasteApp := 0
SetTimer LaunchScreenshotPathPaste, -1

GetLatestScreenshotPath(directory, &modifiedTime := 0) {
    latestPath := ""
    latestTime := 0
    attributes := Buffer(36, 0)
    Loop Files directory "\*", "F" {
        if !RegExMatch(A_LoopFileExt, "i)^(png|jpe?g|bmp|gif|webp|tiff?|heic)$")
            continue
        if !DllCall("GetFileAttributesExW", "wstr", A_LoopFileFullPath, "int", 0, "ptr", attributes)
            continue
        modified := NumGet(attributes, 20, "int64")
        if modified > latestTime || (modified = latestTime && StrCompare(A_LoopFileFullPath, latestPath) > 0) {
            latestTime := modified
            latestPath := A_LoopFileFullPath
        }
    }
    modifiedTime := latestTime
    return latestPath
}

; Add CF_UNICODETEXT without EmptyClipboard so existing image formats survive.
AddScreenshotPathToClipboard(path) {
    memory := DllCall("GlobalAlloc", "uint", 0x42, "uptr", (StrLen(path) + 1) * 2, "ptr")
    if !memory
        throw OSError()
    opened := false
    try {
        pointer := DllCall("GlobalLock", "ptr", memory, "ptr")
        if !pointer
            throw OSError()
        try StrPut(path, pointer, StrLen(path) + 1, "UTF-16")
        finally DllCall("GlobalUnlock", "ptr", memory)
        Loop 20 {
            if DllCall("OpenClipboard", "ptr", A_ScriptHwnd) {
                opened := true
                break
            }
            Sleep 10
        }
        if !opened
            throw OSError()
        if !DllCall("SetClipboardData", "uint", 13, "ptr", memory, "ptr")
            throw OSError()
        memory := 0 ; Windows owns the allocation after SetClipboardData succeeds.
    } finally {
        if opened
            DllCall("CloseClipboard")
        if memory
            DllCall("GlobalFree", "ptr", memory)
    }
}

class ScreenshotPathPaste {
    __New() {
        this.screenshotsDirectory := EnvGet("USERPROFILE") "\Pictures\Screenshots"
        this.stateDirectory := EnvGet("LOCALAPPDATA") "\my-toolbox\macros\screenshot-path-paste"
        this.configPath := this.stateDirectory "\settings.ini"
        this.launcherPath := this.stateDirectory "\launcher.ahk"
        this.diagnosticLog := this.stateDirectory "\diagnostics.log"
        this.shortcutPath := A_AppData "\Microsoft\Windows\Start Menu\Programs\Startup\my-toolbox Paste Screenshot Path.lnk"
        this.diagnosticsEnabled := false
        this.watcher := 0
        this.lastScreenshot := ""
    }

    Start() {
        this.screenshotsDirectory := IniRead(this.configPath, "ScreenshotPath", "Directory", "")
        if !DirExist(this.screenshotsDirectory) && !this.ConfigureDirectory()
            ExitApp 0
        this.ConfigureTray()
        this.RequestStartupConsent()
        this.WatchDirectory()
    }

    WatchDirectory() {
        if IsObject(this.watcher)
            this.watcher.Stop()
        this.watcher := ScreenshotDirectoryWatcher(this.screenshotsDirectory)
        this.CopyLatestPath()
    }

    Run() {
        loop {
            handles := Buffer(A_PtrSize)
            NumPut("ptr", this.watcher.event, handles)
            result := DllCall("MsgWaitForMultipleObjectsEx", "uint", 1, "ptr", handles, "uint", 0xFFFFFFFF, "uint", 0x04FF, "uint", 0x06, "uint")
            if result = 0
                this.ProcessChanges()
            else if result = 0xFFFFFFFF
                throw OSError()
            Sleep -1
        }
    }

    ProcessChanges() {
        if !this.watcher.Wait(0)
            return
        this.watcher.Rearm()
        try {
            path := GetLatestScreenshotPath(this.screenshotsDirectory, &modified)
            if path != "" && path "|" modified != this.lastScreenshot
                this.CopyLatestPath()
        } catch as error {
            TrayTip "Could not copy the screenshot path: " error.Message, "Paste Screenshot Path", 3
        }
    }

    ConfigureDirectory(*) {
        defaultPath := this.screenshotsDirectory != "" ? this.screenshotsDirectory : EnvGet("USERPROFILE") "\Pictures\Screenshots"
        loop {
            result := InputBox("Screenshot folder (required). Enter an existing full folder path.", "Paste Screenshot Path", "w560 h140", defaultPath)
            if result.Result != "OK"
                return false
            directory := Trim(result.Value, " `t`r`n" Chr(34))
            if StrLen(directory) > 3
                directory := RTrim(directory, "\/")
            if !RegExMatch(directory, "i)^(?:[a-z]:\\|\\\\)") || !DirExist(directory) {
                MsgBox "Enter an existing full screenshot folder path, or choose Cancel.", "Paste Screenshot Path", "Icon!"
                continue
            }
            DirCreate this.stateDirectory
            IniWrite directory, this.configPath, "ScreenshotPath", "Directory"
            this.screenshotsDirectory := directory
            if IsObject(this.watcher)
                this.WatchDirectory()
            return true
        }
    }

    CopyLatestPath() {
        path := GetLatestScreenshotPath(this.screenshotsDirectory, &modified)
        if path != "" {
            AddScreenshotPathToClipboard(path)
            this.lastScreenshot := path "|" modified
            this.Log("path-copied")
        }
        return path
    }

    ConfigureTray() {
        A_TrayMenu.Delete()
        A_TrayMenu.Add("Status", this.ShowStatus.Bind(this))
        A_TrayMenu.Add("Change screenshot folder", this.ConfigureDirectory.Bind(this))
        A_TrayMenu.Add("Enable diagnostic logging", this.ToggleDiagnostics.Bind(this))
        A_TrayMenu.Add("Enable at sign-in", this.EnableAtSignIn.Bind(this))
        A_TrayMenu.Add()
        A_TrayMenu.Add("Exit", this.Exit.Bind(this))
        A_TrayMenu.Add("Uninstall", this.Uninstall.Bind(this))
        A_TrayMenu.Default := "Status"
        A_IconTip := "Paste Screenshot Path: Watching screenshots"
    }

    RequestStartupConsent() {
        marker := this.stateDirectory "\startup-consent.txt"
        if FileExist(marker)
            return
        if !DirExist(this.screenshotsDirectory)
            return
        answer := MsgBox("Enable Paste Screenshot Path at sign-in? You can change this later from the tray menu.", "Paste Screenshot Path", "YN Default2 Icon?")
        DirCreate this.stateDirectory
        FileAppend(answer = "Yes" ? "enabled`n" : "declined`n", marker, "UTF-8")
        if answer = "Yes"
            this.EnableAtSignIn()
    }

    EnableAtSignIn(*) {
        try {
            DirCreate this.stateDirectory
            launcher := this.BuildLauncher()
            if FileExist(this.launcherPath)
                FileDelete this.launcherPath
            FileAppend launcher, this.launcherPath, "UTF-8"
            shell := ComObject("WScript.Shell")
            shortcut := shell.CreateShortcut(this.shortcutPath)
            shortcut.TargetPath := A_AhkPath
            quote := Chr(34)
            shortcut.Arguments := quote this.launcherPath quote
            shortcut.WorkingDirectory := this.stateDirectory
            shortcut.Description := "Paste Screenshot Path"
            shortcut.Save()
            this.Log("startup-enabled")
            TrayTip "Enabled at sign-in.", "Paste Screenshot Path", 2
        } catch as error {
            MsgBox "Could not enable at sign-in: " error.Message, "Paste Screenshot Path", "Iconx"
        }
    }

    BuildLauncher() {
        quote := Chr(34)
        runLine := "Run Chr(34) A_AhkPath Chr(34) " quote " " quote " Chr(34) target Chr(34)"
        if InStr(StrLower(A_ScriptFullPath), "\my-toolbox\versions\") {
            return "#Requires AutoHotkey v2.0`n"
                . "currentFile := EnvGet(" quote "LOCALAPPDATA" quote ") " quote "\my-toolbox\current.txt" quote "`n"
                . "if !FileExist(currentFile)`n    ExitApp 1`n"
                . "version := Trim(FileRead(currentFile))`n"
                . "if !RegExMatch(version, " quote "^[A-Za-z0-9._-]+$" quote ")`n    ExitApp 1`n"
                . "target := EnvGet(" quote "LOCALAPPDATA" quote ") " quote "\my-toolbox\versions\" quote " version " quote "\packages\macros\autohotkey\paste_screenshot_path.ahk" quote "`n"
                . "if !FileExist(target)`n    ExitApp 1`n"
                . runLine "`n"
        }
        escaped := StrReplace(A_ScriptFullPath, quote, quote quote)
        return "#Requires AutoHotkey v2.0`n"
            . "target := " quote escaped quote "`n"
            . "if !FileExist(target)`n    ExitApp 1`n"
            . runLine "`n"
    }

    ShowStatus(*) {
        MsgBox "Screenshot folder: " this.screenshotsDirectory "`nNew screenshot paths are copied to the clipboard automatically. Paste with Ctrl+V.", "Paste Screenshot Path", "Iconi"
    }

    ToggleDiagnostics(*) {
        this.diagnosticsEnabled := !this.diagnosticsEnabled
        if this.diagnosticsEnabled
            A_TrayMenu.Check("Enable diagnostic logging")
        else
            A_TrayMenu.Uncheck("Enable diagnostic logging")
        if this.diagnosticsEnabled {
            DirCreate this.stateDirectory
            this.Log("diagnostics-enabled")
        }
    }

    Log(event) {
        if !this.diagnosticsEnabled
            return
        try FileAppend FormatTime(, "yyyy-MM-dd HH:mm:ss") " event=" event "`n", this.diagnosticLog, "UTF-8"
    }

    Exit(*) {
        ExitApp 0
    }

    Uninstall(*) {
        answer := MsgBox("Remove Paste Screenshot Path settings, startup files, and diagnostic logs?", "Paste Screenshot Path", "YN Default2 Icon!")
        if answer != "Yes"
            return
        try {
            if FileExist(this.shortcutPath)
                FileDelete this.shortcutPath
            if DirExist(this.stateDirectory)
                DirDelete this.stateDirectory, true
            ExitApp 0
        } catch as error {
            MsgBox "Could not uninstall: " error.Message, "Paste Screenshot Path", "Iconx"
        }
    }
}

; Windows signals this handle when a file is created, renamed, or written.
; No polling timer, asynchronous buffers, or image processing is needed.
class ScreenshotDirectoryWatcher {
    __New(directory) {
        this.event := DllCall("FindFirstChangeNotificationW", "wstr", directory, "int", false, "uint", 0x19, "ptr")
        if !this.event || this.event = -1
            throw OSError()
    }

    Wait(timeout) {
        return DllCall("WaitForSingleObject", "ptr", this.event, "uint", timeout, "uint") = 0
    }

    Rearm() {
        if !DllCall("FindNextChangeNotification", "ptr", this.event)
            throw OSError()
    }

    Stop() {
        if this.event && this.event != -1 {
            DllCall("FindCloseChangeNotification", "ptr", this.event)
            this.event := 0
        }
    }

    __Delete() {
        this.Stop()
    }
}

RunSelfTest() {
    originalClipboard := ClipboardAll()
    directory := A_Temp "\tb-screenshot-test-" DllCall("GetCurrentProcessId")
    DirCreate directory
    try {
        Assert(GetLatestScreenshotPath(directory) = "", "empty folder")
        FileAppend "old", directory "\old.png"
        FileSetTime "20200101000000", directory "\old.png", "M"
        FileAppend "new", directory "\new screenshot.PNG"
        FileSetTime "20210101000000", directory "\new screenshot.PNG", "M"
        FileAppend "ignored", directory "\notes.txt"
        Assert(GetLatestScreenshotPath(directory) = directory "\new screenshot.PNG", "newest image regardless of clipboard or age")
        FileDelete directory "\new screenshot.PNG"
        Assert(GetLatestScreenshotPath(directory) = directory "\old.png", "deleted newest file")
        DirCreate directory "\folder.png"
        Assert(GetLatestScreenshotPath(directory) = directory "\old.png", "ignore directories")
        Assert(GetLatestScreenshotPath(directory "\missing") = "", "missing folder")
        app := ScreenshotPathPaste()
        app.screenshotsDirectory := directory
        savedClipboard := ClipboardAll()
        try {
            A_Clipboard := "unrelated clipboard text"
            Assert(app.CopyLatestPath() = directory "\old.png", "copy newest path")
            Assert(A_Clipboard = directory "\old.png", "clipboard contains full path")
            ; A one-pixel DIB models the image format supplied by Windows screenshots.
            image := DllCall("GlobalAlloc", "uint", 0x42, "uptr", 44, "ptr")
            pixels := DllCall("GlobalLock", "ptr", image, "ptr")
            NumPut("uint", 40, "int", 1, "int", 1, "ushort", 1, "ushort", 32, pixels)
            NumPut("uint", 0x123456, pixels, 40)
            DllCall("GlobalUnlock", "ptr", image)
            Assert(DllCall("OpenClipboard", "ptr", A_ScriptHwnd), "open image clipboard")
            try {
                DllCall("EmptyClipboard")
                Assert(DllCall("SetClipboardData", "uint", 8, "ptr", image, "ptr"), "seed screenshot image")
            } finally DllCall("CloseClipboard")
            app.CopyLatestPath()
            Assert(A_Clipboard = directory "\old.png", "path accompanies screenshot image")
            Assert(DllCall("IsClipboardFormatAvailable", "uint", 8), "screenshot image remains available")
            Assert(DllCall("OpenClipboard", "ptr", A_ScriptHwnd), "open preserved image")
            try {
                preserved := DllCall("GetClipboardData", "uint", 8, "ptr")
                pixels := DllCall("GlobalLock", "ptr", preserved, "ptr")
                try Assert(NumGet(pixels, 40, "uint") = 0x123456, "screenshot pixels remain unchanged")
                finally DllCall("GlobalUnlock", "ptr", preserved)
            } finally DllCall("CloseClipboard")
        } finally A_Clipboard := savedClipboard
        app.stateDirectory := directory
        app.configPath := directory "\settings.ini"
        IniWrite directory, app.configPath, "ScreenshotPath", "Directory"
        FileAppend "declined", directory "\startup-consent.txt"
        app.screenshotsDirectory := ""
        app.Start()
        Assert(app.screenshotsDirectory = directory, "load saved screenshot folder on startup")
        app.Start()
        Assert(app.screenshotsDirectory = directory, "restart without a stale mutex")
        savedClipboard := ClipboardAll()
        try {
            A_Clipboard := "ordinary copied text"
            FileAppend "screenshot", directory "\automatic.png"
            Assert(app.watcher.Wait(5000), "new screenshot signals watcher without a hotkey")
            app.ProcessChanges()
            Assert(A_Clipboard = directory "\automatic.png", "new screenshot path is copied automatically")
            A_Clipboard := "keep ordinary text"
            FileAppend "unrelated", directory "\notes.txt"
            Assert(app.watcher.Wait(5000), "directory event")
            app.ProcessChanges()
            Assert(A_Clipboard = "keep ordinary text", "unrelated file changes preserve clipboard")
        } finally {
            app.watcher.Stop()
            A_Clipboard := savedClipboard
        }
        Assert(InStr(app.BuildLauncher(), "#Requires AutoHotkey v2.0"), "startup launcher")
    } finally {
        A_Clipboard := originalClipboard
        DirDelete directory, true
    }
}

Assert(condition, message) {
    if !condition
        throw Error("Self-test failed: " message)
}

LaunchScreenshotPathPaste() {
    global ScreenshotPathPasteApp
    for argument in A_Args {
        if argument = "--self-test" {
            OnError((*) => ExitApp(1))
            RunSelfTest()
            ExitApp 0
        }
    }
    ScreenshotPathPasteApp := ScreenshotPathPaste()
    OnExit(StopScreenshotWatcher)
    ScreenshotPathPasteApp.Start()
    ScreenshotPathPasteApp.Run()
}

StopScreenshotWatcher(*) {
    global ScreenshotPathPasteApp
    if IsObject(ScreenshotPathPasteApp) && IsObject(ScreenshotPathPasteApp.watcher)
        ScreenshotPathPasteApp.watcher.Stop()
}
