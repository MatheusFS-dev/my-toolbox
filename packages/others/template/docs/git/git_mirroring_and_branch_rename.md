# What is actually mirrored

Git mirroring covers **commits, branches, tags, refs**. It does **not** mirror issues, PRs, wiki, releases metadata, permissions, CI settings.

## one `git push` pushes to both (best for daily use)

Configure the existing remote to have **multiple push URLs**. Git supports `pushurl` and pushes to all configured push URLs.

Example pattern (keep fetch from Bitbucket, push to both):

```bash
git remote set-url origin <bitbucket_repo_url>
git remote set-url --add --push origin <bitbucket_repo_url>
git remote set-url --add --push origin <github_repo_url>
```


---

# Rename branch `master` to `main`

If you are currently on `master`:

```bash
git branch -m main
```

If you are not on `master`:

```bash
git branch -m master main
```

(`git branch -m` renames a branch.)

## 2) Push `main` to each remote and set upstream

### If you only have `origin`

```bash
git push -u origin main
```

### If you also have a `github` remote

```bash
git push -u origin main
git push -u github main
```

## 3) Change the default branch on the hosting services. Example:

You must do this in **Bitbucket** and **GitHub**, otherwise they may keep expecting `master`.

* GitHub: Settings → Branches → Default branch → set to `main`
* Bitbucket Cloud: Repository settings → Branches → Main branch → set to `main`

## 4) Delete `master` on each remote (after default branch is switched)

```bash
git push origin --delete master
git push github --delete master   # if applicable
```