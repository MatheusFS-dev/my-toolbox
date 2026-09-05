Use a hard reset, then force-push `main` to both remotes.

First check your remote names:

```bash
git remote -v
```

Example: if your remotes are named:
```text
github = GitHub
origin = Jira/Bitbucket
```

To make `main` exactly equal to your local `dev` and overwrite `main` on both remotes:

```bash
git fetch --all --prune

git switch main

git reset --hard dev

git push --force-with-lease github main
git push --force-with-lease origin main
```


To delete the dev:

```bash
git push github --delete dev
git push origin --delete dev
```
