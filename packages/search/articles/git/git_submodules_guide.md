# Cloning and Pulling Git Submodules

Git submodules are separate repositories referenced by a parent repository. A
normal `git clone` may create their directories without downloading their
contents.

## Clone a repository and its submodules

For a new clone, use:

```bash
git clone --recurse-submodules <repository-url>
```

This clones the parent repository, initializes every submodule, and checks out
the exact submodule commits recorded by the parent repository. It also handles
nested submodules.

Example:

```bash
git clone --recurse-submodules https://github.com/example/project.git
```

## Populate submodules in an existing clone

If the repository is already cloned and the submodule directories are empty,
run this command from the repository root:

```bash
git submodule update --init --recursive
```

The options mean:

- `--init`: initializes submodules listed in `.gitmodules`.
- `--recursive`: also initializes nested submodules.
- `update`: checks out the exact commits expected by the parent repository.

## Pull later changes

To update the parent repository and its submodules to the commits recorded by
the newly pulled parent revision, run:

```bash
git pull --recurse-submodules
git submodule update --init --recursive
```

The second command is explicit and ensures newly added or nested submodules are
initialized.

## Check submodule status

```bash
git submodule status --recursive
```

Common status prefixes:

- No prefix: the submodule is at the recorded commit.
- `-`: the submodule is not initialized.
- `+`: the checked-out commit differs from the parent repository's recorded
  commit.
- `U`: the submodule has merge conflicts.

## Update to the latest remote submodule commits

The commands above reproduce the versions recorded by the parent repository.
They do not automatically select the newest commit from each submodule's remote
branch.

Only when you intentionally want newer remote commits, run:

```bash
git submodule update --remote --recursive
```

This changes the submodule commits in the parent working tree. Review and commit
those updated references in the parent repository if the change is intended.

## Common authentication issue

Submodules use the URLs stored in `.gitmodules`. If cloning a submodule fails,
verify that you have access to its repository and that its HTTPS or SSH URL is
usable in your environment:

```bash
git config --file .gitmodules --get-regexp 'submodule\..*\.url'
```
