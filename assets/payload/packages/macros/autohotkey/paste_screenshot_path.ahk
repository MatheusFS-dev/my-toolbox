#Requires AutoHotkey v2.0
#SingleInstance Force
Persistent

; Paste Screenshot Path correlates the current clipboard image with a file in
; Pictures\Screenshots. It never writes to the clipboard and only owns
; Ctrl+Shift+V while the correlation is current and unambiguous.

global ScreenshotPathPasteApp := 0

#HotIf IsScreenshotPathReady()
$^+v::PasteScreenshotPath()
#HotIf

IsScreenshotPathReady() {
    global ScreenshotPathPasteApp
    return IsObject(ScreenshotPathPasteApp) && ScreenshotPathPasteApp.IsActive()
}

PasteScreenshotPath() {
    global ScreenshotPathPasteApp
    if !ScreenshotPathPasteApp.RevalidateActiveAssociation() {
        Send "^+v"
        return
    }
    SendText ScreenshotPathPasteApp.active.path
}

HasSelfTestArgument() {
    for argument in A_Args {
        if argument = "--self-test"
            return true
    }
    return false
}

class ScreenshotPathPaste {
    static Inactive := "INACTIVE"
    static Pending := "PENDING"
    static Active := "ACTIVE"
    static Invalidated := "INVALIDATED"
    static CorrelationWindowMs := 5000
    static WatcherCadenceMs := 250
    static StabilityWaitMs := 200
    static MaxCandidates := 128
    static MaxEncodedBytes := 128 * 1024 * 1024
    static MaxPixels := 33554432

    __New() {
        this.state := ScreenshotPathPaste.Inactive
        this.pending := 0
        this.active := 0
        this.candidates := Map()
        this.directoryChanged := false
        this.diagnosticsEnabled := false
        this.watcher := 0
        this.gdiplusToken := 0
        this.mutex := 0
        this.screenshotsDirectory := EnvGet("USERPROFILE") "\Pictures\Screenshots"
        this.stateDirectory := EnvGet("LOCALAPPDATA") "\my-toolbox\macros\screenshot-path-paste"
        this.launcherPath := this.stateDirectory "\launcher.ahk"
        this.diagnosticLog := this.stateDirectory "\diagnostics.log"
        this.shortcutPath := A_AppData "\Microsoft\Windows\Start Menu\Programs\Startup\my-toolbox Paste Screenshot Path.lnk"
    }

    Start() {
        if !this.AcquireMutex() {
            ExitApp 0
        }
        this.StartGdiplus()
        this.ConfigureTray()
        if !DirExist(this.screenshotsDirectory) {
            this.SetState(ScreenshotPathPaste.Invalidated, "The Screenshots folder is unavailable.")
            return
        }
        this.RequestStartupConsent()
        OnClipboardChange(this.OnClipboardChange.Bind(this))
        this.watcher := DirectoryWatcher(this.screenshotsDirectory)
        if !this.watcher.Start() {
            this.SetState(ScreenshotPathPaste.Invalidated, "The Screenshots folder cannot be watched.")
            return
        }
        SetTimer this.Tick.Bind(this), ScreenshotPathPaste.WatcherCadenceMs
        this.SetState(ScreenshotPathPaste.Inactive, "Waiting for a screenshot.")
        this.OnClipboardChange(0)
    }

    AcquireMutex() {
        name := "Local\my-toolbox-paste-screenshot-path-" A_UserName
        this.mutex := DllCall("CreateMutexW", "ptr", 0, "int", false, "wstr", name, "ptr")
        return this.mutex && A_LastError != 183
    }

    StartGdiplus() {
        startupInput := Buffer(A_PtrSize = 8 ? 24 : 16, 0)
        NumPut("UInt", 1, startupInput, 0)
        status := DllCall("gdiplus\GdiplusStartup", "ptr*", &token := 0, "ptr", startupInput.Ptr, "ptr", 0, "uint")
        if status != 0
            throw Error("GDI+ initialization failed.")
        this.gdiplusToken := token
    }

    ConfigureTray() {
        A_TrayMenu.Delete()
        A_TrayMenu.Add("Status", this.ShowStatus.Bind(this))
        A_TrayMenu.Add("Enable diagnostic logging", this.ToggleDiagnostics.Bind(this))
        A_TrayMenu.Add("Enable at sign-in", this.EnableAtSignIn.Bind(this))
        A_TrayMenu.Add()
        A_TrayMenu.Add("Exit", this.Exit.Bind(this))
        A_TrayMenu.Add("Uninstall", this.Uninstall.Bind(this))
        A_TrayMenu.Default := "Status"
        this.UpdateTray()
    }

    RequestStartupConsent() {
        marker := this.stateDirectory "\startup-consent.txt"
        if FileExist(marker)
            return
        if !DirExist(this.screenshotsDirectory)
            return
        answer := MsgBox("Enable Paste Screenshot Path at sign-in? You can change this later from the tray menu.", "Paste Screenshot Path", "YN Icon?")
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
            TrayTip "Paste Screenshot Path", "Enabled at sign-in.", 2
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

    OnClipboardChange(*) {
        sequence := DllCall("GetClipboardSequenceNumber", "uint")
        signature := ClipboardImageSignature()
        if !IsObject(signature) {
            this.Invalidate("Clipboard changed to non-image content.")
            return
        }
        this.pending := {sequence: sequence, signature: signature, detectedTick: A_TickCount, detectedTime: A_Now, matches: Map()}
        this.active := 0
        this.candidates := Map()
        this.SetState(ScreenshotPathPaste.Pending, "Waiting for the screenshot file.")
        this.Log("clipboard-image-pending")
        this.ScanCandidates()
    }

    Tick() {
        if IsObject(this.pending) && A_TickCount - this.pending.detectedTick >= ScreenshotPathPaste.CorrelationWindowMs {
            this.ScanCandidates()
            this.FinalizePendingAssociation()
            return
        }
        if IsObject(this.watcher) && this.watcher.HasChanges() {
            this.directoryChanged := true
            this.Log("directory-change")
        }
        if this.directoryChanged || IsObject(this.pending) {
            this.directoryChanged := false
            this.ScanCandidates()
        }
    }

    ScanCandidates() {
        if !IsObject(this.pending) || !DirExist(this.screenshotsDirectory)
            return
        candidates := []
        Loop Files this.screenshotsDirectory "\*.*", "F" {
            if A_LoopFileSize > ScreenshotPathPaste.MaxEncodedBytes
                continue
            age := DateDiff(this.pending.detectedTime, A_LoopFileTimeModified, "Seconds")
            if Abs(age) > 5
                continue
            candidates.Push({path: A_LoopFileFullPath, size: A_LoopFileSize, modified: A_LoopFileTimeModified})
        }
        this.SortCandidates(candidates)
        if candidates.Length > ScreenshotPathPaste.MaxCandidates
            candidates.Length := ScreenshotPathPaste.MaxCandidates
        for candidate in candidates
            this.CheckCandidate(candidate)
    }

    SortCandidates(candidates) {
        Loop candidates.Length {
            outer := A_Index
            Loop candidates.Length - outer {
                inner := A_Index
                if candidates[inner].modified < candidates[inner + 1].modified {
                    swap := candidates[inner]
                    candidates[inner] := candidates[inner + 1]
                    candidates[inner + 1] := swap
                }
            }
        }
    }

    CheckCandidate(candidate) {
        if !IsObject(this.pending)
            return
        key := candidate.path
        if !this.candidates.Has(key) {
            this.candidates[key] := {size: candidate.size, modified: candidate.modified, observedTick: A_TickCount}
            return
        }
        observed := this.candidates[key]
        if observed.size != candidate.size || observed.modified != candidate.modified {
            this.candidates[key] := {size: candidate.size, modified: candidate.modified, observedTick: A_TickCount}
            return
        }
        if A_TickCount - observed.observedTick < ScreenshotPathPaste.StabilityWaitMs
            return
        signature := FileImageSignature(candidate.path)
        if !IsObject(signature) || !ImageSignaturesEqual(signature, this.pending.signature)
            return
        this.RecordCandidateMatch(candidate.path, candidate.size, candidate.modified, signature)
    }

    RecordCandidateMatch(path, size, modified, signature) {
        if !IsObject(this.pending)
            return
        this.pending.matches[path] := {size: size, modified: modified, signature: signature}
        if this.pending.matches.Count > 1
            this.Invalidate("More than one file matches the screenshot.")
    }

    FinalizePendingAssociation() {
        if !IsObject(this.pending)
            return
        if this.pending.matches.Count != 1 {
            this.Invalidate("Screenshot correlation expired without one matching file.")
            return
        }
        if DllCall("GetClipboardSequenceNumber", "uint") != this.pending.sequence {
            this.Invalidate("Clipboard changed during screenshot correlation.")
            return
        }
        clipboard := ClipboardImageSignature()
        if !IsObject(clipboard) || !ImageSignaturesEqual(clipboard, this.pending.signature) {
            this.Invalidate("Clipboard image changed during screenshot correlation.")
            return
        }
        for path, match in this.pending.matches
            break
        this.active := {path: path, sequence: this.pending.sequence, signature: match.signature, size: match.size, modified: match.modified}
        this.pending := 0
        this.SetState(ScreenshotPathPaste.Active, "Screenshot path is ready.")
        this.Log("screenshot-associated")
    }

    RevalidateActiveAssociation() {
        if !this.IsActive()
            return false
        if DllCall("GetClipboardSequenceNumber", "uint") != this.active.sequence {
            this.Invalidate("Clipboard sequence changed.")
            return false
        }
        clipboard := ClipboardImageSignature()
        if !IsObject(clipboard) || !ImageSignaturesEqual(clipboard, this.active.signature) {
            this.Invalidate("Clipboard image changed.")
            return false
        }
        if !FileExist(this.active.path) || FileGetSize(this.active.path) != this.active.size || FileGetTime(this.active.path, "M") != this.active.modified {
            this.Invalidate("Screenshot file changed or disappeared.")
            return false
        }
        file := FileImageSignature(this.active.path)
        if !IsObject(file) || !ImageSignaturesEqual(file, this.active.signature) {
            this.Invalidate("Screenshot file signature changed.")
            return false
        }
        return true
    }

    IsActive() {
        return this.state = ScreenshotPathPaste.Active && IsObject(this.active)
    }

    Invalidate(reason) {
        this.pending := 0
        this.active := 0
        this.candidates := Map()
        this.SetState(ScreenshotPathPaste.Invalidated, reason)
        this.Log("association-invalidated")
    }

    SetState(state, detail) {
        this.state := state
        this.detail := detail
        this.UpdateTray()
    }

    UpdateTray() {
        stateName := this.state
        A_IconTip := "Paste Screenshot Path: " stateName
    }

    ShowStatus(*) {
        MsgBox "State: " this.state "`n" this.detail, "Paste Screenshot Path", "Iconi"
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
        try FileAppend FormatTime(, "yyyy-MM-dd HH:mm:ss") " state=" this.state " event=" event "`n", this.diagnosticLog, "UTF-8"
    }

    Exit(*) {
        ExitApp 0
    }

    Uninstall(*) {
        answer := MsgBox("Remove Paste Screenshot Path startup files and diagnostic logs?", "Paste Screenshot Path", "YN Icon!")
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

class DirectoryWatcher {
    __New(path) {
        this.path := path
        this.handle := 0
        this.event := 0
        this.overlapped := Buffer(A_PtrSize = 8 ? 32 : 20, 0)
        this.buffer := Buffer(65536, 0)
    }

    Start() {
        this.handle := DllCall("CreateFileW", "wstr", this.path, "uint", 0x0001, "uint", 0x00000007, "ptr", 0, "uint", 3, "uint", 0x42000000, "ptr", 0, "ptr")
        if this.handle = -1 || !this.handle
            return false
        this.event := DllCall("CreateEventW", "ptr", 0, "int", false, "int", false, "ptr", 0, "ptr")
        if !this.event
            return false
        NumPut("ptr", this.event, this.overlapped, A_PtrSize = 8 ? 24 : 16)
        return this.Request()
    }

    Request() {
        bytes := 0
        return DllCall("ReadDirectoryChangesW", "ptr", this.handle, "ptr", this.buffer.Ptr, "uint", this.buffer.Size, "int", false, "uint", 0x0000001B, "uint*", &bytes, "ptr", this.overlapped.Ptr, "ptr", 0, "int") || A_LastError = 997
    }

    HasChanges() {
        if !this.handle || DllCall("WaitForSingleObject", "ptr", this.event, "uint", 0, "uint") != 0
            return false
        bytes := 0
        DllCall("GetOverlappedResult", "ptr", this.handle, "ptr", this.overlapped.Ptr, "uint*", &bytes, "int", false)
        NumPut("ptr", 0, this.overlapped, A_PtrSize = 8 ? 24 : 16)
        return this.Request()
    }

    __Delete() {
        if this.handle && this.handle != -1
            DllCall("CloseHandle", "ptr", this.handle)
        if this.event
            DllCall("CloseHandle", "ptr", this.event)
    }
}

ClipboardImageSignature() {
    if !DllCall("OpenClipboard", "ptr", 0, "int")
        return 0
    try {
        bitmap := DllCall("GetClipboardData", "uint", 2, "ptr")
        if !bitmap
            return 0
        status := DllCall("gdiplus\GdipCreateBitmapFromHBITMAP", "ptr", bitmap, "ptr", 0, "ptr*", &image := 0, "uint")
        if status != 0
            return 0
        try return GdipImageSignature(image)
        finally DllCall("gdiplus\GdipDisposeImage", "ptr", image)
    } finally DllCall("CloseClipboard")
}

FileImageSignature(path) {
    try {
        if FileGetSize(path) > ScreenshotPathPaste.MaxEncodedBytes
            return 0
        status := DllCall("gdiplus\GdipCreateBitmapFromFile", "wstr", path, "ptr*", &image := 0, "uint")
        if status != 0
            return 0
        try return GdipImageSignature(image)
        finally DllCall("gdiplus\GdipDisposeImage", "ptr", image)
    } catch {
        return 0
    }
}

GdipImageSignature(image) {
    width := 0, height := 0
    if DllCall("gdiplus\GdipGetImageWidth", "ptr", image, "uint*", &width, "uint") != 0 || DllCall("gdiplus\GdipGetImageHeight", "ptr", image, "uint*", &height, "uint") != 0
        return 0
    if !width || !height || width * height > ScreenshotPathPaste.MaxPixels
        return 0
    rect := Buffer(16, 0), data := Buffer(A_PtrSize = 8 ? 32 : 24, 0)
    NumPut("int", width, rect, 8), NumPut("int", height, rect, 12)
    pixelFormat := 0x26200A
    if DllCall("gdiplus\GdipBitmapLockBits", "ptr", image, "ptr", rect.Ptr, "uint", 1, "int", pixelFormat, "ptr", data.Ptr, "uint") != 0
        return 0
    try {
        stride := NumGet(data, 8, "int"), scan0 := NumGet(data, 16, "ptr")
        if !scan0 || Abs(stride) < width * 4
            return 0
        hash := Sha256Rows(scan0, stride, width * 4, height)
        return IsObject(hash) ? {width: width, height: height, hash: hash} : 0
    } finally DllCall("gdiplus\GdipBitmapUnlockBits", "ptr", image, "ptr", data.Ptr)
}

Sha256Rows(scan0, stride, rowBytes, height) {
    if DllCall("bcrypt\BCryptOpenAlgorithmProvider", "ptr*", &algorithm := 0, "wstr", "SHA256", "ptr", 0, "uint", 0, "uint") != 0
        return 0
    try {
        propertySize := 0
        DllCall("bcrypt\BCryptGetProperty", "ptr", algorithm, "wstr", "ObjectLength", "ptr", 0, "uint", 0, "uint*", &propertySize, "uint", 0)
        property := Buffer(propertySize, 0)
        if DllCall("bcrypt\BCryptGetProperty", "ptr", algorithm, "wstr", "ObjectLength", "ptr", property.Ptr, "uint", property.Size, "uint*", &propertySize, "uint", 0) != 0
            return 0
        objectLength := NumGet(property, 0, "uint")
        hashObject := Buffer(objectLength, 0)
        if DllCall("bcrypt\BCryptCreateHash", "ptr", algorithm, "ptr*", &hash := 0, "ptr", hashObject.Ptr, "uint", hashObject.Size, "ptr", 0, "uint", 0, "uint", 0, "uint") != 0
            return 0
        try {
            offset := stride < 0 ? (height - 1) * -stride : 0
            Loop height {
                if DllCall("bcrypt\BCryptHashData", "ptr", hash, "ptr", scan0 + offset, "uint", rowBytes, "uint", 0, "uint") != 0
                    return 0
                offset += stride
            }
            digest := Buffer(32, 0)
            if DllCall("bcrypt\BCryptFinishHash", "ptr", hash, "ptr", digest.Ptr, "uint", digest.Size, "uint", 0, "uint") != 0
                return 0
            result := ""
            Loop digest.Size
                result .= Format("{:02x}", NumGet(digest, A_Index - 1, "uchar"))
            return result
        } finally DllCall("bcrypt\BCryptDestroyHash", "ptr", hash)
    } finally DllCall("bcrypt\BCryptCloseAlgorithmProvider", "ptr", algorithm, "uint", 0)
}

ImageSignaturesEqual(left, right) {
    return IsObject(left) && IsObject(right) && left.width = right.width && left.height = right.height && left.hash = right.hash
}

RunSelfTest() {
    Assert(ImageSignaturesEqual({width: 10, height: 20, hash: "a"}, {width: 10, height: 20, hash: "a"}), "equal signatures")
    Assert(!ImageSignaturesEqual({width: 10, height: 20, hash: "a"}, {width: 10, height: 21, hash: "a"}), "different dimensions")
    app := ScreenshotPathPaste()
    app.SetState(ScreenshotPathPaste.Pending, "test")
    Assert(app.state = ScreenshotPathPaste.Pending, "pending state")
    app.Invalidate("test")
    Assert(app.state = ScreenshotPathPaste.Invalidated && !IsObject(app.active), "invalidation state")
    launcher := app.BuildLauncher()
    Assert(InStr(launcher, "#Requires AutoHotkey v2.0") && InStr(launcher, "Run Chr(34) A_AhkPath Chr(34)"), "launcher content")
    app.pending := {sequence: 1, signature: {width: 10, height: 20, hash: "a"}, detectedTick: A_TickCount, detectedTime: A_Now, matches: Map()}
    app.RecordCandidateMatch("one.png", 1, A_Now, app.pending.signature)
    app.RecordCandidateMatch("two.png", 1, A_Now, app.pending.signature)
    Assert(app.state = ScreenshotPathPaste.Invalidated, "ambiguous matches")
    Assert(!RegExMatch("../bad", "^[A-Za-z0-9._-]+$"), "malformed version")
    FileAppend "self-test passed`n", "*"
}

Assert(condition, message) {
    if !condition
        throw Error("Self-test failed: " message)
}

if HasSelfTestArgument() {
    RunSelfTest()
    ExitApp 0
}

ScreenshotPathPasteApp := ScreenshotPathPaste()
ScreenshotPathPasteApp.Start()
return
