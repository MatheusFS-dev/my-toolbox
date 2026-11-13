# Portable Monitor

## How to install

Download `monitor-portable.zip` from the release and extract it. The extracted
`monitor-portable` folder is the complete module. Copy the whole folder into
your project, keeping its contents together. Python 3.9 or later is required.

Use `AI_PROMPT.txt` to ask an AI assistant to integrate the package with your
Python code.

## How to configure the JSON

Edit `monitor-gmail.json` in the copied folder:

- `sender`: your Gmail address.
- `app_password`: your Gmail app password.
- `recipients`: a list of email addresses to receive notifications.

Keep this file private and out of version control. The package looks for
`monitor-gmail.json` in the current working directory by default. If you run
code from another directory, pass the path to the copied file as `config_path`
when calling `portable_monitor.run`.

Gmail app passwords require 2-Step Verification and are not available for all
accounts. See [Google's instructions](https://support.google.com/accounts/answer/185833?hl=en).
The machine running the script must be able to connect to Gmail on port 465.
