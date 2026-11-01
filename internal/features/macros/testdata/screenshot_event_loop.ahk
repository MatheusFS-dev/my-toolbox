SetTimer TestScreenshotEventLoop, -1

TestScreenshotEventLoop() {
    global testApp, testDirectory, testPath, testStage, testDeadline, testOriginalClipboard
    testOriginalClipboard := ClipboardAll()
    OnExit((*) => RestoreTestClipboard())
    testDirectory := A_Args.Length ? A_Args[1] : EnvGet("TB_SCREENSHOT_TEST_DIRECTORY")
    OnError(RecordScreenshotTestError)
    testPath := testDirectory "\delayed screenshot.png"
    testStage := 0
    testDeadline := A_TickCount + 5000
    testApp := ScreenshotPathPaste()
    testApp.stateDirectory := testDirectory
    testApp.configPath := testDirectory "\settings.ini"
    testApp.launcherPath := testDirectory "\saved-launcher.ahk"
    testApp.shortcutPath := testDirectory "\startup.lnk"
    IniWrite testDirectory, testApp.configPath, "ScreenshotPath", "Directory"
    order := A_Args.Length >= 2 ? A_Args[2] : "image-first"
    FileAppend order = "startup-repair" ? "enabled`r`n" : "declined", testDirectory "\startup-consent.txt"
    if order = "startup-repair"
        FileAppend "outdated launcher", testApp.launcherPath
    if order != "startup-locked"
        testApp.Start()
    if order = "startup-repair" {
        Assert(FileRead(testApp.launcherPath) = testApp.BuildLauncher(), "repair outdated startup launcher")
        Assert(FileExist(testApp.shortcutPath), "restore missing startup shortcut after saved consent")
    }
    if order = "image-first" || order = "startup-repair"
        PublishTestImage()
    if order = "clipboard-locked" || order = "startup-locked" {
        PublishTestImage()
        helper := testDirectory "\hold-clipboard.ahk"
        ready := testDirectory "\clipboard-locked.txt"
        quote := Chr(34)
        FileAppend '#Requires AutoHotkey v2.0`n'
            . 'if !DllCall("OpenClipboard", "ptr", A_ScriptHwnd)`n    ExitApp 1`n'
            . 'FileAppend "ready", "' ready '"`n'
            . 'Sleep 1200`nDllCall("CloseClipboard")`nExitApp 0`n', helper
        Run quote A_AhkPath quote " " quote helper quote, , "Hide", &helperPID
        OnExit((*) => StopClipboardHelper(helperPID))
        loop 100 {
            if FileExist(ready)
                break
            Sleep 10
        }
        Assert(FileExist(ready), "external process holds clipboard")
    }
    FileAppend "image fixture", testPath
    if order = "startup-locked"
        testApp.Start()
    SetTimer CheckScreenshotEventLoop, 25
    testApp.Run()
}

RecordScreenshotTestError(error, *) {
    global testDirectory
    FileAppend error.Message "`n" error.Stack, testDirectory "\failure.txt"
    ExitApp 1
}

CheckScreenshotEventLoop() {
    global testApp, testDirectory, testPath, testStage, testDeadline
    if A_TickCount > testDeadline
        ExitApp 2
    if testStage = 0 {
        if A_Clipboard != testPath
            return
        testStage := 1
        if A_Args.Length >= 2 && A_Args[2] = "file-first" {
            ; Windows may publish the image after the file watcher copied the path.
            PublishTestImage()
            return
        }
    }
    if testStage = 1 {
        if A_Clipboard != testPath
            return
        Assert(DllCall("IsClipboardFormatAvailable", "uint", 8), "image survives actual event loop")
        Assert(DllCall("OpenClipboard", "ptr", A_ScriptHwnd), "open preserved pixels")
        try {
            image := DllCall("GetClipboardData", "uint", 8, "ptr")
            pixels := DllCall("GlobalLock", "ptr", image, "ptr")
            try Assert(NumGet(pixels, 40, "uint") = 0x123456, "pixels survive clipboard updates")
            finally DllCall("GlobalUnlock", "ptr", image)
        } finally DllCall("CloseClipboard")
        testStage := 2
        A_Clipboard := "ordinary copied text"
        FileAppend "unrelated", testDirectory "\notes.txt"
        SetTimer FinishScreenshotEventLoop, -200
    }
}

FinishScreenshotEventLoop() {
    Assert(A_Clipboard = "ordinary copied text", "ordinary clipboard is preserved")
    PublishTestImage()
    SetTimer FinishOrdinaryImageTest, -200
}

FinishOrdinaryImageTest() {
    global testDirectory
    Assert(A_Clipboard = "", "ordinary image does not receive a stale screenshot path")
    FileAppend "passed", testDirectory "\passed.txt"
    ExitApp 0
}

StopClipboardHelper(pid) {
    if ProcessExist(pid)
        ProcessClose pid
}

PublishTestImage() {
    image := DllCall("GlobalAlloc", "uint", 0x42, "uptr", 44, "ptr")
    pixels := DllCall("GlobalLock", "ptr", image, "ptr")
    NumPut("uint", 40, "int", 1, "int", 1, "ushort", 1, "ushort", 32, pixels)
    NumPut("uint", 0x123456, pixels, 40)
    DllCall("GlobalUnlock", "ptr", image)
    Assert(DllCall("OpenClipboard", "ptr", A_ScriptHwnd), "open image clipboard")
    try {
        DllCall("EmptyClipboard")
        Assert(DllCall("SetClipboardData", "uint", 8, "ptr", image, "ptr"), "publish screenshot image")
    } finally DllCall("CloseClipboard")
}

RestoreTestClipboard() {
    global testOriginalClipboard
    A_Clipboard := testOriginalClipboard
}
