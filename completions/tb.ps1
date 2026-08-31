Register-ArgumentCompleter -Native -CommandName tb -ScriptBlock {
    param($WordToComplete, $CommandAst, $CursorPosition)

    $Elements = @($CommandAst.CommandElements)
    if ($Elements.Count -gt 2) {
        return
    }
    if ($Elements.Count -eq 2 -and $Elements[1].Extent.EndOffset -lt $CursorPosition) {
        return
    }

    $EscapedPrefix = [WildcardPattern]::Escape($WordToComplete)
    tb __complete | Where-Object { $_ -like "$EscapedPrefix*" } | ForEach-Object {
        [Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
