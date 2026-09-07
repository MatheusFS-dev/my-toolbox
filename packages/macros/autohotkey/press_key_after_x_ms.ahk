#Requires AutoHotkey v2.0

key := A_Args.Length >= 1 ? A_Args[1] : "Enter"
delayMs := A_Args.Length >= 2 ? Integer(A_Args[2]) : 12000000

Sleep delayMs
Send "{" key "}"
