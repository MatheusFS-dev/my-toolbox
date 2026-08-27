# Releasing

## Version and tag policy

- Keep pyproject.toml project.version aligned with the Git tag.
- Release order:
  1. Bump project.version.
  2. Merge to main.
  3. Create tag v<same-version>.
  4. Push the tag.
- PyPI versions are immutable and cannot be reused.

## Maintainer checklist

1. Build distributions.

   python -m build

2. Validate distributions.

   twine check dist/*

3. Create and push the release tag.

   git tag vX.Y.Z
   git push origin vX.Y.Z

4. Confirm GitHub Actions Publish workflow completed successfully.

## Trusted Publisher settings

PyPI Trusted Publisher must match:

- Owner: MatheusFS-dev
- Repository: araras
- Workflow file: publish.yml
- Environment: pypi

Optional TestPyPI dry-run setup:

- Workflow file: publish-test.yml
- Environment: testpypi
