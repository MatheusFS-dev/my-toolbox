Run this command to create a requirements.txt file for the current UV venv:

```bash
uv pip freeze > requirements.txt
uv init --bare .
uv python pin 3.12
uv add -r requirements.txt --no-sync
uv lock
```