# Portable Monitor

Run a Python script once and receive a Gmail notification when it finishes or
fails. The package uses only the Python 3.9 or later standard library and does
not require installation. The target keeps its working directory, environment,
stdout, and stderr. Its exit status remains authoritative if email fails.

## Setup

1. Copy the `portable_monitor` directory and `example_submit.py` alongside your
   job files. Rename or copy `monitor-gmail.example.json` to
   `monitor-gmail.json` in the job's working directory.
2. Edit the wrapper to set the target script path and its arguments. Keep your
   scheduler's job directives and submission requirements in the appropriate
   job script. The example runs `target.py` with no arguments.
3. Fill in `sender` with your Gmail address, `app_password` with a Gmail app
   password, and `recipients` with a list of email addresses. Keep this file
   private and out of version control. The configuration path is relative to
   the job's working directory unless you pass an absolute `config_path`.
4. Run the wrapper with `python3 example_submit.py` against a small test script
   and confirm the message arrives before submitting a real job.

Google requires 2-Step Verification for app passwords and some accounts cannot
use them. See [Google's app password instructions](https://support.google.com/accounts/answer/185833?hl=en).
The compute node must be able to reach Gmail on port 465. Cluster network rules
may prevent delivery. Abrupt scheduler termination of the wrapper can prevent
the notification, so leave sufficient time for the target to exit and the email
to be sent. No target output is included in the message.

`AI_PROMPT.txt` contains a reusable prompt for adapting the wrapper to your
script and scheduler. Do not share your credentials with an AI assistant.
