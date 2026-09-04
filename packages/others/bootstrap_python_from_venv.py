"""Bootstrap unpinned Python project metadata from source imports and a venv."""

import ast
import importlib.util
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

try:
    import tomllib
except ImportError:
    _TOMLI_PATH = Path(__file__).resolve().parent / "_vendor" / "tomli"
    _TOMLI_SPEC = importlib.util.spec_from_file_location(
        "_toolbox_tomli",
        _TOMLI_PATH / "__init__.py",
        submodule_search_locations=[str(_TOMLI_PATH)],
    )
    if _TOMLI_SPEC is None or _TOMLI_SPEC.loader is None:
        raise ImportError(f"Could not load bundled Tomli from {_TOMLI_PATH}")
    tomllib = importlib.util.module_from_spec(_TOMLI_SPEC)
    sys.modules[_TOMLI_SPEC.name] = tomllib
    _TOMLI_SPEC.loader.exec_module(tomllib)


SKIP_DIRECTORY_NAMES = {
    ".env",
    ".git",
    ".hg",
    ".mypy_cache",
    ".nox",
    ".pytest_cache",
    ".ruff_cache",
    ".svn",
    ".tox",
    ".venv",
    "__pycache__",
    "build",
    "dist",
    "env",
    "node_modules",
    "site-packages",
    "venv",
}

EXCLUDED_DISTRIBUTIONS = {"pip", "setuptools", "uv", "wheel"}


@dataclass(frozen=True)
class BootstrapPlan:
    """Contain a completely validated bootstrap write plan.

    Attributes:
        project (Path): Existing project root receiving generated files.
        dependencies (list[str]): Sorted unpinned distribution names.
        unresolved (list[str]): Sorted imported modules without venv mappings.
        files (dict[Path, str]): Complete validated output contents.
    """

    project: Path
    dependencies: list[str]
    unresolved: list[str]
    files: dict[Path, str]


def normalize_distribution_name(name: str) -> str:
    """Normalize a distribution name using PEP 503 comparison rules.

    Args:
        name (str): Distribution metadata name.

    Returns:
        str: Lowercase name with runs of hyphens, underscores, and dots
        normalized to one hyphen.

    Raises:
        TypeError: If name is not a string.
    """
    if not isinstance(name, str):
        raise TypeError("name must be a string")
    return re.sub(r"[-_.]+", "-", name).lower()


def resolve_venv(input_path: Path) -> Path:
    """Resolve a venv root and validate its activation script and interpreter.

    Args:
        input_path (Path): Venv root containing bin/activate and bin/python.

    Returns:
        Path: Absolute, symlink-resolved venv root.

    Raises:
        FileNotFoundError: If input_path is not an existing directory.
        ValueError: If required venv files are missing.
    """
    path = input_path.expanduser()
    if not path.is_dir():
        raise FileNotFoundError(f"Venv directory does not exist: {path}")
    path = path.resolve()
    if not (path / "bin" / "activate").is_file():
        raise ValueError(f"Venv is missing {path / 'bin' / 'activate'}")
    python_path = path / "bin" / "python"
    if not python_path.is_file() or not os.access(python_path, os.X_OK):
        raise ValueError(f"Venv is missing executable {python_path}")
    return path


def query_venv_metadata(venv: Path) -> dict:
    """Query distribution and standard-library metadata using the venv Python.

    Args:
        venv (Path): Validated venv root. Its bin/python interpreter executes a
            read-only metadata probe.

    Returns:
        dict: JSON-compatible metadata containing Python version, standard
        library module names, and distribution-to-import mappings.

    Raises:
        subprocess.CalledProcessError: If the venv interpreter probe fails.
        json.JSONDecodeError: If the probe returns malformed JSON.
        ValueError: If the probe omits required metadata.

    Examples:
        query_venv_metadata(venv)["distributions"] lists installed distributions
        without importing project dependencies.
    """
    probe = r"""
import importlib.metadata as metadata
import json
import pathlib
import pkgutil
import sys
import sysconfig

distributions = []
for distribution in metadata.distributions():
    name = distribution.metadata.get("Name")
    if not name:
        continue
    modules = set()
    top_level = distribution.read_text("top_level.txt")
    if top_level:
        modules.update(
            line.strip()
            for line in top_level.splitlines()
            if line.strip() and line.strip().isidentifier()
        )
    if not modules:
        for item in distribution.files or []:
            first = pathlib.PurePosixPath(str(item)).parts[0]
            if first.endswith(".py"):
                candidate = first[:-3]
            else:
                candidate = first
            if candidate.isidentifier():
                modules.add(candidate)
    distributions.append({"name": name, "modules": sorted(modules)})

stdlib = set(getattr(sys, "stdlib_module_names", set()))
stdlib.update(sys.builtin_module_names)
if not hasattr(sys, "stdlib_module_names"):
    stdlib_paths = {
        sysconfig.get_path("stdlib"),
        sysconfig.get_path("platstdlib"),
        sysconfig.get_config_var("DESTSHARED"),
    }
    for module in pkgutil.iter_modules(
        sorted(path for path in stdlib_paths if path)
    ):
        stdlib.add(module.name)
print(json.dumps({
    "python": [sys.version_info.major, sys.version_info.minor],
    "stdlib": sorted(stdlib),
    "distributions": distributions,
}))
"""
    result = subprocess.run(
        [str(venv / "bin" / "python"), "-c", probe],
        check=True,
        capture_output=True,
        text=True,
    )
    data = json.loads(result.stdout)
    if (
        not isinstance(data.get("python"), list)
        or len(data["python"]) != 2
        or not isinstance(data.get("stdlib"), list)
        or not isinstance(data.get("distributions"), list)
    ):
        raise ValueError("Venv metadata probe returned an incomplete result")
    return data


def collect_source_files(project: Path, scan_notebooks: bool) -> list[Path]:
    """Collect project Python files and optionally notebooks exactly once.

    Args:
        project (Path): Existing project root.
        scan_notebooks (bool): True includes .ipynb files; false scans only .py
            files. Skipped build, venv, VCS, and cache directories are identical
            in both modes.

    Returns:
        list[Path]: Sorted source file paths.

    Raises:
        FileNotFoundError: If project is not an existing directory.
        OSError: If project traversal fails.
    """
    if not project.is_dir():
        raise FileNotFoundError(f"Project directory does not exist: {project}")
    files: list[Path] = []
    for directory, names, filenames in os.walk(project, followlinks=False):
        names[:] = sorted(name for name in names if name not in SKIP_DIRECTORY_NAMES)
        for filename in sorted(filenames):
            path = Path(directory) / filename
            if path.suffix == ".py" or (scan_notebooks and path.suffix == ".ipynb"):
                files.append(path)
    return files


def extract_python_imports(source_text: str, source_path: Path) -> set[str]:
    """Extract absolute top-level imports from Python source.

    Args:
        source_text (str): Complete UTF-8 Python source.
        source_path (Path): Diagnostic filename supplied to the AST parser.

    Returns:
        set[str]: Imported top-level module names.

    Raises:
        SyntaxError: If source_text is malformed Python.
    """
    tree = ast.parse(source_text, filename=str(source_path))
    modules: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            modules.update(alias.name.split(".", 1)[0] for alias in node.names)
        elif (
            isinstance(node, ast.ImportFrom)
            and node.level == 0
            and node.module is not None
        ):
            modules.add(node.module.split(".", 1)[0])
    return modules


def extract_notebook_imports(notebook_path: Path) -> set[str]:
    """Extract imports from every code cell in one notebook.

    Args:
        notebook_path (Path): UTF-8 Jupyter notebook JSON file.

    Returns:
        set[str]: Imported top-level module names across all code cells.

    Raises:
        json.JSONDecodeError: If notebook JSON is malformed.
        TypeError: If cells or code-cell sources have invalid types.
        SyntaxError: If any code cell contains malformed Python.
        OSError: If the notebook cannot be read.
    """
    data = json.loads(notebook_path.read_text(encoding="utf-8"))
    cells = data.get("cells", [])
    if not isinstance(cells, list):
        raise TypeError(f"Notebook cells must be a list: {notebook_path}")
    modules: set[str] = set()
    for index, cell in enumerate(cells):
        if not isinstance(cell, dict):
            raise TypeError(f"Notebook cell {index} is not an object: {notebook_path}")
        if cell.get("cell_type") != "code":
            continue
        source = cell.get("source", "")
        if isinstance(source, list):
            if not all(isinstance(line, str) for line in source):
                raise TypeError(f"Notebook code cell {index} contains non-text source")
            source = "".join(source)
        if not isinstance(source, str):
            raise TypeError(f"Notebook code cell {index} source is not text")
        modules.update(
            extract_python_imports(source, Path(f"{notebook_path}:cell-{index}"))
        )
    return modules


def local_modules(project: Path) -> set[str]:
    """Infer local top-level modules in project-root and src layouts.

    Args:
        project (Path): Existing project root.

    Returns:
        set[str]: Module names excluded from external dependency mapping.

    Raises:
        OSError: If project or src entries cannot be inspected.
    """
    modules: set[str] = set()
    roots = [project]
    if (project / "src").is_dir():
        roots.append(project / "src")
    for root in roots:
        for child in root.iterdir():
            if child.name in SKIP_DIRECTORY_NAMES:
                continue
            if child.is_file() and child.suffix == ".py":
                modules.add(child.stem)
            elif child.is_dir() and child.name.isidentifier():
                modules.add(child.name)
    return modules


def infer_imports(
    project: Path,
    source_files: list[Path],
    stdlib: set[str],
) -> set[str]:
    """Parse a fixed source-file list and exclude standard and local modules.

    Args:
        project (Path): Existing project root.
        source_files (list[Path]): Single collected scan result containing .py
            and optional .ipynb paths.
        stdlib (set[str]): Standard-library module names from the selected venv.

    Returns:
        set[str]: External top-level imports needing distribution mapping.

    Raises:
        SyntaxError: If any Python source or notebook code cell is malformed.
        json.JSONDecodeError: If a notebook is malformed JSON.
        UnicodeDecodeError: If source text is not UTF-8.
        ValueError: If notebook structure is malformed.
        OSError: If source files cannot be read.
    """
    imported: set[str] = set()
    for path in source_files:
        if path.suffix == ".py":
            imported.update(
                extract_python_imports(path.read_text(encoding="utf-8"), path)
            )
        else:
            imported.update(extract_notebook_imports(path))
    imported.difference_update(stdlib)
    imported.difference_update(local_modules(project))
    return imported


def map_imports_to_distributions(
    imported: set[str],
    distributions: list[dict],
) -> tuple[list[str], list[str]]:
    """Map imports through selected-venv distribution metadata.

    Args:
        imported (set[str]): External top-level import names.
        distributions (list[dict]): Probe records containing each distribution
            name and its top-level modules.

    Returns:
        tuple[list[str], list[str]]: Sorted unpinned distribution names and
        sorted unresolved imports.

    Raises:
        TypeError: If distribution metadata fields have invalid types.
        ValueError: If an import maps ambiguously to multiple installed
            distributions. Ambiguity never selects a fallback.
    """
    module_map: dict[str, set[str]] = {}
    for distribution in distributions:
        name = distribution.get("name")
        modules = distribution.get("modules")
        if not isinstance(name, str) or not isinstance(modules, list):
            raise TypeError("Venv distribution metadata is malformed")
        for module in modules:
            if not isinstance(module, str):
                raise TypeError("Venv distribution module metadata is malformed")
            module_map.setdefault(module, set()).add(name)
    dependencies: set[str] = set()
    unresolved: list[str] = []
    ambiguous: dict[str, list[str]] = {}
    for module in sorted(imported):
        matches = sorted(
            module_map.get(module, set()),
            key=normalize_distribution_name,
        )
        if not matches:
            unresolved.append(module)
        elif len(matches) > 1:
            ambiguous[module] = matches
        elif normalize_distribution_name(matches[0]) not in EXCLUDED_DISTRIBUTIONS:
            dependencies.add(matches[0])
    if ambiguous:
        details = "; ".join(
            f"{module}: {', '.join(names)}"
            for module, names in sorted(ambiguous.items())
        )
        raise ValueError(f"Import-to-distribution mapping is ambiguous: {details}")
    return sorted(dependencies, key=normalize_distribution_name), unresolved


def make_project_name(project: Path) -> str:
    """Create a distribution-like project name from a directory name.

    Args:
        project (Path): Project root whose basename supplies the name.

    Returns:
        str: Lowercase name containing valid distribution-name characters.

    Raises:
        ValueError: If no valid characters remain.
    """
    name = re.sub(r"[^A-Za-z0-9._-]+", "-", project.name).strip("-._").lower()
    if not name:
        raise ValueError("Could not infer a project name")
    return name


def format_dependencies(dependencies: list[str]) -> str:
    """Format unpinned distribution names as a TOML array.

    Args:
        dependencies (list[str]): Sorted distribution names without versions.

    Returns:
        str: Multi-line dependencies assignment.

    Raises:
        ValueError: If a dependency contains unsupported control characters.
    """
    lines = ["dependencies = ["]
    for dependency in dependencies:
        if any(character in dependency for character in "\r\n"):
            raise ValueError(f"Invalid distribution name: {dependency!r}")
        escaped = dependency.replace("\\", "\\\\").replace('"', '\\"')
        lines.append(f'    "{escaped}",')
    lines.append("]")
    return "\n".join(lines)


def find_table_span(text: str, header: str) -> Optional[tuple[int, int]]:
    """Find one top-level TOML table text span.

    Args:
        text (str): Valid TOML document text.
        header (str): Exact header such as [project] or [tool.uv].

    Returns:
        Optional[tuple[int, int]]: Start and end offsets, or None when absent.

    Raises:
        ValueError: If header is not a simple TOML table header.
    """
    if re.fullmatch(r"\[[A-Za-z0-9_.-]+\]", header) is None:
        raise ValueError(f"Invalid TOML table header: {header}")
    target_path = {
        "[project]": ("project",),
        "[tool.uv]": ("tool", "uv"),
    }.get(header)
    if target_path is None:
        raise ValueError(f"Unsupported TOML table header: {header}")
    masked = mask_toml_strings_and_comments(text)
    matches = list(re.finditer(r"^[ \t]*\[\[?[^\n]*\]\]?[ \t]*$", masked, re.MULTILINE))
    for index, match in enumerate(matches):
        original_header = text[match.start() : match.end()]
        probe_key = "__my_toolbox_table_probe__"
        try:
            parsed = tomllib.loads(f"{original_header}\n{probe_key} = true\n")
        except tomllib.TOMLDecodeError:
            continue
        container: object = parsed
        for component in target_path:
            if not isinstance(container, dict) or component not in container:
                break
            container = container[component]
        else:
            if isinstance(container, dict) and container.get(probe_key) is True:
                end = (
                    matches[index + 1].start()
                    if index + 1 < len(matches)
                    else len(text)
                )
                return match.start(), end
    return None


def mask_toml_strings_and_comments(text: str) -> str:
    """Mask TOML strings and comments while preserving structural offsets.

    Args:
        text (str): Valid TOML document text. Every masked character is
            replaced with a space, while newlines and syntax outside strings
            and comments remain unchanged.

    Returns:
        str: Text with the same length and newline positions as the input.

    Raises:
        ValueError: If a quoted string is unterminated. Callers normally parse
            the TOML before invoking this function, so this indicates invalid
            direct input.

    Examples:
        ``mask_toml_strings_and_comments('key = "[table]"')`` preserves
        ``key =`` but masks the quoted value, preventing false table matches.
    """
    masked = list(text)
    index = 0
    state = "normal"
    while index < len(text):
        if state == "comment":
            if text[index] == "\n":
                state = "normal"
            else:
                masked[index] = " "
            index += 1
            continue
        if state in {"basic", "literal"}:
            masked[index] = " "
            if state == "basic" and text[index] == "\\":
                index += 1
                if index < len(text) and text[index] != "\n":
                    masked[index] = " "
            elif (state == "basic" and text[index] == '"') or (
                state == "literal" and text[index] == "'"
            ):
                state = "normal"
            index += 1
            continue
        if state in {"multiline-basic", "multiline-literal"}:
            delimiter = '"""' if state == "multiline-basic" else "'''"
            if text.startswith(delimiter, index):
                masked[index : index + 3] = [" ", " ", " "]
                index += 3
                state = "normal"
                continue
            if text[index] != "\n":
                masked[index] = " "
            if state == "multiline-basic" and text[index] == "\\":
                index += 1
                if index < len(text) and text[index] != "\n":
                    masked[index] = " "
            index += 1
            continue
        if text[index] == "#":
            masked[index] = " "
            state = "comment"
            index += 1
            continue
        if text.startswith('"""', index):
            masked[index : index + 3] = [" ", " ", " "]
            state = "multiline-basic"
            index += 3
            continue
        if text.startswith("'''", index):
            masked[index : index + 3] = [" ", " ", " "]
            state = "multiline-literal"
            index += 3
            continue
        if text[index] == '"':
            masked[index] = " "
            state = "basic"
        elif text[index] == "'":
            masked[index] = " "
            state = "literal"
        index += 1
    if state not in {"normal", "comment"}:
        raise ValueError("Unterminated TOML string")
    return "".join(masked)


def find_assignment_span(table: str, key: str) -> Optional[tuple[int, int]]:
    """Find a complete bare-key assignment outside TOML strings and comments.

    Args:
        table (str): Complete valid TOML table text including its header.
        key (str): Valid bare key whose assignment should be located.

    Returns:
        Optional[tuple[int, int]]: Start and end offsets covering the entire
        assignment, including a trailing newline when present, or None.

    Raises:
        ValueError: If key is invalid or a located assignment cannot be parsed.

    Examples:
        A multi-line array assignment returns one span covering every array
        line, while matching key-like text inside a quoted value is ignored.
    """
    if re.fullmatch(r"[A-Za-z0-9_-]+", key) is None:
        raise ValueError(f"Invalid TOML key: {key}")
    masked = mask_toml_strings_and_comments(table)
    match = re.search(rf"^[ \t]*{re.escape(key)}[ \t]*=", masked, re.MULTILINE)
    if match is None:
        return None
    start = match.start()
    end = table.find("\n", match.end())
    while True:
        if end == -1:
            end = len(table)
        else:
            end += 1
        candidate = table[start:end]
        try:
            tomllib.loads("[probe]\n" + candidate)
        except tomllib.TOMLDecodeError:
            if end == len(table):
                raise ValueError(f"Could not parse TOML assignment for {key}")
            end = table.find("\n", end)
            continue
        return start, end


def replace_scalar(table: str, key: str, raw_value: str) -> str:
    """Replace or insert one scalar key in a TOML table.

    Args:
        table (str): Complete simple TOML table text including its header.
        key (str): Bare key to update.
        raw_value (str): Already formatted TOML value.

    Returns:
        str: Updated table ending with one newline.

    Raises:
        ValueError: If key is not a supported bare key.
    """
    if re.fullmatch(r"[A-Za-z0-9_-]+", key) is None:
        raise ValueError(f"Invalid scalar key: {key}")
    span = find_assignment_span(table, key)
    replacement = f"{key} = {raw_value}\n"
    if span is not None:
        start, end = span
        return table[:start] + replacement + table[end:]
    lines = table.rstrip().splitlines()
    lines.insert(1, replacement.rstrip())
    return "\n".join(lines) + "\n"


def replace_dependencies(table: str, dependencies: list[str]) -> str:
    """Replace or insert project dependencies in a TOML table.

    Args:
        table (str): Complete [project] table text.
        dependencies (list[str]): Unpinned distribution names.

    Returns:
        str: Updated project table ending with one newline.

    Raises:
        ValueError: If table does not begin with [project].
    """
    header_end = table.find("\n") + 1
    if header_end == 0:
        raise ValueError("Expected a complete [project] table")
    span = find_assignment_span(table, "dependencies")
    replacement = format_dependencies(dependencies) + "\n"
    if span is not None:
        start, end = span
        return table[:start] + replacement + table[end:]
    insert_at = header_end
    for candidate in ("name", "version", "requires-python"):
        candidate_span = find_assignment_span(table, candidate)
        if candidate_span is not None:
            insert_at = max(insert_at, candidate_span[1])
    return table[:insert_at] + replacement + table[insert_at:]


def update_pyproject(
    text: str,
    project: Path,
    dependencies: list[str],
    python_version: tuple[int, int],
    restrict_uv_environment: bool,
) -> str:
    """Preserve unrelated TOML while updating generated project metadata.

    Args:
        text (str): Existing valid TOML or an empty string.
        project (Path): Project root used only when a project table is absent.
        dependencies (list[str]): Sorted unpinned dependency names.
        python_version (tuple[int, int]): Selected venv major and minor version.
        restrict_uv_environment (bool): True inserts or updates tool.uv
            environments for the current platform; false leaves tool.uv content
            unchanged and avoids adding the table.

    Returns:
        str: Updated, validated TOML document ending with one newline.

    Raises:
        ValueError: If existing or generated TOML is malformed.
    """
    try:
        tomllib.loads(text)
    except tomllib.TOMLDecodeError as error:
        raise ValueError(f"Malformed pyproject.toml: {error}") from error
    major, minor = python_version
    requires_python = f'">={major}.{minor},<{major}.{minor + 1}"'
    project_span = find_table_span(text, "[project]")
    if project_span is None:
        prefix = text.rstrip()
        table = (
            "[project]\n"
            f'name = "{make_project_name(project)}"\n'
            'version = "0.1.0"\n'
            f"requires-python = {requires_python}\n"
            f"{format_dependencies(dependencies)}\n"
        )
        updated = (prefix + "\n\n" if prefix else "") + table
    else:
        start, end = project_span
        table = text[start:end].rstrip() + "\n"
        table = replace_scalar(table, "requires-python", requires_python)
        table = replace_dependencies(table, dependencies)
        updated = text[:start] + table + text[end:]
    if restrict_uv_environment:
        marker = (
            f"sys_platform == '{sys.platform}' and "
            f"platform_machine == '{platform.machine()}'"
        )
        raw_value = '["' + marker.replace('"', '\\"') + '"]'
        uv_span = find_table_span(updated, "[tool.uv]")
        if uv_span is None:
            updated = updated.rstrip() + f"\n\n[tool.uv]\nenvironments = {raw_value}\n"
        else:
            start, end = uv_span
            table = replace_scalar(
                updated[start:end].rstrip() + "\n",
                "environments",
                raw_value,
            )
            updated = updated[:start] + table + updated[end:]
    updated = updated.rstrip() + "\n"
    try:
        tomllib.loads(updated)
    except tomllib.TOMLDecodeError as error:
        raise ValueError(f"Generated pyproject.toml is malformed: {error}") from error
    return updated


def prepare_bootstrap(
    project: Path,
    venv: Path,
    scan_notebooks: bool,
    restrict_uv_environment: bool,
) -> BootstrapPlan:
    """Scan once, map imports through a venv, and validate every output.

    Args:
        project (Path): Existing project root.
        venv (Path): Existing venv root containing bin/activate and bin/python.
        scan_notebooks (bool): True includes notebook code cells; false scans
            only Python files, reducing work and excluding notebook-only imports.
        restrict_uv_environment (bool): True writes a current-platform tool.uv
            restriction; false preserves any existing tool.uv configuration.

    Returns:
        BootstrapPlan: Complete no-write plan for all four generated files.

    Raises:
        FileNotFoundError: If project or venv is missing.
        SyntaxError: If any scanned Python source is malformed.
        ValueError: If TOML, notebooks, metadata, or distribution mapping is
            malformed or ambiguous.
        subprocess.CalledProcessError: If the venv metadata probe fails.
        OSError: If project files cannot be traversed or read.

    Examples:
        prepare_bootstrap(project, venv, True, True) includes notebooks and adds
        the current-platform uv restriction.
    """
    if not project.is_dir():
        raise FileNotFoundError(f"Project directory does not exist: {project}")
    project = project.resolve()
    venv = resolve_venv(venv)
    pyproject_path = project / "pyproject.toml"
    output_paths = (
        project / "requirements.inferred.txt",
        project / "requirements.txt",
        pyproject_path,
        project / ".python-version",
    )
    for output_path in output_paths:
        if output_path.is_symlink():
            raise ValueError(
                f"Generated output path must not be a symlink: {output_path}"
            )
        if output_path.exists() and not output_path.is_file():
            raise ValueError(
                f"Generated output path must be a regular file: {output_path}"
            )
    existing_toml = (
        pyproject_path.read_text(encoding="utf-8") if pyproject_path.exists() else ""
    )
    try:
        tomllib.loads(existing_toml)
    except tomllib.TOMLDecodeError as error:
        raise ValueError(f"Malformed pyproject.toml: {error}") from error
    source_files = collect_source_files(project, scan_notebooks)
    metadata = query_venv_metadata(venv)
    imported = infer_imports(project, source_files, set(metadata["stdlib"]))
    dependencies, unresolved = map_imports_to_distributions(
        imported,
        metadata["distributions"],
    )
    python_version = (int(metadata["python"][0]), int(metadata["python"][1]))
    requirements = "\n".join(dependencies)
    if requirements:
        requirements += "\n"
    files = {
        project / "requirements.inferred.txt": requirements,
        project / "requirements.txt": requirements,
        pyproject_path: update_pyproject(
            existing_toml,
            project,
            dependencies,
            python_version,
            restrict_uv_environment,
        ),
        project / ".python-version": f"{python_version[0]}.{python_version[1]}\n",
    }
    return BootstrapPlan(project, dependencies, unresolved, files)


def atomic_write_text(path: Path, content: str) -> None:
    """Atomically replace one UTF-8 project file.

    Args:
        path (Path): Destination in an existing project directory.
        content (str): Complete validated file content.

    Returns:
        None.

    Raises:
        ValueError: If path is a symbolic link. Generated outputs must be
            regular files so replacement cannot silently remove a link.
        OSError: If temporary writing, permission preservation, or replacement
            fails.
    """
    if path.is_symlink():
        raise ValueError(f"Generated output path must not be a symlink: {path}")
    existing_mode = path.stat().st_mode & 0o777 if path.exists() else 0o644
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.",
        dir=path.parent,
        text=True,
    )
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        temporary_path.chmod(existing_mode)
        os.replace(temporary_path, path)
    finally:
        if temporary_path.exists():
            temporary_path.unlink()


def write_bootstrap_files(
    plan: BootstrapPlan,
    create_backups: bool,
) -> list[Path]:
    """Write all prepared files with optional pre-write backups.

    Args:
        plan (BootstrapPlan): Fully validated output plan.
        create_backups (bool): True copies every conflicting existing output to
            a unique timestamped sibling before any write; false creates none.

    Returns:
        list[Path]: Created backup paths.

    Raises:
        FileExistsError: If a unique backup path unexpectedly exists.
        OSError: If backups or atomic per-file replacements fail. No cross-file
            rollback is claimed after writing begins.
    """
    backups: list[Path] = []
    timestamp = (
        time.strftime("%Y%m%d-%H%M%S") + f"-{time.time_ns() % 1_000_000_000:09d}"
    )
    if create_backups:
        for path in plan.files:
            if not path.exists():
                continue
            backup = path.with_name(f"{path.name}.bak.{timestamp}")
            if backup.exists():
                raise FileExistsError(f"Backup already exists: {backup}")
            shutil.copy2(path, backup)
            backups.append(backup)
    for path, content in plan.files.items():
        atomic_write_text(path, content)
    return backups


def prompt_yes_no(question: str, default: Optional[bool]) -> bool:
    """Prompt for a yes/no answer with an explicit documented default.

    Args:
        question (str): Prompt text without the answer suffix.
        default (Optional[bool]): True makes Enter yes, False makes Enter no, and
            None rejects Enter.

    Returns:
        bool: Parsed answer.

    Raises:
        ValueError: If the answer is empty without a default or not yes/no.
        EOFError: If input closes before an answer.
    """
    suffix = " [Y/n]" if default is True else " [y/N]" if default is False else " [y/n]"
    answer = input(question + suffix + ": ").strip().lower()
    if not answer:
        if default is None:
            raise ValueError("An explicit yes/no answer is required")
        return default
    if answer in {"y", "yes"}:
        return True
    if answer in {"n", "no"}:
        return False
    raise ValueError(f"Invalid yes/no answer: {answer}")


def validate_uv() -> str:
    """Resolve and execute uv before any optional-lock mutation.

    Args:
        None.

    Returns:
        str: Resolved uv executable path.

    Raises:
        FileNotFoundError: If uv is absent from PATH.
        subprocess.CalledProcessError: If uv --version fails.
    """
    executable = shutil.which("uv")
    if executable is None:
        raise FileNotFoundError("uv was not found in PATH")
    subprocess.run(
        [executable, "--version"],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return executable


def run_interactive() -> None:
    """Collect options, preflight, confirm unresolved imports, and write files.

    Args:
        None.

    Returns:
        None.

    Raises:
        FileNotFoundError: If project, venv, or selected uv executable is missing.
        SyntaxError: If scanned source is malformed.
        ValueError: If input, TOML, notebooks, metadata, or mappings are invalid.
        subprocess.CalledProcessError: If metadata probing or uv execution fails.
        OSError: If files cannot be read, backed up, or written.
        EOFError: If interactive input closes before required answers.
    """
    project_text = input(f"Project directory [{Path.cwd()}]: ").strip()
    project = Path(project_text).expanduser() if project_text else Path.cwd()
    default_venv = os.environ.get("VIRTUAL_ENV", "")
    prompt = f"Venv path [{default_venv}]: " if default_venv else "Venv path: "
    venv_text = input(prompt).strip() or default_venv
    if not venv_text:
        raise ValueError("Venv path cannot be empty")
    scan_notebooks = prompt_yes_no("Scan notebooks too?", True)
    restrict_uv = prompt_yes_no(
        "Restrict uv resolution to the current platform?",
        True,
    )
    create_lock = prompt_yes_no("Run uv lock after writing?", False)
    uv_executable = validate_uv() if create_lock else None

    plan = prepare_bootstrap(project, Path(venv_text), scan_notebooks, restrict_uv)
    print("\nInferred unpinned dependencies:")
    if plan.dependencies:
        for dependency in plan.dependencies:
            print(f"  - {dependency}")
    else:
        print("  (none)")
    if plan.unresolved:
        print("\nUnresolved imports:")
        for module in plan.unresolved:
            print(f"  - {module}")
        if not prompt_yes_no("Continue with unresolved imports?", None):
            print("No changes made.")
            return

    conflicts = [path for path in plan.files if path.exists()]
    create_backups = False
    if conflicts:
        print("\nExisting generated files:")
        for path in conflicts:
            print(f"  - {path}")
        create_backups = prompt_yes_no("Create backups before replacement?", True)
    print("\nFiles to write:")
    for path in plan.files:
        print(f"  - {path}")
    if not prompt_yes_no("Write these files?", None):
        print("No changes made.")
        return
    backups = write_bootstrap_files(plan, create_backups)
    for backup in backups:
        print(f"[INFO] Backup created: {backup}")
    for path in plan.files:
        print(f"[OK] Wrote {path}")
    if uv_executable is not None:
        subprocess.run([uv_executable, "lock"], cwd=plan.project, check=True)
        print("[OK] Created uv.lock")


if __name__ == "__main__":
    if len(sys.argv) != 1:
        raise SystemExit("bootstrap-python-from-venv does not accept arguments")
    run_interactive()
