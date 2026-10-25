# Fix Git Showing All Files as Modified

If a copied Git repository suddenly shows many or all files as modified because of permission changes, disable executable-bit tracking.

For the main repository:

```bash
git config core.fileMode false
```

For all submodules, including nested ones:

```bash
git submodule foreach --recursive 'git config core.fileMode false'
```

This does not modify file contents or remove real code changes. It only makes Git ignore executable permission differences.